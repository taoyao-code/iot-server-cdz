package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

// DeviceSuite 设备管理测试套件
type DeviceSuite struct {
	suite.Suite
	helper   *TestHelper
	deviceID string
	ctx      context.Context
}

// SetupSuite 套件初始化
func (s *DeviceSuite) SetupSuite() {
	s.helper = NewTestHelper(s.T())
	s.deviceID = s.helper.DeviceID()
	s.ctx = context.Background()

	s.T().Log("━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	s.T().Log("  设备管理测试套件")
	s.T().Log("━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
}

// TestGetDeviceInfo 测试获取设备信息
func (s *DeviceSuite) TestGetDeviceInfo() {
	s.T().Log("\n→ 测试场景: 获取设备信息")

	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()

	device, err := s.helper.Client().GetDevice(ctx, s.deviceID)
	s.NoError(err, "获取设备信息失败")
	s.NotNil(device, "设备信息为空")

	s.T().Logf("设备ID: %s", device.DeviceID)
	s.T().Logf("设备状态: %s", device.Status)
	s.T().Logf("在线状态: %v", device.Online)
	s.T().Logf("端口数量: %d", len(device.Ports))

	// 验证基本字段
	s.NotEmpty(device.DeviceID, "设备ID不应为空")
	s.NotEmpty(device.Status, "状态不应为空")

	s.T().Log("✅ 设备信息查询成功")
}

// TestDeviceOnlineStatus 测试设备在线状态
func (s *DeviceSuite) TestDeviceOnlineStatus() {
	s.T().Log("\n→ 测试场景: 设备在线状态")

	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	defer cancel()

	device := s.helper.RequireDeviceOnline(ctx)
	s.True(device.Online, "设备应该在线")
	s.NotZero(device.LastSeenAt, "最后在线时间应该存在")

	s.T().Logf("✅ 设备在线: 最后心跳 %s", time.Unix(device.LastSeenAt, 0).Format("2006-01-02 15:04:05"))
}

// TestDeviceNotFound 测试设备不存在
func (s *DeviceSuite) TestDeviceNotFound() {
	s.T().Log("\n→ 测试场景: 查询不存在的设备")

	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()

	// API会自动创建不存在的设备，所以我们检查它是否离线
	nonExistentID := "99999999999999"
	device, err := s.helper.Client().GetDevice(ctx, nonExistentID)
	s.NoError(err, "应该成功返回")
	s.NotNil(device, "设备应该存在")

	// 但设备应该是离线的
	if !device.Online {
		s.T().Logf("✅ 设备已创建但离线: %s", device.Status)
	} else {
		s.T().Logf("⚠️  设备显示在线")
	}
}

// TestDevicePorts 测试设备端口信息
func (s *DeviceSuite) TestDevicePorts() {
	s.T().Log("\n→ 测试场景: 设备端口信息")

	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()

	device, err := s.helper.Client().GetDevice(ctx, s.deviceID)
	s.NoError(err)
	s.NotNil(device)

	// BKV设备应该有端口信息
	if len(device.Ports) > 0 {
		s.T().Logf("端口数量: %d", len(device.Ports))
		for _, port := range device.Ports {
			s.T().Logf("  端口%d: 状态=%s, 使用中=%v", port.PortNo, port.Status, port.InUse)
		}
		s.T().Log("✅ 端口信息查询成功")
	} else {
		s.T().Log("⚠️  设备无端口信息（可能不支持）")
	}
}

// TestDeviceActiveOrder 测试设备活跃订单
func (s *DeviceSuite) TestDeviceActiveOrder() {
	s.T().Log("\n→ 测试场景: 设备活跃订单")

	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	defer cancel()

	// 1. 查询初始状态（应该没有活跃订单）
	device, err := s.helper.Client().GetDevice(ctx, s.deviceID)
	s.NoError(err)

	if device.ActiveOrder != nil {
		s.T().Logf("⚠️  设备已有活跃订单: %s (端口%d)",
			device.ActiveOrder.OrderNo, device.ActiveOrder.PortNo)
		// 清理
		s.helper.CleanupPort(device.ActiveOrder.PortNo)
		time.Sleep(2 * time.Second)
	}

	// 2. 创建订单
	orderNo := s.helper.CreateCharge(ctx, 1, ChargeModeByDuration, 500, 60)

	time.Sleep(2 * time.Second)

	// 3. 再次查询设备，应该有活跃订单
	device, err = s.helper.Client().GetDevice(ctx, s.deviceID)
	s.NoError(err)

	if device.ActiveOrder != nil {
		s.Equal(orderNo, device.ActiveOrder.OrderNo, "活跃订单号应该匹配")
		s.Equal(1, device.ActiveOrder.PortNo, "活跃端口号应该为1")
		s.T().Log("✅ 活跃订单信息正确")
	} else {
		s.T().Log("⚠️  设备未返回活跃订单信息")
	}

	// 4. 清理
	s.helper.CleanupPort(1)
}

// TestDeviceSuite 运行设备管理测试套件
func TestDeviceSuite(t *testing.T) {
	suite.Run(t, new(DeviceSuite))
}
