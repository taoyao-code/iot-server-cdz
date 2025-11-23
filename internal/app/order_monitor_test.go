package app

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	pgstorage "github.com/taoyao-code/iot-server/internal/storage/pg"
)

// setupMonitorTestRepo 创建测试用的 Repository（连接到测试数据库）
func setupMonitorTestRepo(t *testing.T) *pgstorage.Repository {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://postgres:postgres@localhost:5432/iot_test?sslmode=disable"
	}

	ctx := context.Background()
	testDB, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skip("测试数据库不可用，跳过测试")
		return nil
	}

	// 验证连接
	if err := testDB.Ping(ctx); err != nil {
		t.Skip("测试数据库不可用，跳过测试")
		return nil
	}

	t.Cleanup(func() {
		testDB.Close()
	})

	return &pgstorage.Repository{Pool: testDB}
}

// cleanupMonitorTestData 清理测试数据
func cleanupMonitorTestData(t *testing.T, repo *pgstorage.Repository, devicePhyID string) {
	ctx := context.Background()
	_, err := repo.Pool.Exec(ctx, "DELETE FROM devices WHERE phy_id = $1", devicePhyID)
	if err != nil {
		t.Logf("清理测试数据失败: %v", err)
	}
}

// createMonitorTestDevice 创建测试设备
func createMonitorTestDevice(t *testing.T, repo *pgstorage.Repository, phyID string) int64 {
	ctx := context.Background()
	deviceID, err := repo.EnsureDevice(ctx, phyID)
	require.NoError(t, err, "创建测试设备失败")

	// 更新last_seen_at确保设备"在线"
	_, err = repo.Pool.Exec(ctx, "UPDATE devices SET last_seen_at = NOW() WHERE id=$1", deviceID)
	require.NoError(t, err, "更新设备状态失败")

	return deviceID
}

// createMonitorTestOrder 创建测试订单
func createMonitorTestOrder(t *testing.T, repo *pgstorage.Repository, deviceID int64, portNo int, orderHex string, status int) {
	ctx := context.Background()
	err := repo.UpsertOrderProgress(ctx, deviceID, portNo, orderHex, 0, 0, 0, nil)
	require.NoError(t, err, "创建测试订单失败")

	// 如果需要特定状态，更新订单状态
	if status != 0 {
		_, err = repo.Pool.Exec(ctx, "UPDATE orders SET status = $1 WHERE order_no = $2", status, orderHex)
		require.NoError(t, err, "更新订单状态失败")
	}
}

// TestNewOrderMonitor 测试OrderMonitor初始化
func TestNewOrderMonitor(t *testing.T) {
	repo := setupMonitorTestRepo(t)
	logger := zap.NewNop()

	monitor := NewOrderMonitor(repo, nil, logger)

	assert.NotNil(t, monitor)
	assert.Equal(t, 1*time.Minute, monitor.checkInterval)
	assert.Equal(t, 5*time.Minute, monitor.pendingTimeout)
	assert.Equal(t, 2*time.Hour, monitor.chargingTimeout)
}

// TestOrderMonitor_Stats 测试统计功能
func TestOrderMonitor_Stats(t *testing.T) {
	repo := setupMonitorTestRepo(t)
	logger := zap.NewNop()

	monitor := NewOrderMonitor(repo, nil, logger)
	monitor.statsChecked = 10
	monitor.statsPending = 2
	monitor.statsChargingLong = 1

	stats := monitor.Stats()

	assert.Equal(t, int64(10), stats["checked"])
	assert.Equal(t, int64(2), stats["pending_alerted"])
	assert.Equal(t, int64(1), stats["charging_long_alerted"])
	assert.Equal(t, 60.0, stats["check_interval_sec"])
	assert.Equal(t, 300.0, stats["pending_timeout_sec"])
	assert.Equal(t, 7200.0, stats["charging_timeout_sec"])
}

// TestOrderMonitor_CheckStalePendingOrders 测试pending订单超时检测
func TestOrderMonitor_CheckStalePendingOrders(t *testing.T) {
	repo := setupMonitorTestRepo(t)
	phyID := "MONITOR_TEST_001"
	defer cleanupMonitorTestData(t, repo, phyID)

	ctx := context.Background()
	deviceID := createMonitorTestDevice(t, repo, phyID)

	// 创建一个超时的pending订单（created_at设为10分钟前）
	orderNo := "STALE_ORD_001"
	createMonitorTestOrder(t, repo, deviceID, 1, orderNo, 0) // status=0 (pending)

	// 手动修改created_at为10分钟前
	tenMinAgo := time.Now().Add(-10 * time.Minute)
	_, err := repo.Pool.Exec(ctx, "UPDATE orders SET created_at = $1 WHERE order_no = $2", tenMinAgo, orderNo)
	require.NoError(t, err)

	// 创建监控器
	logger := zap.NewNop()
	monitor := NewOrderMonitor(repo, nil, logger)
	monitor.pendingTimeout = 5 * time.Minute // 5分钟超时阈值

	// 执行检查
	monitor.checkStalePendingOrders(ctx)

	// 验证统计
	assert.Equal(t, int64(1), monitor.statsPending, "应检测到1个超时pending订单")
}

// TestOrderMonitor_CheckLongChargingOrders 测试charging订单超长时间检测
func TestOrderMonitor_CheckLongChargingOrders(t *testing.T) {
	repo := setupMonitorTestRepo(t)
	phyID := "MONITOR_TEST_002"
	defer cleanupMonitorTestData(t, repo, phyID)

	ctx := context.Background()
	deviceID := createMonitorTestDevice(t, repo, phyID)

	// 创建一个charging订单
	orderNo := "LONG_CHARGING_001"
	createMonitorTestOrder(t, repo, deviceID, 1, orderNo, 0)

	// 更新为charging状态，start_time设为3小时前
	threeHoursAgo := time.Now().Add(-3 * time.Hour)
	_, err := repo.Pool.Exec(ctx,
		"UPDATE orders SET status = 1, start_time = $1 WHERE order_no = $2",
		threeHoursAgo, orderNo)
	require.NoError(t, err)

	// 创建监控器
	logger := zap.NewNop()
	monitor := NewOrderMonitor(repo, nil, logger)
	monitor.chargingTimeout = 2 * time.Hour // 2小时超时阈值

	// 执行检查
	monitor.checkLongChargingOrders(ctx)

	// 验证统计
	assert.Equal(t, int64(1), monitor.statsChargingLong, "应检测到1个超长charging订单")
}

// TestOrderMonitor_CleanupCancellingOrders 测试cancelling订单自动清理
func TestOrderMonitor_CleanupCancellingOrders(t *testing.T) {
	repo := setupMonitorTestRepo(t)
	phyID := "MONITOR_TEST_003"
	defer cleanupMonitorTestData(t, repo, phyID)

	ctx := context.Background()
	deviceID := createMonitorTestDevice(t, repo, phyID)

	// 创建cancelling订单（status=8）
	orderNo := "CANCELLING_001"
	createMonitorTestOrder(t, repo, deviceID, 1, orderNo, 8) // status=8 (cancelling)

	// 手动修改updated_at为40秒前
	fortySecsAgo := time.Now().Add(-40 * time.Second)
	_, err := repo.Pool.Exec(ctx, "UPDATE orders SET updated_at = $1 WHERE order_no = $2", fortySecsAgo, orderNo)
	require.NoError(t, err)

	// 创建监控器并执行清理
	logger := zap.NewNop()
	monitor := NewOrderMonitor(repo, nil, logger)
	monitor.cleanupCancellingOrders(ctx)

	// 验证订单状态已变为cancelled（status=5）
	var status int
	err = repo.Pool.QueryRow(ctx, "SELECT status FROM orders WHERE order_no = $1", orderNo).Scan(&status)
	require.NoError(t, err)
	assert.Equal(t, 5, status, "订单状态应自动变为cancelled(5)")
}

// TestOrderMonitor_CleanupStoppingOrders 测试stopping订单自动清理
func TestOrderMonitor_CleanupStoppingOrders(t *testing.T) {
	repo := setupMonitorTestRepo(t)
	phyID := "MONITOR_TEST_004"
	defer cleanupMonitorTestData(t, repo, phyID)

	ctx := context.Background()
	deviceID := createMonitorTestDevice(t, repo, phyID)

	// 创建stopping订单（status=9）
	orderNo := "STOPPING_001"
	createMonitorTestOrder(t, repo, deviceID, 1, orderNo, 9) // status=9 (stopping)

	// 手动修改updated_at为40秒前
	fortySecsAgo := time.Now().Add(-40 * time.Second)
	_, err := repo.Pool.Exec(ctx, "UPDATE orders SET updated_at = $1 WHERE order_no = $2", fortySecsAgo, orderNo)
	require.NoError(t, err)

	// 创建监控器并执行清理
	logger := zap.NewNop()
	monitor := NewOrderMonitor(repo, nil, logger)
	monitor.cleanupStoppingOrders(ctx)

	// 验证订单状态已变为stopped（status=7）
	var status int
	err = repo.Pool.QueryRow(ctx, "SELECT status FROM orders WHERE order_no = $1", orderNo).Scan(&status)
	require.NoError(t, err)
	assert.Equal(t, 7, status, "订单状态应自动变为stopped(7)")
}

// TestOrderMonitor_CleanupInterruptedOrders
func TestOrderMonitor_CleanupInterruptedOrders(t *testing.T) {
	repo := setupMonitorTestRepo(t)
	phyID := "MONITOR_TEST_005"
	defer cleanupMonitorTestData(t, repo, phyID)

	ctx := context.Background()
	deviceID := createMonitorTestDevice(t, repo, phyID)

	// 创建interrupted订单（status=10）
	orderNo := "INTERRUPTED_001"
	createMonitorTestOrder(t, repo, deviceID, 1, orderNo, 10) // status=10 (interrupted)

	// 手动修改updated_at为70秒前（超过60秒阈值）
	seventySecsAgo := time.Now().Add(-70 * time.Second)
	_, err := repo.Pool.Exec(ctx, "UPDATE orders SET updated_at = $1 WHERE order_no = $2", seventySecsAgo, orderNo)
	require.NoError(t, err)

	// 创建监控器并执行清理
	logger := zap.NewNop()
	monitor := NewOrderMonitor(repo, nil, logger)
	monitor.cleanupInterruptedOrders(ctx)

	// 验证订单状态已变为failed（status=6）
	var status int
	var failureReason *string
	err = repo.Pool.QueryRow(ctx, "SELECT status, failure_reason FROM orders WHERE order_no = $1", orderNo).Scan(&status, &failureReason)
	require.NoError(t, err)
	assert.Equal(t, 6, status, "订单状态应自动变为failed(6)")
	assert.NotNil(t, failureReason)
	assert.Contains(t, *failureReason, "device offline timeout", "失败原因应包含超时信息")
}

// TestOrderMonitor_Check 测试完整检查流程
func TestOrderMonitor_Check(t *testing.T) {
	repo := setupMonitorTestRepo(t)
	phyID := "MONITOR_TEST_006"
	defer cleanupMonitorTestData(t, repo, phyID)

	ctx := context.Background()
	deviceID := createMonitorTestDevice(t, repo, phyID)

	// 创建多种状态的订单
	createMonitorTestOrder(t, repo, deviceID, 1, "ORD_PENDING", 0)      // pending
	createMonitorTestOrder(t, repo, deviceID, 2, "ORD_CANCELLING", 8)   // cancelling
	createMonitorTestOrder(t, repo, deviceID, 3, "ORD_STOPPING", 9)     // stopping
	createMonitorTestOrder(t, repo, deviceID, 4, "ORD_INTERRUPTED", 10) // interrupted

	// 修改时间让它们超时
	fortySecsAgo := time.Now().Add(-40 * time.Second)
	seventySecsAgo := time.Now().Add(-70 * time.Second)
	_, err := repo.Pool.Exec(ctx, "UPDATE orders SET updated_at = $1 WHERE order_no IN ($2, $3)", fortySecsAgo, "ORD_CANCELLING", "ORD_STOPPING")
	require.NoError(t, err)
	_, err = repo.Pool.Exec(ctx, "UPDATE orders SET updated_at = $1 WHERE order_no = $2", seventySecsAgo, "ORD_INTERRUPTED")
	require.NoError(t, err)

	// 创建监控器并执行检查
	logger := zap.NewNop()
	monitor := NewOrderMonitor(repo, nil, logger)
	monitor.check(ctx)

	// 验证统计计数器
	assert.Equal(t, int64(1), monitor.statsChecked, "应执行1次检查")

	// 验证订单状态
	var status1, status2, status3 int
	err = repo.Pool.QueryRow(ctx, "SELECT status FROM orders WHERE order_no = $1", "ORD_CANCELLING").Scan(&status1)
	require.NoError(t, err)
	assert.Equal(t, 5, status1, "cancelling订单应变为cancelled")

	err = repo.Pool.QueryRow(ctx, "SELECT status FROM orders WHERE order_no = $1", "ORD_STOPPING").Scan(&status2)
	require.NoError(t, err)
	assert.Equal(t, 7, status2, "stopping订单应变为stopped")

	err = repo.Pool.QueryRow(ctx, "SELECT status FROM orders WHERE order_no = $1", "ORD_INTERRUPTED").Scan(&status3)
	require.NoError(t, err)
	assert.Equal(t, 6, status3, "interrupted订单应变为failed")
}

// TestOrderMonitor_Start_Shutdown 测试监控器启动和关闭
func TestOrderMonitor_Start_Shutdown(t *testing.T) {
	repo := setupMonitorTestRepo(t)
	logger := zap.NewNop()

	monitor := NewOrderMonitor(repo, nil, logger)
	monitor.checkInterval = 100 * time.Millisecond // 缩短检查间隔加快测试

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// 启动监控器（会在500ms后自动关闭）
	monitor.Start(ctx)

	// 验证至少执行了几次检查
	assert.GreaterOrEqual(t, monitor.statsChecked, int64(3), "应至少执行3次检查")
}
