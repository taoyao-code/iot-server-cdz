package health

import (
	"context"
	"time"
)

// Status 健康状态
type Status string

const (
	StatusHealthy   Status = "healthy"   // 健康
	StatusDegraded  Status = "degraded"  // 降级（部分功能受损但仍可服务）
	StatusUnhealthy Status = "unhealthy" // 不健康（无法服务）
)

// CheckResult 健康检查结果
type CheckResult struct {
	Status  Status                 `json:"status"`
	Message string                 `json:"message,omitempty"`
	Details map[string]interface{} `json:"details,omitempty"`
	Latency time.Duration          `json:"latency"`
}

// Checker 健康检查器接口
type Checker interface {
	Name() string
	Check(ctx context.Context) CheckResult
}
