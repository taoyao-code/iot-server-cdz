package gormrepo

import (
	"context"
	"errors"
	"strconv"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/taoyao-code/iot-server/internal/storage"
	"github.com/taoyao-code/iot-server/internal/storage/models"
)

// Repository 基于 GORM 的 CoreRepo 实现。
// 使用 isTx 标记区分事务上下文，避免嵌套事务重复 Begin/Commit。
type Repository struct {
	db   *gorm.DB
	isTx bool
}

// New 返回一个使用给定 *gorm.DB 的 CoreRepo 实例。
func New(db *gorm.DB) storage.CoreRepo {
	return &Repository{db: db}
}

// WithTx 复用现有事务或开启新事务执行 fn。
func (r *Repository) WithTx(ctx context.Context, fn func(storage.CoreRepo) error) error {
	if r.isTx {
		return fn(r)
	}

	tx := r.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return tx.Error
	}

	child := &Repository{db: tx, isTx: true}
	if err := fn(child); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit().Error
}

// EnsureDevice 若设备不存在则插入，存在则刷新 updated_at。
func (r *Repository) EnsureDevice(ctx context.Context, phyID string) (*models.Device, error) {
	now := time.Now()
	record := &models.Device{
		PhyID:      phyID,
		LastSeenAt: &now,
	}

	err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "phy_id"}},
			DoUpdates: clause.Assignments(map[string]interface{}{"updated_at": gorm.Expr("NOW()")}),
		}).
		Create(record).Error
	if err != nil {
		return nil, err
	}

	return r.GetDeviceByPhyID(ctx, phyID)
}

// TouchDeviceLastSeen 刷新设备 last_seen_at（不存在则插入）。
func (r *Repository) TouchDeviceLastSeen(ctx context.Context, phyID string, at time.Time) error {
	ts := at
	record := &models.Device{
		PhyID:      phyID,
		LastSeenAt: &ts,
	}

	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "phy_id"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"last_seen_at": gorm.Expr("excluded.last_seen_at"),
				"updated_at":   gorm.Expr("NOW()"),
			}),
		}).
		Create(record).Error
}

// GetDeviceByPhyID 通过物理 ID 查询设备。
func (r *Repository) GetDeviceByPhyID(ctx context.Context, phyID string) (*models.Device, error) {
	var device models.Device
	err := r.db.WithContext(ctx).Where("phy_id = ?", phyID).First(&device).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	return &device, err
}

// ListDevices 分页返回设备列表，按 id 倒序。
func (r *Repository) ListDevices(ctx context.Context, limit, offset int) ([]models.Device, error) {
	var devices []models.Device
	q := r.db.WithContext(ctx).Order("id DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if offset > 0 {
		q = q.Offset(offset)
	}
	if err := q.Find(&devices).Error; err != nil {
		return nil, err
	}
	return devices, nil
}

// UpsertPortSnapshot 写入端口快照，冲突时更新状态/功率/时间。
func (r *Repository) UpsertPortSnapshot(ctx context.Context, deviceID int64, portNo int32, status int32, powerW *int32, updatedAt time.Time) error {
	record := &models.Port{
		DeviceID:  deviceID,
		PortNo:    portNo,
		Status:    status,
		PowerW:    powerW,
		UpdatedAt: updatedAt,
	}

	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "device_id"}, {Name: "port_no"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"status":     gorm.Expr("excluded.status"),
				"power_w":    gorm.Expr("excluded.power_w"),
				"updated_at": gorm.Expr("excluded.updated_at"),
			}),
		}).
		Create(record).Error
}

// GetPort 获取指定端口信息。
func (r *Repository) GetPort(ctx context.Context, deviceID int64, portNo int32) (*models.Port, error) {
	var port models.Port
	err := r.db.WithContext(ctx).
		Where("device_id = ? AND port_no = ?", deviceID, portNo).
		First(&port).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	return &port, err
}

// UpdatePortStatus 仅更新端口状态位。
func (r *Repository) UpdatePortStatus(ctx context.Context, deviceID int64, portNo int32, status int32) error {
	res := r.db.WithContext(ctx).
		Model(&models.Port{}).
		Where("device_id = ? AND port_no = ?", deviceID, portNo).
		Updates(map[string]interface{}{
			"status":     status,
			"updated_at": gorm.Expr("NOW()"),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// CleanupPendingOrders 标记超时 pending 订单为完成状态。
func (r *Repository) CleanupPendingOrders(ctx context.Context, deviceID int64, before time.Time) (int64, error) {
	const pendingStatus int32 = 0
	res := r.db.WithContext(ctx).
		Model(&models.Order{}).
		Where("device_id = ? AND status = ? AND created_at < ?", deviceID, pendingStatus, before).
		Updates(map[string]interface{}{
			"status":     3,
			"updated_at": gorm.Expr("NOW()"),
		})
	if res.Error != nil {
		return 0, res.Error
	}
	return res.RowsAffected, nil
}

// LockActiveOrderForPort 锁定指定端口的活跃订单
func (r *Repository) LockActiveOrderForPort(ctx context.Context, deviceID int64, portNo int32) (*models.Order, bool, error) {
	var order models.Order
	res := r.db.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
		Where("device_id = ? AND port_no = ? AND status IN ?", deviceID, portNo, lockOrderStatuses).
		Order("created_at DESC").
		Limit(1).
		Find(&order)
	if res.Error != nil {
		return nil, false, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, false, nil
	}
	return &order, true, nil
}

// LockOrCreatePort 行锁定端口记录，若不存在则创建后返回
func (r *Repository) LockOrCreatePort(ctx context.Context, deviceID int64, portNo int32) (*models.Port, error) {
	var port models.Port
	res := r.db.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("device_id = ? AND port_no = ?", deviceID, portNo).
		Take(&port)
	if res.Error != nil {
		if errors.Is(res.Error, gorm.ErrRecordNotFound) {
			port = models.Port{
				DeviceID:  deviceID,
				PortNo:    portNo,
				Status:    0,
				UpdatedAt: time.Now(),
			}
			if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&port).Error; err != nil {
				return nil, err
			}
			res = r.db.WithContext(ctx).
				Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("device_id = ? AND port_no = ?", deviceID, portNo).
				Take(&port)
			if res.Error != nil {
				return nil, res.Error
			}
			return &port, nil
		}
		return nil, res.Error
	}
	return &port, nil
}

// CreateOrder 创建订单记录。
func (r *Repository) CreateOrder(ctx context.Context, order *models.Order) error {
	return r.db.WithContext(ctx).Create(order).Error
}

// activeOrderStatuses 标记进行中订单状态。
var activeOrderStatuses = []int32{0, 1, 2}
var lockOrderStatuses = []int32{0, 1, 2, 8, 9, 10}

// GetActiveOrder 返回设备端口最近的活跃订单。
func (r *Repository) GetActiveOrder(ctx context.Context, deviceID int64, portNo int32) (*models.Order, error) {
	var order models.Order
	err := r.db.WithContext(ctx).
		Where("device_id = ? AND port_no = ? AND status IN ?", deviceID, portNo, activeOrderStatuses).
		Order("created_at DESC").
		First(&order).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	return &order, err
}

// GetOrderByOrderNo 根据订单号查询。
func (r *Repository) GetOrderByOrderNo(ctx context.Context, orderNo string) (*models.Order, error) {
	var order models.Order
	err := r.db.WithContext(ctx).Where("order_no = ?", orderNo).First(&order).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	return &order, err
}

// UpdateOrderStatus 更新订单状态。
func (r *Repository) UpdateOrderStatus(ctx context.Context, orderID int64, status int32) error {
	res := r.db.WithContext(ctx).
		Model(&models.Order{}).
		Where("id = ?", orderID).
		Updates(map[string]interface{}{
			"status":     status,
			"updated_at": gorm.Expr("NOW()"),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// CompleteOrder 完成订单并写入结束信息。
func (r *Repository) CompleteOrder(ctx context.Context, deviceID int64, portNo int32, endReason int32, endTime time.Time, amountCent *int64, kwh0p01 *int64) error {
	updates := map[string]interface{}{
		"end_time":   endTime,
		"end_reason": endReason,
		"status":     3,
		"updated_at": gorm.Expr("NOW()"),
	}
	if amountCent != nil {
		updates["amount_cent"] = amountCent
	}
	if kwh0p01 != nil {
		updates["kwh_0p01"] = kwh0p01
	}

	res := r.db.WithContext(ctx).
		Model(&models.Order{}).
		Where("device_id = ? AND port_no = ?", deviceID, portNo).
		Where("status IN ?", []int32{2, 9}).
		Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// SettleOrder 结算订单（兼容 business_no 与 order_no 两种匹配方式）。
// 语义等价于 internal/storage/pg.Repository.SettleOrder。
func (r *Repository) SettleOrder(ctx context.Context, deviceID int64, portNo int, orderHex string, durationSec int, kwh0p01 int, reason int) error {
	// 1) 尝试按 business_no 更新（第三方 StartCharge 流程）
	if biz, err := strconv.ParseInt(orderHex, 16, 32); err == nil {
		const updateByBiz = `
UPDATE orders
SET end_time   = NOW(),
    kwh_0p01   = ?,
    end_reason = ?,
    status     = 3,
    updated_at = NOW()
WHERE device_id   = ?
  AND port_no     = ?
  AND business_no = ?`
		res := r.db.WithContext(ctx).Exec(updateByBiz, kwh0p01, reason, deviceID, portNo, int(biz))
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected > 0 {
			return nil
		}
	}

	// 2) 回退路径：按 order_no upsert（兼容老协议/测试用例）
	const upsertByOrderNo = `
INSERT INTO orders (device_id, port_no, order_no, start_time, end_time, kwh_0p01, end_reason, status)
VALUES (?, ?, ?, NOW()-make_interval(secs => ?), NOW(), ?, ?, 3)
ON CONFLICT (order_no)
DO UPDATE SET end_time=NOW(), kwh_0p01=EXCLUDED.kwh_0p01, end_reason=EXCLUDED.end_reason, status=3, updated_at=NOW()`

	res := r.db.WithContext(ctx).Exec(upsertByOrderNo, deviceID, portNo, orderHex, durationSec, kwh0p01, reason)
	return res.Error
}

// AppendCmdLog 写入指令日志。
func (r *Repository) AppendCmdLog(ctx context.Context, log *models.CmdLog) error {
	return r.db.WithContext(ctx).Create(log).Error
}

// ListRecentCmdLogs 返回设备最近的指令日志。
func (r *Repository) ListRecentCmdLogs(ctx context.Context, deviceID int64, limit int) ([]models.CmdLog, error) {
	var logs []models.CmdLog
	q := r.db.WithContext(ctx).Where("device_id = ?", deviceID).Order("created_at DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if err := q.Find(&logs).Error; err != nil {
		return nil, err
	}
	return logs, nil
}

// EnqueueOutbound 入队下行消息。
func (r *Repository) EnqueueOutbound(ctx context.Context, msg *models.OutboundMessage) (int64, error) {
	if err := r.db.WithContext(ctx).Create(msg).Error; err != nil {
		return 0, err
	}
	return msg.ID, nil
}

// DequeuePendingForDevice 查询设备待发送的消息。
func (r *Repository) DequeuePendingForDevice(ctx context.Context, deviceID int64, limit int) ([]models.OutboundMessage, error) {
	var items []models.OutboundMessage
	now := time.Now()
	q := r.db.WithContext(ctx).
		Where("device_id = ? AND status = 0 AND (not_before IS NULL OR not_before <= ?)", deviceID, now).
		Order("priority ASC, created_at ASC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if err := q.Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

// MarkOutboundSent 将消息标记为已发送。
func (r *Repository) MarkOutboundSent(ctx context.Context, id int64) error {
	return r.updateOutboundStatus(ctx, id, 1, nil)
}

// MarkOutboundDone 将消息标记为完成。
func (r *Repository) MarkOutboundDone(ctx context.Context, id int64) error {
	return r.updateOutboundStatus(ctx, id, 2, nil)
}

// MarkOutboundFailed 将消息标记为失败并记录错误。
func (r *Repository) MarkOutboundFailed(ctx context.Context, id int64, lastError string) error {
	return r.updateOutboundStatus(ctx, id, 3, map[string]interface{}{
		"last_error":  lastError,
		"retry_count": gorm.Expr("retry_count + 1"),
	})
}

func (r *Repository) updateOutboundStatus(ctx context.Context, id int64, status int32, extra map[string]interface{}) error {
	updates := map[string]interface{}{
		"status":     status,
		"updated_at": gorm.Expr("NOW()"),
	}
	for k, v := range extra {
		updates[k] = v
	}

	res := r.db.WithContext(ctx).
		Model(&models.OutboundMessage{}).
		Where("id = ?", id).
		Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
