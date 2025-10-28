#!/bin/bash

DEVICE_ID="${1:-82210225000520}"
INTERVAL=2

echo "=========================================="
echo "设备断网恢复监控"
echo "设备ID: $DEVICE_ID"
echo "监控间隔: ${INTERVAL}秒"
echo "=========================================="
echo ""
echo "请按以下步骤操作:"
echo "1. 运行此脚本开始监控"
echo "2. 手动断开设备网络（拔网线或关闭设备网络）"
echo "3. 等待10-30秒"
echo "4. 恢复设备网络连接"
echo "5. 观察设备重连和数据同步"
echo ""
echo "开始监控..."
echo "按 Ctrl+C 停止"
echo ""

LAST_STATUS="unknown"
CHECK_COUNT=0

while true; do
  CHECK_COUNT=$((CHECK_COUNT + 1))
  TIMESTAMP=$(date '+%Y-%m-%d %H:%M:%S')
  
  # 检查设备在线状态（通过TCP连接）
  SERVER_LOG=$(ssh root@182.43.177.92 "docker logs --tail 5 iot-server-prod 2>&1 | grep -E '$DEVICE_ID.*connected|$DEVICE_ID.*disconnected|$DEVICE_ID.*heartbeat'" 2>/dev/null)
  
  # 检查最近是否有心跳
  RECENT_HEARTBEAT=$(ssh root@182.43.177.92 "docker logs --since 5s iot-server-prod 2>&1 | grep -c '$DEVICE_ID'" 2>/dev/null || echo "0")
  
  if [ "$RECENT_HEARTBEAT" -gt 0 ]; then
    CURRENT_STATUS="ONLINE"
  else
    CURRENT_STATUS="OFFLINE"
  fi
  
  # 状态变化时显示
  if [ "$CURRENT_STATUS" != "$LAST_STATUS" ]; then
    echo ""
    echo "[$TIMESTAMP] 检测到状态变化: $LAST_STATUS -> $CURRENT_STATUS"
    echo "----------------------------------------"
    
    if [ "$CURRENT_STATUS" = "ONLINE" ]; then
      echo "✅ 设备已上线/重连"
      echo "   正在检查数据同步..."
      
      # 等待几秒让数据同步
      sleep 3
      
      # 检查最近的订单状态
      echo "   最近订单状态:"
      curl -s -H "X-Api-Key: sk_test_thirdparty_key_for_testing_12345678" \
        "http://182.43.177.92:7055/api/v1/third/orders?device_id=$DEVICE_ID&limit=3" \
        | jq -r '.data.orders[]? | "     订单: \(.order_no) - 状态: \(.status) - 端口: \(.port_no)"' 2>/dev/null || echo "     N/A"
    else
      echo "❌ 设备已离线/断开"
    fi
    
    LAST_STATUS="$CURRENT_STATUS"
  else
    # 每10次检查显示一次状态
    if [ $((CHECK_COUNT % 5)) -eq 0 ]; then
      echo -n "."
    fi
  fi
  
  sleep $INTERVAL
done
