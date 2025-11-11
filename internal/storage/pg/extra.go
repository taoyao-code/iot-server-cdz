package pg

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// GatewaySocket 组网插座信息（与协议处理层解耦的最小结构）
type GatewaySocket struct {
	GatewayID      string
	SocketNo       int
	SocketMAC      string
	SocketUID      string
	Channel        int
	Status         int
	SignalStrength *int
	LastSeenAt     *time.Time
}

// OTATask OTA 升级任务最小模型
type OTATask struct {
	ID        int64
	DeviceID  int64
	Version   string
	Target    string // 固件目标标识
	Status    int    // 0=pending,1=running,2=completed,3=failed
	Progress  int    // 0-100
	ErrorMsg  *string
	CreatedAt time.Time
	UpdatedAt time.Time
	// API层使用的字段
	TargetType      int
	TargetSocketNo  *int
	FirmwareVersion string
	FTPServer       string
	FTPPort         int
	FileName        string
	FileSize        *int64
}

// ===== 订单相关：按端口的订单状态流转 =====

// GetPendingOrderByPort 返回某设备端口的 pending 订单（若无返回 nil, nil）
func (r *Repository) GetPendingOrderByPort(ctx context.Context, deviceID int64, portNo int) (*Order, error) {
	const q = `SELECT id, device_id, '' as phy_id, port_no, order_no, start_time, end_time, kwh_0p01, amount_cent, status
		FROM orders WHERE device_id=$1 AND port_no=$2 AND status=0 ORDER BY id DESC LIMIT 1`
	var (
		ord Order
		kwh *int64
		amt *int64
	)
	err := r.Pool.QueryRow(ctx, q, deviceID, portNo).Scan(&ord.ID, &ord.DeviceID, &ord.PhyID, &ord.PortNo, &ord.OrderNo, &ord.StartTime, &ord.EndTime, &kwh, &amt, &ord.Status)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	ord.Kwh01 = kwh
	ord.AmountCent = amt
	return &ord, nil
}

// UpdateOrderToCharging 将订单更新为 charging，并设置开始时间（若为空则使用 NOW()）
func (r *Repository) UpdateOrderToCharging(ctx context.Context, orderNo string, startTime time.Time) error {
	const q = `UPDATE orders SET status=1, start_time=COALESCE($2, NOW()), updated_at=NOW() WHERE order_no=$1`
	var st *time.Time
	if !startTime.IsZero() {
		st = &startTime
	}
	_, err := r.Pool.Exec(ctx, q, orderNo, st)
	return err
}

// GetChargingOrderByPort 返回 charging 订单（若无返回 nil,nil）
func (r *Repository) GetChargingOrderByPort(ctx context.Context, deviceID int64, portNo int) (*Order, error) {
	const q = `SELECT id, device_id, '' as phy_id, port_no, order_no, start_time, end_time, kwh_0p01, amount_cent, status
		FROM orders WHERE device_id=$1 AND port_no=$2 AND status=1 ORDER BY id DESC LIMIT 1`
	var (
		ord Order
		kwh *int64
		amt *int64
	)
	err := r.Pool.QueryRow(ctx, q, deviceID, portNo).Scan(&ord.ID, &ord.DeviceID, &ord.PhyID, &ord.PortNo, &ord.OrderNo, &ord.StartTime, &ord.EndTime, &kwh, &amt, &ord.Status)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	ord.Kwh01 = kwh
	ord.AmountCent = amt
	return &ord, nil
}

// CompleteOrderByPort 完成端口上的充电订单（设置 end_time 并置状态为 completed=2）
func (r *Repository) CompleteOrderByPort(ctx context.Context, deviceID int64, portNo int, endTime time.Time, reason int) error {
	const q = `UPDATE orders SET end_time=COALESCE($3, NOW()), status=2, updated_at=NOW()
		WHERE device_id=$1 AND port_no=$2 AND status=1`
	var et *time.Time
	if !endTime.IsZero() {
		et = &endTime
	}
	_, err := r.Pool.Exec(ctx, q, deviceID, portNo, et)
	return err
}

// CancelOrderByPort 取消 pending 订单（置为 cancelled=9）
func (r *Repository) CancelOrderByPort(ctx context.Context, deviceID int64, portNo int) error {
	_, err := r.Pool.Exec(ctx, `UPDATE orders SET status=9, updated_at=NOW() WHERE device_id=$1 AND port_no=$2 AND status=0`, deviceID, portNo)
	return err
}

// MarkChargingOrdersAsInterrupted 将设备上的 charging 订单标记为 interrupted=10
func (r *Repository) MarkChargingOrdersAsInterrupted(ctx context.Context, deviceID int64) (int64, error) {
	cmdTag, err := r.Pool.Exec(ctx, `UPDATE orders SET status=10, updated_at=NOW() WHERE device_id=$1 AND status=1`, deviceID)
	if err != nil {
		return 0, err
	}
	return cmdTag.RowsAffected(), nil
}

// GetInterruptedOrders 查询设备的 interrupted 订单
func (r *Repository) GetInterruptedOrders(ctx context.Context, deviceID int64) ([]Order, error) {
	rows, err := r.Pool.Query(ctx, `SELECT id, device_id, '' as phy_id, port_no, order_no, start_time, end_time, kwh_0p01, amount_cent, status
		FROM orders WHERE device_id=$1 AND status=10 ORDER BY id DESC`, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []Order
	for rows.Next() {
		var (
			ord Order
			kwh *int64
			amt *int64
		)
		if err := rows.Scan(&ord.ID, &ord.DeviceID, &ord.PhyID, &ord.PortNo, &ord.OrderNo, &ord.StartTime, &ord.EndTime, &kwh, &amt, &ord.Status); err != nil {
			return nil, err
		}
		ord.Kwh01 = kwh
		ord.AmountCent = amt
		res = append(res, ord)
	}
	return res, rows.Err()
}

// RecoverOrder 将 interrupted 订单恢复为 charging=1
func (r *Repository) RecoverOrder(ctx context.Context, orderNo string) error {
	_, err := r.Pool.Exec(ctx, `UPDATE orders SET status=1, updated_at=NOW() WHERE order_no=$1 AND status=10`, orderNo)
	return err
}

// FailOrder 将订单标记为 failed=11
func (r *Repository) FailOrder(ctx context.Context, orderNo, reason string) error {
	_, err := r.Pool.Exec(ctx, `UPDATE orders SET status=11, updated_at=NOW() WHERE order_no=$1`, orderNo)
	return err
}

// ===== 参数写入回读：最小空实现（避免引入额外表） =====

func (r *Repository) StoreParamWrite(ctx context.Context, deviceID int64, paramID int, value []byte, msgID int) error {
	// 最小空实现：直接返回成功
	return nil
}

func (r *Repository) GetParamWritePending(ctx context.Context, deviceID int64, paramID int) ([]byte, int, error) {
	// 最小空实现：无待验证值
	return nil, 0, nil
}

func (r *Repository) ConfirmParamWrite(ctx context.Context, deviceID int64, paramID int, msgID int) error {
	return nil
}

func (r *Repository) FailParamWrite(ctx context.Context, deviceID int64, paramID int, msgID int, errMsg string) error {
	return nil
}

// ===== 组网插座：最小空实现 =====

func (r *Repository) UpsertGatewaySocket(ctx context.Context, socket *GatewaySocket) error {
	return nil
}

func (r *Repository) DeleteGatewaySocket(ctx context.Context, gatewayID string, socketNo int) error {
	return nil
}

func (r *Repository) GetGatewaySockets(ctx context.Context, gatewayID string) ([]GatewaySocket, error) {
	return []GatewaySocket{}, nil
}

// ===== OTA 任务：最小空实现 =====

func (r *Repository) CreateOTATask(ctx context.Context, task *OTATask) (int64, error) {
	return 0, nil
}

func (r *Repository) GetOTATask(ctx context.Context, taskID int64) (*OTATask, error) {
	return nil, nil
}

func (r *Repository) UpdateOTATaskStatus(ctx context.Context, taskID int64, status int, errorMsg *string) error {
	return nil
}

func (r *Repository) UpdateOTATaskProgress(ctx context.Context, taskID int64, progress int, status int) error {
	return nil
}

func (r *Repository) GetDeviceOTATasks(ctx context.Context, deviceID int64, limit int) ([]OTATask, error) {
	return []OTATask{}, nil
}

// ===== Card 相关：刷卡充电业务 =====

// Card 卡片信息
type Card struct {
	CardNo    string
	Balance   float64
	Status    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// CardTransaction 刷卡交易记录
type CardTransaction struct {
	ID              int64
	CardNo          string
	DeviceID        string
	PhyID           string
	OrderNo         string
	ChargeMode      int
	Amount          *float64
	DurationMinutes *int
	Status          string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (r *Repository) GetCard(ctx context.Context, cardNo string) (*Card, error) {
	return nil, nil
}

func (r *Repository) CreateCard(ctx context.Context, cardNo string, balance float64, status string) (*Card, error) {
	return nil, nil
}

func (r *Repository) GetTransaction(ctx context.Context, orderNo string) (*CardTransaction, error) {
	return nil, nil
}

func (r *Repository) CreateTransaction(ctx context.Context, tx *CardTransaction) (*CardTransaction, error) {
	return nil, nil
}

func (r *Repository) UpdateTransactionChargingWithEvent(ctx context.Context, orderNo string, eventData []byte) error {
	return nil
}

func (r *Repository) FailTransactionWithEvent(ctx context.Context, orderNo, reason string, eventData []byte) error {
	return nil
}

func (r *Repository) UpdateCardBalance(ctx context.Context, cardNo string, amount float64, changeType, description string) error {
	return nil
}

func (r *Repository) CompleteTransaction(ctx context.Context, orderNo string, energyKwh, totalAmount float64) error {
	return nil
}

func (r *Repository) GetCardTransactions(ctx context.Context, cardNo string, limit int) ([]CardTransaction, error) {
	return []CardTransaction{}, nil
}

func (r *Repository) GetNextSequenceNo(ctx context.Context, orderNo string) (int, error) {
	return 1, nil
}

func (r *Repository) InsertEvent(ctx context.Context, orderNo, eventType string, eventData []byte, sequenceNo int) error {
	return nil
}

// ===== 参数管理 =====

type DeviceParam struct {
	ID          int64
	DeviceID    int64
	ParamID     int
	ParamValue  []byte
	MsgID       int
	Status      int
	CreatedAt   time.Time
	ConfirmedAt *time.Time
	ErrorMsg    *string
}

func (r *Repository) ListDeviceParams(ctx context.Context, deviceID int64) ([]DeviceParam, error) {
	return []DeviceParam{}, nil
}

// ===== 事件推送 =====

type Event struct {
	ID           int64
	OrderNo      string
	EventType    string
	EventData    []byte
	RetryCount   int
	Status       int
	CreatedAt    time.Time
	PushedAt     *time.Time
	SequenceNo   int
	ErrorMessage *string
}

func (r *Repository) GetPendingEvents(ctx context.Context, limit int) ([]Event, error) {
	return []Event{}, nil
}

func (r *Repository) MarkEventPushed(ctx context.Context, eventID int64) error {
	return nil
}

func (r *Repository) MarkEventFailed(ctx context.Context, eventID int64, errorMsg string) error {
	return nil
}

func (r *Repository) GetOrderEvents(ctx context.Context, orderNo string) ([]Event, error) {
	return []Event{}, nil
}
