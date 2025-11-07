package app

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestPortStatusSyncer_getExpectedPortStatus P1-4: 测试期望状态计算
func TestPortStatusSyncer_getExpectedPortStatus(t *testing.T) {
	syncer := &PortStatusSyncer{}

	tests := []struct {
		name         string
		orderStatus  int
		expectedPort int
	}{
		{
			name:         "charging订单期望端口charging",
			orderStatus:  2,
			expectedPort: 2,
		},
		{
			name:         "cancelling订单期望端口charging",
			orderStatus:  8,
			expectedPort: 2,
		},
		{
			name:         "stopping订单期望端口charging",
			orderStatus:  9,
			expectedPort: 2,
		},
		{
			name:         "interrupted订单期望端口charging",
			orderStatus:  10,
			expectedPort: 2,
		},
		{
			name:         "其他状态期望端口free",
			orderStatus:  3,
			expectedPort: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := syncer.getExpectedPortStatus(tt.orderStatus)
			assert.Equal(t, tt.expectedPort, result,
				"订单状态 %d 的期望端口状态应该是 %d，实际是 %d",
				tt.orderStatus, tt.expectedPort, result)
		})
	}
}

// TestPortStatusSyncer_shouldAutoFix P1-4: 测试自动修复判断
func TestPortStatusSyncer_shouldAutoFix(t *testing.T) {
	syncer := &PortStatusSyncer{}

	tests := []struct {
		name          string
		orderStatus   int
		portStatus    *int
		online        bool
		minutesSince  int
		shouldAutoFix bool
		description   string
	}{
		{
			name:          "设备离线超过5分钟且订单charging",
			orderStatus:   2,
			portStatus:    intPtr(2),
			online:        false,
			minutesSince:  6,
			shouldAutoFix: true,
			description:   "应该自动失败订单",
		},
		{
			name:          "端口free但订单charging且超过15分钟",
			orderStatus:   2,
			portStatus:    intPtr(0),
			online:        true,
			minutesSince:  16,
			shouldAutoFix: true,
			description:   "应该自动完成订单",
		},
		{
			name:          "设备离线但时间不足5分钟",
			orderStatus:   2,
			portStatus:    intPtr(2),
			online:        false,
			minutesSince:  4,
			shouldAutoFix: false,
			description:   "等待恢复，不自动修复",
		},
		{
			name:          "端口free但时间不足15分钟",
			orderStatus:   2,
			portStatus:    intPtr(0),
			online:        true,
			minutesSince:  10,
			shouldAutoFix: false,
			description:   "等待更新，不自动修复",
		},
		{
			name:          "设备在线且端口状态正常",
			orderStatus:   2,
			portStatus:    intPtr(2),
			online:        true,
			minutesSince:  10,
			shouldAutoFix: false,
			description:   "状态正常，不需要修复",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			timeSinceUpdate := time.Duration(tt.minutesSince) * time.Minute
			result := syncer.shouldAutoFix(tt.orderStatus, tt.portStatus, tt.online, timeSinceUpdate)
			assert.Equal(t, tt.shouldAutoFix, result, tt.description)
		})
	}
}

// intPtr 辅助函数：返回int指针
func intPtr(i int) *int {
	return &i
}
