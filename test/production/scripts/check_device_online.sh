#!/bin/bash

# 检查设备在线状态脚本
# 功能: 快速检查指定设备是否在线
# 使用: ./check_device_online.sh [device_id]

DEVICE_ID="${1:-82210225000520}"
SERVER="182.43.177.92"
HTTP_PORT="7055"
API_KEY="sk_test_1234567890"

# 颜色
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo "检查设备: $DEVICE_ID"
echo ""

# 方法1: 通过API查询
RESPONSE=$(curl -s -H "X-Api-Key: $API_KEY" \
    "http://$SERVER:$HTTP_PORT/api/v1/third/devices/$DEVICE_ID" 2>/dev/null)

if [ $? -eq 0 ] && [ -n "$RESPONSE" ]; then
    ONLINE=$(echo "$RESPONSE" | jq -r '.data.online // false')
    LAST_SEEN=$(echo "$RESPONSE" | jq -r '.data.last_seen_at // "N/A"')
    
    echo "API查询结果:"
    if [ "$ONLINE" = "true" ]; then
        echo -e "  状态: ${GREEN}● 在线${NC}"
    else
        echo -e "  状态: ${RED}● 离线${NC}"
    fi
    echo "  最后心跳: $LAST_SEEN"
else
    echo -e "${RED}✗ API查询失败${NC}"
fi

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "如需详细监控，请运行:"
echo "  ./monitor_device.sh $DEVICE_ID"
echo ""

