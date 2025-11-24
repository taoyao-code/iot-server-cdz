## ADDED Requirements

### Requirement: BKV 控制命令必须通过 UID 映射填充插座编号
系统 SHALL 在下发 BKV 开始/停止充电或查询命令前，使用插座 UID 查找到对应的 MAC 与 socket_no（1 字节），并将 socket 字段填为该 socket_no，port 字段使用请求的 port_no（0 基）。

#### Scenario: 通过 UID 成功下发控制
- GIVEN 平台已存在 UID=301015011402415 与 MAC=854121800889，socket_no=1 的映射
- WHEN 调用开始充电 API，参数包含 `socket_uid=301015011402415` 且 `port_no=0`
- THEN 系统 SHALL 构造 BKV 控制帧，socket 字段=1，port 字段=0，并成功写入下行队列

#### Scenario: 映射缺失时报错
- GIVEN 平台不存在该 UID 的 socket 映射
- WHEN 调用开始充电 API，参数包含未知 `socket_uid`
- THEN 系统 SHALL 拒绝请求，返回可诊断的错误码/信息（含 UID），且不会下发控制命令

### Requirement: 组网/上报须写入 UID↔MAC↔socket 映射
系统 SHALL 在收到组网 ACK 或状态上报中携带的插座标识时，更新（插座 UID, MAC, socket_no, gateway_id）的映射，用于后续控制。

#### Scenario: 组网 ACK 写入映射
- GIVEN 平台已知网关 ID，收到包含插座 UID 与 MAC、socket_no 的组网 ACK/上报
- THEN 系统 SHALL 将 UID↔MAC↔socket_no 映射写入存储（含 gateway_id），可被后续控制查询
