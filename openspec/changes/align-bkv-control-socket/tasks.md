## 1. 规范/模型
- [ ] 明确定义插座映射数据模型：`socket_uid`(pk)、`socket_mac`、`socket_no`(uint8)、`gateway_id`，必要时附带 `port_range` 说明；记录存储位置（DB/缓存）与索引。
- [ ] 设计映射读写接口（upsert + 按 UID 查询），确定所属包与依赖，避免跨层耦合。
- [ ] 确认组网/上报帧中 socket_no、UID、MAC 的解析方式与合法范围。

## 2. API / 前端
- [ ] 更新 Start/Stop API 请求体为必填 `socket_uid`(string) + `port_no`(0基)，增加格式校验、错误码与响应文案（包含 UID）。
- [ ] 调整控制台/前端表单：输入或选择 `socket_uid`，port_no 默认 0，提示 socket_no 自动解析。
- [ ] 补充 API swagger/注释示例，体现新字段与错误返回。

## 3. 控制链路
- [ ] 控制下发前按 UID 查询映射，缺失时短路返回可诊断错误（包含 UID）；日志打印 device_id/socket_uid/socket_no/port_no。
- [ ] BKV 下行编码（Start/Stop/Query 等）统一使用映射的 `socket_no`，`port` 使用请求的 0 基值，移除隐式 +1/-1。
- [ ] 核查 driver command source 及相关调用点，保证 socket_no 传递一致。

## 4. 数据同步
- [ ] 在组网刷新/ACK 解析到 UID/MAC/socket_no/gateway_id 后写入映射（upsert），失败时告警并含原始报文信息。
- [ ] 状态/异常上报若包含 socket_no/UID，补充映射或校验一致性，避免漂移。
- [ ] 增加监控/日志：映射写入次数、缺失次数、控制下发 socket_no/port_no。

## 5. 验证
- [ ] 基于真实/模拟日志覆盖：组网成功、映射缺失报错、正常开始/停止充电。
- [ ] 自动化测试：至少一条成功（有映射）和一条失败（无映射/冲突）的控制链路用例，校验 socket_no/port_no 编码。
