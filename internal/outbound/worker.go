package outbound

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Worker 最小下行队列消费者（占位：仅标记已发送/完成）
type Worker struct {
	DB        *pgxpool.Pool
	Interval  time.Duration
	BatchSize int
	Throttle  time.Duration
}

func New(db *pgxpool.Pool) *Worker {
	return &Worker{DB: db, Interval: time.Second, BatchSize: 50, Throttle: 500 * time.Millisecond}
}

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
	// 选取待发送任务（status=0）
	rows, err := w.DB.Query(ctx, `SELECT id, device_id, port_no, cmd, payload FROM outbound_queue
        WHERE status=0 AND (not_before IS NULL OR not_before<=NOW())
        ORDER BY priority, created_at
        LIMIT $1`, w.BatchSize)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var deviceID int64
		var portNo *int32
		var cmd int
		var payload []byte
		if err := rows.Scan(&id, &deviceID, &portNo, &cmd, &payload); err != nil {
			continue
		}

		// 占位：直接标记为 sent->done
		_, _ = w.DB.Exec(ctx, `UPDATE outbound_queue SET status=1 WHERE id=$1`, id)
		// 模拟发送耗时与节流
		time.Sleep(w.Throttle)
		_, _ = w.DB.Exec(ctx, `UPDATE outbound_queue SET status=2 WHERE id=$1`, id)
	}
}
