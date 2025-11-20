package bkv

import (
	"encoding/hex"
	"testing"
)

// TestParseBKVControlCommand_ByTime 测试按时充电控制指令解析
func TestParseBKVControlCommand_ByTime(t *testing.T) {
	// 基于协议文档的按时充电指令: 插座02, A孔, 开启, 按时, 240分钟, 0Wh
	// 02 00 01 01 00f0 0000
	data, _ := hex.DecodeString("0200010100f00000")

	cmd, err := ParseBKVControlCommand(data)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if cmd.SocketNo != 2 {
		t.Errorf("expected socket 2, got %d", cmd.SocketNo)
	}
	if cmd.Port != PortA {
		t.Errorf("expected port A, got %d", cmd.Port)
	}
	if cmd.Switch != SwitchOn {
		t.Errorf("expected switch on, got %d", cmd.Switch)
	}
	if cmd.Mode != ChargingModeByTime {
		t.Errorf("expected by time mode, got %d", cmd.Mode)
	}
	if cmd.Duration != 240 {
		t.Errorf("expected duration 240, got %d", cmd.Duration)
	}
}

// TestParseBKVControlCommand_ByPowerLevel 测试按功率充电控制指令解析
func TestParseBKVControlCommand_ByPowerLevel(t *testing.T) {
	// 基于协议文档的按功率充电指令（简化版本）
	// 01 00 01 03 0000 0000 0064 05 [5个挡位信息]
	// 挡位1: 07d0(2000W) 0019(25分) 003c(60分钟)
	hexStr := "01000103000000000064050" +
		"7d00019003c" + // 挡位1: 2000W, 25分, 60分钟
		"0fa00032003c" + // 挡位2: 4000W, 50分, 60分钟
		"17700064003c" + // 挡位3: 6000W, 100分, 60分钟
		"1f400096003c" + // 挡位4: 8000W, 150分, 60分钟
		"4e2001f40078" // 挡位5: 20000W, 500分, 120分钟

	data, _ := hex.DecodeString(hexStr)

	cmd, err := ParseBKVControlCommand(data)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if cmd.Mode != ChargingModeByLevel {
		t.Errorf("expected by power level mode, got %d", cmd.Mode)
	}
	if cmd.PaymentAmount != 100 {
		t.Errorf("expected payment 100, got %d", cmd.PaymentAmount)
	}
	if cmd.LevelCount != 5 {
		t.Errorf("expected 5 levels, got %d", cmd.LevelCount)
	}
	if len(cmd.PowerLevels) != 5 {
		t.Errorf("expected 5 power levels, got %d", len(cmd.PowerLevels))
	}

	// 检查第一个挡位
	if cmd.PowerLevels[0].Power != 2000 {
		t.Errorf("expected power 2000W, got %d", cmd.PowerLevels[0].Power)
	}
	if cmd.PowerLevels[0].Price != 25 {
		t.Errorf("expected price 25, got %d", cmd.PowerLevels[0].Price)
	}
}

// TestParseBKVChargingEnd_Normal 测试普通充电结束解析
func TestParseBKVChargingEnd_Normal(t *testing.T) {
	// 基于协议文档的充电结束上报
	// 插座02, 版本5036, 温度30, RSSI20, A孔, 状态98, 业务号0068, 功率0000, 电流0001, 用电量0050, 时间002d
	hexStr := "02503630200098006800000001005000" + "2d"

	data, _ := hex.DecodeString(hexStr) // 裸数据（从插座号开始）

	end, err := ParseBKVChargingEnd(data)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if end.SocketNo != 2 {
		t.Errorf("expected socket 2, got %d", end.SocketNo)
	}
	if end.SoftwareVer != 0x5036 {
		t.Errorf("expected version 5036, got %x", end.SoftwareVer)
	}
	if end.Temperature != 48 { // 0x30 = 48
		t.Errorf("expected temperature 48, got %d", end.Temperature)
	}
	if end.BusinessNo != 0x0068 {
		t.Errorf("expected business no 0068, got %04x", end.BusinessNo)
	}
	if end.EnergyUsed != 80 { // 0x0050 = 80
		t.Errorf("expected energy 80, got %d", end.EnergyUsed)
	}
	if end.ChargingTime != 45 { // 0x002d = 45
		t.Errorf("expected time 45, got %d", end.ChargingTime)
	}

	// 检查从状态位推导的结束原因
	// 状态98 = 10011000，bit4=1表示空载
	if end.EndReason != ReasonNoLoad {
		t.Errorf("expected no load reason, got %d", end.EndReason)
	}
}

// TestParseBKVChargingEnd_FromCtlData 测试从0x0015完整data段解析结束上报
func TestParseBKVChargingEnd_FromCtlData(t *testing.T) {
	// 对应 docs 2.2.9 的完整上行帧 data 段：
	// 0011(帧长) 02(子命令) 02(插座号) 5036(版本) 30(温度) 20(RSSI)
	// 00(插孔) 98(状态) 0068(业务号) 0000(功率) 0001(电流) 0050(电量) 002d(时间)
	hexStr := "001102025036302000980068000000010050002d"
	data, _ := hex.DecodeString(hexStr)

	end, err := ParseBKVChargingEnd(data)
	if err != nil {
		t.Fatalf("parse from ctl data error: %v", err)
	}

	if end.SocketNo != 2 || end.Port != 0 {
		t.Fatalf("unexpected socket/port: socket=%d port=%d", end.SocketNo, end.Port)
	}
	if end.BusinessNo != 0x0068 {
		t.Fatalf("unexpected business no: %04x", end.BusinessNo)
	}
	if end.EnergyUsed != 80 || end.ChargingTime != 45 {
		t.Fatalf("unexpected energy/time: kwh01=%d time=%d", end.EnergyUsed, end.ChargingTime)
	}
}

// TestDeriveEndReasonFromStatus 测试从状态位推导结束原因
func TestDeriveEndReasonFromStatus(t *testing.T) {
	testCases := []struct {
		status   uint8
		expected ChargingEndReason
		desc     string
	}{
		{0x98, ReasonNoLoad, "空载结束 (bit4=1)"},
		{0x88, ReasonOverCurrent, "电流异常 (bit2=0)"},
		{0x80, ReasonOverTemp, "温度异常 (bit3=0)"},
		{0x84, ReasonOverTemp, "温度异常 (bit3=0)"},
		{0x18, ReasonPowerOff, "离线状态 (bit7=0)"},
		{0xEE, ReasonNormal, "正常状态 (无空载、无异常)"},
	}

	for _, tc := range testCases {
		result := deriveEndReasonFromStatus(tc.status)
		if result != tc.expected {
			t.Errorf("%s: expected reason %d, got %d", tc.desc, tc.expected, result)
		}
	}
}

// TestGetControlCommandType 测试控制指令类型识别
func TestGetControlCommandType(t *testing.T) {
	testCases := []struct {
		data     []byte
		expected string
		desc     string
	}{
		{[]byte{1, 0, 1, 1}, "charging_by_time", "按时充电"},
		{[]byte{1, 0, 1, 0}, "charging_by_energy", "按电量充电"},
		{[]byte{1, 0, 1, 3}, "charging_by_power_level", "按功率充电"},
		{[]byte{1, 0, 1, 99}, "unknown", "未知模式"},
		{[]byte{1, 0}, "unknown", "数据不足"},
	}

	for _, tc := range testCases {
		result := GetControlCommandType(tc.data)
		if result != tc.expected {
			t.Errorf("%s: expected %s, got %s", tc.desc, tc.expected, result)
		}
	}
}

// TestBKVControlCommand_Integration 测试与协议文档示例的集成
func TestBKVControlCommand_Integration(t *testing.T) {
	// 基于协议文档中的实际示例，简化为核心控制字段
	// 协议格式: 插座号 插孔号 开关 模式 时长(2字节) 电量(2字节)
	// 插座02, A孔, 开启, 按时, 240分钟(0x00f0), 0Wh
	protocolExample := "0200010100f00000"
	data, _ := hex.DecodeString(protocolExample)

	cmd, err := ParseBKVControlCommand(data)
	if err != nil {
		t.Fatalf("parse protocol example error: %v", err)
	}

	// 验证解析结果与协议文档描述一致
	if cmd.SocketNo != 2 {
		t.Errorf("expected socket 2, got %d", cmd.SocketNo)
	}
	if cmd.Port != PortA {
		t.Errorf("expected port A, got %d", cmd.Port)
	}
	if cmd.Switch != SwitchOn {
		t.Errorf("expected switch on, got %d", cmd.Switch)
	}
	if cmd.Mode != ChargingModeByTime {
		t.Errorf("expected by time mode, got %d", cmd.Mode)
	}
	// 240分钟 = 0x00f0
	if cmd.Duration != 240 {
		t.Errorf("expected duration 240 minutes, got %d", cmd.Duration)
	}
}
