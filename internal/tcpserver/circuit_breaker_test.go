package tcpserver

import (
	"errors"
	"testing"
	"time"
)

func TestCircuitBreaker(t *testing.T) {
	t.Run("熔断器状态转换", func(t *testing.T) {
		breaker := NewCircuitBreaker(3, 100*time.Millisecond)

		// 初始状态应该是Closed
		if breaker.State() != StateClosed {
			t.Fatalf("初始状态应该是Closed，实际: %v", breaker.State())
		}

		// 连续3次失败，应该触发熔断
		testErr := errors.New("test error")
		for i := 0; i < 3; i++ {
			_ = breaker.Call(func() error { return testErr })
		}

		// 应该进入Open状态
		if breaker.State() != StateOpen {
			t.Fatalf("3次失败后应该是Open状态，实际: %v", breaker.State())
		}

		// Open状态下，调用应该立即返回错误
		err := breaker.Call(func() error { return nil })
		if err != ErrCircuitOpen {
			t.Fatalf("Open状态应该返回ErrCircuitOpen，实际: %v", err)
		}

		// 等待超时后，应该进入HalfOpen状态
		time.Sleep(150 * time.Millisecond)

		// 第一次调用成功，应该进入HalfOpen
		err = breaker.Call(func() error { return nil })
		if err != nil {
			t.Fatalf("超时后第一次调用应该成功: %v", err)
		}

		// 应该是HalfOpen状态
		if breaker.State() != StateHalfOpen {
			t.Fatalf("应该进入HalfOpen状态，实际: %v", breaker.State())
		}

		// 继续成功调用，应该恢复到Closed
		for i := 0; i < 3; i++ {
			_ = breaker.Call(func() error { return nil })
		}

		if breaker.State() != StateClosed {
			t.Fatalf("成功后应该恢复到Closed状态，实际: %v", breaker.State())
		}
	})

	t.Run("半开状态失败立即熔断", func(t *testing.T) {
		breaker := NewCircuitBreaker(2, 100*time.Millisecond)

		// 触发熔断
		testErr := errors.New("test error")
		_ = breaker.Call(func() error { return testErr })
		_ = breaker.Call(func() error { return testErr })

		if breaker.State() != StateOpen {
			t.Fatal("应该进入Open状态")
		}

		// 等待超时，进入HalfOpen
		time.Sleep(150 * time.Millisecond)
		_ = breaker.Call(func() error { return nil }) // 第一次成功进入HalfOpen

		// HalfOpen状态失败，应该立即熔断
		_ = breaker.Call(func() error { return testErr })

		if breaker.State() != StateOpen {
			t.Fatalf("HalfOpen失败应该立即回到Open，实际: %v", breaker.State())
		}
	})

	t.Run("统计功能", func(t *testing.T) {
		breaker := NewCircuitBreaker(5, 1*time.Second)

		// 3次失败
		testErr := errors.New("test error")
		for i := 0; i < 3; i++ {
			_ = breaker.Call(func() error { return testErr })
		}

		// 2次成功
		for i := 0; i < 2; i++ {
			_ = breaker.Call(func() error { return nil })
		}

		stats := breaker.Stats()
		if stats.FailureCount != 3 {
			t.Errorf("期望3次失败，实际: %d", stats.FailureCount)
		}
		if stats.SuccessCount != 2 {
			t.Errorf("期望2次成功，实际: %d", stats.SuccessCount)
		}
		if stats.State != "closed" {
			t.Errorf("期望closed状态，实际: %s", stats.State)
		}
	})

	t.Run("状态变化回调", func(t *testing.T) {
		ch := make(chan struct {
			from State
			to   State
		}, 2)
		breaker := NewCircuitBreaker(2, 100*time.Millisecond)

		breaker.SetStateChangeCallback(func(from, to State) {
			ch <- struct {
				from State
				to   State
			}{from: from, to: to}
		})

		// 触发熔断
		testErr := errors.New("test error")
		_ = breaker.Call(func() error { return testErr })
		_ = breaker.Call(func() error { return testErr })

		// 等待回调（带超时）
		select {
		case evt := <-ch:
			if evt.from != StateClosed || evt.to != StateOpen {
				t.Errorf("状态转换回调错误，from: %v, to: %v", evt.from, evt.to)
			}
		case <-time.After(50 * time.Millisecond):
			t.Fatalf("状态变化回调未触发")
		}
	})
}
