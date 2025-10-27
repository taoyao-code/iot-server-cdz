#!/bin/bash

# 监控与可观测性验证脚本
# 功能: 验证Prometheus指标、日志、事件推送等监控系统
# 使用: ./06_monitoring_check.sh

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

# ==================== 检查1: Prometheus指标 ====================
check_prometheus_metrics() {
    run_check "Prometheus指标"
    
    echo ""
    print_info "获取关键指标..."
    
    # 获取metrics
    local metrics_response=$(curl -s "http://$TEST_SERVER:$TEST_HTTP_PORT/metrics" 2>/dev/null)
    
    if [ $? -ne 0 ] || [ -z "$metrics_response" ]; then
        check_failed "无法获取Prometheus指标"
        return
    fi
    
    check_passed "Prometheus指标端点可访问"
    
    echo ""
    print_info "关键指标概览:"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    
    # TCP连接数
    local tcp_accepted=$(echo "$metrics_response" | grep "tcp_connections_accepted_total" | grep -v "#" | awk '{print $2}')
    if [ -n "$tcp_accepted" ]; then
        echo "  TCP连接总数: $tcp_accepted"
    fi
    
    # 活跃会话数
    local active_sessions=$(echo "$metrics_response" | grep "active_sessions{" | grep -v "#" | awk '{print $2}')
    if [ -n "$active_sessions" ]; then
        echo "  活跃会话数: $active_sessions"
        if [ "$(echo "$active_sessions > 0" | bc)" = "1" ]; then
            check_passed "有活跃会话: $active_sessions"
        else
            check_failed "无活跃会话"
        fi
    fi
    
    # 订单创建数
    local orders_created=$(echo "$metrics_response" | grep "orders_created_total" | grep -v "#" | awk '{print $2}')
    if [ -n "$orders_created" ]; then
        echo "  订单创建总数: $orders_created"
    fi
    
    # 订单结算数
    local orders_settled=$(echo "$metrics_response" | grep "orders_settled_total" | grep -v "#" | awk '{print $2}')
    if [ -n "$orders_settled" ]; then
        echo "  订单结算总数: $orders_settled"
    fi
    
    # 下行消息数
    local outbound_sent=$(echo "$metrics_response" | grep "outbound_messages_sent_total" | grep -v "#" | awk '{print $2}')
    if [ -n "$outbound_sent" ]; then
        echo "  下行消息总数: $outbound_sent"
    fi
    
    echo ""
    
    # 检查指标完整性
    local required_metrics=(
        "tcp_connections_accepted_total"
        "active_sessions"
        "orders_created_total"
    )
    
    local missing_metrics=0
    for metric in "${required_metrics[@]}"; do
        if ! echo "$metrics_response" | grep -q "$metric"; then
            print_warning "缺少指标: $metric"
            ((missing_metrics++))
        fi
    done
    
    if [ $missing_metrics -eq 0 ]; then
        check_passed "所有关键指标均存在"
    else
        check_failed "缺少 $missing_metrics 个关键指标"
    fi
}

# ==================== 检查2: 日志完整性 ====================
check_log_integrity() {
    run_check "日志完整性"
    
    echo ""
    print_info "分析日志级别分布..."
    echo ""
    echo "在服务器上执行:"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "docker logs --tail 1000 iot-server-prod 2>&1 | jq -r '.level' 2>/dev/null | sort | uniq -c"
    echo ""
    echo "预期分布:"
    echo "  - info: 占主要部分 (>80%)"
    echo "  - warn: 少量 (<15%)"
    echo "  - error: 极少或为0 (<5%)"
    echo ""
    
    read -p "info日志数: " info_count
    read -p "warn日志数: " warn_count
    read -p "error日志数: " error_count
    
    if [ -n "$info_count" ] && [ -n "$warn_count" ] && [ -n "$error_count" ]; then
        local total=$((info_count + warn_count + error_count))
        
        if [ $total -gt 0 ]; then
            local error_rate=$((error_count * 100 / total))
            
            echo ""
            echo "日志统计:"
            echo "  总日志数: $total"
            echo "  info: $info_count ($((info_count * 100 / total))%)"
            echo "  warn: $warn_count ($((warn_count * 100 / total))%)"
            echo "  error: $error_count (${error_rate}%)"
            echo ""
            
            if [ $error_rate -eq 0 ]; then
                check_passed "无ERROR级别日志"
            elif [ $error_rate -lt 5 ]; then
                check_passed "ERROR日志占比较低: ${error_rate}%"
            else
                check_failed "ERROR日志占比过高: ${error_rate}%"
            fi
        fi
    fi
    
    echo ""
    print_info "检查最近的ERROR日志..."
    echo ""
    echo "docker logs --tail 500 iot-server-prod 2>&1 | jq -r 'select(.level==\"error\") | .msg' | head -10"
    echo ""
    
    read -p "是否发现严重错误? (y/n): " has_errors
    
    if [ "$has_errors" = "n" ] || [ "$has_errors" = "N" ]; then
        check_passed "无严重错误日志"
    else
        check_failed "发现严重错误，需要处理"
    fi
}

# ==================== 检查3: 健康检查端点 ====================
check_health_endpoints() {
    run_check "健康检查端点"
    
    echo ""
    print_info "检查 /healthz 端点..."
    
    local health_response=$(curl -s -w "\n%{http_code}" "http://$TEST_SERVER:$TEST_HTTP_PORT/healthz")
    local health_code=$(echo "$health_response" | tail -1)
    local health_body=$(echo "$health_response" | sed '$d')
    
    echo "HTTP响应码: $health_code"
    echo "响应内容: $health_body"
    
    if [ "$health_code" = "200" ]; then
        check_passed "/healthz 端点正常"
    else
        check_failed "/healthz 端点异常 (HTTP $health_code)"
    fi
    
    echo ""
    print_info "检查 /readyz 端点..."
    
    local ready_response=$(curl -s -w "\n%{http_code}" "http://$TEST_SERVER:$TEST_HTTP_PORT/readyz")
    local ready_code=$(echo "$ready_response" | tail -1)
    local ready_body=$(echo "$ready_response" | sed '$d')
    
    echo "HTTP响应码: $ready_code"
    echo "响应内容: $ready_body"
    
    if [ "$ready_code" = "200" ]; then
        check_passed "/readyz 端点正常"
    else
        check_failed "/readyz 端点异常 (HTTP $ready_code)"
    fi
}

# ==================== 检查4: 事件推送成功率 ====================
check_event_push_rate() {
    run_check "事件推送成功率"
    
    echo ""
    print_info "检查事件队列状态..."
    
    echo "1. 事件队列长度:"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "docker exec -it iot-redis-prod redis-cli -a 123456 LLEN \"thirdparty:event_queue\""
    echo ""
    
    read -p "队列长度: " queue_length
    
    if [ -n "$queue_length" ] && [ "$queue_length" -lt 10 ]; then
        check_passed "事件队列积压少: $queue_length"
    elif [ -n "$queue_length" ] && [ "$queue_length" -lt 100 ]; then
        check_passed "事件队列积压适中: $queue_length"
    elif [ -n "$queue_length" ]; then
        check_failed "事件队列积压严重: $queue_length"
    fi
    
    echo ""
    echo "2. 推送失败计数（Prometheus指标）:"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    
    local metrics=$(curl -s "http://$TEST_SERVER:$TEST_HTTP_PORT/metrics" 2>/dev/null)
    
    # 推送成功数
    local push_success=$(echo "$metrics" | grep "thirdparty_push_total{.*result=\"success\"" | awk '{print $2}')
    # 推送失败数
    local push_failure=$(echo "$metrics" | grep "thirdparty_push_total{.*result=\"failure\"" | awk '{print $2}')
    
    if [ -n "$push_success" ] && [ -n "$push_failure" ]; then
        local total=$((push_success + push_failure))
        
        if [ $total -gt 0 ]; then
            local success_rate=$((push_success * 100 / total))
            
            echo "  推送成功: $push_success"
            echo "  推送失败: $push_failure"
            echo "  成功率: ${success_rate}%"
            echo ""
            
            if [ $success_rate -ge 99 ]; then
                check_passed "推送成功率优秀: ${success_rate}%"
            elif [ $success_rate -ge 95 ]; then
                check_passed "推送成功率良好: ${success_rate}%"
            else
                check_failed "推送成功率较低: ${success_rate}%"
            fi
        else
            print_warning "暂无推送统计数据"
        fi
    else
        print_warning "无法获取推送统计指标"
    fi
    
    echo ""
    echo "3. 平均推送延迟:"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    
    local push_duration=$(echo "$metrics" | grep "thirdparty_push_duration_seconds_sum" | awk '{print $2}')
    local push_count=$(echo "$metrics" | grep "thirdparty_push_duration_seconds_count" | awk '{print $2}')
    
    if [ -n "$push_duration" ] && [ -n "$push_count" ] && [ "$(echo "$push_count > 0" | bc)" = "1" ]; then
        local avg_duration=$(echo "scale=3; $push_duration / $push_count * 1000" | bc)
        echo "  平均延迟: ${avg_duration}ms"
        
        if [ "$(echo "$avg_duration < 500" | bc)" = "1" ]; then
            check_passed "推送延迟正常: ${avg_duration}ms"
        elif [ "$(echo "$avg_duration < 2000" | bc)" = "1" ]; then
            check_passed "推送延迟可接受: ${avg_duration}ms"
        else
            check_failed "推送延迟过高: ${avg_duration}ms"
        fi
    fi
}

# ==================== 检查5: 系统资源监控 ====================
check_system_resources() {
    run_check "系统资源监控"
    
    echo ""
    print_warning "此检查需要在服务器上查看容器资源使用"
    echo ""
    echo "在服务器上执行:"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "docker stats --no-stream iot-server-prod iot-postgres-prod iot-redis-prod"
    echo ""
    
    echo "请记录以下信息:"
    echo ""
    
    read -p "IoT Server CPU使用率 (%): " app_cpu
    read -p "IoT Server 内存使用 (MB): " app_mem
    read -p "PostgreSQL CPU使用率 (%): " db_cpu
    read -p "PostgreSQL 内存使用 (MB): " db_mem
    read -p "Redis CPU使用率 (%): " redis_cpu
    read -p "Redis 内存使用 (MB): " redis_mem
    
    echo ""
    print_separator
    echo "资源使用汇总:"
    print_separator
    
    # 验证应用服务器资源
    if [ -n "$app_cpu" ]; then
        echo "IoT Server:"
        echo "  CPU: ${app_cpu}%"
        if [ "$(echo "$app_cpu < 50" | bc)" = "1" ]; then
            echo "  └─ ${GREEN}✓ CPU使用正常${NC}"
        else
            echo "  └─ ${YELLOW}⚠ CPU使用较高${NC}"
        fi
    fi
    
    if [ -n "$app_mem" ]; then
        echo "  内存: ${app_mem}MB"
        if [ "$(echo "$app_mem < 2000" | bc)" = "1" ]; then
            echo "  └─ ${GREEN}✓ 内存使用正常${NC}"
            ((PASSED_CHECKS++))
        else
            echo "  └─ ${YELLOW}⚠ 内存使用较高${NC}"
            ((FAILED_CHECKS++))
        fi
    fi
    
    # 验证数据库资源
    if [ -n "$db_cpu" ] && [ -n "$db_mem" ]; then
        echo ""
        echo "PostgreSQL:"
        echo "  CPU: ${db_cpu}%"
        echo "  内存: ${db_mem}MB"
        check_passed "数据库资源监控正常"
    fi
    
    # 验证Redis资源
    if [ -n "$redis_cpu" ] && [ -n "$redis_mem" ]; then
        echo ""
        echo "Redis:"
        echo "  CPU: ${redis_cpu}%"
        echo "  内存: ${redis_mem}MB"
        check_passed "Redis资源监控正常"
    fi
}

# ==================== 主检查流程 ====================
print_header "监控与可观测性验证"
echo "服务器: $TEST_SERVER"
echo "检查时间: $(date '+%Y-%m-%d %H:%M:%S')"
echo ""

# 执行所有检查
check_prometheus_metrics
echo ""

check_log_integrity
echo ""

check_health_endpoints
echo ""

check_event_push_rate
echo ""

check_system_resources
echo ""

# ==================== 检查总结 ====================
generate_test_report "监控与可观测性验证" $PASSED_CHECKS $FAILED_CHECKS

exit $([ $FAILED_CHECKS -eq 0 ] && echo 0 || echo 1)

