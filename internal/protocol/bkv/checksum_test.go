package bkv

import (
	"testing"
)

func TestCalculateChecksum(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected byte
	}{
		{
			name:     "空数据",
			data:     []byte{},
			expected: 0x00,
		},
		{
			name:     "单字节",
			data:     []byte{0xAA},
			expected: 0xAA,
		},
		{
			name:     "两个相同字节",
			data:     []byte{0xAA, 0xAA},
			expected: 0x54, // 累加结果 (0xAA + 0xAA = 0x154, byte溢出 = 0x54)
		},
		{
			name:     "多字节",
			data:     []byte{0x10, 0x07, 0x00, 0x00, 0x00, 0x01},
			expected: byte(0x10 + 0x07 + 0x00 + 0x00 + 0x00 + 0x01),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateChecksum(tt.data)
			if result != tt.expected {
				t.Errorf("CalculateChecksum() = 0x%02X, expected 0x%02X", result, tt.expected)
			}
		})
	}
}

func TestVerifyChecksum(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{
			name:    "空数据",
			data:    []byte{},
			wantErr: true,
		},
		{
			name:    "正确的校验和",
			data:    []byte{0x10, 0x07, 0x00, 0x00, 0x00, 0x01, 0x18}, // 最后一字节是校验和 (0x10+0x07+0+0+0+0x01=0x18)
			wantErr: false,
		},
		{
			name:    "错误的校验和",
			data:    []byte{0x10, 0x07, 0x00, 0x00, 0x00, 0x01, 0xFF}, // 错误的校验和
			wantErr: true,
		},
		{
			name:    "单字节（仅校验和）",
			data:    []byte{0x00},
			wantErr: false, // 空数据的校验和是0
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := VerifyChecksum(tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("VerifyChecksum() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && err != ErrChecksumMismatch && !tt.wantErr {
				t.Errorf("VerifyChecksum() unexpected error: %v", err)
			}
		})
	}
}

func TestBuildChecksummedData(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected []byte
	}{
		{
			name:     "空数据",
			data:     []byte{},
			expected: []byte{0x00},
		},
		{
			name:     "心跳命令",
			data:     []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x01},
			expected: []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x01},
		},
		{
			name:     "启动充电命令",
			data:     []byte{0x10, 0x07, 0x00, 0x00, 0x00, 0x01, 0x01},
			expected: []byte{0x10, 0x07, 0x00, 0x00, 0x00, 0x01, 0x01, 0x19}, // 0x10+0x07+0+0+0+0x01+0x01=0x19
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildChecksummedData(tt.data)
			if len(result) != len(tt.expected) {
				t.Fatalf("BuildChecksummedData() length = %d, expected %d", len(result), len(tt.expected))
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("BuildChecksummedData()[%d] = 0x%02X, expected 0x%02X", i, result[i], tt.expected[i])
				}
			}
		})
	}
}

func TestChecksumRoundTrip(t *testing.T) {
	// 测试构建和验证的往返流程
	testData := [][]byte{
		{0x10, 0x07},
		{0x00, 0x00, 0x00, 0x00, 0x00, 0x01},
		{0x10, 0x04, 0x00, 0x00, 0x00, 0x02, 0x01, 0x00},
	}

	for i, data := range testData {
		checksummed := BuildChecksummedData(data)
		err := VerifyChecksum(checksummed)
		if err != nil {
			t.Errorf("Test %d: round-trip failed: %v", i, err)
		}
	}
}

func TestParserWithChecksum(t *testing.T) {
	// 测试解析器集成校验和验证
	tests := []struct {
		name    string
		frame   []byte
		wantErr bool
		errType error
	}{
		{
			name: "正确的心跳帧",
			frame: []byte{
				0xFC, 0xFE, // magic
				0x00, 0x11, // len=17
				0x00, 0x00, // cmd=0x0000
				0x00, 0x00, 0x00, 0x01, // msgID=1
				0x00,                                     // direction
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // gatewayID
				0x12,       // checksum (0x00+0x11+0x00+0x00+0x00+0x00+0x00+0x01+0x00+0+0+0+0+0+0+0=0x12)
				0xFC, 0xEE, // tail
			},
			wantErr: false,
		},
		{
			name: "错误的校验和",
			frame: []byte{
				0xFC, 0xFE,
				0x00, 0x11,
				0x00, 0x00,
				0x00, 0x00, 0x00, 0x01,
				0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0xFF, // 错误的checksum
				0xFC, 0xEE,
			},
			wantErr: true,
			errType: ErrChecksumMismatch,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.frame)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && tt.errType != nil && err != tt.errType {
				t.Errorf("Parse() error type = %v, expected %v", err, tt.errType)
			}
		})
	}
}
