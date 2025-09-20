package gn

import (
	"encoding/hex"
	"testing"
)

func TestFrame_EncodeDecodeRoundTrip(t *testing.T) {
	// 测试数据
	gwid, _ := hex.DecodeString("82200520004869")
	payload := []byte{0x20, 0x20, 0x07, 0x30, 0x16, 0x45, 0x45}

	// 创建下行帧
	frame, err := NewFrame(0x0000, 0x00000000, gwid, payload, true)
	if err != nil {
		t.Fatalf("NewFrame failed: %v", err)
	}

	// 编码
	encoded, err := frame.Encode()
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	t.Logf("Encoded frame: %s", hex.EncodeToString(encoded))

	// 解码
	decoded, err := ParseFrame(encoded)
	if err != nil {
		t.Fatalf("ParseFrame failed: %v", err)
	}

	// 验证字段
	if decoded.Header != frame.Header {
		t.Errorf("Header mismatch: expected 0x%04X, got 0x%04X", frame.Header, decoded.Header)
	}
	if decoded.Command != frame.Command {
		t.Errorf("Command mismatch: expected 0x%04X, got 0x%04X", frame.Command, decoded.Command)
	}
	if decoded.Sequence != frame.Sequence {
		t.Errorf("Sequence mismatch: expected 0x%08X, got 0x%08X", frame.Sequence, decoded.Sequence)
	}
	if decoded.Direction != frame.Direction {
		t.Errorf("Direction mismatch: expected 0x%02X, got 0x%02X", frame.Direction, decoded.Direction)
	}
	if decoded.GetGatewayIDHex() != frame.GetGatewayIDHex() {
		t.Errorf("GatewayID mismatch: expected %s, got %s", 
			frame.GetGatewayIDHex(), decoded.GetGatewayIDHex())
	}
}

func TestParseFrame_RealExample(t *testing.T) {
	// 来自文档的实际心跳回复示例
	hexStr := "fcff0018000000000000008220052000486920200730164545a7fcee"
	data, err := hex.DecodeString(hexStr)
	if err != nil {
		t.Fatalf("Failed to decode hex: %v", err)
	}

	t.Logf("Testing frame: %s", hexStr)
	t.Logf("Frame length: %d bytes", len(data))

	frame, err := ParseFrame(data)
	if err != nil {
		t.Fatalf("ParseFrame failed: %v", err)
	}

	// 验证基本字段
	if !frame.IsDownlink() {
		t.Error("Expected downlink frame")
	}
	if frame.Command != 0x0000 {
		t.Errorf("Expected command 0x0000, got 0x%04X", frame.Command)
	}
	if frame.Sequence != 0x00000000 {
		t.Errorf("Expected sequence 0x00000000, got 0x%08X", frame.Sequence)
	}

	expectedGWID := "82200520004869"
	if frame.GetGatewayIDHex() != expectedGWID {
		t.Errorf("Expected gateway ID %s, got %s", expectedGWID, frame.GetGatewayIDHex())
	}

	t.Logf("Parsed frame successfully: cmd=0x%04X, seq=0x%08X, gwid=%s", 
		frame.Command, frame.Sequence, frame.GetGatewayIDHex())
	t.Logf("Payload: %s", hex.EncodeToString(frame.Payload))
}

func TestCalculateChecksum(t *testing.T) {
	// 测试校验和计算 - 从文档示例
	// fcff0018000000000000008220052000486920200730164545a7fcee
	// 校验和计算范围: 从长度字段开始到载荷结束
	hexStr := "0018000000000000008220052000486920200730164545"
	data, _ := hex.DecodeString(hexStr)

	checksum := calculateChecksum(data)
	expected := uint8(0xa7) // 从文档示例

	if checksum != expected {
		t.Errorf("Checksum mismatch: expected 0x%02X, got 0x%02X", expected, checksum)
	}
}

func TestFrame_InvalidGatewayID(t *testing.T) {
	// 测试无效网关ID长度
	gwid := []byte{0x01, 0x02, 0x03} // 只有3字节，应该是7字节

	_, err := NewFrame(0x0000, 0x00000000, gwid, nil, false)
	if err == nil {
		t.Error("Expected error for invalid gateway ID length")
	}
}

func TestParseFrame_InvalidHeader(t *testing.T) {
	// 测试无效帧头
	data := []byte{0xFF, 0xFF, 0x00, 0x18} // 无效帧头
	data = append(data, make([]byte, 20)...) // 填充足够长度

	_, err := ParseFrame(data)
	if err == nil {
		t.Error("Expected error for invalid frame header")
	}
}

func TestParseFrame_TooShort(t *testing.T) {
	// 测试过短的帧
	data := []byte{0xFC, 0xFE, 0x00, 0x18} // 太短

	_, err := ParseFrame(data)
	if err == nil {
		t.Error("Expected error for too short frame")
	}
}