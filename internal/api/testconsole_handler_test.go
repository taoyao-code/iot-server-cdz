package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestDecodeConsolePortStatus 测试端口状态转换函数
func TestDecodeConsolePortStatus(t *testing.T) {
	tests := []struct {
		name         string
		raw          int
		expectedEnum int
		expectedMeta PortStatusMeta
		description  string
	}{
		{
			name:         "充电中状态 (bit7=1)",
			raw:          0x80,
			expectedEnum: 1,
			expectedMeta: PortStatusMeta{
				RawStatus:  0x80,
				IsOnline:   false,
				IsCharging: true,
				IsIdle:     false,
				HasFault:   true,
			},
			description: "设备充电中但不在线（异常状态）",
		},
		{
			name:         "空闲状态 (bit0=1, bit3=1)",
			raw:          0x09,
			expectedEnum: 0,
			expectedMeta: PortStatusMeta{
				RawStatus:  0x09,
				IsOnline:   true,
				IsCharging: false,
				IsIdle:     true,
				HasFault:   false,
			},
			description: "设备在线且空闲",
		},
		{
			name:         "故障状态 (bit0=0)",
			raw:          0x00,
			expectedEnum: 2,
			expectedMeta: PortStatusMeta{
				RawStatus:  0x00,
				IsOnline:   false,
				IsCharging: false,
				IsIdle:     false,
				HasFault:   true,
			},
			description: "设备离线/故障",
		},
		{
			name:         "充电中优先级测试 (bit7=1, bit3=1)",
			raw:          0x88,
			expectedEnum: 1,
			expectedMeta: PortStatusMeta{
				RawStatus:  0x88,
				IsOnline:   false,
				IsCharging: true,
				IsIdle:     true,
				HasFault:   true,
			},
			description: "同时满足充电和空闲，优先返回充电中",
		},
		{
			name:         "正常充电状态 (bit0=1, bit7=1)",
			raw:          0x81,
			expectedEnum: 1,
			expectedMeta: PortStatusMeta{
				RawStatus:  0x81,
				IsOnline:   true,
				IsCharging: true,
				IsIdle:     false,
				HasFault:   false,
			},
			description: "设备在线且充电中（正常充电）",
		},
		{
			name:         "仅在线状态 (bit0=1)",
			raw:          0x01,
			expectedEnum: 0,
			expectedMeta: PortStatusMeta{
				RawStatus:  0x01,
				IsOnline:   true,
				IsCharging: false,
				IsIdle:     false,
				HasFault:   false,
			},
			description: "设备在线但未充电（默认为空闲）",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enumResult, metaResult := decodeConsolePortStatus(tt.raw)

			// 验证业务枚举值
			assert.Equal(t, tt.expectedEnum, enumResult,
				"业务枚举值不匹配 (%s)", tt.description)

			// 验证元数据字段
			assert.Equal(t, tt.expectedMeta.RawStatus, metaResult.RawStatus,
				"RawStatus 不匹配")
			assert.Equal(t, tt.expectedMeta.IsOnline, metaResult.IsOnline,
				"IsOnline 不匹配")
			assert.Equal(t, tt.expectedMeta.IsCharging, metaResult.IsCharging,
				"IsCharging 不匹配")
			assert.Equal(t, tt.expectedMeta.IsIdle, metaResult.IsIdle,
				"IsIdle 不匹配")
			assert.Equal(t, tt.expectedMeta.HasFault, metaResult.HasFault,
				"HasFault 不匹配")
		})
	}
}

// TestDecodeConsolePortStatus_PriorityRules 测试状态优先级规则
func TestDecodeConsolePortStatus_PriorityRules(t *testing.T) {
	t.Run("充电中优先于离线", func(t *testing.T) {
		// bit7=1 (充电), bit0=0 (离线) → 应返回充电中(1)而非故障(2)
		enum, meta := decodeConsolePortStatus(0x80)
		assert.Equal(t, 1, enum, "充电中应优先于离线判定")
		assert.True(t, meta.IsCharging, "应标记为充电中")
		assert.False(t, meta.IsOnline, "应标记为离线")
		assert.True(t, meta.HasFault, "离线应标记为故障")
	})

	t.Run("充电中优先于空闲", func(t *testing.T) {
		// bit7=1 (充电), bit3=1 (空闲), bit0=1 (在线) → 应返回充电中(1)
		enum, meta := decodeConsolePortStatus(0x89)
		assert.Equal(t, 1, enum, "充电中应优先于空闲判定")
		assert.True(t, meta.IsCharging, "应标记为充电中")
		assert.True(t, meta.IsIdle, "空闲位应保留")
		assert.True(t, meta.IsOnline, "应标记为在线")
	})

	t.Run("离线优先于空闲", func(t *testing.T) {
		// bit3=1 (空闲), bit0=0 (离线) → 应返回故障(2)而非空闲(0)
		enum, meta := decodeConsolePortStatus(0x08)
		assert.Equal(t, 2, enum, "离线应优先于空闲判定")
		assert.False(t, meta.IsOnline, "应标记为离线")
		assert.True(t, meta.HasFault, "应标记为故障")
	})
}

// TestDecodeConsolePortStatus_EdgeCases 测试边界情况
func TestDecodeConsolePortStatus_EdgeCases(t *testing.T) {
	t.Run("全0状态", func(t *testing.T) {
		enum, meta := decodeConsolePortStatus(0x00)
		assert.Equal(t, 2, enum, "全0应解析为故障")
		assert.True(t, meta.HasFault)
	})

	t.Run("全1状态", func(t *testing.T) {
		enum, meta := decodeConsolePortStatus(0xFF)
		assert.Equal(t, 1, enum, "全1应优先解析为充电中")
		assert.True(t, meta.IsCharging)
		assert.True(t, meta.IsOnline)
		assert.True(t, meta.IsIdle)
	})

	t.Run("高字节忽略", func(t *testing.T) {
		// 只取低8位，高字节应被忽略
		enum1, _ := decodeConsolePortStatus(0x0109)
		enum2, _ := decodeConsolePortStatus(0xFF09)
		assert.Equal(t, enum1, enum2, "高字节应被忽略")
	})
}
