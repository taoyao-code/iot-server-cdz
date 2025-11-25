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

	// ---------- 网关插座映射 ----------
	// UpsertGatewaySocket 写入或更新网关插座映射（按 gateway_id+socket_no 唯一键）
	UpsertGatewaySocket(ctx context.Context, socket *models.GatewaySocket) error
	// GetGatewaySocketByUID 通过 socket_uid 查询映射（若无返回 ErrRecordNotFound）
	GetGatewaySocketByUID(ctx context.Context, uid string) (*models.GatewaySocket, error)

	// ---------- 端口 ----------
	// UpsertPortSnapshot 更新或插入端口快照（与 BKV 位图保持一致）
	UpsertPortSnapshot(ctx context.Context, deviceID int64, portNo int32, status int32, powerW *int32, updatedAt time.Time) error
	// GetPort 读取端口信息
	GetPort(ctx context.Context, deviceID int64, portNo int32) (*models.Port, error)
	// UpdatePortStatus 仅更新端口状态位（不改 updated_at 以外字段）
	UpdatePortStatus(ctx context.Context, deviceID int64, portNo int32, status int32) error
	// LockOrCreatePort 行锁定端口记录，不存在则创建
	LockOrCreatePort(ctx context.Context, deviceID int64, portNo int32) (*models.Port, error)

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
