package pg

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPort_UpsertPortState 测试端口状态更新
func TestPort_UpsertPortState(t *testing.T) {
	repo := setupTestRepo(t)
	phyID := "TEST_PORT_001"
	defer cleanupTestData(t, repo, phyID)

	ctx := context.Background()
	deviceID := createTestDevice(t, repo, phyID)

	// 1. 首次插入端口状态
	power := 100
	err := repo.UpsertPortState(ctx, deviceID, 1, 1, &power)
	require.NoError(t, err)

	// 2. 验证端口状态已保存
	ports, err := repo.ListPortsByPhyID(ctx, phyID)
	require.NoError(t, err)
	assert.Len(t, ports, 1)
	assert.Equal(t, 1, ports[0].PortNo)
	assert.Equal(t, 1, ports[0].Status)
	require.NotNil(t, ports[0].PowerW)
	assert.Equal(t, 100, *ports[0].PowerW)
}

// TestPort_UpsertPortStateUpdate 测试端口状态更新（幂等）
func TestPort_UpsertPortStateUpdate(t *testing.T) {
	repo := setupTestRepo(t)
	phyID := "TEST_PORT_002"
	defer cleanupTestData(t, repo, phyID)

	ctx := context.Background()
	deviceID := createTestDevice(t, repo, phyID)

	// 1. 首次插入
	power1 := 100
	err := repo.UpsertPortState(ctx, deviceID, 1, 1, &power1)
	require.NoError(t, err)

	// 2. 更新状态
	power2 := 200
	err = repo.UpsertPortState(ctx, deviceID, 1, 2, &power2)
	require.NoError(t, err)

	// 3. 验证状态已更新
	ports, err := repo.ListPortsByPhyID(ctx, phyID)
	require.NoError(t, err)
	assert.Len(t, ports, 1)
	assert.Equal(t, 2, ports[0].Status)
	require.NotNil(t, ports[0].PowerW)
	assert.Equal(t, 200, *ports[0].PowerW)
}

// TestPort_UpsertMultiplePorts 测试多端口状态
func TestPort_UpsertMultiplePorts(t *testing.T) {
	repo := setupTestRepo(t)
	phyID := "TEST_PORT_003"
	defer cleanupTestData(t, repo, phyID)

	ctx := context.Background()
	deviceID := createTestDevice(t, repo, phyID)

	// 插入3个端口的状态
	for i := 1; i <= 3; i++ {
		power := i * 100
		err := repo.UpsertPortState(ctx, deviceID, i, 1, &power)
		require.NoError(t, err)
	}

	// 验证3个端口
	ports, err := repo.ListPortsByPhyID(ctx, phyID)
	require.NoError(t, err)
	assert.Len(t, ports, 3)

	// 验证端口按port_no排序
	for i, port := range ports {
		assert.Equal(t, i+1, port.PortNo)
	}
}

// TestPort_UpsertPortStateNullPower 测试端口功率为空
func TestPort_UpsertPortStateNullPower(t *testing.T) {
	repo := setupTestRepo(t)
	phyID := "TEST_PORT_004"
	defer cleanupTestData(t, repo, phyID)

	ctx := context.Background()
	deviceID := createTestDevice(t, repo, phyID)

	// 插入端口状态，功率为nil
	err := repo.UpsertPortState(ctx, deviceID, 1, 1, nil)
	require.NoError(t, err)

	// 验证功率为nil
	ports, err := repo.ListPortsByPhyID(ctx, phyID)
	require.NoError(t, err)
	assert.Len(t, ports, 1)
	assert.Nil(t, ports[0].PowerW)
}

// TestPort_ListPortsByPhyIDNotFound 测试查询不存在设备的端口
func TestPort_ListPortsByPhyIDNotFound(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	// 查询不存在的设备
	ports, err := repo.ListPortsByPhyID(ctx, "NONEXISTENT_DEVICE")
	require.NoError(t, err)
	assert.Empty(t, ports)
}

// TestCmdLog_InsertCmdLog 测试插入命令日志
func TestCmdLog_InsertCmdLog(t *testing.T) {
	repo := setupTestRepo(t)
	phyID := "TEST_CMDLOG_001"
	defer cleanupTestData(t, repo, phyID)

	ctx := context.Background()
	deviceID := createTestDevice(t, repo, phyID)

	// 插入上行日志
	payload := []byte{0x01, 0x02, 0x03}
	err := repo.InsertCmdLog(ctx, deviceID, 100, 0x11, 0, payload, true)
	require.NoError(t, err)

	// 验证日志已插入
	var count int
	err = repo.Pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM cmd_log WHERE device_id=$1 AND msg_id=$2",
		deviceID, 100).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

// TestCmdLog_InsertMultipleLogs 测试插入多条日志
func TestCmdLog_InsertMultipleLogs(t *testing.T) {
	repo := setupTestRepo(t)
	phyID := "TEST_CMDLOG_002"
	defer cleanupTestData(t, repo, phyID)

	ctx := context.Background()
	deviceID := createTestDevice(t, repo, phyID)

	// 插入3条日志
	for i := 1; i <= 3; i++ {
		payload := []byte{byte(i)}
		err := repo.InsertCmdLog(ctx, deviceID, 100+i, 0x11, 0, payload, true)
		require.NoError(t, err)
	}

	// 验证3条日志
	var count int
	err := repo.Pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM cmd_log WHERE device_id=$1",
		deviceID).Scan(&count)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, count, 3)
}

// TestCmdLog_UpDownDirection 测试上下行日志
func TestCmdLog_UpDownDirection(t *testing.T) {
	repo := setupTestRepo(t)
	phyID := "TEST_CMDLOG_003"
	defer cleanupTestData(t, repo, phyID)

	ctx := context.Background()
	deviceID := createTestDevice(t, repo, phyID)

	// 插入上行日志 (direction=0)
	err := repo.InsertCmdLog(ctx, deviceID, 101, 0x11, 0, []byte{0x01}, true)
	require.NoError(t, err)

	// 插入下行日志 (direction=1)
	err = repo.InsertCmdLog(ctx, deviceID, 102, 0x21, 1, []byte{0x02}, true)
	require.NoError(t, err)

	// 验证两条日志都已插入
	var upCount, downCount int
	err = repo.Pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM cmd_log WHERE device_id=$1 AND direction=0",
		deviceID).Scan(&upCount)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, upCount, 1)

	err = repo.Pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM cmd_log WHERE device_id=$1 AND direction=1",
		deviceID).Scan(&downCount)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, downCount, 1)
}

// TestCmdLog_SuccessFailure 测试成功/失败日志
func TestCmdLog_SuccessFailure(t *testing.T) {
	repo := setupTestRepo(t)
	phyID := "TEST_CMDLOG_004"
	defer cleanupTestData(t, repo, phyID)

	ctx := context.Background()
	deviceID := createTestDevice(t, repo, phyID)

	// 插入成功日志
	err := repo.InsertCmdLog(ctx, deviceID, 201, 0x11, 0, []byte{0x01}, true)
	require.NoError(t, err)

	// 插入失败日志
	err = repo.InsertCmdLog(ctx, deviceID, 202, 0x11, 0, []byte{0x02}, false)
	require.NoError(t, err)

	// 验证成功/失败日志
	var successCount, failCount int
	err = repo.Pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM cmd_log WHERE device_id=$1 AND success=true",
		deviceID).Scan(&successCount)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, successCount, 1)

	err = repo.Pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM cmd_log WHERE device_id=$1 AND success=false",
		deviceID).Scan(&failCount)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, failCount, 1)
}

// TestCmdLog_EmptyPayload 测试空payload
func TestCmdLog_EmptyPayload(t *testing.T) {
	repo := setupTestRepo(t)
	phyID := "TEST_CMDLOG_005"
	defer cleanupTestData(t, repo, phyID)

	ctx := context.Background()
	deviceID := createTestDevice(t, repo, phyID)

	// 插入空payload日志
	err := repo.InsertCmdLog(ctx, deviceID, 301, 0x11, 0, []byte{}, true)
	require.NoError(t, err)

	// 插入nil payload日志
	err = repo.InsertCmdLog(ctx, deviceID, 302, 0x11, 0, nil, true)
	require.NoError(t, err)

	// 验证都已插入
	var count int
	err = repo.Pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM cmd_log WHERE device_id=$1 AND msg_id IN (301, 302)",
		deviceID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}
