#!/bin/bash

# 基础充电流程测试脚本
# 功能: 测试完整的充电流程（启动→充电→结束）
# 使用: ./02_basic_charging_test.sh [device_id] [port_no]

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

# 配置变量
SERVER="182.43.177.92"
HTTP_PORT="7055"
API_KEY="sk_test_thirdparty_key_for_testing_12345678"
DEVICE_ID="${1:-82210225000520}"
PORT_NO="${2:-1}"

# 测试结果
TEST_PASSED=0
TEST_FAILED=0
ORDER_NO=""

print_header() {
    echo ""
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${BLUE}  $1${NC}"
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
}

print_success() {
    echo -e "${GREEN}✓${NC} $1"
    ((TEST_PASSED++))
}

print_failure() {
    echo -e "${RED}✗${NC} $1"
    ((TEST_FAILED++))
}

print_info() {
    echo -e "${CYAN}→${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}⚠${NC} $1"
}

# 检查依赖
check_dependencies() {
    local missing=0
    
    if ! command -v curl &> /dev/null; then
        echo "错误: 需要安装 curl"
        missing=1
    fi
    
    if ! command -v jq &> /dev/null; then
        echo "错误: 需要安装 jq"
        missing=1
    fi
    
    if [ $missing -eq 1 ]; then
        exit 1
    fi
}

# API调用辅助函数
api_call() {
    local method=$1
    local path=$2
    local data=$3
    
    if [ -n "$data" ]; then
        curl -s -w "\n%{http_code}" -X "$method" \
            -H "Content-Type: application/json" \
            -H "X-Api-Key: $API_KEY" \
            -d "$data" \
            "http://$SERVER:$HTTP_PORT$path"
    else
        curl -s -w "\n%{http_code}" -X "$method" \
            -H "X-Api-Key: $API_KEY" \
            "http://$SERVER:$HTTP_PORT$path"
    fi
}

# 开始测试
print_header "IoT充电桩基础充电流程测试"
echo "测试设备: $DEVICE_ID"
echo "测试端口: $PORT_NO"
echo "测试时间: $(date '+%Y-%m-%d %H:%M:%S')"
echo ""

check_dependencies

# ==================== 测试步骤1: 调用充电API ====================
print_header "步骤1: 启动充电"

CHARGE_PAYLOAD=$(cat <<EOF
{
  "port_no": $PORT_NO,
  "charge_mode": 1,
  "duration": 300,
  "amount": 500,
  "price_per_kwh": 150,
  "service_fee": 50
}
EOF
)

print_info "发送充电请求..."
echo "请求数据: $CHARGE_PAYLOAD"

RESPONSE=$(api_call "POST" "/api/v1/third/devices/$DEVICE_ID/charge" "$CHARGE_PAYLOAD")
HTTP_CODE=$(echo "$RESPONSE" | tail -1)
BODY=$(echo "$RESPONSE" | sed '$d')

echo ""
echo "HTTP响应码: $HTTP_CODE"
echo "响应内容: $BODY" | jq '.' 2>/dev/null || echo "$BODY"
echo ""

if [ "$HTTP_CODE" = "200" ]; then
    print_success "API调用成功"
    
    # 提取订单号
    ORDER_NO=$(echo "$BODY" | jq -r '.data.order_no' 2>/dev/null)
    
    if [ -n "$ORDER_NO" ] && [ "$ORDER_NO" != "null" ]; then
        print_success "订单创建成功: $ORDER_NO"
        echo ""
        echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
        echo "📝 请记录订单号: $ORDER_NO"
        echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
        echo ""
    else
        print_failure "未能获取订单号"
        echo "请手动检查响应内容"
        exit 1
    fi
else
    print_failure "API调用失败 (HTTP $HTTP_CODE)"
    echo "响应: $BODY"
    exit 1
fi

# ==================== 测试步骤2: 等待指令下发 ====================
print_header "步骤2: 验证指令下发"

print_info "等待5秒让指令下发到设备..."
sleep 5

print_warning "请手动在服务器上执行以下命令查看日志:"
echo ""
echo "  docker logs --tail 50 iot-server-prod | grep \"0x82\\|$DEVICE_ID\""
echo ""
echo "预期看到:"
echo "  - {\"msg\":\"outbound message enqueued\",\"device\":\"$DEVICE_ID\",\"cmd\":\"0x82\"}"
echo "  - {\"msg\":\"BKV frame sent\",\"gateway_id\":\"$DEVICE_ID\",\"cmd\":\"0x82\"}"
echo ""

read -p "是否看到指令下发日志? (y/n): " SAW_COMMAND
if [ "$SAW_COMMAND" = "y" ] || [ "$SAW_COMMAND" = "Y" ]; then
    print_success "指令下发确认"
else
    print_failure "未看到指令下发日志"
fi

# ==================== 测试步骤3: 物理插入插头 ====================
print_header "步骤3: 插入充电插头"

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "🔌 请立即执行物理操作:"
echo "   1. 将充电插头插入设备端口 $PORT_NO"
echo "   2. 观察端口指示灯是否亮起"
echo "   3. 等待5秒让设备检测插入"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

read -p "按回车继续（插入后）..." 

print_info "等待10秒让设备确认订单..."
sleep 10

# ==================== 测试步骤4: 验证订单状态 ====================
print_header "步骤4: 验证订单状态"

print_info "查询订单状态..."
RESPONSE=$(api_call "GET" "/api/v1/third/orders/$ORDER_NO" "")
HTTP_CODE=$(echo "$RESPONSE" | tail -1)
BODY=$(echo "$RESPONSE" | sed '$d')

if [ "$HTTP_CODE" = "200" ]; then
    STATUS=$(echo "$BODY" | jq -r '.data.status' 2>/dev/null)
    
    echo "当前订单状态: $STATUS"
    
    if [ "$STATUS" = "charging" ]; then
        print_success "订单状态正确: charging"
    elif [ "$STATUS" = "pending" ]; then
        print_warning "订单仍在pending状态，可能插头未正确插入"
    else
        print_failure "订单状态异常: $STATUS"
    fi
else
    print_failure "查询订单失败 (HTTP $HTTP_CODE)"
fi

# ==================== 测试步骤5: 监控充电进度 ====================
print_header "步骤5: 监控充电进度 (5分钟)"

print_info "开始监控充电进度，每10秒查询一次..."
echo ""
echo "时间       | 时长(秒) | 电量(度) | 功率(W) | 状态"
echo "-----------|----------|----------|---------|----------"

MONITOR_ROUNDS=30  # 5分钟 = 30 * 10秒

for i in $(seq 1 $MONITOR_ROUNDS); do
    sleep 10
    
    RESPONSE=$(api_call "GET" "/api/v1/third/orders/$ORDER_NO" "")
    HTTP_CODE=$(echo "$RESPONSE" | tail -1)
    BODY=$(echo "$RESPONSE" | sed '$d')
    
    if [ "$HTTP_CODE" = "200" ]; then
        DURATION=$(echo "$BODY" | jq -r '.data.duration_sec // 0')
        KWH=$(echo "$BODY" | jq -r '.data.total_kwh // 0')
        POWER=$(echo "$BODY" | jq -r '.data.current_power // 0')
        STATUS=$(echo "$BODY" | jq -r '.data.status')
        
        printf "%-10s | %-8s | %-8s | %-7s | %s\n" \
            "$(date '+%H:%M:%S')" "$DURATION" "$KWH" "$POWER" "$STATUS"
        
        # 如果已完成，提前退出
        if [ "$STATUS" = "completed" ]; then
            echo ""
            print_success "充电已完成!"
            break
        fi
    else
        echo "查询失败"
    fi
done

echo ""

# ==================== 测试步骤6: 拔出插头或等待结束 ====================
print_header "步骤6: 充电结束"

RESPONSE=$(api_call "GET" "/api/v1/third/orders/$ORDER_NO" "")
BODY=$(echo "$RESPONSE" | sed '$d')
STATUS=$(echo "$BODY" | jq -r '.data.status')

if [ "$STATUS" != "completed" ]; then
    echo ""
    echo "选择结束方式:"
    echo "  1) 继续等待自动结束 (剩余时间)"
    echo "  2) 手动拔出插头"
    echo "  3) 调用远程停止API"
    echo ""
    
    read -p "请选择 (1/2/3): " END_CHOICE
    
    case $END_CHOICE in
        1)
            print_info "继续等待..."
            # 继续监控
            while true; do
                sleep 10
                RESPONSE=$(api_call "GET" "/api/v1/third/orders/$ORDER_NO" "")
                BODY=$(echo "$RESPONSE" | sed '$d')
                STATUS=$(echo "$BODY" | jq -r '.data.status')
                
                if [ "$STATUS" = "completed" ]; then
                    print_success "充电已完成"
                    break
                fi
            done
            ;;
        2)
            echo ""
            echo "请拔出充电插头..."
            read -p "按回车确认已拔出..." 
            print_info "等待10秒让设备上报结算..."
            sleep 10
            ;;
        3)
            print_info "调用停止API..."
            STOP_PAYLOAD="{\"port_no\": $PORT_NO, \"order_no\": \"$ORDER_NO\"}"
            RESPONSE=$(api_call "POST" "/api/v1/third/devices/$DEVICE_ID/stop" "$STOP_PAYLOAD")
            HTTP_CODE=$(echo "$RESPONSE" | tail -1)
            
            if [ "$HTTP_CODE" = "200" ]; then
                print_success "停止指令发送成功"
            else
                print_failure "停止指令失败"
            fi
            
            sleep 10
            ;;
        *)
            print_warning "无效选择，继续等待..."
            ;;
    esac
fi

# ==================== 测试步骤7: 验证最终结算 ====================
print_header "步骤7: 验证订单结算"

print_info "查询最终订单数据..."
RESPONSE=$(api_call "GET" "/api/v1/third/orders/$ORDER_NO" "")
HTTP_CODE=$(echo "$RESPONSE" | tail -1)
BODY=$(echo "$RESPONSE" | sed '$d')

if [ "$HTTP_CODE" = "200" ]; then
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "📊 订单最终数据:"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "$BODY" | jq '{
        order_no: .data.order_no,
        status: .data.status,
        duration_sec: .data.duration_sec,
        total_kwh: .data.total_kwh,
        current_power: .data.current_power,
        final_amount: .data.final_amount,
        end_reason: .data.end_reason
    }' 2>/dev/null || echo "$BODY"
    echo ""
    
    # 验证关键字段
    STATUS=$(echo "$BODY" | jq -r '.data.status')
    DURATION=$(echo "$BODY" | jq -r '.data.duration_sec // 0')
    KWH=$(echo "$BODY" | jq -r '.data.total_kwh // 0')
    
    if [ "$STATUS" = "completed" ]; then
        print_success "订单状态: completed"
    else
        print_failure "订单状态异常: $STATUS"
    fi
    
    if [ "$DURATION" -gt 0 ]; then
        print_success "充电时长: ${DURATION}秒"
        
        # 检查时长误差（目标300秒，允许±10秒）
        DIFF=$((DURATION - 300))
        DIFF=${DIFF#-}  # 取绝对值
        
        if [ $DIFF -le 10 ]; then
            print_success "时长误差: ${DIFF}秒 (≤10秒)"
        else
            print_warning "时长误差: ${DIFF}秒 (>10秒)"
        fi
    else
        print_failure "充电时长为0"
    fi
    
    if [ "$(echo "$KWH > 0" | bc 2>/dev/null || echo "1")" = "1" ]; then
        print_success "充电电量: ${KWH}度"
    else
        print_warning "充电电量为0"
    fi
else
    print_failure "查询订单失败"
fi

# ==================== 测试总结 ====================
print_header "测试总结"

echo ""
echo "测试设备: $DEVICE_ID"
echo "测试端口: $PORT_NO"
echo "订单号: $ORDER_NO"
echo ""
echo "通过检查: $TEST_PASSED"
echo "失败检查: $TEST_FAILED"
echo ""

if [ $TEST_FAILED -eq 0 ]; then
    echo -e "${GREEN}✓ 基础充电流程测试通过${NC}"
    echo ""
    echo "建议后续测试:"
    echo "  - 执行远程停止测试"
    echo "  - 执行异常场景测试"
    exit 0
else
    echo -e "${RED}✗ 测试发现 $TEST_FAILED 个问题${NC}"
    echo ""
    echo "请检查:"
    echo "  - 设备是否在线"
    echo "  - 端口是否空闲"
    echo "  - 网络连接是否正常"
    echo "  - 服务器日志是否有错误"
    exit 1
fi

