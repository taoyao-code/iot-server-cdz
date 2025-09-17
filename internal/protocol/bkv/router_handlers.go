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
}

// IsBKVCommand 判断是否为BKV协议支持的命令
func IsBKVCommand(cmd uint16) bool {
	_, exists := BKVCommands[cmd]
	return exists
}