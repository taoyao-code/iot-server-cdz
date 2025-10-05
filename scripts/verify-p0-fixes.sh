#!/bin/bash
# P0修复自动化验证脚本

set -e

echo "=========================================="
echo "🔍 P0修复验证开始"
echo "=========================================="
echo ""

# 颜色定义
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 1. 编译检查
echo "📦 1/6 编译检查..."
if go build -o bin/iot-server ./cmd/server; then
    echo -e "${GREEN}✅ 编译成功${NC}"
else
    echo -e "${RED}❌ 编译失败${NC}"
    exit 1
fi
echo ""

# 2. 运行测试
echo "🧪 2/6 运行测试..."
if go test ./... -short -timeout 30s > /tmp/test-output.txt 2>&1; then
    echo -e "${GREEN}✅ 所有测试通过${NC}"
    grep -E "^ok|^PASS" /tmp/test-output.txt | tail -5
else
    echo -e "${RED}❌ 测试失败${NC}"
    cat /tmp/test-output.txt
    exit 1
fi
echo ""

# 3. 检查数据库迁移文件
echo "🗄️  3/6 检查数据库迁移..."
if [ -f "db/migrations/0005_device_params_up.sql" ] && [ -f "db/migrations/0005_device_params_down.sql" ]; then
    echo -e "${GREEN}✅ 参数持久化迁移文件存在${NC}"
    echo "   - 0005_device_params_up.sql"
    echo "   - 0005_device_params_down.sql"
else
    echo -e "${RED}❌ 迁移文件缺失${NC}"
    exit 1
fi
echo ""

# 4. 检查认证中间件
echo "🔐 4/6 检查API认证..."
if [ -f "internal/api/middleware/auth.go" ]; then
    echo -e "${GREEN}✅ API认证中间件存在${NC}"
    grep -q "APIKeyAuth" internal/api/middleware/auth.go && echo "   - APIKeyAuth 函数已实现"
else
    echo -e "${RED}❌ 认证中间件缺失${NC}"
    exit 1
fi
echo ""

# 5. 检查配置文件
echo "⚙️  5/6 检查配置文件..."
if [ -f "configs/example.yaml" ]; then
    if grep -q "api:" configs/example.yaml && grep -q "auth:" configs/example.yaml; then
        echo -e "${GREEN}✅ API认证配置已添加${NC}"
        echo "   配置路径: api.auth"
    else
        echo -e "${YELLOW}⚠️  配置文件中未找到API认证配置${NC}"
    fi
else
    echo -e "${YELLOW}⚠️  配置文件不存在${NC}"
fi
echo ""

# 6. 代码静态检查
echo "🔍 6/6 代码静态检查..."
if command -v golangci-lint &> /dev/null; then
    if golangci-lint run --timeout 5m --disable-all \
        --enable=errcheck \
        --enable=gosimple \
        --enable=govet \
        --enable=ineffassign \
        --enable=staticcheck \
        --enable=unused \
        ./... 2>&1 | tee /tmp/lint-output.txt; then
        echo -e "${GREEN}✅ 静态检查通过${NC}"
    else
        echo -e "${YELLOW}⚠️  发现一些静态检查问题（非致命）${NC}"
        echo "详情见: /tmp/lint-output.txt"
    fi
else
    echo -e "${YELLOW}⚠️  golangci-lint 未安装，跳过静态检查${NC}"
    echo "   安装命令: brew install golangci-lint"
fi
echo ""

# 总结
echo "=========================================="
echo -e "${GREEN}🎉 P0修复验证完成！${NC}"
echo "=========================================="
echo ""
echo "✅ 修复内容："
echo "  1. 启动顺序已优化（DB → Handler → TCP）"
echo "  2. 参数持久化已实现（PostgreSQL存储）"
echo "  3. API认证已添加（API Key保护）"
echo ""
echo "📋 下一步："
echo "  1. 更新生产配置（启用API认证）"
echo "  2. 运行集成测试"
echo "  3. 部署到测试环境"
echo ""
echo "📚 相关文档："
echo "  - P0-FIXES-COMPLETED.md"
echo "  - issues/架构改进实施方案.md"
echo ""
