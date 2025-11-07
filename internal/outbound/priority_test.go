package outbound

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestGetCommandPriority P1-6: 测试命令优先级分配
func TestGetCommandPriority(t *testing.T) {
	tests := []struct {
		name     string
		cmd      uint16
		expected int
	}{
		{
			name:     "停止充电=紧急优先级",
			cmd:      0x1010,
			expected: PriorityEmergency,
		},
		{
			name:     "启动充电=高优先级",
			cmd:      0x1011,
			expected: PriorityHigh,
		},
		{
			name:     "查询端口状态=高优先级",
			cmd:      0x1012,
			expected: PriorityHigh,
		},
		{
			name:     "参数设置=普通优先级",
			cmd:      0x1003,
			expected: PriorityNormal,
		},
		{
			name:     "OTA升级=低优先级",
			cmd:      0x1014,
			expected: PriorityLow,
		},
		{
			name:     "心跳ACK=普通优先级",
			cmd:      0x0000,
			expected: PriorityNormal,
		},
		{
			name:     "未知命令=普通优先级",
			cmd:      0x9999,
			expected: PriorityNormal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			priority := GetCommandPriority(tt.cmd)
			assert.Equal(t, tt.expected, priority,
				"命令 0x%04X 的优先级应该是 %d，实际是 %d",
				tt.cmd, tt.expected, priority)
		})
	}
}

// TestPriorityValues P1-6: 测试优先级数值定义
func TestPriorityValues(t *testing.T) {
	// 确保优先级数值是递增的（数值越小=优先级越高）
	assert.Less(t, PriorityEmergency, PriorityHigh, "紧急 < 高")
	assert.Less(t, PriorityHigh, PriorityNormal, "高 < 普通")
	assert.Less(t, PriorityNormal, PriorityLow, "普通 < 低")
	assert.Less(t, PriorityLow, PriorityBackground, "低 < 后台")
}
