package bkv

import (
	"context"
	"testing"
	"time"

	"github.com/taoyao-code/iot-server/internal/coremodel"
	"go.uber.org/zap/zaptest"
)

type mockOutboundSender struct {
	lastGateway string
	lastCmd     uint16
	lastMsgID   uint32
	lastData    []byte
	calls       int
	err         error
}

func (m *mockOutboundSender) SendDownlink(gatewayID string, cmd uint16, msgID uint32, data []byte) error {
	m.lastGateway = gatewayID
	m.lastCmd = cmd
	m.lastMsgID = msgID
	m.lastData = append([]byte(nil), data...)
	m.calls++
	return m.err
}

func TestCommandSource_StartCharge(t *testing.T) {
	sender := &mockOutboundSender{}
	cs := NewCommandSource(sender, zaptest.NewLogger(t))

	modeCode := int32(1)
	durationSec := int32(600)
	biz := coremodel.BusinessNo("4660")

	cmd := &coremodel.CoreCommand{
		Type:     coremodel.CommandStartCharge,
		DeviceID: coremodel.DeviceID("82200520004869"),
		PortNo:   0,
		BusinessNo: func() *coremodel.BusinessNo {
			return &biz
		}(),
		IssuedAt: time.Now(),
		StartCharge: &coremodel.StartChargePayload{
			Mode:              "duration",
			ModeCode:          &modeCode,
			TargetDurationSec: &durationSec,
		},
	}

	if err := cs.SendCoreCommand(context.Background(), cmd); err != nil {
		t.Fatalf("SendCoreCommand failed: %v", err)
	}
	if sender.calls != 1 {
		t.Fatalf("expected outbound call")
	}
	if sender.lastCmd != 0x0015 {
		t.Fatalf("unexpected command code: %x", sender.lastCmd)
	}
	if sender.lastGateway != "82200520004869" {
		t.Fatalf("unexpected gateway: %s", sender.lastGateway)
	}
	if len(sender.lastData) == 0 {
		t.Fatalf("expected payload data")
	}
}

func TestCommandSource_Unsupported(t *testing.T) {
	sender := &mockOutboundSender{}
	cs := NewCommandSource(sender, zaptest.NewLogger(t))

	cmd := &coremodel.CoreCommand{
		Type:     coremodel.CoreCommandType("Unknown"),
		DeviceID: "822312",
		IssuedAt: time.Now(),
	}

	if err := cs.SendCoreCommand(context.Background(), cmd); err == nil {
		t.Fatalf("expected error for unsupported command")
	}
}

func TestCommandSource_StopCharge(t *testing.T) {
	sender := &mockOutboundSender{}
	cs := NewCommandSource(sender, zaptest.NewLogger(t))

	biz := coremodel.BusinessNo("0x30")
	cmd := &coremodel.CoreCommand{
		Type:     coremodel.CommandStopCharge,
		DeviceID: coremodel.DeviceID("82200520004869"),
		PortNo:   1,
		BusinessNo: func() *coremodel.BusinessNo {
			return &biz
		}(),
		IssuedAt: time.Now(),
		StopCharge: &coremodel.StopChargePayload{
			Reason: "unit_test",
		},
	}

	if err := cs.SendCoreCommand(context.Background(), cmd); err != nil {
		t.Fatalf("SendCoreCommand failed: %v", err)
	}
	if sender.lastCmd != 0x0015 {
		t.Fatalf("unexpected command: %x", sender.lastCmd)
	}
	if sender.calls != 1 {
		t.Fatalf("expected outbound call")
	}
}

func TestCommandSource_QueryPortStatus(t *testing.T) {
	sender := &mockOutboundSender{}
	cs := NewCommandSource(sender, zaptest.NewLogger(t))

	socket := int32(2)
	cmd := &coremodel.CoreCommand{
		Type:     coremodel.CommandQueryPortStatus,
		DeviceID: coremodel.DeviceID("82200520004869"),
		IssuedAt: time.Now(),
		QueryPortStatus: &coremodel.QueryPortStatusPayload{
			SocketNo: &socket,
		},
	}

	if err := cs.SendCoreCommand(context.Background(), cmd); err != nil {
		t.Fatalf("SendCoreCommand failed: %v", err)
	}
	if sender.calls != 1 {
		t.Fatalf("expected outbound call")
	}
	if sender.lastCmd != 0x0015 {
		t.Fatalf("unexpected command: %x", sender.lastCmd)
	}
	if len(sender.lastData) != 4 || sender.lastData[2] != 0x1D || sender.lastData[3] != byte(socket) {
		t.Fatalf("unexpected payload: %v", sender.lastData)
	}
}
