package ap3000

import (
	"context"
	"encoding/hex"
	"testing"
)

type testRepo struct {
	ensureID       int64
	logs           int
	lastLogSuccess bool
	upserts        int
	lastPort       int
	lastStatus     int
	progressCnt    int
	settleCnt      int
	lastOrderHex   string
}

func (r *testRepo) EnsureDevice(ctx context.Context, phyID string) (int64, error) {
	return r.ensureID, nil
}

func (r *testRepo) InsertCmdLog(ctx context.Context, deviceID int64, msgID int, cmd int, direction int16, payload []byte, success bool) error {
	r.logs++
	r.lastLogSuccess = success
	return nil
}

func (r *testRepo) UpsertPortState(ctx context.Context, deviceID int64, portNo int, status int, powerW *int) error {
	r.upserts++
	r.lastPort = portNo
	r.lastStatus = status
	return nil
}

func (r *testRepo) UpsertOrderProgress(ctx context.Context, deviceID int64, portNo int, orderHex string, durationSec int, kwh01 int, status int, powerW01 *int) error {
	r.progressCnt++
	r.lastOrderHex = orderHex
	return nil
}

func (r *testRepo) SettleOrder(ctx context.Context, deviceID int64, portNo int, orderHex string, durationSec int, kwh01 int, reason int) error {
	r.settleCnt++
	r.lastOrderHex = orderHex
	return nil
}

func (r *testRepo) AckOutboundByMsgID(ctx context.Context, deviceID int64, msgID int, ok bool, errCode *int) error {
	return nil
}

func TestHandle03_Settlement(t *testing.T) {
	// Build minimal 0x03 payload:
	// dur(2)=300, maxP(2)=16, kwh(2)=123, port(1)=1, mode(1)=1, card(4)=0, reason(1)=1, order(16)=0x010203..10
	payload := []byte{0x2C, 0x01, 0x10, 0x00, 0x7B, 0x00, 0x01, 0x01, 0, 0, 0, 0, 0x01}
	ord := make([]byte, 16)
	for i := 0; i < 16; i++ {
		ord[i] = byte(i + 1)
	}
	payload = append(payload, ord...)

	repo := &testRepo{ensureID: 1}
	h := &Handlers{Repo: repo}
	f := &Frame{PhyID: "P1", MsgID: 1, Cmd: 0x03, Data: payload}
	if err := h.Handle03(context.Background(), f); err != nil {
		t.Fatalf("handle03 err: %v", err)
	}
	if repo.settleCnt != 1 {
		t.Fatalf("expected settleCnt=1 got %d", repo.settleCnt)
	}
	if hex.EncodeToString(ord) != repo.lastOrderHex {
		t.Fatalf("order hex mismatch")
	}
	if repo.logs == 0 || !repo.lastLogSuccess {
		t.Fatalf("cmd log not recorded or not success")
	}
}

func TestHandle06_Progress(t *testing.T) {
	// Build minimal 0x06 payload:
	// port(1)=2, status(1)=1, dur(2)=30, kwh(2)=50, mode(1)=1, pwr(2)=100, max/min/avg(6) skip, order(16)=0xAA.. (16 bytes)
	payload := []byte{0x02, 0x01, 0x1E, 0x00, 0x32, 0x00, 0x01, 0x64, 0x00}
	payload = append(payload, []byte{0, 0, 0, 0, 0, 0}...)
	ord := make([]byte, 16)
	for i := 0; i < 16; i++ {
		ord[i] = 0xAA
	}
	payload = append(payload, ord...)

	repo := &testRepo{ensureID: 2}
	h := &Handlers{Repo: repo}
	f := &Frame{PhyID: "P2", MsgID: 2, Cmd: 0x06, Data: payload}
	if err := h.Handle06(context.Background(), f); err != nil {
		t.Fatalf("handle06 err: %v", err)
	}
	if repo.progressCnt != 1 {
		t.Fatalf("expected progressCnt=1 got %d", repo.progressCnt)
	}
	if repo.upserts == 0 || repo.lastPort != 2 || repo.lastStatus != 1 {
		t.Fatalf("port state not updated correctly")
	}
	if repo.logs == 0 || !repo.lastLogSuccess {
		t.Fatalf("cmd log not recorded or not success")
	}
}
