# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 项目概述

IOT Server 是一个高性能充电桩物联网服务器，采用 Go 1.24+ 开发，支持 50,000+ 并发设备连接。核心功能包括设备管理、TCP 长连接通信、多协议解析（AP3000/BKV/GN）、订单结算和第三方集成。

技术栈：Gin (HTTP)、PostgreSQL (主存储)、Redis (会话/队列)、Prometheus (监控)

## 常用命令

### 开发环境

```bash
# 本地开发（推荐）
make dev-up              # 启动依赖服务（PostgreSQL + Redis）
make dev-run             # 启动应用服务器（使用 configs/local.yaml）
make dev-down            # 停止依赖服务
make dev-all             # 一键启动完整环境

# Docker Compose 环境
make compose-up          # 启动完整开发环境（包含应用服务器）
make compose-down        # 停止开发环境
make compose-logs        # 查看日志
```

### 测试

```bash
# 完整测试套件（推荐）
make test-all            # 运行所有测试（单元测试 + P1 验证测试）

# 单项测试
make test                # 单元测试（带 race 检测）
make test-quick          # 快速测试（无 race 检测）
make test-verbose        # 详细输出
make test-coverage       # 生成覆盖率报告（coverage.html）
make test-p1             # P1 问题修复验证测试

# E2E 测试
make test-e2e            # 运行端到端集成测试

# 运行单个测试
go test -v -run TestFunctionName ./internal/package/
```

### 代码质量

```bash
make fmt                 # 格式化代码（自动修复）
make fmt-check           # 检查格式（不修改文件）
make vet                 # 静态分析
make lint                # golangci-lint 检查
make install-hooks       # 安装 Git pre-commit hooks
```

### 构建与部署

```bash
# 构建
make build               # 构建本地版本 -> bin/iot-server
make build-linux         # 构建 Linux 版本（用于部署）

# 部署（测试服务器）
make auto-deploy         # 自动化部署（构建 + 部署）⭐
make quick-deploy        # 快速部署（仅替换二进制）
make deploy              # 标准部署（测试环境，无备份）
make deploy-full         # 完整部署（带数据库备份）

# Docker 镜像
make docker-build        # 构建 Docker 镜像
```

### API 文档

```bash
make api-docs            # 生成 Swagger API 文档 -> api/swagger/
```

### 运维

```bash
make backup              # 备份数据库
make restore             # 恢复备份
make clean               # 清理构建文件
make clean-all           # 深度清理（包括缓存）
```

## 项目架构

### 架构模式

采用 **多层事件驱动架构**，核心分层：

```
Gateway 层（TCP 服务器）
    ↓ 协议检测
Protocol 层（AP3000/BKV 解析器）
    ↓ 命令分发
Business Logic 层（处理器 + 服务）
    ↓ 异步队列
Persistence 层（PostgreSQL + Redis）
```

### 核心组件职责

| 包路径 | 职责 |
|--------|------|
| `internal/app/` | 应用引导、依赖注入、9 阶段启动流程 |
| `internal/api/` | HTTP REST API（设备查询、订单管理、第三方配置） |
| `internal/gateway/` | TCP 服务器、协议检测、连接管理 |
| `internal/protocol/` | 协议解析器（ap3000、bkv、gn） |
| `internal/session/` | Redis 会话管理、在线状态检测 |
| `internal/storage/` | 数据库操作（repository 模式）、Redis 队列 |
| `internal/service/` | 业务逻辑（卡片服务、定价引擎） |
| `internal/thirdparty/` | 第三方集成（Webhook 推送、事件队列） |

### 关键设计决策

1. **TCP 服务器最后启动** - 确保所有依赖（数据库、Redis、服务）就绪后才接受设备连接，避免半初始化状态
2. **异步出站命令** - 下行命令通过 Redis 队列异步发送，处理器逻辑与 TCP I/O 解耦
3. **直接 SQL（无 ORM）** - 使用 `pgx` 直接操作数据库，显式控制事务和锁
4. **协议适配器模式** - 新协议通过实现 `ProtocolHandler` 接口添加，无需修改 Gateway 代码
5. **加权在线检测** - 综合心跳、TCP 状态、ACK 失败三个维度判定设备在线状态

### 数据流示例

#### 设备连接流程
```
设备 TCP 连接
  → Gateway 读取首包
  → 协议检测（AP3000/BKV）
  → 创建 ProtocolHandler
  → 保存会话到 Redis
  → 循环读取消息帧
```

#### 订单创建流程
```
HTTP API (/api/v1/orders)
  → OrderHandler 验证参数
  → OrderService 创建订单（DB 事务）
  → OutboundAdapter 推送到 Redis 队列
  → Worker 从队列取出
  → 通过 SessionManager 发送 TCP 命令
```

#### 心跳处理流程
```
设备发送心跳
  → BKVHandler 解析帧
  → 更新会话时间戳（Redis）
  → HeartbeatHandler 处理
  → 立即返回 ACK（优先级队列）
```

### 重要修复（P1 问题）

以下是已修复的关键问题，修改代码时需注意：

- **P1-1**：心跳超时设置为 60 秒（`internal/session/manager.go`）
- **P1-2**：延迟 ACK 拒绝（10 秒窗口，`internal/service/order_service.go`）
- **P1-3**：端口并发冲突保护（数据库事务 + 行锁，`internal/storage/port_repository.go`）
- **P1-4**：端口状态自动同步（`internal/app/port_syncer.go`，默认启用）
- **P1-5**：订单取消/停止中间态处理（`internal/protocol/bkv/handler.go`）
- **P1-6**：队列优先级标准化（`internal/storage/redis_queue.go`）
- **P1-7**：事件推送 Outbox 模式（`internal/thirdparty/event_queue.go`，默认启用）

运行 `make test-p1` 验证这些修复的测试用例。

## 配置文件

- `configs/example.yaml` - 开发环境配置模板
- `configs/local.yaml` - 本地开发配置（`make dev-run` 使用）
- `configs/production.yaml` - 生产环境配置
- `scripts/env.example` - Docker Compose 环境变量模板

配置通过环境变量 `IOT_CONFIG` 指定路径。

## 数据库

### 连接信息（本地开发）

- PostgreSQL: `localhost:5432`（用户：`iot`，密码：`iot123`，数据库：`iot_server`）
- Redis: `localhost:6379`（密码：`123456`）

### 数据库迁移

迁移脚本位于 `db/migrations/`，由应用启动时自动执行（`internal/app/bootstrap.go`）。

手动执行迁移：
```bash
psql -U iot -d iot_server -f db/migrations/001_initial_schema.sql
```

## 协议说明

支持三种协议，文档位于 `docs/协议/`：

1. **AP3000** - 交流桩协议（帧格式：STX + 长度 + 数据 + CRC + ETX）
2. **BKV** - 直流桩协议（25+ 命令类型，TLV 编码）
3. **GN** - 组网协议（设备间通信）

协议解析器位置：
- `internal/protocol/ap3000/parser.go`
- `internal/protocol/bkv/parser.go`
- `internal/protocol/gn/parser.go`

添加新协议需实现 `internal/protocol/protocol.go` 中的 `ProtocolHandler` 接口。

## 第三方集成

Webhook 推送机制（`internal/thirdparty/webhook.go`）：
- 异步推送事件到第三方 URL
- 支持重试（最多 3 次，指数退避）
- HMAC-SHA256 签名验证
- 超时时间：10 秒

配置第三方推送 URL：
```bash
curl -X POST http://localhost:7055/api/v1/third-party \
  -H "Content-Type: application/json" \
  -d '{"name":"客户A","webhook_url":"https://example.com/webhook","api_key":"secret"}'
```

## 监控与健康检查

```bash
# 健康检查
curl http://localhost:7055/healthz        # 存活检查
curl http://localhost:7055/readyz         # 就绪检查（包含依赖检查）

# Prometheus 指标
curl http://localhost:7055/metrics
```

关键指标：
- `iot_connected_devices` - 在线设备数
- `iot_http_requests_total` - HTTP 请求总数
- `iot_protocol_messages_total` - 协议消息计数
- `iot_outbound_queue_size` - 出站队列长度

## 开发工作流

推荐工作流：

1. **启动环境**：`make dev-up`（启动依赖服务）
2. **运行服务器**：`make dev-run`（启动应用）
3. **修改代码**：编辑相关文件
4. **运行测试**：`make test-all`（完整测试）
5. **格式化代码**：`make fmt`（自动格式化）
6. **部署测试**：`make auto-deploy`（自动化部署）

### 添加新功能

1. 在 `internal/` 对应包中添加代码
2. 在 `internal/` 对应包的 `_test.go` 中添加单元测试
3. 如需修改数据库，添加迁移脚本到 `db/migrations/`
4. 如需暴露 API，在 `internal/api/` 添加路由和处理器
5. 运行 `make test-all` 确保所有测试通过
6. 运行 `make fmt` 格式化代码

### 调试技巧

- 查看应用日志：`make compose-logs` 或 `make dev-logs`
- 查看 PostgreSQL 日志：`docker-compose logs postgres`
- 查看 Redis 命令：`redis-cli -a 123456 MONITOR`
- 连接数据库：`psql -h localhost -U iot -d iot_server`
- 查看 Redis 会话：`redis-cli -a 123456 KEYS "session:*"`

## 性能注意事项

- **连接池**：PostgreSQL 连接池大小配置在 `configs/*.yaml` 的 `database.pool_size`
- **并发控制**：TCP 连接使用 Goroutine per Connection 模式，依赖 Go runtime 调度
- **内存优化**：大对象（如订单事件）使用对象池减少 GC 压力
- **Redis 优化**：会话数据使用 Hash 结构，TTL 设置为 90 秒（心跳超时 60 秒 + 30 秒缓冲）

## 安全注意事项

- **SQL 注入防护**：所有 SQL 查询使用参数化查询（`pgx` 占位符 `$1, $2, ...`）
- **API 认证**：生产环境需配置 API Key（通过 HTTP Header `X-API-Key` 传递）
- **限流**：HTTP API 使用令牌桶限流（配置：`api.rate_limit`）
- **数据加密**：敏感数据（如卡号）存储前需加密
- **日志脱敏**：避免在日志中打印密码、API Key 等敏感信息

## 故障排查

### 常见问题

1. **设备连接失败** → 检查防火墙端口 7070（TCP）是否开放
2. **数据库连接超时** → 检查 PostgreSQL 是否启动：`docker-compose ps`
3. **Redis 连接失败** → 检查 Redis 密码配置是否正确
4. **心跳超时断连** → 检查 `session.heartbeat_timeout` 配置（应为 60 秒）
5. **订单推送失败** → 查看 `internal/thirdparty/event_queue.go` 日志，检查 Redis 队列

### 日志级别

修改 `configs/*.yaml` 中的 `log.level`：
- `debug` - 详细调试日志（包含协议帧内容）
- `info` - 常规信息（默认）
- `warn` - 警告信息
- `error` - 错误信息

## 提交规范

遵循 [Conventional Commits](https://www.conventionalcommits.org/)：

```
feat: 添加新功能
fix: 修复 Bug
refactor: 重构代码
test: 添加测试
docs: 更新文档
chore: 构建/工具变更
```

示例：
```bash
git commit -m "feat: 添加 AP3000 协议心跳 ACK 支持"
git commit -m "fix: 修复端口状态并发更新冲突"
```

## 相关文档

- [README.md](README.md) - 项目介绍和快速开始
- [DEPLOYMENT.md](DEPLOYMENT.md) - 完整部署指南
- [docs/架构/项目架构设计.md](docs/架构/项目架构设计.md) - 详细架构文档
- [docs/协议/](docs/协议/) - 协议规范文档
- [docs/api/](docs/api/) - HTTP API 文档
- [docs/CI-CD-GUIDE.md](docs/CI-CD-GUIDE.md) - CI/CD 自动化部署
