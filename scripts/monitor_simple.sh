#!/bin/bash
# 单窗口简化监控工具
# 功能：快速诊断系统状态（无需tmux）
# 使用：./monitor_simple.sh [interval]

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

# 配置
SERVER="${SERVER:-182.43.177.92}"  # 默认监控远程服务器
HTTP_PORT="${HTTP_PORT:-7055}"
REFRESH_INTERVAL=${1:-5}  # 刷新间隔（秒）
REMOTE_MODE="${REMOTE_MODE:-true}"  # 是否远程监控模式
SSH_HOST="${SSH_HOST:-root@182.43.177.92}"  # SSH主机

# 清屏并显示标题
show_header() {
    clear
    echo -e "${BOLD}${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${BOLD}${BLUE}  IoT充电桩系统监控 - $(date '+%Y-%m-%d %H:%M:%S')${NC}"
    echo -e "${BOLD}${BLUE}  刷新间隔: ${REFRESH_INTERVAL}秒 | 按 Ctrl+C 退出${NC}"
    echo -e "${BOLD}${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""
}

# 显示服务健康状态
show_health() {
    echo -e "${BOLD}【服务健康】${NC}"
    
    local health_response=$(curl -s "http://$SERVER:$HTTP_PORT/healthz" 2>&1 || echo "ERROR")
    
    if echo "$health_response" | grep -q "healthy\|ok"; then
        echo -e "  状态: ${GREEN}✓ 正常${NC}"
    else
        echo -e "  状态: ${RED}✗ 异常${NC}"
    fi
    
    # 显示指标（如果有）
    local metrics=$(curl -s "http://$SERVER:$HTTP_PORT/metrics" 2>&1 | grep -E "^iot_" | head -5 || echo "")
    if [ -n "$metrics" ]; then
        echo "  关键指标:"
        echo "$metrics" | while read -r line; do
            echo "    $line"
        done | head -5
    fi
    
    echo ""
}

# 显示在线设备
show_devices() {
    echo -e "${BOLD}【在线设备】${NC}"
    
    # 从Redis获取在线设备（需要Redis访问权限）
    local container_name="iot-redis-prod"
    
    if [ "$REMOTE_MODE" = "true" ]; then
        # 远程模式：通过SSH连接
        local device_count=$(ssh -o ConnectTimeout=3 "$SSH_HOST" "docker exec -it $container_name redis-cli -a 123456 --no-auth-warning KEYS 'session:device:*' 2>/dev/null | wc -l" 2>/dev/null | tr -d '[:space:]')
        if [ -n "$device_count" ] && [ "$device_count" -gt 0 ]; then
            echo -e "  在线数量: ${GREEN}$device_count${NC}"
        else
            echo -e "  ${YELLOW}暂无在线设备或无法访问Redis${NC}"
        fi
    else
        # 本地模式
        if docker ps --format '{{.Names}}' | grep -q "$container_name"; then
            local online_devices=$(docker exec -it $container_name redis-cli -a 123456 --no-auth-warning KEYS "session:device:*" 2>/dev/null | wc -l | tr -d '[:space:]')
            echo -e "  在线数量: ${GREEN}$online_devices${NC}"
        else
            echo -e "  ${YELLOW}无法连接Redis${NC}"
        fi
    fi
    
    echo ""
}

# 显示活跃订单
show_orders() {
    echo -e "${BOLD}【活跃订单】${NC}"
    
    # 从日志中提取最近的订单信息
    if [ "$REMOTE_MODE" = "true" ]; then
        local log_cmd="ssh -o ConnectTimeout=3 '$SSH_HOST' 'docker logs --tail 100 iot-server-prod 2>&1' 2>/dev/null"
    else
        local log_cmd="docker logs --tail 100 iot-server-prod 2>&1"
    fi
    
    # 查找充电中的订单
    local charging_orders=$(eval "$log_cmd" | grep -i "charging\|order" | grep -v "completed" | tail -3)
    
    if [ -n "$charging_orders" ]; then
        echo "$charging_orders" | while IFS= read -r line; do
            # 提取订单号
            if echo "$line" | grep -q "order_no"; then
                local order_no=$(echo "$line" | grep -oP 'order_no["\s:]+\K[A-Za-z0-9-]+' | head -1)
                if [ -n "$order_no" ]; then
                    echo -e "  - ${CYAN}$order_no${NC}"
                fi
            fi
        done | head -5
    else
        echo "  无活跃订单"
    fi
    
    echo ""
}

# 显示最近错误
show_errors() {
    echo -e "${BOLD}【最近错误】(最近10条)${NC}"
    
    if [ "$REMOTE_MODE" = "true" ]; then
        local log_cmd="ssh -o ConnectTimeout=3 '$SSH_HOST' 'docker logs --tail 200 iot-server-prod 2>&1' 2>/dev/null"
    else
        local log_cmd="docker logs --tail 200 iot-server-prod 2>&1"
    fi
    
    local errors=$(eval "$log_cmd" | grep -i "error\|fail\|panic" | tail -10)
    
    if [ -n "$errors" ]; then
        echo "$errors" | while IFS= read -r line; do
            # 提取时间和错误信息
            local timestamp=$(echo "$line" | grep -oE '[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}' | head -1)
            local msg=$(echo "$line" | sed 's/.*error["\s:]*//i' | cut -c1-80)
            
            if [ -n "$timestamp" ]; then
                echo -e "  ${RED}[$timestamp]${NC} $msg"
            else
                echo -e "  ${RED}$line${NC}" | cut -c1-100
            fi
        done
    else
        echo -e "  ${GREEN}无错误${NC}"
    fi
    
    echo ""
}

# 显示TCP连接统计
show_tcp_stats() {
    echo -e "${BOLD}【TCP连接】${NC}"
    
    # 从日志中查找TCP相关信息
    if [ "$REMOTE_MODE" = "true" ]; then
        local log_cmd="ssh -o ConnectTimeout=3 '$SSH_HOST' 'docker logs --tail 50 iot-server-prod 2>&1' 2>/dev/null"
    else
        local log_cmd="docker logs --tail 50 iot-server-prod 2>&1"
    fi
    local tcp_info=$(eval "$log_cmd" | grep -i "tcp\|connection" | tail -3)
    
    if [ -n "$tcp_info" ]; then
        echo "$tcp_info" | while IFS= read -r line; do
            echo "  $(echo "$line" | cut -c1-90)"
        done
    else
        echo "  无TCP活动"
    fi
    
    echo ""
}

# 显示最近协议帧
show_recent_frames() {
    echo -e "${BOLD}【最近协议帧】(最近5条)${NC}"
    
    if [ "$REMOTE_MODE" = "true" ]; then
        local log_cmd="ssh -o ConnectTimeout=3 '$SSH_HOST' 'docker logs --tail 100 iot-server-prod 2>&1' 2>/dev/null"
    else
        local log_cmd="docker logs --tail 100 iot-server-prod 2>&1"
    fi
    local frames=$(eval "$log_cmd" | grep -i "0x0015\|0x1000\|BKV frame" | tail -5)
    
    if [ -n "$frames" ]; then
        echo "$frames" | while IFS= read -r line; do
            # 提取命令码和网关ID
            local cmd=$(echo "$line" | grep -oE '0x[0-9a-fA-F]{4}' | head -1)
            local gateway=$(echo "$line" | grep -oE '[0-9a-fA-F]{14}' | head -1)
            local direction=$(echo "$line" | grep -q "uplink\|上行" && echo "↑" || echo "↓")
            
            if [ -n "$cmd" ]; then
                echo -e "  $direction ${CYAN}$cmd${NC} - $gateway"
            else
                echo "  $(echo "$line" | cut -c1-80)"
            fi
        done
    else
        echo "  无协议帧活动"
    fi
    
    echo ""
}

# 显示系统资源
show_resources() {
    echo -e "${BOLD}【系统资源】${NC}"
    
    # Docker容器资源使用
    if [ "$REMOTE_MODE" = "true" ]; then
        echo "  远程监控模式"
    else
        if command -v docker &> /dev/null; then
            local stats=$(docker stats --no-stream --format "table {{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}" | grep "iot-server-prod" || echo "")
            if [ -n "$stats" ]; then
                echo "  容器资源:"
                echo "$stats" | awk 'NR==1 {print "    " $2 " | " $3}' 2>/dev/null || true
            fi
        fi
    fi
    
    echo ""
}

# 显示快捷操作提示
show_tips() {
    echo -e "${BOLD}${CYAN}【快捷操作】${NC}"
    echo "  查看完整日志: docker logs -f iot-server-prod"
    echo "  查看错误日志: docker logs iot-server-prod 2>&1 | grep -i error"
    echo "  重启服务: docker restart iot-server-prod"
    echo "  执行测试: cd test/scripts && ./test_charge_lifecycle.sh --auto"
    echo ""
}

# 主循环
main() {
    # 显示模式信息
    if [ "$REMOTE_MODE" = "true" ]; then
        echo -e "${CYAN}远程监控模式${NC}"
        echo -e "监控服务器: ${BOLD}$SERVER${NC}"
        echo ""
    fi
    
    # 持续监控
    while true; do
        show_header
        show_health
        show_devices
        show_orders
        show_errors
        show_tcp_stats
        show_recent_frames
        show_resources
        show_tips
        
        # 等待刷新或退出
        sleep "$REFRESH_INTERVAL"
    done
}

# 运行主函数
main

