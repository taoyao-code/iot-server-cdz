#!/bin/bash

# 数据一致性验证脚本
# 功能: 验证订单、命令日志、端口状态的数据一致性
# 使用: ./05_data_consistency_check.sh

set -e

# 加载辅助函数
SCRIPT_DIR=$(dirname "$0")
source "$SCRIPT_DIR/helper_functions.sh"

# 测试统计
TOTAL_CHECKS=0
PASSED_CHECKS=0
FAILED_CHECKS=0

run_check() {
    local check_name=$1
    ((TOTAL_CHECKS++))
    print_header "检查: $check_name"
}

check_passed() {
    print_success "$1"
    ((PASSED_CHECKS++))
}

check_failed() {
    print_failure "$1"
    ((FAILED_CHECKS++))
}

# ==================== 检查1: 订单数据完整性 ====================
check_order_integrity() {
    run_check "订单数据完整性"
    
    echo ""
    print_info "检查孤立订单（长时间未完成的订单）..."
    echo ""
    echo "在服务器上执行:"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "docker exec -it iot-postgres-prod psql -U iot -d iot_server -c \\"
    echo "\"SELECT COUNT(*) as orphan_orders"
    echo " FROM orders"
    echo " WHERE status IN ('pending', 'charging')"
    echo "   AND created_at < NOW() - INTERVAL '1 hour';\""
    echo ""
    
    read -p "孤立订单数: " orphan_count
    
    if [ -n "$orphan_count" ] && [ "$orphan_count" -eq 0 ]; then
        check_passed "无孤立订单"
    elif [ -n "$orphan_count" ]; then
        check_failed "发现 $orphan_count 个孤立订单"
        echo ""
        print_warning "建议查看这些订单的详细信息:"
        echo "docker exec -it iot-postgres-prod psql -U iot -d iot_server -c \\"
        echo "\"SELECT order_no, status, device_id, port_no, created_at"
        echo " FROM orders"
        echo " WHERE status IN ('pending', 'charging')"
        echo "   AND created_at < NOW() - INTERVAL '1 hour';\""
    fi
    
    echo ""
    print_info "检查订单状态分布..."
    echo ""
    echo "docker exec -it iot-postgres-prod psql -U iot -d iot_server -c \\"
    echo "\"SELECT status, COUNT(*) as count"
    echo " FROM orders"
    echo " WHERE created_at > NOW() - INTERVAL '24 hours'"
    echo " GROUP BY status;\""
    echo ""
    
    read -p "按回车继续..." 
    
    echo ""
    print_info "检查订单金额异常..."
    echo ""
    echo "docker exec -it iot-postgres-prod psql -U iot -d iot_server -c \\"
    echo "\"SELECT COUNT(*) as invalid_amount_orders"
    echo " FROM orders"
    echo " WHERE status = 'completed'"
    echo "   AND (total_kwh = 0 OR duration_sec = 0)"
    echo "   AND created_at > NOW() - INTERVAL '24 hours';\""
    echo ""
    
    read -p "异常订单数: " invalid_count
    
    if [ -n "$invalid_count" ] && [ "$invalid_count" -eq 0 ]; then
        check_passed "无异常订单数据"
    elif [ -n "$invalid_count" ]; then
        check_failed "发现 $invalid_count 个异常订单"
    fi
}

# ==================== 检查2: 命令日志一致性 ====================
check_command_log_consistency() {
    run_check "命令日志一致性"
    
    echo ""
    print_info "检查上下行命令日志匹配..."
    echo ""
    echo "在服务器上执行:"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "docker exec -it iot-postgres-prod psql -U iot -d iot_server -c \\"
    echo "\"SELECT"
    echo "     direction,"
    echo "     COUNT(*) as count"
    echo " FROM cmd_logs"
    echo " WHERE created_at > NOW() - INTERVAL '1 hour'"
    echo " GROUP BY direction;\""
    echo ""
    
    read -p "按回车继续..." 
    
    echo ""
    print_info "检查未确认的下行指令..."
    echo ""
    echo "docker exec -it iot-postgres-prod psql -U iot -d iot_server -c \\"
    echo "\"SELECT COUNT(*) as pending_commands"
    echo " FROM outbound_queue"
    echo " WHERE status = 'pending'"
    echo "   AND created_at < NOW() - INTERVAL '5 minutes';\""
    echo ""
    
    read -p "未确认指令数: " pending_cmd
    
    if [ -n "$pending_cmd" ] && [ "$pending_cmd" -eq 0 ]; then
        check_passed "所有指令已确认"
    elif [ -n "$pending_cmd" ] && [ "$pending_cmd" -lt 10 ]; then
        check_passed "少量未确认指令: $pending_cmd (可能设备离线)"
    elif [ -n "$pending_cmd" ]; then
        check_failed "大量未确认指令: $pending_cmd"
    fi
}

# ==================== 检查3: 端口状态一致性 ====================
check_port_status_consistency() {
    run_check "端口状态一致性"
    
    echo ""
    print_info "验证端口状态与订单状态的一致性..."
    echo ""
    echo "在服务器上执行:"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "docker exec -it iot-postgres-prod psql -U iot -d iot_server -c \\"
    echo "\"SELECT"
    echo "     d.phy_id,"
    echo "     p.port_no,"
    echo "     CASE p.status"
    echo "         WHEN 0 THEN '空闲'"
    echo "         WHEN 1 THEN '充电中'"
    echo "         WHEN 2 THEN '故障'"
    echo "         ELSE '未知' END as port_status,"
    echo "     o.status as order_status,"
    echo "     o.order_no"
    echo " FROM ports p"
    echo " JOIN devices d ON p.device_id = d.id"
    echo " LEFT JOIN orders o ON o.device_id = d.id"
    echo "     AND o.port_no = p.port_no"
    echo "     AND o.status IN ('pending', 'charging')"
    echo " WHERE d.phy_id IN ('82210225000520', '82241218000382')"\
    echo " ORDER BY d.phy_id, p.port_no;\""
    echo ""
    
    read -p "是否发现不一致? (y/n): " has_inconsistency
    
    if [ "$has_inconsistency" = "n" ] || [ "$has_inconsistency" = "N" ]; then
        check_passed "端口状态与订单状态一致"
    else
        check_failed "发现端口状态不一致"
        echo ""
        print_warning "可能的原因:"
        echo "  - 订单结算延迟"
        echo "  - 端口状态更新失败"
        echo "  - 设备离线时创建订单"
    fi
    
    echo ""
    print_info "检查故障端口..."
    echo ""
    echo "docker exec -it iot-postgres-prod psql -U iot -d iot_server -c \\"
    echo "\"SELECT COUNT(*) as fault_ports"
    echo " FROM ports"
    echo " WHERE status = 2;\""
    echo ""
    
    read -p "故障端口数: " fault_count
    
    if [ -n "$fault_count" ] && [ "$fault_count" -eq 0 ]; then
        check_passed "无故障端口"
    elif [ -n "$fault_count" ]; then
        check_failed "发现 $fault_count 个故障端口"
    fi
}

# ==================== 检查4: 会话数据一致性 ====================
check_session_consistency() {
    run_check "会话数据一致性"
    
    echo ""
    print_info "检查Redis会话与数据库设备状态的一致性..."
    echo ""
    
    if ! check_service_health; then
        print_warning "服务健康检查失败，跳过会话一致性检查"
        return
    fi
    
    echo "1. 查看Redis中的活跃会话:"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "docker exec -it iot-redis-prod redis-cli -a 123456 KEYS \"session:*\" | wc -l"
    echo ""
    
    read -p "Redis会话数: " redis_sessions
    
    echo ""
    echo "2. 查看数据库中的在线设备:"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "docker exec -it iot-postgres-prod psql -U iot -d iot_server -c \\"
    echo "\"SELECT COUNT(*) as online_devices"
    echo " FROM devices"
    echo " WHERE last_seen_at > NOW() - INTERVAL '30 seconds';\""
    echo ""
    
    read -p "在线设备数: " online_devices
    
    if [ -n "$redis_sessions" ] && [ -n "$online_devices" ]; then
        local diff=$((redis_sessions - online_devices))
        diff=${diff#-}  # 取绝对值
        
        if [ $diff -le 2 ]; then
            check_passed "会话数据基本一致 (Redis:$redis_sessions, DB:$online_devices)"
        else
            check_failed "会话数据差异较大 (Redis:$redis_sessions, DB:$online_devices, 差异:$diff)"
        fi
    fi
}

# ==================== 检查5: 事件队列健康度 ====================
check_event_queue_health() {
    run_check "事件队列健康度"
    
    echo ""
    print_info "检查第三方事件队列..."
    echo ""
    echo "docker exec -it iot-redis-prod redis-cli -a 123456 LLEN \"thirdparty:event_queue\""
    echo ""
    
    read -p "事件队列长度: " queue_len
    
    if [ -n "$queue_len" ] && [ "$queue_len" -eq 0 ]; then
        check_passed "事件队列无积压"
    elif [ -n "$queue_len" ] && [ "$queue_len" -lt 100 ]; then
        check_passed "事件队列积压较少: $queue_len"
    elif [ -n "$queue_len" ]; then
        check_failed "事件队列积压严重: $queue_len"
        echo ""
        print_warning "可能的原因:"
        echo "  - Webhook端点不可达"
        echo "  - 网络延迟较高"
        echo "  - Worker数量不足"
    fi
    
    echo ""
    print_info "检查死信队列..."
    echo ""
    echo "docker exec -it iot-redis-prod redis-cli -a 123456 LLEN \"thirdparty:dlq\""
    echo ""
    
    read -p "死信队列长度: " dlq_len
    
    if [ -n "$dlq_len" ] && [ "$dlq_len" -eq 0 ]; then
        check_passed "无死信消息"
    elif [ -n "$dlq_len" ] && [ "$dlq_len" -lt 10 ]; then
        check_passed "少量死信消息: $dlq_len"
    elif [ -n "$dlq_len" ]; then
        check_failed "大量死信消息: $dlq_len"
    fi
}

# ==================== 主检查流程 ====================
print_header "数据一致性验证"
echo "服务器: $TEST_SERVER"
echo "检查时间: $(date '+%Y-%m-%d %H:%M:%S')"
echo ""

print_warning "此脚本需要在服务器上执行多个SQL查询"
echo "请确保有服务器访问权限"
echo ""

read -p "是否继续? (y/n): " continue_check

if [ "$continue_check" != "y" ] && [ "$continue_check" != "Y" ]; then
    echo "已取消检查"
    exit 0
fi

echo ""

# 执行所有检查
check_order_integrity
echo ""

check_command_log_consistency
echo ""

check_port_status_consistency
echo ""

check_session_consistency
echo ""

check_event_queue_health
echo ""

# ==================== 检查总结 ====================
print_header "数据一致性检查总结"

echo ""
echo "总检查项: $TOTAL_CHECKS"
echo -e "${GREEN}通过: $PASSED_CHECKS${NC}"
echo -e "${RED}失败: $FAILED_CHECKS${NC}"
echo ""

SUCCESS_RATE=$((PASSED_CHECKS * 100 / TOTAL_CHECKS))

if [ $SUCCESS_RATE -ge 90 ]; then
    echo -e "${GREEN}✓ 数据一致性良好 (${SUCCESS_RATE}%)${NC}"
    exit 0
elif [ $SUCCESS_RATE -ge 70 ]; then
    echo -e "${YELLOW}⚠ 数据一致性尚可 (${SUCCESS_RATE}%)${NC}"
    echo "建议检查失败项"
    exit 1
else
    echo -e "${RED}✗ 数据一致性较差 (${SUCCESS_RATE}%)${NC}"
    echo "请立即处理失败项"
    exit 1
fi

