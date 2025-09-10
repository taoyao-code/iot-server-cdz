package ap3000

import (
	"context"
	"testing"
)

type fakeRepo struct {
	ensureDeviceID int64
	error          error
	logs           int
}

func (f *fakeRepo) EnsureDevice(ctx context.Context, phyID string) (int64, error) {
	return f.ensureDeviceID, f.error
}

func (f *fakeRepo) InsertCmdLog(ctx context.Context, deviceID int64, msgID int, cmd int, direction int16, payload []byte, success bool) error {
	f.logs++
	return f.error
}

func TestHandlers_RegisterAndHeartbeat(t *testing.T) {
	fr := &fakeRepo{ensureDeviceID: 42}
	h := &Handlers{Repo: fr}
	f := &Frame{PhyID: "ABC", MsgID: 0x1234, Cmd: 0x20, Data: []byte{0x01}}
	if err := h.HandleRegister(context.Background(), f); err != nil {
		t.Fatalf("err: %v", err)
	}
	if fr.logs != 1 {
		t.Fatalf("expected 1 log, got %d", fr.logs)
	}
	f.Cmd = 0x21
	if err := h.HandleHeartbeat(context.Background(), f); err != nil {
		t.Fatalf("err: %v", err)
	}
	if fr.logs != 2 {
		t.Fatalf("expected 2 logs, got %d", fr.logs)
	}
}

func TestHandlers_Generic(t *testing.T) {
	fr := &fakeRepo{ensureDeviceID: 7}
	h := &Handlers{Repo: fr}
	f := &Frame{PhyID: "X", MsgID: 1, Cmd: 0x82}
	if err := h.HandleGeneric(context.Background(), f); err != nil {
		t.Fatalf("err: %v", err)
	}
	if fr.logs != 1 {
		t.Fatalf("expected 1 log, got %d", fr.logs)
	}
}
