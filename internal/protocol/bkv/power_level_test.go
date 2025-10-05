package bkv

import (
	"testing"
)

// Week 8: 按功率分档充电协议测试

func TestEncodePowerLevelCommand(t *testing.T) {
	tests := []struct {
		name string
		cmd  *PowerLevelCommand
	}{
		{
			name: "Single Level",
			cmd: &PowerLevelCommand{
				PortNo:     1,
				LevelCount: 1,
				Levels: []PowerLevelV2{
					{PowerW: 1000, PriceCents: 50, Duration: 30},
				},
			},
		},
		{
			name: "Three Levels",
			cmd: &PowerLevelCommand{
				PortNo:     2,
				LevelCount: 3,
				Levels: []PowerLevelV2{
					{PowerW: 500, PriceCents: 40, Duration: 10},
					{PowerW: 1000, PriceCents: 50, Duration: 20},
					{PowerW: 1500, PriceCents: 60, Duration: 30},
				},
			},
		},
		{
			name: "Five Levels (Max)",
			cmd: &PowerLevelCommand{
				PortNo:     3,
				LevelCount: 5,
				Levels: []PowerLevelV2{
					{PowerW: 500, PriceCents: 40, Duration: 10},
					{PowerW: 1000, PriceCents: 50, Duration: 15},
					{PowerW: 1500, PriceCents: 60, Duration: 20},
					{PowerW: 2000, PriceCents: 70, Duration: 25},
					{PowerW: 2500, PriceCents: 80, Duration: 30},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := EncodePowerLevelCommand(tt.cmd)

			expectedLen := 2 + int(tt.cmd.LevelCount)*6
			if len(data) != expectedLen {
				t.Errorf("Expected %d bytes, got %d", expectedLen, len(data))
			}

			// 验证可以正确解析
			parsed, err := ParsePowerLevelCommand(data)
			if err != nil {
				t.Fatalf("Parse failed: %v", err)
			}

			if parsed.PortNo != tt.cmd.PortNo {
				t.Errorf("Expected port_no=%d, got %d", tt.cmd.PortNo, parsed.PortNo)
			}

			if parsed.LevelCount != tt.cmd.LevelCount {
				t.Errorf("Expected level_count=%d, got %d", tt.cmd.LevelCount, parsed.LevelCount)
			}

			if len(parsed.Levels) != int(tt.cmd.LevelCount) {
				t.Errorf("Expected %d levels, got %d", tt.cmd.LevelCount, len(parsed.Levels))
			}

			// 验证各档位数据
			for i := 0; i < int(tt.cmd.LevelCount); i++ {
				if parsed.Levels[i].PowerW != tt.cmd.Levels[i].PowerW {
					t.Errorf("Level %d: expected power=%dW, got %dW",
						i+1, tt.cmd.Levels[i].PowerW, parsed.Levels[i].PowerW)
				}
				if parsed.Levels[i].PriceCents != tt.cmd.Levels[i].PriceCents {
					t.Errorf("Level %d: expected price=%d分, got %d分",
						i+1, tt.cmd.Levels[i].PriceCents, parsed.Levels[i].PriceCents)
				}
				if parsed.Levels[i].Duration != tt.cmd.Levels[i].Duration {
					t.Errorf("Level %d: expected duration=%dmin, got %dmin",
						i+1, tt.cmd.Levels[i].Duration, parsed.Levels[i].Duration)
				}
			}

			t.Logf("✅ Encoded %d levels: %d bytes", tt.cmd.LevelCount, len(data))
		})
	}
}

func TestParsePowerLevelEndReport(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected *PowerLevelEndReport
	}{
		{
			name: "Simple End Report",
			data: []byte{
				0x01,                   // port_no = 1
				0x00, 0x1E,             // total_duration = 30分钟
				0x00, 0x00, 0x03, 0xE8, // total_energy = 1000 (10度)
				0x00, 0x00, 0x01, 0xF4, // total_amount = 500分 (5元)
				0x00,                   // end_reason = 0 (正常结束)
			},
			expected: &PowerLevelEndReport{
				PortNo:        1,
				TotalDuration: 30,
				TotalEnergy:   1000,
				TotalAmount:   500,
				EndReason:     0,
			},
		},
		{
			name: "End Report with Level Usage",
			data: []byte{
				0x02,                   // port_no = 2
				0x00, 0x3C,             // total_duration = 60分钟
				0x00, 0x00, 0x07, 0xD0, // total_energy = 2000 (20度)
				0x00, 0x00, 0x03, 0xE8, // total_amount = 1000分 (10元)
				0x00,                   // end_reason = 0 (正常结束)
				0x02,                   // level_count = 2
				// Level 1
				0x01,                   // level_no = 1
				0x00, 0x14,             // duration = 20min
				0x00, 0x00, 0x03, 0x20, // energy = 800 (8度)
				0x00, 0x00, 0x01, 0x90, // amount = 400分 (4元)
				// Level 2
				0x02,                   // level_no = 2
				0x00, 0x28,             // duration = 40min
				0x00, 0x00, 0x04, 0xB0, // energy = 1200 (12度)
				0x00, 0x00, 0x02, 0x58, // amount = 600分 (6元)
			},
			expected: &PowerLevelEndReport{
				PortNo:        2,
				TotalDuration: 60,
				TotalEnergy:   2000,
				TotalAmount:   1000,
				EndReason:     0,
				LevelUsage: []PowerLevelUsage{
					{LevelNo: 1, Duration: 20, Energy: 800, Amount: 400},
					{LevelNo: 2, Duration: 40, Energy: 1200, Amount: 600},
				},
			},
		},
		{
			name: "User Stopped",
			data: []byte{
				0x03,                   // port_no = 3
				0x00, 0x0A,             // total_duration = 10分钟
				0x00, 0x00, 0x00, 0xC8, // total_energy = 200 (2度)
				0x00, 0x00, 0x00, 0x64, // total_amount = 100分 (1元)
				0x01,                   // end_reason = 1 (用户停止)
			},
			expected: &PowerLevelEndReport{
				PortNo:        3,
				TotalDuration: 10,
				TotalEnergy:   200,
				TotalAmount:   100,
				EndReason:     1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report, err := ParsePowerLevelEndReport(tt.data)
			if err != nil {
				t.Fatalf("Parse failed: %v", err)
			}

			if report.PortNo != tt.expected.PortNo {
				t.Errorf("Expected port_no=%d, got %d", tt.expected.PortNo, report.PortNo)
			}

			if report.TotalDuration != tt.expected.TotalDuration {
				t.Errorf("Expected total_duration=%d, got %d", tt.expected.TotalDuration, report.TotalDuration)
			}

			if report.TotalEnergy != tt.expected.TotalEnergy {
				t.Errorf("Expected total_energy=%d, got %d", tt.expected.TotalEnergy, report.TotalEnergy)
			}

			if report.TotalAmount != tt.expected.TotalAmount {
				t.Errorf("Expected total_amount=%d, got %d", tt.expected.TotalAmount, report.TotalAmount)
			}

			if report.EndReason != tt.expected.EndReason {
				t.Errorf("Expected end_reason=%d, got %d", tt.expected.EndReason, report.EndReason)
			}

			if len(report.LevelUsage) != len(tt.expected.LevelUsage) {
				t.Errorf("Expected %d level usages, got %d", len(tt.expected.LevelUsage), len(report.LevelUsage))
			}

			for i, usage := range report.LevelUsage {
				expected := tt.expected.LevelUsage[i]
				if usage.LevelNo != expected.LevelNo {
					t.Errorf("Level %d: expected level_no=%d, got %d", i, expected.LevelNo, usage.LevelNo)
				}
				if usage.Duration != expected.Duration {
					t.Errorf("Level %d: expected duration=%d, got %d", i, expected.Duration, usage.Duration)
				}
				if usage.Energy != expected.Energy {
					t.Errorf("Level %d: expected energy=%d, got %d", i, expected.Energy, usage.Energy)
				}
				if usage.Amount != expected.Amount {
					t.Errorf("Level %d: expected amount=%d, got %d", i, expected.Amount, usage.Amount)
				}
			}

			t.Logf("✅ Parsed: port=%d, duration=%dmin, energy=%.2fkWh, amount=%.2f元",
				report.PortNo, report.TotalDuration,
				float64(report.TotalEnergy)/100, float64(report.TotalAmount)/100)
		})
	}
}

func TestValidatePowerLevels(t *testing.T) {
	tests := []struct {
		name    string
		levels  []PowerLevelV2
		wantErr bool
	}{
		{
			name: "Valid Single Level",
			levels: []PowerLevelV2{
				{PowerW: 1000, PriceCents: 50, Duration: 30},
			},
			wantErr: false,
		},
		{
			name: "Valid Multiple Levels",
			levels: []PowerLevelV2{
				{PowerW: 500, PriceCents: 40, Duration: 10},
				{PowerW: 1000, PriceCents: 50, Duration: 20},
				{PowerW: 1500, PriceCents: 60, Duration: 30},
			},
			wantErr: false,
		},
		{
			name:    "Empty Levels",
			levels:  []PowerLevelV2{},
			wantErr: true,
		},
		{
			name: "Too Many Levels",
			levels: []PowerLevelV2{
				{PowerW: 500, PriceCents: 40, Duration: 10},
				{PowerW: 1000, PriceCents: 50, Duration: 15},
				{PowerW: 1500, PriceCents: 60, Duration: 20},
				{PowerW: 2000, PriceCents: 70, Duration: 25},
				{PowerW: 2500, PriceCents: 80, Duration: 30},
				{PowerW: 3000, PriceCents: 90, Duration: 35}, // 第6档，超出限制
			},
			wantErr: true,
		},
		{
			name: "Zero Power",
			levels: []PowerLevelV2{
				{PowerW: 0, PriceCents: 50, Duration: 30},
			},
			wantErr: true,
		},
		{
			name: "Zero Price",
			levels: []PowerLevelV2{
				{PowerW: 1000, PriceCents: 0, Duration: 30},
			},
			wantErr: true,
		},
		{
			name: "Zero Duration",
			levels: []PowerLevelV2{
				{PowerW: 1000, PriceCents: 50, Duration: 0},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePowerLevels(tt.levels)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePowerLevels() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				t.Logf("Validation error (expected): %v", err)
			}
		})
	}
}

// TestPowerLevel_E2E 端到端集成测试
func TestPowerLevel_E2E(t *testing.T) {
	t.Run("Three Level Charging Flow", func(t *testing.T) {
		// 1. 创建3档充电命令
		cmd := &PowerLevelCommand{
			PortNo:     1,
			LevelCount: 3,
			Levels: []PowerLevelV2{
				{PowerW: 500, PriceCents: 40, Duration: 10},   // 0.5kW, 0.4元/度, 10分钟
				{PowerW: 1000, PriceCents: 50, Duration: 20},  // 1kW, 0.5元/度, 20分钟
				{PowerW: 1500, PriceCents: 60, Duration: 30},  // 1.5kW, 0.6元/度, 30分钟
			},
		}

		// 2. 编码命令
		cmdData := EncodePowerLevelCommand(cmd)
		t.Logf("Sent command: %d bytes, %d levels", len(cmdData), cmd.LevelCount)

		// 3. 验证编码
		if len(cmdData) != 2+3*6 {
			t.Errorf("Expected 20 bytes, got %d", len(cmdData))
		}

		// 4. 模拟充电完成上报
		endData := []byte{
			0x01,                   // port_no = 1
			0x00, 0x3C,             // total_duration = 60分钟
			0x00, 0x00, 0x0B, 0xB8, // total_energy = 3000 (30度)
			0x00, 0x00, 0x05, 0xDC, // total_amount = 1500分 (15元)
			0x00,                   // end_reason = 0 (正常结束)
			0x03,                   // level_count = 3
			// Level 1: 10分钟
			0x01, 0x00, 0x0A, 0x00, 0x00, 0x00, 0x52, 0x00, 0x00, 0x00, 0x28,
			// Level 2: 20分钟
			0x02, 0x00, 0x14, 0x00, 0x00, 0x01, 0x4A, 0x00, 0x00, 0x01, 0x04,
			// Level 3: 30分钟
			0x03, 0x00, 0x1E, 0x00, 0x00, 0x0A, 0x1C, 0x00, 0x00, 0x04, 0xB0,
		}

		report, err := ParsePowerLevelEndReport(endData)
		if err != nil {
			t.Fatalf("Parse end report failed: %v", err)
		}

		if report.PortNo != 1 {
			t.Errorf("Expected port_no=1, got %d", report.PortNo)
		}

		if report.TotalDuration != 60 {
			t.Errorf("Expected 60min, got %dmin", report.TotalDuration)
		}

		if len(report.LevelUsage) != 3 {
			t.Errorf("Expected 3 level usages, got %d", len(report.LevelUsage))
		}

		// 5. 生成确认回复
		reply := EncodePowerLevelEndReply(1, 0)
		if len(reply) != 2 {
			t.Errorf("Expected 2 bytes reply, got %d", len(reply))
		}

		t.Logf("✅ E2E Test passed: 3-level charging completed successfully")
		t.Logf("   Total: %.2fkWh, %.2f元, %d分钟",
			float64(report.TotalEnergy)/100,
			float64(report.TotalAmount)/100,
			report.TotalDuration)
	})
}

func TestGetPowerLevelEndReasonDescription(t *testing.T) {
	tests := []struct {
		reason   uint8
		expected string
	}{
		{0, "正常结束"},
		{1, "用户停止"},
		{2, "设备故障"},
		{3, "超时停止"},
		{99, "未知原因(99)"},
	}

	for _, tt := range tests {
		desc := GetPowerLevelEndReasonDescription(tt.reason)
		if desc != tt.expected {
			t.Errorf("Reason %d: expected '%s', got '%s'", tt.reason, tt.expected, desc)
		}
	}
}

