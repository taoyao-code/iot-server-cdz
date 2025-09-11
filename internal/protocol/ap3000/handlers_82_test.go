package ap3000

import (
	"context"
	"testing"
)

type ackRepo struct {
	ensureID    int64
	ackCalled   int
	lastMsgID   int
	lastOK      bool
	lastErrCode *int
	logs        int
}

func (r *ackRepo) EnsureDevice(ctx context.Context, phyID string) (int64, error) {
	return r.ensureID, nil
}

func (r *ackRepo) InsertCmdLog(ctx context.Context, deviceID int64, msgID int, cmd int, direction int16, payload []byte, success bool) error {
	r.logs++
	return nil
}

func (r *ackRepo) UpsertPortState(ctx context.Context, deviceID int64, portNo int, status int, powerW *int) error {
	return nil
}

func (r *ackRepo) UpsertOrderProgress(ctx context.Context, deviceID int64, portNo int, orderHex string, durationSec int, kwh01 int, status int, powerW01 *int) error {
	return nil
}

func (r *ackRepo) SettleOrder(ctx context.Context, deviceID int64, portNo int, orderHex string, durationSec int, kwh01 int, reason int) error {
	return nil
}

func (r *ackRepo) AckOutboundByMsgID(ctx context.Context, deviceID int64, msgID int, ok bool, errCode *int) error {
	r.ackCalled++
	r.lastMsgID = msgID
	r.lastOK = ok
	r.lastErrCode = errCode
	return nil
}

func TestHandle82Ack_OK(t *testing.T) {
	repo := &ackRepo{ensureID: 10}
	h := &Handlers{Repo: repo}
	f := &Frame{PhyID: "PHY", MsgID: 0x1234, Cmd: 0x82, Data: []byte{0x00}}
	if err := h.Handle82Ack(context.Background(), f); err != nil {
		t.Fatalf("err: %v", err)
	}
	if repo.ackCalled != 1 || !repo.lastOK || repo.lastMsgID != 0x1234 || repo.lastErrCode != nil {
		t.Fatalf("ack not recorded as ok")
	}
	if repo.logs == 0 {
		t.Fatalf("cmd log not recorded")
	}
}

func TestHandle82Ack_Fail(t *testing.T) {
	repo := &ackRepo{ensureID: 11}
	h := &Handlers{Repo: repo}
	f := &Frame{PhyID: "PHY", MsgID: 0xABCD, Cmd: 0x82, Data: []byte{0x02}}
	if err := h.Handle82Ack(context.Background(), f); err != nil {
		t.Fatalf("err: %v", err)
	}
	if repo.ackCalled != 1 || repo.lastOK || repo.lastMsgID != 0xABCD || repo.lastErrCode == nil || *repo.lastErrCode != 2 {
		t.Fatalf("ack not recorded as fail")
	}
}
