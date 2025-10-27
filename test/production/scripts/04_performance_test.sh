#!/bin/bash

# 性能压力测试脚本
# 功能: 测试API并发性能、系统资源使用等
# 使用: ./04_performance_test.sh

set -e

# 加载辅助函数
SCRIPT_DIR=$(dirname "$0")
source "$SCRIPT_DIR/helper_functions.sh"

# 测试统计
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0

run_test() {
    local test_name=$1
    ((TOTAL_TESTS++))
    print_header "测试: $test_name"
}

test_passed() {
    print_success "$1"
    ((PASSED_TESTS++))
}

test_failed() {
    print_failure "$1"
    ((FAILED_TESTS++))
}

# 检查依赖工具
check_tools() {
    local missing=0
    
    if ! command -v ab &> /dev/null; then
        echo "缺少工具: apache-bench (ab)"
        echo "安装命令: sudo apt-get install apache2-utils"
        missing=1
    fi
    
    if [ $missing -eq 1 ]; then
        print_warning "部分测试工具缺失，某些测试将跳过"
    fi
    
    return $missing
}

# ==================== 测试1: API并发测试 ====================
test_api_concurrency() {
    run_test "API并发性能测试"
    
    if ! command -v ab &> /dev/null; then
        print_warning "跳过测试: 需要安装 apache-bench"
        return
    fi
    
    echo ""
    print_info "准备测试数据..."
    
    # 创建测试payload
    local payload_file="/tmp/charge_payload_$$.json"
    cat > "$payload_file" <<EOF
{
  "port_no": 255,
  "charge_mode": 1,
  "duration": 60,
  "amount": 100,
  "price_per_kwh": 150,
  "service_fee": 50
}
EOF
    
    echo ""
    print_info "执行并发测试..."
    echo "  请求总数: 100"
    echo "  并发数: 10"
    echo "  目标API: /api/v1/third/devices/$TEST_DEVICE1/charge"
    echo ""
    
    # 执行ab测试
    local ab_output=$(ab -n 100 -c 10 \
        -H "X-Api-Key: $TEST_API_KEY" \
        -H "Content-Type: application/json" \
        -p "$payload_file" \
        -T "application/json" \
        "http://$TEST_SERVER:$TEST_HTTP_PORT/api/v1/third/devices/$TEST_DEVICE1/charge" 2>&1)
    
    rm -f "$payload_file"
    
    echo "$ab_output"
    echo ""
    
    # 提取关键指标
    local requests_per_sec=$(echo "$ab_output" | grep "Requests per second" | awk '{print $4}')
    local time_per_request=$(echo "$ab_output" | grep "Time per request" | head -1 | awk '{print $4}')
    local failed_requests=$(echo "$ab_output" | grep "Failed requests" | awk '{print $3}')
    local success_rate=$(echo "$ab_output" | grep "Complete requests" | awk '{print $3}')
    
    print_separator
    echo "测试结果:"
    echo "  QPS: $requests_per_sec req/s"
    echo "  平均响应时间: ${time_per_request}ms"
    echo "  失败请求: $failed_requests"
    echo "  成功请求: $success_rate"
    print_separator
    
    # 验证结果
    local qps_ok=0
    local latency_ok=0
    local success_ok=0
    
    if [ -n "$requests_per_sec" ] && [ "$(echo "$requests_per_sec > 10" | bc)" = "1" ]; then
        qps_ok=1
    fi
    
    if [ -n "$time_per_request" ] && [ "$(echo "$time_per_request < 500" | bc)" = "1" ]; then
        latency_ok=1
    fi
    
    if [ "$failed_requests" = "0" ] || [ -z "$failed_requests" ]; then
        success_ok=1
    fi
    
    echo ""
    if [ $qps_ok -eq 1 ]; then
        test_passed "QPS符合预期 (>10)"
    else
        test_failed "QPS过低 (<10)"
    fi
    
    if [ $latency_ok -eq 1 ]; then
        test_passed "响应时间符合预期 (<500ms)"
    else
        test_failed "响应时间过慢 (>500ms)"
    fi
    
    if [ $success_ok -eq 1 ]; then
        test_passed "无失败请求"
    else
        test_failed "存在失败请求: $failed_requests"
    fi
}

# ==================== 测试2: 心跳处理性能 ====================
test_heartbeat_performance() {
    run_test "心跳处理性能测试"
    
    echo ""
    print_info "查询活跃设备数..."
    
    echo "请在服务器上执行以下命令:"
    echo ""
    echo "  docker exec -it iot-postgres-prod psql -U iot -d iot_server -c \\"
    echo "  \"SELECT COUNT(*) as active_devices"
    echo "   FROM devices"
    echo "   WHERE last_seen_at > NOW() - INTERVAL '30 seconds';\""
    echo ""
    
    read -p "活跃设备数: " active_devices
    
    if [ -n "$active_devices" ] && [ "$active_devices" -gt 0 ]; then
        test_passed "活跃设备数: $active_devices"
    else
        test_warning "活跃设备数较少或为空"
    fi
    
    echo ""
    print_info "查看最近的心跳处理日志..."
    echo ""
    echo "在服务器上执行:"
    echo "  docker logs --tail 100 iot-server-prod | grep \"0x0000\\|0x0021\" | tail -10"
    echo ""
    
    read -p "按回车继续..." 
}

# ==================== 测试3: 数据库性能检查 ====================
test_database_performance() {
    run_test "数据库性能检查"
    
    echo ""
    print_warning "此测试需要在服务器上执行SQL查询"
    echo ""
    
    echo "1. 检查慢查询:"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "docker exec -it iot-postgres-prod psql -U iot -d iot_server -c \\"
    echo "\"SELECT query, calls, total_exec_time, mean_exec_time"
    echo " FROM pg_stat_statements"
    echo " ORDER BY mean_exec_time DESC"
    echo " LIMIT 10;\""
    echo ""
    
    read -p "是否有慢查询 (>100ms)? (y/n): " has_slow
    
    if [ "$has_slow" = "n" ] || [ "$has_slow" = "N" ]; then
        test_passed "无明显慢查询"
    else
        test_warning "存在慢查询，需要优化"
    fi
    
    echo ""
    echo "2. 检查表大小:"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "docker exec -it iot-postgres-prod psql -U iot -d iot_server -c \\"
    echo "\"SELECT"
    echo "     tablename,"
    echo "     pg_size_pretty(pg_total_relation_size(schemaname||'.'||tablename)) AS size"
    echo " FROM pg_tables"
    echo " WHERE schemaname = 'public'"
    echo " ORDER BY pg_total_relation_size(schemaname||'.'||tablename) DESC;\""
    echo ""
    
    read -p "按回车继续..." 
    
    echo ""
    echo "3. 检查数据库连接:"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "docker exec -it iot-postgres-prod psql -U iot -d iot_server -c \\"
    echo "\"SELECT count(*) as connection_count FROM pg_stat_activity;\""
    echo ""
    
    read -p "连接数: " conn_count
    
    if [ -n "$conn_count" ] && [ "$conn_count" -lt 50 ]; then
        test_passed "数据库连接数正常: $conn_count"
    elif [ -n "$conn_count" ]; then
        test_warning "数据库连接数较多: $conn_count"
    fi
}

# ==================== 测试4: 系统资源使用 ====================
test_system_resources() {
    run_test "系统资源使用检查"
    
    echo ""
    print_info "查看容器资源使用情况..."
    echo ""
    echo "在服务器上执行:"
    echo "  docker stats --no-stream iot-server-prod"
    echo ""
    
    read -p "按回车继续..." 
    
    echo ""
    read -p "CPU使用率 (%): " cpu_usage
    read -p "内存使用 (MB): " mem_usage
    
    echo ""
    
    if [ -n "$cpu_usage" ]; then
        if [ "$(echo "$cpu_usage < 50" | bc)" = "1" ]; then
            test_passed "CPU使用率正常: ${cpu_usage}%"
        else
            test_warning "CPU使用率较高: ${cpu_usage}%"
        fi
    fi
    
    if [ -n "$mem_usage" ]; then
        if [ "$(echo "$mem_usage < 2000" | bc)" = "1" ]; then
            test_passed "内存使用正常: ${mem_usage}MB"
        else
            test_warning "内存使用较高: ${mem_usage}MB"
        fi
    fi
}

# ==================== 测试5: Redis性能 ====================
test_redis_performance() {
    run_test "Redis性能检查"
    
    echo ""
    print_info "检查Redis状态..."
    echo ""
    echo "在服务器上执行:"
    echo "  docker exec -it iot-redis-prod redis-cli -a 123456 INFO memory"
    echo ""
    
    read -p "按回车继续..." 
    
    echo ""
    echo "检查关键队列:"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "docker exec -it iot-redis-prod redis-cli -a 123456 LLEN \"thirdparty:event_queue\""
    echo ""
    
    read -p "事件队列长度: " queue_len
    
    if [ -n "$queue_len" ] && [ "$queue_len" -lt 100 ]; then
        test_passed "事件队列长度正常: $queue_len"
    elif [ -n "$queue_len" ]; then
        test_warning "事件队列积压较多: $queue_len"
    fi
}

# ==================== 主测试流程 ====================
print_header "性能压力测试套件"
echo "测试服务器: $TEST_SERVER"
echo "测试时间: $(date '+%Y-%m-%d %H:%M:%S')"
echo ""

# 检查工具
check_tools
echo ""

# 执行所有测试
test_api_concurrency
echo ""

test_heartbeat_performance
echo ""

test_database_performance
echo ""

test_system_resources
echo ""

test_redis_performance
echo ""

# ==================== 测试总结 ====================
generate_test_report "性能压力测试" $PASSED_TESTS $FAILED_TESTS

exit $([ $FAILED_TESTS -eq 0 ] && echo 0 || echo 1)

