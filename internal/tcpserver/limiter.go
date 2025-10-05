package tcpserver

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"
)

// ConnectionLimiter 连接数限流器（基于Semaphore）
type ConnectionLimiter struct {
	sem           chan struct{}
	timeout       time.Duration
	maxConn       int
	activeCount   atomic.Int64
	rejectedCount atomic.Int64
}

// NewConnectionLimiter 创建连接限流器
// maxConn: 最大并发连接数
// timeout: 获取连接许可的超时时间
func NewConnectionLimiter(maxConn int, timeout time.Duration) *ConnectionLimiter {
	if maxConn <= 0 {
		maxConn = 10000
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	return &ConnectionLimiter{
		sem:     make(chan struct{}, maxConn),
		timeout: timeout,
		maxConn: maxConn,
	}
}

// Acquire 获取连接许可
func (l *ConnectionLimiter) Acquire(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, l.timeout)
	defer cancel()

	select {
	case l.sem <- struct{}{}:
		l.activeCount.Add(1)
		return nil
	case <-ctx.Done():
		l.rejectedCount.Add(1)
		return fmt.Errorf("connection limit exceeded: max=%d", l.maxConn)
	}
}

// Release 释放连接许可
func (l *ConnectionLimiter) Release() {
	select {
	case <-l.sem:
		l.activeCount.Add(-1)
	default:
		// 不应该发生，但防御性编程
	}
}

// Current 当前活跃连接数
func (l *ConnectionLimiter) Current() int {
	return int(l.activeCount.Load())
}

// Available 可用连接数
func (l *ConnectionLimiter) Available() int {
	return l.maxConn - l.Current()
}

// MaxConnections 最大连接数
func (l *ConnectionLimiter) MaxConnections() int {
	return l.maxConn
}

// RejectedCount 被拒绝的连接数（累计）
func (l *ConnectionLimiter) RejectedCount() int64 {
	return l.rejectedCount.Load()
}

// Stats 获取统计信息
func (l *ConnectionLimiter) Stats() LimiterStats {
	return LimiterStats{
		MaxConnections:    l.maxConn,
		ActiveConnections: l.Current(),
		RejectedTotal:     l.RejectedCount(),
		Utilization:       float64(l.Current()) / float64(l.maxConn),
	}
}

// LimiterStats 限流器统计信息
type LimiterStats struct {
	MaxConnections    int     `json:"max_connections"`
	ActiveConnections int     `json:"active_connections"`
	RejectedTotal     int64   `json:"rejected_total"`
	Utilization       float64 `json:"utilization"` // 0.0 - 1.0
}
