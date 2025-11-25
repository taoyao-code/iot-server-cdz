## 1. 实现
- [x] 1.1 删除 `internal/protocol/bkv/handlers_helper.go` 中充电结束路径的 PortSnapshot 写入（0xB0/原始状态），端口收敛仅依赖 SessionEnded.NextPortStatus。
- [x] 1.2 删除/禁用 `storage/gormrepo.Repository.SettleOrder` 中按 order_no=业务号的 fallback 更新/插入逻辑，未匹配业务号/订单号时直接返回错误并记录日志，不造单。
- [ ] 1.3 停用驱动直连 DB：去除 BKV 事件路径的 EnsureDevice/TouchDeviceLastSeen/端口写库调用，驱动仅通过事件/命令与核心交互；禁止 gorm+pg 双写（app/bootstrap 中仅注入一套 CoreRepo 到驱动）。**状态：未完成，驱动仍注入 CoreRepo 并由 driver_core 写库。**
- [x] 1.4 移除 ACK/控制类端口写库（handleControlUplinkStatus 等）以防误写端口（如 port=1）；保留必要日志，不写表。
- [ ] 1.5 评估/处理 `handleControlChargingProgress`：若 end.Status 含充电位导致 PortSnapshot 推回充电态，需限制或移除。**状态：待确认。**

## 2. 规范与验证
- [x] 2.1 更新 specs（已添加）：驱动不落库、不造单，端口收敛单一路径，禁止 fallback 订单。
- [ ] 2.2 回归测试：覆盖 0x0015/0x1004 结束帧，验证仅写 0x90、不生成幽灵订单、不写错端口；`go test ./internal/protocol/bkv ./internal/app -run ChargingEnd|SessionEnded`（或同等用例）。**状态：待执行。**
- [x] 2.3 openspec validate cleanup-bkv-driver-db-coupling --strict

## 3. 额外发现和修复
- [x] 3.1 修复 `internal/storage/pg/repo.go:SettleOrder` 中同样存在的 fallback 造单逻辑（AP3000 协议使用）
- [x] 3.2 确认 `InsertFallbackOrder` 无调用方，保留用于审计目的
- [x] 3.3 确认 `driver_core.go` 中的写库逻辑为核心层正常业务逻辑；若需与规范解耦，另开任务收敛驱动注入。
