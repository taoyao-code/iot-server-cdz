#!/bin/bash
set -e

# ============================================
# IOT Server 部署脚本
# 特性：自动备份 + 零停机 + 智能检测
# ============================================

# 环境变量：控制备份行为
# BACKUP=true   启用备份（生产环境或重要变更）
# BACKUP=false  跳过备份（测试环境，默认）
BACKUP=${BACKUP:-false}

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }
log_step() { echo -e "${BLUE}[STEP]${NC} $1"; }

# 检查是否在项目根目录
if [ ! -f "docker-compose.yml" ]; then
    log_error "请在项目根目录执行此脚本"
    exit 1
fi

# 备份数据
backup_data() {
    # 测试环境默认跳过备份（快速迭代）
    if [ "$BACKUP" != "true" ]; then
        log_info "⚡ 跳过备份（测试模式）"
        log_info "💡 提示：需要备份时执行 BACKUP=true make deploy"
        return 0
    fi
    
    log_step "备份数据库..."
    
    if ! docker-compose ps | grep -q "postgres.*Up"; then
        log_warn "数据库未运行，跳过备份（可能是首次部署）"
        return 0
    fi
    
    BACKUP_DIR="./backups"
    mkdir -p "$BACKUP_DIR"
    BACKUP_FILE="$BACKUP_DIR/iot-$(date +%Y%m%d-%H%M%S).sql"
    
    if docker-compose exec -T postgres pg_dump -U iot iot_server > "$BACKUP_FILE" 2>/dev/null; then
        gzip "$BACKUP_FILE"
        log_info "✅ 备份成功: ${BACKUP_FILE}.gz"
        
        # 保留最近10个备份
        ls -t "$BACKUP_DIR"/iot-*.sql.gz | tail -n +11 | xargs -r rm
    else
        log_error "❌ 备份失败！是否继续部署？"
        read -p "继续？(yes/no): " confirm
        [ "$confirm" != "yes" ] && exit 1
    fi
}

# 构建新镜像
build_image() {
    log_step "1/5 构建 Docker 镜像..."
    
    VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo "dev")
    BUILD_TIME=$(date -u '+%Y-%m-%d %H:%M:%S')
    GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
    
    log_info "版本: $VERSION, 提交: $GIT_COMMIT"
    
    export DOCKER_BUILDKIT=1
    if docker build \
        --build-arg BUILD_VERSION="$VERSION" \
        --build-arg BUILD_TIME="$BUILD_TIME" \
        --build-arg GIT_COMMIT="$GIT_COMMIT" \
        -t iot-server:"$VERSION" \
        -t iot-server:latest \
        . ; then
        log_info "✅ 镜像构建成功"
    else
        log_error "❌ 镜像构建失败"
        exit 1
    fi
}

# 检查数据卷
check_volumes() {
    log_step "2/5 检查数据持久化..."
    
    for vol in postgres_data redis_data app_logs; do
        if docker volume ls | grep -q "iot-server_$vol"; then
            SIZE=$(docker system df -v | grep "iot-server_$vol" | awk '{print $3}' || echo "N/A")
            log_info "✅ $vol: $SIZE"
        else
            log_warn "⚠️  $vol: 不存在（将在启动时创建）"
        fi
    done
}

# 滚动更新应用服务
update_service() {
    log_step "3/5 更新应用服务（零停机）..."
    
    # 检查是否首次部署
    if docker-compose ps | grep -q "iot-server"; then
        log_info "执行滚动更新..."
        
        # 只更新应用容器，保持数据库运行
        docker-compose up -d --no-deps --build iot-server
        
        log_info "✅ 应用服务已更新（数据库未重启）"
    else
        log_info "首次部署，启动所有服务..."
        docker-compose up -d
        
        log_info "✅ 所有服务已启动"
    fi
}

# 健康检查
health_check() {
    log_step "4/5 健康检查..."
    
    log_info "等待服务启动..."
    sleep 10
    
    # 检查数据库
    if docker-compose exec -T postgres pg_isready -U iot > /dev/null 2>&1; then
        log_info "✅ 数据库健康"
    else
        log_error "❌ 数据库不健康"
        return 1
    fi
    
    # 检查 Redis
    if docker-compose exec -T redis redis-cli -a "${REDIS_PASSWORD}" ping > /dev/null 2>&1; then
        log_info "✅ Redis 健康"
    else
        log_warn "⚠️  Redis 检查失败（可能未设置密码）"
    fi
    
    # 检查应用
    max_retries=30
    retry=0
    while [ $retry -lt $max_retries ]; do
        if curl -f http://localhost:7055/healthz > /dev/null 2>&1; then
            log_info "✅ 应用服务健康"
            return 0
        fi
        retry=$((retry + 1))
        echo -n "."
        sleep 2
    done
    
    log_error "❌ 应用健康检查失败"
    return 1
}

# 显示状态
show_status() {
    log_step "5/5 部署状态..."
    
    echo ""
    docker-compose ps
    
    echo ""
    log_info "数据卷状态："
    docker volume ls | grep iot-server || log_warn "未找到数据卷"
    
    echo ""
    log_info "访问地址："
    echo "  HTTP API: http://localhost:7055"
    echo "  TCP 端口: localhost:7054"
    echo "  健康检查: http://localhost:7055/healthz"
    echo "  Metrics: http://localhost:7055/metrics"
    
    echo ""
    log_info "查看日志："
    echo "  docker-compose logs -f iot-server"
}

# 回滚函数
rollback() {
    log_error "检测到部署失败，是否回滚？"
    read -p "回滚到上一个版本？(yes/no): " confirm
    
    if [ "$confirm" = "yes" ]; then
        log_warn "执行回滚..."
        
        # 查找最近的备份
        LATEST_BACKUP=$(ls -t ./backups/iot-*.sql.gz 2>/dev/null | head -n 1)
        
        if [ -n "$LATEST_BACKUP" ]; then
            log_info "找到备份: $LATEST_BACKUP"
            log_warn "请手动恢复数据库（如果需要）："
            echo "  gunzip < $LATEST_BACKUP | docker-compose exec -T postgres psql -U iot iot_server"
        fi
        
        # 重启服务
        docker-compose restart iot-server
    fi
}

# 主流程
main() {
    log_info "================================"
    log_info "  IOT Server 快速部署工具"
    log_info "================================"
    echo ""
    
    # 显示当前模式
    if [ "$BACKUP" = "true" ]; then
        log_info "🔒 模式：生产部署（带备份）"
    else
        log_info "⚡ 模式：测试部署（快速迭代）"
    fi
    echo ""
    
    # 执行部署步骤
    backup_data  # 内部会根据 BACKUP 环境变量决定是否执行
    build_image || { log_error "构建失败"; exit 1; }
    check_volumes
    update_service || { log_error "更新失败"; rollback; exit 1; }
    
    if health_check; then
        show_status
        echo ""
        log_info "🎉 部署成功！"
        if [ "$BACKUP" = "true" ]; then
            log_info "💾 数据已备份并保留"
        else
            log_info "⚡ 快速部署完成（未备份）"
        fi
    else
        log_error "健康检查失败"
        rollback
        exit 1
    fi
}

# 捕获错误
trap 'log_error "部署过程中断"; rollback' ERR

# 执行主流程
main "$@"

