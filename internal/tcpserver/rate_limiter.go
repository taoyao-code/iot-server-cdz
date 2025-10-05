package tcpserver

import (
	"context"
	"sync/atomic"

	"golang.org/x/time/rate"
)

// RateLimiter 基于Token Bucket的速率限流器
type RateLimiter struct {
	limiter       *rate.Limiter
	ratePerSec    int
	burst         int
	allowedCount  atomic.Int64
	rejectedCount atomic.Int64
}

// NewRateLimiter 创建速率限流器
// ratePerSec: 每秒允许的请求数（稳定速率）
// burst: 突发容量（桶的大小）
func NewRateLimiter(ratePerSec int, burst int) *RateLimiter {
	if ratePerSec <= 0 {
		ratePerSec = 100 // 默认每秒100个连接
	}
	if burst <= 0 {
		burst = ratePerSec * 2 // 默认突发为稳定速率的2倍
	}

	return &RateLimiter{
		limiter:    rate.NewLimiter(rate.Limit(ratePerSec), burst),
		ratePerSec: ratePerSec,
		burst:      burst,
	}
}

// Allow 检查是否允许请求（非阻塞）
func (l *RateLimiter) Allow() bool {
	if l.limiter.Allow() {
		l.allowedCount.Add(1)
		return true
	}
	l.rejectedCount.Add(1)
	return false
}

// Wait 等待直到允许请求（阻塞，带超时）
func (l *RateLimiter) Wait(ctx context.Context) error {
	if err := l.limiter.Wait(ctx); err != nil {
		l.rejectedCount.Add(1)
		return err
	}
	l.allowedCount.Add(1)
	return nil
}

// AllowedCount 允许的请求数（累计）
func (l *RateLimiter) AllowedCount() int64 {
	return l.allowedCount.Load()
}

// RejectedCount 被拒绝的请求数（累计）
func (l *RateLimiter) RejectedCount() int64 {
	return l.rejectedCount.Load()
}

// Stats 获取统计信息
func (l *RateLimiter) Stats() RateLimiterStats {
	return RateLimiterStats{
		RatePerSecond: l.ratePerSec,
		Burst:         l.burst,
		AllowedTotal:  l.AllowedCount(),
		RejectedTotal: l.RejectedCount(),
	}
}

// RateLimiterStats 速率限流器统计信息
type RateLimiterStats struct {
	RatePerSecond int   `json:"rate_per_second"`
	Burst         int   `json:"burst"`
	AllowedTotal  int64 `json:"allowed_total"`
	RejectedTotal int64 `json:"rejected_total"`
}
