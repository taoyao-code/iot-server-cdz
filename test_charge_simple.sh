#!/bin/bash
# 最简单的充电测试 - 100%验证协议
# 使用设备: 82241218000382

API_KEY="sk_test_thirdparty_key_for_testing_12345678"
SERVER="182.43.177.92:7055"
DEVICE="82241218000382"
PORT=1

echo "=========================================="
echo "简单充电测试 - 完整日志"
echo "=========================================="
echo "设备: $DEVICE"
echo "端口: $PORT"
echo "时间: $(date '+%Y-%m-%d %H:%M:%S')"
echo ""

# 1. 创建订单
echo "[1/4] 创建充电订单..."
RESP=$(curl -s -w "\n%{http_code}" -X POST \
  -H "Content-Type: application/json" \
  -H "X-Api-Key: $API_KEY" \
  -d "{\"port_no\": $PORT, \"charge_mode\": 1, \"duration\": 60, \"amount\": 500, \"price_per_kwh\": 150, \"service_fee\": 50}" \
  "http://$SERVER/api/v1/third/devices/$DEVICE/charge")

HTTP_CODE=$(echo "$RESP" | tail -1)
BODY=$(echo "$RESP" | sed '$d')

echo "HTTP状态: $HTTP_CODE"
echo "响应:"
echo "$BODY" | jq '.'

if [ "$HTTP_CODE" != "200" ]; then
    echo "❌ 创建订单失败"
    exit 1
fi

ORDER_NO=$(echo "$BODY" | jq -r '.data.order_no')
echo ""
echo "✅ 订单号: $ORDER_NO"
echo ""

# 2. 等待指令下发
echo "[2/4] 等待10秒让指令下发..."
sleep 10
echo "✅ 完成"
echo ""

# 3. 查看服务器日志
echo "[3/4] 查看服务器日志（最近的0x0015交互）..."
ssh root@182.43.177.92 "docker logs --tail 100 iot-server-prod" | grep -E "0x0015|订单状态已更新" | tail -10
echo ""

# 4. 查询订单状态
echo "[4/4] 查询订单状态..."
for i in {1..5}; do
    echo "查询 $i/5..."
    RESP=$(curl -s -w "\n%{http_code}" -H "X-Api-Key: $API_KEY" "http://$SERVER/api/v1/third/orders/$ORDER_NO")
    HTTP=$(echo "$RESP" | tail -1)
    BODY=$(echo "$RESP" | sed '$d')
    
    STATUS=$(echo "$BODY" | jq -r '.data.status')
    echo "  状态: $STATUS"
    
    if [ "$STATUS" = "charging" ]; then
        echo "✅ 订单已变为charging状态！"
        echo ""
        echo "完整订单数据:"
        echo "$BODY" | jq '.data'
        exit 0
    fi
    
    sleep 2
done

echo ""
echo "⚠️ 订单仍是pending状态"
echo "最终数据:"
echo "$BODY" | jq '.data'
echo ""

