## ADDED Requirements

### Requirement: Internal E2E test console access control

系统 MUST 提供一个仅限内部测试/运维人员访问的 Web 测试控制台，用于在测试环境和受控的生产环境中，对真实设备执行端到端测试，不得对第三方业务方公开。

#### Scenario: Authorized tester can access test console

- **WHEN** 内部测试/运维人员使用具备测试权限的账号访问 `/internal/test/...` 路由
- **THEN** HTTP 层 MUST 通过内部鉴权（如内部 API Key、IP 白名单或 RBAC 角色）验证其身份
- **AND** 在通过验证后 MUST 允许访问设备列表、设备详情、测试会话时间线等页面

#### Scenario: Unauthorized access is rejected and audited

- **WHEN** 未授权用户或外部系统尝试访问 `/internal/test/...` 路由
- **THEN** 系统 MUST 返回 401 或 403，并拒绝任何设备控制或数据查询
- **AND** MUST 记录审计日志，包括访问路径、来源 IP、认证结果和时间戳，以便后续追踪

---

### Requirement: Test device inventory and status overview

系统 MUST 在测试控制台中展示可用于 E2E 测试的设备清单，并为每台设备展示关键状态信息，帮助测试人员选择合适的实体设备。

#### Scenario: Tester views test device list with real-time status

- **WHEN** 测试人员通过 Web 测试控制台访问设备列表
- **THEN** 系统 MUST 查询 `devices` / `ports` 表以及会话管理器（`SessionManager.IsOnline`），返回至少包含以下字段的列表：
  - `device_phy_id`（设备物理 ID）
  - 数据库中的 `device_id`（内部自增 ID）
  - 当前在线状态（online/offline）
  - 最近心跳时间（来自 `TouchDeviceLastSeen` 或 Redis 会话）
  - 每个端口的状态（来自 `ports.status`）
- **AND** 列表 MUST 支持按设备 ID、在线状态、位置（如有）进行过滤与搜索

#### Scenario: Tester views device details and last active order

- **WHEN** 测试人员在列表中选择某个设备查看详情
- **THEN** 系统 MUST 展示：
  - 设备的注册时间、最近在线时间
  - 所有端口的当前状态和功率（如 `ListPortsByPhyID` 所示）
  - 最近一笔活动订单（如有）：订单号、端口号、状态、开始/结束时间等，来源于 `orders` 表

---

### Requirement: Test session identification and end-to-end traceability

系统 MUST 为每一次端到端测试生成唯一的测试会话标识 `test_session_id`，并在“第三方 API → 出站队列 → 设备 → 订单 → 事件推送”的全链路中保持可追踪性。

#### Scenario: Test session id is generated on test request

- **WHEN** 测试人员在 Web 测试控制台针对某设备发起一次新的端到端测试（例如通过内部 `POST /internal/test/devices/{phy_id}/charge`）
- **THEN** 测试控制台后端 MUST 生成一个全局唯一的 `test_session_id`（例如 UUID）
- **AND** MUST：
  - 在内部 HTTP 请求上下文中保存该标识，并通过 header（如 `X-Test-Session-Id`）传递到后续 handler
  - 在调用 `ThirdPartyHandler.StartCharge` 时，将 `test_session_id` 传入日志上下文

#### Scenario: Test session id is persisted across all layers

- **WHEN** 一次测试会话从第三方 API 或内部测试接口开始，到设备执行、订单结算、事件推送结束
- **THEN** 系统 MUST 在以下存储中持久化 `test_session_id` 或等效的相关字段：
  - `orders` 表：对应订单行上 MUST 记录 `test_session_id`（通过 schema 扩展或映射到已有字段）
  - 出站队列表：`outbound_queue.correlation_id` MUST 用于记录该测试会话标识
  - 指令日志表 `cmd_log`（如启用）：对应记录中 SHOULD 包含 `test_session_id`（直接字段或可解析的编码）
  - 事件推送：在推送给第三方的事件 JSON 中，数据部分 MUST 包含 `testSessionId` 字段，其值与订单/队列中的标识一致
- **AND** 系统 MUST 提供按 `test_session_id` 查询测试会话的接口（见后续时间线视图要求）。

---

### Requirement: E2E test console APIs for devices and sessions

系统 MUST 通过内部 HTTP API 提供对测试设备和测试会话的查询与控制能力，基于当前实现的 `ThirdPartyHandler` 与 `Repository` 复用业务逻辑，而不是绕过核心流程。

#### Scenario: Internal test APIs reuse third-party handler logic

- **WHEN** 内部测试控制台需要发起一次“启动充电”测试
- **THEN** 内部 handler MUST 使用与第三方调用路径一致的业务逻辑：
  - 通过构造 `StartChargeRequest` 调用 `ThirdPartyHandler.StartCharge`
  - 复用 `EnsureDevice`、设备在线检查（`SessionManager.IsOnline`）、端口状态检查、事务 + 行锁创建订单的逻辑
  - 复用 BKV 控制帧构造与 `OutboundQueue.Enqueue` 下发指令的逻辑
- **AND** 系统 MUST NOT 为测试控制台实现一套绕开订单/队列的“快捷通道”，以避免测试路径与真实业务路径不一致。

#### Scenario: Tester can query device and session via internal APIs

- **WHEN** 测试人员通过 Web 控制台调用：
  - `GET /internal/test/devices`
  - `GET /internal/test/devices/{phy_id}`
  - `GET /internal/test/sessions/{test_session_id}`
- **THEN** 系统 MUST：
  - 返回由 `Repository.ListDevices` / `GetDeviceByPhyID` / `ListPortsByPhyID` 组合而成的设备视图
  - 返回包含订单、出站队列、指令日志、事件推送信息的统一 session 视图，便于在单个页面上查看该测试会话的全链路数据

---

### Requirement: End-to-end timeline view and data export

系统 MUST 为每次测试会话提供按时间排序的“全链路时间线”视图，并支持将完整数据导出，以便用于问题分析和上线决策。

#### Scenario: Tester inspects E2E timeline for a test_session_id

- **WHEN** 测试人员在 Web 测试控制台中通过 `test_session_id` 打开某次测试详情
- **THEN** 系统 MUST 通过聚合数据库和队列数据，构建按时间排序的事件序列，包括但不限于：
  - 测试控制台或第三方发起的 HTTP 请求与响应（摘要信息，如 URL、状态码、关键字段）
  - 对应订单在 `orders` 表中的创建、状态变更（pending/charging/completed/…）
  - 向设备下发的 BKV 控制帧（`cmd` 编号、payload 十六进制摘要、`msg_id`）
  - 设备心跳、状态上报或告警导致的 DB 更新记录（关联 `device_id`、`port_no`）
  - 推送给第三方的事件 JSON 及其响应（状态码、body 摘要）
- **AND** 系统 MUST 以统一的结构化 JSON 格式提供这些事件，前端可以直接渲染为时间线。

#### Scenario: Tester exports full E2E data for audit

- **WHEN** 测试人员点击“导出”按钮
- **THEN** 系统 MUST 导出该 `test_session_id` 相关的完整结构化数据（例如 JSON 文件），包含所有时间线节点的原始字段
- **AND** 在导出前 MUST 对任何敏感信息（如用户标识、卡号等）进行合理脱敏
- **AND** 导出结果 MUST 足以支持测试/运维/业务/财务联合审阅和对账。

---

### Requirement: Standard E2E test scenarios on real devices

系统 MUST 在测试环境中，基于真实实体设备执行一组标准化的端到端测试场景，覆盖正常与常见异常路径，并将执行结果记录在案。

#### Scenario: Happy-path charging E2E test

- **WHEN** 测试人员在测试环境中：
  - 通过设备列表选择一台在线且被标记为“可测试”的设备
  - 通过 Web 测试控制台选择“正常充电成功”场景，配置测试参数（端口号、充电模式、金额/时长等）
  - 发起测试并获得 `test_session_id`
- **THEN** 系统 MUST：
  - 成功通过 `ThirdPartyHandler.StartCharge` 创建订单，并在 `orders` 表中记录为 pending/charging
  - 通过 `OutboundQueue` 和 `RedisWorker` 成功向目标设备发送 BKV 控制命令，设备开始充电
  - 在充电结束后，正确更新订单为 completed，并计算 kWh/金额等字段
  - 将订单完成事件推送给第三方，第三方返回成功
- **AND** 测试控制台 MUST 展示上述全过程中的关键数据和状态，且这些数据在 DB 与第三方系统中完全一致。

#### Scenario: Device offline or port busy is handled safely

- **WHEN** 测试人员在设备离线或端口已占用的条件下发起测试
- **THEN** 系统 MUST：
  - 在设备离线时，通过 `SessionManager.IsOnline` 检测并拒绝创建订单（返回明确错误码和信息），不产生 new pending 订单
  - 在端口忙或状态不一致时，通过 `verifyPortStatus` 和事务行锁逻辑及时发现，并返回 409/业务错误码，同时不创建新订单
- **AND** 测试控制台 MUST 在时间线和导出数据中清晰标明拒绝的原因和相关状态。

#### Scenario: Webhook failures and retries are observable

- **WHEN** 在某次测试中，第三方回调接口故意返回 5xx 或模拟网络错误
- **THEN** 系统 MUST：
  - 按 `Pusher` 定义的重试策略（最大重试次数 + Backoff）进行多次重试
  - 在重试仍失败时，将事件标记为失败并记录在死信队列（如有实现），或通过 `GetOrderEvents` 等兜底接口可查询到失败原因
- **AND** 测试控制台 MUST 在时间线视图中展示每次 Webhook 尝试的结果（状态码、错误信息），以便测试/运维判断问题所在。

---

### Requirement: Controlled production E2E verification

系统 MUST 支持在生产环境中进行受控的小流量 E2E 验证，以在真实环境下验证“设备 ↔ 中间件 ↔ 第三方”的整体稳定性和数据一致性，同时严格控制风险。

#### Scenario: Production E2E tests on whitelisted devices

- **WHEN** 业务/运维根据事先约定的方案，在生产环境中选取一批白名单设备和小额测试场景，通过测试控制台执行 E2E 测试
- **THEN** 系统 MUST：
  - 仅允许对预设的白名单设备执行测试场景
  - 按每次测试的 `test_session_id` 记录完整链路数据，并允许导出
  - 不因测试功能影响非白名单设备或正常业务流量
- **AND** 测试结束后，相关测试订单和第三方账务记录 MUST 能够通过对账验证一致性，并形成书面验证报告，作为上线 Gate 的一部分。