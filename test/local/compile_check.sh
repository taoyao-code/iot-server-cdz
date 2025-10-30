#!/bin/bash
# 本地编译验证脚本
# 目的：确保代码无语法错误、能正常编译
# 使用：./compile_check.sh

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

print_success() { echo -e "${GREEN}✓${NC} $1"; }
print_failure() { echo -e "${RED}✗${NC} $1"; }
print_info() { echo -e "${BLUE}→${NC} $1"; }
print_warning() { echo -e "${YELLOW}⚠${NC} $1"; }

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  本地编译验证"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# 进入项目根目录
SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
PROJECT_ROOT=$(cd "$SCRIPT_DIR/../.." && pwd)
cd "$PROJECT_ROOT"

print_info "项目目录: $PROJECT_ROOT"
echo ""

# 步骤1: 检查Go环境
print_info "[1/5] 检查Go环境..."
if ! command -v go &> /dev/null; then
    print_failure "Go未安装"
    echo "请先安装Go: https://golang.org/dl/"
    exit 1
fi

GO_VERSION=$(go version)
print_success "Go环境正常: $GO_VERSION"
echo ""

# 步骤2: 整理依赖
print_info "[2/5] 整理Go依赖..."
if go mod tidy; then
    print_success "依赖整理完成"
else
    print_failure "依赖整理失败"
    exit 1
fi
echo ""

# 步骤3: 代码格式检查
print_info "[3/5] 检查代码格式..."
UNFORMATTED=$(gofmt -l . 2>/dev/null | grep -v vendor || true)
if [ -z "$UNFORMATTED" ]; then
    print_success "代码格式正确"
else
    print_warning "以下文件需要格式化:"
    echo "$UNFORMATTED"
    echo ""
    echo "运行以下命令修复:"
    echo "  make fmt"
    echo ""
fi
echo ""

# 步骤4: 静态分析
print_info "[4/5] 运行静态分析..."
if go vet ./... 2>&1; then
    print_success "静态分析通过"
else
    print_failure "静态分析发现问题"
    echo ""
    echo "请修复上述问题后重试"
    exit 1
fi
echo ""

# 步骤5: 编译检查
print_info "[5/5] 编译检查..."
if go build -o /dev/null ./cmd/server 2>&1; then
    print_success "编译成功"
else
    print_failure "编译失败"
    echo ""
    echo "请修复编译错误后重试"
    exit 1
fi
echo ""

# 完成
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo -e "${GREEN}✓ 本地验证通过${NC}"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "下一步："
echo "  - 部署到生产: make auto-deploy"
echo "  - 查看帮助: make help"
echo ""

exit 0

