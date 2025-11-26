# Project Context

## Purpose

IOT Server 是一个专为充电桩设备设计的高性能物联网服务器，核心目标包括：

- **设备连接管理**：支持 50,000+ 并发 TCP 长连接
- **多协议支持**：AP3000、BKV、GN 等主流充电桩协议
- **实时数据采集**：设备状态、充电会话、计量数据
- **远程控制**：启停充电、参数配置、OTA 升级
- **第三方集成**：Webhook 事件推送、RESTful API 对接
- **高可用运维**：健康检查、限流熔断、自动重连

## Tech Stack

### 核心语言与框架
- **Go 1.25.4** - 主开发语言
- **Gin** - HTTP Web 框架
- **GORM + pgx/v5** - ORM 与 PostgreSQL 驱动
- **go-redis/v9** - Redis 客户端
- **zap** - 结构化日志
- **viper** - 配置管理

### 存储层
- **PostgreSQL 15** - 主数据库（设备、会话、事件）
- **Redis 7** - 分布式会话、消息队列、缓存

### 监控与可观测
- **Prometheus** - 指标采集
- **Grafana** - 可视化看板（可选）
- **lumberjack** - 日志轮转

### 基础设施
- **Docker / Docker Compose** - 容器化部署
- **GitHub Actions** - CI/CD 自动化
- **Swagger (swaggo)** - API 文档生成

## Project Conventions

### Code Style

- **格式化**：使用 `gofmt -s` 自动格式化，CI 强制检查
- **静态分析**：`go vet` + `golangci-lint` 全量检查
- **命名规范**：
  - 包名：小写单词，如 `session`、`storage`
  - 接口名：动词或描述性名词，如 `Manager`、`Repository`
  - 常量：全大写下划线分隔，如 `PortStatusIdle`
- **注释语言**：中文注释（与现有代码保持一致）
- **Pre-commit Hook**：自动格式化检查（`make install-hooks`）

### Architecture Patterns

采用**分层架构 + 依赖注入**设计：

```
cmd/server/              # 应用入口
internal/
├── app/                 # 应用引导、组件工厂
│   └── bootstrap/       # 统一启动流程（9阶段）
├── api/                 # HTTP 路由与处理器
│   └── middleware/      # 认证、限流中间件
├── gateway/             # TCP 连接处理器
├── protocol/            # 协议适配层
│   ├── ap3000/          # AP3000 协议解析器
│   ├── bkv/             # BKV 协议解析器
│   └── gn/              # GN 组网协议
├── session/             # 分布式会话管理（Redis）
├── storage/             # 数据访问层
│   ├── pg/              # PostgreSQL 仓储
│   └── redis/           # Redis 队列
├── coremodel/           # 领域模型（设备/端口/会话）
├── service/             # 业务服务（计费、刷卡）
├── thirdparty/          # 第三方集成（Webhook）
├── outbound/            # 下行消息队列
├── health/              # 健康检查
└── metrics/             # Prometheus 指标
```

**关键设计模式**：
- **适配器模式**：协议驱动与核心解耦（`DriverCore`）
- **事件驱动**：`CoreEvent` 标准化上报，`EventQueue` 异步推送
- **Outbox 模式**：可靠事件投递，防止消息丢失
- **熔断器模式**：TCP 连接限流保护（`circuit_breaker`）

### Testing Strategy

- **单元测试**：`go test -race`，强制 race 检测
- **覆盖率目标**：核心模块 > 70%
- **测试分类**：按 package 并行执行（app/service/api/protocol/outbound/storage）
- **集成测试**：CI 环境自动启动 PostgreSQL + Redis 容器
- **E2E 测试**：`make test-e2e`（test/e2e 目录）

```bash
make test-all      # 完整测试套件
make test-quick    # 快速测试（无race）
make test-coverage # 生成覆盖率报告
```

### Git Workflow

- **主分支**：`main`（生产）、`develop`（开发）
- **提交规范**：[Conventional Commits](https://www.conventionalcommits.org/)
  - `feat:` 新功能
  - `fix:` 修复
  - `refactor:` 重构
  - `docs:` 文档
  - `test:` 测试
  - `chore:` 工具/配置
- **PR 流程**：提交到 `main`/`develop` 触发 CI
- **版本发布**：Git Tag 触发 `deploy-production.yml`

## Domain Context

### 充电桩设备模型

```
设备(Device) 1:N 端口(Port) 1:N 充电会话(Session)
```

- **DeviceID**：设备物理标识（协议上报）
- **PortNo**：端口编号（0-based）
- **BusinessNo**：上游业务订单号（第三方下发）

### 端口状态机

```
offline -> idle -> charging -> completed/interrupted
                            -> fault
```

- `idle`：空闲可用
- `charging`：正在充电
- `fault`：故障
- `stopping`：停止中（中间态）

### 协议适配

所有协议事件通过 `CoreEvent` 标准化上报：
- `DeviceHeartbeat` - 设备心跳
- `PortSnapshot` - 端口状态快照
- `SessionStarted/Progress/Ended` - 会话生命周期
- `ExceptionReported` - 异常告警

下行命令通过 `CoreCommand` 标准化下发：
- `StartCharge/StopCharge` - 启停充电
- `SetParams` - 参数配置
- `TriggerOTA` - 固件升级

## Important Constraints

### 技术约束

- **Redis 必选**：分布式会话管理必须依赖 Redis
- **PostgreSQL 必选**：持久化存储唯一选择
- **TCP 长连接**：设备侧不支持 HTTP，仅 TCP
- **并发限制**：单实例最大 10,000 连接，需水平扩展

### 业务约束

- **心跳超时**：120 秒无心跳判定离线
- **ACK 窗口**：下行命令 30 秒内需 ACK
- **事件去重**：同一事件 1 小时内不重复推送

### 运维约束

- **健康检查**：`/healthz`（存活）、`/readyz`（就绪）
- **优雅停机**：10 秒超时
- **资源限制**：4 CPU / 4GB 内存

## External Dependencies

### 必选依赖

| 服务 | 版本 | 用途 |
|------|------|------|
| PostgreSQL | 15+ | 主数据库 |
| Redis | 7+ | 会话/队列 |

### 可选依赖

| 服务 | 用途 |
|------|------|
| Prometheus | 指标采集 |
| Grafana | 可视化 |
| 第三方 Webhook | 事件推送 |

### 配置方式

环境变量优先（`IOT_` 前缀），回退到 YAML 配置文件：
- `IOT_DATABASE_DSN` - 数据库连接
- `IOT_REDIS_ADDR` - Redis 地址
- `WEBHOOK_URL` - 第三方推送地址
