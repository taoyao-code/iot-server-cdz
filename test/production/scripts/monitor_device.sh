#!/bin/bash

# 设备实时监控脚本
# 功能: 实时监控设备状态、端口状态、活跃订单
# 使用: ./monitor_device.sh [device_id]

DEVICE_ID="${1:-82210225000520}"
SERVER="182.43.177.92"
HTTP_PORT="7055"
API_KEY="sk_test_1234567890"

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${BLUE}  设备实时监控 - $DEVICE_ID${NC}"
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""
echo "提示: 按 Ctrl+C 退出监控"
echo ""

while true; do
    clear
    echo -e "${CYAN}═══════════════════════════════════════════════════════════${NC}"
    echo -e "${CYAN}  设备监控面板 - $(date '+%Y-%m-%d %H:%M:%S')${NC}"
    echo -e "${CYAN}═══════════════════════════════════════════════════════════${NC}"
    echo ""
    
    # 查询设备信息
    RESPONSE=$(curl -s -H "X-Api-Key: $API_KEY" \
        "http://$SERVER:$HTTP_PORT/api/v1/third/devices/$DEVICE_ID" 2>/dev/null)
    
    if [ $? -eq 0 ] && [ -n "$RESPONSE" ]; then
        # 设备基本信息
        ONLINE=$(echo "$RESPONSE" | jq -r '.data.online // false')
        LAST_SEEN=$(echo "$RESPONSE" | jq -r '.data.last_seen_at // "N/A"')
        
        echo -e "${YELLOW}📱 设备信息${NC}"
        echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
        echo "设备ID: $DEVICE_ID"
        
        if [ "$ONLINE" = "true" ]; then
            echo -e "状态: ${GREEN}● 在线${NC}"
        else
            echo -e "状态: ${RED}● 离线${NC}"
        fi
        
        echo "最后心跳: $LAST_SEEN"
        echo ""
        
        # 端口状态
        echo -e "${YELLOW}🔌 端口状态${NC}"
        echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
        
        PORTS=$(echo "$RESPONSE" | jq -r '.data.ports[]? | 
            "端口 \(.port_no): \(
                if .status == 0 then "空闲"
                elif .status == 1 then "充电中"
                elif .status == 2 then "故障"
                else "未知(\(.status))" end
            ) | 功率: \(.power_w // 0)W"')
        
        if [ -n "$PORTS" ]; then
            echo "$PORTS"
        else
            echo "暂无端口数据"
        fi
        echo ""
        
    else
        echo -e "${RED}✗ 无法获取设备信息${NC}"
        echo ""
    fi
    
    # 活跃订单
    echo -e "${YELLOW}📋 活跃订单${NC}"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    
    ORDERS_RESPONSE=$(curl -s -H "X-Api-Key: $API_KEY" \
        "http://$SERVER:$HTTP_PORT/api/v1/third/orders?device_id=$DEVICE_ID&status=charging,pending&page=1&size=5" 2>/dev/null)
    
    if [ $? -eq 0 ] && [ -n "$ORDERS_RESPONSE" ]; then
        ORDERS=$(echo "$ORDERS_RESPONSE" | jq -r '.data.orders[]? | 
            "订单: \(.order_no) | 端口\(.port_no) | \(.status) | 时长:\(.duration_sec // 0)s | 电量:\(.total_kwh // 0)度"')
        
        if [ -n "$ORDERS" ]; then
            echo "$ORDERS"
        else
            echo "无活跃订单"
        fi
    else
        echo "无法查询订单"
    fi
    
    echo ""
    echo -e "${CYAN}═══════════════════════════════════════════════════════════${NC}"
    echo "刷新间隔: 5秒 | 按 Ctrl+C 退出"
    
    sleep 5
done

