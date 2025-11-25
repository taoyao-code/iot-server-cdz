package app

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/taoyao-code/iot-server/internal/coremodel"
	"github.com/taoyao-code/iot-server/internal/storage"
	"github.com/taoyao-code/iot-server/internal/storage/models"
)

// fullStubCoreRepo 完整的模拟存储库，用于端到端测试
// 简化版：移除订单相关逻辑，专注于端口状态管理
type fullStubCoreRepo struct {
	mu          sync.Mutex
	devices     map[string]*models.Device
	ports       map[string]*portState
	nextID      int64
	upsertCount int
}

type portState struct {
	deviceID  int64
	portNo    int32
	status    int32
	powerW    *int32
	updatedAt time.Time
}

func newFullStubCoreRepo() *fullStubCoreRepo {
	return &fullStubCoreRepo{
		devices: make(map[string]*models.Device),
		ports:   make(map[string]*portState),
		nextID:  1,
	}
}

func (r *fullStubCoreRepo) portKey(deviceID int64, portNo int32) string {
	return fmt.Sprintf("%d:%d", deviceID, portNo)
}

func (r *fullStubCoreRepo) WithTx(ctx context.Context, fn func(storage.CoreRepo) error) error {
	return fn(r)
}

func (r *fullStubCoreRepo) EnsureDevice(ctx context.Context, phyID string) (*models.Device, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if dev, ok := r.devices[phyID]; ok {
		return dev, nil
	}
	dev := &models.Device{ID: r.nextID, PhyID: phyID}
	r.devices[phyID] = dev
	r.nextID++
	return dev, nil
}

func (r *fullStubCoreRepo) TouchDeviceLastSeen(ctx context.Context, phyID string, at time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if dev, ok := r.devices[phyID]; ok {
		dev.LastSeenAt = &at
	}
	return nil
}

func (r *fullStubCoreRepo) GetDeviceByPhyID(ctx context.Context, phyID string) (*models.Device, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if dev, ok := r.devices[phyID]; ok {
		return dev, nil
	}
	return nil, errors.New("not found")
}

func (r *fullStubCoreRepo) ListDevices(ctx context.Context, limit, offset int) ([]models.Device, error) {
	return nil, errors.New("not implemented")
}

func (r *fullStubCoreRepo) UpsertGatewaySocket(ctx context.Context, socket *models.GatewaySocket) error {
	return nil
}

func (r *fullStubCoreRepo) GetGatewaySocketByUID(ctx context.Context, uid string) (*models.GatewaySocket, error) {
	return nil, errors.New("not implemented")
}

func (r *fullStubCoreRepo) UpsertPortSnapshot(ctx context.Context, deviceID int64, portNo int32, status int32, powerW *int32, updatedAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ports[r.portKey(deviceID, portNo)] = &portState{
		deviceID:  deviceID,
		portNo:    portNo,
		status:    status,
		powerW:    powerW,
		updatedAt: updatedAt,
	}
	r.upsertCount++
	return nil
}

func (r *fullStubCoreRepo) GetPort(ctx context.Context, deviceID int64, portNo int32) (*models.Port, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p, ok := r.ports[r.portKey(deviceID, portNo)]; ok {
		return &models.Port{DeviceID: deviceID, PortNo: portNo, Status: p.status, PowerW: p.powerW}, nil
	}
	return nil, errors.New("not found")
}

func (r *fullStubCoreRepo) UpdatePortStatus(ctx context.Context, deviceID int64, portNo int32, status int32) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p, ok := r.ports[r.portKey(deviceID, portNo)]; ok {
		p.status = status
	}
	return nil
}

func (r *fullStubCoreRepo) LockOrCreatePort(ctx context.Context, deviceID int64, portNo int32) (*models.Port, error) {
	return nil, errors.New("not implemented")
}

// 订单相关方法保留接口，返回空实现
func (r *fullStubCoreRepo) CreateOrder(ctx context.Context, order *models.Order) error {
	return nil
}

func (r *fullStubCoreRepo) GetActiveOrder(ctx context.Context, deviceID int64, portNo int32) (*models.Order, error) {
	return nil, errors.New("not implemented")
}

func (r *fullStubCoreRepo) GetOrderByOrderNo(ctx context.Context, orderNo string) (*models.Order, error) {
	return nil, errors.New("not implemented")
}

func (r *fullStubCoreRepo) GetOrderByBusinessNo(ctx context.Context, deviceID int64, businessNo int32) (*models.Order, error) {
	return nil, errors.New("not implemented")
}

func (r *fullStubCoreRepo) LockActiveOrderForPort(ctx context.Context, deviceID int64, portNo int32) (*models.Order, bool, error) {
	return nil, false, errors.New("not implemented")
}

func (r *fullStubCoreRepo) UpdateOrderStatus(ctx context.Context, orderID int64, status int32) error {
	return nil
}

func (r *fullStubCoreRepo) CompleteOrder(ctx context.Context, deviceID int64, portNo int32, endReason int32, endTime time.Time, amountCent *int64, kwh0p01 *int64) error {
	return nil
}

func (r *fullStubCoreRepo) SettleOrder(ctx context.Context, deviceID int64, portNo int, orderHex string, durationSec int, kwh0p01 int, reason int) error {
	return nil
}

func (r *fullStubCoreRepo) UpsertOrderProgress(ctx context.Context, deviceID int64, portNo int32, orderNo string, businessNo *int32, durationSec int32, kwh0p01 int32, status int32, powerW01 *int32) error {
	return nil
}

func (r *fullStubCoreRepo) CleanupPendingOrders(ctx context.Context, deviceID int64, before time.Time) (int64, error) {
	return 0, nil
}

func (r *fullStubCoreRepo) AppendCmdLog(ctx context.Context, log *models.CmdLog) error {
	return nil
}

func (r *fullStubCoreRepo) ListRecentCmdLogs(ctx context.Context, deviceID int64, limit int) ([]models.CmdLog, error) {
	return nil, nil
}

func (r *fullStubCoreRepo) EnqueueOutbound(ctx context.Context, msg *models.OutboundMessage) (int64, error) {
	return 0, nil
}

func (r *fullStubCoreRepo) DequeuePendingForDevice(ctx context.Context, deviceID int64, limit int) ([]models.OutboundMessage, error) {
	return nil, nil
}

func (r *fullStubCoreRepo) MarkOutboundSent(ctx context.Context, id int64) error {
	return nil
}

func (r *fullStubCoreRepo) MarkOutboundDone(ctx context.Context, id int64) error {
	return nil
}

func (r *fullStubCoreRepo) MarkOutboundFailed(ctx context.Context, id int64, lastError string) error {
	return nil
}

// getPortStatus 获取端口状态
func (r *fullStubCoreRepo) getPortStatus(deviceID int64, portNo int32) (int32, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p, ok := r.ports[r.portKey(deviceID, portNo)]; ok {
		return p.status, true
	}
	return 0, false
}

// TestChargingFlow_Complete 测试完整的充电流程（简化版）
// 流程：心跳 -> 充电开始 -> 充电进行中(多次) -> 充电结束
// 验证：仅端口状态正确更新
func TestChargingFlow_Complete(t *testing.T) {
	repo := newFullStubCoreRepo()
	driver := NewDriverCore(repo, nil, zap.NewNop())
	ctx := context.Background()

	devicePhyID := "TEST-BKV-DEVICE"
	var portNo coremodel.PortNo = 0
	businessNo := coremodel.BusinessNo("10C3") // 4291

	// 步骤1: 设备心跳（设备上线）
	t.Run("Step1_DeviceHeartbeat", func(t *testing.T) {
		ev := &coremodel.CoreEvent{
			Type:     coremodel.EventDeviceHeartbeat,
			DeviceID: coremodel.DeviceID(devicePhyID),
			DeviceHeartbeat: &coremodel.DeviceHeartbeatPayload{
				DeviceID:   coremodel.DeviceID(devicePhyID),
				LastSeenAt: time.Now(),
			},
		}
		err := driver.HandleCoreEvent(ctx, ev)
		require.NoError(t, err)

		device, err := repo.GetDeviceByPhyID(ctx, devicePhyID)
		require.NoError(t, err)
		assert.NotNil(t, device)
		assert.Equal(t, devicePhyID, device.PhyID)
	})

	// 步骤2: 充电开始
	t.Run("Step2_SessionStarted", func(t *testing.T) {
		chargingStatus := int32(0xA0) // bit7(在线) + bit5(充电)
		ev := &coremodel.CoreEvent{
			Type:       coremodel.EventSessionStarted,
			DeviceID:   coremodel.DeviceID(devicePhyID),
			PortNo:     &portNo,
			BusinessNo: &businessNo,
			SessionStarted: &coremodel.SessionStartedPayload{
				DeviceID:   coremodel.DeviceID(devicePhyID),
				PortNo:     portNo,
				BusinessNo: businessNo,
				StartedAt:  time.Now(),
				Metadata:   map[string]string{"raw_status": fmt.Sprintf("%d", chargingStatus)},
			},
		}
		err := driver.HandleCoreEvent(ctx, ev)
		require.NoError(t, err)

		// 验证端口状态已更新为充电中
		device, _ := repo.GetDeviceByPhyID(ctx, devicePhyID)
		portStatus, ok := repo.getPortStatus(device.ID, int32(portNo))
		require.True(t, ok, "端口状态应已创建")
		assert.Equal(t, chargingStatus, portStatus, "端口状态应为充电中(0xA0)")
	})

	// 步骤3: 充电进行中（模拟多次状态上报）
	t.Run("Step3_SessionProgress", func(t *testing.T) {
		device, _ := repo.GetDeviceByPhyID(ctx, devicePhyID)

		for i := 1; i <= 3; i++ {
			chargingStatus := int32(0xA0) // 仍在充电
			duration := int32(i * 60)     // 每次增加60秒
			energy := int32(i * 10)       // 每次增加0.1kWh
			power := int32(1000)          // 100W

			ev := &coremodel.CoreEvent{
				Type:       coremodel.EventSessionProgress,
				DeviceID:   coremodel.DeviceID(devicePhyID),
				PortNo:     &portNo,
				BusinessNo: &businessNo,
				SessionProgress: &coremodel.SessionProgressPayload{
					DeviceID:    coremodel.DeviceID(devicePhyID),
					PortNo:      portNo,
					BusinessNo:  businessNo,
					DurationSec: &duration,
					EnergyKWh01: &energy,
					PowerW:      &power,
					RawStatus:   &chargingStatus,
					OccurredAt:  time.Now(),
				},
			}
			err := driver.HandleCoreEvent(ctx, ev)
			require.NoError(t, err, "第%d次进度上报应成功", i)

			// 验证端口状态仍为充电中
			portStatus, ok := repo.getPortStatus(device.ID, int32(portNo))
			require.True(t, ok)
			assert.Equal(t, chargingStatus, portStatus, "第%d次上报后端口状态应仍为充电中", i)
		}
	})

	// 步骤4: 充电结束
	t.Run("Step4_SessionEnded", func(t *testing.T) {
		device, _ := repo.GetDeviceByPhyID(ctx, devicePhyID)

		idleStatus := int32(0x90) // bit7(在线) + bit4(空载) = 空闲
		rawReason := int32(0)     // 正常结束
		duration := int32(300)    // 5分钟
		energy := int32(50)       // 0.5kWh

		ev := &coremodel.CoreEvent{
			Type:       coremodel.EventSessionEnded,
			DeviceID:   coremodel.DeviceID(devicePhyID),
			PortNo:     &portNo,
			BusinessNo: &businessNo,
			SessionEnded: &coremodel.SessionEndedPayload{
				DeviceID:       coremodel.DeviceID(devicePhyID),
				PortNo:         portNo,
				BusinessNo:     businessNo,
				DurationSec:    duration,
				EnergyKWh01:    energy,
				RawReason:      &rawReason,
				NextPortStatus: &idleStatus,
				OccurredAt:     time.Now(),
			},
		}
		err := driver.HandleCoreEvent(ctx, ev)
		require.NoError(t, err)

		// 验证端口状态已变为空闲
		portStatus, ok := repo.getPortStatus(device.ID, int32(portNo))
		require.True(t, ok)
		assert.Equal(t, idleStatus, portStatus, "充电结束后端口状态应为空闲(0x90)")
	})
}

// TestChargingFlow_ProgressShouldNotTriggerEnd 测试充电进行中状态正确更新
func TestChargingFlow_ProgressShouldNotTriggerEnd(t *testing.T) {
	repo := newFullStubCoreRepo()
	driver := NewDriverCore(repo, nil, zap.NewNop())
	ctx := context.Background()

	devicePhyID := "TEST-BKV-PROGRESS"
	var portNo coremodel.PortNo = 0
	businessNo := coremodel.BusinessNo("002B") // 43

	// 先创建设备和初始充电状态
	device, err := repo.EnsureDevice(ctx, devicePhyID)
	require.NoError(t, err)

	// 模拟充电进行中的状态上报（状态位bit5=1表示充电中）
	for i := 1; i <= 5; i++ {
		chargingStatus := int32(0xB0) // bit7(在线) + bit5(充电) + bit4(空载) - 这是日志中的状态
		duration := int32(i * 60)
		energy := int32(i * 10)
		power := int32(1000)

		ev := &coremodel.CoreEvent{
			Type:       coremodel.EventSessionProgress,
			DeviceID:   coremodel.DeviceID(devicePhyID),
			PortNo:     &portNo,
			BusinessNo: &businessNo,
			SessionProgress: &coremodel.SessionProgressPayload{
				DeviceID:    coremodel.DeviceID(devicePhyID),
				PortNo:      portNo,
				BusinessNo:  businessNo,
				DurationSec: &duration,
				EnergyKWh01: &energy,
				PowerW:      &power,
				RawStatus:   &chargingStatus,
				OccurredAt:  time.Now(),
			},
		}
		err := driver.HandleCoreEvent(ctx, ev)
		require.NoError(t, err, "第%d次进度上报应成功", i)

		// 验证端口状态保持充电中
		portStatus, ok := repo.getPortStatus(device.ID, int32(portNo))
		require.True(t, ok)
		assert.True(t, (portStatus&0x20) != 0, "第%d次上报后端口状态的bit5应为1(充电中)", i)
	}
}

// TestChargingFlow_StatusBit5Determines 测试端口状态正确反映事件中的状态
func TestChargingFlow_StatusBit5Determines(t *testing.T) {
	tests := []struct {
		name       string
		status     int32
		isCharging bool
	}{
		{"0xB0-充电中", 0xB0, true},
		{"0xA0-充电中", 0xA0, true},
		{"0x90-空闲", 0x90, false},
		{"0x80-仅在线", 0x80, false},
		{"0x30-充电中", 0x30, true},
		{"0x10-空闲", 0x10, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFullStubCoreRepo()
			driver := NewDriverCore(repo, nil, zap.NewNop())
			ctx := context.Background()

			devicePhyID := fmt.Sprintf("TEST-STATUS-%X", tt.status)
			var portNo coremodel.PortNo = 0
			businessNo := coremodel.BusinessNo("0001")

			// 使用 SessionEnded 事件测试
			rawReason := int32(0)
			ev := &coremodel.CoreEvent{
				Type:       coremodel.EventSessionEnded,
				DeviceID:   coremodel.DeviceID(devicePhyID),
				PortNo:     &portNo,
				BusinessNo: &businessNo,
				SessionEnded: &coremodel.SessionEndedPayload{
					DeviceID:       coremodel.DeviceID(devicePhyID),
					PortNo:         portNo,
					BusinessNo:     businessNo,
					DurationSec:    60,
					EnergyKWh01:    10,
					RawReason:      &rawReason,
					NextPortStatus: &tt.status,
				},
			}
			err := driver.HandleCoreEvent(ctx, ev)
			require.NoError(t, err)

			// 验证端口状态
			device, _ := repo.GetDeviceByPhyID(ctx, devicePhyID)
			portStatus, ok := repo.getPortStatus(device.ID, int32(portNo))
			require.True(t, ok)
			assert.Equal(t, tt.status, portStatus, "端口状态应与事件中的状态一致")
		})
	}
}

// TestPortSnapshot_UpdatesStatus 测试 PortSnapshot 事件更新端口状态
func TestPortSnapshot_UpdatesStatus(t *testing.T) {
	repo := newFullStubCoreRepo()
	driver := NewDriverCore(repo, nil, zap.NewNop())
	ctx := context.Background()

	devicePhyID := "TEST-SNAPSHOT"
	var portNo coremodel.PortNo = 1

	testCases := []struct {
		status int32
		power  int32
	}{
		{0xA0, 1000}, // 充电中，100W
		{0xB0, 500},  // 充电中，50W
		{0x90, 0},    // 空闲
	}

	for i, tc := range testCases {
		power := tc.power
		ev := &coremodel.CoreEvent{
			Type:     coremodel.EventPortSnapshot,
			DeviceID: coremodel.DeviceID(devicePhyID),
			PortSnapshot: &coremodel.PortSnapshot{
				DeviceID:  coremodel.DeviceID(devicePhyID),
				PortNo:    portNo,
				RawStatus: tc.status,
				PowerW:    &power,
				At:        time.Now(),
			},
		}
		err := driver.HandleCoreEvent(ctx, ev)
		require.NoError(t, err, "第%d次快照更新应成功", i+1)

		device, _ := repo.GetDeviceByPhyID(ctx, devicePhyID)
		portStatus, ok := repo.getPortStatus(device.ID, int32(portNo))
		require.True(t, ok)
		assert.Equal(t, tc.status, portStatus, "第%d次快照后状态应正确", i+1)
	}
}

// TestSessionEnded_PersistsPortStatus 测试充电结束事件正确持久化端口状态
func TestSessionEnded_PersistsPortStatus(t *testing.T) {
	repo := newFullStubCoreRepo()
	driver := NewDriverCore(repo, nil, zap.NewNop())
	ctx := context.Background()

	devicePhyID := "TEST-SESSION-END"
	var portNo coremodel.PortNo = 0
	businessNo := coremodel.BusinessNo("ABCD")

	idleStatus := int32(0x90)
	rawReason := int32(8) // 空载结束

	ev := &coremodel.CoreEvent{
		Type:       coremodel.EventSessionEnded,
		DeviceID:   coremodel.DeviceID(devicePhyID),
		PortNo:     &portNo,
		BusinessNo: &businessNo,
		SessionEnded: &coremodel.SessionEndedPayload{
			DeviceID:       coremodel.DeviceID(devicePhyID),
			PortNo:         portNo,
			BusinessNo:     businessNo,
			DurationSec:    180,
			EnergyKWh01:    30,
			RawReason:      &rawReason,
			NextPortStatus: &idleStatus,
		},
	}

	err := driver.HandleCoreEvent(ctx, ev)
	require.NoError(t, err)

	// 验证端口状态已持久化
	device, _ := repo.GetDeviceByPhyID(ctx, devicePhyID)
	portStatus, ok := repo.getPortStatus(device.ID, int32(portNo))
	require.True(t, ok, "端口状态应已持久化")
	assert.Equal(t, idleStatus, portStatus, "端口状态应为空闲")
	assert.Equal(t, 1, repo.upsertCount, "端口快照应被写入一次")
}
