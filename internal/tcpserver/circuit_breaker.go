package tcpserver

import (
	"errors"
	"sync"
	"time"
)

// State 熔断器状态
type State int

const (
	StateClosed   State = iota // 正常状态，允许请求通过
	StateOpen                  // 熔断状态，拒绝所有请求
	StateHalfOpen              // 半开状态，允许少量请求试探
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half_open"
	default:
		return "unknown"
	}
}

// CircuitBreaker 熔断器
type CircuitBreaker struct {
	mu            sync.RWMutex
	state         State
	failureCount  int
	successCount  int
	lastFailTime  time.Time
	lastStateTime time.Time
	tripCount     int64 // 熔断次数

	// 配置
	threshold   int           // 失败次数阈值（触发熔断）
	timeout     time.Duration // 熔断超时（Open → HalfOpen）
	halfOpenMax int           // 半开状态最大测试请求数

	// 回调
	onStateChange func(from, to State)
}

// NewCircuitBreaker 创建熔断器
func NewCircuitBreaker(threshold int, timeout time.Duration) *CircuitBreaker {
	if threshold <= 0 {
		threshold = 5 // 默认连续5次失败触发熔断
	}
	if timeout <= 0 {
		timeout = 30 * time.Second // 默认30秒后尝试恢复
	}

	return &CircuitBreaker{
		state:         StateClosed,
		threshold:     threshold,
		timeout:       timeout,
		halfOpenMax:   5, // 半开状态允许5个测试请求
		lastStateTime: time.Now(),
	}
}

var (
	// ErrCircuitOpen 熔断器打开，拒绝请求
	ErrCircuitOpen = errors.New("circuit breaker is open")
	// ErrTooManyRequests 半开状态请求过多
	ErrTooManyRequests = errors.New("too many requests in half-open state")
)

// Call 执行函数，受熔断器保护
func (cb *CircuitBreaker) Call(fn func() error) error {
	if err := cb.beforeCall(); err != nil {
		return err
	}

	err := fn()
	cb.afterCall(err)

	return err
}

// beforeCall 调用前检查
func (cb *CircuitBreaker) beforeCall() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateClosed:
		// 正常状态，允许通过
		return nil

	case StateOpen:
		// 检查是否可以进入半开状态
		if time.Since(cb.lastFailTime) > cb.timeout {
			cb.transitionTo(StateHalfOpen)
			cb.failureCount = 0
			cb.successCount = 0
			return nil
		}
		// 仍在熔断期
		return ErrCircuitOpen

	case StateHalfOpen:
		// 半开状态，限制请求数
		if cb.successCount+cb.failureCount >= cb.halfOpenMax {
			return ErrTooManyRequests
		}
		return nil

	default:
		return ErrCircuitOpen
	}
}

// afterCall 调用后记录结果
func (cb *CircuitBreaker) afterCall(err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err != nil {
		// 失败
		cb.onFailure()
	} else {
		// 成功
		cb.onSuccess()
	}
}

// onFailure 处理失败
func (cb *CircuitBreaker) onFailure() {
	cb.failureCount++
	cb.lastFailTime = time.Now()

	switch cb.state {
	case StateClosed:
		// 检查是否达到阈值
		if cb.failureCount >= cb.threshold {
			cb.transitionTo(StateOpen)
			cb.tripCount++
		}

	case StateHalfOpen:
		// 半开状态失败，立即熔断
		cb.transitionTo(StateOpen)
		cb.tripCount++
	}
}

// onSuccess 处理成功
func (cb *CircuitBreaker) onSuccess() {
	cb.successCount++

	switch cb.state {
	case StateHalfOpen:
		// 半开状态成功足够次数，恢复正常
		if cb.successCount >= cb.halfOpenMax/2 {
			cb.transitionTo(StateClosed)
			cb.failureCount = 0
			cb.successCount = 0
		}

	case StateClosed:
		// 正常状态成功，重置失败计数
		if cb.successCount%100 == 0 {
			cb.failureCount = 0
		}
	}
}

// transitionTo 状态转换
func (cb *CircuitBreaker) transitionTo(newState State) {
	if cb.state == newState {
		return
	}

	oldState := cb.state
	cb.state = newState
	cb.lastStateTime = time.Now()

	if cb.onStateChange != nil {
		// 异步回调，避免阻塞
		go cb.onStateChange(oldState, newState)
	}
}

// State 获取当前状态
func (cb *CircuitBreaker) State() State {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// Stats 获取统计信息
func (cb *CircuitBreaker) Stats() CircuitBreakerStats {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	return CircuitBreakerStats{
		State:             cb.state.String(),
		FailureCount:      cb.failureCount,
		SuccessCount:      cb.successCount,
		TripCount:         cb.tripCount,
		LastStateChange:   cb.lastStateTime,
		TimeSinceLastFail: time.Since(cb.lastFailTime),
	}
}

// SetStateChangeCallback 设置状态变化回调
func (cb *CircuitBreaker) SetStateChangeCallback(fn func(from, to State)) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.onStateChange = fn
}

// Reset 重置熔断器（用于测试或手动恢复）
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.transitionTo(StateClosed)
	cb.failureCount = 0
	cb.successCount = 0
}

// CircuitBreakerStats 熔断器统计信息
type CircuitBreakerStats struct {
	State             string        `json:"state"`
	FailureCount      int           `json:"failure_count"`
	SuccessCount      int           `json:"success_count"`
	TripCount         int64         `json:"trip_count"`
	LastStateChange   time.Time     `json:"last_state_change"`
	TimeSinceLastFail time.Duration `json:"time_since_last_fail"`
}
