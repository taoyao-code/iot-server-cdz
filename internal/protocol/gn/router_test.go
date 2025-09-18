package gn

import (
	"context"
	"encoding/hex"
	"testing"
)

func TestRouter_Route(t *testing.T) {
	handler := NewDefaultHandler()
	router := NewRouter(handler)
	
	// 创建测试帧
	gwid, _ := hex.DecodeString("82200520004869")
	
	tests := []struct {
		name    string
		cmd     uint16
		payload []byte
		wantErr bool
	}{
		{
			name:    "heartbeat",
			cmd:     CmdHeartbeat,
			payload: []byte("test_heartbeat"),
			wantErr: false,
		},
		{
			name:    "status_report",
			cmd:     CmdStatusReport,
			payload: []byte("test_status"),
			wantErr: false,
		},
		{
			name:    "unknown_command",
			cmd:     0x9999,
			payload: nil,
			wantErr: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 构建帧
			frame, err := NewFrame(tt.cmd, 0x12345678, gwid, tt.payload, false)
			if err != nil {
				t.Fatalf("NewFrame failed: %v", err)
			}
			
			// 编码帧
			data, err := frame.Encode()
			if err != nil {
				t.Fatalf("Encode failed: %v", err)
			}
			
			// 路由
			err = router.Route(context.Background(), data)
			if (err != nil) != tt.wantErr {
				t.Errorf("Route() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestParseHeartbeat(t *testing.T) {
	// 基于文档示例构造心跳载荷
	// ICCID: 89860463112070319417 (18字节BCD)
	// 固件版本: cV.1r46 (7字节)
	// RSSI: 31 (1字节)
	
	payload := []byte{
		0x38, 0x39, 0x38, 0x36, 0x30, 0x34, 0x36, 0x33, 0x31, // ICCID前9字节
		0x31, 0x32, 0x30, 0x37, 0x30, 0x33, 0x31, 0x39, 0x34, // ICCID后9字节
		0x63, 0x56, 0x2e, 0x31, 0x72, 0x34, 0x36,             // 固件版本 "cV.1r46"
		0x1f,                                                 // RSSI = 31
	}
	
	iccid, rssi, fwVer, err := ParseHeartbeat(payload)
	if err != nil {
		t.Fatalf("ParseHeartbeat failed: %v", err)
	}
	
	expectedICCID := hex.EncodeToString(payload[:18])
	if iccid != expectedICCID {
		t.Errorf("ICCID mismatch: expected %s, got %s", expectedICCID, iccid)
	}
	
	if rssi != 31 {
		t.Errorf("RSSI mismatch: expected 31, got %d", rssi)
	}
	
	if fwVer != "cV.1r46" {
		t.Errorf("Firmware version mismatch: expected 'cV.1r46', got '%s'", fwVer)
	}
	
	t.Logf("Parsed heartbeat: ICCID=%s, RSSI=%d, FwVer=%s", iccid, rssi, fwVer)
}

func TestBuildTimeSync(t *testing.T) {
	payload := BuildTimeSync()
	
	if len(payload) != 14 {
		t.Errorf("Expected time sync payload length 14, got %d", len(payload))
	}
	
	timeStr := string(payload)
	t.Logf("Time sync payload: %s", timeStr)
	
	// 验证格式 YYYYMMDDHHMMSS
	if len(timeStr) != 14 {
		t.Error("Time format should be 14 characters")
	}
}

func TestParseSocketStatus(t *testing.T) {
	// 构造一个简单的插座状态TLV数据
	tlvs := TLVList{
		NewTLVUint8(TagSocketNumber, 1),      // 插座1
		NewTLVUint16(TagSoftwareVer, 0xFFFF), // 软件版本
		NewTLVUint8(TagTemperature, 37),      // 温度37度
		NewTLVUint8(TagRSSI, 25),             // RSSI
		// 插孔属性
		NewTLV(TagSocketAttr, EncodeTLVs(TLVList{
			NewTLVUint8(TagPortNumber, 0),     // 插孔0
			NewTLVUint8(TagPortStatus, 0x80),  // 状态
			NewTLVUint16(TagVoltage, 2275),    // 227.5V
			NewTLVUint16(TagPower, 1000),      // 100.0W
		})),
	}
	
	payload := EncodeTLVs(tlvs)
	t.Logf("Socket status payload: %s", hex.EncodeToString(payload))
	
	sockets, err := ParseSocketStatus(payload)
	if err != nil {
		t.Fatalf("ParseSocketStatus failed: %v", err)
	}
	
	if len(sockets) != 1 {
		t.Fatalf("Expected 1 socket, got %d", len(sockets))
	}
	
	socket := sockets[0]
	if socket.Number != 1 {
		t.Errorf("Expected socket number 1, got %d", socket.Number)
	}
	if socket.SoftwareVer != 0xFFFF {
		t.Errorf("Expected software version 0xFFFF, got 0x%04X", socket.SoftwareVer)
	}
	if socket.Temperature != 37 {
		t.Errorf("Expected temperature 37, got %d", socket.Temperature)
	}
	if socket.RSSI != 25 {
		t.Errorf("Expected RSSI 25, got %d", socket.RSSI)
	}
	
	if len(socket.Ports) != 1 {
		t.Fatalf("Expected 1 port, got %d", len(socket.Ports))
	}
	
	port := socket.Ports[0]
	if port.Number != 0 {
		t.Errorf("Expected port number 0, got %d", port.Number)
	}
	if port.StatusBits != 0x80 {
		t.Errorf("Expected status bits 0x80, got 0x%02X", port.StatusBits)
	}
	if port.Voltage != 227.5 {
		t.Errorf("Expected voltage 227.5V, got %.1fV", port.Voltage)
	}
	if port.Power != 100.0 {
		t.Errorf("Expected power 100.0W, got %.1fW", port.Power)
	}
	
	t.Logf("Parsed socket: %+v", socket)
}

func TestBuildStatusQuery(t *testing.T) {
	payload := BuildStatusQuery(1)
	
	if len(payload) != 1 {
		t.Errorf("Expected status query payload length 1, got %d", len(payload))
	}
	
	if payload[0] != 1 {
		t.Errorf("Expected socket number 1, got %d", payload[0])
	}
}

// TestHandler 测试用的处理器
type TestHandler struct {
	lastCommand uint16
	lastGwid    string
	lastPayload []byte
	callCount   int
}

func (h *TestHandler) HandleHeartbeat(ctx context.Context, frame *Frame, gwid string, payload []byte) error {
	h.recordCall(CmdHeartbeat, gwid, payload)
	return nil
}

func (h *TestHandler) HandleStatusReport(ctx context.Context, frame *Frame, gwid string, payload []byte) error {
	h.recordCall(CmdStatusReport, gwid, payload)
	return nil
}

func (h *TestHandler) HandleStatusQuery(ctx context.Context, frame *Frame, gwid string, payload []byte) error {
	h.recordCall(CmdStatusQuery, gwid, payload)
	return nil
}

func (h *TestHandler) HandleControl(ctx context.Context, frame *Frame, gwid string, payload []byte) error {
	h.recordCall(CmdControl, gwid, payload)
	return nil
}

func (h *TestHandler) HandleControlEnd(ctx context.Context, frame *Frame, gwid string, payload []byte) error {
	h.recordCall(CmdControlEnd, gwid, payload)
	return nil
}

func (h *TestHandler) HandleParamSet(ctx context.Context, frame *Frame, gwid string, payload []byte) error {
	h.recordCall(CmdParamSet, gwid, payload)
	return nil
}

func (h *TestHandler) HandleParamQuery(ctx context.Context, frame *Frame, gwid string, payload []byte) error {
	h.recordCall(CmdParamQuery, gwid, payload)
	return nil
}

func (h *TestHandler) HandleException(ctx context.Context, frame *Frame, gwid string, payload []byte) error {
	h.recordCall(CmdException, gwid, payload)
	return nil
}

func (h *TestHandler) recordCall(cmd uint16, gwid string, payload []byte) {
	h.lastCommand = cmd
	h.lastGwid = gwid
	h.lastPayload = make([]byte, len(payload))
	copy(h.lastPayload, payload)
	h.callCount++
}

func TestRouter_WithTestHandler(t *testing.T) {
	handler := &TestHandler{}
	router := NewRouter(handler)
	
	gwid, _ := hex.DecodeString("82200520004869")
	payload := []byte("test_data")
	
	// 创建心跳帧
	frame, err := NewFrame(CmdHeartbeat, 0x12345678, gwid, payload, false)
	if err != nil {
		t.Fatalf("NewFrame failed: %v", err)
	}
	
	data, err := frame.Encode()
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}
	
	// 路由
	err = router.Route(context.Background(), data)
	if err != nil {
		t.Fatalf("Route failed: %v", err)
	}
	
	// 验证处理器被调用
	if handler.callCount != 1 {
		t.Errorf("Expected 1 handler call, got %d", handler.callCount)
	}
	if handler.lastCommand != CmdHeartbeat {
		t.Errorf("Expected command 0x%04X, got 0x%04X", CmdHeartbeat, handler.lastCommand)
	}
	if handler.lastGwid != "82200520004869" {
		t.Errorf("Expected gwid '82200520004869', got '%s'", handler.lastGwid)
	}
	if hex.EncodeToString(handler.lastPayload) != hex.EncodeToString(payload) {
		t.Errorf("Payload mismatch: expected %s, got %s", 
			hex.EncodeToString(payload), hex.EncodeToString(handler.lastPayload))
	}
}