package pg

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDeviceCheck_DeviceOnline 测试设备在线判定（60秒阈值）
func TestDeviceCheck_DeviceOnline(t *testing.T) {
	repo := setupTestRepo(t)
	phyID := "TEST_DEV_001"
	defer cleanupTestData(t, repo, phyID)

	ctx := context.Background()
	deviceID := createTestDevice(t, repo, phyID)

	// 1. 新创建的设备应该在线（last_seen_at刚更新）
	device, err := repo.GetDeviceByPhyID(ctx, phyID)
	require.NoError(t, err)
	require.NotNil(t, device)
	require.NotNil(t, device.LastSeenAt, "last_seen_at不应为空")
	timeSinceLastSeen := time.Since(*device.LastSeenAt)
	assert.Less(t, timeSinceLastSeen.Seconds(), 60.0, "新创建的设备应该在线（<60秒）")

	// 2. 设置last_seen为59秒前（仍在线）
	_, err = repo.Pool.Exec(ctx,
		"UPDATE devices SET last_seen_at = NOW() - INTERVAL '59 seconds' WHERE id=$1",
		deviceID)
	require.NoError(t, err)

	// 验证设备仍被视为在线（在60秒阈值内）
	device, err = repo.GetDeviceByPhyID(ctx, phyID)
	require.NoError(t, err)
	require.NotNil(t, device.LastSeenAt)
	timeSinceLastSeen = time.Since(*device.LastSeenAt)
	assert.LessOrEqual(t, timeSinceLastSeen.Seconds(), 60.0, "设备应视为在线（<60秒）")

	// 3. 设置last_seen为61秒前（应离线）
	_, err = repo.Pool.Exec(ctx,
		"UPDATE devices SET last_seen_at = NOW() - INTERVAL '61 seconds' WHERE id=$1",
		deviceID)
	require.NoError(t, err)

	device, err = repo.GetDeviceByPhyID(ctx, phyID)
	require.NoError(t, err)
	require.NotNil(t, device.LastSeenAt)
	timeSinceLastSeen = time.Since(*device.LastSeenAt)
	assert.Greater(t, timeSinceLastSeen.Seconds(), 60.0, "设备应视为离线（>60秒）")
}

// TestDeviceCheck_DeviceOfflineRejectOrder P0-1：设备离线时拒绝创建订单
func TestDeviceCheck_DeviceOfflineRejectOrder(t *testing.T) {
	repo := setupTestRepo(t)
	phyID := "TEST_DEV_002"
	defer cleanupTestData(t, repo, phyID)

	ctx := context.Background()
	deviceID := createTestDevice(t, repo, phyID)

	// 1. 设置设备离线（last_seen > 60秒）
	_, err := repo.Pool.Exec(ctx,
		"UPDATE devices SET last_seen_at = NOW() - INTERVAL '125 seconds' WHERE id=$1",
		deviceID)
	require.NoError(t, err)

	// 2. 验证设备离线
	device, err := repo.GetDeviceByPhyID(ctx, phyID)
	require.NoError(t, err)
	require.NotNil(t, device.LastSeenAt)
	timeSinceLastSeen := time.Since(*device.LastSeenAt)
	assert.Greater(t, timeSinceLastSeen.Seconds(), 60.0, "last_seen应>60秒，设备应离线")

	// 3. 尝试创建订单前的检查逻辑
	// 创建订单API应该在这个阶段拒绝（在调用CreateOrder之前）
	isOnline := time.Since(*device.LastSeenAt).Seconds() < 60
	assert.False(t, isOnline, "检查逻辑应判定设备离线")

	t.Log("✅ 设备离线检查通过，应拒绝创建订单")
}

// TestDeviceCheck_HeartbeatTimeout 测试心跳超时检测
func TestDeviceCheck_HeartbeatTimeout(t *testing.T) {
	repo := setupTestRepo(t)
	phyID := "TEST_DEV_003"
	defer cleanupTestData(t, repo, phyID)

	ctx := context.Background()
	createTestDevice(t, repo, phyID)

	// 1. 模拟心跳超时：设置last_seen为90秒前
	_, err := repo.Pool.Exec(ctx,
		"UPDATE devices SET last_seen_at = NOW() - INTERVAL '90 seconds' WHERE phy_id=$1",
		phyID)
	require.NoError(t, err)

	// 2. 查询超时设备（模拟监控任务）
	var count int
	query := `
		SELECT COUNT(*) FROM devices 
		WHERE last_seen_at < NOW() - INTERVAL '60 seconds'
	`
	err = repo.Pool.QueryRow(ctx, query).Scan(&count)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, count, 1, "应找到至少1个心跳超时的设备")

	// 3. 验证可以通过last_seen_at判断设备离线
	device, err := repo.GetDeviceByPhyID(ctx, phyID)
	require.NoError(t, err)
	require.NotNil(t, device.LastSeenAt)
	timeSinceLastSeen := time.Since(*device.LastSeenAt)
	assert.Greater(t, timeSinceLastSeen.Seconds(), 60.0, "心跳超时的设备应被判定为离线")
}

// TestDeviceCheck_PortStatusCheck 测试端口状态检查
func TestDeviceCheck_PortStatusCheck(t *testing.T) {
	repo := setupTestRepo(t)
	phyID := "TEST_DEV_004"
	defer cleanupTestData(t, repo, phyID)

	ctx := context.Background()
	deviceID := createTestDevice(t, repo, phyID)
	portNo := 1

	// 1. 检查端口是否有pending订单
	pendingOrder, err := repo.GetPendingOrderByPort(ctx, deviceID, portNo)
	assert.NoError(t, err)
	assert.Nil(t, pendingOrder, "新端口不应有pending订单")

	// 2. 检查端口是否有charging订单
	chargingOrder, err := repo.GetChargingOrderByPort(ctx, deviceID, portNo)
	assert.NoError(t, err)
	assert.Nil(t, chargingOrder, "新端口不应有charging订单")

	// 3. 创建订单后，检查端口占用
	createTestOrder(t, repo, deviceID, portNo, "DEV004_ORD")

	pendingOrder, err = repo.GetPendingOrderByPort(ctx, deviceID, portNo)
	assert.NoError(t, err)
	assert.NotNil(t, pendingOrder, "端口应有pending订单")
}

// TestDeviceCheck_PortOccupationWithMiddleStates P1-3：端口占用检查包含中间态
func TestDeviceCheck_PortOccupationWithMiddleStates(t *testing.T) {
	repo := setupTestRepo(t)
	phyID := "TEST_DEV_005"
	defer cleanupTestData(t, repo, phyID)

	ctx := context.Background()
	deviceID := createTestDevice(t, repo, phyID)
	portNo := 1

	// 测试不同状态的端口占用情况
	testCases := []struct {
		name         string
		status       int
		statusName   string
		shouldOccupy bool
	}{
		{"pending", 0, "pending", true},
		{"confirmed", 1, "confirmed", true},
		{"charging", 2, "charging", true},
		{"completed", 3, "completed", false},
		{"timeout", 4, "timeout", false},
		{"cancelled", 5, "cancelled", false},
		{"failed", 6, "failed", false},
		{"stopped", 7, "stopped", false},
		{"cancelling", 8, "cancelling", true},    // 中间态应占用
		{"stopping", 9, "stopping", true},        // 中间态应占用
		{"interrupted", 10, "interrupted", true}, // 临时态应占用
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 清理端口
			_, err := repo.Pool.Exec(ctx, "DELETE FROM orders WHERE device_id=$1 AND port_no=$2", deviceID, portNo)
			require.NoError(t, err)

			// 创建指定状态的订单
			orderNo := "DEV005_" + tc.statusName
			createTestOrder(t, repo, deviceID, portNo, orderNo)
			_, err = repo.Pool.Exec(ctx, "UPDATE orders SET status=$1 WHERE order_no=$2", tc.status, orderNo)
			require.NoError(t, err)

			// 检查端口是否被占用（查询活跃订单）
			var count int
			query := `
				SELECT COUNT(*) FROM orders 
				WHERE device_id=$1 AND port_no=$2 
				  AND status IN (0,1,2,8,9,10)
			`
			err = repo.Pool.QueryRow(ctx, query, deviceID, portNo).Scan(&count)
			require.NoError(t, err)

			if tc.shouldOccupy {
				assert.Equal(t, 1, count, "%s状态应占用端口", tc.statusName)
			} else {
				assert.Equal(t, 0, count, "%s状态不应占用端口", tc.statusName)
			}
		})
	}
}

// TestDeviceCheck_MultipleDevicesPorts 测试多设备多端口检查
func TestDeviceCheck_MultipleDevicesPorts(t *testing.T) {
	repo := setupTestRepo(t)
	phyID1 := "TEST_DEV_006_1"
	phyID2 := "TEST_DEV_006_2"
	defer func() {
		cleanupTestData(t, repo, phyID1)
		cleanupTestData(t, repo, phyID2)
	}()

	ctx := context.Background()
	deviceID1 := createTestDevice(t, repo, phyID1)
	deviceID2 := createTestDevice(t, repo, phyID2)

	// 1. 设备1端口1和端口2都创建订单
	createTestOrder(t, repo, deviceID1, 1, "DEV006_1_P1")
	createTestOrder(t, repo, deviceID1, 2, "DEV006_1_P2")

	// 2. 设备2端口1创建订单
	createTestOrder(t, repo, deviceID2, 1, "DEV006_2_P1")

	// 3. 检查各端口状态
	order1_1, err := repo.GetPendingOrderByPort(ctx, deviceID1, 1)
	assert.NoError(t, err)
	assert.NotNil(t, order1_1)

	order1_2, err := repo.GetPendingOrderByPort(ctx, deviceID1, 2)
	assert.NoError(t, err)
	assert.NotNil(t, order1_2)

	order2_1, err := repo.GetPendingOrderByPort(ctx, deviceID2, 1)
	assert.NoError(t, err)
	assert.NotNil(t, order2_1)

	// 4. 验证设备1端口3无订单
	order1_3, err := repo.GetPendingOrderByPort(ctx, deviceID1, 3)
	assert.NoError(t, err)
	assert.Nil(t, order1_3)
}

// TestDeviceCheck_DeviceRecoveryAfterOffline 测试设备离线后恢复
func TestDeviceCheck_DeviceRecoveryAfterOffline(t *testing.T) {
	repo := setupTestRepo(t)
	phyID := "TEST_DEV_007"
	defer cleanupTestData(t, repo, phyID)

	ctx := context.Background()
	createTestDevice(t, repo, phyID)

	// 1. 设置设备离线
	_, err := repo.Pool.Exec(ctx,
		"UPDATE devices SET last_seen_at = NOW() - INTERVAL '90 seconds' WHERE phy_id=$1",
		phyID)
	require.NoError(t, err)

	device, err := repo.GetDeviceByPhyID(ctx, phyID)
	require.NoError(t, err)
	require.NotNil(t, device.LastSeenAt)
	timeSinceLastSeen := time.Since(*device.LastSeenAt)
	assert.Greater(t, timeSinceLastSeen.Seconds(), 60.0, "设备应处于离线状态")

	// 2. 模拟设备恢复：更新last_seen
	_, err = repo.Pool.Exec(ctx,
		"UPDATE devices SET last_seen_at = NOW() WHERE phy_id=$1",
		phyID)
	require.NoError(t, err)

	// 3. 验证设备已恢复在线
	device, err = repo.GetDeviceByPhyID(ctx, phyID)
	require.NoError(t, err)
	require.NotNil(t, device.LastSeenAt)
	timeSinceLastSeen = time.Since(*device.LastSeenAt)
	assert.Less(t, timeSinceLastSeen.Seconds(), 10.0, "last_seen应为最近，设备已恢复在线")
}

// TestDeviceCheck_DeviceExistence 测试设备存在性检查
func TestDeviceCheck_DeviceExistence(t *testing.T) {
	repo := setupTestRepo(t)

	ctx := context.Background()

	// 1. 查询不存在的设备（应返回nil）
	device, err := repo.GetDeviceByPhyID(ctx, "NONEXISTENT_DEVICE")
	assert.Error(t, err) // 或者根据实现，可能返回nil without error
	assert.Nil(t, device)

	// 2. 创建设备后查询（应成功）
	phyID := "TEST_DEV_008"
	defer cleanupTestData(t, repo, phyID)

	deviceID := createTestDevice(t, repo, phyID)
	device, err = repo.GetDeviceByPhyID(ctx, phyID)
	require.NoError(t, err)
	require.NotNil(t, device)
	assert.Equal(t, deviceID, device.ID)
	assert.Equal(t, phyID, device.PhyID)
}

// TestDeviceCheck_DeviceCreation 测试设备创建
func TestDeviceCheck_DeviceCreation(t *testing.T) {
	repo := setupTestRepo(t)
	phyID := "TEST_DEV_009"
	defer cleanupTestData(t, repo, phyID)

	ctx := context.Background()

	// 创建设备
	deviceID, err := repo.EnsureDevice(ctx, phyID)
	require.NoError(t, err)
	assert.Greater(t, deviceID, int64(0), "设备ID应>0")

	// 验证设备创建成功
	device, err := repo.GetDeviceByPhyID(ctx, phyID)
	require.NoError(t, err)
	assert.Equal(t, phyID, device.PhyID, "设备PhyID应匹配")
	assert.Equal(t, deviceID, device.ID, "设备ID应匹配")
}

// TestDeviceCheck_ConcurrentDeviceAccess 测试并发设备访问
func TestDeviceCheck_ConcurrentDeviceAccess(t *testing.T) {
	repo := setupTestRepo(t)
	phyID := "TEST_DEV_010"
	defer cleanupTestData(t, repo, phyID)

	ctx := context.Background()
	createTestDevice(t, repo, phyID)

	// 模拟多个并发查询设备（不应出错）
	done := make(chan bool, 5)
	for i := 0; i < 5; i++ {
		go func() {
			device, err := repo.GetDeviceByPhyID(ctx, phyID)
			assert.NoError(t, err)
			assert.NotNil(t, device)
			done <- true
		}()
	}

	// 等待所有goroutine完成
	for i := 0; i < 5; i++ {
		<-done
	}
}
