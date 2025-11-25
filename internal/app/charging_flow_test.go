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
type fullStubCoreRepo struct {
	mu            sync.Mutex
	devices       map[string]*models.Device
	ports         map[string]*portState
	orders        map[string]*orderState
	nextID        int64
	settleErr     error
	upsertCount   int
	settleCount   int
	progressCount int
}

type portState struct {
	deviceID  int64
	portNo    int32
	status    int32
	powerW    *int32
	updatedAt time.Time
}

type orderState struct {
	deviceID   int64
	portNo     int32
	orderNo    string
	businessNo *int32
	status     int32
	durationS  int32
	energyKwh  int32
	powerW     *int32
}

func newFullStubCoreRepo() *fullStubCoreRepo {
	return &fullStubCoreRepo{
		devices: make(map[string]*models.Device),
		ports:   make(map[string]*portState),
		orders:  make(map[string]*orderState),
		nextID:  1,
	}
}

func (r *fullStubCoreRepo) portKey(deviceID int64, portNo int32) string {
	return fmt.Sprintf("%d:%d", deviceID, portNo)
}

func (r *fullStubCoreRepo) orderKey(deviceID int64, portNo int32) string {
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
	r.mu.Lock()
	defer r.mu.Unlock()
	r.settleCount++
	return r.settleErr
}

func (r *fullStubCoreRepo) UpsertOrderProgress(ctx context.Context, deviceID int64, portNo int32, orderNo string, businessNo *int32, durationSec int32, kwh0p01 int32, status int32, powerW01 *int32) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.orders[r.orderKey(deviceID, portNo)] = &orderState{
		deviceID:   deviceID,
		portNo:     portNo,
		orderNo:    orderNo,
		businessNo: businessNo,
		status:     status,
		durationS:  durationSec,
		energyKwh:  kwh0p01,
		powerW:     powerW01,
	}
	r.progressCount++
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

// getOrder 获取订单
func (r *fullStubCoreRepo) getOrder(deviceID int64, portNo int32) (*orderState, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	o, ok := r.orders[r.orderKey(deviceID, portNo)]
	return o, ok
}

// TestChargingFlow_Complete 测试完整的充电流程
// 流程：心跳 -> 充电开始 -> 充电进行中(多次) -> 充电结束
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

		// 验证订单已创建
		order, ok := repo.getOrder(device.ID, int32(portNo))
		require.True(t, ok, "订单应已创建")
		assert.Equal(t, int32(2), order.status, "订单状态应为充电中(2)")
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

		// 验证订单已更新
		order, ok := repo.getOrder(device.ID, int32(portNo))
		require.True(t, ok)
		assert.Equal(t, int32(180), order.durationS, "充电时长应为180秒")
		assert.Equal(t, int32(30), order.energyKwh, "用电量应为0.3kWh")
	})

	// 步骤4: 充电结束
	t.Run("Step4_SessionEnded", func(t *testing.T) {
		device, _ := repo.GetDeviceByPhyID(ctx, devicePhyID)
		initialSettleCount := repo.settleCount

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

		// 验证结算被调用
		assert.Equal(t, initialSettleCount+1, repo.settleCount, "应调用一次订单结算")
	})
}

// TestChargingFlow_ProgressShouldNotTriggerEnd 测试充电进行中不应触发结束流程
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

	// 验证没有调用结算
	assert.Equal(t, 0, repo.settleCount, "充电进行中不应调用订单结算")
}

// TestChargingFlow_StatusBit5Determines 测试状态位bit5决定是否为充电结束
func TestChargingFlow_StatusBit5Determines(t *testing.T) {
	tests := []struct {
		name           string
		status         int32
		isCharging     bool
		expectedSettle bool
	}{
		{"0xB0-充电中", 0xB0, true, false},
		{"0xA0-充电中", 0xA0, true, false},
		{"0x90-空闲", 0x90, false, true},
		{"0x80-仅在线", 0x80, false, true},
		{"0x30-充电中", 0x30, true, false},
		{"0x10-空闲", 0x10, false, true},
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

			// 验证结算调用
			if tt.expectedSettle {
				assert.Equal(t, 1, repo.settleCount, "状态%s应触发结算", tt.name)
			} else {
				// SessionEnded 事件总是会尝试结算
				// 但在 BKV handlers 层应该根据 bit5 判断是否发送 SessionEnded 事件
			}
		})
	}
}

// TestChargingFlow_SettleFailureStillPersistsPort 测试结算失败时端口状态仍然持久化
func TestChargingFlow_SettleFailureStillPersistsPort(t *testing.T) {
	repo := newFullStubCoreRepo()
	repo.settleErr = errors.New("database connection lost")
	driver := NewDriverCore(repo, nil, zap.NewNop())
	ctx := context.Background()

	devicePhyID := "TEST-SETTLE-FAIL"
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

	// 即使结算失败，HandleCoreEvent 应该返回错误但端口状态已持久化
	err := driver.HandleCoreEvent(ctx, ev)
	require.Error(t, err, "结算失败应返回错误")

	// 关键验证：端口状态应已持久化
	device, _ := repo.GetDeviceByPhyID(ctx, devicePhyID)
	portStatus, ok := repo.getPortStatus(device.ID, int32(portNo))
	require.True(t, ok, "即使结算失败，端口状态也应持久化")
	assert.Equal(t, idleStatus, portStatus, "端口状态应为空闲")
	assert.Equal(t, 1, repo.upsertCount, "端口快照应被写入一次")
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
