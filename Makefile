SHELL := /bin/bash
APP := iot-server
PKG := ./...

# 版本信息
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u '+%Y-%m-%d %H:%M:%S')
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

.PHONY: all tidy fmt fmt-check vet build run test clean lint help install-hooks

# 默认目标
all: tidy fmt vet build

# Go工具链
tidy:
	@echo "整理依赖..."
	go mod tidy

fmt:
	@echo "格式化代码..."
	gofmt -s -w .

fmt-check:
	@echo "检查代码格式..."
	@if [ "$$(gofmt -s -l . | wc -l)" -gt 0 ]; then \
		echo "❌ 以下文件需要格式化:"; \
		gofmt -s -l .; \
		echo ""; \
		echo "运行 'make fmt' 自动修复"; \
		exit 1; \
	fi
	@echo "✅ 代码格式检查通过"

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

# 本地开发环境（仅依赖服务）
.PHONY: dev-up dev-down dev-logs dev-status dev-run dev-clean dev-all dev-check-ports

dev-check-ports:
	@./scripts/check-ports.sh

dev-up:
	@echo "🚀 启动本地开发依赖服务..."
	docker-compose -f docker-compose.local.yml up -d
	@echo ""
	@echo "✅ 依赖服务已启动！"
	@echo "   PostgreSQL: localhost:5432 (用户: iot, 密码: iot123, 数据库: iot_server)"
	@echo ""
	@echo "📝 注意事项："
	@echo "   - Redis: 使用本地现有 Redis (localhost:6379, 密码: 123456)"
	@echo "   - 如需独立 Redis，请编辑 docker-compose.local.yml 取消注释"
	@echo ""
	@echo "💡 下一步: 运行 'make dev-run' 启动应用服务器"

dev-down:
	@echo "停止本地开发依赖服务..."
	docker-compose -f docker-compose.local.yml down
	@echo "✅ 依赖服务已停止"

dev-logs:
	@echo "查看依赖服务日志..."
	docker-compose -f docker-compose.local.yml logs -f

dev-status:
	@echo "检查依赖服务状态..."
	docker-compose -f docker-compose.local.yml ps

dev-run:
	@echo "🚀 启动本地开发服务器..."
	@echo "配置文件: configs/local.yaml"
	IOT_CONFIG=configs/local.yaml go run ./cmd/server

dev-clean:
	@echo "清理本地开发环境（包括数据卷）..."
	docker-compose -f docker-compose.local.yml down -v
	@echo "✅ 本地开发环境已清理"

dev-all: dev-up
	@echo ""
	@echo "⏳ 等待服务启动 (5秒)..."
	@sleep 5
	@echo ""
	@$(MAKE) dev-run

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

# 监控和调试
.PHONY: monitor monitor-diagnose monitor-logs monitor-errors monitor-help

monitor-help:
	@./scripts/monitor.sh help

monitor:
	@./scripts/monitor.sh diagnose

monitor-logs:
	@./scripts/monitor.sh logs

monitor-errors:
	@./scripts/monitor.sh errors 30

monitor-metrics:
	@./scripts/monitor.sh metrics

# TCP 模块测试
.PHONY: tcp-check tcp-connect tcp-metrics tcp-test-all

tcp-check:
	@./scripts/tcp-test.sh check-port

tcp-connect:
	@./scripts/tcp-test.sh connect

tcp-metrics:
	@./scripts/tcp-test.sh metrics

tcp-test-all:
	@./scripts/tcp-test.sh run-all

# 协议实时监控
.PHONY: protocol-live protocol-logs protocol-stats protocol-devices

protocol-live:
	@./scripts/protocol-monitor.sh live

protocol-logs:
	@./scripts/protocol-monitor.sh logs

protocol-stats:
	@./scripts/protocol-monitor.sh stats

protocol-devices:
	@./scripts/protocol-monitor.sh devices

# Git Hooks
install-hooks:
	@echo "安装 Git hooks..."
	@chmod +x .git/hooks/pre-commit 2>/dev/null || true
	@if [ ! -f .git/hooks/pre-commit ]; then \
		echo '#!/bin/sh' > .git/hooks/pre-commit; \
		echo 'echo "🔍 运行 pre-commit 检查..."' >> .git/hooks/pre-commit; \
		echo '' >> .git/hooks/pre-commit; \
		echo '# 检查代码格式' >> .git/hooks/pre-commit; \
		echo 'if [ "$$(gofmt -s -l . | wc -l)" -gt 0 ]; then' >> .git/hooks/pre-commit; \
		echo '    echo "❌ 代码格式检查失败！以下文件需要格式化:"' >> .git/hooks/pre-commit; \
		echo '    gofmt -s -l .' >> .git/hooks/pre-commit; \
		echo '    echo ""' >> .git/hooks/pre-commit; \
		echo '    echo "请运行以下命令修复格式问题："' >> .git/hooks/pre-commit; \
		echo '    echo "  make fmt"' >> .git/hooks/pre-commit; \
		echo '    echo ""' >> .git/hooks/pre-commit; \
		echo '    echo "或者自动修复并重新提交："' >> .git/hooks/pre-commit; \
		echo '    echo "  make fmt && git add . && git commit --amend --no-edit"' >> .git/hooks/pre-commit; \
		echo '    exit 1' >> .git/hooks/pre-commit; \
		echo 'fi' >> .git/hooks/pre-commit; \
		echo '' >> .git/hooks/pre-commit; \
		echo 'echo "✅ 代码格式检查通过"' >> .git/hooks/pre-commit; \
		echo 'exit 0' >> .git/hooks/pre-commit; \
		chmod +x .git/hooks/pre-commit; \
		echo "✅ Pre-commit hook 已安装"; \
	else \
		echo "⚠️  Pre-commit hook 已存在，跳过安装"; \
		echo "   如需重新安装，请先删除 .git/hooks/pre-commit"; \
	fi

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

# API文档生成
.PHONY: swagger-init swagger-gen swagger-validate api-docs

swagger-init:
	@echo "安装 swag 工具..."
	@which swag > /dev/null || go install github.com/swaggo/swag/cmd/swag@latest
	@echo "✅ swag 工具已就绪"

swagger-gen:
	@echo "生成Swagger API文档..."
	@which swag > /dev/null || (echo "❌ swag 工具未安装，运行: make swagger-init" && exit 1)
	swag init -g cmd/server/main.go -o api/swagger --parseDependency --parseInternal
	@echo "✅ API文档已生成: api/swagger/swagger.json"
	@echo "   查看文档: api/swagger/swagger.yaml"

swagger-validate:
	@echo "验证OpenAPI文档..."
	@which swagger > /dev/null || (echo "⚠️  swagger 工具未安装，跳过验证" && exit 0)
	swagger validate api/openapi/openapi.yaml
	@echo "✅ OpenAPI文档验证通过"

api-docs: swagger-init swagger-gen
	@echo "✅ API文档生成完成"
	@echo ""
	@echo "📖 查看API文档:"
	@echo "   JSON: api/swagger/swagger.json"
	@echo "   YAML: api/swagger/swagger.yaml"
	@echo "   HTML: 启动服务后访问 http://localhost:7055/swagger/index.html"

# 帮助
help:
	@echo "IOT Server Makefile命令："
	@echo ""
	@echo "🚀 本地开发（推荐）："
	@echo "  make dev-all         - 一键启动（依赖服务+应用服务器）"
	@echo "  make dev-up          - 启动依赖服务（PostgreSQL，复用本地Redis）"
	@echo "  make dev-run         - 启动应用服务器（需先执行 dev-up）"
	@echo "  make dev-check-ports - 检查端口占用情况"
	@echo "  make dev-down        - 停止依赖服务"
	@echo "  make dev-logs        - 查看依赖服务日志"
	@echo "  make dev-status      - 检查依赖服务状态"
	@echo "  make dev-clean       - 清理本地开发环境（包括数据）"
	@echo ""
	@echo "开发相关："
	@echo "  make build           - 构建应用"
	@echo "  make run             - 运行开发服务器（使用 example.yaml）"
	@echo "  make test            - 运行测试"
	@echo "  make test-coverage   - 生成测试覆盖率报告"
	@echo "  make fmt             - 格式化代码（自动修复）"
	@echo "  make fmt-check       - 检查代码格式（不修改）"
	@echo "  make lint            - 代码检查"
	@echo "  make install-hooks   - 安装 Git pre-commit hooks"
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
	@echo "监控调试："
	@echo "  make monitor               - 运行完整诊断（推荐）"
	@echo "  make monitor-logs          - 查看实时日志"
	@echo "  make monitor-errors        - 查看错误日志"
	@echo "  make monitor-metrics       - 查看业务指标"
	@echo "  make monitor-help          - 查看所有监控命令"
	@echo ""
	@echo "TCP 模块测试："
	@echo "  make tcp-check             - 检查 TCP 端口"
	@echo "  make tcp-connect           - 测试 TCP 连接"
	@echo "  make tcp-metrics           - 查看 TCP 指标"
	@echo "  make tcp-test-all          - 运行所有 TCP 测试"
	@echo ""
	@echo "协议实时监控："
	@echo "  make protocol-live         - 综合监控（推荐，需 tmux）"
	@echo "  make protocol-logs         - 实时协议日志"
	@echo "  make protocol-stats        - 实时统计数据"
	@echo "  make protocol-devices      - 查看在线设备"
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
	@echo "API文档："
	@echo "  make api-docs        - 生成完整API文档（推荐）"
	@echo "  make swagger-init    - 安装swagger工具"
	@echo "  make swagger-gen     - 生成swagger文档"
	@echo "  make swagger-validate - 验证OpenAPI文档"
	@echo ""
	@echo "当前版本: $(VERSION)"
	@echo ""
	@echo "💡 提示: 现已支持 Swagger 自动生成API文档"
	@echo "   运行 'make api-docs' 生成完整文档"


