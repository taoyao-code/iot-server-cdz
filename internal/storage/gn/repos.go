package gn

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DevicesRepo GN设备仓储接口
type DevicesRepo interface {
	// UpsertHeartbeat 插入或更新设备心跳信息
	UpsertHeartbeat(ctx context.Context, deviceID string, gatewayID string, iccid string, rssi int, fwVer string) error
	
	// FindByID 根据设备ID查找设备
	FindByID(ctx context.Context, deviceID string) (*Device, error)
	
	// UpdateSeen 更新设备最后见时间
	UpdateSeen(ctx context.Context, deviceID string) error
}

// PortsRepo GN端口仓储接口
type PortsRepo interface {
	// UpsertPortSnapshot 批量更新端口快照
	UpsertPortSnapshot(ctx context.Context, deviceID string, ports []PortSnapshot) error
	
	// ListByDevice 获取设备的所有端口
	ListByDevice(ctx context.Context, deviceID string) ([]PortSnapshot, error)
}

// InboundLogsRepo 入站日志仓储接口
type InboundLogsRepo interface {
	// Append 追加入站日志
	Append(ctx context.Context, deviceID string, cmd int, seq int, payloadHex string, parsedOK bool, reason string) error
}

// OutboundQueueRepo 出站队列仓储接口
type OutboundQueueRepo interface {
	// Enqueue 入队出站消息
	Enqueue(ctx context.Context, deviceID string, cmd int, seq int, payload []byte) (int64, error)
	
	// DequeueDue 获取到期的待发送消息
	DequeueDue(ctx context.Context, limit int) ([]OutboundMessage, error)
	
	// MarkSent 标记消息已发送
	MarkSent(ctx context.Context, id int64, nextTS time.Time) error
	
	// Ack 确认消息
	Ack(ctx context.Context, deviceID string, seq int) error
	
	// MarkDead 标记消息为死信
	MarkDead(ctx context.Context, id int64, reason string) error
	
	// ListStuckSince 获取卡住的消息
	ListStuckSince(ctx context.Context, ts time.Time) ([]OutboundMessage, error)
}

// ParamsPendingRepo 待处理参数仓储接口
type ParamsPendingRepo interface {
	// Add 添加待处理参数
	Add(ctx context.Context, deviceID string, paramID int, value string, seq int) error
	
	// ListByDevice 获取设备的待处理参数
	ListByDevice(ctx context.Context, deviceID string) ([]ParamPending, error)
	
	// Pop 移除已处理的参数
	Pop(ctx context.Context, deviceID string, paramID int) error
}

// Device GN设备信息
type Device struct {
	DeviceID  string     `json:"device_id"`
	GatewayID string     `json:"gateway_id"`
	ICCID     string     `json:"iccid"`
	LastSeen  *time.Time `json:"last_seen"`
	RSSI      int        `json:"rssi"`
	FwVer     string     `json:"fw_ver"`
}

// PortSnapshot GN端口快照
type PortSnapshot struct {
	DeviceID   string    `json:"device_id"`
	PortNo     int       `json:"port_no"`
	StatusBits int       `json:"status_bits"`
	BizNo      string    `json:"biz_no"`
	Voltage    float64   `json:"voltage"`    // V
	Current    float64   `json:"current"`    // A
	Power      float64   `json:"power"`      // W
	Energy     float64   `json:"energy"`     // kWh
	Duration   int       `json:"duration"`   // minutes
	UpdatedAt  time.Time `json:"updated_at"`
}

// OutboundMessage 出站消息
type OutboundMessage struct {
	ID       int64     `json:"id"`
	DeviceID string    `json:"device_id"`
	Cmd      int       `json:"cmd"`
	Seq      int       `json:"seq"`
	Payload  []byte    `json:"payload"`
	Status   int       `json:"status"` // 0=pending, 1=sent, 2=acked, 3=dead
	Tries    int       `json:"tries"`
	NextTS   time.Time `json:"next_ts"`
}

// ParamPending 待处理参数
type ParamPending struct {
	ID       int64  `json:"id"`
	DeviceID string `json:"device_id"`
	ParamID  int    `json:"param_id"`
	Value    string `json:"value"`
	Seq      int    `json:"seq"`
}

// PostgresRepos PostgreSQL实现的GN仓储集合
type PostgresRepos struct {
	pool     *pgxpool.Pool
	Devices  DevicesRepo
	Ports    PortsRepo
	Inbound  InboundLogsRepo
	Outbound OutboundQueueRepo
	Params   ParamsPendingRepo
}

// NewPostgresRepos 创建PostgreSQL仓储实现
func NewPostgresRepos(pool *pgxpool.Pool) *PostgresRepos {
	repos := &PostgresRepos{
		pool: pool,
	}
	
	// 创建具体实现
	repos.Devices = &postgresDevicesRepo{pool: pool}
	repos.Ports = &postgresPortsRepo{pool: pool}
	repos.Inbound = &postgresInboundLogsRepo{pool: pool}
	repos.Outbound = &postgresOutboundQueueRepo{pool: pool}
	repos.Params = &postgresParamsPendingRepo{pool: pool}
	
	return repos
}

// 设备仓储PostgreSQL实现
type postgresDevicesRepo struct {
	pool *pgxpool.Pool
}

func (r *postgresDevicesRepo) UpsertHeartbeat(ctx context.Context, deviceID string, gatewayID string, iccid string, rssi int, fwVer string) error {
	const q = `INSERT INTO devices (phy_id, gateway_id, iccid, rssi, fw_ver, last_seen_at, created_at, updated_at)
               VALUES ($1, $2, $3, $4, $5, NOW(), NOW(), NOW())
               ON CONFLICT (phy_id) 
               DO UPDATE SET 
                 gateway_id = EXCLUDED.gateway_id,
                 iccid = EXCLUDED.iccid,
                 rssi = EXCLUDED.rssi,
                 fw_ver = EXCLUDED.fw_ver,
                 last_seen_at = NOW(),
                 updated_at = NOW()`
	_, err := r.pool.Exec(ctx, q, deviceID, gatewayID, iccid, rssi, fwVer)
	return err
}

func (r *postgresDevicesRepo) FindByID(ctx context.Context, deviceID string) (*Device, error) {
	const q = `SELECT phy_id, gateway_id, iccid, last_seen_at, rssi, fw_ver
               FROM devices WHERE phy_id = $1`
	
	device := &Device{}
	err := r.pool.QueryRow(ctx, q, deviceID).Scan(
		&device.DeviceID, &device.GatewayID, &device.ICCID, 
		&device.LastSeen, &device.RSSI, &device.FwVer)
	
	if err != nil {
		return nil, err
	}
	
	return device, nil
}

func (r *postgresDevicesRepo) UpdateSeen(ctx context.Context, deviceID string) error {
	const q = `UPDATE devices SET last_seen_at = NOW(), updated_at = NOW() WHERE phy_id = $1`
	_, err := r.pool.Exec(ctx, q, deviceID)
	return err
}

// 端口仓储PostgreSQL实现
type postgresPortsRepo struct {
	pool *pgxpool.Pool
}

func (r *postgresPortsRepo) UpsertPortSnapshot(ctx context.Context, deviceID string, ports []PortSnapshot) error {
	if len(ports) == 0 {
		return nil
	}
	
	// 首先获取设备内部ID
	var internalDeviceID int64
	err := r.pool.QueryRow(ctx, `SELECT id FROM devices WHERE phy_id = $1`, deviceID).Scan(&internalDeviceID)
	if err != nil {
		return err
	}
	
	// 批量更新端口
	for _, port := range ports {
		const q = `INSERT INTO ports (device_id, port_no, status, status_bits, biz_no, voltage, current, power, energy, duration, updated_at)
                   VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW())
                   ON CONFLICT (device_id, port_no)
                   DO UPDATE SET
                     status_bits = EXCLUDED.status_bits,
                     biz_no = EXCLUDED.biz_no,
                     voltage = EXCLUDED.voltage,
                     current = EXCLUDED.current,
                     power = EXCLUDED.power,
                     energy = EXCLUDED.energy,
                     duration = EXCLUDED.duration,
                     updated_at = NOW()`
		
		_, err = r.pool.Exec(ctx, q, internalDeviceID, port.PortNo, port.StatusBits, 
			port.StatusBits, port.BizNo, port.Voltage, port.Current, port.Power, port.Energy, port.Duration)
		if err != nil {
			return err
		}
	}
	
	return nil
}

func (r *postgresPortsRepo) ListByDevice(ctx context.Context, deviceID string) ([]PortSnapshot, error) {
	const q = `SELECT p.port_no, p.status_bits, p.biz_no, p.voltage, p.current, p.power, p.energy, p.duration, p.updated_at
               FROM ports p 
               JOIN devices d ON p.device_id = d.id 
               WHERE d.phy_id = $1 
               ORDER BY p.port_no`
	
	rows, err := r.pool.Query(ctx, q, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var ports []PortSnapshot
	for rows.Next() {
		var port PortSnapshot
		port.DeviceID = deviceID
		
		err := rows.Scan(&port.PortNo, &port.StatusBits, &port.BizNo,
			&port.Voltage, &port.Current, &port.Power, &port.Energy, &port.Duration, &port.UpdatedAt)
		if err != nil {
			return nil, err
		}
		
		ports = append(ports, port)
	}
	
	return ports, rows.Err()
}

// 入站日志仓储PostgreSQL实现
type postgresInboundLogsRepo struct {
	pool *pgxpool.Pool
}

func (r *postgresInboundLogsRepo) Append(ctx context.Context, deviceID string, cmd int, seq int, payloadHex string, parsedOK bool, reason string) error {
	// 获取设备内部ID
	var internalDeviceID int64
	err := r.pool.QueryRow(ctx, `SELECT id FROM devices WHERE phy_id = $1`, deviceID).Scan(&internalDeviceID)
	if err != nil {
		return err
	}
	
	const q = `INSERT INTO inbound_logs (device_id, cmd, seq, payload_hex, parsed_ok, reason, created_at)
               VALUES ($1, $2, $3, $4, $5, $6, NOW())`
	_, err = r.pool.Exec(ctx, q, internalDeviceID, cmd, seq, payloadHex, parsedOK, reason)
	return err
}

// 出站队列仓储PostgreSQL实现
type postgresOutboundQueueRepo struct {
	pool *pgxpool.Pool
}

func (r *postgresOutboundQueueRepo) Enqueue(ctx context.Context, deviceID string, cmd int, seq int, payload []byte) (int64, error) {
	// 获取设备内部ID
	var internalDeviceID int64
	err := r.pool.QueryRow(ctx, `SELECT id FROM devices WHERE phy_id = $1`, deviceID).Scan(&internalDeviceID)
	if err != nil {
		return 0, err
	}
	
	const q = `INSERT INTO outbound_queue (device_id, cmd, seq, payload, status, tries, created_at)
               VALUES ($1, $2, $3, $4, 0, 0, NOW())
               RETURNING id`
	
	var id int64
	err = r.pool.QueryRow(ctx, q, internalDeviceID, cmd, seq, payload).Scan(&id)
	return id, err
}

func (r *postgresOutboundQueueRepo) DequeueDue(ctx context.Context, limit int) ([]OutboundMessage, error) {
	const q = `SELECT o.id, d.phy_id, o.cmd, o.seq, o.payload, o.status, o.tries, COALESCE(o.next_ts, NOW())
               FROM outbound_queue o
               JOIN devices d ON o.device_id = d.id
               WHERE o.status IN (0, 1) AND (o.next_ts IS NULL OR o.next_ts <= NOW())
               ORDER BY o.created_at
               LIMIT $1`
	
	rows, err := r.pool.Query(ctx, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var messages []OutboundMessage
	for rows.Next() {
		var msg OutboundMessage
		err := rows.Scan(&msg.ID, &msg.DeviceID, &msg.Cmd, &msg.Seq, &msg.Payload, &msg.Status, &msg.Tries, &msg.NextTS)
		if err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}
	
	return messages, rows.Err()
}

func (r *postgresOutboundQueueRepo) MarkSent(ctx context.Context, id int64, nextTS time.Time) error {
	const q = `UPDATE outbound_queue SET status = 1, tries = tries + 1, next_ts = $2 WHERE id = $1`
	_, err := r.pool.Exec(ctx, q, id, nextTS)
	return err
}

func (r *postgresOutboundQueueRepo) Ack(ctx context.Context, deviceID string, seq int) error {
	const q = `UPDATE outbound_queue o SET status = 2 
               FROM devices d 
               WHERE o.device_id = d.id AND d.phy_id = $1 AND o.seq = $2`
	_, err := r.pool.Exec(ctx, q, deviceID, seq)
	return err
}

func (r *postgresOutboundQueueRepo) MarkDead(ctx context.Context, id int64, reason string) error {
	const q = `UPDATE outbound_queue SET status = 3, reason = $2 WHERE id = $1`
	_, err := r.pool.Exec(ctx, q, id, reason)
	return err
}

func (r *postgresOutboundQueueRepo) ListStuckSince(ctx context.Context, ts time.Time) ([]OutboundMessage, error) {
	const q = `SELECT o.id, d.phy_id, o.cmd, o.seq, o.payload, o.status, o.tries, o.next_ts
               FROM outbound_queue o
               JOIN devices d ON o.device_id = d.id
               WHERE o.status = 1 AND o.next_ts < $1`
	
	rows, err := r.pool.Query(ctx, q, ts)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var messages []OutboundMessage
	for rows.Next() {
		var msg OutboundMessage
		err := rows.Scan(&msg.ID, &msg.DeviceID, &msg.Cmd, &msg.Seq, &msg.Payload, &msg.Status, &msg.Tries, &msg.NextTS)
		if err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}
	
	return messages, rows.Err()
}

// 参数仓储PostgreSQL实现
type postgresParamsPendingRepo struct {
	pool *pgxpool.Pool
}

func (r *postgresParamsPendingRepo) Add(ctx context.Context, deviceID string, paramID int, value string, seq int) error {
	// 获取设备内部ID
	var internalDeviceID int64
	err := r.pool.QueryRow(ctx, `SELECT id FROM devices WHERE phy_id = $1`, deviceID).Scan(&internalDeviceID)
	if err != nil {
		return err
	}
	
	const q = `INSERT INTO params_pending (device_id, param_id, value, seq, created_at)
               VALUES ($1, $2, $3, $4, NOW())`
	_, err = r.pool.Exec(ctx, q, internalDeviceID, paramID, value, seq)
	return err
}

func (r *postgresParamsPendingRepo) ListByDevice(ctx context.Context, deviceID string) ([]ParamPending, error) {
	const q = `SELECT p.id, d.phy_id, p.param_id, p.value, p.seq
               FROM params_pending p
               JOIN devices d ON p.device_id = d.id
               WHERE d.phy_id = $1
               ORDER BY p.created_at`
	
	rows, err := r.pool.Query(ctx, q, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var params []ParamPending
	for rows.Next() {
		var param ParamPending
		err := rows.Scan(&param.ID, &param.DeviceID, &param.ParamID, &param.Value, &param.Seq)
		if err != nil {
			return nil, err
		}
		params = append(params, param)
	}
	
	return params, rows.Err()
}

func (r *postgresParamsPendingRepo) Pop(ctx context.Context, deviceID string, paramID int) error {
	const q = `DELETE FROM params_pending p
               USING devices d 
               WHERE p.device_id = d.id AND d.phy_id = $1 AND p.param_id = $2`
	_, err := r.pool.Exec(ctx, q, deviceID, paramID)
	return err
}