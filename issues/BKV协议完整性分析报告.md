# BKV 协议完整性分析报告

> **生成时间**: 2025-10-05  
> **基准文档**: `docs/协议/设备对接指引-组网设备2024(1).txt` V1.7  
> **项目状态**: 部分实现，存在多处功能缺失

---

## 一、执行摘要

本报告对照《设备对接指引-组网设备2024(1).txt》V1.7 版本，系统性检查了 `iot-server` 项目中 BKV 协议的实现完整性。

### 核心发现

| 指标 | 数值 | 说明 |
|------|------|------|
| **协议要求命令总数** | 21 | 外层14个 + BKV子协议7个 |
| **已完整实现** | 5 | 心跳、状态上报、基础控制、充电结束、异常上报 |
| **部分实现** | 4 | 参数设置/查询、按功率充电、控制命令 |
| **完全缺失** | 12 | 刷卡充电、OTA、组网管理、余额查询、设备回应等 |
| **整体完成度** | **约 43%** | 基础功能可用，进阶功能缺失严重 |

### 风险等级

🔴 **高风险**:

- 无法支持刷卡支付模式（影响业务核心流程）
- OTA 升级缺失（无法远程维护）
- 组网管理缺失（无法动态管理设备）

🟡 **中风险**:

- 参数设置使用内存存储（重启丢失）
- 服务费计费模式缺失（影响计费灵活性）

---

## 二、详细功能对照表

### 2.1 基础功能 (Section 2.1)

| 协议章节 | 命令码 | 功能描述 | 实现状态 | 文件位置 | 问题说明 |
|---------|--------|----------|---------|---------|---------|
| 2.1.1 | 0x0000 | 心跳上报/回复 | ✅ 完整 | `handlers.go:28-48` | 无 |
| 2.2.2 | - | 服务器下发数据/模块上报数据 | ✅ 完整 | `parser.go` | 基础帧解析已实现 |
| 2.2.3 | 0x1017 | 插座状态上报 | ✅ 完整 | `handlers.go:50-107`<br>`tlv.go:433` | 支持TLV格式解析 |
| 2.2.4 | 0x1D | 查询插座状态 | ⚠️ 部分 | 未见专门处理 | 需确认是否支持主动查询 |
| 2.2.5 | 0x08 | 下发网络节点列表-刷新列表 | ❌ 缺失 | `router_handlers.go:23` | 仅映射到通用处理器 |
| 2.2.6 | 0x09 | 下发网络节点列表-添加单个插座 | ❌ 缺失 | 无 | 完全未实现 |
| 2.2.7 | 0x0A | 下发网络节点列表-删除单个插座 | ❌ 缺失 | 无 | 完全未实现，协议文档仅写"略"需与厂商确认 |
| 2.2.8 | 0x07<br>0x1007 | 控制设备(按时/按电量) | ⚠️ 部分 | `handlers.go:238-337` | 基础控制已实现，BKV格式需补充 |
| 2.2.9 | 0x02<br>0x1004 | 充电结束上报 | ✅ 完整 | `handlers.go:339-414`<br>`handlers.go:183-236` | 支持旧格式和BKV格式 |

---

### 2.2 进阶功能 (Section 2.2)

| 协议章节 | 命令码 | 功能描述 | 实现状态 | 文件位置 | 问题说明 |
|---------|--------|----------|---------|---------|---------|
| 2.2.1 | 0x17 | 按功率下发充电命令 | ⚠️ 部分 | `handlers.go:269-281` | 有ChargingModeByLevel，但多档位逻辑不完整 |
| 2.2.2 | 0x18 | 按功率充电结束上报 | ⚠️ 部分 | 未见专门处理 | 需补充结算功率和分档计费逻辑 |
| 2.2.3 | 0x0B | 刷卡充电-设备上报卡号 | ❌ 缺失 | `handlers.go:594-605` | 仅有空函数骨架 |
| 2.2.3 | 0x0B | 刷卡充电-平台下发充电指令 | ❌ 缺失 | 无 | 完全未实现，支持3种模式：按时/按量/按功率 |
| 2.2.3 | 0x0F | **刷卡充电-设备回应订单** | ❌ 缺失 | 无 | **完全遗漏**，文档522-534行 |
| 2.2.3 | 0x0C | 刷卡充电-充电结束上报 | ❌ 缺失 | 无 | 完全未实现，有两种格式 |
| 2.2.4 | 0x1A | 刷卡查询余额 | ❌ 缺失 | 无 | 完全未实现 |
| 2.2.5 | 0x1B | 设置设备允许语音播报时间 | ❌ 缺失 | 无 | 完全未实现 |
| 2.2.6 | 0x1011 | 插座系统参数设置 | ⚠️ 部分 | `handlers.go:458-547`<br>`wire.go:22-40` | **重大问题**: 参数存储使用内存map，重启丢失 |
| 2.2.7 | 0x1012 | 插座系统参数查询 | ⚠️ 部分 | `handlers.go:564-577` | 基础功能有，缺少参数持久化 |
| 2.2.8 | 0x1010 | 异常事件上报 | ✅ 完整 | `handlers.go:549-562` | 已实现，但未建立专门异常表 |
| 2.2.9 | 0x07 | OTA升级 | ❌ 缺失 | `router_handlers.go:27-30` | 仅映射到通用处理器，无具体逻辑 |
| - | - | 按电费+服务费充电 | ❌ 缺失 | 无 | 完全未实现（协议文档379-453行） |

---

## 三、关键问题详解

### 🔴 问题 1: 刷卡充电流程完全缺失

**影响范围**: 无法支持线下刷卡支付场景，影响用户体验和业务收入

**协议要求** (协议文档 454-597 行):

```
完整流程（7步）：
1. 设备识别卡片 → 上报卡号 (cmd=0x0B 上行, 461-478行)
2. 平台验证卡片 → 下发充电指令 (cmd=0x0B 下行, 480-521行)
   - 按时/按量模式: 480-496行
   - 按功率模式: 498-521行 ⭐ 注意支持功率档位
3. 设备收到订单 → 回复确认 (cmd=0x0F 上行, 522-534行) ⭐ 易遗漏
4. [可选] 设备请求余额 → 平台回复 (cmd=0x1A, 599-632行)
5. 充电进行中 → 周期状态上报 (cmd=0x1017)
6. 充电完成 → 上报结束信息 (cmd=0x0C 上行, 535-597行)
   - 按时/按量格式: 536-563行
   - 按功率格式: 564-585行（包含每档充电时间）
7. 平台确认 → 回复结束 (cmd=0x0C 下行, 587-597行)
```

**当前实现**:

```go
// handlers.go:594-605 仅有空骨架
func (h *Handlers) handleCardCharging(ctx context.Context, deviceID int64, payload *BKVPayload) error {
    // 解析刷卡相关信息
    // 这里可以实现刷卡充电的完整流程：
    // 1. 验证卡片有效性
    // 2. 检查余额
    // 3. 创建充电订单
    // 4. 更新端口状态
    
    success := true
    return h.Repo.InsertCmdLog(ctx, deviceID, 0, int(payload.Cmd), 1, []byte("Card Charging"), success)
}
```

**需要实现**:

1. `ParseCardReport()` - 解析设备上报的卡号（协议文档 461-478 行）
2. `EncodeCardChargingCommand()` - 编码充电指令，支持3种模式：
   - 按时/按量: 480-496行
   - 按功率（含档位配置）: 498-521行
3. `HandleOrderConfirmation()` - ⭐ **处理设备回应订单 (cmd=0x0F, 522-534行)**
4. `HandleCardBalanceQuery()` - 处理余额查询（599-632 行）
5. `HandleCardChargingEnd()` - 处理充电结束，区分两种格式：
   - 按时/按量: 536-563行
   - 按功率（含每档时间）: 564-585行
6. 数据库层面：
   - 卡片信息表 `cards` (卡号、余额、状态)
   - 刷卡充电订单扩展字段 (卡号、计费模式、花费金额)

**关键注意事项**:

- ⚠️ **命令 0x0F 容易遗漏**，但它是刷卡流程的必要确认环节
- ⚠️ 刷卡充电有3种模式，编码和解析逻辑需区分
- ⚠️ 按功率刷卡的结束上报包含每档充电时间，需特殊处理

**预估工作量**: 5-6 工作日（原估3-5天，增加命令0x0F和多模式支持）

---

### 🔴 问题 2: OTA 升级功能缺失

**影响范围**: 无法远程升级设备固件，运维成本高

**协议要求** (协议文档 798-812 行):

```
平台下发示例:
fcff0025000715c187ad008221080600041301652591a2001547562e387231322e4f54410000bbfcee

字段解析:
- 01: 升级DTU (01=DTU, 02=插座)
- 652591a2: FTP服务器IP (101.37.145.162)
- 0015: FTP端口 (21)
- 47562e387231322e4f54410000: 文件名 (Gv.8r12.OTA，13字节ASCII转hex，不足补0)
```

**当前实现**:

```go
// router_handlers.go:27-30
adapter.Register(0x0007, func(f *Frame) error {
    return handlers.HandleGeneric(context.Background(), f)  // 仅记录日志，无具体操作
})
```

**需要实现**:

1. `EncodeOTACommand()` - 编码OTA升级指令

   ```go
   type OTACommand struct {
       TargetType uint8  // 01=DTU, 02=Socket
       FTPServer  string // IP地址
       FTPPort    uint16
       FileName   string // 13字节，不足补0
   }
   ```

2. `HandleOTAProgress()` - 处理设备上报的升级进度
3. `HandleOTAResult()` - 处理升级结果上报
4. 数据库表 `ota_tasks`:

   ```sql
   CREATE TABLE ota_tasks (
       id SERIAL PRIMARY KEY,
       device_id BIGINT REFERENCES devices(id),
       target_type SMALLINT,
       firmware_version VARCHAR(20),
       ftp_url TEXT,
       status SMALLINT, -- 0=待下发, 1=下载中, 2=升级中, 3=成功, -1=失败
       progress SMALLINT,
       error_msg TEXT,
       created_at TIMESTAMPTZ DEFAULT NOW(),
       updated_at TIMESTAMPTZ DEFAULT NOW()
   );
   ```

**预估工作量**: 2-3 工作日

---

### 🔴 问题 3: 组网管理功能缺失

**影响范围**: 无法动态管理网关下的插座设备，影响设备扩展和维护

**协议要求**:

- **2.2.5 刷新列表** (协议文档 183-215 行): 批量下发组网设备列表
- **2.2.6 添加单个插座** (216-245 行): 动态添加新插座
- **2.2.7 删除单个插座** (246-248 行): 移除故障/废弃插座

**当前实现**:

```go
// router_handlers.go:23-26
adapter.Register(0x0005, func(f *Frame) error {
    return handlers.HandleGeneric(context.Background(), f)  // 仅通用处理
})
```

**命令格式示例** (刷新列表):

```
fcff00310005001c94f90086004459453005001d0804014500307002470245003070074303350030
7012470425910240232075fcee

解析:
- 08: 命令码
- 04: 信道 (1-15)
- 01: 1号插座
- 450030700247: 1号插座MAC
- 02: 2号插座
- 450030700743: 2号插座MAC
...
```

**需要实现**:

1. `ParseNetworkNodeList()` - 解析节点列表
2. `EncodeNetworkRefresh()` - 编码刷新列表命令
3. `EncodeNetworkAddNode()` - 编码添加节点命令
4. `EncodeNetworkDeleteNode()` - 编码删除节点命令
5. 数据库表 `gateway_sockets`:

   ```sql
   CREATE TABLE gateway_sockets (
       id SERIAL PRIMARY KEY,
       gateway_id VARCHAR(50) NOT NULL,
       socket_no SMALLINT NOT NULL,
       socket_mac VARCHAR(20) NOT NULL,
       socket_uid VARCHAR(20),
       channel SMALLINT,
       status SMALLINT DEFAULT 0,
       created_at TIMESTAMPTZ DEFAULT NOW(),
       UNIQUE(gateway_id, socket_no)
   );
   ```

**预估工作量**: 2-3 工作日

---

### 🟡 问题 4: 参数设置使用内存存储

**影响范围**: 服务重启后参数设置记录丢失，无法进行参数写入后的回读校验

**当前实现** (`wire.go:9-40`):

```go
type repoAdapter struct {
 *pgstorage.Repository
 // 简单的内存存储用于参数校验（生产环境应该使用数据库）
 // TODO: Replace with proper database-backed parameter storage. See issue #XXX.
 paramStore map[string]paramEntry  // ⚠️ 内存存储，重启丢失
}

type paramEntry struct {
 Value []byte
 MsgID int
}
```

**问题分析**:

1. 参数设置后重启丢失，无法追溯历史
2. 分布式部署时无法共享状态
3. 无法支持参数回读校验的持久化需求

**需要实现**:

```sql
-- 数据库迁移脚本
CREATE TABLE device_param_writes (
    id SERIAL PRIMARY KEY,
    device_id BIGINT REFERENCES devices(id),
    param_id SMALLINT NOT NULL,
    param_value BYTEA NOT NULL,
    msg_id INTEGER NOT NULL,
    status SMALLINT DEFAULT 0, -- 0=待验证, 1=已验证, -1=验证失败
    verified_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(device_id, param_id, msg_id)
);

CREATE INDEX idx_param_writes_device ON device_param_writes(device_id, param_id);
```

**代码修改**:

```go
// internal/storage/pg/repo.go
func (r *Repository) StoreParamWrite(ctx context.Context, deviceID int64, paramID int, value []byte, msgID int) error {
    query := `INSERT INTO device_param_writes (device_id, param_id, param_value, msg_id) 
              VALUES ($1, $2, $3, $4)
              ON CONFLICT (device_id, param_id, msg_id) DO UPDATE 
              SET param_value = EXCLUDED.param_value`
    _, err := r.Pool.Exec(ctx, query, deviceID, paramID, value, msgID)
    return err
}

func (r *Repository) GetParamWritePending(ctx context.Context, deviceID int64, paramID int) ([]byte, int, error) {
    query := `SELECT param_value, msg_id FROM device_param_writes 
              WHERE device_id = $1 AND param_id = $2 AND status = 0 
              ORDER BY created_at DESC LIMIT 1`
    var value []byte
    var msgID int
    err := r.Pool.QueryRow(ctx, query, deviceID, paramID).Scan(&value, &msgID)
    return value, msgID, err
}
```

**预估工作量**: 1 工作日

---

### 🟡 问题 5: 按电费+服务费计费模式缺失

**影响范围**: 无法支持电费和服务费分开计费的业务场景

**协议要求** (协议文档 379-453 行):

**控制命令示例**:

```
0x88: 0064 - 支付金额 (1元=100分)
0x80: 01 - 服务费收取模式 (0=按时, 1=按电量)
0x89: 01 - 服务费档位数
0x83: 173B00320032 - 服务费每档信息 (时段+电费+服务费)
```

**充电结束上报示例**:

```
0x85: 0000 - 电费金额
0x86: 0000 - 服务费金额
0x89: 01 - 服务费档位数
0x84: 000100000000 - 每档已充时间&电量
```

**需要实现**:

1. 扩展 `BKVControlCommand` 结构体:

   ```go
   type ServiceFeeConfig struct {
       Mode      uint8    // 0=按时, 1=按电量
       LevelCount uint8   // 档位数
       Levels    []FeeLevel
   }
   
   type FeeLevel struct {
       StartTime   uint16 // 开始时间 HHMM
       EndTime     uint16 // 结束时间 HHMM
       ElectricFee uint16 // 电费 (分/度)
       ServiceFee  uint16 // 服务费 (分/度)
   }
   ```

2. 充电结束时分别记录电费和服务费:

   ```go
   type ChargingEndReport struct {
       // ... 现有字段
       ElectricFeeCents int  // 电费(分)
       ServiceFeeCents  int  // 服务费(分)
       LevelUsage       []LevelUsage
   }
   ```

3. 数据库扩展:

   ```sql
   ALTER TABLE orders ADD COLUMN electric_fee_cents INTEGER;
   ALTER TABLE orders ADD COLUMN service_fee_cents INTEGER;
   ALTER TABLE orders ADD COLUMN fee_level_usage JSONB;
   ```

**预估工作量**: 2-3 工作日

---

## 四、数据库缺口分析

### 4.1 当前数据库表结构推断

基于代码中的 `repoAPI` 接口和调用分析：

```sql
-- 现有表（推断）
devices (id, phy_id, ...)
cmd_logs (device_id, msg_id, cmd, direction, payload, success, created_at)
port_states (device_id, port_no, status, power_w, updated_at)
orders (device_id, port_no, order_hex, duration_sec, kwh_01, status, power_w_01, settled_at, reason)
outbound_queue (device_id, msg_id, ...)
```

### 4.2 需要新增的表

```sql
-- 1. 卡片管理表
CREATE TABLE cards (
    id SERIAL PRIMARY KEY,
    card_no VARCHAR(20) UNIQUE NOT NULL,
    card_type SMALLINT DEFAULT 0, -- 0=在线卡, 1=离线卡
    balance_cents INTEGER DEFAULT 0,
    status SMALLINT DEFAULT 1, -- 1=正常, 0=冻结
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- 2. 刷卡充电订单扩展
ALTER TABLE orders ADD COLUMN card_no VARCHAR(20);
ALTER TABLE orders ADD COLUMN charging_mode SMALLINT; -- 1=按时, 2=按量, 3=按功率
ALTER TABLE orders ADD COLUMN spent_cents INTEGER; -- 花费金额(分)
ALTER TABLE orders ADD COLUMN settlement_power INTEGER; -- 结算功率(0.1W)

-- 3. OTA 任务表
CREATE TABLE ota_tasks (
    id SERIAL PRIMARY KEY,
    device_id BIGINT REFERENCES devices(id),
    target_type SMALLINT, -- 1=DTU, 2=Socket
    firmware_version VARCHAR(20),
    ftp_server VARCHAR(50),
    ftp_port INTEGER,
    file_name VARCHAR(50),
    status SMALLINT DEFAULT 0, -- 0=待下发, 1=下载中, 2=升级中, 3=成功, -1=失败
    progress SMALLINT,
    error_msg TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- 4. 网关插座组网关系表
CREATE TABLE gateway_sockets (
    id SERIAL PRIMARY KEY,
    gateway_id VARCHAR(50) NOT NULL,
    socket_no SMALLINT NOT NULL,
    socket_mac VARCHAR(20) NOT NULL,
    socket_uid VARCHAR(20),
    channel SMALLINT,
    status SMALLINT DEFAULT 0,
    rssi SMALLINT,
    temperature SMALLINT,
    software_version VARCHAR(20),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(gateway_id, socket_no)
);
CREATE INDEX idx_gateway_sockets_gateway ON gateway_sockets(gateway_id);
CREATE INDEX idx_gateway_sockets_mac ON gateway_sockets(socket_mac);

-- 5. 设备参数写入记录表
CREATE TABLE device_param_writes (
    id SERIAL PRIMARY KEY,
    device_id BIGINT REFERENCES devices(id),
    param_id SMALLINT NOT NULL,
    param_value BYTEA NOT NULL,
    msg_id INTEGER NOT NULL,
    status SMALLINT DEFAULT 0, -- 0=待验证, 1=已验证, -1=验证失败
    verified_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(device_id, param_id, msg_id)
);
CREATE INDEX idx_param_writes_device ON device_param_writes(device_id, param_id);

-- 6. 异常事件表
CREATE TABLE device_exceptions (
    id SERIAL PRIMARY KEY,
    device_id BIGINT REFERENCES devices(id),
    socket_no SMALLINT,
    port_no SMALLINT,
    exception_type SMALLINT, -- 异常类型
    exception_reason SMALLINT, -- 异常原因
    status_value INTEGER,
    over_voltage INTEGER,
    under_voltage INTEGER,
    leakage_current INTEGER,
    over_temp SMALLINT,
    charging_status INTEGER,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_exceptions_device_time ON device_exceptions(device_id, created_at DESC);

-- 7. 设备语音播报配置表
CREATE TABLE device_voice_configs (
    id SERIAL PRIMARY KEY,
    device_id BIGINT REFERENCES devices(id),
    socket_no SMALLINT,
    port_no SMALLINT,
    buzzer_enabled BOOLEAN DEFAULT TRUE,
    voice_enabled BOOLEAN DEFAULT TRUE,
    time_slots JSONB, -- [{"start":"0000","end":"2359"}]
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(device_id, socket_no, port_no)
);
```

---

## 五、协议命令注册完整性检查

### 5.1 当前注册情况

**文件**: `internal/protocol/bkv/router_handlers.go`

```go
func RegisterHandlers(adapter *Adapter, handlers *Handlers) {
 adapter.Register(0x0000, func(f *Frame) error { ... }) // ✅ 心跳
 adapter.Register(0x1000, func(f *Frame) error { ... }) // ✅ BKV子协议
 adapter.Register(0x0015, func(f *Frame) error { ... }) // ✅ 控制指令
 adapter.Register(0x0005, func(f *Frame) error { ... }) // ⚠️ 网络节点(通用)
 adapter.Register(0x0007, func(f *Frame) error { ... }) // ⚠️ OTA(通用)
}
```

### 5.2 需要补充注册的命令

```go
func RegisterHandlers(adapter *Adapter, handlers *Handlers) {
 // 基础命令
 adapter.Register(0x0000, handlers.makeHandler(handlers.HandleHeartbeat))
 adapter.Register(0x1000, handlers.makeHandler(handlers.HandleBKVStatus))
 adapter.Register(0x0015, handlers.makeHandler(handlers.HandleControl))
 
 // 网络节点管理 ⭐ 需要新增
 adapter.Register(0x0008, handlers.makeHandler(handlers.HandleNetworkRefresh))
 adapter.Register(0x0009, handlers.makeHandler(handlers.HandleNetworkAddNode))
 adapter.Register(0x000A, handlers.makeHandler(handlers.HandleNetworkDeleteNode))
 
 // 刷卡充电 ⭐ 需要新增
 adapter.Register(0x000B, handlers.makeHandler(handlers.HandleCardCharging))
 adapter.Register(0x000C, handlers.makeHandler(handlers.HandleCardChargingEnd))
 adapter.Register(0x001A, handlers.makeHandler(handlers.HandleCardBalanceQuery))
 
 // 语音播报 ⭐ 需要新增
 adapter.Register(0x001B, handlers.makeHandler(handlers.HandleVoiceConfig))
 
 // OTA升级 ⭐ 需要补充
 adapter.Register(0x0007, handlers.makeHandler(handlers.HandleOTA))
 
 // 查询插座状态 ⭐ 需要补充
 adapter.Register(0x001D, handlers.makeHandler(handlers.HandleQuerySocketState))
}

// 辅助方法：将处理器统一包装
func (h *Handlers) makeHandler(fn func(context.Context, *Frame) error) func(*Frame) error {
 return func(f *Frame) error {
  return fn(context.Background(), f)
 }
}
```

---

## 六、测试覆盖度分析

### 6.1 现有测试文件

```
internal/protocol/bkv/
├── adapter_test.go              ✅ 适配器测试
├── control_commands_test.go     ✅ 控制命令测试
├── extended_commands_test.go    ✅ 扩展命令测试
├── handlers_param_test.go       ✅ 参数处理测试
├── handlers_test.go             ✅ 处理器测试
├── parser_test.go               ✅ 解析器测试
├── protocol_replay_test.go      ✅ 协议回放测试
├── reason_map_test.go           ✅ 原因映射测试
├── replay_ext_test.go           ✅ 扩展回放测试
├── replay_test.go               ✅ 回放测试
├── router_test.go               ✅ 路由测试
└── tlv_test.go                  ✅ TLV解析测试
```

### 6.2 缺失的测试用例

需要为以下功能补充测试：

1. **刷卡充电流程测试** `card_charging_test.go`
   - 设备上报卡号 → 平台验证 → 下发充电指令 → 充电结束
   - 余额查询流程
   - 离线卡/在线卡区分

2. **OTA升级测试** `ota_test.go`
   - 平台下发升级命令
   - 设备上报升级进度
   - 升级成功/失败处理

3. **组网管理测试** `network_test.go`
   - 刷新节点列表
   - 添加/删除单个插座
   - 组网状态查询

4. **按电费+服务费测试** `service_fee_test.go`
   - 多档位服务费配置
   - 分时段计费
   - 电费和服务费分别结算

5. **参数持久化测试** `param_persistence_test.go`
   - 参数写入 → 重启 → 回读校验
   - 数据库持久化验证

---

## 七、实施计划

### Phase 1: 高优先级缺陷修复 (1-2周)

| 任务 | 工作量 | 责任人 | 验收标准 |
|-----|--------|-------|---------|
| 参数存储持久化 | 1d | - | 参数写入后重启不丢失 |
| 组网管理功能 | 3d | - | 可远程管理网关插座列表 |
| 刷卡充电流程 | 5d | - | 支持完整刷卡充电流程 |
| OTA升级功能 | 3d | - | 可远程升级设备固件 |

### Phase 2: 计费功能完善 (1周)

| 任务 | 工作量 | 责任人 | 验收标准 |
|-----|--------|-------|---------|
| 按功率充电完善 | 2d | - | 支持多档位功率计费 |
| 电费+服务费计费 | 3d | - | 支持分时段电费和服务费 |
| 余额查询功能 | 1d | - | 支持刷卡余额查询 |

### Phase 3: 辅助功能补充 (3-5天)

| 任务 | 工作量 | 责任人 | 验收标准 |
|-----|--------|-------|---------|
| 语音播报配置 | 1d | - | 可设置静音时段 |
| 查询插座状态 | 1d | - | 支持主动查询插座 |
| 异常事件专表 | 1d | - | 异常事件独立存储 |

### Phase 4: 测试与文档 (1周)

| 任务 | 工作量 | 责任人 | 验收标准 |
|-----|--------|-------|---------|
| 补充单元测试 | 3d | - | 覆盖率达到80%+ |
| 集成测试 | 2d | - | 完整流程测试通过 |
| 协议对接文档 | 1d | - | 更新API文档 |

**总预估工作量**: 3-4 周

---

## 八、风险与建议

### 8.1 技术风险

1. **数据迁移风险**: 新增多个数据库表，需要编写迁移脚本
   - **建议**: 使用数据库版本管理工具，先在测试环境验证

2. **兼容性风险**: 修改现有协议处理逻辑可能影响已上线设备
   - **建议**: 保留旧格式兼容，使用协议版本号区分

3. **性能风险**: 刷卡充电流程增加数据库交互次数
   - **建议**: 优化数据库索引，使用连接池

4. **协议文档不完整风险**: 部分命令响应格式标注为"略"
   - **文档 316行**: 充电结束上报的平台回复 - 略
   - **文档 345行**: 按功率充电的设备回复 - 略（仅注明回复业务号）
   - **文档 248行**: 删除单个插座命令 - 略
   - **建议**: 与设备厂商确认这些"略"的部分是否可以省略响应

### 8.2 业务建议

1. **优先实现刷卡充电**: 这是核心支付场景，直接影响用户体验

2. **OTA功能尽快上线**: 避免现场维护成本过高

3. **参数持久化是基础**: 必须先解决，否则影响其他功能稳定性

4. **建立设备协议测试环境**: 模拟器或真实设备，用于验证协议实现

### 8.3 代码质量建议

1. **统一错误处理**: 当前各处理器错误处理不一致，建议统一封装

2. **日志规范化**: 关键操作需要记录详细日志，便于排查问题

3. **指标监控**: 为关键操作添加 Prometheus 指标
   - 刷卡成功率
   - OTA升级成功率
   - 组网操作成功率
   - 参数设置成功率

4. **文档完善**:
   - 协议实现说明文档
   - API对接文档
   - 运维手册

---

## 九、协议约束与业务规则

### 9.1 重要约束条件

| 字段 | 约束 | 文档位置 | 说明 |
|------|------|---------|------|
| **插座编号** | 1-250 | 201行 | 同一网关下不允许重复 |
| **信道** | 1-15 | 195行 | 组网时任意数字 |
| **充电时长** | 1-900分钟 | 268行 | 按时充电的最大限制 |
| **语音时段** | 最多2段 | 648行 | 静音配置最大支持2个时段 |
| **OTA文件名** | 13字节 | 811行 | ASCII转hex，不足末位补0 |

### 9.2 周期性行为

| 行为 | 周期 | 文档位置 | 触发条件 |
|------|------|---------|---------|
| **心跳上报** | 1分钟 | 56行 | 定时触发 |
| **插座状态上报** | 5分钟 | 94, 287行 | 定时 + 状态变化立即上报 |
| **充电中状态上报** | 5分钟 | 287行 | 充电过程中周期上报功率/电量等 |

⚠️ **重要**: 状态变化（充电/离线等）时会立即上报，不等待周期

### 9.3 插座状态位解析

**示例**: `0x98 = 10011000` (二进制，高位在前)

| Bit | 含义 | 0 | 1 |
|-----|------|---|---|
| Bit 7 | 在线状态 | 离线 | 在线 |
| Bit 6 | 计量状态 | 异常 | 正常 |
| Bit 5 | 充电状态 | 未充电 | 充电中 |
| Bit 4 | 空载状态 | 正常 | 空载 |
| Bit 3 | 温度状态 | 异常 | 正常 |
| Bit 2 | 电流状态 | 异常 | 正常 |
| Bit 1 | 功率状态 | 异常 | 正常 |
| Bit 0 | 预留 | - | - |

**文档引用**: 309行、551行

⚠️ **关键业务逻辑**:

- Bit 4（空载）判断可用于确定充电结束原因
- Bit 7（在线）判断设备是否在线
- 状态位需要正确解析，影响订单结算和异常判断

### 9.4 协议文档"略"标注汇总

以下响应格式在文档中标注为"略"，需要与设备厂商确认：

| 命令 | 方向 | 文档位置 | 影响 |
|------|------|---------|------|
| 充电结束上报的平台回复 | 平台→设备 | 316-317行 | 🟡 中 - 可能仅需ACK |
| 按功率充电的设备回复 | 设备→平台 | 345行 | 🟡 中 - 注明"回复业务号" |
| 删除单个插座命令 | 双向 | 246-248行 | 🟡 中 - 完全缺失示例 |

**建议**:

1. 充电结束的平台回复：推测仅需简单ACK，可参考其他命令格式
2. 按功率充电回复：文档注明"回复业务号"，应该是标准ACK+业务号
3. 删除插座命令：最关键，需要设备厂商提供完整示例

---

## 十、附录

### A. 协议命令速查表

| 命令码 | 十六进制 | 功能 | 方向 | 文档章节 |
|--------|---------|------|------|---------|
| 0 | 0x0000 | 心跳上报/回复 | 双向 | 2.1.1 |
| 2 | 0x0002 | 充电结束上报(旧) | 上行 | 2.2.9 |
| 5 | 0x0005 | 网络节点相关 | 下行 | 2.2.5-7 |
| 7 | 0x0007 | OTA升级/控制 | 下行 | OTA |
| 8 | 0x0008 | 刷新节点列表 | 下行 | 2.2.5 |
| 9 | 0x0009 | 添加单个插座 | 下行 | 2.2.6 |
| 10 | 0x000A | 删除单个插座 | 下行 | 2.2.7 |
| 11 | 0x000B | 刷卡充电 | 双向 | 2.2.3 |
| 12 | 0x000C | 刷卡充电结束 | 双向 | 2.2.3 |
| 15 | 0x000F | **设备回应订单** ⭐ | 上行 | **2.2.3(522-534)** |
| 21 | 0x0015 | 控制指令/查询状态 | 双向 | 2.2.4/2.2.8 |
| 23 | 0x0017 | 按功率充电命令 | 下行 | 2.2.1 |
| 24 | 0x0018 | 按功率充电结束 | 上行 | 2.2.2 |
| 26 | 0x001A | 查询余额 | 双向 | 2.2.4 |
| 27 | 0x001B | 语音播报配置 | 双向 | 2.2.5 |
| 29 | 0x001D | 查询插座状态 | 双向 | 2.2.4 |
| 4096 | 0x1000 | BKV子协议包 | 双向 | - |
| 4100 | 0x1004 | BKV充电结束 | 上行 | - |
| 4103 | 0x1007 | BKV控制命令 | 双向 | - |
| 4112 | 0x1010 | 异常事件上报 | 上行 | 2.2.8 |
| 4113 | 0x1011 | 参数设置 | 双向 | 2.2.6 |
| 4114 | 0x1012 | 参数查询 | 双向 | 2.2.7 |
| 4119 | 0x1017 | 插座状态上报 | 上行 | 2.2.3 |

### B. BKV TLV 字段标签速查

| 标签 | 字段名 | 数据类型 | 单位/格式 | 说明 |
|------|--------|---------|-----------|------|
| 0x01 | Cmd | uint16 | - | BKV命令码 |
| 0x02 | Seq | uint64 | - | 帧序列号 |
| 0x03 | GatewayID | bytes | - | 网关ID |
| 0x07 | Temp | uint8 | ℃ | 温度 |
| 0x08 | PortNo | uint8 | - | 插孔号 |
| 0x09 | Status | uint8 | 位标志 | 插座状态（8位） |
| 0x0A | OrderNo | uint16 | - | 业务号 |
| 0x0B | Power | uint16 | **0.1W** | 瞬时功率，0x1000=409.6W |
| 0x0C | Current | uint16 | **mA** | 瞬时电流，0x1000=4.096A |
| 0x0D | Energy | uint16 | **0.01kWh** | 用电量，0x0050=0.08kWh |
| 0x0E | Duration | uint16 | **分钟** | 充电时间，0x2d=45min |
| 0x0F | ACK | uint8 | 1=OK, 0=错误 | 应答标志 |
| 0x11 | PowerLimit | uint16 | 功率限值 |
| 0x12 | ChargingMode | uint8 | 充电模式 |
| 0x13 | Switch | uint8 | 开关 |
| 0x21 | FullChargeDelay | uint16 | 充满续充时间(秒) |
| 0x22 | NullChargeDelay | uint16 | 空载延时(秒) |
| 0x23 | FullChargePower | uint16 | 充满功率阈值 |
| 0x24 | NullChargePower | uint16 | 空载功率阈值 |
| 0x25 | HighTempThreshold | uint8 | 高温阈值 |
| 0x2E | EndTime | bytes | 充电结束时间 |
| 0x2F | EndReason | uint8 | 结束原因 |
| 0x47 | ControlType | uint8 | 控制类型 |
| 0x4A | SocketNo | uint8 | 插座序号 |
| 0x4B-4F | ExceptionXXX | - | 异常相关字段 |
| 0x50-58 | StatusXXX | - | 状态相关字段 |
| 0x59 | MaxChargeTime | uint16 | 最大充电时间 |
| 0x60 | TrickleThreshold | uint8 | 涓流阈值 |
| 0x68 | BaseAmount | - | 按键基础金额 |
| 0x80-89 | ServiceFeeXXX | - | 服务费相关 |
| 0x93 | AntiPulseTime | uint16 | 防脉冲时间 |
| 0x94 | SocketStatusCluster | bytes | 插座状态集群 |
| 0x3E | SoftwareVer | uint16 | - | 插座软件版本 |
| 0x4A | SocketNo | uint8 | 1-250 | 插座序号 |
| 0x5B | PortAttr | bytes | - | 插孔属性 |
| 0x95 | Voltage | uint16 | **0.1V** | 电压，2275=227.5V |
| 0x96 | RSSI | uint8 | dBm | 信号强度 |

### C. 协议基础格式

**包头魔术字**:

- `0xFCFE`: 上行包（设备→平台）
- `0xFCFF`: 下行包（平台→设备）

**包尾**: `0xFCEE`（固定）

**数据方向标志**:

- `0x00`: 下行（平台→设备）
- `0x01`: 上行（设备→平台）

**帧流水号规则**:

- 设备主动上报：`0x00000000`
- 平台主动下发：需要带流水号
- 设备回复：使用平台下发的相同流水号

**校验和**: 简单累加校验（文档未详细说明算法）

**时间格式**: `YYYYMMDDHHmmss` (14字节)

- 示例：`20200730164545` = 2020年07月30日 16:45:45

**字符串编码**:

- ICCID等字段：Hex转String
- OTA文件名：ASCII转Hex（13字节，不足补0）

**LED指示状态**:

- 组网前：三颗LED一起闪烁
- 组网成功：蓝色LED长亮，充电指示灯熄灭
- 充电中：充电指示灯亮起

### D. 开发实现参考（代码示例）

#### 1. 单位转换实现

```go
// 数据单位转换函数
func ConvertPower(raw uint16) float64 {
    return float64(raw) * 0.1 // 0.1W -> W
}

func ConvertCurrent(raw uint16) float64 {
    return float64(raw) * 0.001 // mA -> A
}

func ConvertEnergy(raw uint16) float64 {
    return float64(raw) * 0.01 // 0.01kWh -> kWh
}

func ConvertVoltage(raw uint16) float64 {
    return float64(raw) * 0.1 // 0.1V -> V
}
```

#### 2. 帧验证实现

```go
const (
    MagicUplink   = 0xFCFE // 设备→平台
    MagicDownlink = 0xFCFF // 平台→设备
    MagicTail     = 0xFCEE // 包尾
)

func ValidateFrame(data []byte) bool {
    if len(data) < 4 {
        return false
    }
    head := binary.BigEndian.Uint16(data[0:2])
    tail := binary.BigEndian.Uint16(data[len(data)-2:])
    
    return (head == MagicUplink || head == MagicDownlink) && tail == MagicTail
}
```

#### 3. 时间格式转换

```go
func ParseProtocolTime(timeBytes []byte) (time.Time, error) {
    // YYYYMMDDHHmmss -> time.Time
    str := string(timeBytes)
    return time.Parse("20060102150405", str)
}

func EncodeProtocolTime(t time.Time) []byte {
    return []byte(t.Format("20060102150405"))
}
```

#### 4. 帧流水号处理

```go
func BuildDownlinkFrame(cmd uint16, data []byte) []byte {
    msgID := GenerateMsgID() // 平台下发必须有流水号
    // ... 构建帧
}

func BuildUplinkFrame(cmd uint16, data []byte, replyToMsgID uint32) []byte {
    if replyToMsgID == 0 {
        // 主动上报，流水号为0
    } else {
        // 回复平台，使用相同流水号
    }
    // ... 构建帧
}
```

---

**报告结束**

*本报告由项目分析工具自动生成，建议定期更新并与实际开发进度同步。*
