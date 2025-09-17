package bkv

import (
	"context"
	"encoding/hex"
	"testing"
)

type fakeRepo struct {
	ensured    int64
	logs       int
	upserts    int
	lastPort   int
	lastStatus int
	devices    map[string]int64
	nextDevID  int64
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		devices:   make(map[string]int64),
		nextDevID: 1,
	}
}

func (f *fakeRepo) EnsureDevice(ctx context.Context, phyID string) (int64, error) {
	if devID, exists := f.devices[phyID]; exists {
		return devID, nil
	}
	devID := f.nextDevID
	f.nextDevID++
	f.devices[phyID] = devID
	f.ensured = devID
	return devID, nil
}

func (f *fakeRepo) InsertCmdLog(ctx context.Context, deviceID int64, msgID int, cmd int, direction int16, payload []byte, success bool) error {
	f.logs++
	return nil
}

func (f *fakeRepo) UpsertPortState(ctx context.Context, deviceID int64, portNo int, status int, powerW *int) error {
	f.upserts++
	f.lastPort = portNo
	f.lastStatus = status
	return nil
}

func (f *fakeRepo) SettleOrder(ctx context.Context, deviceID int64, portNo int, orderHex string, durationSec int, kwh01 int, reason int) error {
	f.logs++
	return nil
}

func (f *fakeRepo) AckOutboundByMsgID(ctx context.Context, deviceID int64, msgID int, ok bool, errCode *int) error {
	f.logs++
	return nil
}

func TestHandlers_Heartbeat(t *testing.T) {
	fr := newFakeRepo()
	h := &Handlers{Repo: fr}
	
	frame := &Frame{
		Cmd:       0x0000,
		MsgID:     123,
		Direction: 0x01,
		GatewayID: "82200520004869",
		Data:      []byte{0x01, 0x02},
	}
	
	if err := h.HandleHeartbeat(context.Background(), frame); err != nil {
		t.Fatalf("err: %v", err)
	}
	if fr.logs != 1 {
		t.Fatalf("expected 1 log, got %d", fr.logs)
	}
	if len(fr.devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(fr.devices))
	}
}

func TestHandlers_BKVStatus(t *testing.T) {
	fr := newFakeRepo()
	h := &Handlers{Repo: fr}
	
	// 构造BKV状态报文
	bkvData := "04010110170a0102000000000000000009010382231214002700650194"
	data, _ := hex.DecodeString(bkvData)
	
	frame := &Frame{
		Cmd:       0x1000,
		MsgID:     456,
		Direction: 0x01,
		GatewayID: "82231214002700",
		Data:      data,
	}
	
	if err := h.HandleBKVStatus(context.Background(), frame); err != nil {
		t.Fatalf("err: %v", err)
	}
	
	// 应该记录命令日志
	if fr.logs != 1 {
		t.Fatalf("expected 1 log, got %d", fr.logs)
	}
	
	// 应该更新端口状态 (两个端口)
	if fr.upserts != 2 {
		t.Fatalf("expected 2 port upserts, got %d", fr.upserts)
	}
}

func TestHandlers_Control(t *testing.T) {
	fr := newFakeRepo()
	h := &Handlers{Repo: fr}
	
	frame := &Frame{
		Cmd:       0x0015,
		MsgID:     789,
		Direction: 0x00, // 下行控制
		GatewayID: "82200520004869",
		Data:      []byte{0x02, 0x00, 0x01}, // 控制数据
	}
	
	if err := h.HandleControl(context.Background(), frame); err != nil {
		t.Fatalf("err: %v", err)
	}
	if fr.logs != 1 {
		t.Fatalf("expected 1 log, got %d", fr.logs)
	}
}

func TestHandlers_Generic(t *testing.T) {
	fr := newFakeRepo()
	h := &Handlers{Repo: fr}
	
	frame := &Frame{
		Cmd:       0x0005,
		MsgID:     999,
		Direction: 0x01,
		GatewayID: "82200520004869",
		Data:      []byte{0x08, 0x04},
	}
	
	if err := h.HandleGeneric(context.Background(), frame); err != nil {
		t.Fatalf("err: %v", err)
	}
	if fr.logs != 1 {
		t.Fatalf("expected 1 log, got %d", fr.logs)
	}
}

func TestRegisterHandlers(t *testing.T) {
	fr := newFakeRepo()
	adapter := NewBKVProtocol(fr, nil)
	
	// 测试心跳
	heartbeatFrame := BuildUplink(0x0000, 123, "82200520004869", []byte{0x01})
	err := adapter.ProcessBytes(heartbeatFrame)
	if err != nil {
		t.Fatalf("ProcessBytes failed: %v", err)
	}
	
	if fr.logs != 1 {
		t.Fatalf("expected 1 log after heartbeat, got %d", fr.logs)
	}
	
	// 测试控制指令
	controlFrame := Build(0x0015, 456, "82200520004869", []byte{0x02, 0x00, 0x01})
	err = adapter.ProcessBytes(controlFrame)
	if err != nil {
		t.Fatalf("ProcessBytes failed for control: %v", err)
	}
	
	if fr.logs != 2 {
		t.Fatalf("expected 2 logs after control, got %d", fr.logs)
	}
}

func TestIsBKVCommand(t *testing.T) {
	// 测试支持的命令
	supportedCmds := []uint16{0x0000, 0x1000, 0x0015, 0x0005, 0x0007}
	for _, cmd := range supportedCmds {
		if !IsBKVCommand(cmd) {
			t.Errorf("0x%04x should be a BKV command", cmd)
		}
	}
	
	// 测试不支持的命令
	if IsBKVCommand(0x9999) {
		t.Error("0x9999 should not be a BKV command")
	}
}
