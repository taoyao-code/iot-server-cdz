package bkv

import (
	"context"
	"testing"
)

type fakeRepoParam struct {
	ensured     int64
	logs        int
	lastCmd     int
	lastSuccess bool
}

func (f *fakeRepoParam) EnsureDevice(ctx context.Context, phyID string) (int64, error) {
	f.ensured = 1
	return 1, nil
}

func (f *fakeRepoParam) InsertCmdLog(ctx context.Context, deviceID int64, msgID int, cmd int, direction int16, payload []byte, success bool) error {
	f.logs++
	f.lastCmd = cmd
	f.lastSuccess = success
	return nil
}

func (f *fakeRepoParam) UpsertPortState(ctx context.Context, deviceID int64, portNo int, status int, powerW *int) error {
	return nil
}

func (f *fakeRepoParam) UpsertOrderProgress(ctx context.Context, deviceID int64, portNo int, orderHex string, durationSec int, kwh01 int, status int, powerW01 *int) error {
	return nil
}

func (f *fakeRepoParam) SettleOrder(ctx context.Context, deviceID int64, portNo int, orderHex string, durationSec int, kwh01 int, reason int) error {
	return nil
}

func (f *fakeRepoParam) AckOutboundByMsgID(ctx context.Context, deviceID int64, msgID int, ok bool, errCode *int) error {
	return nil
}

func TestHandleParam_ReadbackSuccess(t *testing.T) {
	fr := &fakeRepoParam{}
	h := &Handlers{Repo: fr}
	// 0x85 回读，有负载视为成功
	f := &Frame{Cmd: 0x85, Data: []byte{0x01}}
	if err := h.HandleParam(context.Background(), f); err != nil {
		t.Fatalf("err: %v", err)
	}
	if fr.logs == 0 || fr.lastCmd != int(0x85) || fr.lastSuccess != true {
		t.Fatalf("expected success log for 0x85, got logs=%d cmd=%d success=%v", fr.logs, fr.lastCmd, fr.lastSuccess)
	}
}

func TestHandleParam_ReadbackFailure(t *testing.T) {
	fr := &fakeRepoParam{}
	h := &Handlers{Repo: fr}
	// 0x85 回读，无负载视为失败
	f := &Frame{Cmd: 0x85, Data: []byte{}}
	if err := h.HandleParam(context.Background(), f); err != nil {
		t.Fatalf("err: %v", err)
	}
	if fr.logs == 0 || fr.lastCmd != int(0x85) || fr.lastSuccess != false {
		t.Fatalf("expected failure log for 0x85, got logs=%d cmd=%d success=%v", fr.logs, fr.lastCmd, fr.lastSuccess)
	}
}

func TestHandleControl_Log(t *testing.T) {
	fr := &fakeRepoParam{}
	h := &Handlers{Repo: fr}
	f := &Frame{Cmd: 0x90, Data: []byte{0x01, 0x02}}
	if err := h.HandleControl(context.Background(), f); err != nil {
		t.Fatalf("err: %v", err)
	}
	if fr.logs == 0 || fr.lastCmd != int(0x90) {
		t.Fatalf("expected control log, got logs=%d cmd=%d", fr.logs, fr.lastCmd)
	}
}
