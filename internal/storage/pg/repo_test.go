package pg

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testDB *pgxpool.Pool

// TestMain 设置测试环境
func TestMain(m *testing.M) {
	// 从环境变量读取测试数据库连接
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://postgres:postgres@localhost:5432/iot_test?sslmode=disable"
	}

	ctx := context.Background()
	var err error
	testDB, err = pgxpool.New(ctx, dsn)
	if err != nil {
		// 如果无法连接测试数据库，跳过测试
		os.Exit(0)
	}
	defer testDB.Close()

	// 验证连接
	if err := testDB.Ping(ctx); err != nil {
		os.Exit(0)
	}

	// 运行测试
	code := m.Run()
	os.Exit(code)
}

// setupTestRepo 创建测试用的 Repository
func setupTestRepo(t *testing.T) *Repository {
	if testDB == nil {
		t.Skip("测试数据库不可用，跳过测试")
	}
	return &Repository{Pool: testDB}
}

// cleanupTestData 清理测试数据
func cleanupTestData(t *testing.T, repo *Repository, devicePhyID string) {
	ctx := context.Background()
	// 删除测试设备及其关联数据（级联删除）
	_, err := repo.Pool.Exec(ctx, "DELETE FROM devices WHERE phy_id = $1", devicePhyID)
	if err != nil {
		t.Logf("清理测试数据失败: %v", err)
	}
}

// createTestDevice 创建测试设备
func createTestDevice(t *testing.T, repo *Repository, phyID string) int64 {
	ctx := context.Background()
	deviceID, err := repo.EnsureDevice(ctx, phyID)
	require.NoError(t, err, "创建测试设备失败")

	// 更新last_seen_at确保设备"在线"
	_, err = repo.Pool.Exec(ctx, "UPDATE devices SET last_seen_at = NOW() WHERE id=$1", deviceID)
	require.NoError(t, err, "更新设备状态失败")

	return deviceID
}

// createTestOrder 创建测试订单
func createTestOrder(t *testing.T, repo *Repository, deviceID int64, portNo int, orderHex string) {
	ctx := context.Background()
	// 使用 UpsertOrderProgress 创建订单（status=0表示pending）
	err := repo.UpsertOrderProgress(ctx, deviceID, portNo, orderHex, 0, 0, 0, nil)
	require.NoError(t, err, "创建测试订单失败")
}

// TestOrderStatusTransition_PendingToCharging 测试订单状态：pending → charging
func TestOrderStatusTransition_PendingToCharging(t *testing.T) {
	repo := setupTestRepo(t)
	phyID := "TEST_DEVICE_001"
	defer cleanupTestData(t, repo, phyID)

	ctx := context.Background()
	deviceID := createTestDevice(t, repo, phyID)
	orderNo := "ORD001"
	portNo := 1

	// 1. 创建pending订单
	createTestOrder(t, repo, deviceID, portNo, orderNo)

	// 2. 验证初始状态为pending
	order, err := repo.GetPendingOrderByPort(ctx, deviceID, portNo)
	require.NoError(t, err)
	require.NotNil(t, order)
	assert.Equal(t, 0, order.Status, "初始状态应为pending(0)")

	// 3. 更新为charging
	startTime := time.Now()
	err = repo.UpdateOrderToCharging(ctx, orderNo, startTime)
	require.NoError(t, err, "更新为charging失败")

	// 4. 验证状态已变为charging
	chargingOrder, err := repo.GetChargingOrderByPort(ctx, deviceID, portNo)
	require.NoError(t, err)
	require.NotNil(t, chargingOrder)
	assert.Equal(t, 2, chargingOrder.Status, "状态应为charging(2)")
	assert.NotNil(t, chargingOrder.StartTime, "开始时间不应为空")
}

// TestOrderStatusTransition_ChargingToCompleted 测试订单状态：charging → completed
func TestOrderStatusTransition_ChargingToCompleted(t *testing.T) {
	repo := setupTestRepo(t)
	phyID := "TEST_DEVICE_002"
	defer cleanupTestData(t, repo, phyID)

	ctx := context.Background()
	deviceID := createTestDevice(t, repo, phyID)
	orderNo := "ORD002"
	portNo := 1

	// 1. 创建charging订单
	createTestOrder(t, repo, deviceID, portNo, orderNo)
	err := repo.UpdateOrderToCharging(ctx, orderNo, time.Now())
	require.NoError(t, err)

	// 2. 验证状态为charging
	order, err := repo.GetChargingOrderByPort(ctx, deviceID, portNo)
	require.NoError(t, err)
	require.NotNil(t, order)
	assert.Equal(t, 2, order.Status)

	// 3. 完成订单
	endTime := time.Now()
	err = repo.CompleteOrderByPort(ctx, deviceID, portNo, endTime, 0)
	require.NoError(t, err, "完成订单失败")

	// 4. 验证状态已变为completed（端口不应再有charging订单）
	chargingOrder, err := repo.GetChargingOrderByPort(ctx, deviceID, portNo)
	assert.NoError(t, err)
	assert.Nil(t, chargingOrder, "端口不应再有charging订单")
}

// TestOrderStatusTransition_PendingToCancel 测试订单状态：pending → cancelled
func TestOrderStatusTransition_PendingToCancel(t *testing.T) {
	repo := setupTestRepo(t)
	phyID := "TEST_DEVICE_003"
	defer cleanupTestData(t, repo, phyID)

	ctx := context.Background()
	deviceID := createTestDevice(t, repo, phyID)
	orderNo := "ORD003"
	portNo := 1

	// 1. 创建pending订单
	createTestOrder(t, repo, deviceID, portNo, orderNo)

	// 2. 验证初始状态
	order, err := repo.GetPendingOrderByPort(ctx, deviceID, portNo)
	require.NoError(t, err)
	require.NotNil(t, order)

	// 3. 取消订单
	err = repo.CancelOrderByPort(ctx, deviceID, portNo)
	require.NoError(t, err, "取消订单失败")

	// 4. 验证端口不再有pending订单
	cancelledOrder, err := repo.GetPendingOrderByPort(ctx, deviceID, portNo)
	assert.NoError(t, err)
	assert.Nil(t, cancelledOrder, "端口不应再有pending订单")
}

// TestOrderStatusTransition_ChargingToInterrupted 测试订单状态：charging → interrupted
func TestOrderStatusTransition_ChargingToInterrupted(t *testing.T) {
	repo := setupTestRepo(t)
	phyID := "TEST_DEVICE_004"
	defer cleanupTestData(t, repo, phyID)

	ctx := context.Background()
	deviceID := createTestDevice(t, repo, phyID)
	orderNo := "ORD004"
	portNo := 1

	// 1. 创建charging订单
	createTestOrder(t, repo, deviceID, portNo, orderNo)
	err := repo.UpdateOrderToCharging(ctx, orderNo, time.Now())
	require.NoError(t, err)

	// 2. 标记为interrupted（模拟设备离线）
	count, err := repo.MarkChargingOrdersAsInterrupted(ctx, deviceID)
	require.NoError(t, err, "标记interrupted失败")
	assert.Equal(t, int64(1), count, "应标记1个订单为interrupted")

	// 3. 验证订单已interrupted
	orders, err := repo.GetInterruptedOrders(ctx, deviceID)
	require.NoError(t, err)
	assert.Len(t, orders, 1, "应有1个interrupted订单")
	assert.Equal(t, 10, orders[0].Status, "状态应为interrupted(10)")
}

// TestOrderStatusTransition_InterruptedToCharging 测试订单状态：interrupted → charging（恢复）
func TestOrderStatusTransition_InterruptedToCharging(t *testing.T) {
	repo := setupTestRepo(t)
	phyID := "TEST_DEVICE_005"
	defer cleanupTestData(t, repo, phyID)

	ctx := context.Background()
	deviceID := createTestDevice(t, repo, phyID)
	orderNo := "ORD005"
	portNo := 1

	// 1. 创建interrupted订单
	createTestOrder(t, repo, deviceID, portNo, orderNo)
	err := repo.UpdateOrderToCharging(ctx, orderNo, time.Now())
	require.NoError(t, err)
	_, err = repo.MarkChargingOrdersAsInterrupted(ctx, deviceID)
	require.NoError(t, err)

	// 2. 恢复订单
	err = repo.RecoverOrder(ctx, orderNo)
	require.NoError(t, err, "恢复订单失败")

	// 3. 验证订单已恢复为charging
	order, err := repo.GetChargingOrderByPort(ctx, deviceID, portNo)
	require.NoError(t, err)
	require.NotNil(t, order, "应有charging订单")
	assert.Equal(t, 2, order.Status, "状态应为charging(2)")
}

// TestOrderStatusTransition_InterruptedToFailed 测试订单状态：interrupted → failed
func TestOrderStatusTransition_InterruptedToFailed(t *testing.T) {
	repo := setupTestRepo(t)
	phyID := "TEST_DEVICE_006"
	defer cleanupTestData(t, repo, phyID)

	ctx := context.Background()
	deviceID := createTestDevice(t, repo, phyID)
	orderNo := "ORD006"
	portNo := 1

	// 1. 创建interrupted订单
	createTestOrder(t, repo, deviceID, portNo, orderNo)
	err := repo.UpdateOrderToCharging(ctx, orderNo, time.Now())
	require.NoError(t, err)
	_, err = repo.MarkChargingOrdersAsInterrupted(ctx, deviceID)
	require.NoError(t, err)

	// 2. 标记为failed
	err = repo.FailOrder(ctx, orderNo, "device_offline_timeout")
	require.NoError(t, err, "标记failed失败")

	// 3. 验证端口不再有charging或interrupted订单
	chargingOrder, err := repo.GetChargingOrderByPort(ctx, deviceID, portNo)
	assert.NoError(t, err)
	assert.Nil(t, chargingOrder, "端口不应再有charging订单")

	interruptedOrders, err := repo.GetInterruptedOrders(ctx, deviceID)
	assert.NoError(t, err)
	assert.Empty(t, interruptedOrders, "不应有interrupted订单")
}

// TestOrderConcurrency_PortOccupation 测试并发场景：端口占用检查
func TestOrderConcurrency_PortOccupation(t *testing.T) {
	repo := setupTestRepo(t)
	phyID := "TEST_DEVICE_007"
	defer cleanupTestData(t, repo, phyID)

	ctx := context.Background()
	deviceID := createTestDevice(t, repo, phyID)
	portNo := 1

	// 1. 创建第一个charging订单
	orderNo1 := "ORD007_1"
	createTestOrder(t, repo, deviceID, portNo, orderNo1)
	err := repo.UpdateOrderToCharging(ctx, orderNo1, time.Now())
	require.NoError(t, err)

	// 2. 尝试在同一端口创建第二个订单（应该能创建，但不应变为charging）
	orderNo2 := "ORD007_2"
	createTestOrder(t, repo, deviceID, portNo, orderNo2)

	// 3. 验证只有第一个订单在charging
	chargingOrder, err := repo.GetChargingOrderByPort(ctx, deviceID, portNo)
	require.NoError(t, err)
	require.NotNil(t, chargingOrder)
	assert.Equal(t, orderNo1, chargingOrder.OrderNo, "应该是第一个订单在charging")
}

// TestDevicePortStatus_CheckBeforeCreateOrder 测试设备端口状态检查
func TestDevicePortStatus_CheckBeforeCreateOrder(t *testing.T) {
	repo := setupTestRepo(t)
	phyID := "TEST_DEVICE_008"
	defer cleanupTestData(t, repo, phyID)

	ctx := context.Background()
	deviceID := createTestDevice(t, repo, phyID)
	portNo := 1

	// 1. 获取设备信息（验证设备存在）
	device, err := repo.GetDeviceByPhyID(ctx, phyID)
	require.NoError(t, err)
	require.NotNil(t, device)
	assert.Equal(t, phyID, device.PhyID)

	// 2. 检查端口是否有pending订单（应该没有）
	pendingOrder, err := repo.GetPendingOrderByPort(ctx, deviceID, portNo)
	assert.NoError(t, err)
	assert.Nil(t, pendingOrder, "新设备端口不应有pending订单")

	// 3. 检查端口是否有charging订单（应该没有）
	chargingOrder, err := repo.GetChargingOrderByPort(ctx, deviceID, portNo)
	assert.NoError(t, err)
	assert.Nil(t, chargingOrder, "新设备端口不应有charging订单")
}

// TestOrderQuery_GetOrderByID 测试订单查询
func TestOrderQuery_GetOrderByID(t *testing.T) {
	repo := setupTestRepo(t)
	phyID := "TEST_DEVICE_009"
	defer cleanupTestData(t, repo, phyID)

	ctx := context.Background()
	deviceID := createTestDevice(t, repo, phyID)
	orderNo := "ORD009"
	portNo := 1

	// 1. 创建订单
	createTestOrder(t, repo, deviceID, portNo, orderNo)

	// 2. 获取订单
	order, err := repo.GetPendingOrderByPort(ctx, deviceID, portNo)
	require.NoError(t, err)
	require.NotNil(t, order)

	// 3. 通过ID查询订单
	orderByID, err := repo.GetOrderByID(ctx, order.ID)
	require.NoError(t, err)
	require.NotNil(t, orderByID)

	// 4. 验证字段
	assert.Equal(t, order.ID, orderByID.ID)
	assert.Equal(t, phyID, orderByID.PhyID)
	assert.Equal(t, portNo, orderByID.PortNo)
	assert.Equal(t, deviceID, orderByID.DeviceID)
}

// TestOrderList_ListOrdersByPhyID 测试订单列表查询
func TestOrderList_ListOrdersByPhyID(t *testing.T) {
	repo := setupTestRepo(t)
	phyID := "TEST_DEVICE_010"
	defer cleanupTestData(t, repo, phyID)

	ctx := context.Background()
	deviceID := createTestDevice(t, repo, phyID)

	// 1. 创建多个订单
	createTestOrder(t, repo, deviceID, 1, "ORD010_1")
	createTestOrder(t, repo, deviceID, 2, "ORD010_2")

	// 2. 查询订单列表
	orders, err := repo.ListOrdersByPhyID(ctx, phyID, 10, 0)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(orders), 2, "应至少有2个订单")

	// 3. 验证订单属于正确的设备
	for _, order := range orders {
		assert.Equal(t, phyID, order.PhyID)
	}
}
