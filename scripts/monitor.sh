#!/bin/bash

# ============================================
# IOT Server 监控与调试工具
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
log_title() { echo -e "${CYAN}========== $1 ==========${NC}"; }

# 检查 jq 是否安装
check_jq() {
    if ! command -v jq &> /dev/null; then
        log_warn "jq 未安装，建议安装以获得更好的 JSON 显示效果"
        echo "  macOS: brew install jq"
        echo "  Linux: apt-get install jq 或 yum install jq"
        return 1
    fi
    return 0
}

HAS_JQ=$(check_jq && echo "true" || echo "false")

# 默认配置
API_PORT=${API_PORT:-7055}

# ============================================
# 1. 健康检查
# ============================================
health_check() {
    log_title "系统健康检查"
    
    if [ "$HAS_JQ" = "true" ]; then
        curl -s http://localhost:$API_PORT/health | jq '.'
    else
        curl -s http://localhost:$API_PORT/health
    fi
    
    echo ""
    log_info "详细健康检查端点："
    echo "  完整健康: http://localhost:$API_PORT/health"
    echo "  就绪探针: http://localhost:$API_PORT/health/ready"
    echo "  存活探针: http://localhost:$API_PORT/health/live"
}

# ============================================
# 2. 实时日志
# ============================================
logs() {
    local service="${1:-iot-server}"
    local lines="${2:-100}"
    
    log_title "查看 ${service} 日志（最近 ${lines} 行）"
    
    if [ "$service" = "all" ]; then
        docker-compose logs --tail="$lines" -f
    else
        docker-compose logs --tail="$lines" -f "$service"
    fi
}

# ============================================
# 3. Prometheus 指标
# ============================================
metrics() {
    log_title "Prometheus 监控指标"
    
    echo ""
    log_info "📊 业务关键指标："
    echo ""
    
    # TCP 连接统计
    echo "🔌 TCP 连接："
    curl -s http://localhost:$API_PORT/metrics | grep -E "^tcp_accept_total|^tcp_bytes_received" | head -5
    
    echo ""
    echo "📱 设备在线数："
    curl -s http://localhost:$API_PORT/metrics | grep "^session_online_count"
    
    echo ""
    echo "💓 心跳统计："
    curl -s http://localhost:$API_PORT/metrics | grep "^session_heartbeat_total"
    
    echo ""
    echo "📦 协议解析（BKV/GN）："
    curl -s http://localhost:$API_PORT/metrics | grep -E "^bkv_parse_total|^gn_parse_total"
    
    echo ""
    echo "🔄 出站队列："
    curl -s http://localhost:$API_PORT/metrics | grep "^outbound_"
    
    echo ""
    echo "❌ 会话离线原因："
    curl -s http://localhost:$API_PORT/metrics | grep "^session_offline_total"
    
    echo ""
    log_info "📈 完整指标: http://localhost:$API_PORT/metrics"
}

# ============================================
# 4. 容器状态
# ============================================
status() {
    log_title "服务运行状态"
    docker-compose ps
    
    echo ""
    log_title "资源使用情况"
    docker stats --no-stream --format "table {{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}\t{{.NetIO}}\t{{.BlockIO}}"
}

# ============================================
# 5. 数据库检查
# ============================================
db_check() {
    log_title "数据库连接检查"
    
    if docker-compose exec -T postgres pg_isready -U iot; then
        log_info "✅ 数据库连接正常"
        
        echo ""
        log_info "数据库统计："
        docker-compose exec -T postgres psql -U iot -d iot_server -c "
            SELECT 
                schemaname,
                relname as tablename,
                n_live_tup as rows
            FROM pg_stat_user_tables
            ORDER BY n_live_tup DESC
            LIMIT 10;
        "
    else
        log_error "❌ 数据库连接失败"
        return 1
    fi
}

# ============================================
# 6. Redis 检查
# ============================================
redis_check() {
    log_title "Redis 状态检查"
    
    echo ""
    log_info "Redis Info:"
    docker-compose exec -T redis redis-cli info stats | grep -E "total_connections|total_commands"
    
    echo ""
    log_info "Redis 内存使用:"
    docker-compose exec -T redis redis-cli info memory | grep -E "used_memory_human|maxmemory_human"
    
    echo ""
    log_info "连接的客户端:"
    docker-compose exec -T redis redis-cli client list | wc -l | xargs -I {} echo "{} 个客户端"
}

# ============================================
# 7. 错误日志分析
# ============================================
errors() {
    local minutes="${1:-30}"
    
    log_title "最近 ${minutes} 分钟的错误日志"
    
    echo ""
    log_warn "应用错误："
    docker-compose logs --since="${minutes}m" iot-server 2>&1 | grep -i "error\|fatal\|panic" | tail -20
    
    echo ""
    log_warn "数据库错误："
    docker-compose logs --since="${minutes}m" postgres 2>&1 | grep -i "error\|fatal" | tail -10
    
    echo ""
    log_warn "Redis 错误："
    docker-compose logs --since="${minutes}m" redis 2>&1 | grep -i "error\|warning" | tail -10
}

# ============================================
# 8. 网络连接检查
# ============================================
network() {
    log_title "网络连接检查"
    
    echo ""
    log_info "监听端口："
    docker-compose ps | grep -E "PORTS|iot-server|postgres|redis"
    
    echo ""
    log_info "测试连接："
    echo -n "HTTP API ($API_PORT): "
    curl -s -o /dev/null -w "%{http_code}" http://localhost:$API_PORT/health && echo " ✅" || echo " ❌"
    
    echo -n "TCP 端口 (7065-BKV): "
    timeout 2 bash -c "cat < /dev/null > /dev/tcp/localhost/7065" 2>/dev/null && echo "✅" || echo "❌"
    
    echo -n "Postgres (5433): "
    timeout 2 bash -c "cat < /dev/null > /dev/tcp/localhost/5433" 2>/dev/null && echo "✅" || echo "❌"
    
    echo -n "Redis (6380): "
    timeout 2 bash -c "cat < /dev/null > /dev/tcp/localhost/6380" 2>/dev/null && echo "✅" || echo "❌"
}

# ============================================
# 9. 完整诊断
# ============================================
diagnose() {
    log_title "🔍 完整系统诊断"
    echo ""
    
    health_check
    echo ""
    
    status
    echo ""
    
    network
    echo ""
    
    db_check
    echo ""
    
    redis_check
    echo ""
    
    errors 30
    echo ""
    
    log_info "✅ 诊断完成"
}

# ============================================
# 10. 性能分析
# ============================================
performance() {
    log_title "性能指标分析"
    
    echo ""
    log_info "Go Runtime 指标："
    curl -s http://localhost:$API_PORT/metrics | grep -E "^go_goroutines|^go_threads|^go_memstats"
    
    echo ""
    log_info "HTTP 请求统计："
    curl -s http://localhost:$API_PORT/metrics | grep -E "^gin_"
    
    echo ""
    log_info "进程统计："
    curl -s http://localhost:$API_PORT/metrics | grep -E "^process_"
}

# ============================================
# 11. 搜索日志
# ============================================
search_logs() {
    local keyword="$1"
    local service="${2:-iot-server}"
    local lines="${3:-50}"
    
    if [ -z "$keyword" ]; then
        log_error "请提供搜索关键词"
        echo "用法: $0 search <关键词> [服务名] [行数]"
        return 1
    fi
    
    log_title "搜索日志: \"$keyword\""
    docker-compose logs --tail=1000 "$service" | grep -i "$keyword" | tail -n "$lines"
}

# ============================================
# 12. 导出诊断报告
# ============================================
export_report() {
    local report_file="./logs/diagnostic_report_$(date +%Y%m%d_%H%M%S).txt"
    mkdir -p ./logs
    
    log_title "导出诊断报告"
    
    {
        echo "=================================="
        echo "IOT Server 诊断报告"
        echo "时间: $(date)"
        echo "=================================="
        echo ""
        
        echo "1. 健康检查"
        echo "----------"
        curl -s http://localhost:$API_PORT/health
        echo ""
        
        echo "2. 服务状态"
        echo "----------"
        docker-compose ps
        echo ""
        
        echo "3. 资源使用"
        echo "----------"
        docker stats --no-stream
        echo ""
        
        echo "4. 最近错误"
        echo "----------"
        docker-compose logs --since=1h iot-server 2>&1 | grep -i "error\|fatal" | tail -50
        echo ""
        
        echo "5. 关键指标"
        echo "----------"
        curl -s http://localhost:$API_PORT/metrics | grep -E "session_online|tcp_accept|heartbeat"
        
    } > "$report_file"
    
    log_info "✅ 报告已导出: $report_file"
}

# ============================================
# 帮助信息
# ============================================
usage() {
    cat << EOF
IOT Server 监控与调试工具

用法: $0 <命令> [参数]

命令：
  health              健康检查
  logs [服务] [行数]  查看日志（默认: iot-server, 100行）
  metrics             查看 Prometheus 指标
  status              服务状态和资源使用
  db                  数据库检查
  redis               Redis 检查
  errors [分钟]       错误日志分析（默认: 最近30分钟）
  network             网络连接检查
  diagnose            完整诊断（推荐）
  performance         性能指标分析
  search <关键词>     搜索日志
  export              导出诊断报告
  help                显示帮助

示例：
  $0 diagnose                    # 运行完整诊断
  $0 logs iot-server 200         # 查看最近200行日志
  $0 logs all                    # 查看所有服务日志
  $0 errors 60                   # 查看最近1小时错误
  $0 search "connection refused" # 搜索连接错误
  $0 export                      # 导出诊断报告

快捷方式：
  $0 h    = health
  $0 l    = logs
  $0 m    = metrics
  $0 s    = status
  $0 e    = errors
  $0 d    = diagnose

访问地址：
  HTTP API:   http://localhost:7055
  健康检查:   http://localhost:7055/health
  Metrics:    http://localhost:7055/metrics
  TCP 端口:   localhost:7065 (BKV协议)

环境变量：
  API_PORT    API 服务器端口（默认: 7055）

EOF
}

# ============================================
# 主程序
# ============================================
main() {
    local cmd="${1:-help}"
    
    case "$cmd" in
        health|h)
            health_check
            ;;
        logs|l)
            logs "${2:-iot-server}" "${3:-100}"
            ;;
        metrics|m)
            metrics
            ;;
        status|s)
            status
            ;;
        db)
            db_check
            ;;
        redis)
            redis_check
            ;;
        errors|e)
            errors "${2:-30}"
            ;;
        network|n)
            network
            ;;
        diagnose|d)
            diagnose
            ;;
        performance|perf|p)
            performance
            ;;
        search)
            search_logs "$2" "${3:-iot-server}" "${4:-50}"
            ;;
        export)
            export_report
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

