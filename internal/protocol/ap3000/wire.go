package ap3000

import (
	"context"

	"github.com/taoyao-code/iot-server/internal/metrics"
	pgstorage "github.com/taoyao-code/iot-server/internal/storage/pg"
)

// NewHandlers 构造 AP3000 处理集合
func NewHandlers(repo *pgstorage.Repository, pusher interface {
	SendJSON(ctx context.Context, endpoint string, payload any) (int, []byte, error)
}, pushURL string, m *metrics.AppMetrics,
) *Handlers {
	return &Handlers{Repo: repo, Pusher: pusher, PushURL: pushURL, Metrics: m}
}
