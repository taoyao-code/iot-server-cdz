#!/bin/bash
# E2E 测试诊断脚本

set -e

API_KEY="sk_test_thirdparty_key_for_testing_12345678"
BASE_URL="http://182.43.177.92:7055"
DEVICE_ID="82241218000382"

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  E2E 测试环境诊断"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# 检查设备状态
echo "📡 检查设备状态..."
DEVICE_INFO=$(curl -s -H "X-Api-Key: $API_KEY" "$BASE_URL/api/v1/third/devices/$DEVICE_ID")
echo "$DEVICE_INFO" | jq '.'

ONLINE=$(echo "$DEVICE_INFO" | jq -r '.data.online')
STATUS=$(echo "$DEVICE_INFO" | jq -r '.data.status')
LAST_SEEN=$(echo "$DEVICE_INFO" | jq -r '.data.last_seen_at')

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "诊断结果："
echo ""

if [ "$ONLINE" = "true" ]; then
    echo "✅ 设备在线 (状态: $STATUS)"
    echo ""
    echo "可以运行测试："
    echo "  cd test/e2e && make test"
else
    echo "❌ 设备离线 (状态: $STATUS)"
    
    if [ "$LAST_SEEN" != "null" ]; then
        NOW=$(date +%s)
        OFFLINE_SECONDS=$((NOW - LAST_SEEN))
        OFFLINE_MINUTES=$((OFFLINE_SECONDS / 60))
        echo "   离线时长: ${OFFLINE_MINUTES} 分钟 (${OFFLINE_SECONDS} 秒)"
    fi
    
    echo ""
    echo "⚠️  问题：设备离线时仍可下单，但订单会一直 pending"
    echo ""
    echo "建议操作："
    echo "  1. 检查设备网络连接"
    echo "  2. 重启设备"
    echo "  3. 查看服务端日志: docker logs iot-server"
    echo ""
    echo "详细分析见: docs/设备离线下单问题分析.md"
fi

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
