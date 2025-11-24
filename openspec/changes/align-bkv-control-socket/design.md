# Align BKV Control Socket - Design

## 背景
- 控制入口仅传 `port_no`，BKV 下行 socket 字段固定 0，端口/插座混淆导致命令可能写错插座。
- 组网刷新/ACK 已包含 UID/MAC/socket_no，但未落库，控制链路缺少权威映射。
- 日志与错误反馈未包含 socket_uid/socket_no，诊断困难。

## 目标
- 以插座 UID 作为控制入口唯一标识，通过映射获得 socket_no/MAC/gateway_id。
- 下行命令统一填充映射 socket_no，port 字段沿用请求的 0 基 port_no。
- 映射缺失时明确拒绝并返回含 UID 的错误；日志可追踪 socket_uid/socket_no/port_no。

## 非目标
- 不改协议帧结构和 CRC/加密。
- 不调整计费/订单状态机，仅校准控制参数来源。
- 不引入新网关/设备管理功能。

## 数据模型与存储
- 实体：`SocketMapping`
  - `socket_uid` (string, pk)
  - `socket_mac` (string, optional 用于校验/诊断)
  - `socket_no` (uint8, 1-250)
  - `gateway_id` (string, 设备物理 ID)
  - `updated_at` (timestamp)
- 存储：首选 DB 表（便于持久和索引），如需缓存可增加二级缓存但不改变单一真相。
- 接口：
  - `UpsertSocketMapping(ctx, uid, mac, socketNo, gatewayID) error`
  - `GetSocketMappingByUID(ctx, uid) (*SocketMapping, error)`
  - 可选：`ListMappingsByGateway(ctx, gatewayID)` 用于排查。

## 控制链路改造
- API 层：Start/Stop 请求体要求 `socket_uid` + `port_no`(0 基)，校验 UID 非空/长度；错误响应包含 UID。
- 控制分发：下发前查询映射；缺失/冲突时返回业务可诊断错误，不入队。
- BKV CommandSource：
  - Start/Stop/Query 统一填入映射 `socket_no`；`port` 使用请求 0 基值，不做 +1/-1。
  - 记录日志字段：device_id, socket_uid, socket_no, port_no, business_no。

## 数据同步
- 组网刷新/ACK：解析 UID/MAC/socket_no/gateway_id，执行 upsert；异常时记录 raw payload。
- 状态/异常上报：若含 socket_no/UID，则用于补全或校验映射（不覆盖冲突，记录警告）。

## 监控与日志
- 日志键：`device_id`、`socket_uid`、`socket_no`、`port_no`、`business_no`、`gateway_id`。
- 计数指标：`socket_mapping_upsert_total`、`socket_mapping_missing_total`、`bkv_control_dispatch_total{result}`。

## 风险与缓解
- 映射缺失导致控制失败：错误返回带 UID，提示补充映射后重试。
- 映射漂移（socket_no 变化）：上报/组网 upsert 覆盖，同步记录来源与时间戳供排查。
- 兼容旧调用方：错误提示迁移到 `socket_uid + port_no`，必要时提供短期兼容参数开关（默认关闭，限制范围）。
