package bkv

import "context"

// RegisterHandlers 注册BKV协议的所有指令处理器
func RegisterHandlers(adapter *Adapter, handlers *Handlers) {
	// 心跳相关
	adapter.Register(0x0000, func(f *Frame) error {
		return handlers.HandleHeartbeat(context.Background(), f)
	})

	// BKV子协议 (插座状态上报、控制等)
	adapter.Register(0x1000, func(f *Frame) error {
		return handlers.HandleBKVStatus(context.Background(), f)
	})

	// 控制指令
	adapter.Register(0x0015, func(f *Frame) error {
		return handlers.HandleControl(context.Background(), f)
	})

	// 参数查询指令 (协议文档中提到的)
	adapter.Register(0x0005, func(f *Frame) error {
		return handlers.HandleGeneric(context.Background(), f)
	})

	// OTA升级指令
	adapter.Register(0x0007, func(f *Frame) error {
		return handlers.HandleGeneric(context.Background(), f)
	})

	// Week4: 刷卡充电相关
	// 0x0B: 刷卡上报/下发充电指令
	adapter.Register(0x000B, func(f *Frame) error {
		return handlers.HandleCardSwipe(context.Background(), f)
	})

	// 0x0F: 订单确认
	adapter.Register(0x000F, func(f *Frame) error {
		return handlers.HandleOrderConfirm(context.Background(), f)
	})

	// 0x0C: 充电结束上报/确认
	adapter.Register(0x000C, func(f *Frame) error {
		return handlers.HandleChargeEnd(context.Background(), f)
	})

	// 0x1A: 余额查询
	adapter.Register(0x001A, func(f *Frame) error {
		return handlers.HandleBalanceQuery(context.Background(), f)
	})
}

// NewBKVProtocol 创建完整的BKV协议实例
func NewBKVProtocol(repo repoAPI, reasonMap *ReasonMap) *Adapter {
	adapter := NewAdapter()
	handlers := &Handlers{
		Repo:   repo,
		Reason: reasonMap,
	}

	RegisterHandlers(adapter, handlers)
	return adapter
}

// BKVCommands BKV协议支持的命令列表
var BKVCommands = map[uint16]string{
	0x0000: "心跳上报/回复",
	0x1000: "BKV子协议数据 (插座状态上报等)",
	0x0015: "控制设备 (按时/按量/按功率)",
	0x0005: "网络节点列表相关",
	0x0007: "OTA升级",
	0x000B: "刷卡上报/下发充电指令",
	0x000C: "充电结束上报/确认",
	0x000F: "订单确认",
	0x001A: "余额查询",
}

// IsBKVCommand 判断是否为BKV协议支持的命令
func IsBKVCommand(cmd uint16) bool {
	_, exists := BKVCommands[cmd]
	return exists
}
