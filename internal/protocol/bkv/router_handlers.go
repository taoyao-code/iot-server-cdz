package bkv

import (
	"context"

	"github.com/taoyao-code/iot-server/internal/coremodel"
)

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
		return handlers.HandleNetworkList(context.Background(), f)
	}) // 0x0005: 网络节点列表相关 (2.2.5/2.2.6 ACK)

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

	// Week 6: 组网管理
	// 0x08: 刷新插座列表
	adapter.Register(0x0008, func(f *Frame) error {
		return handlers.HandleNetworkRefresh(context.Background(), f)
	})

	// 0x09: 添加插座
	adapter.Register(0x0009, func(f *Frame) error {
		return handlers.HandleNetworkAddNode(context.Background(), f)
	})

	// 0x0A: 删除插座
	adapter.Register(0x000A, func(f *Frame) error {
		return handlers.HandleNetworkDeleteNode(context.Background(), f)
	})

	// Week 7: OTA升级
	// 0x07: OTA升级响应+进度上报（上行）
	adapter.Register(0x0007, func(f *Frame) error {
		// 根据数据长度判断是响应还是进度
		if len(f.Data) >= 3 && len(f.Data) <= 10 {
			return handlers.HandleOTAResponse(context.Background(), f)
		} else if len(f.Data) >= 4 {
			return handlers.HandleOTAProgress(context.Background(), f)
		}
		return handlers.HandleGeneric(context.Background(), f)
	})

	// Week 8: 按功率分档充电
	// 0x17: 按功率分档充电命令（下行，由API触发）
	// 0x18: 按功率充电结束上报（上行）
	adapter.Register(0x0018, func(f *Frame) error {
		return handlers.HandlePowerLevelEnd(context.Background(), f)
	})

	// Week 9: 参数管理
	// 0x01: 批量读取参数（上行响应）
	adapter.Register(0x0001, func(f *Frame) error {
		return handlers.HandleParamReadResponse(context.Background(), f)
	})

	// 0x02: 批量写入参数（上行响应）
	adapter.Register(0x0002, func(f *Frame) error {
		return handlers.HandleParamWriteResponse(context.Background(), f)
	})

	// 0x03: 参数同步（上行响应）
	adapter.Register(0x0003, func(f *Frame) error {
		return handlers.HandleParamSyncResponse(context.Background(), f)
	})

	// 0x04: 参数重置（上行响应）
	adapter.Register(0x0004, func(f *Frame) error {
		return handlers.HandleParamResetResponse(context.Background(), f)
	})

	// Week 10: 扩展功能
	// 0x1B: 语音配置（上行响应）
	adapter.Register(0x001B, func(f *Frame) error {
		return handlers.HandleVoiceConfigResponse(context.Background(), f)
	})

	// 0x0D/0x0E/0x1D: 查询插座状态（上行响应）
	adapter.Register(0x000D, func(f *Frame) error {
		return handlers.HandleSocketStateResponse(context.Background(), f)
	})
	adapter.Register(0x000E, func(f *Frame) error {
		return handlers.HandleSocketStateResponse(context.Background(), f)
	})
	adapter.Register(0x001D, func(f *Frame) error {
		return handlers.HandleSocketStateResponse(context.Background(), f)
	})

	// 0x19: 服务费充电结束（上行）
	adapter.Register(0x0019, func(f *Frame) error {
		return handlers.HandleServiceFeeEnd(context.Background(), f)
	})
}

// NewBKVProtocol 创建完整的BKV协议实例
func NewBKVProtocol(reasonMap *ReasonMap) *Adapter {
	adapter := NewAdapter()
	handlers := &Handlers{
		Reason:     reasonMap,
		CoreEvents: &nopEventSink{},
	}

	RegisterHandlers(adapter, handlers)
	return adapter
}

// BKVCommands BKV协议支持的命令列表
var BKVCommands = map[uint16]string{
	0x0000: "心跳上报/回复",
	0x0001: "批量读取参数 (下行/上行)",
	0x0002: "批量写入参数 (下行/上行)",
	0x0003: "参数同步 (下行/上行)",
	0x0004: "参数重置 (下行/上行)",
	0x0005: "网络节点列表相关",
	0x0007: "OTA升级 (下行/上行)",
	0x0008: "刷新插座列表 (组网管理)",
	0x0009: "添加插座 (组网管理)",
	0x000A: "删除插座 (组网管理)",
	0x000B: "刷卡上报/下发充电指令",
	0x000C: "充电结束上报/确认",
	0x000D: "查询插座状态 (下行/上行)",
	0x000E: "查询插座状态 (下行/上行)",
	0x000F: "订单确认",
	0x0015: "控制设备 (按时/按量/按功率)",
	0x0017: "按功率分档充电 (下行)",
	0x0018: "按功率充电结束上报 (上行)",
	0x0019: "服务费充电 (下行/上行)",
	0x001A: "余额查询",
	0x001B: "语音配置 (下行/上行)",
	0x001D: "查询插座状态 (下行/上行)",
	0x1000: "BKV子协议数据 (插座状态上报等)",
	0x1017: "插座状态上报 (上行)",
}

// IsBKVCommand 判断是否为BKV协议支持的命令
func IsBKVCommand(cmd uint16) bool {
	_, exists := BKVCommands[cmd]
	return exists
}

// nopEventSink 用于协议层测试：丢弃所有核心事件。
type nopEventSink struct{}

func (n *nopEventSink) HandleCoreEvent(_ context.Context, _ *coremodel.CoreEvent) error {
	return nil
}
