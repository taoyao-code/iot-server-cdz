#!/bin/bash
# ============================================
# 通用配置文件 - 所有脚本的统一配置
# ============================================

# 端口配置
export TCP_PORT=7054      # BKV协议TCP端口 (容器内7000)
export API_PORT=7055      # HTTP API端口 (容器内8080)
export POSTGRES_PORT=5433 # PostgreSQL数据库端口
export REDIS_PORT=6380    # Redis端口
export PROMETHEUS_PORT=9090  # Prometheus监控端口（可选）
export GRAFANA_PORT=3000     # Grafana可视化端口（可选）

# 端口说明
# 7054 - TCP设备协议端口（BKV组网协议）
# 7055 - HTTP API/健康检查/Metrics端口

# 颜色输出
export RED='\033[0;31m'
export GREEN='\033[0;32m'
export YELLOW='\033[1;33m'
export BLUE='\033[0;34m'
export CYAN='\033[0;36m'
export MAGENTA='\033[0;35m'
export NC='\033[0m'

# 日志函数
log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }
log_success() { echo -e "${GREEN}[✓]${NC} $1"; }
log_fail() { echo -e "${RED}[✗]${NC} $1"; }
log_title() { echo -e "${CYAN}========== $1 ==========${NC}"; }
log_step() { echo -e "${BLUE}[STEP]${NC} $1"; }
log_data() { echo -e "${MAGENTA}[DATA]${NC} $1"; }

# 端口检测函数
check_port_available() {
    local port=$1
    local service_name=${2:-"服务"}
    
    if lsof -i :$port 2>/dev/null > /dev/null; then
        log_error "端口 $port ($service_name) 已被占用："
        lsof -i :$port
        return 1
    fi
    return 0
}

# 端口连接测试
test_port_connection() {
    local host=${1:-localhost}
    local port=$2
    local timeout=${3:-5}
    
    if timeout $timeout bash -c "cat < /dev/null > /dev/tcp/$host/$port" 2>/dev/null; then
        return 0
    fi
    return 1
}

# HTTP健康检查
check_http_health() {
    local url=${1:-"http://localhost:$API_PORT/healthz"}
    local timeout=${2:-5}
    
    if curl -f -s --connect-timeout $timeout --max-time $timeout "$url" > /dev/null 2>&1; then
        return 0
    fi
    return 1
}

# 显示端口配置信息
show_port_info() {
    log_title "端口配置信息"
    echo "TCP 协议端口: $TCP_PORT (BKV组网设备)"
    echo "HTTP API端口: $API_PORT (健康检查/Metrics)"
    echo "PostgreSQL:   $POSTGRES_PORT"
    echo "Redis:        $REDIS_PORT"
    echo ""
    echo "访问地址："
    echo "  健康检查: http://localhost:$API_PORT/healthz"
    echo "  就绪探针: http://localhost:$API_PORT/readyz"
    echo "  监控指标: http://localhost:$API_PORT/metrics"
}

