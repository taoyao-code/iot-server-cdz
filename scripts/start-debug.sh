#!/bin/bash
# 本地调试环境启动脚本
# 功能:启动 PostgreSQL 和 Redis 容器用于本地开发调试

set -e

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}======================================${NC}"
echo -e "${BLUE}   IOT Server 本地调试环境启动${NC}"
echo -e "${BLUE}======================================${NC}"
echo ""

# 检查 Docker 是否运行
if ! docker info > /dev/null 2>&1; then
    echo -e "${RED}✗ Docker 未运行,请先启动 Docker${NC}"
    exit 1
fi

# 停止并删除现有的调试容器(如果存在)
echo -e "${YELLOW}→ 清理现有调试容器...${NC}"
docker-compose -f docker-compose.debug.yml down 2>/dev/null || true

# 启动调试环境
echo -e "${YELLOW}→ 启动 PostgreSQL 和 Redis 容器...${NC}"
docker-compose -f docker-compose.debug.yml --env-file .env.debug up -d

# 等待服务健康检查
echo -e "${YELLOW}→ 等待服务就绪...${NC}"
sleep 5

# 检查容器状态
POSTGRES_STATUS=$(docker inspect -f '{{.State.Health.Status}}' iot-postgres-debug 2>/dev/null || echo "unknown")
REDIS_STATUS=$(docker inspect -f '{{.State.Health.Status}}' iot-redis-debug 2>/dev/null || echo "unknown")

echo ""
echo -e "${GREEN}✓ 调试环境启动完成!${NC}"
echo ""
echo -e "${BLUE}服务状态:${NC}"
echo -e "  PostgreSQL: ${POSTGRES_STATUS} (localhost:5432)"
echo -e "  Redis: ${REDIS_STATUS} (localhost:6379)"
echo ""

if [ "$POSTGRES_STATUS" != "healthy" ] || [ "$REDIS_STATUS" != "healthy" ]; then
    echo -e "${YELLOW}⚠ 某些服务可能尚未就绪,请稍等片刻...${NC}"
    echo -e "${YELLOW}  可以使用 'docker-compose -f docker-compose.debug.yml logs' 查看日志${NC}"
    echo ""
fi

echo -e "${GREEN}下一步:${NC}"
echo -e "  1. 加载环境变量:"
echo -e "     ${BLUE}source .env.debug${NC}"
echo ""
echo -e "  2. 启动 IOT Server (选择其中一种方式):"
echo -e "     ${BLUE}IOT_CONFIG=configs/local.yaml go run cmd/server/main.go${NC}"
echo -e "     或在 IDE 中设置环境变量 IOT_CONFIG=configs/local.yaml 后启动调试"
echo ""
echo -e "  3. 停止调试环境:"
echo -e "     ${BLUE}./scripts/stop-debug.sh${NC}"
echo ""
echo -e "${BLUE}======================================${NC}"
