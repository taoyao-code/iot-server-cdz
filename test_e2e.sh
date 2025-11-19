#!/bin/bash

API_BASE="http://182.43.177.92:7055"
API_KEY="sk_test_admin_key_for_testing_12345678"
DEVICE_ID="82241218000382"

echo "========================================="
echo "IoT Server E2E Test Suite"
echo "========================================="
echo ""

TOTAL=0
PASSED=0
FAILED=0

function test_case() {
    local name=$1
    echo ""
    echo "Test #$((TOTAL+1)): $name"
    echo "---"
}

function pass() {
    echo "✅ PASSED: $1"
    PASSED=$((PASSED+1))
    TOTAL=$((TOTAL+1))
}

function fail() {
    echo "❌ FAILED: $1"
    FAILED=$((FAILED+1))
    TOTAL=$((TOTAL+1))
}

# Test 1: 查询设备初始状态
test_case "查询设备初始状态"
response=$(curl -s -X GET "${API_BASE}/internal/test/devices/${DEVICE_ID}" \
    -H "Content-Type: application/json" \
    -H "X-Internal-API-Key: ${API_KEY}")
echo "$response" | jq .

port0_status=$(echo "$response" | jq -r '.data.ports[0].status')
port1_status=$(echo "$response" | jq -r '.data.ports[1].status')

if [ "$port0_status" = "0" ] && [ "$port1_status" = "0" ]; then
    pass "两个端口都是idle状态"
else
    fail "端口状态异常: port0=$port0_status, port1=$port1_status"
fi

# Test 2: device-offline场景
test_case "场景测试: device-offline"
response=$(curl -s -X POST "${API_BASE}/internal/test/devices/${DEVICE_ID}/charge" \
    -H "Content-Type: application/json" \
    -H "X-Internal-API-Key: ${API_KEY}" \
    -d '{"port_no":0,"charge_mode":1,"amount":100,"duration_minutes":5,"scenario_id":"device-offline"}')
echo "$response" | jq .

code=$(echo "$response" | jq -r '.code')
if [ "$code" = "503" ]; then
    pass "device-offline场景返回503"
else
    fail "device-offline场景返回码错误: $code (期望: 503)"
fi

# Test 3: normal-charge场景（端口A）
test_case "场景测试: normal-charge (端口A)"
response=$(curl -s -X POST "${API_BASE}/internal/test/devices/${DEVICE_ID}/charge" \
    -H "Content-Type: application/json" \
    -H "X-Internal-API-Key: ${API_KEY}" \
    -d '{"port_no":0,"charge_mode":1,"amount":200,"duration_minutes":10,"scenario_id":"normal-charge"}')
echo "$response" | jq .

code=$(echo "$response" | jq -r '.code')
order_no=$(echo "$response" | jq -r '.data.order_no')

if [ "$code" = "0" ]; then
    echo "   订单号: $order_no"

    # 等待端口状态更新
    sleep 2

    # 验证端口状态
    dev_response=$(curl -s -X GET "${API_BASE}/internal/test/devices/${DEVICE_ID}" \
        -H "X-Internal-API-Key: ${API_KEY}")
    port0_status=$(echo "$dev_response" | jq -r '.data.ports[0].status')

    if [ "$port0_status" = "1" ]; then
        pass "端口A状态正确变为occupied(1)"
    else
        fail "端口A状态错误: $port0_status (期望: 1)"
    fi
else
    fail "normal-charge创建订单失败: code=$code"
fi

# Test 4: port-busy场景（端口A已被占用）
test_case "场景测试: port-busy (端口A已占用)"
response=$(curl -s -X POST "${API_BASE}/internal/test/devices/${DEVICE_ID}/charge" \
    -H "Content-Type: application/json" \
    -H "X-Internal-API-Key: ${API_KEY}" \
    -d '{"port_no":0,"charge_mode":1,"amount":100,"duration_minutes":5,"scenario_id":"port-busy"}')
echo "$response" | jq .

code=$(echo "$response" | jq -r '.code')
current_order=$(echo "$response" | jq -r '.data.current_order')

if [ "$code" = "409" ]; then
    echo "   冲突订单: $current_order"
    pass "port-busy场景正确返回409冲突"
else
    fail "port-busy场景返回码错误: $code (期望: 409)"
fi

# Test 5: 正常充电（端口B）
test_case "正常充电测试 (端口B)"
response=$(curl -s -X POST "${API_BASE}/internal/test/devices/${DEVICE_ID}/charge" \
    -H "Content-Type: application/json" \
    -H "X-Internal-API-Key: ${API_KEY}" \
    -d '{"port_no":1,"charge_mode":1,"amount":150,"duration_minutes":8,"scenario_id":"normal-charge"}')
echo "$response" | jq .

code=$(echo "$response" | jq -r '.code')
order_no=$(echo "$response" | jq -r '.data.order_no')

if [ "$code" = "0" ]; then
    echo "   订单号: $order_no"

    # 等待端口状态更新
    sleep 2

    # 验证端口状态
    dev_response=$(curl -s -X GET "${API_BASE}/internal/test/devices/${DEVICE_ID}" \
        -H "X-Internal-API-Key: ${API_KEY}")
    port1_status=$(echo "$dev_response" | jq -r '.data.ports[1].status')

    if [ "$port1_status" = "1" ]; then
        pass "端口B状态正确变为occupied(1)"
    else
        fail "端口B状态错误: $port1_status (期望: 1)"
    fi
else
    fail "端口B创建订单失败: code=$code"
fi

# Test 6: 订单列表查询
test_case "订单列表查询"
response=$(curl -s -X GET "${API_BASE}/internal/test/orders?limit=10" \
    -H "X-Internal-API-Key: ${API_KEY}")
echo "$response" | jq .

order_count=$(echo "$response" | jq '.data | length')

if [ "$order_count" -ge "2" ]; then
    pass "成功查询到 $order_count 个订单"
else
    fail "订单数量异常: $order_count (期望 >= 2)"
fi

# Test 7: 设备最终状态验证
test_case "设备最终状态验证"
response=$(curl -s -X GET "${API_BASE}/internal/test/devices/${DEVICE_ID}" \
    -H "X-Internal-API-Key: ${API_KEY}")
echo "$response" | jq .

port0_status=$(echo "$response" | jq -r '.data.ports[0].status')
port1_status=$(echo "$response" | jq -r '.data.ports[1].status')
active_orders_count=$(echo "$response" | jq '.data.active_orders | length')

if [ "$port0_status" = "1" ] && [ "$port1_status" = "1" ]; then
    echo "   活跃订单数: $active_orders_count"
    pass "两个端口都是occupied状态"
else
    fail "端口状态异常: port0=$port0_status, port1=$port1_status"
fi

# 测试总结
echo ""
echo "========================================="
echo "Test Summary"
echo "========================================="
echo "Total:  $TOTAL"
echo "Passed: $PASSED ✅"
echo "Failed: $FAILED ❌"
echo ""

if [ $FAILED -eq 0 ]; then
    echo "🎉 All tests passed!"
    exit 0
else
    echo "⚠️  Some tests failed."
    exit 1
fi
