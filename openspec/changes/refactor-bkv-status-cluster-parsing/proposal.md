# Change: 重构 BKV 状态集群解析以符合协议规范

## Why

当前 BKV 状态集群 (0x1017) 解析实现与协议文档 `docs/协议/设备对接指引-组网设备2024(1).txt` 存在严重不一致：

1. **TLV 标签包装格式不匹配**：协议使用 `0x03/0x04` 前缀包装字段标签，但现有代码假设标签直接出现
2. **字段提取失败**：插座序号、版本、温度、RSSI、端口属性等关键数据无法正确解析
3. **端口状态缺失**：0x5B 端口属性容器内的电压、功率、电流、用电量等字段全部丢失

这导致：
- 设备状态监控数据不完整
- 充电会话无法正确关联端口状态
- 平台无法获取实时功率/电流/电压读数

## What Changes

### 核心变更

- **新增** `ParseBKVStatusCluster` 专用函数，正确解析 0x03/0x04 前缀 TLV 格式
- **新增** BKV TLV 标签常量定义，与协议文档完全对齐
- **修改** `GetSocketStatus` 使用新解析函数
- **修改** `HandleBKVStatus` 处理器使用重构后的状态结构

### 影响范围

- `internal/protocol/bkv/tlv.go` - TLV 解析核心逻辑
- `internal/protocol/bkv/handlers.go` - 状态处理器
- `internal/protocol/bkv/wire.go` - 帧解析（可能涉及）

## Impact

- **Affected specs**: bkv-protocol (新建)
- **Affected code**:
  - `internal/protocol/bkv/tlv.go:GetSocketStatus`
  - `internal/protocol/bkv/tlv.go:parseSocketStatusFields`
  - `internal/protocol/bkv/tlv.go:parsePortsFromFields`
  - `internal/protocol/bkv/handlers.go:HandleBKVStatus`
- **Breaking changes**:
  - `SocketStatus` 结构体字段可能调整
  - 旧的解析函数标记为 deprecated
- **Migration**:
  - 上层业务逻辑使用标准化的 `SocketStatus` 接口，无需修改
  - 日志输出格式可能变化

## 协议文档参考

协议示例报文 (2.2.3 插座状态上报)：
```
fcfe0091100000000000018223121400270004010110170a01020000000000000000090103822312
1400270065019403014a0104013effff030107250301961e28015b030108000301098004010a0000040
19508e304010b000004010c000104010d000004010e000028015b030108010301098004010a00000401
9508e304010b000004010c000104010d000004010e000030fcee
```

关键字段映射：
| 协议标签 | 含义 | 当前状态 |
|----------|------|----------|
| 0x4A | 插座序号 | ❌ 解析失败 |
| 0x3E | 插座版本 | ❌ 解析失败 |
| 0x07 | 温度 | ❌ 解析失败 |
| 0x96 | RSSI | ❌ 解析失败 |
| 0x5B | 端口属性容器 | ❌ 解析失败 |
| 0x08 | 插孔号 | ❌ 解析失败 |
| 0x09 | 插座状态 | ❌ 解析失败 |
| 0x0A | 业务号 | ❌ 解析失败 |
| 0x95 | 电压 | ❌ 解析失败 |
| 0x0B | 功率 | ❌ 解析失败 |
| 0x0C | 电流 | ❌ 解析失败 |
| 0x0D | 用电量 | ❌ 解析失败 |
| 0x0E | 充电时间 | ❌ 解析失败 |
