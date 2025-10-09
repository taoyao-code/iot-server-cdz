package bkv

import (
	"encoding/hex"
	"testing"
)

func TestParseBKVPayload_StatusReport(t *testing.T) {
	// 协议文档中的插座状态上报数据
	// 04010110170a010200000000000000000901038223121400270065019403014a0104013effff0301
	// 07250301961e28015b030108000301098004010a000004019508e304010b000004010c000104010d000
	// 004010e000028015b030108010301098004010a000004019508e304010b000004010c000104010d0000
	// 04010e0000
	hexStr := "04010110170a010200000000000000000901038223121400270065019403014a0104013effff030107250301961e28015b030108000301098004010a000004019508e304010b000004010c000104010d000004010e000028015b030108010301098004010a000004019508e304010b000004010c000104010d000004010e0000"

	data, err := hex.DecodeString(hexStr)
	if err != nil {
		t.Fatalf("hex decode error: %v", err)
	}

	payload, err := ParseBKVPayload(data)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if payload.Cmd != 0x1017 {
		t.Errorf("expected cmd 0x1017, got 0x%04x", payload.Cmd)
	}

	if payload.FrameSeq != 0 {
		t.Errorf("expected frameSeq 0, got %d", payload.FrameSeq)
	}

	if payload.GatewayID != "82231214002700" {
		t.Errorf("expected gatewayID 82231214002700, got %s", payload.GatewayID)
	}

	// 应该有多个TLV字段
	if len(payload.Fields) < 3 {
		t.Errorf("expected at least 3 TLV fields, got %d", len(payload.Fields))
	}

	// 调试输出字段
	for i, field := range payload.Fields {
		t.Logf("Field %d: tag=0x%02x, len=%d, value=%02x", i, field.Tag, field.Length, field.Value)
	}

	// 测试插座状态解析 (简化版本，只检查找到了0x65字段)
	var found0x65 bool
	for _, field := range payload.Fields {
		if field.Tag == 0x65 {
			found0x65 = true
			break
		}
	}
	if !found0x65 {
		t.Error("should find 0x65 field for socket status")
	}
}

func TestFrame_GetBKVPayload(t *testing.T) {
	// 构造一个BKV帧
	bkvData := "04010110170a010200000000000000000901038223121400270065019403014a0104013effff"
	data, _ := hex.DecodeString(bkvData)

	frame := &Frame{
		Cmd:  0x1000,
		Data: data,
	}

	payload, err := frame.GetBKVPayload()
	if err != nil {
		t.Fatalf("failed to get BKV payload: %v", err)
	}

	if payload.Cmd != 0x1017 {
		t.Errorf("expected BKV cmd 0x1017, got 0x%04x", payload.Cmd)
	}

	// 测试缓存
	payload2, err := frame.GetBKVPayload()
	if err != nil {
		t.Fatalf("failed to get cached BKV payload: %v", err)
	}

	if payload != payload2 {
		t.Error("BKV payload should be cached")
	}
}

func TestFrame_IsHeartbeat(t *testing.T) {
	// 测试简单心跳
	frame1 := &Frame{Cmd: 0x0000}
	if !frame1.IsHeartbeat() {
		t.Error("frame with cmd 0x0000 should be heartbeat")
	}

	// 测试BKV心跳 (需要足够的数据: 26字节)
	bkvData := "04010110170a0102000000000000000009010382231214002700"
	data, _ := hex.DecodeString(bkvData)
	frame2 := &Frame{
		Cmd:  0x1000,
		Data: data,
	}
	if !frame2.IsHeartbeat() {
		t.Error("BKV frame with cmd 0x1017 should be heartbeat")
	}

	// 测试非心跳
	frame3 := &Frame{Cmd: 0x0015}
	if frame3.IsHeartbeat() {
		t.Error("frame with cmd 0x0015 should not be heartbeat")
	}
}

func TestFrame_IsBKVFrame(t *testing.T) {
	frame1 := &Frame{Cmd: 0x1000}
	if !frame1.IsBKVFrame() {
		t.Error("frame with cmd 0x1000 should be BKV frame")
	}

	frame2 := &Frame{Cmd: 0x0000}
	if frame2.IsBKVFrame() {
		t.Error("frame with cmd 0x0000 should not be BKV frame")
	}
}
