package bkv

import (
	"context"
	"sync"

	"github.com/taoyao-code/iot-server/internal/coremodel"
)

// RegisterHandlers 注册BKV协议的所有指令处理器
// 协议规范帧命令码分类：
//
//	核心命令（协议规范定义）：
//	  0x0000 - 心跳上报/回复
//	  0x1000 - BKV兼容包（包含BKV子协议：0x1017状态上报、0x1004充电结束等）
//	  0x0005 - 网络节点列表（子命令：0x08刷新/0x09添加/0x0A删除）
//	  0x0015 - 控制命令（子命令：0x07控制/0x02结束/0x0B刷卡等）
//	  0x0007 - OTA升级
//
//	扩展命令（部分固件使用独立帧命令码，标准协议应使用0x0015+子命令）：
//	  0x000B, 0x000C, 0x000F, 0x0017, 0x0018, 0x0019, 0x001A, 0x001B, 0x001D 等
func RegisterHandlers(adapter *Adapter, handlers *Handlers) {
	// === 核心协议帧命令 ===

	// 0x0000: 心跳上报/回复 (协议文档 2.1.1)
	adapter.Register(0x0000, func(f *Frame) error {
		return handlers.HandleHeartbeat(context.Background(), f)
	})

	// 0x1000: BKV兼容包 (协议文档 2.2.2)
	// 内含BKV子协议命令：0x1017(状态上报)、0x1004(充电结束)、0x1007(控制)等
	adapter.Register(0x1000, func(f *Frame) error {
		return handlers.HandleBKVStatus(context.Background(), f)
	})

	// 0x0015: 控制命令 (协议文档 2.2.8-2.2.9)
	// 内含子命令：0x07(控制)、0x02(充电结束)、0x0B(刷卡)、0x0C(刷卡结束)等
	adapter.Register(0x0015, func(f *Frame) error {
		return handlers.HandleControl(context.Background(), f)
	})

	// 0x0005: 网络节点列表 (协议文档 2.2.5-2.2.7)
	// 内含子命令：0x08(刷新列表)、0x09(添加节点)、0x0A(删除节点)
	adapter.Register(0x0005, func(f *Frame) error {
		return handlers.HandleNetworkList(context.Background(), f)
	})

	// 0x0007: OTA升级 (协议文档OTA章节)
	adapter.Register(0x0007, func(f *Frame) error {
		// 根据数据长度判断是响应还是进度
		if len(f.Data) >= 3 && len(f.Data) <= 10 {
			return handlers.HandleOTAResponse(context.Background(), f)
		} else if len(f.Data) >= 4 {
			return handlers.HandleOTAProgress(context.Background(), f)
		}
		return handlers.HandleGeneric(context.Background(), f)
	})

	// === 扩展帧命令（兼容使用独立帧命令码的固件变体） ===
	// 注意：标准协议应使用0x0015帧+子命令实现这些功能

	// 0x000B: [扩展] 刷卡上报 - 标准协议使用0x0015+子命令0x0B
	adapter.Register(0x000B, func(f *Frame) error {
		return handlers.HandleCardSwipe(context.Background(), f)
	})

	// 0x000C: [扩展] 刷卡充电结束 - 标准协议使用0x0015+子命令0x0C
	adapter.Register(0x000C, func(f *Frame) error {
		return handlers.HandleChargeEnd(context.Background(), f)
	})

	// 0x000F: [扩展] 订单确认 - 标准协议使用0x0015+子命令0x0F
	adapter.Register(0x000F, func(f *Frame) error {
		return handlers.HandleOrderConfirm(context.Background(), f)
	})

	// 0x0017: [扩展] 按功率下发充电 - 标准协议使用0x0015+子命令0x17
	// (协议文档 2.2.1 按功率下发充电命令)
	adapter.Register(0x0017, func(f *Frame) error {
		return handlers.HandleControl(context.Background(), f)
	})

	// 0x0018: [扩展] 按功率充电结束 - 标准协议使用0x0015+子命令0x18
	adapter.Register(0x0018, func(f *Frame) error {
		return handlers.HandlePowerLevelEnd(context.Background(), f)
	})

	// 0x0019: [扩展] 服务费充电结束
	adapter.Register(0x0019, func(f *Frame) error {
		return handlers.HandleServiceFeeEnd(context.Background(), f)
	})

	// 0x001A: [扩展] 余额查询 - 标准协议使用0x0015+子命令0x1A
	adapter.Register(0x001A, func(f *Frame) error {
		return handlers.HandleBalanceQuery(context.Background(), f)
	})

	// 0x001B: [扩展] 语音配置 - 标准协议使用0x0015+子命令0x1B
	adapter.Register(0x001B, func(f *Frame) error {
		return handlers.HandleVoiceConfigResponse(context.Background(), f)
	})

	// 0x000D/0x000E/0x001D: [扩展] 查询插座状态响应
	adapter.Register(0x000D, func(f *Frame) error {
		return handlers.HandleSocketStateResponse(context.Background(), f)
	})
	adapter.Register(0x000E, func(f *Frame) error {
		return handlers.HandleSocketStateResponse(context.Background(), f)
	})
	adapter.Register(0x001D, func(f *Frame) error {
		return handlers.HandleSocketStateResponse(context.Background(), f)
	})

	// === 非标准扩展帧命令（参数管理） ===

	// 0x0001: [非标准] 批量读取参数响应
	adapter.Register(0x0001, func(f *Frame) error {
		return handlers.HandleParamReadResponse(context.Background(), f)
	})

	// 0x0002: [非标准] 批量写入参数响应
	adapter.Register(0x0002, func(f *Frame) error {
		return handlers.HandleParamWriteResponse(context.Background(), f)
	})

	// 0x0003: [非标准] 参数同步响应
	adapter.Register(0x0003, func(f *Frame) error {
		return handlers.HandleParamSyncResponse(context.Background(), f)
	})

	// 0x0004: [非标准] 参数重置响应
	adapter.Register(0x0004, func(f *Frame) error {
		return handlers.HandleParamResetResponse(context.Background(), f)
	})
}

// NewBKVProtocol 创建完整的BKV协议实例
func NewBKVProtocol(reasonMap *ReasonMap) *Adapter {
	adapter := NewAdapter()
	handlers := &Handlers{
		Reason:     reasonMap,
		CoreEvents: &nopEventSink{},
		sessions:   &sync.Map{}, // 避免控制/订单确认路径的 nil panic
	}

	RegisterHandlers(adapter, handlers)
	return adapter
}

// FrameCommands 帧命令码列表（协议规范2024）
// 注意：这是包头中的 Frame.Cmd，不是 BKV 子命令码！
var FrameCommands = map[uint16]string{
	// === 协议规范定义的帧命令码 ===
	0x0000: "心跳上报/回复 (2.1.1)",
	0x1000: "BKV兼容包 (2.2.2) - 包含BKV子协议",
	0x0005: "网络节点列表 (2.2.5-2.2.7) - 子命令: 0x08刷新/0x09添加/0x0A删除",
	0x0015: "控制命令 (2.2.4/2.2.8/2.2.9/进阶) - 子命令: 0x02/0x18(充电结束)、0x07/0x17(控制)、0x0B/0x0C/0x0F(刷卡流程)、0x1A(余额)、0x1B(语音)、0x1D(状态查询)",
	0x0007: "OTA升级 (OTA章节)",

	// === 扩展帧命令码（部分固件使用独立帧命令码） ===
	// 注意：协议规范中这些功能使用0x0015帧+子命令实现
	// 以下注册是为了兼容使用独立帧命令码的固件变体
	0x000B: "[扩展] 刷卡充电 - 标准协议使用0x0015+子命令0x0B",
	0x000C: "[扩展] 刷卡充电结束 - 标准协议使用0x0015+子命令0x0C",
	0x000F: "[扩展] 订单确认 - 标准协议使用0x0015+子命令0x0F",
	0x0017: "[扩展] 按功率充电 - 标准协议使用0x0015+子命令0x17",
	0x0018: "[扩展] 按功率充电结束 - 标准协议使用0x0015+子命令0x18",
	0x0019: "[扩展] 服务费充电",
	0x001A: "[扩展] 余额查询 - 标准协议使用0x0015+子命令0x1A",
	0x001B: "[扩展] 语音配置 - 标准协议使用0x0015+子命令0x1B",
	0x001D: "[扩展] 查询插座状态 - 标准协议使用0x0015+子命令0x1D",

	// === 非标准扩展 ===
	0x0001: "[非标准] 批量读取参数",
	0x0002: "[非标准] 批量写入参数",
	0x0003: "[非标准] 参数同步",
	0x0004: "[非标准] 参数重置",
	0x000D: "[非标准] 查询插座状态",
	0x000E: "[非标准] 查询插座状态",
}

// BKVSubCommands BKV子命令码列表（在0x1000帧中的BKVPayload.Cmd）
// 注意：这些是TLV解析后的BKV子协议命令码，不是帧命令码！
var BKVSubCommands = map[uint16]string{
	0x1017: "插座状态上报 (2.2.3) - 包含 tag=0x65+value=0x94",
	0x1004: "BKV充电结束上报 (服务费模式)",
	0x1007: "BKV控制命令 (服务费模式)",
	0x1010: "异常事件上报 (2.2.8进阶)",
	0x1011: "插座系统参数设置 (2.2.6进阶)",
	0x1012: "插座系统参数查询 (2.2.7进阶)",
	0x1013: "参数设置ACK - 注意：不包含状态字段！",
}

// BKVCommands 兼容旧代码的别名
// Deprecated: 请使用 FrameCommands 或 BKVSubCommands
var BKVCommands = FrameCommands

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
