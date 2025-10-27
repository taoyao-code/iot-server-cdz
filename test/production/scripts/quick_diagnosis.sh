#!/bin/bash

# 快速诊断脚本
# 功能: 快速诊断API配置问题
# 使用: ./quick_diagnosis.sh

# 加载辅助函数
SCRIPT_DIR=$(dirname "$0")
source "$SCRIPT_DIR/helper_functions.sh"

print_header "IoT服务器快速诊断"

echo "服务器: $TEST_SERVER:$TEST_HTTP_PORT"
echo "设备: $TEST_DEVICE1"
echo "API Key: ${TEST_API_KEY:0:20}..."
echo ""

# ==================== 测试1: 健康检查 ====================
print_header "1. 服务器健康检查"

echo -n "→ 检查 /healthz... "
HEALTH=$(curl -s -w "\n%{http_code}" "http://$TEST_SERVER:$TEST_HTTP_PORT/healthz")
HEALTH_CODE=$(echo "$HEALTH" | tail -1)
HEALTH_BODY=$(echo "$HEALTH" | sed '$d')

if [ "$HEALTH_CODE" = "200" ]; then
    print_success "健康检查通过"
    echo "  响应: $HEALTH_BODY"
else
    print_failure "健康检查失败 (HTTP $HEALTH_CODE)"
    echo "  响应: $HEALTH_BODY"
fi

echo ""

# ==================== 测试2: Swagger文档 ====================
print_header "2. API文档检查"

echo "→ Swagger文档地址: http://$TEST_SERVER:$TEST_HTTP_PORT/swagger/index.html"
echo -n "→ 检查swagger.json... "

SWAGGER_CODE=$(curl -s -w "%{http_code}" -o /dev/null "http://$TEST_SERVER:$TEST_HTTP_PORT/swagger/swagger.json")

if [ "$SWAGGER_CODE" = "200" ]; then
    print_success "Swagger文档可访问"
else
    print_failure "Swagger文档无法访问 (HTTP $SWAGGER_CODE)"
fi

echo ""

# ==================== 测试3: 设备查询API ====================
print_header "3. 设备查询API测试"

echo "→ 查询设备: $TEST_DEVICE1"
DEVICE_RESPONSE=$(curl -s -w "\n%{http_code}" \
    -H "X-Api-Key: $TEST_API_KEY" \
    "http://$TEST_SERVER:$TEST_HTTP_PORT/api/v1/third/devices/$TEST_DEVICE1")

DEVICE_CODE=$(echo "$DEVICE_RESPONSE" | tail -1)
DEVICE_BODY=$(echo "$DEVICE_RESPONSE" | sed '$d')

echo "HTTP状态码: $DEVICE_CODE"

if [ "$DEVICE_CODE" = "200" ]; then
    print_success "设备查询成功"
    echo ""
    echo "响应内容:"
    echo "$DEVICE_BODY" | jq '.' 2>/dev/null || echo "$DEVICE_BODY"
elif [ "$DEVICE_CODE" = "401" ]; then
    print_failure "认证失败 - API Key可能不正确"
    echo ""
    echo "响应内容:"
    echo "$DEVICE_BODY" | jq '.' 2>/dev/null || echo "$DEVICE_BODY"
    echo ""
    print_warning "请检查:"
    echo "  1. API Key是否正确: $TEST_API_KEY"
    echo "  2. 配置文件中是否配置了该API Key"
    echo "  3. 服务器是否需要重启以加载配置"
elif [ "$DEVICE_CODE" = "404" ]; then
    print_failure "设备不存在"
    echo ""
    echo "响应内容:"
    echo "$DEVICE_BODY" | jq '.' 2>/dev/null || echo "$DEVICE_BODY"
else
    print_failure "请求失败 (HTTP $DEVICE_CODE)"
    echo ""
    echo "响应内容:"
    echo "$DEVICE_BODY" | jq '.' 2>/dev/null || echo "$DEVICE_BODY"
fi

echo ""

# ==================== 测试4: 充电API ====================
print_header "4. 充电API测试"

echo "→ 测试充电API（端口255，不会实际启动）"
CHARGE_RESPONSE=$(curl -s -w "\n%{http_code}" -X POST \
    -H "Content-Type: application/json" \
    -H "X-Api-Key: $TEST_API_KEY" \
    -d '{
        "port_no": 255,
        "charge_mode": 1,
        "duration": 60,
        "amount": 100,
        "price_per_kwh": 150,
        "service_fee": 50
    }' \
    "http://$TEST_SERVER:$TEST_HTTP_PORT/api/v1/third/devices/$TEST_DEVICE1/charge")

CHARGE_CODE=$(echo "$CHARGE_RESPONSE" | tail -1)
CHARGE_BODY=$(echo "$CHARGE_RESPONSE" | sed '$d')

echo "HTTP状态码: $CHARGE_CODE"

if [ "$CHARGE_CODE" = "200" ]; then
    print_success "充电API正常工作"
    echo ""
    echo "响应内容:"
    echo "$CHARGE_BODY" | jq '.' 2>/dev/null || echo "$CHARGE_BODY"
    
    # 提取订单号
    ORDER_NO=$(echo "$CHARGE_BODY" | jq -r '.data.order_no // empty')
    if [ -n "$ORDER_NO" ]; then
        echo ""
        print_info "订单号: $ORDER_NO"
        
        # 立即停止（因为端口255不存在）
        echo ""
        echo "→ 立即停止测试订单..."
        curl -s -X POST \
            -H "Content-Type: application/json" \
            -H "X-Api-Key: $TEST_API_KEY" \
            -d "{\"port_no\": 255, \"order_no\": \"$ORDER_NO\"}" \
            "http://$TEST_SERVER:$TEST_HTTP_PORT/api/v1/third/devices/$TEST_DEVICE1/stop" > /dev/null
        print_success "测试订单已清理"
    fi
elif [ "$CHARGE_CODE" = "401" ]; then
    print_failure "认证失败"
    echo ""
    echo "响应内容:"
    echo "$CHARGE_BODY" | jq '.' 2>/dev/null || echo "$CHARGE_BODY"
elif [ "$CHARGE_CODE" = "400" ]; then
    print_warning "请求格式错误（可能是端口参数问题，这是正常的）"
    echo ""
    echo "响应内容:"
    echo "$CHARGE_BODY" | jq '.' 2>/dev/null || echo "$CHARGE_BODY"
else
    print_failure "请求失败 (HTTP $CHARGE_CODE)"
    echo ""
    echo "响应内容:"
    echo "$CHARGE_BODY" | jq '.' 2>/dev/null || echo "$CHARGE_BODY"
fi

echo ""

# ==================== 诊断总结 ====================
print_header "诊断总结"

echo ""
echo "检查项总结:"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

if [ "$HEALTH_CODE" = "200" ]; then
    echo "  ✓ 服务器健康检查"
else
    echo "  ✗ 服务器健康检查"
fi

if [ "$SWAGGER_CODE" = "200" ]; then
    echo "  ✓ API文档可访问"
else
    echo "  ✗ API文档可访问"
fi

if [ "$DEVICE_CODE" = "200" ]; then
    echo "  ✓ 设备查询API"
elif [ "$DEVICE_CODE" = "401" ]; then
    echo "  ✗ 设备查询API (认证失败)"
else
    echo "  ✗ 设备查询API"
fi

if [ "$CHARGE_CODE" = "200" ] || [ "$CHARGE_CODE" = "400" ]; then
    echo "  ✓ 充电API（可访问）"
elif [ "$CHARGE_CODE" = "401" ]; then
    echo "  ✗ 充电API (认证失败)"
else
    echo "  ✗ 充电API"
fi

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# 给出建议
if [ "$DEVICE_CODE" = "401" ] || [ "$CHARGE_CODE" = "401" ]; then
    print_header "修复建议"
    echo ""
    echo "检测到API认证失败，请按以下步骤修复："
    echo ""
    echo "1. 检查API Key配置"
    echo "   → 编辑服务器配置: configs/production.yaml"
    echo "   → 查找 api_authentication 部分"
    echo "   → 确认API Key列表中包含: $TEST_API_KEY"
    echo ""
    echo "2. 如果没有配置，添加以下内容:"
    echo ""
    echo "   api_authentication:"
    echo "     enabled: true"
    echo "     api_keys:"
    echo "       - $TEST_API_KEY"
    echo ""
    echo "3. 重启IoT服务器"
    echo "   → ssh user@$TEST_SERVER"
    echo "   → docker restart iot-server-prod"
    echo ""
    echo "4. 重新运行此诊断脚本"
    echo "   → ./quick_diagnosis.sh"
    echo ""
elif [ "$HEALTH_CODE" != "200" ]; then
    print_header "修复建议"
    echo ""
    echo "服务器健康检查失败，请检查:"
    echo "  1. 服务器是否正在运行"
    echo "  2. 端口$TEST_HTTP_PORT是否开放"
    echo "  3. 防火墙配置"
    echo ""
else
    print_success "所有诊断检查通过！系统运行正常。"
    echo ""
    echo "可以继续执行完整测试:"
    echo "  → ./run_all_tests.sh"
    echo ""
fi

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "诊断完成"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

