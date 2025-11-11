package pg

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDevice_EnsureDeviceCreate 测试EnsureDevice创建新设备
func TestDevice_EnsureDeviceCreate(t *testing.T) {
	repo := setupTestRepo(t)
	phyID := "TEST_ENSURE_001"
	defer cleanupTestData(t, repo, phyID)

	ctx := context.Background()

	// 1. 首次调用EnsureDevice应创建设备
	deviceID1, err := repo.EnsureDevice(ctx, phyID)
	require.NoError(t, err)
	assert.Greater(t, deviceID1, int64(0), "设备ID应>0")

	// 2. 验证设备已创建
	device, err := repo.GetDeviceByPhyID(ctx, phyID)
	require.NoError(t, err)
	assert.Equal(t, phyID, device.PhyID)
	assert.Equal(t, deviceID1, device.ID)
}

// TestDevice_EnsureDeviceIdempotent 测试EnsureDevice幂等性
func TestDevice_EnsureDeviceIdempotent(t *testing.T) {
	repo := setupTestRepo(t)
	phyID := "TEST_ENSURE_002"
	defer cleanupTestData(t, repo, phyID)

	ctx := context.Background()

	// 1. 首次调用
	deviceID1, err := repo.EnsureDevice(ctx, phyID)
	require.NoError(t, err)

	// 2. 第二次调用应返回相同ID（不创建新设备）
	deviceID2, err := repo.EnsureDevice(ctx, phyID)
	require.NoError(t, err)
	assert.Equal(t, deviceID1, deviceID2, "EnsureDevice应该是幂等的")

	// 3. 验证只有一个设备
	var count int
	err = repo.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM devices WHERE phy_id=$1", phyID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "应该只有1个设备")
}

// TestDevice_TouchDeviceLastSeen 测试设备心跳更新
func TestDevice_TouchDeviceLastSeen(t *testing.T) {
	repo := setupTestRepo(t)
	phyID := "TEST_TOUCH_001"
	defer cleanupTestData(t, repo, phyID)

	ctx := context.Background()
	deviceID := createTestDevice(t, repo, phyID)

	// 1. 设置last_seen为60秒前
	_, err := repo.Pool.Exec(ctx,
		"UPDATE devices SET last_seen_at = NOW() - INTERVAL '60 seconds' WHERE id=$1",
		deviceID)
	require.NoError(t, err)

	// 2. 调用TouchDeviceLastSeen更新心跳
	err = repo.TouchDeviceLastSeen(ctx, phyID, time.Now())
	require.NoError(t, err)

	// 3. 验证last_seen已更新
	device, err := repo.GetDeviceByPhyID(ctx, phyID)
	require.NoError(t, err)
	require.NotNil(t, device.LastSeenAt)

	timeSinceLastSeen := time.Since(*device.LastSeenAt)
	assert.Less(t, timeSinceLastSeen.Seconds(), 5.0, "last_seen应该刚刚更新")
}

// TestDevice_TouchDeviceLastSeenByPhyID 测试通过PhyID更新心跳
func TestDevice_TouchDeviceLastSeenByPhyID(t *testing.T) {
	repo := setupTestRepo(t)
	phyID := "TEST_TOUCH_002"
	defer cleanupTestData(t, repo, phyID)

	ctx := context.Background()
	createTestDevice(t, repo, phyID)

	// 1. 直接通过phy_id更新
	_, err := repo.Pool.Exec(ctx,
		"UPDATE devices SET last_seen_at = NOW() WHERE phy_id=$1",
		phyID)
	require.NoError(t, err)

	// 2. 验证更新成功
	device, err := repo.GetDeviceByPhyID(ctx, phyID)
	require.NoError(t, err)
	require.NotNil(t, device.LastSeenAt)

	timeSinceLastSeen := time.Since(*device.LastSeenAt)
	assert.Less(t, timeSinceLastSeen.Seconds(), 5.0)
}

// TestDevice_ListDevices 测试设备列表查询
func TestDevice_ListDevices(t *testing.T) {
	repo := setupTestRepo(t)

	// 创建多个测试设备
	phyIDs := []string{"TEST_LIST_001", "TEST_LIST_002", "TEST_LIST_003"}
	for _, phyID := range phyIDs {
		defer cleanupTestData(t, repo, phyID)
	}

	ctx := context.Background()
	for _, phyID := range phyIDs {
		createTestDevice(t, repo, phyID)
	}

	// 查询设备列表 (limit=10, offset=0)
	devices, err := repo.ListDevices(ctx, 10, 0)
	require.NoError(t, err)

	// 验证至少包含我们创建的3个设备
	assert.GreaterOrEqual(t, len(devices), 3, "至少应该有3个设备")

	// 验证返回的设备有正确的字段
	if len(devices) > 0 {
		device := devices[0]
		assert.NotEmpty(t, device.PhyID, "PhyID不应为空")
		assert.Greater(t, device.ID, int64(0), "ID应>0")
	}
}

// TestDevice_ListDevicesPagination 测试设备列表分页
func TestDevice_ListDevicesPagination(t *testing.T) {
	repo := setupTestRepo(t)

	// 创建5个测试设备
	phyIDs := make([]string, 5)
	for i := 0; i < 5; i++ {
		phyIDs[i] = fmt.Sprintf("TEST_PAGE_%03d", i)
		defer cleanupTestData(t, repo, phyIDs[i])
	}

	ctx := context.Background()
	for _, phyID := range phyIDs {
		createTestDevice(t, repo, phyID)
	}

	// 1. 页面1: limit=2, offset=0
	devices1, err := repo.ListDevices(ctx, 2, 0)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(devices1), 2, "第1页最备2条")

	// 2. 页面2: limit=2, offset=2
	devices2, err := repo.ListDevices(ctx, 2, 2)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(devices2), 2, "第2页最备2条")

	// 3. 验证两页数据不重复（如果都有数据）
	if len(devices1) > 0 && len(devices2) > 0 {
		assert.NotEqual(t, devices1[0].ID, devices2[0].ID, "不同页的数据不应重复")
	}
}

// TestDevice_GetDeviceByPhyIDNotFound 测试查询不存在的设备
func TestDevice_GetDeviceByPhyIDNotFound(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	// 查询不存在的设备
	device, err := repo.GetDeviceByPhyID(ctx, "NONEXISTENT_DEVICE_999")
	assert.Error(t, err, "查询不存在的设备应返回错误")
	assert.Nil(t, device)
}

// TestDevice_ConcurrentEnsureDevice 测试并发创建设备
func TestDevice_ConcurrentEnsureDevice(t *testing.T) {
	repo := setupTestRepo(t)
	phyID := "TEST_CONCURRENT_001"
	defer cleanupTestData(t, repo, phyID)

	ctx := context.Background()

	// 并发调用EnsureDevice
	const goroutines = 5
	results := make(chan int64, goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			deviceID, err := repo.EnsureDevice(ctx, phyID)
			require.NoError(t, err)
			results <- deviceID
		}()
	}

	// 收集结果
	ids := make([]int64, goroutines)
	for i := 0; i < goroutines; i++ {
		ids[i] = <-results
	}

	// 验证所有返回的ID相同（只创建了一个设备）
	firstID := ids[0]
	for _, id := range ids {
		assert.Equal(t, firstID, id, "并发调用EnsureDevice应返回相同ID")
	}

	// 验证数据库中只有一个设备
	var count int
	err := repo.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM devices WHERE phy_id=$1", phyID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "应该只创建了1个设备")
}
