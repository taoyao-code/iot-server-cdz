package storage

import (
	"context"
	"time"

	"github.com/taoyao-code/iot-server/internal/storage/models"
)

// CoreRepo 面向“中间件核心”的存储抽象。
// 约束：
// - 禁止上层直接写 SQL，统一通过本接口访问
// - 实现需要提供事务封装 WithTx，保证核心路径原子性
// - 接口必须保持 DB-agnostic（面向模型与基础类型）
type CoreRepo interface {
	// ---------- 事务 ----------
	// WithTx 在单个事务中执行 fn，fn 内使用 repo 执行的所有写入/读取都在同一事务中。
	// 实现应保证嵌套调用正确复用当前事务。
	WithTx(ctx context.Context, fn func(repo CoreRepo) error) error

	// ---------- 设备 ----------
	// EnsureDevice 若 phyID 不存在则创建，返回设备记录
	EnsureDevice(ctx context.Context, phyID string) (*models.Device, error)
	// TouchDeviceLastSeen 刷新设备最近心跳时间（若设备不存在可选择创建或返回错误，由实现决定并在设计中固定）
	TouchDeviceLastSeen(ctx context.Context, phyID string, at time.Time) error
	// GetDeviceByPhyID 通过物理 ID 查询设备
	GetDeviceByPhyID(ctx context.Context, phyID string) (*models.Device, error)
	// ListDevices 简单列表示例（仅用于管理/调试）
	ListDevices(ctx context.Context, limit, offset int) ([]models.Device, error)

	// ---------- 端口 ----------
	// UpsertPortSnapshot 更新或插入端口快照（与 BKV 位图保持一致）
	UpsertPortSnapshot(ctx context.Context, deviceID int64, portNo int32, status int32, powerW *int32, updatedAt time.Time) error
	// GetPort 读取端口信息
	GetPort(ctx context.Context, deviceID int64, portNo int32) (*models.Port, error)
	// UpdatePortStatus 仅更新端口状态位（不改 updated_at 以外字段）
	UpdatePortStatus(ctx context.Context, deviceID int64, portNo int32, status int32) error

	// ---------- 订单 ----------
	// CreateOrder 创建订单（要求调用方填充必要字段：DeviceID/PortNo/OrderNo/...）
	CreateOrder(ctx context.Context, order *models.Order) error
	// GetActiveOrder 获取指定设备端口的活动订单（若使用 status 字段区分活动/非活动，由实现定义判定条件）
	GetActiveOrder(ctx context.Context, deviceID int64, portNo int32) (*models.Order, error)
	// GetOrderByOrderNo 通过订单号查询订单
	GetOrderByOrderNo(ctx context.Context, orderNo string) (*models.Order, error)
	// GetOrderByBusinessNo 通过 business_no 查询订单
	GetOrderByBusinessNo(ctx context.Context, deviceID int64, businessNo int32) (*models.Order, error)
	// UpdateOrderStatus 更新订单状态
	UpdateOrderStatus(ctx context.Context, orderID int64, status int32) error
	// CompleteOrder 完成订单（写入结束原因、时间、累计电量/金额等；实现内保证与端口状态一致性可选在同一事务完成）
	CompleteOrder(ctx context.Context, deviceID int64, portNo int32, endReason int32, endTime time.Time, amountCent *int64, kwh0p01 *int64) error
	// SettleOrder 结算订单（兼容 business_no 与 order_no 两种匹配方式）
	// 语义等价于原 pg 仓储中的 SettleOrder：优先按 device_id+port_no+business_no 更新，
	// 若无匹配则按 order_no upsert（创建或更新）终态订单。
	SettleOrder(ctx context.Context, deviceID int64, portNo int, orderHex string, durationSec int, kwh0p01 int, reason int) error
	// UpsertOrderProgress 插入或更新订单进度（基于 order_no 冲突覆盖状态/进度）
	UpsertOrderProgress(ctx context.Context, deviceID int64, portNo int32, orderNo string, businessNo *int32, durationSec int32, kwh0p01 int32, status int32, powerW01 *int32) error

	// ---------- 指令日志 ----------
	// AppendCmdLog 追加一条上下行指令日志
	AppendCmdLog(ctx context.Context, log *models.CmdLog) error
	// ListRecentCmdLogs 读取设备最近的日志
	ListRecentCmdLogs(ctx context.Context, deviceID int64, limit int) ([]models.CmdLog, error)

	// ---------- 下行队列 ----------
	// EnqueueOutbound 入队一条下行消息，返回消息 ID
	EnqueueOutbound(ctx context.Context, msg *models.OutboundMessage) (int64, error)
	// DequeuePendingForDevice 出队（或查询）设备的待发送消息（仅选择 status=0 且 not_before 窗口内的消息）
	DequeuePendingForDevice(ctx context.Context, deviceID int64, limit int) ([]models.OutboundMessage, error)
	// MarkOutboundSent 标记已发送
	MarkOutboundSent(ctx context.Context, id int64) error
	// MarkOutboundDone 标记完成
	MarkOutboundDone(ctx context.Context, id int64) error
	// MarkOutboundFailed 标记失败并记录错误
	MarkOutboundFailed(ctx context.Context, id int64, lastError string) error
}
