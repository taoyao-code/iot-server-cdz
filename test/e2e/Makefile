.PHONY: help test test-verbose test-charge test-device test-order clean

# 默认目标
help:
	@echo "IoT E2E 测试 Makefile"
	@echo ""
	@echo "可用命令:"
	@echo "  make test          - 运行所有测试"
	@echo "  make test-verbose  - 运行所有测试（详细输出）"
	@echo "  make test-charge   - 只运行充电流程测试"
	@echo "  make test-device   - 只运行设备管理测试"
	@echo "  make test-order    - 只运行订单管理测试"
	@echo "  make clean         - 清理测试缓存"
	@echo ""

# 运行所有测试
test:
	@echo "运行所有 E2E 测试..."
	go test -v -timeout 10m ./...

# 运行所有测试（详细输出）
test-verbose:
	@echo "运行所有 E2E 测试（详细模式）..."
	E2E_VERBOSE=true go test -v -timeout 10m ./...

# 只运行充电流程测试
test-charge:
	@echo "运行充电流程测试..."
	go test -v -timeout 5m -run ChargeSuite

# 只运行设备管理测试
test-device:
	@echo "运行设备管理测试..."
	go test -v -timeout 5m -run DeviceSuite

# 只运行订单管理测试
test-order:
	@echo "运行订单管理测试..."
	go test -v -timeout 5m -run OrderSuite

# 清理测试缓存
clean:
	@echo "清理测试缓存..."
	go clean -testcache

# 运行测试并生成覆盖率报告
coverage:
	@echo "生成覆盖率报告..."
	go test -v -timeout 10m -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "覆盖率报告已生成: coverage.html"

# 安装依赖
deps:
	@echo "安装依赖..."
	go mod download
	go mod tidy

# 检查代码
lint:
	@echo "检查代码..."
	go vet ./...
	go fmt ./...
