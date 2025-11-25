## 1. 实现
- [ ] 1.1 删除 `internal/protocol/bkv/handlers_helper.go` 中充电结束路径的 PortSnapshot 写入（0xB0/原始状态），端口收敛仅依赖 SessionEnded.NextPortStatus。
- [ ] 1.2 删除/禁用 `storage/gormrepo.Repository.SettleOrder` 中按 order_no=业务号的 fallback 更新/插入逻辑，未匹配业务号/订单号时直接返回错误并记录日志，不造单。
- [ ] 1.3 停用驱动直连 DB：去除 BKV 事件路径的 EnsureDevice/TouchDeviceLastSeen/端口写库调用，驱动仅通过事件/命令与核心交互；禁止 gorm+pg 双写（app/bootstrap 中仅注入一套 CoreRepo 到驱动）。
- [ ] 1.4 移除 ACK/控制类端口写库（handleControlUplinkStatus 等）以防误写端口（如 port=1）；保留必要日志，不写表。

## 2. 规范与验证
- [ ] 2.1 更新 specs（已添加）：驱动不落库、不造单，端口收敛单一路径，禁止 fallback 订单。
- [ ] 2.2 回归测试：覆盖 0x0015/0x1004 结束帧，验证仅写 0x90、不生成幽灵订单、不写错端口；`go test ./internal/protocol/bkv ./internal/app -run ChargingEnd|SessionEnded`（或同等用例）。
- [ ] 2.3 openspec validate cleanup-bkv-driver-db-coupling --strict
