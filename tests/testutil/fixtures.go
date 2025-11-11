package testutil

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestDevice 测试设备数据
type TestDevice struct {
	ID    int64
	PhyID string
}

// TestPort 测试端口数据
type TestPort struct {
	DeviceID int64
	PortNo   int
	Status   int
	PowerW   *int
}

// TestOrder 测试订单数据
type TestOrder struct {
	ID        int64
	DeviceID  int64
	PortNo    int
	OrderNo   string
	Status    int
	StartTime time.Time
}

// CreateTestDevice 创建测试设备
func CreateTestDevice(t *testing.T, pool *pgxpool.Pool, phyID string) *TestDevice {
	t.Helper()

	if phyID == "" {
		phyID = fmt.Sprintf("TEST_%s", uuid.New().String()[:8])
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	const q = `INSERT INTO devices (phy_id, last_seen_at) 
	           VALUES ($1, NOW()) 
	           RETURNING id`

	var id int64
	err := pool.QueryRow(ctx, q, phyID).Scan(&id)
	RequireNoError(t, err, "failed to create test device")

	return &TestDevice{
		ID:    id,
		PhyID: phyID,
	}
}

// CreateTestPort 创建测试端口
func CreateTestPort(t *testing.T, pool *pgxpool.Pool, deviceID int64, portNo int, status int) *TestPort {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	const q = `INSERT INTO ports (device_id, port_no, status, updated_at) 
	           VALUES ($1, $2, $3, NOW())`

	_, err := pool.Exec(ctx, q, deviceID, portNo, status)
	RequireNoError(t, err, "failed to create test port")

	return &TestPort{
		DeviceID: deviceID,
		PortNo:   portNo,
		Status:   status,
		PowerW:   nil,
	}
}

// CreateTestOrder 创建测试订单
func CreateTestOrder(t *testing.T, pool *pgxpool.Pool, deviceID int64, portNo int, status int) *TestOrder {
	t.Helper()

	orderNo := fmt.Sprintf("TEST_%s", uuid.New().String()[:16])

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	const q = `INSERT INTO orders (device_id, port_no, order_no, start_time, status) 
	           VALUES ($1, $2, $3, NOW(), $4) 
	           RETURNING id, start_time`

	var id int64
	var startTime time.Time
	err := pool.QueryRow(ctx, q, deviceID, portNo, orderNo, status).Scan(&id, &startTime)
	RequireNoError(t, err, "failed to create test order")

	return &TestOrder{
		ID:        id,
		DeviceID:  deviceID,
		PortNo:    portNo,
		OrderNo:   orderNo,
		Status:    status,
		StartTime: startTime,
	}
}

// CreateTestCard 创建测试卡号
func CreateTestCard(t *testing.T, pool *pgxpool.Pool, cardNo string) int64 {
	t.Helper()

	if cardNo == "" {
		cardNo = fmt.Sprintf("TEST_CARD_%s", uuid.New().String()[:12])
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	const q = `INSERT INTO cards (card_no, status, created_at) 
	           VALUES ($1, 1, NOW()) 
	           ON CONFLICT (card_no) DO UPDATE SET updated_at = NOW()
	           RETURNING id`

	var id int64
	err := pool.QueryRow(ctx, q, cardNo).Scan(&id)
	RequireNoError(t, err, "failed to create test card")

	return id
}

// GetOrderByID 获取订单（用于测试验证）
func GetOrderByID(t *testing.T, pool *pgxpool.Pool, id int64) *TestOrder {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	const q = `SELECT id, device_id, port_no, order_no, status, start_time 
	           FROM orders WHERE id = $1`

	var order TestOrder
	err := pool.QueryRow(ctx, q, id).Scan(
		&order.ID,
		&order.DeviceID,
		&order.PortNo,
		&order.OrderNo,
		&order.Status,
		&order.StartTime,
	)
	RequireNoError(t, err, "failed to get order")

	return &order
}

// GetPortStatus 获取端口状态（用于测试验证）
func GetPortStatus(t *testing.T, pool *pgxpool.Pool, deviceID int64, portNo int) int {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	const q = `SELECT status FROM ports WHERE device_id = $1 AND port_no = $2`

	var status int
	err := pool.QueryRow(ctx, q, deviceID, portNo).Scan(&status)
	RequireNoError(t, err, "failed to get port status")

	return status
}

// CountOrders 统计订单数量（用于测试验证）
func CountOrders(t *testing.T, pool *pgxpool.Pool, deviceID int64) int {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	const q = `SELECT COUNT(*) FROM orders WHERE device_id = $1`

	var count int
	err := pool.QueryRow(ctx, q, deviceID).Scan(&count)
	RequireNoError(t, err, "failed to count orders")

	return count
}
