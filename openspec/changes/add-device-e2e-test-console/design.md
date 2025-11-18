## 背景

本系统是充电桩 IoT 中间件，核心职责是：

- 接入 BKV/AP3000 等协议的真实设备（TCP 长连接 + 会话管理 + 心跳 + 状态上报）。
- 暴露第三方 HTTP API（`/api/v1/third/...`）供业务方发起设备控制与订单查询。
- 使用 Postgres 持久化设备与订单状态，使用 Redis 维护会话和下行队列。
- 通过 Webhook 向第三方推送订单和设备相关事件。

现有测试体系包括：

- `go test ./internal/...` 的单元和集成测试。
- Docker 测试环境（`docker-compose.test.yml`）。
- 文档层面的技术规范与第三方 API、事件推送规范。

但在“真实设备 + 第三方回调”的场景下，目前缺少：

- 一个中心化的 Web 测试控制台来发起和观察完整 E2E 流程。
- 一套贯穿 HTTP → 队列 → 设备 → 订单 → 事件 的 test_session_id 追踪机制。
- 覆盖常见异常场景（离线、端口占用、断网、Webhook 失败等）的标准化 E2E 测试矩阵。

本变更旨在在不破坏现有第三方 API 行为的前提下，引入内部测试控制台和 E2E 流程，以降低上线风险。

## 目标 / 非目标

### 目标

- 提供 **内部 Web 测试控制台**：
  - 列出可用于 E2E 测试的真实设备及其当前状态。
  - 通过内部 API 发起 "启动充电" / "停止充电" 等测试，底层完全复用现有 `ThirdPartyHandler` 逻辑。
  - 展示单次测试会话在 HTTP/DB/队列/设备/事件 各层面的完整时间线数据，并支持导出。
- 引入 **`test_session_id` 追踪机制**：
  - 从测试入口生成全局唯一 ID。
  - 在订单、出站队列、指令日志、事件 JSON 等处统一记录；日志中也包含该 ID。
  - 提供按 `test_session_id` 聚合和查询 E2E 数据的接口。
- 制定 **测试环境 + 生产环境白名单设备的小流量 E2E 验证流程**，并写入 Runbook。

### 非目标

- 不改变对外第三方 API 的业务语义和返回格式（除了增加内部使用的可选字段外）。
- 不引入新的外部依赖（数据库/消息系统等），只在现有 Postgres 和 Redis 基础上扩展。
- 不替代现有单元/集成测试体系，而是构建其上的一层“真实设备 E2E 验证”。

## 决策

### 决策 1：内部测试路径复用 `ThirdPartyHandler`

**方案 A（选定）**：

- 在 `internal/api` 下新增 `TestConsoleHandler`（命名示例），提供 `/internal/test/...` 路由：
  - `POST /internal/test/devices/{phy_id}/charge` 内部构造 `StartChargeRequest`，调用 `ThirdPartyHandler.StartCharge`。
  - `POST /internal/test/devices/{phy_id}/stop` 调用 `ThirdPartyHandler.StopCharge`。
  - 使用同一套设备在线校验（`SessionManager.IsOnline`）、端口并发检查（事务+行锁）、订单创建和 BKV 控制帧构造逻辑。

**理由：**

- 保证测试路径与真实业务路径完全一致，避免“测出来没问题，但真实路径不一样”的偏差。
- 降低维护成本：后续统一维护 `ThirdPartyHandler` 即可，测试控制台只是包装层。

**方案 B（放弃）**：

- 为测试控制台单独实现直连设备的“快捷命令”，绕过订单/队列/事件。

**放弃原因：**

- 虽然实现上更简单，但无法覆盖真实业务数据流（订单状态机、事件推送等），失去 E2E 意义。

### 决策 2：`test_session_id` 的传递和存储

**生成位置：**

- 在内部测试接口层（TestConsoleHandler）生成：
  - 每次点击“发起测试”生成一个 UUID 作为 `test_session_id`。
  - 通过 Gin context 和 HTTP header（如 `X-Test-Session-Id`）传递给 `ThirdPartyHandler`。

**存储与映射：**

- DB：
  - `orders` 表新增列 `test_session_id TEXT NULL`（或使用现有 correlation 字段，如果已经存在并满足需求）。
  - 如有 `cmd_log` 表，考虑新增 `test_session_id` 列，或将其作为可解析的扩展字段持久化。
  - `outbound_queue` 表复用 `correlation_id` 列存储 `test_session_id`，便于关联出站命令与测试会话。
- 队列结构：
  - `redisstorage.OutboundMessage`（在 `internal/storage/redis` 中定义）扩展属性：`TestSessionID string` 或重用已有 `CorrelationID` 字段。
- 事件 JSON：
  - 在生成 `thirdparty.Event` 时，将 `test_session_id` 放入 `Data["testSessionId"]`。

**日志：**

- 在以下位置增加 `test_session_id` 字段：
  - `ThirdPartyHandler.StartCharge` / `StopCharge` / `GetDevice` / `GetOrder` 日志调用。
  - `RedisWorker.processOne` 下发命令日志。
  - 事件推送相关日志（`Pusher.SendJSON` 调用方）。

**理由：**

- 利用现有的 `correlation_id` 和事件结构，避免大范围 schema 改动。
- 按单一 ID 即可贯穿 HTTP → 订单 → 队列 → 设备 → 事件，简单明确。

### 决策 3：时间线视图的构建方式

**方案 A（选定）：通过 DB/队列聚合构建**

- 新增一个内部 service，负责按 `test_session_id` 聚合以下数据：
  - `orders`：`SELECT ... FROM orders WHERE test_session_id = $1`。
  - 出站队列：`SELECT ... FROM outbound_queue WHERE correlation_id = $1`。
  - 指令日志：`SELECT ... FROM cmd_log WHERE test_session_id = $1`（如有）。
  - 订单事件：通过 `Repository.GetOrderEvents` 等接口获得事件列表，筛选出带 `testSessionId` 的记录。
- 将上述数据组装为统一的时间线结构（按 timestamp 排序）。

**理由：**

- 使用已有存储（Postgres + Redis）的记录，不依赖实时日志解析。
- 时间线视图可重复生成，适合脱机审计和导出。

**方案 B（放弃）：从日志系统实时解析**

- 依赖集中日志（如 ELK），通过 `test_session_id` 在日志中搜索并拼接时间线。

**放弃原因：**

- 会引入对外部日志系统的紧耦合，且难以保证格式长期稳定。
- 不利于在本地/测试服务器快速部署和使用。

### 决策 4：生产环境 E2E 验证策略

- 仅对“白名单设备 + 小额金额”执行 E2E 测试。
- 测试控制台生产环境必须有更严格的访问控制：
  - 仅开放给少数有审批记录的用户。
  - 仅允许访问 /internal/test/... 下的有限子场景（例如只允许“正常充电成功”测试）。
- 每次生产测试必须：
  - 记录 `test_session_id` → 导出时间线 JSON。
  - 由业务/财务/运维参与对账确认。

## 风险 / 权衡

### 风险 1：内部 API 暴露带来的安全风险

- 若 `/internal/test/...` 被错误地暴露给外部，可能导致未授权的设备控制。

**缓解措施：**

- 必须在路由层增加显式的内部鉴权中间件：
  - IP 白名单。
  - 内部 API Key。
  - 或 RBAC 角色限制 + VPN 访问。
- 在配置中提供开关（如 `enable_test_console`），默认关闭生产环境测试控制台。

### 风险 2：DB schema 变更引起的影响

- 新增 `test_session_id` 或修改 `outbound_queue`/`cmd_log` 等表需要 migration，可能影响现有数据或性能。

**缓解措施：**

- 使用向前兼容的 ALTER 语句（新增可空列，默认值为空）。
- 在测试环境先执行 migration，并在 E2E 测试中覆盖相关路径。
- 对高频表评估索引策略：`
  - 初期不对 `test_session_id` 建索引（仅 E2E 查询使用，规模有限）。
  - 如后续需要频繁按 `test_session_id` 查找，可增加覆盖索引。

### 风险 3：额外的日志和查询对性能的影响

- 在高 QPS 情况下，增加日志字段和 timeline 查询可能带来一定开销。

**缓解措施：**

- 针对日志：
  - 仅在测试环境和生产 E2E 测试时启用详细日志（通过 log level 或 feature flag 控制）。
- 针对时间线查询：
  - 限制单次查询的数据量（只取与该 `test_session_id` 相关的记录）。
  - 为 timeline API 增加速率限制，避免误用。

### 风险 4：测试逻辑与业务逻辑长期偏离

- 若未来业务逻辑更新而测试控制台未同步更新，E2E 测试结果可能不再代表真实业务路径。

**缓解措施：**

- 测试控制台只做“薄封装”，核心逻辑统一依赖 `ThirdPartyHandler` 和 `Repository`。
- 在变更第三方 API / 订单状态机 / 事件结构时，将更新测试控制台视为同一变更的一部分。

## 迁移计划

1. **准备阶段**
   - 设计并确认需要的 DB 字段（`orders.test_session_id`、`cmd_log.test_session_id` 等）。
   - 在 `openspec/changes/add-device-e2e-test-console` 下完成 proposal/spec/tasks/design（本变更）。

2. **Schema 迁移**
   - 编写并审核 SQL migration 脚本：
     - `ALTER TABLE orders ADD COLUMN test_session_id TEXT NULL;`
     - 如需要，对 `cmd_log` / `outbound_queue` 等表新增相应列。
   - 在测试环境执行 migration，并运行现有测试（`go test ./internal/...`）。

3. **应用代码实现**
   - 在 `internal/api` 中新增 TestConsoleHandler 与 `/internal/test/...` 路由。
   - 在 `ThirdPartyHandler` 和出站队列/事件相关逻辑中增加 `test_session_id` 传递与日志字段。
   - 实现时间线聚合 service 和导出接口。

4. **测试环境验证**
   - 按 `tasks.md` 中的 E2E 测试矩阵执行多轮测试：
     - 覆盖正常充电、离线、端口占用、断网中断、Webhook 失败重试等场景。
   - 确认测试控制台展示和导出数据与 DB/设备/第三方一致。

5. **生产可用性准备**
   - 在生产环境执行 schema 迁移（禁用测试控制台路由）。
   - 配置白名单设备和更严格的访问控制策略。

6. **生产小流量 E2E 验证**
   - 临时启用生产环境测试控制台受限功能。
   - 执行少量 E2E 测试，完成业务/财务对账，并形成验证报告。
   - 关闭测试控制台或保留在严格受控模式。

## 开放问题

- 是否需要将 `test_session_id` 暴露给第三方 API 调用方（例如作为响应字段），以便外部系统也能按此 ID 检索？
- 是否需要为 `test_session_id` 建立专门的索引，以支持未来可能的批量查询（目前规划为少量内部测试，优先不建索引）。
- 时间线数据是否需要长期存储？
  - 若需要，可考虑单独表或离线导出到对象存储；目前规划为“按需导出 + 日志/DB 保留”。
- Web 测试控制台前端是否集成到现有管理后台，还是独立部署？
  - 独立部署可以降低与生产管理后台的耦合，但需要额外的部署和权限管理策略。