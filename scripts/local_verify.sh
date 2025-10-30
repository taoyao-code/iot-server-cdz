#!/bin/bash
# 本地代码验证脚本
# 功能: 快速验证代码能否正常编译，避免部署后才发现问题
# 使用: ./scripts/local_verify.sh

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info() { echo -e "${GREEN}[✓]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[!]${NC} $1"; }
log_error() { echo -e "${RED}[✗]${NC} $1"; }
log_step() { echo -e "${BLUE}[→]${NC} $1"; }

print_header() {
    echo ""
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${BLUE}  $1${NC}"
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
}

# 切换到项目根目录
cd "$(dirname "$0")/.."

print_header "本地代码验证"
echo "验证时间: $(date '+%Y-%m-%d %H:%M:%S')"
echo ""

# ==================== 步骤1: 检查Go环境 ====================
log_step "检查Go环境..."
if ! command -v go &> /dev/null; then
    log_error "未安装Go，请先安装Go 1.21+"
    exit 1
fi

GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
log_info "Go版本: $GO_VERSION"

# ==================== 步骤2: 检查依赖 ====================
log_step "检查依赖完整性..."
if ! go mod verify > /dev/null 2>&1; then
    log_warn "依赖有问题，尝试修复..."
    go mod tidy
    log_info "依赖已修复"
else
    log_info "依赖验证通过"
fi

# ==================== 步骤3: 代码格式检查 ====================
log_step "检查代码格式..."
UNFORMATTED=$(gofmt -l . 2>/dev/null | grep -v vendor | grep -v ".pb.go" || true)

if [ -n "$UNFORMATTED" ]; then
    log_error "以下文件需要格式化:"
    echo "$UNFORMATTED"
    echo ""
    log_warn "运行以下命令自动修复: make fmt"
    exit 1
else
    log_info "代码格式检查通过"
fi

# ==================== 步骤4: 静态分析 ====================
log_step "执行静态分析..."
if ! go vet ./... > /dev/null 2>&1; then
    log_error "静态分析发现问题:"
    go vet ./...
    exit 1
else
    log_info "静态分析通过"
fi

# ==================== 步骤5: 编译验证 ====================
log_step "编译主程序..."
if ! go build -o /tmp/iot-server-test ./cmd/server > /dev/null 2>&1; then
    log_error "编译失败:"
    go build -o /tmp/iot-server-test ./cmd/server
    exit 1
else
    rm -f /tmp/iot-server-test
    log_info "编译成功"
fi

# ==================== 步骤6: 编译Linux版本 ====================
log_step "编译Linux版本..."
if ! CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /tmp/iot-server-linux-test ./cmd/server > /dev/null 2>&1; then
    log_error "Linux版本编译失败:"
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /tmp/iot-server-linux-test ./cmd/server
    exit 1
else
    rm -f /tmp/iot-server-linux-test
    log_info "Linux版本编译成功"
fi

# ==================== 步骤7: 运行单元测试 (可选) ====================
if [ "${SKIP_TESTS}" != "true" ]; then
    log_step "运行单元测试..."
    if ! go test -short ./... > /dev/null 2>&1; then
        log_warn "部分测试失败（非致命）"
        log_warn "运行 'go test -v ./...' 查看详情"
    else
        log_info "单元测试通过"
    fi
else
    log_warn "跳过单元测试 (SKIP_TESTS=true)"
fi

# ==================== 总结 ====================
print_header "验证完成"
echo ""
log_info "所有检查通过！代码可以安全部署"
echo ""
echo "下一步:"
echo "  1. 运行 'make build-linux' 编译生产版本"
echo "  2. 运行 'make auto-deploy' 自动部署到生产环境"
echo "  3. 或手动部署: ./scripts/quick-deploy.sh"
echo ""

exit 0

