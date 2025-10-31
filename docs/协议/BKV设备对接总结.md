# BKV 设备对接规范

> 最后更新：2025-10-31  
> 协议版本：组网设备 2024(1) - 章节 2.2.8  
> 验证设备：82241218000382（单机版，插座号 0，2 个插孔）

---

## 一、设备类型识别

| 设备类型   | 插座号    | 插孔数      | 特征               |
| ---------- | --------- | ----------- | ------------------ |
| **单机版** | 0（固定） | 2 个（0/1） | 单机设备，无组网   |
| 组网版     | 1-250     | 2 个/插座   | 多插座通过网关管理 |

**识别方法**：测试插座号 1→ 失败则为单机版（用 0）

---

## 二、协议格式（0x0015 控制命令）

### 2.1 下行控制

```
格式：[长度2B][0x07][插座1B][插孔1B][开关1B][模式1B][时长2B][业务号2B]
示例：0008 07 00 00 01 01 003c 0000
      ^^^^ 参数长度（不含0x07）
           ^^ 控制命令
              ^^ 插座0（单机版）
                 ^^ 插孔0（A孔）/01（B孔）
                    ^^ 开关：1=开,0=关
                       ^^ 模式：1=按时长,0=按电量
                          ^^^^ 时长（分钟）
                               ^^^^ 业务号
```

**关键点**：

- 长度 = 参数字节数（**不含 0x07 命令字节**）
- 单机版插座号**必须为 0**
- API 端口 → 协议插孔：port1→0, port2→1

### 2.2 设备 ACK

```
格式：[长度2B][0x07][结果1B][插座1B][插孔1B][业务号2B]
示例：0005 07 01 00 00 0001
           ^^ ^^ ^^ ^^
           |  |  |  └─ 插孔号
           |  |  └──── 插座号
           |  └─────── 结果：01=成功,00=失败
           └────────── 控制命令
```

**解析顺序**：`inner[1]=result, inner[2]=socketNo, inner[3]=portNo`

### 2.3 充电上报（0x02）

```
格式：[长度2B][0x02][插座1B][版本2B][温度1B][RSSI1B][插孔1B][状态1B][业务号2B][功率2B][电流2B][用电量2B][时长2B]
示例：0011 02 00 0000 00 00 01 98 0001 0000 0000 0050 002d
      ^^^^ 17字节
              ^^ 插孔号
                       ^^ 状态字节（bit解析见下）
                                 ^^^^ 功率（0.1W）
                                      ^^^^ 电流（0.001A）
                                           ^^^^ 用电量（0.01kWh）
                                                ^^^^ 时长（分钟）
```

**状态字节**（8bit）：

```
bit7: 在线 | bit6: 预留 | bit5: 计量正常 | bit4: 充电中
bit3: 空载 | bit2: 温度正常 | bit1: 电流正常 | bit0: 功率正常
```

**上报时机**：充电结束/周期 5 分钟/状态变化

---

## 三、代码实现要点

### 3.1 插座号设置（`internal/api/thirdparty_handler.go`）

```go
// 单机版设备必须用插座号0
socketNo := uint8(0)

// 生成控制payload
innerPayload := h.encodeStartControlPayload(socketNo, mapped, ...)

// 长度字段 = 参数字节数（不含0x07）
paramLen := len(innerPayload) - 1
payload[0] = byte(paramLen >> 8)
payload[1] = byte(paramLen)
copy(payload[2:], innerPayload)
```

### 3.2 ACK 解析（`internal/protocol/bkv/handlers.go`）

```go
// 严格按协议顺序解析
result := inner[1]     // 01=成功, 00=失败
socketNo := inner[2]   // 插座号
portNo := inner[3]     // 插孔号：0=A孔,1=B孔
businessNo := uint16(inner[4]) // 业务号低字节

// 协议插孔号→API端口号
apiPortNo := int(portNo) + 1  // 插孔0→端口1, 插孔1→端口2
```

### 3.3 充电上报处理

```go
// 解析充电数据
power := int(f.Data[12])<<8 | int(f.Data[13])    // 0.1W
current := int(f.Data[14])<<8 | int(f.Data[15])  // 0.001A
energy := int(f.Data[16])<<8 | int(f.Data[17])   // 0.01kWh
duration := int(f.Data[18])<<8 | int(f.Data[19]) // 分钟
status := f.Data[9] // 状态字节

// Metrics采集（可选）
if h.Metrics != nil {
    deviceID := fmt.Sprintf("%d", devID)
    portNo := fmt.Sprintf("%d", apiPortNo)
    h.Metrics.GetChargeReportPowerGauge().WithLabelValues(deviceID, portNo).Set(float64(power) / 10.0)
    // ... 其他指标
}
```

---

## 四、测试用例

### 端口 1 充电流程（12:00:50）

```
1. API请求：POST /api/v1/charge/start {port_no:1}
2. 下行帧：  00080700000101003c0000 （插座0+插孔0）
3. 设备ACK： 0005070100000001 （成功）
4. 语音播报："左边插孔已打开"
5. 停止命令：00080700000001000000 （开关=0）
6. 停止ACK： 0005070100000000 （成功）
7. 充电上报：0011020000000000... （状态恢复空闲）
```

### 端口 2 充电流程（12:02:08）

```
1. API请求：POST /api/v1/charge/start {port_no:2}
2. 下行帧：  00080700010101003c0000 （插座0+插孔1）
3. 设备ACK： 0005070100010001 （成功）
4. 语音播报："右边插孔已打开" → "右边插孔已插入"
5. 停止命令：00080700010001000000
6. 停止ACK： 0005070100010000 （成功）
7. 充电上报：端口2状态=01（曾充电）
```

**订单状态流转**：`pending → charging → cancelled`

---

## 五、故障排查

| 问题               | 检查项   | 解决方法                |
| ------------------ | -------- | ----------------------- |
| ACK 返回失败（00） | 插座号   | 单机版必须用 0          |
|                    | 长度字段 | 参数字节数（不含 0x07） |
|                    | 插孔号   | 必须为 0 或 1           |
| 订单卡 pending     | 设备在线 | 检查 Redis 会话         |
|                    | 下行队列 | 查看 outbound_queue 表  |
|                    | ACK 解析 | 验证 result 字段处理    |
| 设备无响应         | TCP 连接 | 检查 session 状态       |
|                    | 命令格式 | 验证协议头/校验和       |
|                    | 队列堵塞 | 检查 Redis 队列         |

---

## 六、监控指标（Prometheus）

```
# 充电状态计数
charge_report_total{device_id,port_no,status} counter

# 实时功率（W）
charge_report_power_watts{device_id,port_no} gauge

# 实时电流（A）
charge_report_current_amperes{device_id,port_no} gauge

# 累计电量（Wh）
charge_report_energy_wh_total{device_id,port_no} counter
```

**采集位置**：`internal/protocol/bkv/handlers.go` 充电上报处理函数

---

## 七、相关文件

| 文件                                 | 功能                     |
| ------------------------------------ | ------------------------ |
| `internal/api/thirdparty_handler.go` | 第三方 API，充电控制入口 |
| `internal/protocol/bkv/handlers.go`  | 协议处理，ACK 解析       |
| `internal/protocol/bkv/voice.go`     | 控制命令编码             |
| `internal/metrics/metrics.go`        | Prometheus 指标定义      |
| `internal/app/bootstrap/app.go`      | 依赖注入                 |
| `test/e2e/charge_test.go`            | 端到端测试               |

**协议文档**：`docs/协议/设备对接指引-组网设备2024(1).txt` 章节 2.2.8

---

## 四、测试用例

### 端口 1 充电流程

```
1. API请求：POST /api/v1/charge/start {port_no:1}
2. 下行帧：  00080700000101003c0000 （插座0+插孔0）
3. 设备ACK： 0005070100000001 （成功）
4. 停止命令：00080700000001000000 （开关=0）
5. 充电上报：0011020000000000... （状态恢复空闲）
```

### 端口 2 充电流程

```
1. API请求：POST /api/v1/charge/start {port_no:2}
2. 下行帧：  00080700010101003c0000 （插座0+插孔1）
3. 设备ACK： 0005070100010001 （成功）
4. 停止命令：00080700010001000000
5. 充电上报：端口2状态=01（曾充电）
```

---

## 五、故障排查

| 问题               | 检查项   | 解决方法                |
| ------------------ | -------- | ----------------------- |
| ACK 返回失败（00） | 插座号   | 单机版必须用 0          |
|                    | 长度字段 | 参数字节数（不含 0x07） |
|                    | 插孔号   | 必须为 0 或 1           |
| 订单卡 pending     | 设备在线 | 检查 Redis 会话         |
|                    | 下行队列 | 查看 outbound_queue 表  |
|                    | ACK 解析 | 验证 result 字段处理    |
| 设备无响应         | TCP 连接 | 检查 session 状态       |
|                    | 命令格式 | 验证协议头/校验和       |
|                    | 队列堵塞 | 检查 Redis 队列         |

---
