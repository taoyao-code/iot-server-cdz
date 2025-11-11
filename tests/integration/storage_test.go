package integration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/taoyao-code/iot-server/internal/storage/pg"
	"github.com/taoyao-code/iot-server/tests/testutil"
)

// TestStorageDeviceOperations 测试设备存储操作
func TestStorageDeviceOperations(t *testing.T) {
	db := getTestDB(t)
	defer cleanupTest(t)

	repo := &pg.Repository{Pool: db}

	t.Run("EnsureDevice_NewDevice", func(t *testing.T) {
		phyID := "TEST_DEVICE_001"

		deviceID, err := repo.EnsureDevice(context.Background(), phyID)
		require.NoError(t, err)
		assert.Greater(t, deviceID, int64(0))

		// 验证设备已创建
		device, err := repo.GetDeviceByPhyID(context.Background(), phyID)
		require.NoError(t, err)
		assert.Equal(t, phyID, device.PhyID)
		assert.NotNil(t, device.LastSeenAt)
	})

	t.Run("EnsureDevice_ExistingDevice", func(t *testing.T) {
		phyID := "TEST_DEVICE_002"

		// 第一次创建
		id1, err := repo.EnsureDevice(context.Background(), phyID)
		require.NoError(t, err)

		// 第二次应返回相同 ID
		id2, err := repo.EnsureDevice(context.Background(), phyID)
		require.NoError(t, err)
		assert.Equal(t, id1, id2)
	})

	t.Run("TouchDeviceLastSeen", func(t *testing.T) {
		phyID := "TEST_DEVICE_003"

		deviceID, err := repo.EnsureDevice(context.Background(), phyID)
		require.NoError(t, err)

		// 更新心跳时间
		touchTime := time.Now().Add(-10 * time.Second)
		err = repo.TouchDeviceLastSeen(context.Background(), phyID, touchTime)
		require.NoError(t, err)

		// 验证时间已更新
		device, err := repo.GetDeviceByPhyID(context.Background(), phyID)
		require.NoError(t, err)
		assert.NotNil(t, device.LastSeenAt)
		assert.WithinDuration(t, touchTime, *device.LastSeenAt, time.Second)

		_ = deviceID
	})
}

// TestStoragePortOperations 测试端口存储操作
func TestStoragePortOperations(t *testing.T) {
	db := getTestDB(t)
	defer cleanupTest(t)

	repo := &pg.Repository{Pool: db}

	t.Run("UpsertPortState_NewPort", func(t *testing.T) {
		device := testutil.CreateTestDevice(t, db, "")
		portNo := 1
		status := 0 // 空闲
		power := 5000

		err := repo.UpsertPortState(context.Background(), device.ID, portNo, status, &power)
		require.NoError(t, err)

		// 验证端口状态
		ports, err := repo.ListPortsByPhyID(context.Background(), device.PhyID)
		require.NoError(t, err)
		require.Len(t, ports, 1)
		assert.Equal(t, portNo, ports[0].PortNo)
		assert.Equal(t, status, ports[0].Status)
		require.NotNil(t, ports[0].PowerW)
		assert.Equal(t, power, *ports[0].PowerW)
	})

	t.Run("UpsertPortState_UpdateExisting", func(t *testing.T) {
		device := testutil.CreateTestDevice(t, db, "")
		portNo := 1

		// 初始状态
		status1 := 0
		err := repo.UpsertPortState(context.Background(), device.ID, portNo, status1, nil)
		require.NoError(t, err)

		// 更新状态
		status2 := 1 // 充电中
		power := 7000
		err = repo.UpsertPortState(context.Background(), device.ID, portNo, status2, &power)
		require.NoError(t, err)

		// 验证状态已更新
		ports, err := repo.ListPortsByPhyID(context.Background(), device.PhyID)
		require.NoError(t, err)
		require.Len(t, ports, 1)
		assert.Equal(t, status2, ports[0].Status)
		require.NotNil(t, ports[0].PowerW)
		assert.Equal(t, power, *ports[0].PowerW)
	})
}

// TestStorageOrderOperations 测试订单存储操作
func TestStorageOrderOperations(t *testing.T) {
	db := getTestDB(t)
	defer cleanupTest(t)

	repo := &pg.Repository{Pool: db}

	t.Run("UpsertOrderProgress_NewOrder", func(t *testing.T) {
		device := testutil.CreateTestDevice(t, db, "")
		portNo := 1
		orderNo := "TEST_ORDER_001"
		status := 1   // 进行中
		kwh01 := 1500 // 15.00 kWh

		err := repo.UpsertOrderProgress(context.Background(), device.ID, portNo, orderNo, 0, kwh01, status, nil)
		require.NoError(t, err)

		// 验证订单已创建
		orders, err := repo.ListOrdersByPhyID(context.Background(), device.PhyID, 10, 0)
		require.NoError(t, err)
		require.Len(t, orders, 1)
		assert.Equal(t, orderNo, orders[0].OrderNo)
		assert.Equal(t, status, orders[0].Status)
	})

	t.Run("UpsertOrderProgress_UpdateExisting", func(t *testing.T) {
		device := testutil.CreateTestDevice(t, db, "")
		portNo := 1
		orderNo := "TEST_ORDER_002"

		// 初始进度
		kwh1 := 1000
		status := 1
		err := repo.UpsertOrderProgress(context.Background(), device.ID, portNo, orderNo, 0, kwh1, status, nil)
		require.NoError(t, err)

		// 更新进度
		kwh2 := 2500
		err = repo.UpsertOrderProgress(context.Background(), device.ID, portNo, orderNo, 0, kwh2, status, nil)
		require.NoError(t, err)

		// 验证进度已更新
		orders, err := repo.ListOrdersByPhyID(context.Background(), device.PhyID, 10, 0)
		require.NoError(t, err)
		require.Len(t, orders, 1)
		require.NotNil(t, orders[0].Kwh01)
		assert.Equal(t, int64(kwh2), *orders[0].Kwh01)
	})

	t.Run("SettleOrder", func(t *testing.T) {
		device := testutil.CreateTestDevice(t, db, "")
		portNo := 1
		orderNo := "TEST_ORDER_003"
		durationSec := 3600 // 1小时
		kwh01 := 5000       // 50.00 kWh
		reason := 1         // 正常结束

		err := repo.SettleOrder(context.Background(), device.ID, portNo, orderNo, durationSec, kwh01, reason)
		require.NoError(t, err)

		// 验证订单已结算
		orders, err := repo.ListOrdersByPhyID(context.Background(), device.PhyID, 10, 0)
		require.NoError(t, err)
		require.Len(t, orders, 1)
		assert.Equal(t, 2, orders[0].Status) // 已结束
		assert.NotNil(t, orders[0].EndTime)
	})
}

// TestStorageTransactionIsolation 测试数据库事务隔离（P1-3 端口并发冲突）
func TestStorageTransactionIsolation(t *testing.T) {
	db := getTestDB(t)
	defer cleanupTest(t)

	repo := &pg.Repository{Pool: db}

	t.Run("ConcurrentPortUpdates", func(t *testing.T) {
		device := testutil.CreateTestDevice(t, db, "")
		portNo := 1

		// 并发更新端口状态
		done := make(chan error, 2)

		go func() {
			err := repo.UpsertPortState(context.Background(), device.ID, portNo, 1, nil)
			done <- err
		}()

		go func() {
			err := repo.UpsertPortState(context.Background(), device.ID, portNo, 2, nil)
			done <- err
		}()

		// 等待两个更新完成
		err1 := <-done
		err2 := <-done

		// 至少有一个成功
		assert.True(t, err1 == nil || err2 == nil, "at least one update should succeed")

		// 验证最终状态
		ports, err := repo.ListPortsByPhyID(context.Background(), device.PhyID)
		require.NoError(t, err)
		require.Len(t, ports, 1)
		assert.True(t, ports[0].Status == 1 || ports[0].Status == 2, "status should be 1 or 2")
	})
}
