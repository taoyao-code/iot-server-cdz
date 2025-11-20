package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestAutoStopByDuration 按时长自动停止完整流程：
// StartCharge(按时长) → 设备自动按时长到期停止 → BKV充电结束上报驱动 SettleOrder →
// 订单终态=completed，端口状态收敛为idle
func TestAutoStopByDuration(t *testing.T) {
	helper := NewTestHelper(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	deviceID := helper.DeviceID()

	// 1. 确保设备在线
	device := helper.RequireDeviceOnline(ctx)
	require.Equal(t, deviceID, device.DeviceID)

	// 2. 启动按时长充电（1分钟），依赖设备自动停止
	t.Log("→ 启动按时长自动停止充电 (1分钟)...")
	orderNo := helper.CreateCharge(ctx, 1, ChargeModeByDuration, 100, 1)
	require.NotEmpty(t, orderNo)

	// 3. 等待订单进入charging
	t.Log("→ 等待订单进入charging状态...")
	_ = helper.WaitForOrderStatus(ctx, orderNo, OrderStatusCharging, 30*time.Second)

	// 4. 等待设备按时长自动停止：
	//    - 逻辑上应在1分钟左右触发充电结束上报 + 结算
	//    - 这里给足一点缓冲时间（总共最多等 90 秒）
	t.Log("→ 等待设备按时长自动停止并结算订单 (最多90秒)...")
	deadlineCtx, cancelWait := context.WithTimeout(ctx, 90*time.Second)
	defer cancelWait()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	var finalOrder *OrderInfo
	for {
		select {
		case <-deadlineCtx.Done():
			t.Fatalf("订单在期望时间内未自动结算: %s", orderNo)
		case <-ticker.C:
			cur := helper.GetOrder(ctx, orderNo)
			t.Logf("当前订单状态: %s", cur.Status)
			if cur.Status == OrderStatusCompleted {
				finalOrder = cur
				t.Logf("✅ 订单已自动结算完成: %s", orderNo)
				goto checkPort
			}
		}
	}

checkPort:
	require.NotNil(t, finalOrder, "最终订单不应为空")

	// 5. 再查一次设备，确认端口不再处于充电/占用状态
	t.Log("→ 校验设备端口状态已释放...")
	deviceAfter, err := helper.Client().GetDevice(ctx, deviceID)
	require.NoError(t, err)
	require.NotNil(t, deviceAfter)

	if deviceAfter.ActiveOrder != nil && deviceAfter.ActiveOrder.OrderNo == orderNo {
		t.Fatalf("设备仍然认为订单处于活动状态: %s", orderNo)
	}

	t.Log("✅ 按时长自动停止 E2E 测试通过")
}
