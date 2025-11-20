#!/bin/bash
# 停止本地调试环境

set -e

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}======================================${NC}"
echo -e "${BLUE}   停止本地调试环境${NC}"
echo -e "${BLUE}======================================${NC}"
echo ""

# 停止容器
echo -e "${YELLOW}→ 停止 PostgreSQL 和 Redis 容器...${NC}"
docker-compose -f docker-compose.debug.yml down

echo ""
echo -e "${GREEN}✓ 调试环境已停止${NC}"
echo ""
echo -e "${YELLOW}注意: 数据已保留在 Docker volumes 中${NC}"
echo -e "如需完全清理数据,请运行:"
echo -e "  ${BLUE}docker-compose -f docker-compose.debug.yml down -v${NC}"
echo ""
