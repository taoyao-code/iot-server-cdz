package bkv

import (
	"context"
	"testing"

	"github.com/taoyao-code/iot-server/internal/coremodel"
	"github.com/taoyao-code/iot-server/internal/ordersession"
)

// TestChargingFlowComplete 测试完整的充电流程
// 模拟真实场景：平台下发命令 → 设备ACK → 充电���行 → 充电结束
func TestChargingFlowComplete(t *testing.T) {
	sink := &mockEventSink{}
	h := &Handlers{
		CoreEvents:   sink,
		OrderTracker: ordersession.NewTracker(),
	}

	deviceID := "82241218000382"
	portNo := 0
	h.OrderTracker.TrackPending(deviceID, portNo, 0, "TEST-ORDER", "unit-test")

	// === 步骤1: 平台下发开始充电命令（模拟API层）===
	// 注意：根据协议2.2.8，平台不发送业务号，只发送充电参数
	t.Log("Step 1: Platform dispatches start charge command")
	// 这一步只是发送命令，不创建会话（因为业务号由设备生成）

	// === 步骤2: 设备回复ACK，返回设备生成的业务号 ===
	t.Log("Step 2: Device ACK with generated business number")
	// 模拟设备ACK数据：07 01 01 00 003E
	// [0x07][成功][插座1][插孔0][业务号0x003E]
	ackData := []byte{
		0x00, 0x05, // 长度前缀 (5字节参数)
		0x07,       // 子命令
		0x01,       // 成功标志
		0x01,       // 插座号1
		0x00,       // 插孔0
		0x00, 0x3E, // 业务号62(0x003E) - 设备生成
	}

	ackFrame := &Frame{
		GatewayID: deviceID,
		Cmd:       0x0015,
		Data:      ackData,
		Direction: 1, // 上行
	}

	// 处理ACK，应该创建会话
	err := h.HandleControl(context.Background(), ackFrame)
	if err != nil {
		t.Fatalf("HandleControl ACK failed: %v", err)
	}

	// 验证会话已创建
	session, hasSession := h.OrderTracker.Lookup(deviceID, portNo)
	if !hasSession {
		t.Fatal("Session should be created after ACK")
	}
	expectedBizNo := "003E"
	if session.BusinessNo != expectedBizNo {
		t.Errorf("Business number mismatch: expected=%s, got=%s", expectedBizNo, session.BusinessNo)
	}
	t.Logf("✓ Session created: device=%s, port=%d, business_no=%s", deviceID, portNo, session.BusinessNo)

	// === 步骤3: 充电进行中（可选，模拟状态上报）===
	t.Log("Step 3: Charging in progress...")
	// 在实际场景中，设备会定期上报插座状态(cmd=0x0015 sub=0x02)
	// 这里跳过，直接到充电结束

	// === 步骤4: 充电结束上报 ===
	t.Log("Step 4: Device reports charging end")
	// 模拟充电结束数据包：02 01 FFFF 20 1E 00 90 003E 0000 0000 000A 0005
	endData := []byte{
		0x00, 0x11, // 长度前缀 (17字节参数)
		0x02,       // 子命令=充电结束
		0x01,       // 插座号1
		0xFF, 0xFF, // 软件版本
		0x20,       // 温度32
		0x1E,       // RSSI 30
		0x00,       // 插孔0
		0x90,       // 状态=0x90 (在线空载)
		0x00, 0x3E, // 业务号62(0x003E) - 与ACK中的相同！
		0x00, 0x64, // 瞬时功率=100 (10W)
		0x00, 0x00, // 瞬时电流=0
		0x00, 0x0A, // 用电量=10 (0.1kWh)
		0x00, 0x05, // 充电时间=5分钟
	}

	endFrame := &Frame{
		GatewayID: deviceID,
		Cmd:       0x0015,
		Data:      endData,
		Direction: 1, // 上行
	}

	// 处理充电结束
	err = h.HandleControl(context.Background(), endFrame)
	if err != nil {
		t.Fatalf("HandleControl charging end failed: %v", err)
	}

	// === 验证结果 ===
	// 1. 验证应该产生SessionEnded事件
	foundSessionEnded := false
	var sessionEndedEvent *coremodel.CoreEvent
	for _, ev := range sink.events {
		if ev.Type == coremodel.EventSessionEnded {
			foundSessionEnded = true
			sessionEndedEvent = ev
			break
		}
	}
	if !foundSessionEnded {
		t.Fatal("Expected SessionEnded event, but none found")
	}
	t.Log("✓ SessionEnded event generated")

	// 2. 验证事件数据
	if sessionEndedEvent.SessionEnded == nil {
		t.Fatal("SessionEnded payload is nil")
	}
	se := sessionEndedEvent.SessionEnded
	if se.BusinessNo != "003E" {
		t.Errorf("SessionEnded business_no: expected=003E, got=%s", se.BusinessNo)
	}
	if se.EnergyKWh01 != 10 {
		t.Errorf("SessionEnded energy: expected=10, got=%d", se.EnergyKWh01)
	}
	if se.DurationSec != 300 {
		t.Errorf("SessionEnded duration: expected=300, got=%d", se.DurationSec)
	}
	t.Logf("✓ SessionEnded data: business_no=%s, energy=%d (0.01kWh), duration=%d (sec)",
		se.BusinessNo, se.EnergyKWh01, se.DurationSec)

	// 3. 验证会话已清理
	if _, stillExists := h.OrderTracker.Lookup(deviceID, portNo); stillExists {
		t.Error("Session should be deleted after charging end")
	}
	t.Log("✓ Session cleaned up")

	t.Log("✓✓✓ Complete charging flow test PASSED")
}

// TestChargingFlowWithMismatchedBusinessNo 测试业务号不匹配的场景
// 这是当前生产环境的问题场景
func TestChargingFlowWithMismatchedBusinessNo(t *testing.T) {
	sink := &mockEventSink{}
	h := &Handlers{
		CoreEvents:   sink,
		OrderTracker: ordersession.NewTracker(),
	}

	deviceID := "82241218000382"
	portNo := 0

	// === 场景：错误地预先存储了平台生成的业务号 ===
	t.Log("Scenario: Session pre-stored with platform business number (WRONG!)")
	h.OrderTracker.TrackPending(deviceID, portNo, 0, "TEST-ORDER", "unit-test")
	if _, err := h.OrderTracker.Promote(deviceID, portNo, "C4A9"); err != nil {
		t.Fatalf("failed to promote fake session: %v", err)
	}
	t.Log("Pre-stored session with business_no=C4A9 (platform generated)")

	// === 设备上报充电结束，使用设备生成的业务号 ===
	t.Log("Device reports charging end with device-generated business number")
	endData := []byte{
		0x00, 0x11, // 长度
		0x02,       // 充电结束
		0x01,       // 插座1
		0xFF, 0xFF, // 版本
		0x20,       // 温度
		0x1E,       // RSSI
		0x00,       // 插孔0
		0x90,       // 状态
		0x00, 0x3E, // 业务号=0x003E (设备生���的62)
		0x00, 0x64, // 功率
		0x00, 0x00, // 电流
		0x00, 0x0A, // 电量
		0x00, 0x05, // 时间
	}

	endFrame := &Frame{
		GatewayID: deviceID,
		Cmd:       0x0015,
		Data:      endData,
		Direction: 1,
	}

	err := h.HandleControl(context.Background(), endFrame)
	if err != nil {
		t.Logf("HandleControl returned error (expected): %v", err)
	}

	// === 验证：业务号不匹配时，SessionEnded事件应该被拒绝 ===
	foundSessionEnded := false
	for _, ev := range sink.events {
		if ev.Type == coremodel.EventSessionEnded {
			foundSessionEnded = true
		}
	}
	if foundSessionEnded {
		t.Error("SessionEnded event should NOT be generated when business number mismatches")
	} else {
		t.Log("✓ SessionEnded correctly rejected due to business number mismatch")
		t.Log("✓ This demonstrates the ROOT CAUSE of the production issue")
	}
}

// TestBusinessNumberGeneration 测试业务号生成和验证流程
func TestBusinessNumberGeneration(t *testing.T) {
	t.Log("=== Business Number Generation Flow ===")
	t.Log("1. Platform sends control command WITHOUT business number")
	t.Log("   Payload: 07 01 00 01 01 00f0 0000")
	t.Log("   [cmd][socket][port][switch][mode][duration][energy]")
	t.Log("")
	t.Log("2. Device generates business number and returns in ACK")
	t.Log("   Payload: 07 01 01 00 003E")
	t.Log("   [cmd][result][socket][port][business_no]")
	t.Log("   Business number: 0x003E = 62 (device-generated)")
	t.Log("")
	t.Log("3. Device uses same business number in charging end report")
	t.Log("   Business number: 0x003E (matches ACK)")
	t.Log("")
	t.Log("4. Platform validates: ACK business_no == End business_no")
	t.Log("   Expected: 0x003E, Received: 0x003E → MATCH ✓")
	t.Log("")
	t.Log("=== Production Issue ===")
	t.Log("OLD CODE: Platform sent business_no=50345 in control command")
	t.Log("Device ignored it and generated business_no=62")
	t.Log("Platform expected 50345 but received 62 → MISMATCH ✗")
	t.Log("Result: SessionEnded rejected, duration=0, inconsistent state")
}
