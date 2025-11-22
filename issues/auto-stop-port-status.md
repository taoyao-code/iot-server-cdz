# Bug：按时长自动停止后端口状态仍为 0x81（充电中）

## 概要

场景：BKV 设备使用“按时长 1 分钟自动停止”模式进行充电。  
期望：  
- 充电到时后，设备自动断电并上报“充电结束”；  
- 服务器结算订单（`orders.status=3/7` 终态），端口状态收敛为空闲（`ports.status=0x09`）。  

实际：  
- 物理侧设备已经自动停止输出；  
- 服务器中 `ports.status` 仍为 `129`（`0x81`，在线+充电），没有回到 `0x09`。  

## 复现步骤（当前环境）

1. 通过测试控制台对设备 `phy_id=82241218000382`、端口 `port_no=0` 发起按时长充电：  
   - `POST /internal/test/devices/{phy_id}/charge`，`charge_mode=1`（按时长），`amount=100`，`duration=1` 分钟。  
2. 等待订单进入 `charging` 状态，端口状态变为 `0x81`（`ports.status=129`）。  
3. 不再手动发送停止命令，等待 1 分钟由设备自动停止充电。  
4. 再等待数十秒，期间设备心跳保持正常。  
5. 通过测试控制台或直接查询数据库查看端口状态：  
   - `SELECT port_no, status FROM ports p JOIN devices d ON p.device_id=d.id WHERE d.phy_id='82241218000382';`  

结果：端口 0 的 `status` 仍为 `129`，未回落到 `9`。

## 相关日志特征

1. **控制 ACK（第一个“结束”信号）存在**  
   - 设备上行 `cmd=0x0015`、短帧 `data_hex="00050701000000XX"`：  
     - `inner[0]=0x07`：控制 ACK 子命令；  
     - `inner[1]=0x01`：表示控制成功；  
     - `inner[2]=socketNo`，`inner[3]=portNo`，`inner[4..5]=business_no`。  
   - 对应 `HandleControl` 中 0x07 分支：会驱动订单流转 pending→charging 或 charging→completed，并同步端口状态（0x81/0x09）。  

2. **充电结束上报（第二个“结束”信号）在异常场景中缺失**  
   - 期望在自动停止时看到 `cmd=0x0015` 的长帧：  
     - `data[0..1]=帧长`，`data[2]=0x02`（充电结束子命令）；  
     - 后续字段包含：插座号、版本、温度、电量 `kwh_0p01`、充电时间 `durationMin`、业务号等。  
   - 实际在 19:56–19:59 的运行日志中：  
     - 没有任何新的 `cmd="0x0015"` 长数据帧（`data_len=20` 的 `001102...` 结构）；  
     - 也没有任何 `cmd="0x1000"` / `IsChargingEnd()` 对应的 BKV 子协议结束上报帧。  

3. **数据库侧没有“端口收敛为 0x09”的写入**  
   - 日志中只有两类与 `ports` 相关的写：  
     - `INSERT INTO ports ... status=129`：开始充电时被写成 `0x81`；  
     - 早前一次手动停止 + `OrderMonitor.cleanupStoppingOrders` 的 `UPDATE ports SET status=9`。  
   - 在本次“按 1 分钟自动停止”流程对应的时间窗口内：  
     - 没有任何额外的 `INSERT/UPDATE ports ... status=9`；  
     - `FinalizeOrderAndPort` / `SettleOrder` 没有触发对目标订单的终态结算。  

4. **PortStatusSyncer 的兜底逻辑未触发**  
   - `fixLonelyChargingPorts` 查询 `ports.status & 0x80 != 0 且无任何活跃订单` 的端口，仅做 `SELECT 0`，没有后续 `UpsertPortState(..., 0x09)`；  
   - `checkChargingOrdersConsistency` 仅扫描并记录不一致，没有在当前条件下触发 `autoFixInconsistency`。  

## 当前代码与已知修复

### 1. `FinalizeOrderAndPort` 的 SQL 参数类型问题（已修）

日志曾出现：

> `cleanup stopping orders: finalize failed ... could not determine data type of parameter $4 (SQLSTATE 42P08)`

原因：  
- `FinalizeOrderAndPort` 的 `UPDATE orders ... failure_reason = CASE WHEN $4 IS NOT NULL AND $1 = 6 THEN $4 ...` 始终引用 `$4`，在非失败终态场景下传 `nil` 时，PostgreSQL 无法推断 `$4` 类型。  

已修方案（`internal/storage/pg/extra.go`）：  
- 根据 `newStatus` 与 `failureReason` 是否非空，拆成两条 SQL：  
  - 失败终态且有 reason：`UPDATE orders ... failure_reason = $4`；  
  - 其他终态：不引用 `$4`，只更新 `status/end_time/updated_at`。  

这一修复保证了 `OrderMonitor.cleanupStoppingOrders` 等路径可以稳定执行，不再因 SQLSTATE 42P08 中断。

### 2. `HandleControl` 中充电结束上报被 ACK 分支“吞掉”的问题（已修）

原始逻辑（简化）：

```go
if f.IsUplink() {
    if len(f.Data) >= 2 && len(f.Data) < 64 {
        // 尝试解析控制 ACK（inner[0] == 0x07）
    } else if len(f.Data) >= 15 {
        // 长数据：充电结束上报 (ParseBKVChargingEnd)
    }
}
```

问题：  
- 设备实际发送的充电结束包是 `00 11 02 ...` 这类长度帧：`len(f.Data)=20`，`f.Data[2]=0x02`；  
- 满足 `len(f.Data) >= 2 && len(f.Data) < 64`，优先进入 ACK 分支；  
- 在 ACK 分支里 `inner[0]==0x02`（不是 0x07），不会进一步处理，直接返回；  
- 因此永远不会进入 `else if len(f.Data) >= 15` 的“充电结束上报”分支，`SettleOrder` 和端口收敛都不会发生。

现有修复（`internal/protocol/bkv/handlers.go`）：  
- 在 `HandleControl` 中 **优先识别 `f.Data[2]==0x02` 的充电结束上报**，然后才走控制 ACK 分支：

```go
if f.IsUplink() {
    if len(f.Data) >= 20 && f.Data[2] == 0x02 {
        // ParseBKVChargingEnd → SettleOrder → UpsertPortState(..., 0x09)
    } else if len(f.Data) >= 2 && len(f.Data) < 64 {
        // 原有的 0x07 控制 ACK 处理逻辑
    }
}
```

这样可以保证：
- 控制 ACK（0x07）仍按原逻辑处理；  
- 充电结束上报（0x02）不会再被 ACK 分支“吞掉”，能正确驱动订单结算和端口状态收敛。

单元测试（`go test ./internal/protocol/bkv`）在本地通过。

## 剩余不确定点（需要设备侧配合确认）

在当前提供的运行日志中，**看不到本次“按 1 分钟自动停止”场景下的 0x0015 长帧或 BKV 0x1004 帧**，因此目前仍无法确定：

1. 设备在自动停止时是否确实发送了协议文档中的“充电结束上报”（cmd=0x0015/子命令0x02 或 BKV cmd=0x1004）；  
2. 若发送了，数据格式是否完全符合现有 `ParseBKVChargingEnd`/`HandleChargingEnd` 的解析预期。

这部分需要后续在设备侧抓一帧完整的“自动停止时的上行报文”（原始 hex），才能进一步确认是否还需调整解析器。

## 后续建议

1. **设备侧抓包**  
   - 在“按时长自动停止”场景下抓取一条完整的上行报文（包含 0x0015 / 0x1000 的 raw hex），对照 `ParseBKVChargingEnd` 和 TLV 解析逻辑核对字段。  

2. **日志增强（可选）**  
   - 在 `HandleControl` 解析失败或 `ParseBKVChargingEnd` 返回错误时，输出一条 `WARN` 日志，附上 `f.Data` 的 hex，方便后续对照。  

3. **监控和验收**  
   - 在生产/测试环境中加一个简单检查：  
     - 对每条 `orders.status` 从 charging→completed/settled 的流转，验证对应的 `ports.status` 是否进入 `0x09`（idle）；  
     - 对长时间停留在 `0x81` 且无活跃订单的端口，通过 `PortStatusSyncer` 的指标 `ConsistencyLonelyPortFixTotal` 观察是否存在异常增长。  

本 issue 记录当前已知的行为、分析和代码改动，便于后续在有更多设备侧数据时继续验证和完善。

