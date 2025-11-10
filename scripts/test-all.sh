#!/bin/bash
# 完整测试脚本 - 验证所有功能模块
# 用法: ./scripts/test-all.sh [--verbose]

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

# 配置
VERBOSE=false
if [[ "$1" == "--verbose" || "$1" == "-v" ]]; then
    VERBOSE=true
fi

# 日志函数
log_info() {
    echo -e "${BLUE}ℹ ${NC}$1"
}

log_success() {
    echo -e "${GREEN}✅ ${NC}$1"
}

log_warning() {
    echo -e "${YELLOW}⚠️  ${NC}$1"
}

log_error() {
    echo -e "${RED}❌ ${NC}$1"
}

# 测试计数器
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0
SKIPPED_TESTS=0

# 运行测试函数
run_test() {
    local test_name="$1"
    local test_cmd="$2"
    local required="${3:-true}"

    TOTAL_TESTS=$((TOTAL_TESTS + 1))

    echo ""
    log_info "运行测试: $test_name"

    if $VERBOSE; then
        if eval "$test_cmd"; then
            log_success "$test_name - 通过"
            PASSED_TESTS=$((PASSED_TESTS + 1))
            return 0
        else
            if [[ "$required" == "true" ]]; then
                log_error "$test_name - 失败"
                FAILED_TESTS=$((FAILED_TESTS + 1))
                return 1
            else
                log_warning "$test_name - 失败（非必需）"
                SKIPPED_TESTS=$((SKIPPED_TESTS + 1))
                return 0
            fi
        fi
    else
        if eval "$test_cmd > /dev/null 2>&1"; then
            log_success "$test_name - 通过"
            PASSED_TESTS=$((PASSED_TESTS + 1))
            return 0
        else
            if [[ "$required" == "true" ]]; then
                log_error "$test_name - 失败"
                FAILED_TESTS=$((FAILED_TESTS + 1))
                # 显示错误输出
                echo "执行命令: $test_cmd"
                eval "$test_cmd"
                return 1
            else
                log_warning "$test_name - 失败（非必需）"
                SKIPPED_TESTS=$((SKIPPED_TESTS + 1))
                return 0
            fi
        fi
    fi
}

# 打印测试报告
print_report() {
    echo ""
    echo "=================================================="
    echo "测试报告"
    echo "=================================================="
    echo "总测试数: $TOTAL_TESTS"
    echo -e "${GREEN}通过: $PASSED_TESTS${NC}"
    echo -e "${RED}失败: $FAILED_TESTS${NC}"
    echo -e "${YELLOW}跳过: $SKIPPED_TESTS${NC}"
    echo "=================================================="

    if [ $FAILED_TESTS -eq 0 ]; then
        echo -e "${GREEN}✅ 所有测试通过！${NC}"
        return 0
    else
        echo -e "${RED}❌ 有 $FAILED_TESTS 个测试失败${NC}"
        return 1
    fi
}

# 主测试流程
main() {
    echo "=================================================="
    echo "IOT Server 完整测试套件"
    echo "=================================================="
    echo "项目路径: $PROJECT_ROOT"
    echo "详细模式: $VERBOSE"
    echo ""

    # 1. 代码质量检查
    log_info "阶段 1: 代码质量检查"
    run_test "代码格式检查" "test -z \"\$(gofmt -s -l .)\"" true
    run_test "Go vet静态分析" "go vet ./..." true
    run_test "Go mod验证" "go mod verify" true

    # 2. 编译测试
    log_info "阶段 2: 编译测试"
    run_test "项目编译" "go build -o /tmp/iot-server-test ./cmd/server" true

    # 3. 单元测试
    log_info "阶段 3: 单元测试"
    run_test "internal/app 包测试" "go test -race ./internal/app/..." true
    run_test "internal/service 包测试" "go test -race ./internal/service/..." true
    run_test "internal/api 包测试" "go test ./internal/api/..." true
    run_test "internal/protocol 包测试" "go test ./internal/protocol/..." true
    run_test "internal/outbound 包测试" "go test ./internal/outbound/..." true
    run_test "internal/storage 包测试" "go test ./internal/storage/..." false

    # 4. P1问题修复验证
    log_info "阶段 4: P1问题修复验证"
    run_test "P1-1: 心跳超时60秒" "go test -run TestSessionTimeout ./internal/app/" true
    run_test "P1-2: 延迟ACK拒绝" "go test -run TestHandleOrderConfirmation ./internal/service/" true
    run_test "P1-4: 端口状态同步" "go test -run TestPortStatusSyncer ./internal/app/" true

    # 5. 集成测试（可选）
    log_info "阶段 5: 集成测试（可选）"
    run_test "配置文件加载测试" "go test -run TestConfig ./internal/config/..." false

    # 6. 覆盖率测试
    log_info "阶段 6: 测试覆盖率"
    if run_test "生成覆盖率报告" "go test -coverprofile=/tmp/coverage.out ./..." false; then
        coverage=$(go tool cover -func=/tmp/coverage.out | grep total | awk '{print $3}')
        log_info "总体覆盖率: $coverage"
    fi

    # 打印报告
    print_report
}

# 运行主函数
main
exit_code=$?

# 清理临时文件
rm -f /tmp/iot-server-test
rm -f /tmp/coverage.out

exit $exit_code
