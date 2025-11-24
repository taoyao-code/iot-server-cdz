# Change: 明确 BKV 控制链路的插座标识（UID/MAC）映射与下发策略

## 元数据
- **变更ID**: align-bkv-control-socket
- **提案人**: AI Assistant
- **创建日期**: 2025-11-24
- **状态**: 待批准
- **优先级**: 高
- **关联能力**: bkv-device-control

## 背景与问题
- 控制入口只接收 `port_no`。`internal/api/thirdparty_handler.go` 将 `req.PortNo` 直接下传，BKV 下行 `socket_no` 固定 0（`EncodeStart/StopControlPayload`），导致 socket/port 混用且无法定位真实插座。
- 组网/状态上报 (`HandleNetworkRefresh/NetworkAddNode` 等) 仅推送事件，没有落库 UID↔MAC↔socket_no→gateway 映射，控制链路缺少权威来源，缺失时静默写 0。
- 缺少诊断信号：调用方拿不到 UID 级错误提示，日志也缺少 socket_uid/socket_no，排障困难。
- 测试控制台/前端沿用 port 号输入，未暴露 UID，进一步放大概念混淆。

## 目标（What）
- API/控制台必须显式传入 `socket_uid`（字符串） + `port_no`（0 基），不再做隐式 +1/-1 假设；错误码包含 UID。
- 建立并持久化 `socket_uid` ↔ `socket_mac` ↔ `socket_no` ↔ `gateway_id` 映射，来源于组网刷新/ACK 或状态上报。
- BKV 下行（Start/Stop/Query 等）统一用映射获得的 `socket_no`，`port` 字段沿用请求的 `port_no`（0 基）；映射缺失时拒绝并返回可诊断错误。
- 日志/监控记录映射写入、缺失以及下发时的 socket_no/port_no，便于追踪。
- 控制台文案提示 socket_no 自动解析，仅输入 UID + port_no。

## 范围
- API：`internal/api/thirdparty_handler.go`、测试控制台 handler。
- 协议驱动：`internal/protocol/bkv/command_source.go`、组网/状态上报处理。
- 数据层：新增/扩展插座映射存储与查询接口。
- 前端：web/static 控制台字段与提示。
- 测试：控制链路基于 UID 的正/逆向用例。

## 非目标
- 不改动计费/订单流程，只校准控制参数来源。
- 不变更 BKV 帧结构、CRC、加密等协议细节。
- 不引入新的协议或网关管理功能。

## 方案概要
1) 数据模型与存储  
   - 建立插座映射实体：`socket_uid`(pk)、`socket_mac`、`socket_no`(uint8)、`gateway_id`，必要时保留 port 范围字段。  
   - 提供 upsert/查询接口，组网刷新/ACK 触发写入，按 UID 查 socket_no + MAC。

2) API/前端  
   - Start/Stop 请求结构改为必填 `socket_uid` + `port_no`(0 基)；校验 UID 格式并在错误响应中回显。  
   - 测试控制台表单同步调整，默认 port_no=0，提示 socket_no 自动解析。

3) 控制链路  
   - 下发前查询 UID 映射；缺失则返回可诊断错误，不入队。  
   - BKV command source 使用映射 socket_no，`Encode*Payload` 仅接受 0 基 port，无隐式偏移。  
   - 查询(0x001D) 等命令复用同一映射策略，避免 socket_no 漏填。

4) 数据同步与校验  
   - `HandleNetworkRefresh/NetworkAddNode` 解析到 UID/MAC/socket_no 后写入映射并关联 gateway_id。  
   - 如状态/异常上报包含 socket_no/UID，补充映射或校验一致性。

5) 日志与监控  
   - 控制下发日志包含 device_id、socket_uid、socket_no、port_no、business_no。  
   - 映射缺失/写入计数暴露为监控指标或可检索日志。

## 验收标准
- ✅ 传入已知 `socket_uid + port_no` 可成功下发，BKV 帧中 socket 字段等于映射 socket_no，port 等于请求值。  
- ✅ 未知/缺失 UID 时 API 返回带 UID 的错误码与信息，未写入下行队列。  
- ✅ 组网刷新/ACK 产生的 UID/MAC/socket_no 可在存储层查询到，并能被后续控制复用。  
- ✅ 查询(0x001D) 等命令使用同一 socket_no 规则，日志可见映射详情。  
- ✅ 至少一条基于 UID 的成功/失败自动化测试覆盖控制链路。

## 风险与缓解
- 映射缺失导致控制拒绝：提供清晰错误与日志，补全映射后可重试。  
- 存量调用方仍按旧字段调用：在 handler 中返回迁移提示，必要时提供灰度开关。  
- 映射数据漂移：使用 upsert + gateway_id 约束，异常上报时记录告警并支持运维修复。

## 时间线
- 方案审阅通过后 0.5 天内完成数据模型与 handler 设计定稿  
- 1 天完成 API/驱动/映射写入实现及单元测试  
- 0.5 天完成前端调整与联调验证
