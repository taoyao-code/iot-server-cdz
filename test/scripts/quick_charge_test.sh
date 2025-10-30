#!/bin/bash
# 快速充电测试（直接执行，无复杂逻辑）
# 使用: ./quick_charge_test.sh [duration_seconds]

# 不使用 set -e，允许测试继续
# set -e

SERVER="182.43.177.92"
HTTP_PORT="7055"
API_KEY="sk_test_thirdparty_key_for_testing_12345678"
DEVICE_ID="${DEVICE_ID:-82210225000520}"
PORT_NO="${PORT_NO:-1}"
DURATION="${1:-60}"

echo "=========================================="
echo "快速充电测试"
echo "=========================================="
echo "设备: $DEVICE_ID"
echo "端口: $PORT_NO"
echo "时长: ${DURATION}秒"
echo "时间: $(date '+%Y-%m-%d %H:%M:%S')"
echo ""

# 步骤1: 检查设备
echo "[1/5] 检查设备状态..."
curl -s -H "X-Api-Key: $API_KEY" \
  "http://$SERVER:$HTTP_PORT/api/v1/third/devices/$DEVICE_ID" | jq '.data | {online, status, last_seen_at}'
echo ""

# 步骤2: 创建订单
echo "[2/5] 创建充电订单..."
RESPONSE=$(curl -s -H "Content-Type: application/json" \
  -H "X-Api-Key: $API_KEY" \
  -d "{\"port_no\": $PORT_NO, \"charge_mode\": 1, \"duration\": $DURATION, \"amount\": 500, \"price_per_kwh\": 150, \"service_fee\": 50}" \
  "http://$SERVER:$HTTP_PORT/api/v1/third/devices/$DEVICE_ID/charge")

ORDER_NO=$(echo "$RESPONSE" | jq -r '.data.order_no // empty')

if [ -z "$ORDER_NO" ]; then
    echo "❌ 订单创建失败"
    echo "$RESPONSE" | jq '.'
    exit 1
fi

echo "✅ 订单创建成功: $ORDER_NO"
echo ""

# 步骤3: 等待指令下发
echo "[3/5] 等待指令下发 (10秒)..."
sleep 10
echo "✅ 完成"
echo ""

# 步骤4: 提示插入充电插头
echo "[4/5] 🔌 请在设备端口 $PORT_NO 插入充电插头"
echo "观察端口灯光是否变化..."
read -p "按回车继续..." 
echo ""

# 步骤5: 监控充电进度
echo "[5/5] 监控充电进度 (${DURATION}秒 + 30秒缓冲)..."
echo ""
printf "%-10s | %-8s | %-8s | %-7s | %-10s\n" "时间" "时长(秒)" "电量(kWh)" "功率(W)" "状态"
echo "-----------|----------|----------|---------|------------"

MONITOR_TIME=$((DURATION + 30))
for i in $(seq 1 $((MONITOR_TIME / 10))); do
    sleep 10
    
    RESP=$(curl -s -H "X-Api-Key: $API_KEY" \
      "http://$SERVER:$HTTP_PORT/api/v1/third/orders/$ORDER_NO")
    
    STATUS=$(echo "$RESP" | jq -r '.data.status')
    DUR=$(echo "$RESP" | jq -r '.data.duration_sec // 0')
    KWH=$(echo "$RESP" | jq -r '.data.total_kwh // 0')
    POWER=$(echo "$RESP" | jq -r '.data.current_power // 0')
    
    printf "%-10s | %-8s | %-8s | %-7s | %-10s\n" \
        "$(date '+%H:%M:%S')" "$DUR" "$KWH" "$POWER" "$STATUS"
    
    if [ "$STATUS" = "completed" ]; then
        echo ""
        echo "✅ 充电已完成"
        break
    fi
done

# 显示最终结果
echo ""
echo "=========================================="
echo "最终结果:"
echo "=========================================="
curl -s -H "X-Api-Key: $API_KEY" \
  "http://$SERVER:$HTTP_PORT/api/v1/third/orders/$ORDER_NO" | jq '.data'
echo ""

echo "✅ 测试完成"
echo "订单号: $ORDER_NO"
echo ""

