# Project Context

## Purpose

IOT Server 是一个高性能充电桩物联网服务器，用于管理和控制电动汽车充电桩设备。主要目标：

- 支持 50,000+ 并发设备连接的 TCP 长连接管理
- 实时协议解析和命令分发（AP3000/BKV/GN 多协议支持）
- 订单管理和自动结算系统
- 第三方系统集成和事件推送
- 高可用性和容错设计

## Tech Stack

**核心技术**

- Go 1.24+ (主开发语言)
- Gin (HTTP Web 框架)
- pgx (PostgreSQL 驱动，直接 SQL 操作，无 ORM)
- go-redis (Redis 客户端)

**基础设施**

- PostgreSQL 14+ (主存储：设备、订单、事件)
- Redis 6+ (会话管理、消息队列、缓存)
- Prometheus (监控指标)
- Docker / Docker Compose (容器化)

**开发工具**

- golangci-lint (代码检查)
- Testify (测试框架)
- Swagger (API 文档生成)
- Make (构建自动化)

## Project Conventions

### Code Style

**格式化规则**

- 使用 `gofmt` + `goimports` 自动格式化（运行 `make fmt`）
- 强制执行 80-120 字符行宽建议
- Tab 缩进（Go 标准）

**命名约定**

- 包名：小写单词，简短描述性（如 `gateway`, `session`, `storage`）
- 接口：名词或动词 + "er" 后缀（如 `ProtocolHandler`, `SessionManager`）
- 私有成员：小驼峰（`deviceID`, `portStatus`）
- 导出成员：大驼峰（`DeviceID`, `PortStatus`）
- 常量：大驼峰或全大写（配置常量使用 `const` 块）

**注释规范**

- 所有导出函数必须有注释（以函数名开头）
- 复杂逻辑必须有行内注释
- TODO 注释格式：`// TODO(username): description`

**错误处理**

- 使用 `fmt.Errorf` 包装错误，添加上下文
- 避免 panic，除非不可恢复的初始化错误
- 使用 `errors.Is` 和 `errors.As` 判断错误类型

### Architecture Patterns

**多层事件驱动架构**

```
Gateway 层（TCP 服务器）
    ↓ 协议检测
Protocol 层（AP3000/BKV 解析器）
    ↓ 命令分发
Business Logic 层（处理器 + 服务）
    ↓ 异步队列
Persistence 层（PostgreSQL + Redis）
```

**关键设计模式**

- **Repository 模式**：所有数据库操作封装在 `internal/storage/*_repository.go`
- **适配器模式**：协议处理器实现 `ProtocolHandler` 接口
- **策略模式**：定价引擎支持多种计费策略
- **发布订阅**：事件推送使用 Redis 队列解耦
- **Outbox 模式**：第三方事件推送保证最终一致性

**依赖注入**

- 使用构造函数注入（`New*` 函数）
- 依赖在 `internal/app/bootstrap.go` 中组装
- 避免全局变量（除日志器和配置）

**并发模型**

- Goroutine per Connection（TCP 连接）
- Worker Pool（出站命令发送）
- 使用 `context.Context` 传递取消信号
- 数据库连接池大小根据负载调整

### Testing Strategy

**测试金字塔**

- **单元测试**：覆盖业务逻辑和工具函数（目标 70%+ 覆盖率）
  - 位置：`*_test.go` 与被测文件同目录
  - 使用 `testify/assert` 和 `testify/mock`
  - 运行：`make test`（带 race 检测）

- **集成测试**：验证数据库操作和 Redis 交互
  - 使用 Docker Compose 启动依赖
  - 运行：`make test-integration`

- **E2E 测试**：模拟完整设备交互流程
  - 位置：`tests/e2e/`
  - 运行：`make test-e2e`

**测试要求**

- 新功能必须包含单元测试
- P0 Bug 修复必须包含回归测试
- PR 合并前所有测试必须通过（`make test-all`）

### Git Workflow

**分支策略**

- `main`：主分支，保护分支，仅通过 PR 合并
- `feature/*`：新功能开发分支
- `fix/*`：Bug 修复分支
- `refactor/*`：代码重构分支

**提交规范**（遵循 Conventional Commits）

```
<type>(<scope>): <subject>

<body>

<footer>
```

**类型（type）**

- `feat`: 新功能
- `fix`: Bug 修复
- `refactor`: 重构（不改变功能）
- `test`: 添加或修改测试
- `docs`: 文档更新
- `chore`: 构建/工具变更
- `perf`: 性能优化

**示例**

```
feat(bkv): 添加心跳超时检测机制

实现基于 Redis TTL 的心跳超时检测，解决设备假在线问题。
超时时间设置为 60 秒，与设备心跳间隔一致。

Closes #123
```

**PR 要求**

- 标题遵循提交规范
- 描述清晰说明变更内容和原因
- 所有测试通过
- 代码已格式化（`make fmt`）

## Domain Context

**充电桩领域知识**

**设备类型**

- **交流桩**：使用 AP3000 协议，功率 3.5-22kW，单枪
- **直流桩**：使用 BKV 协议，功率 30-360kW，多枪（最多 12 枪）
- **充电过程**：刷卡 → 鉴权 → 启动 → 充电中 → 停止 → 结算

**关键业务流程**

1. **设备上线**：TCP 连接 → 协议握手 → 会话创建 → 上报状态
2. **订单生命周期**：创建 → 等待确认 → 充电中 → 完成/失败 → 推送事件
3. **心跳机制**：设备每 30 秒发送心跳，服务器 60 秒超时断线
4. **端口状态同步**：周期性（5 分钟）扫描所有端口，修正状态不一致

**业务规则**

- 端口只能同时服务一个订单（通过数据库行锁保证）
- 订单取消需等待设备 ACK 确认（10 秒超时窗口）
- 第三方事件推送使用 Outbox 模式保证最终一致性
- 离线订单自动标记为失败（心跳超时触发）

**协议特性**

- **AP3000**：帧格式 STX + 长度 + 数据 + CRC + ETX
- **BKV**：25+ 命令类型，TLV 编码，支持多枪并发
- **GN**：设备间组网通信，用于级联场景

## Important Constraints

**性能约束**

- 支持 50,000 并发 TCP 连接（单实例）
- 心跳处理延迟 < 100ms
- 订单创建响应时间 < 500ms
- 数据库查询超时 5 秒

**技术约束**

- Go 版本 ≥ 1.24（使用泛型和新特性）
- PostgreSQL 版本 ≥ 14（使用 JSONB 和分区表）
- Redis 版本 ≥ 6（使用流和 ACL）
- 所有 SQL 必须参数化（防 SQL 注入）

**安全约束**

- API 必须通过 `X-API-Key` 认证
- 敏感数据（卡号）必须加密存储
- 日志禁止打印密码、Token 等敏感信息
- 生产环境禁用 debug 日志级别

**业务约束**

- 订单金额使用 `decimal` 类型（避免浮点误差）
- 时间戳统一使用 UTC+0 存储
- 设备 ID 全局唯一，不可重复分配
- 端口状态变更必须记录审计日志

**运维约束**

- TCP 服务器必须最后启动（9 阶段启动流程）
- 数据库迁移由应用启动时自动执行
- 配置文件变更需重启服务（不支持热重载）
- 部署前必须备份数据库（`make backup`）

## External Dependencies

**核心依赖服务**

- **PostgreSQL**：主数据存储，保存设备、订单、事件
  - 连接信息：`configs/*.yaml` 中 `database` 配置
  - 数据备份：每次部署前自动备份（`scripts/backup.sh`）

- **Redis**：会话管理、消息队列、缓存
  - 会话 Key 格式：`session:{deviceID}` (TTL 90s)
  - 队列 Key：`outbound:normal`, `outbound:high`, `outbound:realtime`
  - 事件队列：`third_party_events` (Stream 结构)

**第三方集成**

- **客户 Webhook**：推送订单事件（创建、确认、失败、完成）
  - 配置：通过 `/api/v1/third-party` API 添加
  - 签名：HMAC-SHA256（Header: `X-Signature`）
  - 重试：最多 3 次，指数退避（1s, 2s, 4s）

**监控服务**

- **Prometheus**：指标采集（`/metrics` 端点）
  - 关键指标：在线设备数、消息吞吐量、队列长度
  - Grafana Dashboard：（待配置）

**开发依赖**

- **Docker Compose**：本地开发环境
- **golangci-lint**：代码质量检查
- **Swagger**：API 文档生成

**外部 API**（未来集成）

- 支付系统 API（待对接）
- 短信通知 API（待对接）
- 用户系统 API（待对接）

---

## OpenSpec Change History

本节记录所有已归档的 OpenSpec 变更提案及其正式规范。

### 已归档变更

#### 2025-11-20: Consistency Lifecycle Specification

**变更 ID**: `add-consistency-lifecycle-spec`
**规范路径**: `openspec/specs/consistency-lifecycle/spec.md`
**归档路径**: `openspec/changes/archive/2025-11-20-add-consistency-lifecycle-spec/`

**变更摘要:**

定义了设备/端口/订单生命周期的统一一致性策略，确保 DB、Redis 会话、Redis 队列和设备状态之间的最终一致性。

**核心规范:**

1. **单一真相来源（Single Source of Truth）**
   - 设备在线状态：以 SessionManager 为准，DB 视为缓存
   - 端口状态：30 秒收敛窗口
   - 订单状态机：与设备/端口/队列强一致性

2. **订单生命周期状态机**
   - 创建原子性：DB + 命令入队同时成功或失败
   - 过渡状态（stopping/cancelling/interrupted）强制 30-60s 终态化
   - 终态订单自动收敛端口状态

3. **端口状态收敛机制**
   - 订单终态 → 端口 idle/fault 自动更新
   - 后台任务负责修复协议上报缺失场景

4. **一致性感知读 API**
   - 检测并返回 `consistency_status` 字段
   - SessionManager 优先于 DB 状态

5. **后台自愈机制**
   - PortStatusSyncer：修复孤立充电端口
   - OrderMonitor：清理超时过渡状态
   - DeadLetterCleaner：处理命令失败

**影响范围:**

- 新增正式规范：`openspec/specs/consistency-lifecycle/spec.md`
- 新增 6 个 Prometheus 监控指标
- 新增 8 个 E2E 测试场景（100% 覆盖）
- 修复 6 处活跃订单查询和一致性检测代码
- 定义 9 条 OpenSpec 验证规则

**SLA 承诺:**

- 订单过渡状态收敛：stopping ≤30s, interrupted ≤60s
- 端口状态收敛：正常 ≤15s, 异常修复 ≤60s
- 自愈成功率 > 99%

**相关文档:**

- 提案：`archive/2025-11-20-add-consistency-lifecycle-spec/proposal.md`
- 设计：`archive/2025-11-20-add-consistency-lifecycle-spec/design.md`
- 任务清单：`archive/2025-11-20-add-consistency-lifecycle-spec/tasks.md`
- 验收标准：`archive/2025-11-20-add-consistency-lifecycle-spec/acceptance-criteria.md`
- 验证规则：`archive/2025-11-20-add-consistency-lifecycle-spec/.openspec-validation-rules.md`
