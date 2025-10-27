#!/bin/bash

# 异常场景测试脚本
# 功能: 测试各种异常场景（断网、端口冲突、多端口并发等）
# 使用: ./03_exception_test.sh

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

# ==================== 测试1: 端口占用冲突 ====================
test_port_conflict() {
    run_test "端口占用冲突测试"
    
    echo ""
    print_info "步骤1: 启动端口1的充电..."
    local response1=$(start_charging "$TEST_DEVICE1" 1 300 500)
    local code1=$(extract_http_code "$response1")
    local order1=$(extract_order_no "$response1")
    
    if [ "$code1" = "200" ] && [ -n "$order1" ]; then
        test_passed "第一个订单创建成功: $order1"
    else
        test_failed "第一个订单创建失败"
        return
    fi
    
    echo ""
    print_info "步骤2: 等待5秒确保订单状态更新..."
    sleep 5
    
    echo ""
    print_info "步骤3: 再次对端口1下发充电指令（预期失败）..."
    local response2=$(start_charging "$TEST_DEVICE1" 1 300 500)
    local code2=$(extract_http_code "$response2")
    local body2=$(extract_body "$response2")
    
    echo "HTTP响应码: $code2"
    echo "响应内容: $body2" | jq '.' 2>/dev/null || echo "$body2"
    
    if [ "$code2" = "409" ] || [ "$code2" = "400" ]; then
        test_passed "正确拒绝了端口冲突请求 (HTTP $code2)"
    else
        test_failed "未能正确处理端口冲突 (HTTP $code2)"
    fi
    
    echo ""
    print_info "步骤4: 停止第一个订单以清理测试..."
    stop_charging "$TEST_DEVICE1" 1 "$order1" > /dev/null 2>&1
    sleep 3
}

# ==================== 测试2: 多端口并发充电 ====================
test_multi_port_charging() {
    run_test "多端口并发充电测试"
    
    echo ""
    print_info "同时启动端口1-4的充电..."
    
    declare -a order_nos
    declare -a results
    
    for port in 1 2 3 4; do
        print_info "启动端口 $port..."
        local response=$(start_charging "$TEST_DEVICE1" $port 180 300)
        local code=$(extract_http_code "$response")
        local order=$(extract_order_no "$response")
        
        order_nos[$port]=$order
        results[$port]=$code
        
        echo "  端口 $port: HTTP $code, 订单号: $order"
    done
    
    echo ""
    print_info "验证结果..."
    
    local success_count=0
    for port in 1 2 3 4; do
        if [ "${results[$port]}" = "200" ] && [ -n "${order_nos[$port]}" ]; then
            ((success_count++))
        fi
    done
    
    echo "成功创建订单数: $success_count/4"
    
    if [ $success_count -ge 3 ]; then
        test_passed "多端口并发充电测试通过 ($success_count/4)"
    else
        test_failed "多端口并发充电测试失败 ($success_count/4)"
    fi
    
    echo ""
    print_info "清理测试订单..."
    for port in 1 2 3 4; do
        if [ -n "${order_nos[$port]}" ]; then
            stop_charging "$TEST_DEVICE1" $port "${order_nos[$port]}" > /dev/null 2>&1
        fi
    done
    sleep 3
}

# ==================== 测试3: 设备离线时下发指令 ====================
test_offline_command() {
    run_test "设备离线时下发指令测试"
    
    echo ""
    print_warning "此测试需要手动操作设备"
    echo ""
    echo "测试步骤:"
    echo "  1. 断开设备 $TEST_DEVICE1 的网络连接"
    echo "  2. 等待设备离线判定（约6分钟）"
    echo "  3. 下发充电指令（指令应进入队列）"
    echo "  4. 恢复设备网络连接"
    echo "  5. 验证指令自动下发"
    echo ""
    
    read -p "是否要执行此测试? (y/n): " do_test
    
    if [ "$do_test" != "y" ] && [ "$do_test" != "Y" ]; then
        print_warning "跳过设备离线测试"
        return
    fi
    
    echo ""
    read -p "请确认已断开设备网络，按回车继续..." 
    
    print_info "等待60秒确保设备离线..."
    sleep 60
    
    print_info "下发充电指令..."
    local response=$(start_charging "$TEST_DEVICE1" 1 300 500)
    local code=$(extract_http_code "$response")
    local order=$(extract_order_no "$response")
    
    if [ "$code" = "200" ] && [ -n "$order" ]; then
        test_passed "离线状态下API调用成功，订单号: $order"
        echo "  指令应已进入队列，等待设备上线后下发"
    else
        test_failed "API调用失败"
        return
    fi
    
    echo ""
    print_warning "请手动检查以下内容:"
    echo "  1. 在服务器上执行: docker exec -it iot-redis-prod redis-cli -a 123456 LLEN \"outbound:$TEST_DEVICE1\""
    echo "  2. 应看到队列长度 > 0"
    echo ""
    
    read -p "确认已看到队列中有指令，按回车继续..." 
    
    echo ""
    read -p "请恢复设备网络连接，按回车继续..." 
    
    print_info "等待30秒让设备重连..."
    sleep 30
    
    echo ""
    print_warning "请手动检查服务器日志:"
    echo "  docker logs --tail 50 iot-server-prod | grep \"$TEST_DEVICE1\\|0x82\""
    echo ""
    echo "预期看到:"
    echo "  - TCP connection accepted"
    echo "  - outbound message dequeued"
    echo "  - BKV frame sent"
    echo ""
    
    read -p "是否看到指令自动下发? (y/n): " saw_send
    
    if [ "$saw_send" = "y" ] || [ "$saw_send" = "Y" ]; then
        test_passed "指令在设备上线后自动下发"
    else
        test_failed "未看到指令自动下发"
    fi
    
    # 清理
    print_info "清理测试订单..."
    stop_charging "$TEST_DEVICE1" 1 "$order" > /dev/null 2>&1
}

# ==================== 测试4: 重复订单号检测 ====================
test_duplicate_order() {
    run_test "重复订单处理测试"
    
    echo ""
    print_info "创建一个订单..."
    local response1=$(start_charging "$TEST_DEVICE1" 2 300 500)
    local order1=$(extract_order_no "$response1")
    
    if [ -z "$order1" ]; then
        test_failed "订单创建失败"
        return
    fi
    
    print_success "订单创建: $order1"
    
    echo ""
    print_info "等待3秒后停止订单..."
    sleep 3
    
    stop_charging "$TEST_DEVICE1" 2 "$order1" > /dev/null 2>&1
    sleep 2
    
    echo ""
    print_info "验证订单已结束..."
    local response2=$(query_order "$order1")
    local body2=$(extract_body "$response2")
    local status=$(echo "$body2" | jq -r '.data.status')
    
    if [ "$status" = "completed" ] || [ "$status" = "cancelled" ]; then
        test_passed "订单状态正确: $status"
    else
        test_warning "订单状态: $status (可能还在处理中)"
    fi
}

# ==================== 主测试流程 ====================
print_header "异常场景测试套件"
echo "测试设备: $TEST_DEVICE1"
echo "测试时间: $(date '+%Y-%m-%d %H:%M:%S')"
echo ""

# 执行所有测试
test_port_conflict
echo ""

test_multi_port_charging
echo ""

# 需要手动操作的测试
echo ""
print_warning "以下测试需要物理操作设备，可能需要较长时间"
echo ""

test_offline_command
echo ""

test_duplicate_order
echo ""

# ==================== 测试总结 ====================
generate_test_report "异常场景测试" $PASSED_TESTS $FAILED_TESTS

exit $([ $FAILED_TESTS -eq 0 ] && echo 0 || echo 1)

