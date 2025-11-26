package coremodel

import (
	"testing"
)

func TestPortStatusCode_CanCharge(t *testing.T) {
	tests := []struct {
		status    PortStatusCode
		canCharge bool
	}{
		{StatusCodeOffline, false},
		{StatusCodeIdle, true}, // 只有 idle 可以充电
		{StatusCodeCharging, false},
		{StatusCodeFault, false},
	}

	for _, tt := range tests {
		t.Run(tt.status.String(), func(t *testing.T) {
			if got := tt.status.CanCharge(); got != tt.canCharge {
				t.Errorf("PortStatusCode(%d).CanCharge() = %v, want %v", tt.status, got, tt.canCharge)
			}
		})
	}
}

func TestPortStatusCode_ToInfo(t *testing.T) {
	tests := []struct {
		status      PortStatusCode
		code        int
		name        string
		canCharge   bool
		displayText string
	}{
		{StatusCodeOffline, 0, "offline", false, "设备离线"},
		{StatusCodeIdle, 1, "idle", true, "空闲可用"},
		{StatusCodeCharging, 2, "charging", false, "使用中"},
		{StatusCodeFault, 3, "fault", false, "故障"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := tt.status.ToInfo()
			if info.Code != tt.code {
				t.Errorf("ToInfo().Code = %d, want %d", info.Code, tt.code)
			}
			if info.Name != tt.name {
				t.Errorf("ToInfo().Name = %s, want %s", info.Name, tt.name)
			}
			if info.CanCharge != tt.canCharge {
				t.Errorf("ToInfo().CanCharge = %v, want %v", info.CanCharge, tt.canCharge)
			}
			if info.DisplayText != tt.displayText {
				t.Errorf("ToInfo().DisplayText = %s, want %s", info.DisplayText, tt.displayText)
			}
		})
	}
}

func TestRawPortStatus_ToStatusCode(t *testing.T) {
	tests := []struct {
		name   string
		raw    RawPortStatus
		expect PortStatusCode
	}{
		// 离线状态
		{"offline_0x00", 0x00, StatusCodeOffline},
		{"offline_0x20", 0x20, StatusCodeOffline}, // 充电位置位但不在线

		// 空闲状态
		{"idle_0x80", 0x80, StatusCodeIdle}, // 仅在线
		{"idle_0x90", 0x90, StatusCodeIdle}, // 在线+空载（空载不是充电，是空闲）

		// 充电中
		{"charging_0xA0", 0xA0, StatusCodeCharging}, // 在线+充电
		{"charging_0xB0", 0xB0, StatusCodeCharging}, // 在线+充电+空载

		// 故障状态
		{"fault_meter_0xC0", 0xC0, StatusCodeFault},       // 在线+电表故障
		{"fault_overtemp_0x88", 0x88, StatusCodeFault},    // 在线+过温
		{"fault_overcurrent_0x84", 0x84, StatusCodeFault}, // 在线+过流
		{"fault_overpower_0x82", 0x82, StatusCodeFault},   // 在线+过功率

		// 常见设备值
		// 0x98 = 10011000 = 在线 + 空载 + 过温(bit3) → 应该是故障
		{"device_0x98", 0x98, StatusCodeFault},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.raw.ToStatusCode()
			if got != tt.expect {
				t.Errorf("RawPortStatus(0x%02X).ToStatusCode() = %d (%s), want %d (%s)",
					tt.raw, got, got.String(), tt.expect, tt.expect.String())
			}
		})
	}
}

func TestRawPortStatus_Methods(t *testing.T) {
	t.Run("IsOnline", func(t *testing.T) {
		if !RawPortStatus(0x80).IsOnline() {
			t.Error("0x80 should be online")
		}
		if RawPortStatus(0x00).IsOnline() {
			t.Error("0x00 should not be online")
		}
	})

	t.Run("IsCharging", func(t *testing.T) {
		if !RawPortStatus(0xA0).IsCharging() {
			t.Error("0xA0 should be charging")
		}
		if RawPortStatus(0x80).IsCharging() {
			t.Error("0x80 should not be charging")
		}
	})

	t.Run("IsNoLoad", func(t *testing.T) {
		if !RawPortStatus(0x90).IsNoLoad() {
			t.Error("0x90 should be no load")
		}
		if RawPortStatus(0x80).IsNoLoad() {
			t.Error("0x80 should not be no load")
		}
	})

	t.Run("HasFault", func(t *testing.T) {
		if !RawPortStatus(0xC0).HasFault() {
			t.Error("0xC0 (meter fault) should have fault")
		}
		if !RawPortStatus(0x88).HasFault() {
			t.Error("0x88 (over temp) should have fault")
		}
		if RawPortStatus(0x80).HasFault() {
			t.Error("0x80 should not have fault")
		}
	})
}

func TestAllPortStatusInfo(t *testing.T) {
	infos := AllPortStatusInfo()
	if len(infos) != 4 {
		t.Errorf("AllPortStatusInfo() returned %d items, want 4", len(infos))
	}

	// 验证顺序
	expectedCodes := []int{0, 1, 2, 3}
	for i, info := range infos {
		if info.Code != expectedCodes[i] {
			t.Errorf("AllPortStatusInfo()[%d].Code = %d, want %d", i, info.Code, expectedCodes[i])
		}
	}

	// 验证只有 idle 可以充电
	for _, info := range infos {
		if info.Code == 1 && !info.CanCharge {
			t.Error("idle status should have CanCharge=true")
		}
		if info.Code != 1 && info.CanCharge {
			t.Errorf("status code %d should have CanCharge=false", info.Code)
		}
	}
}

func TestGetStatusDefinitions(t *testing.T) {
	defs := GetStatusDefinitions()

	if len(defs.PortStatus) != 4 {
		t.Errorf("GetStatusDefinitions().PortStatus has %d items, want 4", len(defs.PortStatus))
	}

	if len(defs.EndReason) != 8 {
		t.Errorf("GetStatusDefinitions().EndReason has %d items, want 8", len(defs.EndReason))
	}
}

func TestDeriveEndReasonFromStatus(t *testing.T) {
	tests := []struct {
		name   string
		status RawPortStatus
		expect EndReasonCode
	}{
		{"offline", 0x00, ReasonCodePowerOff},
		{"no_load", 0x90, ReasonCodeNoLoad},
		{"over_temp", 0x88, ReasonCodeOverTemp},
		{"over_current", 0x84, ReasonCodeOverCurrent},
		{"over_power", 0x82, ReasonCodeOverPower},
		{"meter_fault", 0xC0, ReasonCodeFault},
		{"normal", 0x80, ReasonCodeNormal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DeriveEndReasonFromStatus(tt.status)
			if got != tt.expect {
				t.Errorf("DeriveEndReasonFromStatus(0x%02X) = %d (%s), want %d (%s)",
					tt.status, got, got.String(), tt.expect, tt.expect.String())
			}
		})
	}
}

func TestRawStatusToCode(t *testing.T) {
	// 测试便捷函数
	if got := RawStatusToCode(0x80); got != StatusCodeIdle {
		t.Errorf("RawStatusToCode(0x80) = %d, want %d", got, StatusCodeIdle)
	}
	if got := RawStatusToCode(0xA0); got != StatusCodeCharging {
		t.Errorf("RawStatusToCode(0xA0) = %d, want %d", got, StatusCodeCharging)
	}
}
