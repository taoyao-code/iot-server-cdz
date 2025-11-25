## ADDED Requirements
### Requirement: BKV driver SHALL NOT persist port snapshots or orders on charging end
BKV 驱动在收到 0x0015/0x1004 充电结束报文时，SHALL 仅发出规范化 `SessionEnded` 事件，并使用事件中的 `NextPortStatus` 驱动端口收敛；SHALL NOT 直接写入 `ports`/`orders` 表，也不得插入 fallback 订单。

#### Scenario: Charging end emits SessionEnded only
- **WHEN** 驱动解析到充电结束报文（含业务号、端口、状态）
- **THEN** 驱动只发 `SessionEnded` 事件，端口状态不经过驱动落库，端口最终收敛由核心处理的 `NextPortStatus` 完成
- **AND** 若业务号匹配失败，核心记录错误但驱动不造单、不写库。

### Requirement: Driver SHALL avoid duplicate or conflicting port writes
驱动在充电结束、控制 ACK、异常 ACK 等路径中 SHALL NOT 写入端口状态，避免 0xB0→0x90→0xB0 抖动或错误端口（如 ACK port=1）被写入。

#### Scenario: No port snapshot on control ACK
- **WHEN** 驱动处理控制 ACK/充电结束 ACK
- **THEN** 不写入 `PortSnapshot`，仅保留日志/事件，端口状态由状态上报或 SessionEnded 决定。

### Requirement: Settlement MUST not create fallback orders
结算逻辑 MUST 仅更新已存在且匹配的业务号/订单号，若未匹配到 SHALL 返回错误并记录诊断，不得插入“order_no=业务号”或其他幽灵订单。

#### Scenario: Settlement without fallback insert
- **WHEN** 结算时未找到匹配业务号/订单号
- **THEN** 返回错误并记录日志
- **AND** 不插入/更新任何新订单行。

### Requirement: Single storage path for driver-core boundary
驱动层 SHALL NOT 直接依赖多套仓储实现（gormrepo/pg 双写），必须通过统一的事件/命令边界与核心交互，核心负责持久化。

#### Scenario: Driver emits events without DB writes
- **WHEN** 驱动需要更新设备/端口/会话状态
- **THEN** 仅通过标准事件（DeviceHeartbeat/PortSnapshot/SessionStarted/SessionProgress/SessionEnded/ExceptionReported）通知核心
- **AND** 不调用任何 DB 写接口（EnsureDevice/TouchDeviceLastSeen/UpsertPort/SettleOrder 等）。

### Requirement: Missing settlement MUST log errors without side-effects
当结算未匹配到业务号/订单号时，系统 MUST 记录错误日志并保持现有数据不变，不得新建/覆盖端口或订单。

#### Scenario: Settlement failure logs only
- **WHEN** 结算请求未匹配到任何订单
- **THEN** 记录包含 device_id/port_no/business_no 的错误日志
- **AND** 不写入 ports/orders/事件队列，仅返回错误。
