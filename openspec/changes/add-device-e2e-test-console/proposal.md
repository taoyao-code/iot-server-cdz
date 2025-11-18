# 变更: 为真实设备引入端到端测试流程与 Web 测试控制台

## 为什么

本项目即将上线，对外暴露的第三方 API（`/api/v1/third/...`）、设备协议（BKV）、订单状态机以及事件推送已经在代码和文档层面趋于稳定，但目前缺少一套**围绕真实设备、真实第三方回调的系统性端到端测试流程**：

- 现有测试（`go test ./internal/...` + 单元/集成测试）主要验证逻辑正确性，难以覆盖“Redis 会话 + 出站队列 + TCP 设备 + Webhook”这一整条链路。
- 测试/运维缺少一个集中视图来发起测试、观察整条链路上的每一跳数据（HTTP 请求、BKV 帧、DB 记录、事件推送），导致问题排查成本高、风险难以量化。
- 生产环境上线前缺乏标准化的“小流量真实验证流程”，对“资金安全 / 账务一致性 / 设备行为”的信心不足。

因此，需要通过一个规范驱动的变更，将 **内部 Web 测试控制台 + 真实设备 E2E 测试流程** 固化下来，在测试环境和受控的生产环境中，做到“每一次测试都可重复、可追踪、可审计”。

## 变更内容

- 引入一个仅内部使用的 **Web 测试控制台**（HTTP + Web UI）：
  - 后端：在 `internal/api` 下新增 `/internal/test/...` 路由和 handler，复用 `ThirdPartyHandler` 中的 `StartCharge` / `StopCharge` / `GetDevice` / `GetOrder` / `GetOrderEvents` 逻辑，支持：
    - 列出可测试设备（基于 `Repository.ListDevices` + `ListPortsByPhyID` + `SessionManager.IsOnline`）。
    - 查看单设备详情（设备信息 + 端口状态 + 活动订单）。
    - 通过内部接口发起测试充电/停止充电，并自动生成 `test_session_id`。
    - 查询某个 `test_session_id` 对应的**全链路时间线**（orders/cmd_log/outbound_queue/事件等）。
  - 前端：实现设备列表页、设备详情 + 场景选择页、时间线视图和数据导出功能，专为测试/运维人员使用。
- 设计并落地 **测试会话标识 `test_session_id`**：
  - 由 Web 测试控制台后端生成（UUID），在一次 E2E 测试从入口到出口全程透传。
  - 在 HTTP header、DB（`orders.test_session_id` 等）、出站队列（`outbound_queue.correlation_id`）、事件 JSON (`Event.Data["testSessionId"]`) 等处统一记录，用于串联和检索整条链路上的所有数据。
- 补充/强化可观测性：
  - 在 `ThirdPartyHandler.StartCharge` / `StopCharge` / `GetDevice` / `GetOrder` 以及 `RedisWorker.processOne`、事件推送等关键日志中增加 `test_session_id`、`order_no`、`device_phy_id` 等字段。
  - 为测试环境定义与 `test_session_id` 相关的指标和简单看板，帮助观察 E2E 测试结果（如命令发送数、延迟、失败原因等）。
- 定义一套 **真实设备 E2E 测试矩阵** 与执行流程：
  - 在测试环境中，以实体设备为对象，围绕 `StartCharge` / 设备执行 / 订单结算 / Webhook 推送，执行多轮标准化场景（正常、离线、端口占用、断网/中断、Webhook 失败重试等）。
  - 在生产环境中，基于白名单设备和小额金额，执行有限次数的真实 E2E 验证，并通过测试控制台导出链路数据，与业务/财务/第三方共同对账。
- 输出配套文档和 Runbook：
  - 《E2E 测试操作手册》：指导测试/运维如何使用 Web 测试控制台。
  - 《异常排查 Runbook》：基于当前 `ThirdPartyHandler`/`Repository`/`RedisWorker`/`RedisManager` 的行为，给出常见异常场景的排查步骤。
  - 明确测试控制台的配置开关和回滚策略，确保在出现问题时可以快速关闭 `/internal/test/...` 路由，不影响现有对外 API。

## 影响

- 受影响/新增的规范能力（capabilities）：
  - **device-e2e-test-console**（新增）：
    - 内部测试控制台的访问控制、API 能力、时间线视图与数据导出。
    - 测试会话标识 `test_session_id` 的生成与链路透传。
    - 真实设备 + 第三方回调的 E2E 测试矩阵与执行要求。
  - **现有能力的可观测性补充**（在相关 spec 中增量补充，未来需要拆分到对应 capability）：
    - 订单生命周期：将 E2E 测试中的订单状态变化与现有 `orders.status` 状态机对齐。
    - 事件推送：确保 `Event` 结构中包含 E2E 所需的追踪字段，并在测试场景中验证重试和 DLQ 行为。
- 受影响的代码模块：
  - `internal/api/thirdparty_handler.go`：
    - 日志字段扩充（`test_session_id` 等），在内部测试路径中 reuse。
  - `internal/api/routes.go` / `thirdparty_routes.go`：
    - 新增 `/internal/test/...` 路由组及其鉴权策略。
  - `internal/storage/pg/repo.go`、`extra.go`：
    - 如需为 `orders`/`cmd_log`/`outbound_queue` 增加 `test_session_id`/`correlation_id` 等字段，需在此集中维护。
  - `internal/outbound/redis_worker.go`：
    - 日志补充，与 `SessionManager.GetConn` 的行为共同支持 E2E 场景调试。
  - `internal/session/redis_manager.go`：
    - 在 E2E 测试中重点验证 `IsOnline`/`GetConn` 的行为（心跳超时/僵尸连接清理）。
  - `internal/thirdparty/pusher.go` 以及事件相关代码：
    - 在 E2E 场景中验证签名、重试、错误处理和可观测性。
- 风险与注意事项：
  - 内部测试接口与控制台必须严格隔离于对外 API，防止被外部调用误当业务接口使用。
  - 在生产环境中执行 E2E 测试必须使用白名单设备和小额金额，并有严格的审批和对账流程。
  - 新增字段/索引（如 `test_session_id`）需要评估对数据库的影响，并通过 Migratons/回滚计划进行控制。