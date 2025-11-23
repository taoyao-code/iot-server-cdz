package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/taoyao-code/iot-server/internal/coremodel"
	"github.com/taoyao-code/iot-server/internal/storage"
	"github.com/taoyao-code/iot-server/internal/storage/models"
)

type fakeCoreRepo struct {
	devices       map[string]*models.Device
	nextID        int64
	portSnapshots []portSnapshotRecord
	orders        map[string]*orderProgressRecord
	settled       []settleRecord
}

type portSnapshotRecord struct {
	deviceID int64
	portNo   int32
	status   int32
	powerW   *int32
}

type orderProgressRecord struct {
	orderNo    string
	deviceID   int64
	portNo     int32
	biz        *int32
	status     int32
	energyKWh  int32
	durationS  int32
	powerW01   *int32
	lastUpdate time.Time
}

type settleRecord struct {
	deviceID int64
	portNo   int
	orderHex string
	duration int
	kwh      int
	reason   int
}

func newFakeCoreRepo() *fakeCoreRepo {
	return &fakeCoreRepo{
		devices: make(map[string]*models.Device),
		nextID:  1,
		orders:  make(map[string]*orderProgressRecord),
	}
}

func (f *fakeCoreRepo) WithTx(ctx context.Context, fn func(repo storage.CoreRepo) error) error {
	return fn(f)
}

func (f *fakeCoreRepo) EnsureDevice(ctx context.Context, phyID string) (*models.Device, error) {
	if dev, ok := f.devices[phyID]; ok {
		return dev, nil
	}
	dev := &models.Device{
		ID:    f.nextID,
		PhyID: phyID,
	}
	f.nextID++
	f.devices[phyID] = dev
	return dev, nil
}

func (f *fakeCoreRepo) TouchDeviceLastSeen(ctx context.Context, phyID string, at time.Time) error {
	_, _ = f.EnsureDevice(ctx, phyID)
	return nil
}

func (f *fakeCoreRepo) GetDeviceByPhyID(ctx context.Context, phyID string) (*models.Device, error) {
	if dev, ok := f.devices[phyID]; ok {
		return dev, nil
	}
	return nil, nil
}

func (f *fakeCoreRepo) ListDevices(ctx context.Context, limit, offset int) ([]models.Device, error) {
	res := make([]models.Device, 0, len(f.devices))
	for _, d := range f.devices {
		res = append(res, *d)
	}
	return res, nil
}

func (f *fakeCoreRepo) UpsertPortSnapshot(ctx context.Context, deviceID int64, portNo int32, status int32, powerW *int32, updatedAt time.Time) error {
	f.portSnapshots = append(f.portSnapshots, portSnapshotRecord{
		deviceID: deviceID,
		portNo:   portNo,
		status:   status,
		powerW:   powerW,
	})
	return nil
}

func (f *fakeCoreRepo) GetPort(ctx context.Context, deviceID int64, portNo int32) (*models.Port, error) {
	return nil, nil
}

func (f *fakeCoreRepo) UpdatePortStatus(ctx context.Context, deviceID int64, portNo int32, status int32) error {
	return nil
}

func (f *fakeCoreRepo) CreateOrder(ctx context.Context, order *models.Order) error { return nil }

func (f *fakeCoreRepo) GetActiveOrder(ctx context.Context, deviceID int64, portNo int32) (*models.Order, error) {
	return nil, nil
}

func (f *fakeCoreRepo) GetOrderByOrderNo(ctx context.Context, orderNo string) (*models.Order, error) {
	return nil, nil
}

func (f *fakeCoreRepo) GetOrderByBusinessNo(ctx context.Context, deviceID int64, businessNo int32) (*models.Order, error) {
	for _, ord := range f.orders {
		if ord.biz != nil && *ord.biz == businessNo && ord.deviceID == deviceID {
			return &models.Order{OrderNo: ord.orderNo, DeviceID: ord.deviceID, PortNo: ord.portNo}, nil
		}
	}
	return nil, nil
}

func (f *fakeCoreRepo) UpdateOrderStatus(ctx context.Context, orderID int64, status int32) error {
	return nil
}

func (f *fakeCoreRepo) CompleteOrder(ctx context.Context, deviceID int64, portNo int32, endReason int32, endTime time.Time, amountCent *int64, kwh0p01 *int64) error {
	return nil
}

func (f *fakeCoreRepo) SettleOrder(ctx context.Context, deviceID int64, portNo int, orderHex string, durationSec int, kwh0p01 int, reason int) error {
	f.settled = append(f.settled, settleRecord{
		deviceID: deviceID,
		portNo:   portNo,
		orderHex: orderHex,
		duration: durationSec,
		kwh:      kwh0p01,
		reason:   reason,
	})
	return nil
}

func (f *fakeCoreRepo) UpsertOrderProgress(ctx context.Context, deviceID int64, portNo int32, orderNo string, businessNo *int32, durationSec int32, kwh0p01 int32, status int32, powerW01 *int32) error {
	rec, ok := f.orders[orderNo]
	if !ok {
		rec = &orderProgressRecord{
			orderNo:  orderNo,
			deviceID: deviceID,
			portNo:   portNo,
		}
	}
	rec.status = status
	rec.durationS = durationSec
	rec.energyKWh = kwh0p01
	rec.powerW01 = powerW01
	rec.biz = businessNo
	rec.lastUpdate = time.Now()
	f.orders[orderNo] = rec
	return nil
}

func (f *fakeCoreRepo) AppendCmdLog(ctx context.Context, log *models.CmdLog) error { return nil }
func (f *fakeCoreRepo) ListRecentCmdLogs(ctx context.Context, deviceID int64, limit int) ([]models.CmdLog, error) {
	return nil, nil
}
func (f *fakeCoreRepo) EnqueueOutbound(ctx context.Context, msg *models.OutboundMessage) (int64, error) {
	return 0, nil
}
func (f *fakeCoreRepo) DequeuePendingForDevice(ctx context.Context, deviceID int64, limit int) ([]models.OutboundMessage, error) {
	return nil, nil
}
func (f *fakeCoreRepo) MarkOutboundSent(ctx context.Context, id int64) error { return nil }
func (f *fakeCoreRepo) MarkOutboundDone(ctx context.Context, id int64) error { return nil }
func (f *fakeCoreRepo) MarkOutboundFailed(ctx context.Context, id int64, lastError string) error {
	return nil
}

func TestDriverCoreSessionLifecycle(t *testing.T) {
	repo := newFakeCoreRepo()
	billingCalled := 0
	core := NewDriverCore(repo, nil, &BillingHookFunc{
		OnStart: func(ctx context.Context, biz string, port int32, cardNo *string) error {
			billingCalled++
			return nil
		},
		OnEnd: func(ctx context.Context, biz string, port int32, amountCent *int64, energyKwh01 int32, durationSec int32) error {
			billingCalled++
			return nil
		},
	}, zaptest.NewLogger(t))

	biz := coremodel.BusinessNo("0x10")
	portNo := coremodel.PortNo(1)

	start := &coremodel.CoreEvent{
		Type:       coremodel.EventSessionStarted,
		DeviceID:   "dev-001",
		PortNo:     &portNo,
		BusinessNo: &biz,
		SessionStarted: &coremodel.SessionStartedPayload{
			DeviceID:     "dev-001",
			PortNo:       1,
			BusinessNo:   "0x10",
			StartedAt:    time.Now(),
			TargetPowerW: func() *int32 { v := int32(1500); return &v }(),
		},
	}
	require.NoError(t, core.HandleCoreEvent(context.Background(), start))

	require.Len(t, repo.orders, 1)
	order := repo.orders["0010"]
	require.NotNil(t, order)
	assert.Equal(t, int32(2), order.status)
	assert.Equal(t, int32(1500), derefInt32(order.powerW01))
	assert.Equal(t, 1, billingCalled)

	progressPower := int32(1200)
	progress := &coremodel.CoreEvent{
		Type:       coremodel.EventSessionProgress,
		DeviceID:   "dev-001",
		PortNo:     &portNo,
		BusinessNo: &biz,
		SessionProgress: &coremodel.SessionProgressPayload{
			DeviceID:    "dev-001",
			PortNo:      1,
			BusinessNo:  "0x10",
			DurationSec: func() *int32 { v := int32(300); return &v }(),
			EnergyKWh01: func() *int32 { v := int32(123); return &v }(),
			PowerW:      &progressPower,
			RawStatus:   func() *int32 { v := int32(0x85); return &v }(),
			OccurredAt:  time.Now(),
		},
	}
	require.NoError(t, core.HandleCoreEvent(context.Background(), progress))

	require.Equal(t, int32(123), repo.orders["0010"].energyKWh)
	require.Equal(t, int32(300), repo.orders["0010"].durationS)
	require.Equal(t, int32(0x85), repo.portSnapshots[len(repo.portSnapshots)-1].status)

	endStatus := int32(0x09)
	end := &coremodel.CoreEvent{
		Type:       coremodel.EventSessionEnded,
		DeviceID:   "dev-001",
		PortNo:     &portNo,
		BusinessNo: &biz,
		SessionEnded: &coremodel.SessionEndedPayload{
			DeviceID:       "dev-001",
			PortNo:         1,
			BusinessNo:     "0x10",
			EnergyKWh01:    200,
			DurationSec:    600,
			RawReason:      func() *int32 { v := int32(3); return &v }(),
			NextPortStatus: &endStatus,
			OccurredAt:     time.Now(),
		},
	}
	require.NoError(t, core.HandleCoreEvent(context.Background(), end))

	assert.Len(t, repo.settled, 1)
	assert.Equal(t, "0010", repo.settled[0].orderHex)
	assert.Equal(t, 600, repo.settled[0].duration)
	assert.Equal(t, 2, billingCalled)

	lastSnap := repo.portSnapshots[len(repo.portSnapshots)-1]
	assert.Equal(t, int32(0x09), lastSnap.status)
}

func TestDriverCoreExceptionSnapshot(t *testing.T) {
	repo := newFakeCoreRepo()
	core := NewDriverCore(repo, nil, nil, zaptest.NewLogger(t))

	portNo := coremodel.PortNo(0)
	raw := int32(0x11)
	ev := &coremodel.CoreEvent{
		Type:     coremodel.EventExceptionReported,
		DeviceID: "dev-002",
		PortNo:   &portNo,
		Exception: &coremodel.ExceptionPayload{
			DeviceID:   "dev-002",
			PortNo:     &portNo,
			Code:       "crc_error",
			Message:    "crc failed",
			Severity:   "warn",
			RawStatus:  &raw,
			OccurredAt: time.Now(),
		},
	}

	require.NoError(t, core.HandleCoreEvent(context.Background(), ev))
	require.Len(t, repo.portSnapshots, 1)
	assert.Equal(t, raw, repo.portSnapshots[0].status)
}

func derefInt32(v *int32) int32 {
	if v == nil {
		return 0
	}
	return *v
}
