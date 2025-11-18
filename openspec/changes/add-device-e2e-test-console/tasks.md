## 0. 整体流程校准 & 风险识别（理解阶段）

- [ ] 0.1 依据 `docs/IoT中间件技术规范.md`、`ARCHITECTURE_SUMMARY.md`，校准本次 E2E 测试要覆盖的完整链路：
  - 第三方 → HTTP API（`internal/api/thirdparty_handler.go` 中 `StartCharge`/`StopCharge`/`GetDevice`/`GetOrder`/`GetOrderEvents`）
  - 业务 → Postgres（`internal/storage/pg/repo.go`、`extra.go` 中订单/设备/端口/队列表逻辑）
  - 出站队列 → Redis（`internal/outbound/redis_worker.go` + `internal/storage/redis` OutboundQueue）
  - 会话管理 → Redis 会话（`internal/session/RedisManager` 通过 `SessionManager.IsOnline` / `GetConn`）
  - 协议层 → BKV 帧构造（`StartCharge`/`StopCharge` 内调用 `bkv.Build`、`encodeStartControlPayload`、`encodeStopControlPayload`）
  - 设备实体 → 真实 BKV 设备行为（开始/停止充电、心跳、告警等）
  - 事件推送 → Webhook（`internal/thirdparty/Pusher.SendJSON`、事件队列相关代码）
- [ ] 0.2 阅读并标记以上各处关键代码，形成一张 **“启动充电 E2E” 时序图**（存档在 `docs/` 下），在图上标出：
  - 主要节点：`ThirdPartyHandler.StartCharge` → `Repository.EnsureDevice` / SQL 事务 → `OutboundQueue.Enqueue` → `RedisWorker.processOne` → 设备 → 设备上报处理 → `Repository.*Order*` 更新 → 事件入队 → `thirdparty.Pusher.SendJSON` → 第三方
  - 核心主键字段：`device_phy_id`、`device_id`、`port_no`、`order_no`、`msg_id`、`event_id` 等
- [ ] 0.3 列出当前代码中所有与订单状态流转/异常处理相关的函数清单（至少）：
  - `Repository` 中：`UpsertOrderProgress`、`SettleOrder`、`GetPendingOrderByPort`、`GetChargingOrderByPort`、`CompleteOrderByPort`、`CancelOrderByPort`、`MarkChargingOrdersAsInterrupted`、`RecoverOrder`、`FailOrder`
  - `ThirdPartyHandler` 中：`StartCharge`、`StopCharge`、`CancelOrder`、`GetOrder`、`ListOrders`、`GetOrderEvents`
  - 设备事件处理侧（如有专门 handler）：确认从 BKV 帧到订单状态 的映射位置
- [ ] 0.4 基于上述理解列出本次 E2E 验证要重点防止的错误类型（资金风险、订单卡死、设备状态与订单不一致、事件丢失/重复、Web 测试界面与真实数据不一致等）。

---

## 1. 环境与实体设备准备

- [ ] 1.1 建立环境矩阵文档，列出并确认：本地开发、`docker-compose.test.yml` 测试环境、测试服务器、生产环境的差异：
  - HTTP Base URL、监听端口
  - Postgres / Redis 地址及数据库名
  - Webhook URL 与 HMAC Secret（参考 `docs/api/事件推送规范.md` 与 `thirdparty.Pusher` 的签名算法）
  - `configs/*.yaml` 中与会话、队列、超时相关的配置（如 `session.heartbeat_timeout_sec`、队列优先级、超时秒数）。
- [ ] 1.2 在测试服务器上通过 SSH + 密钥验证：
  - 应用进程部署版本（git commit）、所用配置文件路径
  - 是否已经启用 Redis 会话管理（`RedisManager`）和 Redis 出站队列（`RedisWorker`），确保测试时路径与本地一致。
- [ ] 1.3 制定实体设备测试清单：
  - 从数据库 `devices` 表中导出将用于测试的 `phy_id` 列表，并与现场实体设备核对编号、位置、端口数、固件版本
  - 在业务侧或 DB 中对这些设备做“测试设备”标记（例如单独表或 tag 字段），避免误用生产设备
- [ ] 1.4 与业务/运维确认 **生产环境白名单设备 + 小额测试策略**：
  - 指定少量 `phy_id` 作为生产验证专用
  - 定义单次测试金额上限、每日测试次数上限
  - 明确执行人、审批人和对账责任人。

---

## 2. 数据追踪与可观测性设计（test_session_id）

- [ ] 2.1 设计统一的测试会话标识：
  - 命名：`test_session_id`（字符串，UUID 格式）
  - 生成位置：Web 测试控制台后端（新建内部 handler，见第 3 节），每次点击“发起测试”时生成
- [ ] 2.2 定义 `test_session_id` 的传递与落地点（精确到代码位置）：
  - HTTP 层：
    - 在 Web 控制台发起的内部 API 请求 header 中传递：`X-Test-Session-Id`
    - 在 `ThirdPartyHandler.StartCharge` / `StopCharge` / `GetDevice` / `GetOrder` 中，通过 `c.GetString("request_id")` 同时记录 `test_session_id`（可以存入 `gin.Context` 的值，例如在中间件中注入）
  - DB 层：
    - `orders` 表：为测试增加一列 `test_session_id`（如已有类似字段则对齐）；在 `StartCharge` 插入订单时一并写入
    - `cmd_log` 表：调用 `Repository.InsertCmdLog` 时增加 `test_session_id` 扩展字段（如需要，新增字段 + 扩展函数）
    - `outbound_queue` 表：使用 `Repository.EnqueueOutbox` 时，将 `correlation_id` 字段用于存储 `test_session_id`
  - Redis 队列：
    - `redisstorage.OutboundMessage` 结构中，如果没有 test 字段，则通过 `ID` 或 `Metadata` 扩展传递 `test_session_id`（需查阅 `internal/storage/redis` 具体定义）
  - 事件推送：
    - 在事件入队层（生成 `thirdparty.Event` 或 Outbox 事件时），把 `test_session_id` 放入 `Event.Data["testSessionId"]`
- [ ] 2.3 日志：
  - 在 `ThirdPartyHandler.StartCharge` / `StopCharge` / `GetDevice` / `GetOrder` 中的 `logger.Info` / `Error` 调用里增加 `zap.String("test_session_id", ...)`
  - 在 `RedisWorker.processOne` 中，现有日志已包含 `msg_id` 和 `phy_id`，补充 `test_session_id`（从队列消息中取，如果结构支持）
  - 在 `thirdparty.Pusher.SendJSON` 调用处，构造 payload 时保证事件 JSON 内包含 `testSessionId` 字段，便于第三方系统对齐。

---

## 3. Web 测试控制台 — 后端 API（对齐现有 Handler 结构）

> 目标：在 `internal/api` 中新增一组 **内部测试接口**，复用 `ThirdPartyHandler` 现有逻辑，专门面向测试控制台，不对外开放。

- [ ] 3.1 新增 `internal/api/testconsole_handler.go`（或类似命名）与对应路由：
  - Handler 结构：
    - 包含 `repo *pg.Repository`、`sess session.SessionManager`、`outboundQ *redisstorage.OutboundQueue`、`eventQueue *thirdparty.EventQueue`、`logger *zap.Logger`
    - 允许内部调用 `ThirdPartyHandler` 或直接调用其公共方法
  - 路由注册：
    - 在 `internal/api/routes.go` 或 `thirdparty_routes.go` 中添加 `/internal/test/...` 前缀的路由组，仅在内部环境启用
- [ ] 3.2 API 设计（示例）：
  - `GET /internal/test/devices`：
    - 使用 `Repository.ListDevices` + `ListPortsByPhyID` 查询设备 + 端口快照
    - 使用 `SessionManager.IsOnline` 判断在线状态
  - `GET /internal/test/devices/{phy_id}`：
    - 调用 `Repository.GetDeviceByPhyID` 获取 DB 信息
    - 调用 `Repository.ListPortsByPhyID` 获取端口信息
    - 查询该设备最新的订单（可用 `ListOrdersByPhyID` 或自定义 SQL）
  - `POST /internal/test/devices/{phy_id}/charge`：
    - 在 handler 中生成 `test_session_id`，写入 context 与 header
    - 内部构造 `StartChargeRequest` 并直接调用 `ThirdPartyHandler.StartCharge(c)`，复用其所有校验和 SQL/队列逻辑
  - `POST /internal/test/devices/{phy_id}/stop`：
    - 类似方式构造 `StopChargeRequest`，调用 `ThirdPartyHandler.StopCharge`
  - `GET /internal/test/sessions/{test_session_id}`：
    - 通过 DB 查询和日志聚合构造“全链路时间线”（详见 3.4）
- [ ] 3.3 鉴权与访问控制：
  - 在 `middleware`（`internal/api/middleware`）中增加一个仅在内部启用的认证逻辑：
    - 校验内部 API Key、IP 白名单或用户角色
    - 对 `/internal/test/...` 路径统一应用
  - 在 handler 中记录操作者信息（例如从 JWT / Header 中取用户 ID/名称）
- [ ] 3.4 `GET /internal/test/sessions/{test_session_id}` 的时间线聚合：
  - 通过 Repository 和自定义 SQL 聚合以下数据：
    - `orders` 表中 `test_session_id` 匹配的记录（时间、状态、金额、kWh）
    - `cmd_log` / `outbound_queue` 中与该 session 相关的记录（按 `correlation_id`/`msg_id`）
    - 事件表（如果有独立事件表）或 `GetOrderEvents` 相关逻辑返回的事件列表
  - 构造一个 JSON 响应结构：
    - `events: [ {timestamp, type, source, payloadSummary, rawPayload} ... ]` 按时间排序
    - 尽量使用已有的 `Order`、`Event` 结构体字段名，避免定义重复模型。

---

## 4. Web 测试控制台 — 前端 UI

> 这里假定前端在单独项目或同一仓库中，任务聚焦功能和数据，不约束具体技术栈。

- [ ] 4.1 设备列表页：
  - 调用 `GET /internal/test/devices`，渲染表格：`phy_id`、在线状态、最近心跳时间、当前/最近订单状态
  - 支持按在线状态、地点（如有）、`phy_id` 搜索过滤
- [ ] 4.2 设备详情 & 测试操作页：
  - 展示 `GetDevice` + `GET /internal/test/devices/{phy_id}` 拼合的信息：设备注册时间、在线状态、端口状态（ports.status）、活动订单号（如 `GetDevice` 中 `active_order`）
  - 提供测试场景选择：
    - 正常充电成功
    - 设备离线 / 心跳超时（可通过操作设备/断网模拟）
    - 端口占用（依赖 `StartCharge` 中端口并发检查逻辑）
    - 第三方 Webhook 失败（通过配置错误 URL 或返回非 2xx）
  - 根据场景预填 `StartChargeRequest` 参数，如 `port_no`、`charge_mode`、`amount`、`duration_minutes`
  - 点击“发起测试”后：
    - 立即显示后端生成的 `test_session_id`
    - 轮询 `GET /internal/test/sessions/{test_session_id}` 或使用推送机制更新时间线视图
- [ ] 4.3 全链路时间线视图：
  - 展示来自 3.4 的时间线数据，区分节点来源：API 请求、DB 操作、出站指令、设备上报、事件推送
  - 每个节点可展开查看关键字段（`order_no`、`device_phy_id`、`event_type` 等）及原始 JSON/HEX 摘要
  - 支持“一键导出”当前 `test_session_id` 的 JSON，便于排查和归档。

---

## 5. 端到端测试矩阵（测试环境，有真实设备参与）

- [ ] 5.1 基于 `docs/IoT中间件技术规范.md` 第 4–6 章，整理出 E2E 测试场景矩阵：
  - 正常启动 + 正常结束：
    - `StartCharge` → 设备启动 → 多次上报 → `CompleteOrderByPort` → 事件推送成功
  - 设备离线：
    - `SessionManager.IsOnline` 返回 false，`StartCharge` 直接 503 拒绝创建订单
  - 端口忙/状态不一致：
    - 利用 `verifyPortStatus` + 事务行锁逻辑，验证端口忙时返回 409/特定错误码
  - 充电中手动停止：
    - `StopCharge` 把订单置为 `OrderStatusStopping`，设备执行停止指令后触发订单完成/终态
  - 断网 / 中断：
    - `MarkChargingOrdersAsInterrupted` + `RecoverOrder` 的行为符合文档
  - Webhook 失败重试：
    - 第三方返回 5xx 或超时，`thirdparty.Pusher` 进行多次重试并进入 DLQ（如有）
- [ ] 5.2 为每个场景定义精确检查点（核对“每一个数据”）：
  - HTTP 层：请求和响应 JSON 字段是否符合 `docs/api/第三方API文档.md` 中的 `code/message/data` 定义
  - DB 层：
    - `orders` 表中 `status`、`start_time`、`end_time`、`kwh_0p01`、`amount_cent` 与设备行为和第三方的金额一致
    - `ports` 表中端口状态与实际设备插座状态一致
  - 队列层：
    - `outbound_queue` 中对应 `cmd` 和 `payload` 是否正确反映 BKV 控制帧
  - 事件层：
    - 推送到第三方的事件 JSON (`Event`) 中 `devicePhyId`、`data.order_no`、`data.total_amount`、`data.total_kwh` 是否与 DB/设备一致
- [ ] 5.3 实际执行测试：
  - 每个场景在测试环境中执行至少 3 轮，记录对应的 `test_session_id`
  - 通过 Web 测试控制台导出每轮的时间线 JSON，与 DB / 设备 / 第三方日志逐项对比
  - 记录所有差异和异常行为，按“文档问题 / 代码 bug / 配置问题 / 外部系统问题”分类。

---

## 6. 生产环境小流量验证（上线前 Gate）

- [ ] 6.1 制定《生产环境 E2E 验证方案》并归档：
  - 使用前面确定的白名单设备
  - 每台设备执行的测试次数与金额
  - 测试的时间窗口（避开高峰）、执行人员名单
- [ ] 6.2 在生产环境启用 **受限版** 测试控制台：
  - 仅开放“正常充电成功”场景
  - `/internal/test/...` 路由启用更严格的鉴权（例如只允许特定子网 + 特定用户）
- [ ] 6.3 执行生产小流量 E2E 测试：
  - 针对每个白名单设备执行 1–2 笔小额测试，记录 `test_session_id`
  - 使用测试控制台导出链路数据，由业务/财务对账第三方系统与本系统的所有金额/时长/电量
  - 将对账结果和任何差异记录到“生产 E2E 验证报告”中，作为上线前的正式凭证。

---

## 7. 文档、Runbook 与回滚策略

- [ ] 7.1 编写《E2E 测试操作手册》（放到 `docs/`）：
  - 面向测试/运维，说明如何使用 Web 测试控制台、如何发起测试、如何查看时间线与导出数据
- [ ] 7.2 编写《异常排查 Runbook》：
  - 当 `StartCharge` 返回特定错误（503/409/40001 等）时，如何根据日志和 DB 状态排查
  - 当设备长时间不回报 / 中断时，如何查看 `MarkChargingOrdersAsInterrupted` 等相关逻辑
  - 当 Webhook 长时间失败时，如何定位事件队列和 DLQ，并触发补偿
- [ ] 7.3 回滚策略：
  - 为 Web 测试控制台提供显式配置开关（如环境变量或 config），可在出现问题时立即关闭 `/internal/test/...` 路由
  - 确保关闭测试控制台不影响现有第三方 API（`/api/v1/third/...`）的正常运行。

---

## 8. 验证与验收

- [ ] 8.1 确认所有设计的 E2E 场景在测试环境至少执行一轮完整测试，并有导出的 JSON 数据作为证明
- [ ] 8.2 确保自动化测试（`go test ./internal/...`，尤其是 `internal/storage/pg`、`internal/api`、`internal/thirdparty`）全部通过，覆盖率不低于 `TESTING.md` 中定义的 CI 阈值
- [ ] 8.3 由技术负责人 + 业务负责人 + 运维负责人共同签字/确认测试结果与风险评估，作为本次变更上线的最终 Gate。