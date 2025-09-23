# 组网设备(GN)协议架构设计

## 概述

GN协议是针对"组网设备"的通信协议实现，支持心跳管理、状态上报、控制指令和可靠消息传输。

## 架构组件

### 1. 协议层 (`internal/protocol/gn/`)

#### 帧格式 (`wire.go`)

- **帧结构**: `fcfe/fcff + len(2) + cmd(2) + seq(4) + dir(1) + gwid(7) + payload + checksum(1) + fcee`
- **校验和**: 累加模256，从长度字段开始计算
- **方向**: 0x01=上行(设备→服务器), 0x00=下行(服务器→设备)

#### TLV解析 (`tlv.go`)

- **格式**: Tag(1字节) + Length(1字节) + Value(N字节)
- **支持类型**: uint8, uint16, uint32, string, bytes
- **嵌套支持**: 插孔属性等复杂结构

#### 命令路由 (`router.go`)

- **A1 (0x0000)**: 心跳/时间同步
- **A3 (0x1000)**: 插座状态上报
- **A4 (0x1001)**: 状态查询
- **C1 (0x2000)**: 控制指令
- **C2 (0x2001)**: 结束上报
- **E2 (0x3000)**: 参数设置
- **E3 (0x3001)**: 参数查询
- **F1 (0x4000)**: 异常上报

### 2. 存储层 (`internal/storage/gn/`)

#### 数据模型

```sql
-- 设备表
devices (device_id, gateway_id, iccid, rssi, fw_ver, last_seen_at)

-- 端口表
ports (device_id, port_no, status_bits, biz_no, voltage, current, power, energy, duration)

-- 入站日志
inbound_logs (device_id, cmd, seq, payload_hex, parsed_ok, reason)

-- 出站队列
outbound_queue (device_id, cmd, seq, payload, status, tries, next_ts)

-- 待处理参数
params_pending (device_id, param_id, value, seq)
```

#### 仓储接口

- **DevicesRepo**: 设备心跳管理
- **PortsRepo**: 端口状态批量更新
- **InboundLogsRepo**: 入站消息审计
- **OutboundQueueRepo**: 出站队列管理
- **ParamsPendingRepo**: 参数队列管理

### 3. 可靠性Worker (`worker.go`)

#### 功能特性

- **冷启动恢复**: 扫描卡住的消息并重新发送
- **定期扫描**: 处理到期的待发送消息
- **重试机制**: 可配置的退避时间(默认15秒)和重试次数(默认1次)
- **ACK处理**: 消息确认和状态更新
- **死信处理**: 超过重试次数的消息标记

#### 状态流转

```
pending(0) → sent(1) → acked(2)
           ↓
         dead(3)
```

### 4. 业务处理 (`business.go`)

#### 主要功能

- **心跳处理**: 解析ICCID、RSSI、固件版本，更新设备信息，回复时间同步
- **状态上报**: 解析多插座多端口TLV数据，批量更新端口快照
- **控制响应**: 处理各类控制指令并ACK确认
- **状态查询**: 主动查询设备状态

## 数据流

### 上行消息流程

1. TCP Server 接收原始数据
2. GN Router 解析帧格式和校验和
3. Business Handler 处理具体业务逻辑
4. Storage Repos 更新数据库状态
5. Inbound Logs 记录处理结果

### 下行消息流程

1. Business Logic 调用 EnqueueMessage
2. Outbound Queue 存储待发消息
3. Worker 定期扫描到期消息
4. Sender 发送到TCP连接
5. ACK Handler 处理设备确认

## 配置示例

```yaml
protocols:
  enable_gn: true
  gn:
    listen: ":9000"              # 监听端口
    checksum: "sum_mod_256"      # 校验算法
    read_buffer: 4096            # 读缓冲
    write_buffer: 4096           # 写缓冲
    idle_timeout: 300            # 连接超时(秒)
    retry_backoff: 15            # 重试退避(秒)
```

## 使用示例

### 创建GN服务

```go
// 创建存储仓储
repos := gn.NewPostgresRepos(dbPool)

// 创建可靠性Worker
workerConfig := gn.DefaultWorkerConfig()
worker := gn.NewWorker(repos, sender, workerConfig)

// 创建业务处理器
handler := gn.NewBusinessHandler(repos, worker)

// 创建路由器
router := gn.NewRouter(handler)

// 启动Worker
worker.Start(ctx)

// 处理消息
router.Route(ctx, rawData)
```

### 发送状态查询

```go
err := handler.SendStatusQuery(ctx, "82200520004869", 1)
```

### 获取设备状态

```go
device, ports, err := handler.GetDeviceStatus(ctx, "82200520004869")
```

## 指标监控

### Worker指标

- `sent_total`: 总发送数
- `retries_total`: 总重试数  
- `acks_total`: 总确认数
- `dead_total`: 死信数
- `in_flight`: 在途消息数

### 建议的Prometheus指标

```
gn_decode_total{result="success|error"}
gn_decode_errors_total{reason="checksum|length|format"}
gn_outbound_sent_total{device_type="gateway"}
gn_outbound_retries_total{device_type="gateway"}
gn_outbound_dead_total{device_type="gateway"}
gn_ack_total{device_type="gateway"}
gn_sessions_active{protocol="gn"}
gn_outbound_inflight{protocol="gn"}
```

## 测试验证

### 单元测试覆盖

- Wire格式编解码：真实协议示例验证
- TLV解析：多种数据类型和嵌套结构
- 路由器：命令分发和处理器调用
- 存储仓储：模拟和集成测试
- Worker：基本流程、重试逻辑、冷启动、死信处理

### 集成测试场景

1. 完整的心跳→时间同步→状态上报流程
2. 状态查询→响应→ACK确认流程
3. 网络异常→重试→恢复流程
4. 冷启动→卡住消息恢复流程

## 故障排查

### 常见问题

1. **校验和错误**: 检查计算范围是否从长度字段开始
2. **帧长度不匹配**: 验证长度字段是否包含额外的2字节
3. **TLV解析失败**: 检查嵌套结构和数据类型
4. **消息卡住**: 查看Worker日志和outbound_queue表状态
5. **重试过多**: 检查网络连接和设备响应

### 日志关键字

- `GN worker: message X sent successfully`: 消息发送成功
- `GN worker: message X marked as dead`: 消息标记为死信
- `GN: Heartbeat processed successfully`: 心跳处理成功
- `parse_error`: 协议解析错误
- `checksum mismatch`: 校验和不匹配

## 扩展建议

### 性能优化

1. 批量处理端口更新
2. 连接池复用
3. 消息队列分片
4. 指标采样优化

### 功能扩展

1. 参数配置下发
2. OTA升级支持
3. 异常告警聚合
4. 历史数据归档

### 运维工具

1. 设备状态监控面板
2. 消息队列管理工具
3. 协议调试工具
4. 性能分析工具
