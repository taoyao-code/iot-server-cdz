package outbound

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/taoyao-code/iot-server/internal/protocol/ap3000"
	"github.com/taoyao-code/iot-server/internal/protocol/bkv"
)

// Worker 最小下行队列消费者（占位：仅标记已发送/完成）
type Worker struct {
	DB         *pgxpool.Pool
	Interval   time.Duration
	BatchSize  int
	Throttle   time.Duration
	MaxRetries int
	// 获取连接：返回具有 Write([]byte) error 能力的对象
	GetConn func(phyID string) (interface{}, bool)
}

func New(db *pgxpool.Pool) *Worker {
	return &Worker{DB: db, Interval: time.Second, BatchSize: 50, Throttle: 500 * time.Millisecond, MaxRetries: 3}
}

// SetGetConn 安装连接获取函数
func (w *Worker) SetGetConn(fn func(phyID string) (interface{}, bool)) { w.GetConn = fn }

func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.tick(ctx)
		}
	}
}

func (w *Worker) tick(ctx context.Context) {
	if w.DB == nil {
		return
	}
	// 先处理ACK超时的已发送任务（status=1）
	w.sweepTimeouts(ctx)
	// 选取待发送任务（status=0）
	rows, err := w.DB.Query(ctx, `SELECT id, phy_id, cmd, payload FROM outbound_queue
        WHERE status=0 AND (not_before IS NULL OR not_before<=NOW())
        ORDER BY priority, created_at
        LIMIT $1`, w.BatchSize)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var phyID *string
		var cmd int
		var payload []byte
		if err := rows.Scan(&id, &phyID, &cmd, &payload); err != nil {
			continue
		}

		if phyID == nil || w.GetConn == nil {
			// 无法发送：回退重试
			_, _ = w.DB.Exec(ctx, `UPDATE outbound_queue SET retry_count=retry_count+1, not_before=NOW()+INTERVAL '3 seconds'*GREATEST(retry_count,1) WHERE id=$1`, id)
			continue
		}
		conn, ok := w.GetConn(*phyID)
		if !ok {
			_, _ = w.DB.Exec(ctx, `UPDATE outbound_queue SET retry_count=retry_count+1, not_before=NOW()+INTERVAL '3 seconds'*GREATEST(retry_count,1) WHERE id=$1`, id)
			continue
		}
		type writer interface {
			Write([]byte) error
			Protocol() string
		}
		wconn, ok := conn.(writer)
		if !ok {
			// 类型不匹配，标记失败
			_, _ = w.DB.Exec(ctx, `UPDATE outbound_queue SET status=3, last_error='no writer', updated_at=NOW() WHERE id=$1`, id)
			continue
		}
		// 生成下行帧（根据协议选择编码）
		msgID := uint16(id & 0xFFFF)
		var frame []byte
		switch wconn.Protocol() {
		case "bkv":
			frame = bkv.Build(byte(cmd), payload)
		default:
			frame = ap3000.Build(*phyID, msgID, byte(cmd), payload)
		}
		if err := wconn.Write(frame); err != nil {
			// 写失败，回退重试
			_, _ = w.DB.Exec(ctx, `UPDATE outbound_queue SET retry_count=retry_count+1, not_before=NOW()+INTERVAL '3 seconds'*GREATEST(retry_count,1), last_error=$2 WHERE id=$1`, id, err.Error())
			continue
		}
		// 标记已发送并记录 msg_id，等待回执更新为 done
		_, _ = w.DB.Exec(ctx, `UPDATE outbound_queue SET status=1, msg_id=$2, updated_at=NOW() WHERE id=$1`, id, int(msgID))
		// 节流
		time.Sleep(w.Throttle)
	}
}

// sweepTimeouts 扫描已发送但超过超时的任务，按退避策略重试或置为dead
func (w *Worker) sweepTimeouts(ctx context.Context) {
	rows, err := w.DB.Query(ctx, `SELECT id, retry_count, timeout_sec FROM outbound_queue
        WHERE status=1 AND timeout_sec IS NOT NULL AND updated_at + (timeout_sec || ' seconds')::interval <= NOW()`)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var rc int
		var to int
		if err := rows.Scan(&id, &rc, &to); err != nil {
			continue
		}
		if w.MaxRetries > 0 && rc >= w.MaxRetries {
			_, _ = w.DB.Exec(ctx, `UPDATE outbound_queue SET status=3, last_error=COALESCE(last_error,'')||' timeout_dead', updated_at=NOW() WHERE id=$1`, id)
			continue
		}
		_, _ = w.DB.Exec(ctx, `UPDATE outbound_queue SET status=0, retry_count=retry_count+1,
            not_before=NOW()+INTERVAL '3 seconds'*GREATEST(retry_count,1), last_error=COALESCE(last_error,'')||' timeout', updated_at=NOW()
            WHERE id=$1`, id)
	}
}
