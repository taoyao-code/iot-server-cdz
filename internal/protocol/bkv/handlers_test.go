package bkv

import (
	"context"
	"testing"
)

type fakeRepo struct {
	ensured    int64
	logs       int
	upserts    int
	lastPort   int
	lastStatus int
}

func (f *fakeRepo) EnsureDevice(ctx context.Context, phyID string) (int64, error) {
	f.ensured = 1
	return 1, nil
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

func TestHandlers_StatusParse(t *testing.T) {
	fr := &fakeRepo{}
	h := &Handlers{Repo: fr}
	f := &Frame{Cmd: 0x11, Data: []byte{2, 3}}
	if err := h.HandleStatus(context.Background(), f); err != nil {
		t.Fatalf("err: %v", err)
	}
	if fr.upserts != 1 || fr.lastPort != 2 || fr.lastStatus != 3 {
		t.Fatalf("upsert wrong: %v %v", fr.lastPort, fr.lastStatus)
	}
	if fr.logs != 1 {
		t.Fatalf("logs: %d", fr.logs)
	}
}
