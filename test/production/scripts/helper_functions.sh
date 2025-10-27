#!/bin/bash

# 测试辅助函数库
# 功能: 提供通用的测试辅助函数
# 使用: source ./helper_functions.sh

# 配置变量
export TEST_SERVER="182.43.177.92"
export TEST_HTTP_PORT="7055"
export TEST_API_KEY="sk_test_thirdparty_key_for_testing_12345678"
export TEST_DEVICE1="82210225000520"
export TEST_DEVICE2="82241218000382"

# 颜色定义
export RED='\033[0;31m'
export GREEN='\033[0;32m'
export YELLOW='\033[1;33m'
export BLUE='\033[0;34m'
export CYAN='\033[0;36m'
export NC='\033[0m'

# 打印函数
print_header() {
    echo ""
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${BLUE}  $1${NC}"
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
}

print_success() {
    echo -e "${GREEN}✓${NC} $1"
}

print_failure() {
    echo -e "${RED}✗${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}⚠${NC} $1"
}

print_info() {
    echo -e "${CYAN}→${NC} $1"
}

# API调用函数
# 用法: api_call METHOD PATH [DATA]
api_call() {
    local method=$1
    local path=$2
    local data=$3
    
    if [ -n "$data" ]; then
        curl -s -w "\n%{http_code}" -X "$method" \
            -H "Content-Type: application/json" \
            -H "X-Api-Key: $TEST_API_KEY" \
            -d "$data" \
            "http://$TEST_SERVER:$TEST_HTTP_PORT$path"
    else
        curl -s -w "\n%{http_code}" -X "$method" \
            -H "X-Api-Key: $TEST_API_KEY" \
            "http://$TEST_SERVER:$TEST_HTTP_PORT$path"
    fi
}

# 启动充电
# 用法: start_charging DEVICE_ID PORT_NO DURATION AMOUNT
start_charging() {
    local device_id=$1
    local port_no=$2
    local duration=${3:-300}
    local amount=${4:-500}
    
    local payload=$(cat <<EOF
{
  "port_no": $port_no,
  "charge_mode": 1,
  "duration": $duration,
  "amount": $amount,
  "price_per_kwh": 150,
  "service_fee": 50
}
EOF
)
    
    api_call "POST" "/api/v1/third/devices/$device_id/charge" "$payload"
}

# 停止充电
# 用法: stop_charging DEVICE_ID PORT_NO ORDER_NO
stop_charging() {
    local device_id=$1
    local port_no=$2
    local order_no=$3
    
    local payload=$(cat <<EOF
{
  "port_no": $port_no,
  "order_no": "$order_no"
}
EOF
)
    
    api_call "POST" "/api/v1/third/devices/$device_id/stop" "$payload"
}

# 查询订单
# 用法: query_order ORDER_NO
query_order() {
    local order_no=$1
    api_call "GET" "/api/v1/third/orders/$order_no" ""
}

# 查询设备
# 用法: query_device DEVICE_ID
query_device() {
    local device_id=$1
    api_call "GET" "/api/v1/third/devices/$device_id" ""
}

# 提取订单号
# 用法: ORDER_NO=$(extract_order_no "$RESPONSE")
extract_order_no() {
    local response=$1
    # 使用sed删除最后一行（兼容macOS和Linux）
    echo "$response" | sed '$d' | jq -r '.data.order_no // empty' 2>/dev/null
}

# 提取HTTP状态码
# 用法: HTTP_CODE=$(extract_http_code "$RESPONSE")
extract_http_code() {
    local response=$1
    echo "$response" | tail -1
}

# 提取响应体
# 用法: BODY=$(extract_body "$RESPONSE")
extract_body() {
    local response=$1
    # 使用sed删除最后一行（兼容macOS和Linux）
    echo "$response" | sed '$d'
}

# 等待订单状态变化
# 用法: wait_for_order_status ORDER_NO TARGET_STATUS TIMEOUT
wait_for_order_status() {
    local order_no=$1
    local target_status=$2
    local timeout=${3:-60}
    local elapsed=0
    
    while [ $elapsed -lt $timeout ]; do
        local response=$(query_order "$order_no")
        local body=$(extract_body "$response")
        local status=$(echo "$body" | jq -r '.data.status')
        
        if [ "$status" = "$target_status" ]; then
            return 0
        fi
        
        sleep 2
        elapsed=$((elapsed + 2))
    done
    
    return 1
}

# 生成测试报告
# 用法: generate_test_report TEST_NAME PASSED FAILED
generate_test_report() {
    local test_name=$1
    local passed=$2
    local failed=$3
    local total=$((passed + failed))
    
    echo ""
    print_header "测试报告: $test_name"
    echo ""
    echo "测试时间: $(date '+%Y-%m-%d %H:%M:%S')"
    echo "总测试数: $total"
    echo -e "${GREEN}通过: $passed${NC}"
    echo -e "${RED}失败: $failed${NC}"
    echo ""
    
    if [ $failed -eq 0 ]; then
        echo -e "${GREEN}✓ 所有测试通过${NC}"
        return 0
    else
        echo -e "${RED}✗ 有 $failed 个测试失败${NC}"
        return 1
    fi
}

# 记录测试结果到文件
# 用法: log_test_result TEST_NAME RESULT DETAILS
log_test_result() {
    local test_name=$1
    local result=$2
    local details=$3
    local log_file="${TEST_LOG_FILE:-test_results.log}"
    
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $test_name: $result - $details" >> "$log_file"
}

# 检查服务健康
check_service_health() {
    local response=$(curl -s "http://$TEST_SERVER:$TEST_HTTP_PORT/healthz")
    
    if echo "$response" | grep -q "healthy\|ok"; then
        return 0
    else
        return 1
    fi
}

# 打印分隔线
print_separator() {
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
}

echo "测试辅助函数库已加载"
echo "可用函数: api_call, start_charging, stop_charging, query_order, query_device, wait_for_order_status, generate_test_report"

