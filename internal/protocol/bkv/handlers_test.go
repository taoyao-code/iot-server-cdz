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

// TestHandleControl_SubCmd02TriggersEnd 测试子命令0x02始终触发充电结束
// 规范：子命令 0x02/0x18 即表示充电结束，不检查 Status 的 bit5 位
// 参考：minimal_bkv_service.go 中的 isChargingEnd() 只检查子命令
func TestHandleControl_SubCmd02TriggersEnd(t *testing.T) {
	sink := &mockEventSink{}
	h := &Handlers{
		CoreEvents: sink,
	}

	// 构造一个 subCmd=0x02 的帧，Status bit5=1（充电中）
	// 关键点：即使 Status 显示充电中，子命令 0x02 也表示充电结束
	// Status = 0xB0 = 10110000: bit7=1(在线), bit5=1(充电), bit4=1(空载)
	data := []byte{
		0x00, 0x11, // 长度 = 17
		0x02,       // subCmd = 充电结束子命令
		0x01,       // 插座号
		0xFF, 0xFF, // 软件版本
		0x20,       // 温度
		0x1E,       // RSSI
		0x00,       // 插孔号
		0xB0,       // 状态 = 0xB0 (bit5=1, 协议正常行为)
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

	// 验证应该产生 SessionEnded 事件（子命令0x02直接触发）
	foundSessionEnded := false
	for _, ev := range sink.events {
		if ev.Type == coremodel.EventSessionEnded {
			foundSessionEnded = true
		}
	}
	if !foundSessionEnded {
		t.Errorf("Expected SessionEnded event for subCmd=0x02, regardless of bit5 status")
	}
}

// TestHandleControl_ChargingEnded 测试充电结束正确触发 SessionEnded
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
		0x90,       // 状态 = 0x90 (bit5=0, 非充电)
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
		t.Errorf("Expected SessionEnded event for subCmd=0x02, but none found")
	}
}

// TestHandleControl_SubCmd02_18_AlwaysEnd 测试子命令0x02和0x18始终表示充电结束
// 这是核心修复：不再依赖 Status bit5 判断是否结束
func TestHandleControl_SubCmd02_18_AlwaysEnd(t *testing.T) {
	tests := []struct {
		name   string
		subCmd byte
		status byte
	}{
		{"subCmd=0x02, status=0xB0(charging)", 0x02, 0xB0},
		{"subCmd=0x02, status=0xA0(charging)", 0x02, 0xA0},
		{"subCmd=0x02, status=0x90(idle)", 0x02, 0x90},
		{"subCmd=0x18, status=0xB0(charging)", 0x18, 0xB0},
		{"subCmd=0x18, status=0xA0(charging)", 0x18, 0xA0},
		{"subCmd=0x18, status=0x90(idle)", 0x18, 0x90},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sink := &mockEventSink{}
			h := &Handlers{
				CoreEvents: sink,
			}

			data := []byte{
				0x00, 0x11, // 长度 = 17
				tt.subCmd,  // subCmd
				0x01,       // 插座号
				0xFF, 0xFF, // 软件版本
				0x20,       // 温度
				0x1E,       // RSSI
				0x00,       // 插孔号
				tt.status,  // 状态
				0x00, 0x2B, // 业务号
				0x00, 0x64, // 瞬时功率
				0x00, 0x00, // 瞬时电流
				0x00, 0x0A, // 用电量
				0x00, 0x05, // 充电时间
			}

			frame := &Frame{
				GatewayID: "TEST-DEVICE",
				Cmd:       0x0015,
				Data:      data,
				Direction: 1,
			}

			_ = h.HandleControl(context.Background(), frame)

			// 子命令 0x02/0x18 必须触发 SessionEnded，无论 Status 如何
			foundSessionEnded := false
			for _, ev := range sink.events {
				if ev.Type == coremodel.EventSessionEnded {
					foundSessionEnded = true
				}
			}

			if !foundSessionEnded {
				t.Errorf("subCmd=0x%02X, status=0x%02X: expected SessionEnded event (subCmd determines ending, not status bit5)", tt.subCmd, tt.status)
			}
		})
	}
}

// TestHandleControl_SessionEndedCarriesNextPortStatus 测试 SessionEnded 事件携带正确的终态
func TestHandleControl_SessionEndedCarriesNextPortStatus(t *testing.T) {
	sink := &mockEventSink{}
	h := &Handlers{
		CoreEvents: sink,
	}

	data := []byte{
		0x00, 0x11, // 长度 = 17
		0x02,       // subCmd = 充电结束
		0x01,       // 插座号
		0xFF, 0xFF, // 软件版本
		0x20,       // 温度
		0x1E,       // RSSI
		0x00,       // 插孔号
		0xB0,       // 状态（无关紧要，终态由协议固定为0x90）
		0x00, 0x2B, // 业务号 = 43
		0x00, 0x64, // 瞬时功率 = 100
		0x00, 0x00, // 瞬时电流
		0x00, 0x0A, // 用电量 = 10
		0x00, 0x05, // 充电时间 = 5
	}

	frame := &Frame{
		GatewayID: "TEST-DEVICE",
		Cmd:       0x0015,
		Data:      data,
		Direction: 1,
	}

	_ = h.HandleControl(context.Background(), frame)

	// 查找 SessionEnded 事件并验证 NextPortStatus
	for _, ev := range sink.events {
		if ev.Type == coremodel.EventSessionEnded && ev.SessionEnded != nil {
			if ev.SessionEnded.NextPortStatus == nil {
				t.Error("SessionEnded.NextPortStatus should not be nil")
			} else if *ev.SessionEnded.NextPortStatus != 0x90 {
				t.Errorf("SessionEnded.NextPortStatus expected=0x90 (idle), got=0x%02X", *ev.SessionEnded.NextPortStatus)
			}
			return
		}
	}
	t.Error("SessionEnded event not found")
}
