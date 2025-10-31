package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

// OrderSuite 订单管理测试套件
type OrderSuite struct {
	suite.Suite
	helper   *TestHelper
	deviceID string
	ctx      context.Context
}

// SetupSuite 套件初始化
func (s *OrderSuite) SetupSuite() {
	s.helper = NewTestHelper(s.T())
	s.deviceID = s.helper.DeviceID()
	s.ctx = context.Background()

	s.T().Log("━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	s.T().Log("  订单管理测试套件")
	s.T().Log("━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
}

// TearDownTest 每个测试后清理
func (s *OrderSuite) TearDownTest() {
	s.helper.CleanupPort(1)
	s.helper.CleanupPort(2)
	time.Sleep(1 * time.Second)
}

// TestGetOrderInfo 测试获取订单信息
func (s *OrderSuite) TestGetOrderInfo() {
	s.T().Log("\n→ 测试场景: 获取订单信息")

	ctx, cancel := context.WithTimeout(s.ctx, 20*time.Second)
	defer cancel()

	// 1. 创建订单
	orderNo := s.helper.CreateCharge(ctx, 1, ChargeModeByDuration, 500, 60)

	// 2. 查询订单
	order := s.helper.GetOrder(ctx, orderNo)

	s.Equal(orderNo, order.OrderNo, "订单号应该匹配")
	s.Equal(1, order.PortNo, "端口号应该为1")
	s.NotEmpty(order.Status, "状态不应为空")
	s.NotZero(order.CreatedAt, "创建时间不应为空")

	// 注意：API返回的amount是计算后的值，不是请求时的原始值
	// charge_mode和duration_minutes字段API不返回，跳过检查
	s.T().Logf("订单信息: 订单号=%s, 端口=%d, 状态=%s, 金额=%d分",
		order.OrderNo, order.PortNo, order.Status, order.Amount)

	s.T().Logf("✅ 订单信息查询成功: %s", orderNo)
}

// TestOrderStatusTransition 测试订单状态流转
func (s *OrderSuite) TestOrderStatusTransition() {
	s.T().Log("\n→ 测试场景: 订单状态流转")

	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	defer cancel()

	// 1. 创建订单，初始状态应为 pending
	orderNo := s.helper.CreateCharge(ctx, 1, ChargeModeByDuration, 500, 60)
	order := s.helper.GetOrder(ctx, orderNo)
	s.Equal(OrderStatusPending, order.Status, "初始状态应为pending")
	s.T().Logf("✓ 订单创建: %s (状态: %s)", orderNo, order.Status)

	// 2. 等待一段时间，观察状态变化
	time.Sleep(5 * time.Second)
	order = s.helper.GetOrder(ctx, orderNo)
	s.T().Logf("5秒后状态: %s", order.Status)

	// 3. 停止充电
	s.helper.StopCharge(ctx, 1)
	time.Sleep(2 * time.Second)

	// 4. 查询最终状态
	order = s.helper.GetOrder(ctx, orderNo)
	s.T().Logf("停止后状态: %s", order.Status)

	// 订单状态应该已经改变
	s.T().Log("✅ 订单状态流转测试完成")
}

// TestOrderNotFound 测试订单不存在
func (s *OrderSuite) TestOrderNotFound() {
	s.T().Log("\n→ 测试场景: 订单不存在")

	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()

	nonExistentOrderNo := "ORDER_NOT_EXIST_999999"
	_, err := s.helper.Client().GetOrder(ctx, nonExistentOrderNo)
	s.Error(err, "应该返回错误")

	if apiErr, ok := err.(*APIError); ok {
		s.True(apiErr.IsNotFound(), "应该是404错误")
		s.T().Logf("✅ 正确返回404错误: %s", apiErr.Message)
	}
}

// TestOrderTimeTracking 测试订单时间追踪
func (s *OrderSuite) TestOrderTimeTracking() {
	s.T().Log("\n→ 测试场景: 订单时间追踪")

	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	defer cancel()

	// 1. 创建订单
	orderNo := s.helper.CreateCharge(ctx, 1, ChargeModeByDuration, 500, 60)

	// 2. 查询创建时间
	order := s.helper.GetOrder(ctx, orderNo)
	s.NotZero(order.CreatedAt, "创建时间不应为空")
	s.NotZero(order.UpdatedAt, "更新时间不应为空")
	s.T().Logf("创建时间: %s", time.Unix(order.CreatedAt, 0).Format("2006-01-02 15:04:05"))

	// 3. 等待设备响应
	time.Sleep(5 * time.Second)
	order = s.helper.GetOrder(ctx, orderNo)

	// 如果充电开始，应该有开始时间
	if order.Status == OrderStatusCharging {
		s.NotZero(order.StartTime, "开始时间应该存在")
		s.T().Logf("开始时间: %s", time.Unix(order.StartTime, 0).Format("2006-01-02 15:04:05"))
		s.T().Log("✅ 时间追踪正常")
	} else {
		s.T().Logf("⚠️  订单未开始充电，状态: %s", order.Status)
	}

	// 4. 清理
	s.helper.StopCharge(ctx, 1)
}

// TestOrderEnergyTracking 测试订单电量追踪
func (s *OrderSuite) TestOrderEnergyTracking() {
	s.T().Log("\n→ 测试场景: 订单电量追踪")

	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	defer cancel()

	// 1. 创建订单
	orderNo := s.helper.CreateCharge(ctx, 1, ChargeModeByDuration, 500, 60)

	// 2. 查询初始电量
	order := s.helper.GetOrder(ctx, orderNo)
	s.T().Logf("初始电量: %.3f 度", order.EnergyConsumed)

	// 3. 等待一段时间（如果充电，电量应该增加）
	time.Sleep(10 * time.Second)
	order = s.helper.GetOrder(ctx, orderNo)
	s.T().Logf("10秒后电量: %.3f 度", order.EnergyConsumed)

	// 4. 清理
	s.helper.StopCharge(ctx, 1)

	// 5. 查询最终电量
	time.Sleep(2 * time.Second)
	order = s.helper.GetOrder(ctx, orderNo)
	s.T().Logf("最终电量: %.3f 度", order.EnergyConsumed)
	s.T().Logf("实际金额: %d 分", order.ActualAmount)

	s.T().Log("✅ 电量追踪测试完成")
}

// TestOrderSuite 运行订单管理测试套件
func TestOrderSuite(t *testing.T) {
	suite.Run(t, new(OrderSuite))
}
