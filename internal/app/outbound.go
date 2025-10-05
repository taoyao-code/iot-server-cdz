package app

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/taoyao-code/iot-server/internal/metrics"
	"github.com/taoyao-code/iot-server/internal/outbound"
	"github.com/taoyao-code/iot-server/internal/session"
)

// StartOutbound 启动下行 Worker 并返回取消函数
// StartOutbound 启动PostgreSQL outbound worker
// P0完成: 支持接口类型以兼容内存和Redis会话管理器
func StartOutbound(dbpool *pgxpool.Pool, throttleMs int, retryMax int, deadRetentionDays int, appm *metrics.AppMetrics, sess session.SessionManager) (context.CancelFunc, *outbound.Worker) {
	wctx, wcancel := context.WithCancel(context.Background())
	outw := outbound.New(dbpool)
	outw.Throttle = time.Duration(throttleMs) * time.Millisecond
	if retryMax > 0 {
		outw.MaxRetries = retryMax
	}
	outw.DeadRetentionDays = deadRetentionDays
	outw.Metrics = appm
	outw.OnAckTimeout = func(phy string) { sess.OnAckTimeout(phy, time.Now()) }
	go outw.Run(wctx)
	return wcancel, outw
}
