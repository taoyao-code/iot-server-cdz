package pg

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository 提供最小持久化能力
type Repository struct {
	Pool *pgxpool.Pool
}

// EnsureDevice 返回设备ID，若不存在则插入并更新最近时间
func (r *Repository) EnsureDevice(ctx context.Context, phyID string) (int64, error) {
	const q = `INSERT INTO devices (phy_id, last_seen_at)
               VALUES ($1, NOW())
               ON CONFLICT (phy_id) DO UPDATE SET updated_at = NOW(), last_seen_at = NOW()
               RETURNING id`
	var id int64
	err := r.Pool.QueryRow(ctx, q, phyID).Scan(&id)
	return id, err
}

// InsertCmdLog 插入指令日志（最小字段）
func (r *Repository) InsertCmdLog(ctx context.Context, deviceID int64, msgID int, cmd int, direction int16, payload []byte, success bool) error {
	const q = `INSERT INTO cmd_log (device_id, msg_id, cmd, direction, payload, success, created_at)
               VALUES ($1,$2,$3,$4,$5,$6,NOW())`
	_, err := r.Pool.Exec(ctx, q, deviceID, msgID, cmd, direction, payload, success)
	return err
}

// UpsertPortState 更新端口快照（最小字段：status/power）
func (r *Repository) UpsertPortState(ctx context.Context, deviceID int64, portNo int, status int, powerW *int) error {
	const q = `INSERT INTO ports (device_id, port_no, status, power_w, updated_at)
               VALUES ($1,$2,$3,$4,NOW())
               ON CONFLICT (device_id, port_no)
               DO UPDATE SET status=EXCLUDED.status, power_w=EXCLUDED.power_w, updated_at=NOW()`
	var pw interface{}
	if powerW == nil {
		pw = nil
	} else {
		pw = *powerW
	}
	_, err := r.Pool.Exec(ctx, q, deviceID, portNo, status, pw)
	return err
}

// UpsertOrderProgress 插入或更新进行中的订单进度（根据 order_no 唯一键或冲突更新）
func (r *Repository) UpsertOrderProgress(ctx context.Context, deviceID int64, portNo int, orderHex string, durationSec int, kwh01 int, status int, powerW01 *int) error {
	const q = `INSERT INTO orders (device_id, port_no, order_no, start_time, status, kwh_0p01)
               VALUES ($1,$2,$3,NOW(),$4,$5)
               ON CONFLICT (order_no)
               DO UPDATE SET status=EXCLUDED.status, kwh_0p01=EXCLUDED.kwh_0p01, updated_at=NOW()`
	_, err := r.Pool.Exec(ctx, q, deviceID, portNo, orderHex, 1, kwh01)
	return err
}

// SettleOrder 结算订单（结束时间、耗电、金额占位、结束原因）
func (r *Repository) SettleOrder(ctx context.Context, deviceID int64, portNo int, orderHex string, durationSec int, kwh01 int, reason int) error {
	const q = `INSERT INTO orders (device_id, port_no, order_no, start_time, end_time, kwh_0p01, status)
               VALUES ($1,$2,$3,NOW()-($4||' seconds')::interval, NOW(), $5, 2)
               ON CONFLICT (order_no)
               DO UPDATE SET end_time=NOW(), kwh_0p01=$5, status=2, updated_at=NOW()`
	_, err := r.Pool.Exec(ctx, q, deviceID, portNo, orderHex, durationSec, kwh01)
	return err
}

// AckOutboundByMsgID 根据 device_id+msg_id 标记下行队列完成或失败
func (r *Repository) AckOutboundByMsgID(ctx context.Context, deviceID int64, msgID int, ok bool, errCode *int) error {
	if ok {
		_, err := r.Pool.Exec(ctx, `UPDATE outbound_queue SET status=2, updated_at=NOW() WHERE device_id=$1 AND msg_id=$2`, deviceID, msgID)
		return err
	}
	var code interface{}
	if errCode != nil {
		code = *errCode
	} else {
		code = nil
	}
	_, err := r.Pool.Exec(ctx, `UPDATE outbound_queue SET status=3, last_error=COALESCE(last_error,'')||' ack_err='||COALESCE($3::text,'unknown'), updated_at=NOW() WHERE device_id=$1 AND msg_id=$2`, deviceID, msgID, code)
	return err
}

// Device 设备基本信息（用于查询）
type Device struct {
	ID         int64
	PhyID      string
	LastSeenAt *time.Time
}

// Order 订单（用于查询）
type Order struct {
	ID         int64
	DeviceID   int64
	PhyID      string
	PortNo     int
	OrderNo    string
	StartTime  *time.Time
	EndTime    *time.Time
	Kwh01      *int64
	AmountCent *int64
	Status     int
}

// ListDevices 简单分页列出设备
func (r *Repository) ListDevices(ctx context.Context, limit, offset int) ([]Device, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := r.Pool.Query(ctx, `SELECT id, phy_id, last_seen_at FROM devices ORDER BY id DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []Device
	for rows.Next() {
		var d Device
		if err := rows.Scan(&d.ID, &d.PhyID, &d.LastSeenAt); err != nil {
			return nil, err
		}
		res = append(res, d)
	}
	return res, rows.Err()
}

// GetDeviceByPhyID 通过物理ID获取设备
func (r *Repository) GetDeviceByPhyID(ctx context.Context, phyID string) (*Device, error) {
	const q = `SELECT id, phy_id, last_seen_at FROM devices WHERE phy_id=$1`
	var d Device
	if err := r.Pool.QueryRow(ctx, q, phyID).Scan(&d.ID, &d.PhyID, &d.LastSeenAt); err != nil {
		return nil, err
	}
	return &d, nil
}

// Port 端口快照（用于查询）
type Port struct {
	PortNo int
	Status int
	PowerW *int
}

// ListPortsByPhyID 按 phyID 查询其端口快照
func (r *Repository) ListPortsByPhyID(ctx context.Context, phyID string) ([]Port, error) {
	rows, err := r.Pool.Query(ctx, `SELECT p.port_no, p.status, p.power_w
        FROM ports p JOIN devices d ON p.device_id = d.id WHERE d.phy_id = $1 ORDER BY p.port_no`, phyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []Port
	for rows.Next() {
		var p Port
		if err := rows.Scan(&p.PortNo, &p.Status, &p.PowerW); err != nil {
			return nil, err
		}
		res = append(res, p)
	}
	return res, rows.Err()
}

// GetOrderByID 根据ID查询订单，包含设备phy_id
func (r *Repository) GetOrderByID(ctx context.Context, id int64) (*Order, error) {
	const q = `SELECT o.id, o.device_id, d.phy_id, o.port_no, o.order_no, o.start_time, o.end_time, o.kwh_0p01, o.amount_cent, o.status
		FROM orders o JOIN devices d ON o.device_id = d.id WHERE o.id=$1`
	var (
		ord Order
		kwh *int64
		amt *int64
	)
	if err := r.Pool.QueryRow(ctx, q, id).Scan(&ord.ID, &ord.DeviceID, &ord.PhyID, &ord.PortNo, &ord.OrderNo, &ord.StartTime, &ord.EndTime, &kwh, &amt, &ord.Status); err != nil {
		return nil, err
	}
	ord.Kwh01 = kwh
	ord.AmountCent = amt
	return &ord, nil
}

// ListOrdersByPhyID 按设备物理ID分页查询订单
func (r *Repository) ListOrdersByPhyID(ctx context.Context, phyID string, limit, offset int) ([]Order, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	const q = `SELECT o.id, o.device_id, d.phy_id, o.port_no, o.order_no, o.start_time, o.end_time, o.kwh_0p01, o.amount_cent, o.status
		FROM orders o JOIN devices d ON o.device_id = d.id
		WHERE d.phy_id=$1 ORDER BY o.id DESC LIMIT $2 OFFSET $3`
	rows, err := r.Pool.Query(ctx, q, phyID, limit, offset)
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

// EnqueueOutbox 插入下行队列记录，返回ID
func (r *Repository) EnqueueOutbox(
	ctx context.Context,
	deviceID int64,
	phyID *string,
	portNo *int,
	cmd int,
	payload []byte,
	priority int,
	correlationID *string,
	notBefore *time.Time,
	timeoutSec int,
) (int64, error) {
	const q = `INSERT INTO outbound_queue (device_id, phy_id, port_no, cmd, payload, priority, status, retry_count, not_before, timeout_sec, correlation_id)
               VALUES ($1,$2,$3,$4,$5,COALESCE($6,100),0,0,$7,COALESCE($8,15),$9)
               RETURNING id`
	var id int64
	var pn interface{}
	if portNo != nil {
		pn = *portNo
	} else {
		pn = nil
	}
	var phy interface{}
	if phyID != nil {
		phy = *phyID
	} else {
		phy = nil
	}
	var corr interface{}
	if correlationID != nil {
		corr = *correlationID
	} else {
		corr = nil
	}
	var nb interface{}
	if notBefore != nil {
		nb = *notBefore
	} else {
		nb = nil
	}
	var pr interface{}
	if priority > 0 {
		pr = priority
	} else {
		pr = nil
	}
	var to interface{}
	if timeoutSec > 0 {
		to = timeoutSec
	} else {
		to = nil
	}
	if err := r.Pool.QueryRow(ctx, q, deviceID, phy, pn, cmd, payload, pr, nb, to, corr).Scan(&id); err != nil {
		return 0, err
	}
	return id, nil
}

// EnqueueOutboxIdempotent 基于 correlation_id 的幂等入队：存在则返回已存在ID，并标记 created=false
func (r *Repository) EnqueueOutboxIdempotent(
	ctx context.Context,
	deviceID int64,
	phyID *string,
	portNo *int,
	cmd int,
	payload []byte,
	priority int,
	correlationID string,
	notBefore *time.Time,
	timeoutSec int,
) (id int64, created bool, err error) {
	// 优先尝试插入
	id, err = r.EnqueueOutbox(ctx, deviceID, phyID, portNo, cmd, payload, priority, &correlationID, notBefore, timeoutSec)
	if err == nil {
		return id, true, nil
	}
	// 冲突时查询已存在记录（依赖唯一索引）
	err = r.Pool.QueryRow(ctx, `SELECT id FROM outbound_queue WHERE correlation_id=$1`, correlationID).Scan(&id)
	if err != nil {
		return 0, false, err
	}
	return id, false, nil
}

// ============================================================================
// P0修复: 设备参数持久化存储方法
// ============================================================================

// StoreParamWrite 存储参数写入请求
func (r *Repository) StoreParamWrite(ctx context.Context, deviceID int64, paramID int, value []byte, msgID int) error {
	const q = `INSERT INTO device_params (device_id, param_id, param_value, msg_id, status, created_at, updated_at)
               VALUES ($1, $2, $3, $4, 0, NOW(), NOW())
               ON CONFLICT (device_id, param_id) 
               DO UPDATE SET 
                   param_value = EXCLUDED.param_value,
                   msg_id = EXCLUDED.msg_id,
                   status = 0,
                   created_at = NOW(),
                   updated_at = NOW(),
                   confirmed_at = NULL,
                   error_message = NULL`
	_, err := r.Pool.Exec(ctx, q, deviceID, paramID, value, msgID)
	return err
}

// GetParamWritePending 获取待确认的参数写入
func (r *Repository) GetParamWritePending(ctx context.Context, deviceID int64, paramID int) ([]byte, int, error) {
	const q = `SELECT param_value, msg_id 
               FROM device_params 
               WHERE device_id = $1 AND param_id = $2 AND status = 0
               ORDER BY created_at DESC
               LIMIT 1`

	var value []byte
	var msgID int
	err := r.Pool.QueryRow(ctx, q, deviceID, paramID).Scan(&value, &msgID)
	if err != nil {
		return nil, 0, err
	}
	return value, msgID, nil
}

// ConfirmParamWrite 确认参数写入成功
func (r *Repository) ConfirmParamWrite(ctx context.Context, deviceID int64, paramID int, msgID int) error {
	const q = `UPDATE device_params 
               SET status = 1, confirmed_at = NOW(), updated_at = NOW()
               WHERE device_id = $1 AND param_id = $2 AND msg_id = $3 AND status = 0`
	result, err := r.Pool.Exec(ctx, q, deviceID, paramID, msgID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("no pending param write found for device=%d param=%d msg=%d", deviceID, paramID, msgID)
	}
	return nil
}

// FailParamWrite 标记参数写入失败
func (r *Repository) FailParamWrite(ctx context.Context, deviceID int64, paramID int, msgID int, errMsg string) error {
	const q = `UPDATE device_params 
               SET status = 2, error_message = $4, updated_at = NOW()
               WHERE device_id = $1 AND param_id = $2 AND msg_id = $3 AND status = 0`
	result, err := r.Pool.Exec(ctx, q, deviceID, paramID, msgID, errMsg)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("no pending param write found for device=%d param=%d msg=%d", deviceID, paramID, msgID)
	}
	return nil
}

// DeviceParam 设备参数结构
type DeviceParam struct {
	ID          int64
	DeviceID    int64
	ParamID     int
	ParamValue  []byte
	MsgID       int
	Status      int // 0=待确认, 1=已确认, 2=失败
	CreatedAt   time.Time
	ConfirmedAt *time.Time
	UpdatedAt   time.Time
	ErrorMsg    *string
}

// ListDeviceParams 查询设备所有参数
func (r *Repository) ListDeviceParams(ctx context.Context, deviceID int64) ([]DeviceParam, error) {
	const q = `SELECT id, device_id, param_id, param_value, msg_id, status, created_at, confirmed_at, updated_at, error_message
               FROM device_params
               WHERE device_id = $1
               ORDER BY param_id, created_at DESC`

	rows, err := r.Pool.Query(ctx, q, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var params []DeviceParam
	for rows.Next() {
		var p DeviceParam
		if err := rows.Scan(&p.ID, &p.DeviceID, &p.ParamID, &p.ParamValue, &p.MsgID,
			&p.Status, &p.CreatedAt, &p.ConfirmedAt, &p.UpdatedAt, &p.ErrorMsg); err != nil {
			return nil, err
		}
		params = append(params, p)
	}
	return params, rows.Err()
}

// GetDeviceParam 获取设备指定参数的最新记录
func (r *Repository) GetDeviceParam(ctx context.Context, deviceID int64, paramID int) (*DeviceParam, error) {
	const q = `SELECT id, device_id, param_id, param_value, msg_id, status, created_at, confirmed_at, updated_at, error_message
               FROM device_params
               WHERE device_id = $1 AND param_id = $2
               ORDER BY created_at DESC
               LIMIT 1`

	var p DeviceParam
	err := r.Pool.QueryRow(ctx, q, deviceID, paramID).Scan(
		&p.ID, &p.DeviceID, &p.ParamID, &p.ParamValue, &p.MsgID,
		&p.Status, &p.CreatedAt, &p.ConfirmedAt, &p.UpdatedAt, &p.ErrorMsg,
	)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// ============ Week4: 刷卡充电系统数据库操作 ============

// Card 卡片信息
type Card struct {
	ID          int64     `json:"id"`
	CardNo      string    `json:"card_no"`
	Balance     float64   `json:"balance"`
	Status      string    `json:"status"`
	UserID      *int64    `json:"user_id,omitempty"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// CardTransaction 刷卡交易记录
type CardTransaction struct {
	ID              int64      `json:"id"`
	CardNo          string     `json:"card_no"`
	DeviceID        string     `json:"device_id"`
	PhyID           string     `json:"phy_id"`
	OrderNo         string     `json:"order_no"`
	ChargeMode      int        `json:"charge_mode"`
	Amount          *float64   `json:"amount,omitempty"`
	DurationMinutes *int       `json:"duration_minutes,omitempty"`
	PowerWatts      *int       `json:"power_watts,omitempty"`
	EnergyKwh       *float64   `json:"energy_kwh,omitempty"`
	Status          string     `json:"status"`
	StartTime       *time.Time `json:"start_time,omitempty"`
	EndTime         *time.Time `json:"end_time,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	FailureReason   *string    `json:"failure_reason,omitempty"`
	PricePerKwh     *float64   `json:"price_per_kwh,omitempty"`
	ServiceFeeRate  *float64   `json:"service_fee_rate,omitempty"`
	TotalAmount     *float64   `json:"total_amount,omitempty"`
}

// CardBalanceLog 余额变更记录
type CardBalanceLog struct {
	ID            int64     `json:"id"`
	CardNo        string    `json:"card_no"`
	TransactionID *int64    `json:"transaction_id,omitempty"`
	ChangeType    string    `json:"change_type"`
	Amount        float64   `json:"amount"`
	BalanceBefore float64   `json:"balance_before"`
	BalanceAfter  float64   `json:"balance_after"`
	Description   string    `json:"description,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// GetCard 根据卡号获取卡片信息
func (r *Repository) GetCard(ctx context.Context, cardNo string) (*Card, error) {
	const q = `SELECT id, card_no, balance, status, user_id, description, created_at, updated_at
               FROM cards WHERE card_no = $1`

	var card Card
	err := r.Pool.QueryRow(ctx, q, cardNo).Scan(
		&card.ID, &card.CardNo, &card.Balance, &card.Status,
		&card.UserID, &card.Description, &card.CreatedAt, &card.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &card, nil
}

// CreateCard 创建新卡片
func (r *Repository) CreateCard(ctx context.Context, cardNo string, balance float64, status string) (*Card, error) {
	const q = `INSERT INTO cards (card_no, balance, status, created_at, updated_at)
               VALUES ($1, $2, $3, NOW(), NOW())
               RETURNING id, card_no, balance, status, user_id, description, created_at, updated_at`

	var card Card
	err := r.Pool.QueryRow(ctx, q, cardNo, balance, status).Scan(
		&card.ID, &card.CardNo, &card.Balance, &card.Status,
		&card.UserID, &card.Description, &card.CreatedAt, &card.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &card, nil
}

// UpdateCardBalance 更新卡片余额（原子操作）
func (r *Repository) UpdateCardBalance(ctx context.Context, cardNo string, amount float64, changeType, description string) error {
	// 开启事务
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// 1. 获取当前余额并加锁
	var balanceBefore float64
	err = tx.QueryRow(ctx, `SELECT balance FROM cards WHERE card_no = $1 FOR UPDATE`, cardNo).Scan(&balanceBefore)
	if err != nil {
		return fmt.Errorf("card not found or locked: %w", err)
	}

	// 2. 计算新余额
	balanceAfter := balanceBefore + amount
	if balanceAfter < 0 {
		return fmt.Errorf("insufficient balance: current=%.2f, required=%.2f", balanceBefore, -amount)
	}

	// 3. 更新余额
	_, err = tx.Exec(ctx, `UPDATE cards SET balance = $1, updated_at = NOW() WHERE card_no = $2`,
		balanceAfter, cardNo)
	if err != nil {
		return err
	}

	// 4. 记录余额变更日志
	_, err = tx.Exec(ctx, `INSERT INTO card_balance_logs 
		(card_no, change_type, amount, balance_before, balance_after, description, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())`,
		cardNo, changeType, amount, balanceBefore, balanceAfter, description)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// CreateTransaction 创建交易记录
func (r *Repository) CreateTransaction(ctx context.Context, tx *CardTransaction) (*CardTransaction, error) {
	const q = `INSERT INTO card_transactions 
		(card_no, device_id, phy_id, order_no, charge_mode, amount, duration_minutes, 
		 power_watts, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW(), NOW())
		RETURNING id, created_at, updated_at`

	err := r.Pool.QueryRow(ctx, q,
		tx.CardNo, tx.DeviceID, tx.PhyID, tx.OrderNo, tx.ChargeMode,
		tx.Amount, tx.DurationMinutes, tx.PowerWatts, tx.Status,
	).Scan(&tx.ID, &tx.CreatedAt, &tx.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return tx, nil
}

// GetTransaction 根据订单号获取交易
func (r *Repository) GetTransaction(ctx context.Context, orderNo string) (*CardTransaction, error) {
	const q = `SELECT id, card_no, device_id, phy_id, order_no, charge_mode, 
		amount, duration_minutes, power_watts, energy_kwh, status,
		start_time, end_time, created_at, updated_at, failure_reason,
		price_per_kwh, service_fee_rate, total_amount
		FROM card_transactions WHERE order_no = $1`

	var tx CardTransaction
	err := r.Pool.QueryRow(ctx, q, orderNo).Scan(
		&tx.ID, &tx.CardNo, &tx.DeviceID, &tx.PhyID, &tx.OrderNo, &tx.ChargeMode,
		&tx.Amount, &tx.DurationMinutes, &tx.PowerWatts, &tx.EnergyKwh, &tx.Status,
		&tx.StartTime, &tx.EndTime, &tx.CreatedAt, &tx.UpdatedAt, &tx.FailureReason,
		&tx.PricePerKwh, &tx.ServiceFeeRate, &tx.TotalAmount,
	)
	if err != nil {
		return nil, err
	}
	return &tx, nil
}

// UpdateTransactionStatus 更新交易状态
func (r *Repository) UpdateTransactionStatus(ctx context.Context, orderNo, status string) error {
	const q = `UPDATE card_transactions 
		SET status = $1, updated_at = NOW() 
		WHERE order_no = $2`
	_, err := r.Pool.Exec(ctx, q, status, orderNo)
	return err
}

// UpdateTransactionCharging 更新为充电中状态（设置开始时间）
func (r *Repository) UpdateTransactionCharging(ctx context.Context, orderNo string) error {
	const q = `UPDATE card_transactions 
		SET status = 'charging', start_time = NOW(), updated_at = NOW()
		WHERE order_no = $1`
	_, err := r.Pool.Exec(ctx, q, orderNo)
	return err
}

// CompleteTransaction 完成交易（更新结束信息）
func (r *Repository) CompleteTransaction(ctx context.Context, orderNo string, energyKwh, totalAmount float64) error {
	const q = `UPDATE card_transactions 
		SET status = 'completed', 
		    end_time = NOW(),
		    energy_kwh = $2,
		    total_amount = $3,
		    updated_at = NOW()
		WHERE order_no = $1`
	_, err := r.Pool.Exec(ctx, q, orderNo, energyKwh, totalAmount)
	return err
}

// FailTransaction 标记交易失败
func (r *Repository) FailTransaction(ctx context.Context, orderNo, reason string) error {
	const q = `UPDATE card_transactions 
		SET status = 'failed', failure_reason = $2, updated_at = NOW()
		WHERE order_no = $1`
	_, err := r.Pool.Exec(ctx, q, orderNo, reason)
	return err
}

// GetCardTransactions 获取卡片的交易记录
func (r *Repository) GetCardTransactions(ctx context.Context, cardNo string, limit int) ([]CardTransaction, error) {
	const q = `SELECT id, card_no, device_id, phy_id, order_no, charge_mode,
		amount, duration_minutes, power_watts, energy_kwh, status,
		start_time, end_time, created_at, updated_at, failure_reason,
		price_per_kwh, service_fee_rate, total_amount
		FROM card_transactions 
		WHERE card_no = $1
		ORDER BY created_at DESC
		LIMIT $2`

	rows, err := r.Pool.Query(ctx, q, cardNo, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var transactions []CardTransaction
	for rows.Next() {
		var tx CardTransaction
		if err := rows.Scan(
			&tx.ID, &tx.CardNo, &tx.DeviceID, &tx.PhyID, &tx.OrderNo, &tx.ChargeMode,
			&tx.Amount, &tx.DurationMinutes, &tx.PowerWatts, &tx.EnergyKwh, &tx.Status,
			&tx.StartTime, &tx.EndTime, &tx.CreatedAt, &tx.UpdatedAt, &tx.FailureReason,
			&tx.PricePerKwh, &tx.ServiceFeeRate, &tx.TotalAmount,
		); err != nil {
			return nil, err
		}
		transactions = append(transactions, tx)
	}
	return transactions, rows.Err()
}

// ===== Week 6: 组网管理 Repository 方法 =====

// GatewaySocket 网关插座
type GatewaySocket struct {
	ID             int64
	GatewayID      string
	SocketNo       int
	SocketMAC      string
	SocketUID      string
	Channel        int
	Status         int
	SignalStrength *int
	LastSeenAt     *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// UpsertGatewaySocket 插入或更新网关插座
func (r *Repository) UpsertGatewaySocket(ctx context.Context, socket *GatewaySocket) error {
	const q = `INSERT INTO gateway_sockets 
	           (gateway_id, socket_no, socket_mac, socket_uid, channel, status, signal_strength, last_seen_at, updated_at)
	           VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
	           ON CONFLICT (gateway_id, socket_no) 
	           DO UPDATE SET 
	               socket_mac = EXCLUDED.socket_mac,
	               socket_uid = EXCLUDED.socket_uid,
	               channel = EXCLUDED.channel,
	               status = EXCLUDED.status,
	               signal_strength = EXCLUDED.signal_strength,
	               last_seen_at = EXCLUDED.last_seen_at,
	               updated_at = NOW()`

	_, err := r.Pool.Exec(ctx, q,
		socket.GatewayID,
		socket.SocketNo,
		socket.SocketMAC,
		socket.SocketUID,
		socket.Channel,
		socket.Status,
		socket.SignalStrength,
		socket.LastSeenAt)

	return err
}

// GetGatewaySockets 查询网关下所有插座
func (r *Repository) GetGatewaySockets(ctx context.Context, gatewayID string) ([]GatewaySocket, error) {
	const q = `SELECT id, gateway_id, socket_no, socket_mac, socket_uid, channel, status, signal_strength, last_seen_at, created_at, updated_at
	           FROM gateway_sockets
	           WHERE gateway_id = $1
	           ORDER BY socket_no`

	rows, err := r.Pool.Query(ctx, q, gatewayID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sockets []GatewaySocket
	for rows.Next() {
		var s GatewaySocket
		err := rows.Scan(&s.ID, &s.GatewayID, &s.SocketNo, &s.SocketMAC, &s.SocketUID,
			&s.Channel, &s.Status, &s.SignalStrength, &s.LastSeenAt, &s.CreatedAt, &s.UpdatedAt)
		if err != nil {
			return nil, err
		}
		sockets = append(sockets, s)
	}

	return sockets, rows.Err()
}

// GetGatewaySocket 查询指定插座
func (r *Repository) GetGatewaySocket(ctx context.Context, gatewayID string, socketNo int) (*GatewaySocket, error) {
	const q = `SELECT id, gateway_id, socket_no, socket_mac, socket_uid, channel, status, signal_strength, last_seen_at, created_at, updated_at
	           FROM gateway_sockets
	           WHERE gateway_id = $1 AND socket_no = $2`

	var s GatewaySocket
	err := r.Pool.QueryRow(ctx, q, gatewayID, socketNo).Scan(
		&s.ID, &s.GatewayID, &s.SocketNo, &s.SocketMAC, &s.SocketUID,
		&s.Channel, &s.Status, &s.SignalStrength, &s.LastSeenAt, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return nil, err
	}

	return &s, nil
}

// DeleteGatewaySocket 删除网关插座
func (r *Repository) DeleteGatewaySocket(ctx context.Context, gatewayID string, socketNo int) error {
	const q = `DELETE FROM gateway_sockets WHERE gateway_id = $1 AND socket_no = $2`
	_, err := r.Pool.Exec(ctx, q, gatewayID, socketNo)
	return err
}

// UpdateSocketStatus 更新插座状态
func (r *Repository) UpdateSocketStatus(ctx context.Context, gatewayID string, socketNo int, status int) error {
	const q = `UPDATE gateway_sockets 
	           SET status = $3, updated_at = NOW()
	           WHERE gateway_id = $1 AND socket_no = $2`

	_, err := r.Pool.Exec(ctx, q, gatewayID, socketNo, status)
	return err
}

// ===== Week 7: OTA升级 Repository 方法 =====

// OTATask OTA升级任务
type OTATask struct {
	ID              int64
	DeviceID        int64
	TargetType      int
	TargetSocketNo  *int
	FirmwareVersion string
	FTPServer       string
	FTPPort         int
	FileName        string
	FileSize        *int64
	Status          int
	Progress        int
	ErrorMsg        *string
	MsgID           *int
	StartedAt       *time.Time
	CompletedAt     *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// CreateOTATask 创建OTA升级任务
func (r *Repository) CreateOTATask(ctx context.Context, task *OTATask) (int64, error) {
	const q = `INSERT INTO ota_tasks 
	           (device_id, target_type, target_socket_no, firmware_version, ftp_server, ftp_port, file_name, file_size, status, created_at)
	           VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
	           RETURNING id`

	var id int64
	err := r.Pool.QueryRow(ctx, q,
		task.DeviceID,
		task.TargetType,
		task.TargetSocketNo,
		task.FirmwareVersion,
		task.FTPServer,
		task.FTPPort,
		task.FileName,
		task.FileSize,
		task.Status).Scan(&id)

	return id, err
}

// GetOTATask 查询OTA任务
func (r *Repository) GetOTATask(ctx context.Context, taskID int64) (*OTATask, error) {
	const q = `SELECT id, device_id, target_type, target_socket_no, firmware_version, ftp_server, ftp_port, 
	                  file_name, file_size, status, progress, error_msg, msg_id, started_at, completed_at, created_at, updated_at
	           FROM ota_tasks WHERE id = $1`

	var task OTATask
	err := r.Pool.QueryRow(ctx, q, taskID).Scan(
		&task.ID, &task.DeviceID, &task.TargetType, &task.TargetSocketNo, &task.FirmwareVersion,
		&task.FTPServer, &task.FTPPort, &task.FileName, &task.FileSize, &task.Status, &task.Progress,
		&task.ErrorMsg, &task.MsgID, &task.StartedAt, &task.CompletedAt, &task.CreatedAt, &task.UpdatedAt)
	if err != nil {
		return nil, err
	}

	return &task, nil
}

// UpdateOTATaskStatus 更新OTA任务状态
func (r *Repository) UpdateOTATaskStatus(ctx context.Context, taskID int64, status int, errorMsg *string) error {
	const q = `UPDATE ota_tasks 
	           SET status = $2, error_msg = $3, updated_at = NOW()
	           WHERE id = $1`

	_, err := r.Pool.Exec(ctx, q, taskID, status, errorMsg)
	return err
}

// UpdateOTATaskProgress 更新OTA任务进度
func (r *Repository) UpdateOTATaskProgress(ctx context.Context, taskID int64, progress int, status int) error {
	const q = `UPDATE ota_tasks 
	           SET progress = $2, status = $3, updated_at = NOW()
	           WHERE id = $1`

	_, err := r.Pool.Exec(ctx, q, taskID, progress, status)
	return err
}

// CompleteOTATask 完成OTA任务
func (r *Repository) CompleteOTATask(ctx context.Context, taskID int64, success bool, errorMsg *string) error {
	status := 3 // 成功
	if !success {
		status = 4 // 失败
	}

	const q = `UPDATE ota_tasks 
	           SET status = $2, progress = $3, error_msg = $4, completed_at = NOW(), updated_at = NOW()
	           WHERE id = $1`

	progress := 100
	if !success {
		progress = 0
	}

	_, err := r.Pool.Exec(ctx, q, taskID, status, progress, errorMsg)
	return err
}

// GetDeviceOTATasks 查询设备的OTA任务列表
func (r *Repository) GetDeviceOTATasks(ctx context.Context, deviceID int64, limit int) ([]OTATask, error) {
	const q = `SELECT id, device_id, target_type, target_socket_no, firmware_version, ftp_server, ftp_port,
	                  file_name, file_size, status, progress, error_msg, msg_id, started_at, completed_at, created_at, updated_at
	           FROM ota_tasks
	           WHERE device_id = $1
	           ORDER BY created_at DESC
	           LIMIT $2`

	rows, err := r.Pool.Query(ctx, q, deviceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []OTATask
	for rows.Next() {
		var task OTATask
		err := rows.Scan(
			&task.ID, &task.DeviceID, &task.TargetType, &task.TargetSocketNo, &task.FirmwareVersion,
			&task.FTPServer, &task.FTPPort, &task.FileName, &task.FileSize, &task.Status, &task.Progress,
			&task.ErrorMsg, &task.MsgID, &task.StartedAt, &task.CompletedAt, &task.CreatedAt, &task.UpdatedAt)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}

	return tasks, rows.Err()
}

// SetOTATaskMsgID 设置OTA任务的消息ID
func (r *Repository) SetOTATaskMsgID(ctx context.Context, taskID int64, msgID int) error {
	const q = `UPDATE ota_tasks 
	           SET msg_id = $2, status = 1, started_at = NOW(), updated_at = NOW()
	           WHERE id = $1`

	_, err := r.Pool.Exec(ctx, q, taskID, msgID)
	return err
}

// ===== P0修复: 订单状态管理方法 =====

// GetPendingOrderByPort 根据设备ID和端口号查询pending状态的订单
// 用于在端口开始充电时查找对应的订单进行状态更新
func (r *Repository) GetPendingOrderByPort(ctx context.Context, deviceID int64, portNo int) (*Order, error) {
	const q = `SELECT id, device_id, port_no, order_no, status, start_time, end_time, 
	                  amount_cent, kwh_0p01
	           FROM orders
	           WHERE device_id = $1 AND port_no = $2 AND status IN (0, 1)
	           ORDER BY created_at DESC
	           LIMIT 1`

	var ord Order

	err := r.Pool.QueryRow(ctx, q, deviceID, portNo).Scan(
		&ord.ID, &ord.DeviceID, &ord.PortNo, &ord.OrderNo, &ord.Status,
		&ord.StartTime, &ord.EndTime, &ord.AmountCent, &ord.Kwh01,
	)
	if err != nil {
		return nil, err
	}

	return &ord, nil
}

// UpdateOrderToCharging 将订单状态从pending更新为charging
// 仅当订单状态为pending(0)时才会更新，实现幂等性
func (r *Repository) UpdateOrderToCharging(ctx context.Context, orderNo string, startTime time.Time) error {
	const q = `UPDATE orders 
	           SET status = 1, start_time = $2, updated_at = NOW()
	           WHERE order_no = $1 AND status = 0
	           RETURNING id`

	var id int64
	err := r.Pool.QueryRow(ctx, q, orderNo, startTime).Scan(&id)
	// 如果没有行被更新（订单不存在或状态不是pending），不返回错误
	// 这样可以实现幂等性：多次调用不会出错
	if err != nil {
		if err.Error() == "no rows in result set" {
			return nil // 幂等：订单已经是charging或不存在
		}
		return fmt.Errorf("update order to charging: %w", err)
	}

	return nil
}
