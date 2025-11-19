package app

import (
	"context"
	"time"

	"github.com/taoyao-code/iot-server/internal/api"
	pgstorage "github.com/taoyao-code/iot-server/internal/storage/pg"
	"go.uber.org/zap"
)

// OrderMonitor 订单状态监控器
// 定期检查pending订单超时、charging订单异常等情况
type OrderMonitor struct {
	repo   *pgstorage.Repository
	logger *zap.Logger

	checkInterval   time.Duration // 检查间隔
	pendingTimeout  time.Duration // pending订单超时阈值
	chargingTimeout time.Duration // charging订单超时阈值 (异常长时间充电)

	// 统计
	statsChecked      int64
	statsPending      int64
	statsChargingLong int64
}

// NewOrderMonitor 创建订单监控器
func NewOrderMonitor(repo *pgstorage.Repository, logger *zap.Logger) *OrderMonitor {
	return &OrderMonitor{
		repo:            repo,
		logger:          logger,
		checkInterval:   1 * time.Minute, // 每分钟检查一次
		pendingTimeout:  5 * time.Minute, // pending超过5分钟告警
		chargingTimeout: 2 * time.Hour,   // charging超过2小时告警
	}
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

	cutoff := time.Now().Add(-m.pendingTimeout)
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

		// TODO: 可选操作
		// 1. 自动取消订单: m.repo.CancelOrderByPort(ctx, deviceID, portNo)
		// 2. 重发充电指令 (如果outbound队列支持)
		// 3. 推送告警到运维系统
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

	cutoff := time.Now().Add(-m.chargingTimeout)
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

		// TODO: 可选操作
		// 1. 查询设备在线状态,如果离线则自动结算订单
		// 2. 主动发送查询指令获取最新充电数据
		// 3. 推送告警到运维系统
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

		// 5 = cancelled，0x09 = BKV idle (在线+空载)
		if err := m.repo.FinalizeOrderAndPort(ctx, orderNo, api.OrderStatusCancelling, api.OrderStatusCancelled, 0x09, nil); err != nil {
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

		// 7 = stopped/settled，0x09 = BKV idle
		if err := m.repo.FinalizeOrderAndPort(ctx, orderNo, api.OrderStatusStopping, 7, 0x09, nil); err != nil {
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

		// 6 = failed，端口收敛为 idle（0x09）；failure_reason 写入一次
		if err := m.repo.FinalizeOrderAndPort(ctx, orderNo, api.OrderStatusInterrupted, api.OrderStatusFailed, 0x09, &reason); err != nil {
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
