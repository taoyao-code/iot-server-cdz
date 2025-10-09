package bkv

import (
	"encoding/binary"
	"encoding/hex"
	"testing"
)

func TestParse_Min(t *testing.T) {
	// 协议文档中的完整hex字符串
	hexStr := "fcfe002e0000000000000182200520004869383938363034363331313230373033313934313763562e31723436001fcafcee"
	raw, err := hex.DecodeString(hexStr)
	if err != nil {
		t.Fatalf("hex decode error: %v", err)
	}

	t.Logf("Raw bytes: %02x", raw)
	t.Logf("Length: %d", len(raw))
	if len(raw) >= 4 {
		frameLen := binary.BigEndian.Uint16(raw[2:4])
		t.Logf("Frame length: %d", frameLen)
		if int(frameLen) <= len(raw) && frameLen >= 2 {
			t.Logf("Last 2 bytes at frame length: %02x", raw[frameLen-2:frameLen])
		}
	}

	fr, err := Parse(raw)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if fr.Cmd != 0x0000 {
		t.Errorf("expected cmd 0x0000, got 0x%04x", fr.Cmd)
	}
	if fr.MsgID != 0x00000000 {
		t.Errorf("expected msgID 0x00000000, got 0x%08x", fr.MsgID)
	}
	if fr.Direction != 0x01 {
		t.Errorf("expected direction 0x01, got 0x%02x", fr.Direction)
	}
	if !fr.IsUplink() {
		t.Error("expected uplink frame")
	}
	if fr.GatewayID != "82200520004869" {
		t.Errorf("expected gatewayID 82200520004869, got %s", fr.GatewayID)
	}
}

func TestParse_Downlink(t *testing.T) {
	// 使用协议文档中的下行报文: 心跳回复
	// fcff0018000000000000008220052000486920200730164545a7fcee
	hexStr := "fcff0018000000000000008220052000486920200730164545a7fcee"
	raw, err := hex.DecodeString(hexStr)
	if err != nil {
		t.Fatalf("hex decode error: %v", err)
	}

	fr, err := Parse(raw)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if fr.Cmd != 0x0000 {
		t.Errorf("expected cmd 0x0000, got 0x%04x", fr.Cmd)
	}
	if fr.Direction != 0x00 {
		t.Errorf("expected direction 0x00, got 0x%02x", fr.Direction)
	}
	if !fr.IsDownlink() {
		t.Error("expected downlink frame")
	}
}

func TestStreamDecoder(t *testing.T) {
	d := NewStreamDecoder()

	// 测试粘包：两个心跳帧拼接
	hexStr1 := "fcfe002e0000000000000182200520004869383938363034363331313230373033313934313763562e31723436001fcafcee"
	hexStr2 := "fcff0018000000000000008220052000486920200730164545a7fcee"

	raw1, _ := hex.DecodeString(hexStr1)
	raw2, _ := hex.DecodeString(hexStr2)

	// 拼接成粘包
	combined := append(raw1, raw2...)

	frames, err := d.Feed(combined)
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}

	if len(frames) != 2 {
		t.Fatalf("expected 2 frames, got %d", len(frames))
	}

	// 检查第一帧
	if frames[0].Cmd != 0x0000 || !frames[0].IsUplink() {
		t.Error("first frame should be uplink heartbeat")
	}

	// 检查第二帧
	if frames[1].Cmd != 0x0000 || !frames[1].IsDownlink() {
		t.Error("second frame should be downlink heartbeat")
	}
}

func TestStreamDecoder_HalfPacket(t *testing.T) {
	d := NewStreamDecoder()

	// 测试半包
	hexStr := "fcfe002e0000000000000182200520004869383938363034363331313230373033313934313763562e31723436001fcafcee"
	raw, _ := hex.DecodeString(hexStr)

	// 先发送一半
	half := len(raw) / 2
	frames1, err := d.Feed(raw[:half])
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(frames1) != 0 {
		t.Fatalf("expected 0 frames from half packet, got %d", len(frames1))
	}

	// 发送剩余部分
	frames2, err := d.Feed(raw[half:])
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(frames2) != 1 {
		t.Fatalf("expected 1 frame after completing packet, got %d", len(frames2))
	}

	if frames2[0].Cmd != 0x0000 {
		t.Errorf("expected cmd 0x0000, got 0x%04x", frames2[0].Cmd)
	}
}
