#!/bin/bash
# 项目清理脚本 - 删除无关和临时文件

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$PROJECT_ROOT"

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 统计计数
CLEANED_COUNT=0

log_info() {
    echo -e "${BLUE}ℹ ${NC}$1"
}

log_success() {
    echo -e "${GREEN}✅ ${NC}$1"
}

log_warning() {
    echo -e "${YELLOW}⚠️  ${NC}$1"
}

echo "=================================================="
echo "项目清理工具"
echo "=================================================="
echo "项目路径: $PROJECT_ROOT"
echo ""

# 1. 删除 macOS 系统文件
log_info "清理 macOS 系统文件 (.DS_Store)..."
DS_COUNT=$(find . -name ".DS_Store" -type f | wc -l | tr -d ' ')
if [ "$DS_COUNT" -gt 0 ]; then
    find . -name ".DS_Store" -type f -delete
    log_success "删除 $DS_COUNT 个 .DS_Store 文件"
    CLEANED_COUNT=$((CLEANED_COUNT + DS_COUNT))
else
    log_info "未发现 .DS_Store 文件"
fi

# 2. 删除测试覆盖率文件
log_info "清理测试覆盖率文件..."
if [ -f "coverage.out" ]; then
    rm -f coverage.out
    log_success "删除 coverage.out"
    CLEANED_COUNT=$((CLEANED_COUNT + 1))
fi
if [ -f "coverage.html" ]; then
    rm -f coverage.html
    log_success "删除 coverage.html"
    CLEANED_COUNT=$((CLEANED_COUNT + 1))
fi

# 3. 删除编译临时文件
log_info "清理编译临时文件..."
TEMP_COUNT=$(find . \( -name "*.tmp" -o -name "*.bak" -o -name "*.swp" -o -name "*.swo" -o -name "*~" \) -type f | wc -l | tr -d ' ')
if [ "$TEMP_COUNT" -gt 0 ]; then
    find . \( -name "*.tmp" -o -name "*.bak" -o -name "*.swp" -o -name "*.swo" -o -name "*~" \) -type f -delete
    log_success "删除 $TEMP_COUNT 个临时文件"
    CLEANED_COUNT=$((CLEANED_COUNT + TEMP_COUNT))
else
    log_info "未发现临时文件"
fi

# 4. 删除空的日志目录
log_info "清理空的日志目录..."
if [ -d "test/logs" ] && [ -z "$(ls -A test/logs)" ]; then
    rmdir test/logs
    log_success "删除空目录: test/logs"
    CLEANED_COUNT=$((CLEANED_COUNT + 1))
fi

# 5. 清理临时构建目录
log_info "清理临时构建目录..."
if [ -d "tmp" ]; then
    rm -rf tmp
    log_success "删除 tmp 目录"
    CLEANED_COUNT=$((CLEANED_COUNT + 1))
fi

# 6. 清理 Go 测试缓存（可选）
if [ "$1" == "--deep" ]; then
    log_info "深度清理: Go 测试缓存..."
    go clean -testcache
    log_success "清理 Go 测试缓存"
fi

# 7. 更新 .gitignore（如果需要）
log_info "检查 .gitignore..."
if ! grep -q ".DS_Store" .gitignore 2>/dev/null; then
    cat >> .gitignore << 'EOF'

# macOS
.DS_Store
.AppleDouble
.LSOverride

# 测试覆盖率
coverage.out
coverage.html

# 临时文件
*.tmp
*.bak
*.swp
*.swo
*~

# 构建产物
/tmp/
/bin/*
!bin/.gitkeep
EOF
    log_success "更新 .gitignore"
fi

echo ""
echo "=================================================="
echo "清理完成"
echo "=================================================="
echo -e "${GREEN}总计清理: $CLEANED_COUNT 个文件/目录${NC}"
echo ""
log_info "提示: 使用 --deep 参数进行深度清理（包括Go缓存）"
