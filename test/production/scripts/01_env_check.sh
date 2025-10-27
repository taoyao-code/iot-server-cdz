#!/bin/bash

# IoT充电桩环境检查脚本
# 功能: 验证测试环境的所有前置条件
# 使用: ./01_env_check.sh

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 配置变量
SERVER="182.43.177.92"
HTTP_PORT="7055"
TCP_PORT="7065"
API_KEY="sk_test_1234567890"
DEVICE1="82210225000520"
DEVICE2="82241218000382"

# 检查结果统计
TOTAL_CHECKS=0
PASSED_CHECKS=0
FAILED_CHECKS=0

# 打印标题
print_header() {
    echo ""
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${BLUE}  $1${NC}"
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
}

# 打印成功信息
print_success() {
    echo -e "${GREEN}✓${NC} $1"
    ((PASSED_CHECKS++))
}

# 打印失败信息
print_failure() {
    echo -e "${RED}✗${NC} $1"
    ((FAILED_CHECKS++))
}

# 打印警告信息
print_warning() {
    echo -e "${YELLOW}⚠${NC} $1"
}

# 打印信息
print_info() {
    echo -e "${BLUE}ℹ${NC} $1"
}

# 检查命令是否存在
check_command() {
    local cmd=$1
    local name=$2
    ((TOTAL_CHECKS++))
    
    if command -v $cmd &> /dev/null; then
        print_success "$name 已安装"
        return 0
    else
        print_failure "$name 未安装"
        return 1
    fi
}

# 开始检查
print_header "IoT充电桩环境检查"
echo "测试服务器: $SERVER"
echo "测试时间: $(date '+%Y-%m-%d %H:%M:%S')"
echo ""

# ==================== 第一部分: 本地工具检查 ====================
print_header "1. 本地工具检查"

check_command "curl" "curl"
check_command "jq" "jq"
check_command "docker" "docker"

echo ""

# ==================== 第二部分: 服务器连通性检查 ====================
print_header "2. 服务器连通性检查"

# 检查HTTP端口
((TOTAL_CHECKS++))
echo -n "检查HTTP端口 ($SERVER:$HTTP_PORT)... "
if curl -s --connect-timeout 5 "http://$SERVER:$HTTP_PORT/healthz" > /dev/null 2>&1; then
    print_success "HTTP端口可访问"
else
    print_failure "HTTP端口无法访问"
fi

# 检查健康检查接口
((TOTAL_CHECKS++))
echo -n "检查健康检查接口... "
HEALTH_RESPONSE=$(curl -s "http://$SERVER:$HTTP_PORT/healthz" 2>/dev/null || echo "ERROR")
if echo "$HEALTH_RESPONSE" | grep -q "healthy\|ok"; then
    print_success "服务健康状态: OK"
    echo "    响应: $HEALTH_RESPONSE"
else
    print_failure "服务健康状态: 异常"
    echo "    响应: $HEALTH_RESPONSE"
fi

# 检查就绪检查接口
((TOTAL_CHECKS++))
echo -n "检查就绪检查接口... "
READY_RESPONSE=$(curl -s "http://$SERVER:$HTTP_PORT/readyz" 2>/dev/null || echo "ERROR")
if echo "$READY_RESPONSE" | grep -q "ready\|ok"; then
    print_success "服务就绪状态: OK"
else
    print_failure "服务就绪状态: 异常"
fi

echo ""

# ==================== 第三部分: API认证检查 ====================
print_header "3. API认证检查"

((TOTAL_CHECKS++))
echo -n "测试API认证... "
API_RESPONSE=$(curl -s -w "\n%{http_code}" \
    -H "X-Api-Key: $API_KEY" \
    "http://$SERVER:$HTTP_PORT/api/v1/third/devices/$DEVICE1" 2>/dev/null)

HTTP_CODE=$(echo "$API_RESPONSE" | tail -1)
BODY=$(echo "$API_RESPONSE" | sed '$d')

if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "404" ]; then
    print_success "API认证通过 (HTTP $HTTP_CODE)"
else
    print_failure "API认证失败 (HTTP $HTTP_CODE)"
    echo "    响应: $BODY"
fi

echo ""

# ==================== 第四部分: 容器状态检查 ====================
print_header "4. 容器状态检查 (需要SSH访问)"

print_info "以下检查需要SSH访问权限，如果没有权限可以跳过"
echo ""

read -p "是否有SSH访问权限? (y/n): " HAS_SSH

if [ "$HAS_SSH" = "y" ] || [ "$HAS_SSH" = "Y" ]; then
    print_info "请手动在服务器上执行以下命令:"
    echo ""
    echo "  ssh user@$SERVER"
    echo "  docker ps | grep iot"
    echo ""
    echo "验证以下容器正在运行:"
    echo "  - iot-server-prod"
    echo "  - iot-postgres-prod"
    echo "  - iot-redis-prod"
    echo ""
else
    print_warning "跳过容器状态检查"
fi

# ==================== 第五部分: 设备在线状态检查 ====================
print_header "5. 设备在线状态检查 (需要数据库访问)"

print_info "以下检查需要数据库访问权限"
echo ""

read -p "是否有数据库访问权限? (y/n): " HAS_DB

if [ "$HAS_DB" = "y" ] || [ "$HAS_DB" = "Y" ]; then
    print_info "请手动在服务器上执行以下命令检查设备状态:"
    echo ""
    echo "  docker exec -it iot-postgres-prod psql -U iot -d iot_server -c \\"
    echo "  \"SELECT phy_id, "
    echo "      CASE WHEN last_seen_at > NOW() - INTERVAL '30 seconds' "
    echo "           THEN '在线' ELSE '离线' END AS status, "
    echo "      last_seen_at "
    echo "   FROM devices "
    echo "   WHERE phy_id IN ('$DEVICE1', '$DEVICE2');\""
    echo ""
    echo "预期结果: 两台设备状态为'在线'"
    echo ""
else
    print_warning "跳过设备在线状态检查"
fi

# ==================== 第六部分: Redis连接检查 ====================
print_header "6. Redis连接检查 (需要服务器访问)"

if [ "$HAS_SSH" = "y" ] || [ "$HAS_SSH" = "Y" ]; then
    print_info "请手动在服务器上执行以下命令检查Redis:"
    echo ""
    echo "  docker exec -it iot-redis-prod redis-cli -a 123456 PING"
    echo ""
    echo "预期响应: PONG"
    echo ""
    
    echo "  docker exec -it iot-redis-prod redis-cli -a 123456 KEYS \"session:*\""
    echo ""
    echo "检查是否有会话数据"
    echo ""
else
    print_warning "跳过Redis连接检查"
fi

# ==================== 第七部分: Webhook配置检查 ====================
print_header "7. Webhook配置检查"

read -p "是否已配置第三方Webhook接收端? (y/n): " HAS_WEBHOOK

if [ "$HAS_WEBHOOK" = "y" ] || [ "$HAS_WEBHOOK" = "Y" ]; then
    read -p "请输入Webhook URL: " WEBHOOK_URL
    
    if [ -n "$WEBHOOK_URL" ]; then
        ((TOTAL_CHECKS++))
        echo -n "测试Webhook连通性... "
        
        WEBHOOK_TEST=$(curl -s -w "\n%{http_code}" -X POST \
            -H "Content-Type: application/json" \
            -d '{"test": true}' \
            "$WEBHOOK_URL" 2>/dev/null || echo "ERROR\n000")
        
        WEBHOOK_CODE=$(echo "$WEBHOOK_TEST" | tail -1)
        
        if [ "$WEBHOOK_CODE" = "200" ] || [ "$WEBHOOK_CODE" = "201" ]; then
            print_success "Webhook端点可访问"
        else
            print_failure "Webhook端点无法访问 (HTTP $WEBHOOK_CODE)"
        fi
    fi
else
    print_warning "Webhook未配置，将无法测试事件推送功能"
fi

echo ""

# ==================== 第八部分: 测试数据准备检查 ====================
print_header "8. 测试数据准备"

read -p "是否准备好充电插头进行物理测试? (y/n): " HAS_CHARGER

if [ "$HAS_CHARGER" = "y" ] || [ "$HAS_CHARGER" = "Y" ]; then
    print_success "充电插头已准备"
else
    print_warning "充电插头未准备，部分测试将无法执行"
fi

echo ""

# ==================== 总结 ====================
print_header "检查总结"

echo ""
echo "总检查项: $TOTAL_CHECKS"
echo -e "${GREEN}通过: $PASSED_CHECKS${NC}"
echo -e "${RED}失败: $FAILED_CHECKS${NC}"
echo ""

SUCCESS_RATE=$((PASSED_CHECKS * 100 / TOTAL_CHECKS))

if [ $SUCCESS_RATE -ge 80 ]; then
    echo -e "${GREEN}✓ 环境检查通过率: ${SUCCESS_RATE}%${NC}"
    echo -e "${GREEN}✓ 可以继续执行测试${NC}"
    exit 0
elif [ $SUCCESS_RATE -ge 60 ]; then
    echo -e "${YELLOW}⚠ 环境检查通过率: ${SUCCESS_RATE}%${NC}"
    echo -e "${YELLOW}⚠ 建议修复失败项后再继续${NC}"
    exit 1
else
    echo -e "${RED}✗ 环境检查通过率: ${SUCCESS_RATE}%${NC}"
    echo -e "${RED}✗ 请修复失败项后重新检查${NC}"
    exit 1
fi

