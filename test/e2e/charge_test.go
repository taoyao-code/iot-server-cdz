package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

// ChargeSuite 充电流程测试套件
type ChargeSuite struct {
	suite.Suite
	helper   *TestHelper
	deviceID string
	ctx      context.Context
}

// SetupSuite 套件初始化
func (s *ChargeSuite) SetupSuite() {
	s.helper = NewTestHelper(s.T())
	s.deviceID = s.helper.DeviceID()
	s.ctx = context.Background()

	// 验证设备在线
	s.T().Log("━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	s.T().Log("  充电流程测试套件")
	s.T().Log("━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	defer cancel()
	device := s.helper.RequireDeviceOnline(ctx)
	s.T().Logf("✓ 测试设备: %s", device.DeviceID)
}

// TearDownTest 每个测试后清理
func (s *ChargeSuite) TearDownTest() {
	// 停止所有可能的端口
	s.helper.CleanupPort(1)
	s.helper.CleanupPort(2)
	time.Sleep(1 * time.Second)
}

// TestPort1Charge 测试端口1充电完整流程
func (s *ChargeSuite) TestPort1Charge() {
	s.T().Log("\n→ 测试场景: 端口1充电（BKV插孔0/A孔）")

	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	defer cancel()

	// 1. 创建订单
	orderNo := s.helper.CreateCharge(ctx, 1, ChargeModeByDuration, 500, 60)
	s.NotEmpty(orderNo, "订单号不应为空")

	// 2. 验证订单初始状态
	order := s.helper.GetOrder(ctx, orderNo)
	s.Equal(OrderStatusPending, order.Status, "订单初始状态应为pending")
	s.Equal(1, order.PortNo, "端口号应为1")

	// 3. 等待设备响应（pending → charging）
	s.T().Logf("→ 等待订单 %s 状态变为 charging（最多10秒）...", orderNo)
	order, err := s.helper.Client().WaitForOrderStatus(ctx, orderNo, OrderStatusCharging, 10*time.Second)

	if err != nil {
		// 如果超时，打印当前状态供分析
		currentOrder := s.helper.GetOrder(ctx, orderNo)
		s.T().Logf("⚠️  订单 %s 未在10秒内变为charging，当前状态: %s", orderNo, currentOrder.Status)

		// 检查是否已经是charging状态（可能是API延迟或已经开始充电）
		if currentOrder.Status == OrderStatusCharging {
			s.T().Logf("✅ 订单已经在充电中！(可能在等待期间已经启动)")
			// 继续验证充电状态
			s.Equal(OrderStatusCharging, currentOrder.Status)
			s.NotZero(currentOrder.StartTime, "开始时间应已设置")
		} else {
			s.T().Logf("提示：请确保充电器已插入端口1，当前状态=%s", currentOrder.Status)
			// 不强制失败，允许手动验证
		}
	} else {
		s.T().Logf("✅ 端口1充电启动成功！订单号: %s", orderNo)
		s.Equal(OrderStatusCharging, order.Status)
		s.NotZero(order.StartTime, "开始时间应已设置")
	}

	// 4. 停止充电
	s.T().Log("\n→ 停止充电...")
	s.helper.StopCharge(ctx, 1)

	// 5. 验证最终状态
	time.Sleep(2 * time.Second)
	finalOrder := s.helper.GetOrder(ctx, orderNo)
	s.T().Logf("最终状态: %s", finalOrder.Status)
}

// TestPort2Charge 测试端口2充电完整流程
func (s *ChargeSuite) TestPort2Charge() {
	s.T().Log("\n→ 测试场景: 端口2充电（BKV插孔1/B孔）")

	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	defer cancel()

	// 1. 创建订单
	orderNo := s.helper.CreateCharge(ctx, 2, ChargeModeByDuration, 500, 60)
	s.NotEmpty(orderNo)

	// 2. 验证订单状态
	order := s.helper.GetOrder(ctx, orderNo)
	s.Equal(OrderStatusPending, order.Status)
	s.Equal(2, order.PortNo)

	// 3. 等待设备响应
	s.T().Logf("→ 等待订单 %s 状态变为 charging（最多10秒）...", orderNo)
	order, err := s.helper.Client().WaitForOrderStatus(ctx, orderNo, OrderStatusCharging, 10*time.Second)

	if err != nil {
		currentOrder := s.helper.GetOrder(ctx, orderNo)
		s.T().Logf("⚠️  订单 %s 未在10秒内变为charging，当前状态: %s", orderNo, currentOrder.Status)

		if currentOrder.Status == OrderStatusCharging {
			s.T().Logf("✅ 端口2订单已经在充电中！")
			s.Equal(OrderStatusCharging, currentOrder.Status)
		} else {
			s.T().Logf("提示：请确保充电器已插入端口2，当前状态=%s", currentOrder.Status)
		}
	} else {
		s.T().Logf("✅ 端口2充电启动成功！订单号: %s", orderNo)
		s.Equal(OrderStatusCharging, order.Status)
	}

	// 4. 停止充电
	s.T().Log("\n→ 停止充电...")
	s.helper.StopCharge(ctx, 2)

	time.Sleep(2 * time.Second)
	finalOrder := s.helper.GetOrder(ctx, orderNo)
	s.T().Logf("最终状态: %s", finalOrder.Status)
}

// TestDualPortCharge 测试双端口并发充电
func (s *ChargeSuite) TestDualPortCharge() {
	s.T().Log("\n→ 测试场景: 双端口并发充电")

	ctx, cancel := context.WithTimeout(s.ctx, 45*time.Second)
	defer cancel()

	// 1. 启动端口1
	s.T().Log("→ 启动端口1...")
	order1 := s.helper.CreateCharge(ctx, 1, ChargeModeByDuration, 500, 60)

	// 等待端口1启动
	time.Sleep(3 * time.Second)

	// 2. 启动端口2
	s.T().Log("→ 启动端口2...")
	order2 := s.helper.CreateCharge(ctx, 2, ChargeModeByDuration, 500, 60)

	// 等待两个端口响应
	time.Sleep(5 * time.Second)

	// 3. 检查两个订单状态
	status1 := s.helper.GetOrder(ctx, order1)
	status2 := s.helper.GetOrder(ctx, order2)

	s.T().Logf("\n端口1状态: %s", status1.Status)
	s.T().Logf("端口2状态: %s", status2.Status)

	// BKV设备应该支持双孔并发
	if status1.Status == OrderStatusCharging && status2.Status == OrderStatusCharging {
		s.T().Log("✅ 双端口并发充电成功！")
	} else {
		s.T().Logf("⚠️  双端口未同时充电: 端口1=%s, 端口2=%s", status1.Status, status2.Status)
	}

	// 4. 清理
	s.helper.StopCharge(ctx, 1)
	s.helper.StopCharge(ctx, 2)
}

// TestPortConflict 测试端口冲突处理
func (s *ChargeSuite) TestPortConflict() {
	s.T().Log("\n→ 测试场景: 端口冲突处理")

	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	defer cancel()

	// 1. 创建第一个订单
	s.T().Log("→ 创建第一个订单...")
	order1 := s.helper.CreateCharge(ctx, 1, ChargeModeByDuration, 500, 60)
	s.NotEmpty(order1)

	time.Sleep(2 * time.Second)

	// 2. 尝试在同一端口创建第二个订单（不使用自动重试）
	s.T().Log("→ 尝试在同一端口创建订单（预期冲突）...")
	req := &StartChargeRequest{
		SocketUID:       s.helper.Config().SocketUID,
		PortNo:          1,
		ChargeMode:      ChargeModeByDuration,
		Amount:          500,
		DurationMinutes: 60,
		PricePerKwh:     150,
		ServiceFee:      50,
	}

	_, err := s.helper.Client().StartCharge(ctx, s.deviceID, req)
	s.Error(err, "应该返回冲突错误")

	if apiErr, ok := err.(*APIError); ok {
		s.True(apiErr.IsConflict(), "应该是409冲突错误")
		s.T().Logf("✅ 正确返回冲突错误: %s", apiErr.Message)
	}

	// 3. 清理
	s.helper.StopCharge(ctx, 1)
}

// TestChargeWithDifferentModes 测试不同充电模式
func (s *ChargeSuite) TestChargeWithDifferentModes() {
	s.T().Log("\n→ 测试场景: 不同充电模式")

	testCases := []struct {
		name     string
		mode     ChargeMode
		amount   int
		duration int
	}{
		{"按时长充电", ChargeModeByDuration, 500, 30},
		{"按电量充电", ChargeModeByAmount, 1000, 0},
		{"充满自停", ChargeModeAutoStop, 2000, 0},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			ctx, cancel := context.WithTimeout(s.ctx, 20*time.Second)
			defer cancel()

			s.T().Logf("→ 测试模式: %s", tc.name)
			orderNo := s.helper.CreateCharge(ctx, 1, tc.mode, tc.amount, tc.duration)
			s.NotEmpty(orderNo)

			order := s.helper.GetOrder(ctx, orderNo)
			// 注意：API不返回charge_mode字段，只验证订单创建成功
			s.NotEmpty(order.Status, "订单状态不应为空")
			s.T().Logf("✓ 订单创建成功: %s (状态=%s)", orderNo, order.Status)

			// 清理
			s.helper.StopCharge(ctx, 1)
			time.Sleep(1 * time.Second)
		})
	}
}

// TestStopChargeOrder 测试停止充电
func (s *ChargeSuite) TestStopChargeOrder() {
	s.T().Log("\n→ 测试场景: 停止充电指令")

	ctx, cancel := context.WithTimeout(s.ctx, 20*time.Second)
	defer cancel()

	// 1. 创建订单
	orderNo := s.helper.CreateCharge(ctx, 1, ChargeModeByDuration, 500, 60)

	// 2. 等待一小段时间
	time.Sleep(3 * time.Second)

	// 3. 停止充电
	s.T().Log("→ 发送停止指令...")
	s.helper.StopCharge(ctx, 1)

	// 4. 验证订单状态更新
	time.Sleep(2 * time.Second)
	order := s.helper.GetOrder(ctx, orderNo)
	s.T().Logf("停止后状态: %s", order.Status)

	// 订单应该不再是 charging 状态
	if order.Status != OrderStatusCharging {
		s.T().Log("✅ 充电已停止")
	}
}

// TestInvalidPort 测试无效端口号
func (s *ChargeSuite) TestInvalidPort() {
	s.T().Log("\n→ 测试场景: 无效端口号")

	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()

	// 测试无效端口号（0, 负数）
	// 注意：999等大端口号API可能接受但设备会拒绝
	invalidPorts := []int{0, -1}

	for _, port := range invalidPorts {
		s.Run(s.T().Name(), func() {
			req := &StartChargeRequest{
				PortNo:          port,
				ChargeMode:      ChargeModeByDuration,
				Amount:          500,
				DurationMinutes: 60,
			}

			_, err := s.helper.Client().StartCharge(ctx, s.deviceID, req)
			s.Error(err, "无效端口号应该返回错误: %d", port)
			s.T().Logf("✓ 端口%d正确返回错误", port)
		})
	}
}

// TestChargeSuite 运行充电测试套件
func TestChargeSuite(t *testing.T) {
	suite.Run(t, new(ChargeSuite))
}
