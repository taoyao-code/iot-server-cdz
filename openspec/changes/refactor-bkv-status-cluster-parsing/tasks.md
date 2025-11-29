# Tasks: 重构 BKV 状态集群解析

## 1. 协议分析与设计

- [ ] 1.1 完整解析协议文档中的 0x1017 状态集群格式
- [ ] 1.2 整理 TLV 标签定义表（0x03/0x04 前缀规则）
- [ ] 1.3 绘制状态集群数据结构图
- [ ] 1.4 设计新的 `SocketStatus`/`PortStatus` 结构体

## 2. TLV 解析核心实现

- [ ] 2.1 新增 BKV TLV 标签常量 (`internal/protocol/bkv/tlv.go`)
  - `TagCommand = 0x01`
  - `TagFrameSeq = 0x02`
  - `TagGatewayID = 0x03`
  - `TagStatusCluster = 0x94`
  - `TagSocketNo = 0x4A`
  - `TagSoftwareVer = 0x3E`
  - `TagTemperature = 0x07`
  - `TagRSSI = 0x96`
  - `TagPortAttr = 0x5B`
  - `TagPortNo = 0x08`
  - `TagPortStatus = 0x09`
  - `TagBusinessNo = 0x0A`
  - `TagVoltage = 0x95`
  - `TagPower = 0x0B`
  - `TagCurrent = 0x0C`
  - `TagEnergy = 0x0D`
  - `TagChargingTime = 0x0E`

- [ ] 2.2 实现 `ParseBKVStatusCluster(raw []byte) (*SocketStatus, error)`
  - 处理 0x03/0x04 前缀包装
  - 识别 0x65/0x94 状态集群标识
  - 逐字段解析插座属性
  - 解析 0x5B 端口属性容器

- [ ] 2.3 实现端口属性子解析器 `parsePortAttributes(data []byte) (*PortStatus, error)`
  - 提取 0x08 插孔号
  - 提取 0x09 状态位
  - 提取 0x0A 业务号
  - 提取 0x95 电压
  - 提取 0x0B/0x0C/0x0D/0x0E 功率/电流/用电量/时间

## 3. 处理器适配

- [ ] 3.1 更新 `GetSocketStatus` 优先调用 `ParseBKVStatusCluster`
- [ ] 3.2 更新 `HandleBKVStatus` 使用新状态结构
- [ ] 3.3 标记旧解析函数为 `@deprecated`
  - `parseSocketStatusFields`
  - `parsePortsFromFields`

## 4. 单元测试

- [ ] 4.1 新增协议示例报文解析测试
  - 验证插座字段：SocketNo=1, SoftwareVer=0xFFFF, Temperature=0x25, RSSI=0x1E
  - 验证端口A：PortNo=0, Status=0x80, Voltage=0x08E3, Power=0, Current=0
  - 验证端口B：PortNo=1, Status=0x80, Voltage=0x08E3

- [ ] 4.2 边界条件测试
  - 数据截断处理
  - 未知标签跳过
  - 多端口解析

- [ ] 4.3 回归测试
  - 确保现有充电流程测试通过
  - 确保心跳/控制命令不受影响

## 5. 集成验证

- [ ] 5.1 使用真实设备报文验证
- [ ] 5.2 确认状态上报事件包含完整字段
- [ ] 5.3 更新日志格式（如需要）

## 6. 文档更新

- [ ] 6.1 更新代码注释，标注协议文档引用
- [ ] 6.2 补充 TLV 格式说明到 README（可选）
