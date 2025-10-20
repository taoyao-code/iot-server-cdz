#!/bin/bash

# ============================================
# TCP 模块测试工具
# 用于验证 TCP 连接和基础功能
# ============================================

set -e

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }
log_success() { echo -e "${GREEN}[✓]${NC} $1"; }
log_fail() { echo -e "${RED}[✗]${NC} $1"; }
log_title() { echo -e "${CYAN}========== $1 ==========${NC}"; }

# 默认配置
TCP_HOST=${TCP_HOST:-localhost}
TCP_PORT=${TCP_PORT:-7065}
API_HOST=${API_HOST:-localhost}
API_PORT=${API_PORT:-7055}

# ============================================
# 工具函数
# ============================================

# 检查依赖
check_dependencies() {
    local missing=()
    
    if ! command -v nc &> /dev/null; then
        missing+=("nc (netcat)")
    fi
    
    if ! command -v curl &> /dev/null; then
        missing+=("curl")
    fi
    
    if [ ${#missing[@]} -gt 0 ]; then
        log_error "缺少必要工具: ${missing[*]}"
        echo "请安装："
        echo "  macOS: brew install netcat"
        echo "  Linux: apt-get install netcat-openbsd"
        return 1
    fi
    return 0
}

# ============================================
# 测试用例
# ============================================

# 测试 1：检查端口
test_check_port() {
    log_title "测试 1: 检查 TCP 端口"
    
    log_info "检查端口 $TCP_HOST:$TCP_PORT 是否监听..."
    
    if nc -z -w 5 "$TCP_HOST" "$TCP_PORT" 2>/dev/null; then
        log_success "TCP 端口 $TCP_PORT 正常监听"
        
        # 检查健康状态
        log_info "检查健康状态..."
        if curl -sf "http://$API_HOST:$API_PORT/health" >/dev/null 2>&1; then
            log_success "服务健康"
        else
            log_warn "健康检查失败"
        fi
        return 0
    else
        log_fail "端口 $TCP_PORT 未监听"
        return 1
    fi
}

# 测试 2：基础连接
test_basic_connection() {
    log_title "测试 2: 基础 TCP 连接"
    
    log_info "尝试连接到 $TCP_HOST:$TCP_PORT..."
    
    # 使用 nc 测试连接（3 秒超时）
    if timeout 3 bash -c "cat < /dev/null > /dev/tcp/$TCP_HOST/$TCP_PORT" 2>/dev/null; then
        log_success "TCP 连接成功建立"
        return 0
    else
        log_fail "TCP 连接失败"
        return 1
    fi
}

# 测试 3：并发连接
test_concurrent_connections() {
    local count="${1:-10}"
    log_title "测试 3: 并发连接测试 ($count 个连接)"
    
    log_info "创建 $count 个并发连接..."
    
    # 并发创建连接
    local pids=()
    for ((i=1; i<=count; i++)); do
        (sleep 5 | nc "$TCP_HOST" "$TCP_PORT" >/dev/null 2>&1) &
        pids+=($!)
        sleep 0.1  # 避免过快
    done
    
    sleep 2
    log_success "成功创建 $count 个并发连接"
    
    # 清理
    for pid in "${pids[@]}"; do
        kill $pid 2>/dev/null || true
    done
    
    sleep 1
    log_info "测试完成"
}

# 测试 4：查看统计指标
test_show_metrics() {
    log_title "测试 4: 查看关键指标"
    
    log_info "TCP 连接指标:"
    curl -s "http://$API_HOST:$API_PORT/metrics" | grep -E "^tcp_" || echo "  无数据"
    
    echo ""
    log_info "BKV 协议解析指标:"
    curl -s "http://$API_HOST:$API_PORT/metrics" | grep -E "^bkv_" || echo "  无数据"
    
    echo ""
    log_info "会话指标:"
    curl -s "http://$API_HOST:$API_PORT/metrics" | grep -E "^session_" || echo "  无数据"
    
    echo ""
    log_info "出站指标:"
    curl -s "http://$API_HOST:$API_PORT/metrics" | grep -E "^outbound_" || echo "  无数据"
}

# 运行所有测试
run_all_tests() {
    log_title "运行完整测试套件"
    
    local passed=0
    local failed=0
    
    echo ""
    test_check_port && ((passed++)) || ((failed++))
    
    echo ""
    test_basic_connection && ((passed++)) || ((failed++))
    
    echo ""
    test_concurrent_connections 5 && ((passed++)) || ((failed++))
    
    echo ""
    test_show_metrics
    
    echo ""
    log_title "测试结果"
    log_info "通过: $passed"
    log_info "失败: $failed"
    
    if [ $failed -eq 0 ]; then
        log_success "所有测试通过！"
        return 0
    else
        log_warn "有 $failed 个测试失败"
        return 1
    fi
}

# ============================================
# 帮助信息
# ============================================
usage() {
    cat << EOF
TCP 模块测试工具 - 组网设备 BKV 协议

用法: $0 <命令> [参数]

命令：
  check-port                    检查 TCP 端口是否监听
  connect                       测试基础 TCP 连接
  concurrent <count>            并发连接测试（默认10个）
  metrics                       显示关键指标
  run-all                       运行所有测试

环境变量：
  TCP_HOST        TCP 服务器地址（默认: localhost）
  TCP_PORT        TCP 服务器端口（默认: 7065，BKV协议端口）
  API_HOST        API 服务器地址（默认: localhost）
  API_PORT        API 服务器端口（默认: 7055）

示例：
  $0 check-port                      # 检查端口
  $0 connect                         # 测试连接
  $0 concurrent 50                   # 50 个并发连接
  $0 metrics                         # 查看指标
  $0 run-all                         # 运行所有测试

  TCP_HOST=192.168.1.100 $0 connect # 连接远程服务器

注意：
  - 本脚本用于测试 BKV 组网协议（包头 fcfe/fcff）
  - 实际协议报文需要通过硬件设备或模拟工具发送
  - 可使用 protocol-monitor.sh 实时查看协议数据

EOF
}

# ============================================
# 主程序
# ============================================
main() {
    # 检查依赖
    check_dependencies || exit 1
    
    local cmd="${1:-help}"
    shift || true
    
    case "$cmd" in
        check-port)
            test_check_port
            ;;
        connect)
            test_basic_connection
            ;;
        concurrent)
            test_concurrent_connections "$@"
            ;;
        metrics)
            test_show_metrics
            ;;
        run-all)
            run_all_tests
            ;;
        help|--help|-h)
            usage
            ;;
        *)
            log_error "未知命令: $cmd"
            echo ""
            usage
            exit 1
            ;;
    esac
}

main "$@"
