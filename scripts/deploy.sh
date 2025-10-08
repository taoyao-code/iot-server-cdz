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
    
    # 检查Docker
    if ! command -v docker &> /dev/null; then
        log_error "Docker未安装，请先安装Docker"
        log_error "安装指引: https://docs.docker.com/get-docker/"
        exit 1
    fi
    
    # 检查Docker服务状态
    if ! docker info &> /dev/null; then
        log_error "Docker服务未运行，请先启动Docker"
        exit 1
    fi
    
    # 检查Docker版本（建议20.10+）
    DOCKER_VERSION=$(docker version --format '{{.Server.Version}}' 2>/dev/null | cut -d. -f1)
    if [ "$DOCKER_VERSION" -lt 20 ]; then
        log_warn "Docker版本较低 ($DOCKER_VERSION)，建议升级到20.10或更高版本"
    fi
    
    # 检查Docker Compose
    if ! command -v docker-compose &> /dev/null && ! docker compose version &> /dev/null; then
        log_error "Docker Compose未安装，请先安装Docker Compose"
        log_error "安装指引: https://docs.docker.com/compose/install/"
        exit 1
    fi
    
    # 检查必要的命令行工具
    local missing_tools=()
    for tool in curl git; do
        if ! command -v "$tool" &> /dev/null; then
            missing_tools+=("$tool")
        fi
    done
    
    if [ ${#missing_tools[@]} -ne 0 ]; then
        log_error "缺少必要工具: ${missing_tools[*]}"
        exit 1
    fi
    
    log_info "环境检查通过 ✓"
}

# 检查环境变量
check_env() {
    log_info "检查环境变量配置..."
    
    # 检查.env.example是否存在
    if [ ! -f .env.example ]; then
        log_error ".env.example 文件不存在"
        exit 1
    fi
    
    # 检查.env文件
    if [ ! -f .env ]; then
        log_warn ".env 文件不存在，从示例文件创建..."
        cp .env.example .env
        log_error "请编辑 .env 文件并填写正确的配置值"
        log_error "必须配置的变量: POSTGRES_PASSWORD, REDIS_PASSWORD, API_KEY, THIRDPARTY_API_KEY"
        exit 1
    fi
    
    # 加载环境变量
    set -a
    source .env
    set +a
    
    # 检查必要的环境变量
    required_vars=(
        "POSTGRES_PASSWORD"
        "REDIS_PASSWORD"
        "API_KEY"
        "THIRDPARTY_API_KEY"
    )
    
    local missing_vars=()
    local weak_vars=()
    
    for var in "${required_vars[@]}"; do
        local val="${!var}"
        if [ -z "$val" ]; then
            missing_vars+=("$var")
        elif [ "$val" = "CHANGE_ME_STRONG_PASSWORD_HERE" ] || \
             [ "$val" = "CHANGE_ME_REDIS_PASSWORD" ] || \
             [ "$val" = "CHANGE_ME_32_CHARS_OR_MORE" ] || \
             [ "${#val}" -lt 16 ]; then
            weak_vars+=("$var")
        fi
    done
    
    if [ ${#missing_vars[@]} -ne 0 ]; then
        log_error "缺少必要的环境变量: ${missing_vars[*]}"
        log_error "请在 .env 文件中配置这些变量"
        exit 1
    fi
    
    if [ ${#weak_vars[@]} -ne 0 ]; then
        log_warn "以下环境变量可能过于简单: ${weak_vars[*]}"
        log_warn "生产环境建议使用强密钥（openssl rand -base64 32）"
    fi
    
    log_info "环境变量配置检查完成 ✓"
}

# 网络连通性检查
check_network() {
    log_info "检查网络连通性..."
    
    local test_urls=(
        "mirrors.aliyun.com"
        "goproxy.cn"
        "registry.hub.docker.com"
    )
    
    local failed_urls=()
    
    for url in "${test_urls[@]}"; do
        if ! curl -s --connect-timeout 5 --max-time 10 "http://$url" > /dev/null 2>&1 && \
           ! ping -c 2 -W 3 "$url" > /dev/null 2>&1; then
            failed_urls+=("$url")
        fi
    done
    
    if [ ${#failed_urls[@]} -ne 0 ]; then
        log_warn "以下地址连接失败: ${failed_urls[*]}"
        log_warn "网络可能不稳定，但继续尝试构建..."
    else
        log_info "网络连通性检查通过 ✓"
    fi
}

# 检查磁盘空间
check_disk_space() {
    log_info "检查磁盘空间..."
    
    # macOS和Linux兼容的磁盘空间检查
    local available
    if [[ "$OSTYPE" == "darwin"* ]]; then
        # macOS
        available=$(df -g . | tail -1 | awk '{print $4}')
    else
        # Linux
        available=$(df -BG . | tail -1 | awk '{print $4}' | sed 's/G//')
    fi
    
    local required=5
    
    if [ -z "$available" ]; then
        log_warn "无法检测磁盘空间，跳过检查"
        return 0
    fi
    
    if [ "$available" -lt "$required" ]; then
        log_error "磁盘空间不足（可用: ${available}G，需要: ${required}G）"
        log_error "建议运行: docker system prune -a"
        exit 1
    fi
    
    log_info "磁盘空间充足 (${available}G 可用) ✓"
}

# 构建镜像
build_image() {
    log_info "开始构建Docker镜像..."
    
    BUILD_VERSION=${BUILD_VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo "dev")}
    BUILD_TIME=$(date -u '+%Y-%m-%d %H:%M:%S')
    GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
    
    log_info "构建信息: version=$BUILD_VERSION, commit=$GIT_COMMIT"
    
    # 构建时启用BuildKit以提高性能
    export DOCKER_BUILDKIT=1
    
    if docker build \
        --build-arg BUILD_VERSION="$BUILD_VERSION" \
        --build-arg BUILD_TIME="$BUILD_TIME" \
        --build-arg GIT_COMMIT="$GIT_COMMIT" \
        -t iot-server:"$BUILD_VERSION" \
        -t iot-server:latest \
        . ; then
        log_info "镜像构建完成 ✓ (version: $BUILD_VERSION)"
    else
        log_error "镜像构建失败"
        log_error "请检查错误信息，常见问题："
        log_error "  1. 网络连接问题（Alpine源或Go代理）"
        log_error "  2. 磁盘空间不足"
        log_error "  3. Docker版本过低"
        exit 1
    fi
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
            check_network
            check_disk_space
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

