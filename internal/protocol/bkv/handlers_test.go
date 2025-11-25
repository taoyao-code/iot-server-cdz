package bkv

import (
	"context"
	"testing"

	"github.com/taoyao-code/iot-server/internal/coremodel"
)

// mockEventSink 用于捕获发送的事件
type mockEventSink struct {
	events []*coremodel.CoreEvent
}

func (m *mockEventSink) HandleCoreEvent(ctx context.Context, ev *coremodel.CoreEvent) error {
	m.events = append(m.events, ev)
	return nil
}

// TestHandleControl_ChargingInProgress 测试充电进行中的状态上报不会触发 SessionEnded
func TestHandleControl_ChargingInProgress(t *testing.T) {
	sink := &mockEventSink{}
	h := &Handlers{
		CoreEvents: sink,
	}

	// 构造一个 subCmd=0x02 但 Status bit5=1（充电中）的帧
	// 格式: [长度高][长度低][subCmd=0x02][插座号][版本高][版本低][温度][RSSI][插孔][状态][业务号高][业务号低]...
	// Status = 0xB0 = 10110000: bit7=1(在线), bit5=1(充电), bit4=1(空载)
	data := []byte{
		0x00, 0x11, // 长度 = 17
		0x02,       // subCmd = 充电结束子命令
		0x01,       // 插座号
		0xFF, 0xFF, // 软件版本
		0x20,       // 温度
		0x1E,       // RSSI
		0x00,       // 插孔号
		0xB0,       // 状态 = 0xB0 (bit5=1, 充电中!)
		0x00, 0x2B, // 业务号 = 43
		0x00, 0x00, // 瞬时功率
		0x00, 0x00, // 瞬时电流
		0x00, 0x00, // 用电量
		0x00, 0x00, // 充电时间
	}

	frame := &Frame{
		GatewayID: "TEST-DEVICE",
		Cmd:       0x0015,
		Data:      data,
		Direction: 1, // 上行
	}

	err := h.HandleControl(context.Background(), frame)
	if err != nil {
		t.Fatalf("HandleControl failed: %v", err)
	}

	// 验证不应该产生 SessionEnded 事件
	for _, ev := range sink.events {
		if ev.Type == coremodel.EventSessionEnded {
			t.Errorf("Expected no SessionEnded event when bit5=1 (charging), but got one")
		}
	}

	// 应该只产生 PortSnapshot 事件
	foundPortSnapshot := false
	for _, ev := range sink.events {
		if ev.Type == coremodel.EventPortSnapshot {
			foundPortSnapshot = true
		}
	}
	if !foundPortSnapshot {
		t.Errorf("Expected PortSnapshot event, but none found")
	}
}

// TestHandleControl_ChargingEnded 测试真正的充电结束会触发 SessionEnded
func TestHandleControl_ChargingEnded(t *testing.T) {
	sink := &mockEventSink{}
	h := &Handlers{
		CoreEvents: sink,
	}

	// 构造一个 subCmd=0x02 且 Status bit5=0（非充电）的帧
	// Status = 0x90 = 10010000: bit7=1(在线), bit5=0(非充电), bit4=1(空载)
	data := []byte{
		0x00, 0x11, // 长度 = 17
		0x02,       // subCmd = 充电结束子命令
		0x01,       // 插座号
		0xFF, 0xFF, // 软件版本
		0x20,       // 温度
		0x1E,       // RSSI
		0x00,       // 插孔号
		0x90,       // 状态 = 0x90 (bit5=0, 非充电!)
		0x00, 0x2B, // 业务号 = 43
		0x00, 0x64, // 瞬时功率 = 100 (10W)
		0x00, 0x00, // 瞬时电流
		0x00, 0x0A, // 用电量 = 10 (0.1kWh)
		0x00, 0x05, // 充电时间 = 5分钟
	}

	frame := &Frame{
		GatewayID: "TEST-DEVICE",
		Cmd:       0x0015,
		Data:      data,
		Direction: 1, // 上行
	}

	err := h.HandleControl(context.Background(), frame)
	if err != nil {
		t.Fatalf("HandleControl failed: %v", err)
	}

	// 验证应该产生 SessionEnded 事件
	foundSessionEnded := false
	for _, ev := range sink.events {
		if ev.Type == coremodel.EventSessionEnded {
			foundSessionEnded = true
		}
	}
	if !foundSessionEnded {
		t.Errorf("Expected SessionEnded event when bit5=0 (not charging), but none found")
	}
}

// TestHandleControl_StatusBit5Detection 测试不同状态位的检测
func TestHandleControl_StatusBit5Detection(t *testing.T) {
	tests := []struct {
		name             string
		status           byte
		expectSessionEnd bool
	}{
		{"0xB0 (bit5=1, charging)", 0xB0, false},
		{"0xA0 (bit5=1, charging)", 0xA0, false},
		{"0x90 (bit5=0, idle)", 0x90, true},
		{"0x80 (bit5=0, online only)", 0x80, true},
		{"0x10 (bit5=0, idle only)", 0x10, true},
		{"0x30 (bit5=1, charging)", 0x30, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sink := &mockEventSink{}
			h := &Handlers{
				CoreEvents: sink,
			}

			data := []byte{
				0x00, 0x11, // 长度 = 17
				0x02,       // subCmd
				0x01,       // 插座号
				0xFF, 0xFF, // 软件版本
				0x20,       // 温度
				0x1E,       // RSSI
				0x00,       // 插孔号
				tt.status,  // 状态
				0x00, 0x2B, // 业务号
				0x00, 0x00, // 瞬时功率
				0x00, 0x00, // 瞬时电流
				0x00, 0x00, // 用电量
				0x00, 0x00, // 充电时间
			}

			frame := &Frame{
				GatewayID: "TEST-DEVICE",
				Cmd:       0x0015,
				Data:      data,
				Direction: 1,
			}

			_ = h.HandleControl(context.Background(), frame)

			foundSessionEnded := false
			for _, ev := range sink.events {
				if ev.Type == coremodel.EventSessionEnded {
					foundSessionEnded = true
				}
			}

			if foundSessionEnded != tt.expectSessionEnd {
				t.Errorf("status=0x%02X: expected SessionEnded=%v, got=%v", tt.status, tt.expectSessionEnd, foundSessionEnded)
			}
		})
	}
}
