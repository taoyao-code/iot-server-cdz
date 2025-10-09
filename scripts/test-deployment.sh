#!/bin/bash
# 部署测试脚本 - 用于验证部署配置的正确性
set -e

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${BLUE}╔════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║   IOT Server 部署配置测试工具         ║${NC}"
echo -e "${BLUE}╔════════════════════════════════════════╗${NC}"
echo ""

# 测试计数器
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0

# 测试函数
test_case() {
    local name="$1"
    local command="$2"
    
    TOTAL_TESTS=$((TOTAL_TESTS + 1))
    echo -n "测试 $TOTAL_TESTS: $name ... "
    
    if eval "$command" > /dev/null 2>&1; then
        echo -e "${GREEN}✓ 通过${NC}"
        PASSED_TESTS=$((PASSED_TESTS + 1))
        return 0
    else
        echo -e "${RED}✗ 失败${NC}"
        FAILED_TESTS=$((FAILED_TESTS + 1))
        return 1
    fi
}

test_case_with_output() {
    local name="$1"
    local command="$2"
    local expected="$3"
    
    TOTAL_TESTS=$((TOTAL_TESTS + 1))
    echo -n "测试 $TOTAL_TESTS: $name ... "
    
    local output=$(eval "$command" 2>&1)
    
    if echo "$output" | grep -q "$expected"; then
        echo -e "${GREEN}✓ 通过${NC}"
        PASSED_TESTS=$((PASSED_TESTS + 1))
        return 0
    else
        echo -e "${RED}✗ 失败${NC}"
        echo "  期望: $expected"
        echo "  实际: $output"
        FAILED_TESTS=$((FAILED_TESTS + 1))
        return 1
    fi
}

echo -e "${BLUE}[1] 检查必要工具${NC}"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
test_case "Docker 已安装" "command -v docker"
test_case "Docker 服务运行中" "docker info"
test_case "Docker Compose 已安装" "command -v docker-compose || docker compose version"
test_case "Git 已安装" "command -v git"
test_case "Curl 已安装" "command -v curl"
echo ""

echo -e "${BLUE}[2] 检查文件完整性${NC}"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
test_case "Dockerfile 存在" "test -f Dockerfile"
test_case "docker-compose.yml 存在" "test -f docker-compose.yml"
test_case ".env.example 存在" "test -f .env.example"
test_case "deploy.sh 存在" "test -f scripts/deploy.sh"
test_case "deploy.sh 可执行" "test -x scripts/deploy.sh"
test_case "production.yaml 存在" "test -f configs/production.yaml"
test_case "bkv_reason_map.yaml 存在" "test -f configs/bkv_reason_map.yaml"
test_case "数据库迁移文件存在" "test -d db/migrations && ls db/migrations/*.sql"
echo ""

echo -e "${BLUE}[3] 检查 Dockerfile 配置${NC}"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
test_case_with_output "使用 Go 1.24" "grep 'FROM golang' Dockerfile | head -1" "golang:1.24"
test_case_with_output "配置了 Alpine 镜像源" "grep 'mirrors.aliyun.com' Dockerfile" "mirrors.aliyun.com"
test_case_with_output "配置了 Go 代理" "grep 'GOPROXY' Dockerfile" "goproxy.cn"
test_case_with_output "使用 Debian 运行镜像" "grep 'FROM debian' Dockerfile" "debian:12-slim"
test_case_with_output "配置了健康检查" "grep 'HEALTHCHECK' Dockerfile" "HEALTHCHECK"
echo ""

echo -e "${BLUE}[4] 检查 .env.example 配置${NC}"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
test_case_with_output ".env.example 包含 POSTGRES_PASSWORD" "cat .env.example" "POSTGRES_PASSWORD"
test_case_with_output ".env.example 包含 REDIS_PASSWORD" "cat .env.example" "REDIS_PASSWORD"
test_case_with_output ".env.example 包含 API_KEY" "cat .env.example" "API_KEY"
test_case_with_output ".env.example 包含 THIRDPARTY_API_KEY" "cat .env.example" "THIRDPARTY_API_KEY"
test_case_with_output ".env.example 包含安全提示" "cat .env.example" "CHANGE_ME"
echo ""

echo -e "${BLUE}[5] 检查 docker-compose.yml 配置${NC}"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
test_case_with_output "配置了 Redis 服务" "cat docker-compose.yml" "redis:"
test_case_with_output "配置了 PostgreSQL 服务" "cat docker-compose.yml" "postgres:"
test_case_with_output "配置了 IOT Server 服务" "cat docker-compose.yml" "iot-server:"
test_case_with_output "配置了健康检查" "cat docker-compose.yml" "healthcheck:"
test_case_with_output "配置了资源限制" "cat docker-compose.yml" "resources:"
echo ""

echo -e "${BLUE}[6] 检查端口配置${NC}"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo -n "测试端口 7054 (TCP协议-BKV) 未被占用 ... "
if ! netstat -tuln 2>/dev/null | grep -q ':7054' && ! lsof -i :7054 2>/dev/null; then
    echo -e "${GREEN}✓ 通过${NC}"
    PASSED_TESTS=$((PASSED_TESTS + 1))
else
    echo -e "${YELLOW}⚠ 端口已被占用${NC}"
fi
TOTAL_TESTS=$((TOTAL_TESTS + 1))

echo -n "测试端口 7055 (HTTP API) 未被占用 ... "
if ! netstat -tuln 2>/dev/null | grep -q ':7055' && ! lsof -i :7055 2>/dev/null; then
    echo -e "${GREEN}✓ 通过${NC}"
    PASSED_TESTS=$((PASSED_TESTS + 1))
else
    echo -e "${YELLOW}⚠ 端口已被占用${NC}"
fi
TOTAL_TESTS=$((TOTAL_TESTS + 1))
echo ""

echo -e "${BLUE}[7] 检查网络连通性${NC}"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo -n "测试连接 Docker Hub ... "
if curl -s --connect-timeout 5 --max-time 10 https://registry.hub.docker.com > /dev/null 2>&1; then
    echo -e "${GREEN}✓ 通过${NC}"
    PASSED_TESTS=$((PASSED_TESTS + 1))
else
    echo -e "${YELLOW}⚠ 连接失败 (可能影响镜像拉取)${NC}"
fi
TOTAL_TESTS=$((TOTAL_TESTS + 1))

echo -n "测试连接阿里云镜像源 ... "
if curl -s --connect-timeout 5 --max-time 10 http://mirrors.aliyun.com > /dev/null 2>&1; then
    echo -e "${GREEN}✓ 通过${NC}"
    PASSED_TESTS=$((PASSED_TESTS + 1))
else
    echo -e "${YELLOW}⚠ 连接失败${NC}"
fi
TOTAL_TESTS=$((TOTAL_TESTS + 1))

echo -n "测试连接 Go 代理 ... "
if curl -s --connect-timeout 5 --max-time 10 https://goproxy.cn > /dev/null 2>&1; then
    echo -e "${GREEN}✓ 通过${NC}"
    PASSED_TESTS=$((PASSED_TESTS + 1))
else
    echo -e "${YELLOW}⚠ 连接失败 (可能影响依赖下载)${NC}"
fi
TOTAL_TESTS=$((TOTAL_TESTS + 1))
echo ""

echo -e "${BLUE}[8] 检查磁盘空间${NC}"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
# macOS和Linux兼容的磁盘空间检查
if [[ "$OSTYPE" == "darwin"* ]]; then
    # macOS
    AVAILABLE=$(df -g . | tail -1 | awk '{print $4}')
else
    # Linux
    AVAILABLE=$(df -BG . | tail -1 | awk '{print $4}' | sed 's/G//')
fi
echo -n "测试磁盘空间 (需要 >=5GB, 当前 ${AVAILABLE}GB) ... "
if [ ! -z "$AVAILABLE" ] && [ "$AVAILABLE" -ge 5 ]; then
    echo -e "${GREEN}✓ 通过${NC}"
    PASSED_TESTS=$((PASSED_TESTS + 1))
elif [ -z "$AVAILABLE" ]; then
    echo -e "${YELLOW}⚠ 无法检测磁盘空间${NC}"
    PASSED_TESTS=$((PASSED_TESTS + 1))
else
    echo -e "${RED}✗ 空间不足${NC}"
    FAILED_TESTS=$((FAILED_TESTS + 1))
fi
TOTAL_TESTS=$((TOTAL_TESTS + 1))
echo ""

echo -e "${BLUE}[9] 检查 Go 模块${NC}"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
test_case "go.mod 存在" "test -f go.mod"
test_case "go.sum 存在" "test -f go.sum"
test_case_with_output "Go 版本为 1.24" "cat go.mod | head -3" "1.24"
echo ""

# 总结
echo -e "${BLUE}╔════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║           测试结果总结                 ║${NC}"
echo -e "${BLUE}╚════════════════════════════════════════╝${NC}"
echo ""
echo "总测试数: $TOTAL_TESTS"
echo -e "${GREEN}通过: $PASSED_TESTS${NC}"
if [ $FAILED_TESTS -gt 0 ]; then
    echo -e "${RED}失败: $FAILED_TESTS${NC}"
fi
echo ""

if [ $FAILED_TESTS -eq 0 ]; then
    echo -e "${GREEN}✓ 所有测试通过！部署配置正确。${NC}"
    echo -e "${GREEN}可以安全执行: make deploy${NC}"
    exit 0
else
    echo -e "${RED}✗ 有 $FAILED_TESTS 项测试失败${NC}"
    echo -e "${YELLOW}请检查失败项后再执行部署${NC}"
    exit 1
fi

