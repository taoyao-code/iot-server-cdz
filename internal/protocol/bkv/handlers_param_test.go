package bkv

import (
	"context"
	"testing"
	"time"

	pgstorage "github.com/taoyao-code/iot-server/internal/storage/pg"
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

func (f *fakeRepoParam) StoreParamWrite(ctx context.Context, deviceID int64, paramID int, value []byte, msgID int) error {
	return nil
}

func (f *fakeRepoParam) GetParamWritePending(ctx context.Context, deviceID int64, paramID int) ([]byte, int, error) {
	// 简单的模拟实现：返回固定的测试值
	return []byte{0x01, 0x02}, 123, nil
}

// P0修复: 新增参数确认和失败方法（测试mock）
func (f *fakeRepoParam) ConfirmParamWrite(ctx context.Context, deviceID int64, paramID int, msgID int) error {
	f.logs++
	return nil
}

func (f *fakeRepoParam) FailParamWrite(ctx context.Context, deviceID int64, paramID int, msgID int, errMsg string) error {
	f.logs++
	return nil
}

// P0修复: 订单状态管理方法
func (f *fakeRepoParam) GetPendingOrderByPort(ctx context.Context, deviceID int64, portNo int) (*pgstorage.Order, error) {
	return nil, nil
}

func (f *fakeRepoParam) UpdateOrderToCharging(ctx context.Context, orderNo string, startTime time.Time) error {
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
	f := &Frame{
		Cmd:       0x85,
		Direction: 0x01, // 上行回读
		Data:      []byte{},
	}
	if err := h.HandleParam(context.Background(), f); err != nil {
		t.Fatalf("err: %v", err)
	}
	if fr.logs == 0 || fr.lastCmd != int(0x85) || fr.lastSuccess != false {
		t.Fatalf("expected failure log for 0x85, got logs=%d cmd=%d success=%v", fr.logs, fr.lastCmd, fr.lastSuccess)
	}
}

func TestHandleParam_WriteAndReadback(t *testing.T) {
	fr := &fakeRepoParam{}
	h := &Handlers{Repo: fr}

	// 测试参数写入（下行）
	writeFrame := &Frame{
		Cmd:       0x83,
		MsgID:     456,
		Direction: 0x00,                           // 下行
		Data:      []byte{0x01, 0x02, 0x01, 0x02}, // paramID=1, len=2, value=[0x01,0x02]
	}

	if err := h.HandleParam(context.Background(), writeFrame); err != nil {
		t.Fatalf("write err: %v", err)
	}

	// 测试参数回读（上行）匹配的情况
	readbackFrame := &Frame{
		Cmd:       0x85,
		MsgID:     789,
		Direction: 0x01,                           // 上行
		Data:      []byte{0x01, 0x02, 0x01, 0x02}, // paramID=1, len=2, value=[0x01,0x02] (匹配)
	}

	if err := h.HandleParam(context.Background(), readbackFrame); err != nil {
		t.Fatalf("readback err: %v", err)
	}

	// 应该有写入和回读的日志
	if fr.logs < 2 {
		t.Fatalf("expected at least 2 logs, got %d", fr.logs)
	}

	// 最后一次应该是成功的回读
	if fr.lastCmd != 0x85 || fr.lastSuccess != true {
		t.Fatalf("expected successful readback, got cmd=%d success=%v", fr.lastCmd, fr.lastSuccess)
	}
}

func TestHandleParam_ReadbackMismatch(t *testing.T) {
	fr := &fakeRepoParam{}
	h := &Handlers{Repo: fr}

	// 测试参数回读（上行）不匹配的情况
	readbackFrame := &Frame{
		Cmd:       0x85,
		MsgID:     789,
		Direction: 0x01,                           // 上行
		Data:      []byte{0x01, 0x02, 0x03, 0x04}, // paramID=1, len=2, value=[0x03,0x04] (不匹配)
	}

	if err := h.HandleParam(context.Background(), readbackFrame); err != nil {
		t.Fatalf("readback err: %v", err)
	}

	// 应该是失败的回读
	if fr.lastCmd != 0x85 || fr.lastSuccess != false {
		t.Fatalf("expected failed readback, got cmd=%d success=%v", fr.lastCmd, fr.lastSuccess)
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

// Week 6: 组网管理方法（测试桩）
func (f *fakeRepoParam) UpsertGatewaySocket(ctx context.Context, socket *pgstorage.GatewaySocket) error {
	return nil
}

func (f *fakeRepoParam) DeleteGatewaySocket(ctx context.Context, gatewayID string, socketNo int) error {
	return nil
}

func (f *fakeRepoParam) GetGatewaySockets(ctx context.Context, gatewayID string) ([]pgstorage.GatewaySocket, error) {
	return nil, nil
}

// Week 7: OTA升级方法（测试桩）
func (f *fakeRepoParam) CreateOTATask(ctx context.Context, task *pgstorage.OTATask) (int64, error) {
	return 1, nil
}

func (f *fakeRepoParam) GetOTATask(ctx context.Context, taskID int64) (*pgstorage.OTATask, error) {
	return nil, nil
}

func (f *fakeRepoParam) UpdateOTATaskStatus(ctx context.Context, taskID int64, status int, errorMsg *string) error {
	return nil
}

func (f *fakeRepoParam) UpdateOTATaskProgress(ctx context.Context, taskID int64, progress int, status int) error {
	return nil
}

func (f *fakeRepoParam) GetDeviceOTATasks(ctx context.Context, deviceID int64, limit int) ([]pgstorage.OTATask, error) {
	return nil, nil
}
