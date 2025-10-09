SHELL := /bin/bash
APP := iot-server
PKG := ./...

# 版本信息
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u '+%Y-%m-%d %H:%M:%S')
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

.PHONY: all tidy fmt vet build run test clean lint help

# 默认目标
all: tidy fmt vet build

# Go工具链
tidy:
	@echo "整理依赖..."
	go mod tidy

fmt:
	@echo "格式化代码..."
	go fmt $(PKG)

vet:
	@echo "静态分析..."
	go vet $(PKG)

lint:
	@echo "Lint检查..."
	golangci-lint run || true

# 构建
build:
	@echo "构建应用..."
	GOOS=$(shell go env GOOS) GOARCH=$(shell go env GOARCH) \
	go build -ldflags="-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME) -X main.GitCommit=$(GIT_COMMIT)" \
	-o bin/$(APP) ./cmd/server
	@echo "构建完成: bin/$(APP) (version: $(VERSION))"

build-linux:
	@echo "构建Linux版本..."
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
	go build -ldflags="-w -s -X main.Version=$(VERSION)" \
	-o bin/$(APP)-linux ./cmd/server

# 运行
run:
	@echo "启动开发服务器..."
	IOT_CONFIG=./configs/example.yaml go run ./cmd/server

run-prod:
	@echo "启动生产模式服务器..."
	IOT_CONFIG=./configs/production.yaml ./bin/$(APP)

# 测试
test:
	@echo "运行测试..."
	go test -race -coverprofile=coverage.out $(PKG)

test-verbose:
	@echo "运行详细测试..."
	go test -v -race -coverprofile=coverage.out $(PKG)

test-coverage:
	@echo "生成覆盖率报告..."
	go test -coverprofile=coverage.out $(PKG)
	go tool cover -html=coverage.out -o coverage.html
	@echo "覆盖率报告: coverage.html"

# Docker Compose - 开发环境
.PHONY: compose-up compose-down compose-logs

compose-up:
	@echo "启动开发环境..."
	docker compose up -d

compose-down:
	@echo "停止开发环境..."
	docker compose down -v

compose-logs:
	docker compose logs -f

# Docker Compose - 生产环境
.PHONY: prod-up prod-down prod-restart prod-logs prod-status

prod-up:
	@echo "启动生产环境..."
	docker-compose -f docker-compose.prod.yml up -d

prod-down:
	@echo "停止生产环境..."
	docker-compose -f docker-compose.prod.yml down

prod-restart:
	@echo "重启生产环境..."
	docker-compose -f docker-compose.prod.yml restart

prod-logs:
	docker-compose -f docker-compose.prod.yml logs -f iot-server

prod-status:
	docker-compose -f docker-compose.prod.yml ps

# Docker镜像
.PHONY: docker-build docker-push docker-clean

docker-build:
	@echo "构建Docker镜像..."
	docker build \
		--build-arg BUILD_VERSION=$(VERSION) \
		--build-arg BUILD_TIME="$(BUILD_TIME)" \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		-t $(APP):$(VERSION) \
		-t $(APP):latest \
		.
	@echo "镜像构建完成: $(APP):$(VERSION)"

docker-push:
	@echo "推送Docker镜像..."
	docker push $(APP):$(VERSION)
	docker push $(APP):latest

docker-clean:
	@echo "清理Docker资源..."
	docker system prune -f

# 部署
.PHONY: deploy backup restore

deploy:
	@echo "执行快速部署（测试模式）..."
	@echo "💡 提示："
	@echo "   测试环境：make deploy           （快速，不备份）"
	@echo "   生产环境：BACKUP=true make deploy（安全，带备份）"
	./scripts/deploy.sh

backup:
	@echo "执行备份..."
	./scripts/backup.sh backup

restore:
	@echo "恢复备份..."
	./scripts/backup.sh restore

# 清理
clean:
	@echo "清理构建文件..."
	rm -rf bin
	rm -f coverage.out coverage.html
	rm -rf tmp

clean-all: clean
	@echo "深度清理..."
	docker-compose -f docker-compose.prod.yml down -v
	docker compose down -v

# CI/CD 相关
.PHONY: ci-check ci-test ci-build ci-setup

ci-check:
	@echo "执行 CI 检查..."
	@echo "1. 代码格式检查..."
	@if [ "$$(gofmt -s -l . | wc -l)" -gt 0 ]; then \
		echo "❌ 以下文件需要格式化:"; \
		gofmt -s -l .; \
		exit 1; \
	fi
	@echo "✅ 代码格式检查通过"
	@echo "2. 静态分析..."
	@go vet $(PKG)
	@echo "✅ 静态分析通过"

ci-test:
	@echo "运行 CI 测试..."
	@go test -v -race -coverprofile=coverage.out $(PKG)
	@go tool cover -func=coverage.out

ci-build:
	@echo "CI 构建..."
	@make build
	@echo "✅ 构建成功"

ci-setup:
	@echo "设置 CI/CD 环境..."
	@if [ ! -f .github/workflows/ci.yml ]; then \
		echo "❌ GitHub Actions 配置文件不存在"; \
		exit 1; \
	fi
	@echo "✅ GitHub Actions 已配置"
	@echo ""
	@echo "下一步："
	@echo "1. 配置 GitHub Secrets（参考 .github/secrets-template.txt）"
	@echo "2. 配置 GitHub Environments（staging, production）"
	@echo "3. 查看完整指南: docs/CI-CD-GUIDE.md"

# 帮助
help:
	@echo "IOT Server Makefile命令："
	@echo ""
	@echo "开发相关："
	@echo "  make build           - 构建应用"
	@echo "  make run             - 运行开发服务器"
	@echo "  make test            - 运行测试"
	@echo "  make test-coverage   - 生成测试覆盖率报告"
	@echo "  make fmt             - 格式化代码"
	@echo "  make lint            - 代码检查"
	@echo ""
	@echo "Docker开发环境："
	@echo "  make compose-up      - 启动开发环境"
	@echo "  make compose-down    - 停止开发环境"
	@echo "  make compose-logs    - 查看日志"
	@echo ""
	@echo "生产环境："
	@echo "  make docker-build    - 构建Docker镜像"
	@echo "  make prod-up         - 启动生产环境"
	@echo "  make prod-down       - 停止生产环境"
	@echo "  make prod-restart    - 重启生产环境"
	@echo "  make prod-logs       - 查看生产环境日志"
	@echo ""
	@echo "部署相关："
	@echo "  make deploy                - 快速部署（测试模式，不备份）"
	@echo "  BACKUP=true make deploy    - 安全部署（生产模式，自动备份）"
	@echo ""
	@echo "维护相关："
	@echo "  make backup          - 备份数据"
	@echo "  make restore         - 恢复数据"
	@echo "  make clean           - 清理构建文件"
	@echo "  make clean-all       - 深度清理（包括Docker）"
	@echo ""
	@echo "CI/CD相关："
	@echo "  make ci-check        - 执行 CI 代码检查"
	@echo "  make ci-test         - 运行 CI 测试套件"
	@echo "  make ci-build        - CI 构建验证"
	@echo "  make ci-setup        - 检查 CI/CD 配置"
	@echo ""
	@echo "当前版本: $(VERSION)"
	@echo ""
	@echo "💡 提示: 现已支持 GitHub Actions 自动化部署"
	@echo "   查看文档: docs/CI-CD-GUIDE.md"


