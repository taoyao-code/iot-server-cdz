#!/bin/bash
# 充电流程自动化测试脚本
# 使用: ./scripts/test-charge-flow.sh

set -e

# 配置
SERVER="182.43.177.92"
API_KEY="sk_test_thirdparty_key_for_testing_12345678"
DEVICE_ID="82210225000520"
PORT_NO=1
SSH_KEY="$HOME/.ssh/id_rsa"

# 日志文件
LOG_DIR="test/logs"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
LOG_FILE="$LOG_DIR/charge_test_$TIMESTAMP.log"

# 创建日志目录
mkdir -p "$LOG_DIR"

# 日志函数
log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1" | tee -a "$LOG_FILE"
}

log_api() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] API: $1" | tee -a "$LOG_FILE"
}

log_db() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] DB: $1" | tee -a "$LOG_FILE"
}

log "======================================"
log "充电流程自动化测试开始"
log "======================================"
log "测试配置:"
log "  服务器: $SERVER"
log "  设备ID: $DEVICE_ID"
log "  端口号: $PORT_NO"
log "  日志文件: $LOG_FILE"
log ""

# ============================================
# 步骤1: 清理测试数据
# ============================================
log "📋 [步骤1] 清理测试数据..."

# 1.1 查询设备的数据库ID
log_db "查询设备数据库ID..."
DEVICE_DB_ID=$(ssh -i "$SSH_KEY" root@$SERVER "docker exec -i iot-postgres-prod psql -U iot -d iot_server -t -c \"SELECT id FROM devices WHERE phy_id='$DEVICE_ID';\"" | xargs)

if [ -z "$DEVICE_DB_ID" ]; then
    log "❌ 设备不存在: $DEVICE_ID"
    exit 1
fi
log_db "设备数据库ID: $DEVICE_DB_ID"

# 1.2 查询待删除的订单
log_db "查询设备的所有订单..."
ssh -i "$SSH_KEY" root@$SERVER "docker exec -i iot-postgres-prod psql -U iot -d iot_server -c \"SELECT order_no, port_no, status, created_at FROM orders WHERE device_id=$DEVICE_DB_ID ORDER BY created_at DESC LIMIT 10;\"" >> "$LOG_FILE"

# 1.3 删除测试订单
log_db "删除设备的所有订单..."
DELETED_COUNT=$(ssh -i "$SSH_KEY" root@$SERVER "docker exec -i iot-postgres-prod psql -U iot -d iot_server -t -c \"DELETE FROM orders WHERE device_id=$DEVICE_DB_ID; SELECT COUNT(*);\"" | tail -1 | xargs)
log_db "已删除 $DELETED_COUNT 个订单"

# 1.4 重置端口状态
log_db "重置端口状态..."
ssh -i "$SSH_KEY" root@$SERVER "docker exec -i iot-postgres-prod psql -U iot -d iot_server -c \"UPDATE ports SET status=0, power_w=NULL WHERE device_id=$DEVICE_DB_ID AND port_no=$PORT_NO;\"" >> "$LOG_FILE"
log_db "端口状态已重置"

log "✅ [步骤1] 测试数据清理完成"
log ""

# ============================================
# 步骤2: 验证设备状态
# ============================================
log "🔍 [步骤2] 验证设备状态..."

# 2.1 检查设备在线状态
log_api "查询设备状态..."
DEVICE_STATUS=$(curl -s -H "X-API-Key: $API_KEY" "http://$SERVER:7055/api/v1/third/devices/$DEVICE_ID")
echo "$DEVICE_STATUS" | jq '.' >> "$LOG_FILE"

ONLINE=$(echo "$DEVICE_STATUS" | jq -r '.data.online')
STATUS=$(echo "$DEVICE_STATUS" | jq -r '.data.status')

log_api "设备在线: $ONLINE"
log_api "设备状态: $STATUS"

if [ "$ONLINE" != "true" ]; then
    log "⚠️  警告: 设备离线，但继续测试"
fi

log "✅ [步骤2] 设备状态验证完成"
log ""

# ============================================
# 步骤3: 查看设备心跳日志
# ============================================
log "📡 [步骤3] 查看设备最近心跳..."
ssh -i "$SSH_KEY" root@$SERVER "docker logs --since 30s iot-server-prod 2>&1 | grep -E 'BKV frame received.*$DEVICE_ID' | tail -5" >> "$LOG_FILE"
log "✅ [步骤3] 心跳日志已记录"
log ""

# ============================================
# 步骤4: 发起充电请求
# ============================================
log "⚡ [步骤4] 发起充电请求..."

CHARGE_REQUEST='{
  "port_no": '$PORT_NO',
  "charge_mode": 2,
  "amount": 5,
  "duration_minutes": 30,
  "external_order_id": "AUTO_TEST_'$TIMESTAMP'"
}'

log_api "充电请求参数:"
echo "$CHARGE_REQUEST" | jq '.' >> "$LOG_FILE"

CHARGE_RESPONSE=$(curl -s -X POST "http://$SERVER:7055/api/v1/third/devices/$DEVICE_ID/charge" \
  -H "X-API-Key: $API_KEY" \
  -H "Content-Type: application/json" \
  -d "$CHARGE_REQUEST")

echo "$CHARGE_RESPONSE" | jq '.' >> "$LOG_FILE"

ORDER_NO=$(echo "$CHARGE_RESPONSE" | jq -r '.data.order_no')
RESPONSE_CODE=$(echo "$CHARGE_RESPONSE" | jq -r '.code')

if [ "$RESPONSE_CODE" != "0" ]; then
    log "❌ 充电请求失败: $(echo $CHARGE_RESPONSE | jq -r '.message')"
    exit 1
fi

log_api "订单号: $ORDER_NO"
log "✅ [步骤4] 充电请求发送成功"
log ""

# ============================================
# 步骤5: 监控充电命令发送
# ============================================
log "📤 [步骤5] 监控充电命令发送 (等待10秒)..."
sleep 3

log "查看充电命令日志..."
ssh -i "$SSH_KEY" root@$SERVER "docker logs --since 15s iot-server-prod 2>&1 | grep -E '($ORDER_NO|0x0015|charge command|downlink)'" >> "$LOG_FILE"

log "✅ [步骤5] 命令发送日志已记录"
log ""

# ============================================
# 步骤6: 检查订单状态
# ============================================
log "📊 [步骤6] 检查订单状态..."

for i in {1..6}; do
    log "第 $i 次查询 (间隔5秒)..."
    
    ORDER_STATUS=$(curl -s -H "X-API-Key: $API_KEY" "http://$SERVER:7055/api/v1/third/orders/$ORDER_NO")
    echo "$ORDER_STATUS" | jq '.' >> "$LOG_FILE"
    
    STATUS=$(echo "$ORDER_STATUS" | jq -r '.data.status')
    log_api "订单状态: $STATUS"
    
    if [ "$STATUS" == "charging" ]; then
        log "✅ 设备已开始充电！"
        break
    fi
    
    if [ $i -lt 6 ]; then
        sleep 5
    fi
done

log "✅ [步骤6] 订单状态检查完成"
log ""

# ============================================
# 步骤7: 查看数据库最终状态
# ============================================
log "💾 [步骤7] 查看数据库最终状态..."

log_db "订单表:"
ssh -i "$SSH_KEY" root@$SERVER "docker exec -i iot-postgres-prod psql -U iot -d iot_server -c \"SELECT order_no, port_no, status, start_time, amount_cent, created_at FROM orders WHERE order_no='$ORDER_NO';\"" >> "$LOG_FILE"

log_db "端口表:"
ssh -i "$SSH_KEY" root@$SERVER "docker exec -i iot-postgres-prod psql -U iot -d iot_server -c \"SELECT device_id, port_no, status, power_w, updated_at FROM ports WHERE device_id=$DEVICE_DB_ID AND port_no=$PORT_NO;\"" >> "$LOG_FILE"

log "✅ [步骤7] 数据库状态已记录"
log ""

# ============================================
# 步骤8: 收集完整的设备日志
# ============================================
log "📋 [步骤8] 收集设备日志 (最近1分钟)..."
ssh -i "$SSH_KEY" root@$SERVER "docker logs --since 1m iot-server-prod 2>&1 | grep -E '$DEVICE_ID'" >> "$LOG_FILE"
log "✅ [步骤8] 设备日志已保存"
log ""

# ============================================
# 测试总结
# ============================================
log "======================================"
log "测试完成"
log "======================================"
log "订单号: $ORDER_NO"
log "日志文件: $LOG_FILE"
log ""
log "💡 查看完整日志: cat $LOG_FILE"
log "💡 查看服务器日志: ssh -i $SSH_KEY root@$SERVER 'docker logs --tail 100 iot-server-prod'"

