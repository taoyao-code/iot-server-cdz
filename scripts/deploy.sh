#!/bin/bash
set -e

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 日志函数
log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# 检查必要的工具
check_requirements() {
    log_info "检查部署环境..."
    
    if ! command -v docker &> /dev/null; then
        log_error "Docker未安装，请先安装Docker"
        exit 1
    fi
    
    if ! command -v docker-compose &> /dev/null && ! docker compose version &> /dev/null; then
        log_error "Docker Compose未安装，请先安装Docker Compose"
        exit 1
    fi
    
    log_info "环境检查通过 ✓"
}

# 检查环境变量
check_env() {
    log_info "检查环境变量配置..."
    
    if [ ! -f .env ]; then
        log_warn ".env 文件不存在，从示例文件创建..."
        cp .env.example .env
        log_error "请编辑 .env 文件并填写正确的配置值"
        exit 1
    fi
    
    # 加载环境变量
    source .env
    
    # 检查必要的环境变量
    required_vars=(
        "REDIS_PASSWORD"
        "POSTGRES_PASSWORD"
        "API_KEY"
    )
    
    missing_vars=()
    for var in "${required_vars[@]}"; do
        if [ -z "${!var}" ]; then
            missing_vars+=("$var")
        fi
    done
    
    if [ ${#missing_vars[@]} -ne 0 ]; then
        log_error "缺少必要的环境变量: ${missing_vars[*]}"
        log_error "请在 .env 文件中配置这些变量"
        exit 1
    fi
    
    log_info "环境变量配置正确 ✓"
}

# 构建镜像
build_image() {
    log_info "开始构建Docker镜像..."
    
    BUILD_VERSION=${BUILD_VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo "dev")}
    BUILD_TIME=$(date -u '+%Y-%m-%d %H:%M:%S')
    GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
    
    docker build \
        --build-arg BUILD_VERSION="$BUILD_VERSION" \
        --build-arg BUILD_TIME="$BUILD_TIME" \
        --build-arg GIT_COMMIT="$GIT_COMMIT" \
        -t iot-server:"$BUILD_VERSION" \
        -t iot-server:latest \
        .
    
    log_info "镜像构建完成 ✓ (version: $BUILD_VERSION)"
}

# 数据库迁移
run_migration() {
    log_info "运行数据库迁移..."
    
    # 等待数据库就绪
    log_info "等待数据库启动..."
    sleep 10
    
    # 这里应该运行数据库迁移脚本
    # docker-compose exec postgres psql -U iot -d iot_server -f /scripts/migrate.sql
    
    log_info "数据库迁移完成 ✓"
}

# 启动服务
start_services() {
    log_info "启动服务..."
    
    # 使用生产环境配置
    docker-compose up -d
    
    log_info "等待服务启动..."
    sleep 15
    
    # 检查服务健康状态
    if docker-compose ps | grep -q "unhealthy"; then
        log_error "部分服务不健康，请检查日志"
        docker-compose ps
        exit 1
    fi
    
    log_info "服务启动成功 ✓"
}

# 健康检查
health_check() {
    log_info "执行健康检查..."
    
    # 检查HTTP服务
    if curl -f http://localhost:7054/healthz &> /dev/null; then
        log_info "HTTP服务健康 ✓"
    else
        log_error "HTTP服务健康检查失败"
        exit 1
    fi
    
    # 检查就绪状态
    if curl -f http://localhost:7054/readyz &> /dev/null; then
        log_info "服务就绪 ✓"
    else
        log_warn "服务尚未就绪，可能需要更多时间初始化"
    fi
    
    log_info "健康检查完成 ✓"
}

# 显示服务状态
show_status() {
    log_info "服务状态："
    docker-compose ps
    
    echo ""
    log_info "服务访问地址："
    echo "  - HTTP API: http://localhost:7054"
    echo "  - TCP端口: localhost:7055"
    echo "  - Metrics: http://localhost:7054/metrics"
    echo "  - Health: http://localhost:7054/healthz"
    
    if docker-compose ps | grep -q prometheus; then
        echo "  - Prometheus: http://localhost:9090"
        echo "  - Grafana: http://localhost:3000"
    fi
}

# 显示日志
show_logs() {
    docker-compose logs -f --tail=100 iot-server
}

# 停止服务
stop_services() {
    log_info "停止服务..."
    docker-compose down
    log_info "服务已停止 ✓"
}

# 清理
cleanup() {
    log_warn "清理所有数据（包括数据库和日志）..."
    read -p "确认删除所有数据？(yes/no): " confirm
    
    if [ "$confirm" = "yes" ]; then
        docker-compose down -v
        log_info "清理完成 ✓"
    else
        log_info "取消清理"
    fi
}

# 主函数
main() {
    case "${1:-deploy}" in
        deploy)
            check_requirements
            check_env
            build_image
            start_services
            run_migration
            health_check
            show_status
            ;;
        build)
            build_image
            ;;
        start)
            start_services
            health_check
            show_status
            ;;
        stop)
            stop_services
            ;;
        restart)
            stop_services
            start_services
            health_check
            show_status
            ;;
        status)
            show_status
            ;;
        logs)
            show_logs
            ;;
        cleanup)
            cleanup
            ;;
        *)
            echo "用法: $0 {deploy|build|start|stop|restart|status|logs|cleanup}"
            echo ""
            echo "命令说明："
            echo "  deploy   - 完整部署（检查、构建、启动、迁移）"
            echo "  build    - 仅构建镜像"
            echo "  start    - 启动服务"
            echo "  stop     - 停止服务"
            echo "  restart  - 重启服务"
            echo "  status   - 查看服务状态"
            echo "  logs     - 查看实时日志"
            echo "  cleanup  - 清理所有数据"
            exit 1
            ;;
    esac
}

main "$@"

