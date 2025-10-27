#!/bin/bash

# 等待服务重启完成脚本
# 功能: 持续检查服务是否重启完成并可用
# 使用: ./wait_for_restart.sh

SCRIPT_DIR=$(dirname "$0")
source "$SCRIPT_DIR/helper_functions.sh"

print_header "等待服务重启完成"

echo "服务器: $TEST_SERVER:$TEST_HTTP_PORT"
echo ""

MAX_WAIT=300  # 最长等待5分钟
WAIT_INTERVAL=5  # 每5秒检查一次
elapsed=0

print_info "开始监控服务状态..."
echo ""

while [ $elapsed -lt $MAX_WAIT ]; do
    # 检查健康检查端点
    health_code=$(curl -s -o /dev/null -w "%{http_code}" "http://$TEST_SERVER:$TEST_HTTP_PORT/healthz" 2>/dev/null)
    
    # 检查API端点
    api_code=$(curl -s -o /dev/null -w "%{http_code}" \
        -H "X-Api-Key: $TEST_API_KEY" \
        "http://$TEST_SERVER:$TEST_HTTP_PORT/api/v1/third/devices/test" 2>/dev/null)
    
    timestamp=$(date '+%H:%M:%S')
    
    if [ "$health_code" = "200" ] && [ "$api_code" != "000" ] && [ "$api_code" != "404" ]; then
        echo -ne "\r[$timestamp] 健康检查: ✓ HTTP $health_code | API端点: ✓ HTTP $api_code"
        echo ""
        echo ""
        print_success "服务已启动并就绪！"
        echo ""
        
        # 运行快速诊断确认
        print_info "运行快速诊断..."
        echo ""
        bash "$SCRIPT_DIR/quick_diagnosis.sh"
        exit 0
    else
        echo -ne "\r[$timestamp] 等待服务启动... (健康: $health_code, API: $api_code) - ${elapsed}s/${MAX_WAIT}s"
        sleep $WAIT_INTERVAL
        elapsed=$((elapsed + WAIT_INTERVAL))
    fi
done

echo ""
echo ""
print_failure "等待超时！服务在 $MAX_WAIT 秒内未能启动"
echo ""
print_info "建议检查:"
echo "  1. 服务器上容器是否正在运行: docker ps | grep iot-server"
echo "  2. 查看容器日志: docker logs iot-server-prod --tail 50"
echo "  3. 检查端口映射: docker port iot-server-prod"
echo "  4. 检查环境变量: docker exec iot-server-prod printenv"
echo ""

exit 1

