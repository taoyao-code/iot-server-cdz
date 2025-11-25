package app

import (
	"context"
	"time"

	"github.com/taoyao-code/iot-server/internal/api"
	"github.com/taoyao-code/iot-server/internal/coremodel"
	"github.com/taoyao-code/iot-server/internal/driverapi"
	pgstorage "github.com/taoyao-code/iot-server/internal/storage/pg"
	"go.uber.org/zap"
)

// OrderMonitor 订单状态监控器
// 定期检查pending订单超时、charging订单异常等情况
type OrderMonitor struct {
	repo      *pgstorage.Repository
	commands  driverapi.CommandSource
	logger    *zap.Logger
	nowFunc   func() time.Time
	sendLimit int

	checkInterval   time.Duration // 检查间隔
	pendingTimeout  time.Duration // pending订单超时阈值
	chargingTimeout time.Duration // charging订单超时阈值 (异常长时间充电)

	// 统计
	statsChecked      int64
	statsPending      int64
	statsChargingLong int64
}

// Start 启动监控器
func (m *OrderMonitor) Start(ctx context.Context) {
	m.logger.Info("order monitor started",
		zap.Duration("check_interval", m.checkInterval),
		zap.Duration("pending_timeout", m.pendingTimeout),
		zap.Duration("charging_timeout", m.chargingTimeout))

	ticker := time.NewTicker(m.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			m.logger.Info("order monitor stopped",
				zap.Int64("checked", m.statsChecked),
				zap.Int64("pending_alerted", m.statsPending),
				zap.Int64("charging_long_alerted", m.statsChargingLong))
			return
		case <-ticker.C:
			m.check(ctx)
		}
	}
}

// check 执行一次检查
func (m *OrderMonitor) check(ctx context.Context) {
	m.statsChecked++
	m.sendLimit = 50

	// 检查pending订单超时
	m.checkStalePendingOrders(ctx)

	// 检查charging订单异常长时间充电
	m.checkLongChargingOrders(ctx)

	// P0修复: 自动流转中间态订单
	m.cleanupCancellingOrders(ctx)
	m.cleanupStoppingOrders(ctx)
	m.cleanupInterruptedOrders(ctx)
}

// checkStalePendingOrders 检查pending订单超时
// pending订单超过阈值时间仍未转为charging,可能存在问题:
// 1. 设备未接收到指令
// 2. 设备ACK失败
// 3. 网络问题导致ACK丢失
func (m *OrderMonitor) checkStalePendingOrders(ctx context.Context) {
	q := `SELECT o.order_no, o.device_id, d.phy_id, o.port_no, o.created_at 
	      FROM orders o
	      JOIN devices d ON o.device_id = d.id
	      WHERE o.status = 0 AND o.created_at < $1
	      ORDER BY o.created_at ASC
	      LIMIT 100`

	cutoff := m.nowFunc().Add(-m.pendingTimeout)
	rows, err := m.repo.Pool.Query(ctx, q, cutoff)
	if err != nil {
		m.logger.Error("query stale pending orders failed", zap.Error(err))
		return
	}
	defer rows.Close()

	for rows.Next() {
		var orderNo string
		var deviceID int64
		var phyID string
		var portNo int
		var createdAt time.Time

		if err := rows.Scan(&orderNo, &deviceID, &phyID, &portNo, &createdAt); err != nil {
			m.logger.Error("scan pending order failed", zap.Error(err))
			continue
		}

		age := time.Since(createdAt)
		m.statsPending++

		m.logger.Warn("⚠️ pending订单超时",
			zap.String("order_no", orderNo),
			zap.Int64("device_id", deviceID),
			zap.String("phy_id", phyID),
			zap.Int("port_no", portNo),
			zap.Time("created_at", createdAt),
			zap.Duration("age", age),
			zap.String("action", "需要人工介入或自动取消"))

		m.maybeQueryPort(ctx, phyID, portNo, orderNo)
	}
}

// checkLongChargingOrders 检查charging订单异常长时间充电
// charging订单超过阈值时间仍未结束,可能存在问题:
// 1. 设备充电结束未上报
// 2. 订单状态更新失败
// 3. 设备掉线但订单未被标记结束
func (m *OrderMonitor) checkLongChargingOrders(ctx context.Context) {
	q := `SELECT o.order_no, o.device_id, d.phy_id, o.port_no, o.start_time,
	             o.kwh_0p01, o.amount_cent
	      FROM orders o
	      JOIN devices d ON o.device_id = d.id
	      WHERE o.status = 1 AND o.start_time < $1
	      ORDER BY o.start_time ASC
	      LIMIT 100`

	cutoff := m.nowFunc().Add(-m.chargingTimeout)
	rows, err := m.repo.Pool.Query(ctx, q, cutoff)
	if err != nil {
		m.logger.Error("query long charging orders failed", zap.Error(err))
		return
	}
	defer rows.Close()

	for rows.Next() {
		var orderNo string
		var deviceID int64
		var phyID string
		var portNo int
		var startTime *time.Time
		var kwh01 *int
		var amountCent *int

		if err := rows.Scan(&orderNo, &deviceID, &phyID, &portNo, &startTime, &kwh01, &amountCent); err != nil {
			m.logger.Error("scan long charging order failed", zap.Error(err))
			continue
		}

		if startTime == nil {
			continue
		}

		duration := time.Since(*startTime)
		m.statsChargingLong++

		kwhUsed := 0.0
		if kwh01 != nil {
			kwhUsed = float64(*kwh01) / 100.0
		}

		m.logger.Warn("⚠️ charging订单超长时间充电",
			zap.String("order_no", orderNo),
			zap.Int64("device_id", deviceID),
			zap.String("phy_id", phyID),
			zap.Int("port_no", portNo),
			zap.Time("start_time", *startTime),
			zap.Duration("duration", duration),
			zap.Float64("kwh_used", kwhUsed),
			zap.String("action", "建议检查设备状态或手动结算"))

		m.maybeQueryPort(ctx, phyID, portNo, orderNo)
	}
}

func (m *OrderMonitor) maybeQueryPort(ctx context.Context, phyID string, portNo int, orderNo string) {
	if m.commands == nil || m.sendLimit <= 0 {
		return
	}
	m.sendLimit--

	socketNo := int32(portNo)
	biz := coremodel.BusinessNo(orderNo)
	cmd := &coremodel.CoreCommand{
		Type:       coremodel.CommandQueryPortStatus,
		DeviceID:   coremodel.DeviceID(phyID),
		PortNo:     coremodel.PortNo(portNo),
		BusinessNo: &biz,
		IssuedAt:   m.nowFunc(),
		QueryPortStatus: &coremodel.QueryPortStatusPayload{
			SocketNo: &socketNo,
		},
	}

	if err := m.commands.SendCoreCommand(ctx, cmd); err != nil {
		m.logger.Warn("order monitor: query port status failed",
			zap.String("phy_id", phyID),
			zap.Int("port_no", portNo),
			zap.String("order_no", orderNo),
			zap.Error(err))
	}
}

// cleanupCancellingOrders P0修复: 自动清理超时的cancelling订单
// cancelling订单超过30秒未收到设备ACK,自动变为cancelled
func (m *OrderMonitor) cleanupCancellingOrders(ctx context.Context) {
	const q = `SELECT order_no 
	           FROM orders 
	           WHERE status = 8 
	             AND updated_at < NOW() - INTERVAL '30 seconds'
	           LIMIT 100`

	rows, err := m.repo.Pool.Query(ctx, q)
	if err != nil {
		m.logger.Error("cleanup cancelling orders failed", zap.Error(err))
		return
	}
	defer rows.Close()

	var processed int64
	for rows.Next() {
		var orderNo string
		if err := rows.Scan(&orderNo); err != nil {
			m.logger.Error("cleanup cancelling orders: scan failed", zap.Error(err))
			continue
		}

		// 一致性审计: 记录超时订单自动取消
		m.logger.Info("consistency: auto-cancelling timeout order",
			// 标准一致性字段
			zap.String("source", "order_monitor"),
			zap.String("scenario", "cancelling_timeout"),
			zap.String("expected_state", "order_cancelled_by_device_ack"),
			zap.String("actual_state", "order_stuck_in_cancelling"),
			zap.String("action", "auto_finalize_to_cancelled"),
			// 业务上下文
			zap.String("order_no", orderNo),
			zap.Int("old_status", api.OrderStatusCancelling),
			zap.Int("new_status", api.OrderStatusCancelled),
			zap.String("reason", "cancelling超时30秒"),
		)

		// 5 = cancelled，0x90 = BKV idle (在线+空载)
		if err := m.repo.FinalizeOrderAndPort(ctx, orderNo, api.OrderStatusCancelling, api.OrderStatusCancelled, 0x90, nil); err != nil {
			m.logger.Error("cleanup cancelling orders: finalize failed",
				zap.String("order_no", orderNo),
				zap.Error(err))
			continue
		}
		processed++
	}

	if processed > 0 {
		m.logger.Info("✅ auto cancelled timeout orders",
			zap.Int64("count", processed),
			zap.String("reason", "cancelling超时30秒"))
	}
}

// cleanupStoppingOrders P0修复: 自动清理超时的stopping订单
// stopping订单超过30秒未收到设备ACK,自动变为stopped
func (m *OrderMonitor) cleanupStoppingOrders(ctx context.Context) {
	const q = `SELECT order_no 
	           FROM orders 
	           WHERE status = 9 
	             AND updated_at < NOW() - INTERVAL '30 seconds'
	           LIMIT 100`

	rows, err := m.repo.Pool.Query(ctx, q)
	if err != nil {
		m.logger.Error("cleanup stopping orders failed", zap.Error(err))
		return
	}
	defer rows.Close()

	var processed int64
	for rows.Next() {
		var orderNo string
		if err := rows.Scan(&orderNo); err != nil {
			m.logger.Error("cleanup stopping orders: scan failed", zap.Error(err))
			continue
		}

		// 一致性审计: 记录超时订单自动停止
		m.logger.Info("consistency: auto-stopping timeout order",
			// 标准一致性字段
			zap.String("source", "order_monitor"),
			zap.String("scenario", "stopping_timeout"),
			zap.String("expected_state", "order_stopped_by_device_ack"),
			zap.String("actual_state", "order_stuck_in_stopping"),
			zap.String("action", "auto_finalize_to_stopped"),
			// 业务上下文
			zap.String("order_no", orderNo),
			zap.Int("old_status", api.OrderStatusStopping),
			zap.Int("new_status", 7),
			zap.String("reason", "stopping超时30秒"),
		)

		// 7 = stopped/settled，0x90 = BKV idle
		if err := m.repo.FinalizeOrderAndPort(ctx, orderNo, api.OrderStatusStopping, 7, 0x90, nil); err != nil {
			m.logger.Error("cleanup stopping orders: finalize failed",
				zap.String("order_no", orderNo),
				zap.Error(err))
			continue
		}
		processed++
	}

	if processed > 0 {
		m.logger.Info("✅ auto stopped timeout orders",
			zap.Int64("count", processed),
			zap.String("reason", "stopping超时30秒"))
	}
}

// cleanupInterruptedOrders P0修复: 自动清理超时的interrupted订单
// interrupted订单超过60秒设备未恢复,自动变为failed
func (m *OrderMonitor) cleanupInterruptedOrders(ctx context.Context) {
	const q = `SELECT order_no 
	           FROM orders 
	           WHERE status = 10 
	             AND updated_at < NOW() - INTERVAL '60 seconds'
	           LIMIT 100`

	rows, err := m.repo.Pool.Query(ctx, q)
	if err != nil {
		m.logger.Error("cleanup interrupted orders failed", zap.Error(err))
		return
	}
	defer rows.Close()

	var processed int64
	reason := "device offline timeout"
	for rows.Next() {
		var orderNo string
		if err := rows.Scan(&orderNo); err != nil {
			m.logger.Error("cleanup interrupted orders: scan failed", zap.Error(err))
			continue
		}

		// 一致性审计: 记录中断订单自动失败
		m.logger.Warn("consistency: auto-failing interrupted order",
			// 标准一致性字段
			zap.String("source", "order_monitor"),
			zap.String("scenario", "interrupted_timeout"),
			zap.String("expected_state", "device_recovered_or_order_resumed"),
			zap.String("actual_state", "device_offline_too_long"),
			zap.String("action", "auto_finalize_to_failed"),
			// 业务上下文
			zap.String("order_no", orderNo),
			zap.Int("old_status", api.OrderStatusInterrupted),
			zap.Int("new_status", api.OrderStatusFailed),
			zap.String("failure_reason", reason),
			zap.String("reason", "设备离线超过60秒未恢复"),
		)

		// 6 = failed，端口收敛为 idle（0x90）；failure_reason 写入一次
		if err := m.repo.FinalizeOrderAndPort(ctx, orderNo, api.OrderStatusInterrupted, api.OrderStatusFailed, 0x90, &reason); err != nil {
			m.logger.Error("cleanup interrupted orders: finalize failed",
				zap.String("order_no", orderNo),
				zap.Error(err))
			continue
		}
		processed++
	}

	if processed > 0 {
		m.logger.Warn("⚠️ auto failed interrupted orders",
			zap.Int64("count", processed),
			zap.String("reason", "设备离线超过60秒未恢复"))
	}
}

// Stats 获取监控统计
func (m *OrderMonitor) Stats() map[string]interface{} {
	return map[string]interface{}{
		"checked":               m.statsChecked,
		"pending_alerted":       m.statsPending,
		"charging_long_alerted": m.statsChargingLong,
		"check_interval_sec":    m.checkInterval.Seconds(),
		"pending_timeout_sec":   m.pendingTimeout.Seconds(),
		"charging_timeout_sec":  m.chargingTimeout.Seconds(),
	}
}
