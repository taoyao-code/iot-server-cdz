package health

import (
	"context"
	"sync"
	"time"
)

// Aggregator 健康检查聚合器
type Aggregator struct {
	checkers []Checker
	mu       sync.RWMutex
}

// NewAggregator 创建聚合器
func NewAggregator(checkers ...Checker) *Aggregator {
	return &Aggregator{
		checkers: checkers,
	}
}

// AddChecker 添加检查器
func (a *Aggregator) AddChecker(checker Checker) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.checkers = append(a.checkers, checker)
}

// CheckAll 执行所有健康检查（并发）
func (a *Aggregator) CheckAll(ctx context.Context) map[string]CheckResult {
	a.mu.RLock()
	defer a.mu.RUnlock()

	results := make(map[string]CheckResult)
	resultsMu := sync.Mutex{}
	wg := sync.WaitGroup{}

	for _, checker := range a.checkers {
		wg.Add(1)
		go func(c Checker) {
			defer wg.Done()

			result := c.Check(ctx)

			resultsMu.Lock()
			results[c.Name()] = result
			resultsMu.Unlock()
		}(checker)
	}

	wg.Wait()
	return results
}

// OverallStatus 计算总体健康状态
func (a *Aggregator) OverallStatus(ctx context.Context) Status {
	results := a.CheckAll(ctx)

	unhealthyCount := 0
	degradedCount := 0

	for _, result := range results {
		switch result.Status {
		case StatusUnhealthy:
			unhealthyCount++
		case StatusDegraded:
			degradedCount++
		}
	}

	// 任何组件Unhealthy，整体Unhealthy
	if unhealthyCount > 0 {
		return StatusUnhealthy
	}

	// 任何组件Degraded，整体Degraded
	if degradedCount > 0 {
		return StatusDegraded
	}

	return StatusHealthy
}

// Ready 判断系统是否就绪（用于K8s readiness probe）
func (a *Aggregator) Ready(ctx context.Context) bool {
	status := a.OverallStatus(ctx)
	// Degraded状态仍然就绪，只有Unhealthy才不就绪
	return status != StatusUnhealthy
}

// Alive 判断系统是否存活（用于K8s liveness probe）
// 简单返回true，因为如果进程挂了就不会响应
func (a *Aggregator) Alive() bool {
	return true
}

// HealthReport 生成健康报告
type HealthReport struct {
	Status    Status                 `json:"status"`
	Timestamp time.Time              `json:"timestamp"`
	Checks    map[string]CheckResult `json:"checks"`
}
