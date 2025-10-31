package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestHelper 测试辅助工具
type TestHelper struct {
	t      *testing.T
	client *APIClient
	config *Config
}

// NewTestHelper 创建测试辅助工具
func NewTestHelper(t *testing.T) *TestHelper {
	cfg := GetConfig()
	if cfg.Verbose {
		t.Logf("Test Configuration:\n%s", cfg.String())
	}
	return &TestHelper{
		t:      t,
		client: NewAPIClient(cfg),
		config: cfg,
	}
}

// Client 获取API客户端
func (h *TestHelper) Client() *APIClient {
	return h.client
}

// Config 获取配置
func (h *TestHelper) Config() *Config {
	return h.config
}

// DeviceID 获取测试设备ID
func (h *TestHelper) DeviceID() string {
	return h.config.TestDeviceID
}

// RequireDeviceOnline 确保设备在线
func (h *TestHelper) RequireDeviceOnline(ctx context.Context) *DeviceInfo {
	h.t.Helper()

	device, err := h.client.GetDevice(ctx, h.config.TestDeviceID)
	require.NoError(h.t, err, "获取设备信息失败")
	require.NotNil(h.t, device, "设备信息为空")

	// 计算离线时长
	offlineSeconds := time.Now().Unix() - device.LastSeenAt

	// 如果设备离线
	if !device.Online {
		h.t.Logf("⚠️  设备当前显示离线")
		h.t.Logf("   最后心跳: %d 秒前", offlineSeconds)

		// 如果离线超过60秒，跳过需要设备响应的测试
		if offlineSeconds > 60 {
			h.t.Logf("❌ 设备离线时间过长（%d秒 > 60秒）", offlineSeconds)
			h.t.Logf("   说明: 当前允许离线下单，但订单会一直pending")
			h.t.Logf("   建议: 1) 检查设备网络连接  2) 重启设备  3) 查看服务端日志")
			h.t.Skip("设备离线超过60秒，跳过需要设备响应的测试")
		}

		// 短暂离线（<60秒），可能很快恢复
		h.t.Logf("   设备短暂离线（%d秒），继续测试（可能需要等待设备上线）...", offlineSeconds)

		// 如果有活跃订单，说明设备之前在工作
		if device.ActiveOrder != nil {
			h.t.Logf("   发现活跃订单: %s (端口%d)", device.ActiveOrder.OrderNo, device.ActiveOrder.PortNo)
		}
	} else if h.config.Verbose {
		h.t.Logf("✓ 设备在线: %s (状态: %s)", device.DeviceID, device.Status)
		h.t.Logf("  最后心跳: %d 秒前", offlineSeconds)
	}

	return device
} // CreateCharge 创建充电订单（自动处理冲突）
func (h *TestHelper) CreateCharge(ctx context.Context, portNo int, mode ChargeMode, amount, duration int) string {
	h.t.Helper()

	req := &StartChargeRequest{
		PortNo:          portNo,
		ChargeMode:      mode,
		Amount:          amount,
		DurationMinutes: duration,
		PricePerKwh:     150, // 1.5元/度
		ServiceFee:      50,  // 5% 服务费
	}

	if h.config.Verbose {
		h.t.Logf("→ 创建充电订单: 端口=%d, 模式=%d, 金额=%d分, 时长=%d分钟",
			portNo, mode, amount, duration)
	}

	// 使用自动冲突重试
	resp, err := h.client.RetryOnConflict(ctx, h.config.TestDeviceID, req)
	require.NoError(h.t, err, "创建充电订单失败")
	require.NotEmpty(h.t, resp.OrderNo, "订单号为空")

	if h.config.Verbose {
		h.t.Logf("✓ 订单创建成功: %s", resp.OrderNo)
	}

	return resp.OrderNo
}

// StopCharge 停止充电
func (h *TestHelper) StopCharge(ctx context.Context, portNo int) {
	h.t.Helper()

	if h.config.Verbose {
		h.t.Logf("→ 停止端口%d充电", portNo)
	}

	err := h.client.StopCharge(ctx, h.config.TestDeviceID, portNo)
	require.NoError(h.t, err, "停止充电失败")

	if h.config.Verbose {
		h.t.Logf("✓ 停止充电成功")
	}
}

// GetOrder 获取订单信息
func (h *TestHelper) GetOrder(ctx context.Context, orderNo string) *OrderInfo {
	h.t.Helper()

	order, err := h.client.GetOrder(ctx, orderNo)
	require.NoError(h.t, err, "获取订单信息失败")
	require.NotNil(h.t, order, "订单信息为空")

	return order
}

// WaitForOrderStatus 等待订单状态
func (h *TestHelper) WaitForOrderStatus(ctx context.Context, orderNo string, expectedStatus OrderStatus, timeout time.Duration) *OrderInfo {
	h.t.Helper()

	if h.config.Verbose {
		h.t.Logf("→ 等待订单 %s 状态变为 %s (超时: %s)", orderNo, expectedStatus, timeout)
	}

	order, err := h.client.WaitForOrderStatus(ctx, orderNo, expectedStatus, timeout)
	require.NoError(h.t, err, fmt.Sprintf("等待订单状态失败: %v", err))

	if h.config.Verbose {
		h.t.Logf("✓ 订单状态已变为: %s", order.Status)
	}

	return order
}

// AssertOrderStatus 断言订单状态
func (h *TestHelper) AssertOrderStatus(ctx context.Context, orderNo string, expectedStatus OrderStatus) {
	h.t.Helper()

	order := h.GetOrder(ctx, orderNo)
	require.Equal(h.t, expectedStatus, order.Status,
		"订单状态不符合预期: expected=%s, actual=%s", expectedStatus, order.Status)
}

// CleanupPort 清理端口（停止充电）
func (h *TestHelper) CleanupPort(portNo int) {
	h.t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 忽略错误，因为可能端口本来就没有在充电
	_ = h.client.StopCharge(ctx, h.config.TestDeviceID, portNo)

	if h.config.Verbose {
		h.t.Logf("✓ 端口%d已清理", portNo)
	}
}

// CreateTestCharge 创建测试充电订单的快捷方法（默认参数）
func (h *TestHelper) CreateTestCharge(ctx context.Context, portNo int) string {
	return h.CreateCharge(ctx, portNo, ChargeModeByDuration, 500, 60)
}
