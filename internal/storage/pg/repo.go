package pg

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository 提供最小持久化能力
type Repository struct {
	Pool *pgxpool.Pool
}

// EnsureDevice 返回设备ID，若不存在则插入（不会刷新已存在设备的 last_seen_at）
func (r *Repository) EnsureDevice(ctx context.Context, phyID string) (int64, error) {
	const q = `INSERT INTO devices (phy_id, last_seen_at)
               VALUES ($1, NOW())
               ON CONFLICT (phy_id) DO UPDATE SET updated_at = NOW()
               RETURNING id`
	var id int64
	err := r.Pool.QueryRow(ctx, q, phyID).Scan(&id)
	return id, err
}

// TouchDeviceLastSeen 刷新设备最近心跳时间（存在则更新，不存在则插入）
func (r *Repository) TouchDeviceLastSeen(ctx context.Context, phyID string, at time.Time) error {
	const q = `INSERT INTO devices (phy_id, last_seen_at)
               VALUES ($1, $2)
               ON CONFLICT (phy_id)
               DO UPDATE SET last_seen_at = EXCLUDED.last_seen_at, updated_at = NOW()`
	_, err := r.Pool.Exec(ctx, q, phyID, at)
	return err
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
	_, err := r.Pool.Exec(ctx, q, deviceID, portNo, orderHex, status, kwh01)
	return err
}

// SettleOrder 结算订单（结束时间、耗电、金额占位、结束原因）
func (r *Repository) SettleOrder(ctx context.Context, deviceID int64, portNo int, orderHex string, durationSec int, kwh01 int, reason int) error {
	const q = `INSERT INTO orders (device_id, port_no, order_no, start_time, end_time, kwh_0p01, status)
               VALUES ($1,$2,$3,NOW()-make_interval(secs => $4), NOW(), $5, 2)
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
	ID            int64
	DeviceID      int64
	PhyID         string
	PortNo        int
	OrderNo       string
	ChargeMode    int // 充电模式: 1=按时长, 2=按电量, 3=按功率, 4=充满自停
	StartTime     *time.Time
	EndTime       *time.Time
	Kwh01         *int64
	AmountCent    *int64
	Status        int
	TestSessionID *string
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
	const q = `SELECT o.id, o.device_id, d.phy_id, o.port_no, o.order_no, o.charge_mode, o.start_time, o.end_time, o.kwh_0p01, o.amount_cent, o.status, o.test_session_id
		FROM orders o JOIN devices d ON o.device_id = d.id WHERE o.id=$1`
	var (
		ord       Order
		kwh       *int64
		amt       *int64
		sessionID *string
	)
	if err := r.Pool.QueryRow(ctx, q, id).Scan(&ord.ID, &ord.DeviceID, &ord.PhyID, &ord.PortNo, &ord.OrderNo, &ord.ChargeMode, &ord.StartTime, &ord.EndTime, &kwh, &amt, &ord.Status, &sessionID); err != nil {
		return nil, err
	}
	ord.Kwh01 = kwh
	ord.AmountCent = amt
	ord.TestSessionID = sessionID
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
	const q = `SELECT o.id, o.device_id, d.phy_id, o.port_no, o.order_no, o.start_time, o.end_time, o.kwh_0p01, o.amount_cent, o.status, o.test_session_id
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
			ord       Order
			kwh       *int64
			amt       *int64
			sessionID *string
		)
		if err := rows.Scan(&ord.ID, &ord.DeviceID, &ord.PhyID, &ord.PortNo, &ord.OrderNo, &ord.StartTime, &ord.EndTime, &kwh, &amt, &ord.Status, &sessionID); err != nil {
			return nil, err
		}
		ord.Kwh01 = kwh
		ord.AmountCent = amt
		ord.TestSessionID = sessionID
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

// [其余代码保持不变...]
