package outbound

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/taoyao-code/iot-server/internal/metrics"
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
	// DeadRetentionDays 用于删除已失败(dead)记录的保留天数；<=0 时不清理
	DeadRetentionDays int
	lastCleanAt       time.Time
	// 可选：指标
	Metrics *metrics.AppMetrics
	// 可选：ACK 超时回调（用于会话多信号判定）
	OnAckTimeout func(phyID string)
}

func New(db *pgxpool.Pool) *Worker {
	return &Worker{DB: db, Interval: time.Second, BatchSize: 50, Throttle: 500 * time.Millisecond, MaxRetries: 3, DeadRetentionDays: 7}
}

// SetGetConn 安装连接获取函数
func (w *Worker) SetGetConn(fn func(phyID string) (interface{}, bool)) { w.GetConn = fn }

func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.Interval)
	defer ticker.Stop()
	// 冷启立即扫描一轮（处理超时与可发送项）
	w.tick(ctx)
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
	// 周期性清理 dead 记录
	if w.DeadRetentionDays > 0 {
		if time.Since(w.lastCleanAt) >= time.Hour {
			w.cleanDead(ctx)
			w.lastCleanAt = time.Now()
		}
	}
	// 刷新队列积压 Gauge
	w.refreshQueueSize(ctx)
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
			if w.Metrics != nil {
				w.Metrics.OutboundResendTotal.Inc()
			}
			continue
		}
		conn, ok := w.GetConn(*phyID)
		if !ok {
			_, _ = w.DB.Exec(ctx, `UPDATE outbound_queue SET retry_count=retry_count+1, not_before=NOW()+INTERVAL '3 seconds'*GREATEST(retry_count,1) WHERE id=$1`, id)
			if w.Metrics != nil {
				w.Metrics.OutboundResendTotal.Inc()
			}
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
			if w.Metrics != nil {
				w.Metrics.OutboundResendTotal.Inc()
			}
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
	rows, err := w.DB.Query(ctx, `SELECT id, retry_count, timeout_sec, COALESCE(phy_id, '') FROM outbound_queue
        WHERE status=1 AND timeout_sec IS NOT NULL AND updated_at + (timeout_sec || ' seconds')::interval <= NOW()`)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var rc int
		var to int
		var phy string
		if err := rows.Scan(&id, &rc, &to, &phy); err != nil {
			continue
		}
		if w.MaxRetries > 0 && rc >= w.MaxRetries {
			_, _ = w.DB.Exec(ctx, `UPDATE outbound_queue SET status=3, last_error=COALESCE(last_error,'')||' timeout_dead', updated_at=NOW() WHERE id=$1`, id)
			if w.Metrics != nil {
				w.Metrics.OutboundTimeoutTotal.Inc()
				w.Metrics.SessionOfflineTotal.WithLabelValues("ack").Inc()
			}
			if w.OnAckTimeout != nil && phy != "" {
				w.OnAckTimeout(phy)
			}
			continue
		}
		_, _ = w.DB.Exec(ctx, `UPDATE outbound_queue SET status=0, retry_count=retry_count+1,
            not_before=NOW()+INTERVAL '3 seconds'*GREATEST(retry_count,1), last_error=COALESCE(last_error,'')||' timeout', updated_at=NOW()
            WHERE id=$1`, id)
		if w.Metrics != nil {
			w.Metrics.OutboundTimeoutTotal.Inc()
			w.Metrics.OutboundResendTotal.Inc()
			w.Metrics.SessionOfflineTotal.WithLabelValues("ack").Inc()
		}
		if w.OnAckTimeout != nil && phy != "" {
			w.OnAckTimeout(phy)
		}
	}
}

// 刷新队列积压 Gauge
func (w *Worker) refreshQueueSize(ctx context.Context) {
	if w.Metrics == nil || w.Metrics.OutboundQueueSize == nil {
		return
	}
	// 先清零 0..3
	for _, s := range []string{"0", "1", "2", "3"} {
		w.Metrics.OutboundQueueSize.WithLabelValues(s).Set(0)
	}
	rows, err := w.DB.Query(ctx, `SELECT status, COUNT(*) FROM outbound_queue GROUP BY status`)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var st int
		var c int64
		if err := rows.Scan(&st, &c); err != nil {
			continue
		}
		w.Metrics.OutboundQueueSize.WithLabelValues(
			fmt.Sprintf("%d", st),
		).Set(float64(c))
	}
}

// cleanDead 清理过期的 dead 记录（status=3），保留最近 DeadRetentionDays 天
func (w *Worker) cleanDead(ctx context.Context) {
	ct, _ := w.DB.Exec(ctx, `DELETE FROM outbound_queue WHERE status=3 AND updated_at < NOW() - ($1 || ' days')::interval`, w.DeadRetentionDays)
	if w.Metrics != nil {
		// pgxpool.CommandTag has RowsAffected()
		if n := ct.RowsAffected(); n > 0 {
			for i := int64(0); i < n; i++ {
				w.Metrics.OutboundDeadCleanupTotal.Inc()
			}
		}
	}
}
