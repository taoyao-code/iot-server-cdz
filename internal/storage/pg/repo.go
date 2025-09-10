package pg

import (
	"context"

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
