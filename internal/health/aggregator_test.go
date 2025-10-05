package health

import (
	"context"
	"testing"
	"time"
)

// mockChecker 模拟检查器
type mockChecker struct {
	name   string
	status Status
}

func (m *mockChecker) Name() string {
	return m.name
}

func (m *mockChecker) Check(ctx context.Context) CheckResult {
	return CheckResult{
		Status:  m.status,
		Message: "mock",
		Latency: time.Millisecond,
	}
}

func TestAggregator(t *testing.T) {
	t.Run("全部健康", func(t *testing.T) {
		agg := NewAggregator(
			&mockChecker{"db", StatusHealthy},
			&mockChecker{"tcp", StatusHealthy},
		)

		status := agg.OverallStatus(context.Background())
		if status != StatusHealthy {
			t.Errorf("期望StatusHealthy，实际: %v", status)
		}

		if !agg.Ready(context.Background()) {
			t.Error("全部健康时应该Ready")
		}
	})

	t.Run("部分降级", func(t *testing.T) {
		agg := NewAggregator(
			&mockChecker{"db", StatusHealthy},
			&mockChecker{"tcp", StatusDegraded},
		)

		status := agg.OverallStatus(context.Background())
		if status != StatusDegraded {
			t.Errorf("期望StatusDegraded，实际: %v", status)
		}

		// 降级状态仍然Ready
		if !agg.Ready(context.Background()) {
			t.Error("降级状态应该仍然Ready")
		}
	})

	t.Run("部分不健康", func(t *testing.T) {
		agg := NewAggregator(
			&mockChecker{"db", StatusHealthy},
			&mockChecker{"tcp", StatusUnhealthy},
		)

		status := agg.OverallStatus(context.Background())
		if status != StatusUnhealthy {
			t.Errorf("期望StatusUnhealthy，实际: %v", status)
		}

		// 不健康状态不Ready
		if agg.Ready(context.Background()) {
			t.Error("不健康状态不应该Ready")
		}
	})

	t.Run("CheckAll并发执行", func(t *testing.T) {
		agg := NewAggregator(
			&mockChecker{"check1", StatusHealthy},
			&mockChecker{"check2", StatusHealthy},
			&mockChecker{"check3", StatusHealthy},
		)

		results := agg.CheckAll(context.Background())
		if len(results) != 3 {
			t.Errorf("期望3个结果，实际: %d", len(results))
		}

		for name, result := range results {
			if result.Status != StatusHealthy {
				t.Errorf("%s: 期望StatusHealthy，实际: %v", name, result.Status)
			}
		}
	})

	t.Run("动态添加检查器", func(t *testing.T) {
		agg := NewAggregator(
			&mockChecker{"initial", StatusHealthy},
		)

		agg.AddChecker(&mockChecker{"added", StatusHealthy})

		results := agg.CheckAll(context.Background())
		if len(results) != 2 {
			t.Errorf("期望2个结果，实际: %d", len(results))
		}
	})

	t.Run("Alive始终返回true", func(t *testing.T) {
		agg := NewAggregator()

		if !agg.Alive() {
			t.Error("Alive应该始终返回true")
		}
	})
}
