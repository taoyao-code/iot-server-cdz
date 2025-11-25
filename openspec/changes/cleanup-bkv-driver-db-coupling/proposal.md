# Change: 移除 BKV 驱动侧直连 DB 及重复落库，回到规范化事件边界

## Why
- 0x0015 充电结束帧被驱动直接写入端口快照（0xB0）再由 SessionEnded 写回 0x90，端口状态抖动，第三方查询得到“充电中”假象。
- SettleOrder 在找不到业务号时按 order_no=业务号插入幽灵订单，真实订单未终态化。
- 驱动层存在双仓储（gorm + pg）和多处 EnsureDevice/TouchDeviceLastSeen/端口/订单写库，违反“驱动不访问核心 DB”的规范，制造延迟和数据不一致。
- ACK 解析错误写入 port=1 等无关端口，放大数据噪声。

## What Changes
- 删掉 BKV 结束帧路径中驱动侧对 ports/orders 的直写（PortSnapshot 0xB0、fallback 造单、ACK 落库），端口收敛只由 SessionEnded.NextPortStatus 驱动。
- 移除 SettleOrder 的 fallback 插入/按 order_no 伪造记录，要求必须匹配已有业务号或订单。
- 精简驱动存储访问：禁用 gormrepo 路径，驱动仅发规范化事件，不再直接调用 CoreRepo/pg 仓储。
- 移除多余的 EnsureDevice/TouchDeviceLastSeen 写库，避免每帧写 DB；清理 ACK 写端口的逻辑。

## Impact
- 影响模块：BKV 驱动事件处理（handlers_helper.go、handlers.go）、命令结算（storage/gormrepo SettleOrder）、驱动存储接入（app/bootstrap 注入路径）。
- 规范对齐：符合 add-middleware-core-thing-model-driver-api 对“驱动-核心事件边界”和“驱动不得直连 DB”的要求。
