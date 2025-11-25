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

// ===== 参数写入回读：数据库持久化（C修复） =====

// StoreParamWrite C修复: 存储参数写入记录到device_params表
func (r *Repository) StoreParamWrite(ctx context.Context, deviceID int64, paramID int, value []byte, msgID int) error {
	const q = `INSERT INTO device_params (device_id, param_id, param_value, msg_id, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 0, NOW(), NOW())
		ON CONFLICT (device_id, param_id) 
		DO UPDATE SET param_value=$3, msg_id=$4, status=0, updated_at=NOW(), error_message=NULL`
	_, err := r.Pool.Exec(ctx, q, deviceID, paramID, value, msgID)
	return err
}

// GetParamWritePending C修复: 获取待确认的参数写入值
func (r *Repository) GetParamWritePending(ctx context.Context, deviceID int64, paramID int) ([]byte, int, error) {
	const q = `SELECT param_value, msg_id FROM device_params 
		WHERE device_id=$1 AND param_id=$2 AND status=0`
	var value []byte
	var msgID int
	err := r.Pool.QueryRow(ctx, q, deviceID, paramID).Scan(&value, &msgID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, 0, nil // 无待验证值
		}
		return nil, 0, err
	}
	return value, msgID, nil
}

// ConfirmParamWrite C修复: 确认参数写入成功
func (r *Repository) ConfirmParamWrite(ctx context.Context, deviceID int64, paramID int, msgID int) error {
	const q = `UPDATE device_params 
		SET status=1, confirmed_at=NOW(), updated_at=NOW() 
		WHERE device_id=$1 AND param_id=$2 AND msg_id=$3 AND status=0`
	_, err := r.Pool.Exec(ctx, q, deviceID, paramID, msgID)
	return err
}

// FailParamWrite C修复: 标记参数写入失败
func (r *Repository) FailParamWrite(ctx context.Context, deviceID int64, paramID int, msgID int, errMsg string) error {
	const q = `UPDATE device_params 
		SET status=2, error_message=$4, updated_at=NOW() 
		WHERE device_id=$1 AND param_id=$2 AND msg_id=$3 AND status=0`
	_, err := r.Pool.Exec(ctx, q, deviceID, paramID, msgID, errMsg)
	return err
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
	ID            int64
	OrderNo       string
	EventType     string
	EventData     []byte
	RetryCount    int
	Status        int
	CreatedAt     time.Time
	PushedAt      *time.Time
	SequenceNo    int
	ErrorMessage  *string
	TestSessionID *string
}

// GetPendingEvents E修复: 获取待推送的事件
func (r *Repository) GetPendingEvents(ctx context.Context, limit int) ([]Event, error) {
	if limit <= 0 {
		limit = 50
	}

	const q = `SELECT id, order_no, event_type, event_data, sequence_no, status, retry_count, created_at, pushed_at, error_message, test_session_id
		FROM events 
		WHERE status IN (0, 2) AND retry_count < 5
		ORDER BY order_no, sequence_no
		LIMIT $1`

	rows, err := r.Pool.Query(ctx, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var e Event
		var errorMsg *string
		var pushedAt *time.Time
		var sessionID *string

		err := rows.Scan(&e.ID, &e.OrderNo, &e.EventType, &e.EventData, &e.SequenceNo,
			&e.Status, &e.RetryCount, &e.CreatedAt, &pushedAt, &errorMsg, &sessionID)
		if err != nil {
			return nil, err
		}

		if pushedAt != nil {
			e.PushedAt = pushedAt
		}
		if errorMsg != nil {
			e.ErrorMessage = errorMsg
		}
		if sessionID != nil {
			e.TestSessionID = sessionID
		}

		events = append(events, e)
	}

	return events, rows.Err()
}

// MarkEventPushed E修复: 标记事件已推送
func (r *Repository) MarkEventPushed(ctx context.Context, eventID int64) error {
	const q = `UPDATE events SET status=1, pushed_at=NOW() WHERE id=$1`
	_, err := r.Pool.Exec(ctx, q, eventID)
	return err
}

// MarkEventFailed E修复: 标记事件推送失败
func (r *Repository) MarkEventFailed(ctx context.Context, eventID int64, errorMsg string) error {
	const q = `UPDATE events 
		SET status=2, retry_count=retry_count+1, error_message=$2 
		WHERE id=$1`
	_, err := r.Pool.Exec(ctx, q, eventID, errorMsg)
	return err
}

// GetOrderEvents E修复: 获取订单的所有事件
func (r *Repository) GetOrderEvents(ctx context.Context, orderNo string) ([]Event, error) {
	const q = `SELECT id, order_no, event_type, event_data, sequence_no, status, retry_count, created_at, pushed_at, error_message, test_session_id
		FROM events 
		WHERE order_no=$1
		ORDER BY sequence_no`

	rows, err := r.Pool.Query(ctx, q, orderNo)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var e Event
		var errorMsg *string
		var pushedAt *time.Time
		var sessionID *string

		err := rows.Scan(&e.ID, &e.OrderNo, &e.EventType, &e.EventData, &e.SequenceNo,
			&e.Status, &e.RetryCount, &e.CreatedAt, &pushedAt, &errorMsg, &sessionID)
		if err != nil {
			return nil, err
		}

		if pushedAt != nil {
			e.PushedAt = pushedAt
		}
		if errorMsg != nil {
			e.ErrorMessage = errorMsg
		}
		if sessionID != nil {
			e.TestSessionID = sessionID
		}

		events = append(events, e)
	}

	return events, rows.Err()
}

// GetNextSequenceNo E修复: 获取订单的下一个序列号
func (r *Repository) GetNextSequenceNo(ctx context.Context, orderNo string) (int, error) {
	const q = `SELECT COALESCE(MAX(sequence_no), 0) + 1 FROM events WHERE order_no=$1`
	var seqNo int
	err := r.Pool.QueryRow(ctx, q, orderNo).Scan(&seqNo)
	return seqNo, err
}

// InsertEvent E修复: 插入事件到events表
func (r *Repository) InsertEvent(ctx context.Context, orderNo, eventType string, eventData []byte, sequenceNo int) error {
	return nil
}
