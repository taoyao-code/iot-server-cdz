package bkv

import (
	"testing"
)

// TestParseBKVExceptionEvent 测试异常事件解析
func TestParseBKVExceptionEvent(t *testing.T) {
	// 创建模拟的BKV载荷用于异常事件测试
	payload := &BKVPayload{
		Cmd:       0x1010,
		GatewayID: "82230811001447",
		Fields: []TLVField{
			{Tag: 0x4A, Value: []byte{0x02}},         // 插座号 = 2
			{Tag: 0x54, Value: []byte{0x08}},         // 插座事件原因 = 8
			{Tag: 0x4B, Value: []byte{0x00}},         // 插座事件状态 = 0
			{Tag: 0x4E, Value: []byte{0x08, 0xBC}},   // 过压值 = 2236
			{Tag: 0x4F, Value: []byte{0x08, 0xBC}},   // 欠压值 = 2236
			{Tag: 0x57, Value: []byte{0x80}},         // 插孔1充电状态 = 128
			{Tag: 0x58, Value: []byte{0xB0}},         // 插孔2充电状态 = 176
		},
	}

	event, err := ParseBKVExceptionEvent(payload)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if event.SocketNo != 2 {
		t.Errorf("expected socket 2, got %d", event.SocketNo)
	}
	if event.SocketEventReason != 8 {
		t.Errorf("expected reason 8, got %d", event.SocketEventReason)
	}
	if event.OverVoltage != 2236 {
		t.Errorf("expected overvoltage 2236, got %d", event.OverVoltage)
	}
	if event.Port1ChargingStatus != 128 {
		t.Errorf("expected port1 status 128, got %d", event.Port1ChargingStatus)
	}
}

// TestParseBKVParameterQuery 测试参数查询解析
func TestParseBKVParameterQuery(t *testing.T) {
	// 基于协议文档的参数查询示例
	payload := &BKVPayload{
		Cmd:       0x1012,
		GatewayID: "82220420000552",
		Fields: []TLVField{
			{Tag: 0x4A, Value: []byte{0x02}},         // 插座号 = 2
			{Tag: 0x23, Value: []byte{0x00, 0x64}},   // 充满功率阈值 = 100 (10.0W)
			{Tag: 0x60, Value: []byte{0x0A}},         // 涓流阈值 = 10%
			{Tag: 0x21, Value: []byte{0x1C, 0x20}},   // 充满续充时间 = 7200s
			{Tag: 0x24, Value: []byte{0x00, 0x08}},   // 空载功率阈值 = 8 (0.8W)
			{Tag: 0x22, Value: []byte{0x00, 0x78}},   // 空载延时时间 = 120s
			{Tag: 0x59, Value: []byte{0x02, 0x58}},   // 最大充电时间 = 600min
			{Tag: 0x25, Value: []byte{0x55}},         // 高温阈值 = 85°C
			{Tag: 0x11, Value: []byte{0x1B, 0x58}},   // 功率限值 = 7000 (700.0W)
			{Tag: 0x10, Value: []byte{0x13, 0x88}},   // 过流限值 = 5000 (5A)
			{Tag: 0x68, Value: []byte{0x00, 0x00}},   // 按键基础金额 = 0
			{Tag: 0x93, Value: []byte{0x00, 0x2D}},   // 防脉冲时间 = 45
		},
	}

	param, err := ParseBKVParameterQuery(payload)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if param.SocketNo != 2 {
		t.Errorf("expected socket 2, got %d", param.SocketNo)
	}
	if param.FullPowerThreshold != 100 {
		t.Errorf("expected full power threshold 100, got %d", param.FullPowerThreshold)
	}
	if param.TrickleThreshold != 10 {
		t.Errorf("expected trickle threshold 10, got %d", param.TrickleThreshold)
	}
	if param.HighTempThreshold != 85 {
		t.Errorf("expected high temp threshold 85, got %d", param.HighTempThreshold)
	}
	if param.PowerLimit != 7000 {
		t.Errorf("expected power limit 7000, got %d", param.PowerLimit)
	}
}

// TestBKVPayload_IsStatusReport 测试状态上报判断
func TestBKVPayload_IsStatusReport(t *testing.T) {
	testCases := []struct {
		cmd      uint16
		expected bool
		desc     string
	}{
		{0x1017, true, "状态上报命令"},
		{0x1004, false, "充电结束命令"},
		{0x1010, false, "异常事件命令"},
		{0x1012, false, "参数查询命令"},
	}

	for _, tc := range testCases {
		payload := &BKVPayload{Cmd: tc.cmd}
		result := payload.IsStatusReport()
		if result != tc.expected {
			t.Errorf("%s: expected %v, got %v", tc.desc, tc.expected, result)
		}
	}
}

// TestBKVPayload_IsChargingEnd 测试充电结束判断
func TestBKVPayload_IsChargingEnd(t *testing.T) {
	testCases := []struct {
		cmd      uint16
		expected bool
		desc     string
	}{
		{0x1004, true, "充电结束命令"},
		{0x1017, false, "状态上报命令"},
		{0x1010, false, "异常事件命令"},
		{0x1012, false, "参数查询命令"},
	}

	for _, tc := range testCases {
		payload := &BKVPayload{Cmd: tc.cmd}
		result := payload.IsChargingEnd()
		if result != tc.expected {
			t.Errorf("%s: expected %v, got %v", tc.desc, tc.expected, result)
		}
	}
}

// TestBKVPayload_IsExceptionReport 测试异常事件判断
func TestBKVPayload_IsExceptionReport(t *testing.T) {
	testCases := []struct {
		cmd      uint16
		expected bool
		desc     string
	}{
		{0x1010, true, "异常事件命令"},
		{0x1017, false, "状态上报命令"},
		{0x1004, false, "充电结束命令"},
		{0x1012, false, "参数查询命令"},
	}

	for _, tc := range testCases {
		payload := &BKVPayload{Cmd: tc.cmd}
		result := payload.IsExceptionReport()
		if result != tc.expected {
			t.Errorf("%s: expected %v, got %v", tc.desc, tc.expected, result)
		}
	}
}

// TestBKVPayload_IsParameterQuery 测试参数查询判断
func TestBKVPayload_IsParameterQuery(t *testing.T) {
	testCases := []struct {
		cmd      uint16
		expected bool
		desc     string
	}{
		{0x1012, true, "参数查询命令"},
		{0x1017, false, "状态上报命令"},
		{0x1004, false, "充电结束命令"},
		{0x1010, false, "异常事件命令"},
	}

	for _, tc := range testCases {
		payload := &BKVPayload{Cmd: tc.cmd}
		result := payload.IsParameterQuery()
		if result != tc.expected {
			t.Errorf("%s: expected %v, got %v", tc.desc, tc.expected, result)
		}
	}
}

// TestBKVPayload_IsCardCharging 测试刷卡充电判断
func TestBKVPayload_IsCardCharging(t *testing.T) {
	// 包含余额字段的载荷（刷卡相关）
	cardPayload := &BKVPayload{
		Cmd: 0x1007,
		Fields: []TLVField{
			{Tag: 0x68, Value: []byte{0x01, 0x00}}, // 余额相关字段
		},
	}

	// 不包含刷卡相关字段的载荷
	normalPayload := &BKVPayload{
		Cmd: 0x1007,
		Fields: []TLVField{
			{Tag: 0x4A, Value: []byte{0x01}}, // 普通字段
		},
	}

	if !cardPayload.IsCardCharging() {
		t.Error("expected card charging payload to be detected")
	}

	if normalPayload.IsCardCharging() {
		t.Error("expected normal payload not to be detected as card charging")
	}
}

// TestBKVCommands_Extension 测试扩展的BKV命令支持
func TestBKVCommands_Extension(t *testing.T) {
	// 测试现有的BKV命令
	supportedCommands := []uint16{0x0000, 0x1000, 0x0015, 0x0005, 0x0007}
	
	for _, cmd := range supportedCommands {
		if !IsBKVCommand(cmd) {
			t.Errorf("command 0x%04X should be supported", cmd)
		}
	}
	
	// 测试不支持的命令（更新：0x01-0x04已在Week9实现）
	unsupportedCommands := []uint16{0x0006, 0x0010, 0x1001, 0x2000}
	
	for _, cmd := range unsupportedCommands {
		if IsBKVCommand(cmd) {
			t.Errorf("command 0x%04X should not be supported", cmd)
		}
	}
}