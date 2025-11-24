package app

import (
	"context"
	"fmt"
	"time"

	"github.com/taoyao-code/iot-server/internal/coremodel"
	"github.com/taoyao-code/iot-server/internal/driverapi"
	"github.com/taoyao-code/iot-server/internal/metrics"
	pgstorage "github.com/taoyao-code/iot-server/internal/storage/pg"
	"go.uber.org/zap"
)

// PortStatusSyncer P1-4: 端口状态定期同步器
// 定期查询设备端口状态，确保与数据库一致
type PortStatusSyncer struct {
	repo     *pgstorage.Repository
	sess     SessionManager // 会话管理器（用于实时在线判断）
	commands driverapi.CommandSource
	metrics  *metrics.AppMetrics
	logger   *zap.Logger

	checkInterval time.Duration // 检查间隔
	queryTimeout  time.Duration // 查询超时

	// 统计
	statsChecked     int64
	statsMismatches  int64
	statsDeviceQuery int64
	statsAutoFixed   int64
}

// SessionManager 会话管理器接口（避免循环依赖）
type SessionManager interface {
	IsOnline(phyID string, now time.Time) bool
}

// NewPortStatusSyncer 创建端口状态同步器
func NewPortStatusSyncer(
	repo *pgstorage.Repository,
	sess SessionManager,
	commands driverapi.CommandSource,
	metrics *metrics.AppMetrics,
	logger *zap.Logger,
) *PortStatusSyncer {
	return &PortStatusSyncer{
		repo:          repo,
		sess:          sess,
		commands:      commands,
		metrics:       metrics,
		logger:        logger,
		checkInterval: 5 * time.Minute, // 每5分钟检查一次
		queryTimeout:  5 * time.Second, // 查询超时5秒
	}
}

// Start 启动端口状态同步器
func (s *PortStatusSyncer) Start(ctx context.Context) {
	s.logger.Info("P1-4: port status syncer started",
		zap.Duration("check_interval", s.checkInterval))

	ticker := time.NewTicker(s.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("P1-4: port status syncer stopped",
				zap.Int64("checked", s.statsChecked),
				zap.Int64("mismatches", s.statsMismatches),
				zap.Int64("device_queries", s.statsDeviceQuery))
			return
		case <-ticker.C:
			s.syncAllDevices(ctx)
		}
	}
}

// syncAllDevices 同步所有在线设备的端口状态
func (s *PortStatusSyncer) syncAllDevices(ctx context.Context) {
	s.statsChecked++

	// P1-4完整实现：
	// 1. 检查charging订单与端口状态一致性（基于数据库）
	s.checkChargingOrdersConsistency(ctx)

	// 1.5 检查“端口显示充电但无任何活跃订单”的不一致场景
	// 典型场景：设备长时间离线，ports.status 保持0x81 (charging)，但 orders 中已无active订单
	s.fixLonelyChargingPorts(ctx)

	// 2. 定期查询在线设备的端口状态（可选，需要设备支持）
	// 注意：由于BKV协议的0x1D查询命令需要设备响应，
	// 而当前系统是异步处理，不适合在同步任务中等待响应。
	// 因此这里只做数据库一致性检查。
	// 如果未来需要实时查询设备状态，建议：
	// - 在API层面提供按需查询接口
	// - 或使用设备主动上报的0x94状态更新（已实现）
	s.queryOnlineDevicesPorts(ctx)
}

// queryOnlineDevicesPorts 查询在线设备的端口状态（扩展功能）
// 修复：不再依赖 devices.online 字段（生产代码从未维护该字段）
// 改用 last_seen_at + SessionManager 实时判断在线状态
func (s *PortStatusSyncer) queryOnlineDevicesPorts(ctx context.Context) {
	// 查询最近60秒内有心跳的设备（不依赖 online 字段）
	query := `
		SELECT id, phy_id, last_seen_at
		FROM devices
		WHERE last_seen_at > NOW() - INTERVAL '60 seconds'
		ORDER BY last_seen_at DESC
		LIMIT 50  -- 每次最多查询50个设备，避免队列堵塞
	`

	rows, err := s.repo.Pool.Query(ctx, query)
	if err != nil {
		s.logger.Error("P1-4: failed to query online devices", zap.Error(err))
		return
	}
	defer rows.Close()

	now := time.Now()
	devicesQueried := 0

	for rows.Next() {
		var deviceID int64
		var phyID string
		var lastSeenAt time.Time

		if err := rows.Scan(&deviceID, &phyID, &lastSeenAt); err != nil {
			s.logger.Error("P1-4: failed to scan device", zap.Error(err))
			continue
		}

		// 使用 SessionManager 实时判断在线状态（单一真相源）
		if !s.sess.IsOnline(phyID, now) {
			continue
		}

		// 为每个设备下发端口状态查询命令（0x1D）
		// 注意：这里只查询端口1作为示例，实际应查询所有端口
		if err := s.queryDevicePort(ctx, deviceID, phyID, 1); err != nil {
			s.logger.Warn("P1-4: failed to query device port",
				zap.Int64("device_id", deviceID),
				zap.String("phy_id", phyID),
				zap.Error(err))
		} else {
			devicesQueried++
		}
	}

	if devicesQueried > 0 {
		s.statsDeviceQuery += int64(devicesQueried)
		s.logger.Debug("P1-4: queried online devices ports",
			zap.Int("devices_queried", devicesQueried))
	}
}

// queryDevicePort 查询单个设备的端口状态
func (s *PortStatusSyncer) queryDevicePort(ctx context.Context, deviceID int64, phyID string, portNo int) error {
	if s.commands == nil {
		return fmt.Errorf("driver command source not configured")
	}

	socketNo := int32(portNo)
	biz := coremodel.BusinessNo(fmt.Sprintf("SYNC-%d", time.Now().UnixNano()))

	cmd := &coremodel.CoreCommand{
		Type:       coremodel.CommandQueryPortStatus,
		DeviceID:   coremodel.DeviceID(phyID),
		PortNo:     coremodel.PortNo(portNo),
		BusinessNo: &biz,
		IssuedAt:   time.Now(),
		QueryPortStatus: &coremodel.QueryPortStatusPayload{
			SocketNo: &socketNo,
		},
	}

	if err := s.commands.SendCoreCommand(ctx, cmd); err != nil {
		return fmt.Errorf("send driver query command: %w", err)
	}

	return nil
}

// fixLonelyChargingPorts 修复“端口处于充电状态但无任何活跃订单”的不一致状态
// 场景: ports.status 含有充电标志(bit7=0x80)，但 orders 中不存在该 device/port 的活跃订单
// 为避免误判，仅在端口状态长时间未更新且设备会话离线的情况下，自动将端口收敛为空闲(0x09)
func (s *PortStatusSyncer) fixLonelyChargingPorts(ctx context.Context) {
	const q = `
		SELECT 
			p.device_id,
			d.phy_id,
			p.port_no,
			p.status,
			p.updated_at,
			d.last_seen_at
		FROM ports p
		JOIN devices d ON p.device_id = d.id
		LEFT JOIN LATERAL (
			SELECT 1
			FROM orders o
			WHERE o.device_id = p.device_id
			  AND o.port_no = p.port_no
			  AND o.status IN (0,1,2,8,9,10) -- pending/confirmed/charging/cancelling/stopping/interrupted
			LIMIT 1
		) ao(is_active) ON TRUE
		WHERE (p.status & 128) <> 0       -- BKV charging bit set
		  AND ao.is_active IS NULL        -- 无任何活跃订单
		ORDER BY p.updated_at ASC
		LIMIT 100
	`

	rows, err := s.repo.Pool.Query(ctx, q)
	if err != nil {
		s.logger.Error("P1-4: failed to query lonely charging ports", zap.Error(err))
		return
	}
	defer rows.Close()

	now := time.Now()
	fixedCount := 0

	for rows.Next() {
		var (
			deviceID  int64
			phyID     string
			portNo    int
			status    int
			updatedAt time.Time
			lastSeen  time.Time
		)

		if err := rows.Scan(&deviceID, &phyID, &portNo, &status, &updatedAt, &lastSeen); err != nil {
			s.logger.Error("P1-4: failed to scan lonely port row", zap.Error(err))
			continue
		}

		// 使用 SessionManager 判断设备是否在线
		online := s.sess.IsOnline(phyID, now)
		age := now.Sub(updatedAt)

		// 仅在设备离线且端口状态至少15分钟未更新时收敛，避免误伤正常充电
		if online || age < 15*time.Minute {
			s.logger.Debug("P1-4: skip lonely charging port (conditions not met)",
				zap.Int64("device_id", deviceID),
				zap.String("phy_id", phyID),
				zap.Int("port_no", portNo),
				zap.Int("status", status),
				zap.Bool("online", online),
				zap.Duration("age", age))
			continue
		}

		// 一致性审计: 统一的自愈日志格式
		s.logger.Warn("consistency: auto-fixing lonely charging port",
			// 标准一致性字段
			zap.String("source", "port_status_syncer"),
			zap.String("scenario", "lonely_charging_port"),
			zap.String("expected_state", "port_idle_or_has_order"),
			zap.String("actual_state", "port_charging_no_order"),
			zap.String("action", "converge_to_idle"),
			// 业务上下文
			zap.Int64("device_id", deviceID),
			zap.String("phy_id", phyID),
			zap.Int("port_no", portNo),
			zap.Int("port_status", status),
			zap.String("port_status_hex", fmt.Sprintf("0x%02x", status)),
			zap.Bool("device_online", online),
			zap.Time("port_updated_at", updatedAt),
			zap.Time("device_last_seen_at", lastSeen),
			zap.Duration("stale_duration", age),
		)

		// Prometheus 指标: 记录孤立端口修复事件
		if s.metrics != nil {
			s.metrics.ConsistencyLonelyPortFixTotal.Inc()
			s.metrics.ConsistencyEventsTotal.WithLabelValues(
				"port_status_syncer",
				"lonely_charging_port",
				"warn",
			).Inc()
			s.metrics.ConsistencyAutoFixTotal.WithLabelValues(
				"port_status_syncer",
				"lonely_charging_port",
				"converge_to_idle",
			).Inc()
		}

		// 收敛端口状态为空闲 (0x90 = bit7在线 + bit4空载)
		const idleStatus = 0x90
		if err := s.repo.UpsertPortState(ctx, deviceID, portNo, idleStatus, nil); err != nil {
			s.logger.Error("P1-4: failed to auto-fix lonely charging port",
				zap.Int64("device_id", deviceID),
				zap.String("phy_id", phyID),
				zap.Int("port_no", portNo),
				zap.Error(err))
			continue
		}

		fixedCount++
	}

	if fixedCount > 0 {
		s.statsAutoFixed += int64(fixedCount)
		s.logger.Info("P1-4: lonely charging ports auto-fixed",
			zap.Int("count", fixedCount))
	}
}

// checkChargingOrdersConsistency 检查charging订单与端口状态的一致性
func (s *PortStatusSyncer) checkChargingOrdersConsistency(ctx context.Context) {
	// P1-4完整实现：检查所有charging订单的端口状态一致性
	// 修复：使用SessionManager实时判断online，不使用数据库old字段

	// 场景1：订单charging，端口free → 订单可能已完成但未更新
	// 场景2：订单charging，端口charging → 正常
	// 场景3：订单charging，端口null → 端口记录缺失

	query := `
		SELECT 
			o.order_no, 
			o.device_id, 
			o.port_no, 
			o.status as order_status,
			o.updated_at as order_updated_at,
			p.status as port_status,
			d.phy_id,
			d.last_seen_at
		FROM orders o
		LEFT JOIN ports p ON o.device_id = p.device_id AND o.port_no = p.port_no
		LEFT JOIN devices d ON o.device_id = d.id
		WHERE o.status IN (2, 8, 9, 10)  -- charging, cancelling, stopping, interrupted
		ORDER BY o.updated_at DESC
		LIMIT 100
	`

	rows, err := s.repo.Pool.Query(ctx, query)
	if err != nil {
		s.logger.Error("P1-4: failed to query charging orders", zap.Error(err))
		return
	}
	defer rows.Close()

	inconsistentCount := 0
	now := time.Now() // 用于实时在线判断

	for rows.Next() {
		var orderNo string
		var deviceID int64
		var portNo int
		var orderStatus int
		var orderUpdatedAt time.Time
		var portStatus *int
		var phyID string
		var lastSeenAt time.Time

		if err := rows.Scan(&orderNo, &deviceID, &portNo, &orderStatus, &orderUpdatedAt,
			&portStatus, &phyID, &lastSeenAt); err != nil {
			s.logger.Error("P1-4: failed to scan row", zap.Error(err))
			continue
		}

		// 关键修复：使用SessionManager实时判断在线状态
		online := s.sess.IsOnline(phyID, now)

		// 检查一致性
		mismatchType := ""
		if portStatus == nil {
			mismatchType = "port_missing"
			inconsistentCount++
		} else {
			expectedStatus := s.getExpectedPortStatus(orderStatus)
			if *portStatus != expectedStatus {
				mismatchType = "status_mismatch"
				inconsistentCount++
			}
		}

		if mismatchType != "" {
			s.statsMismatches++

			// P1-4: 记录Prometheus指标
			if s.metrics != nil && s.metrics.PortStatusMismatchTotal != nil {
				s.metrics.PortStatusMismatchTotal.WithLabelValues(mismatchType).Inc()
			}

			// 判断严重程度
			timeSinceUpdate := time.Since(orderUpdatedAt)
			severity := "info"
			if timeSinceUpdate > 5*time.Minute {
				severity = "warning"
			}
			if timeSinceUpdate > 15*time.Minute {
				severity = "error"
			}

			s.logger.Warn("P1-4: port status inconsistency detected",
				zap.String("severity", severity),
				zap.String("mismatch_type", mismatchType),
				zap.String("order_no", orderNo),
				zap.Int64("device_id", deviceID),
				zap.String("phy_id", phyID),
				zap.Int("port_no", portNo),
				zap.Int("order_status", orderStatus),
				zap.Intp("port_status", portStatus),
				zap.Bool("device_online", online),
				zap.Time("last_seen_at", lastSeenAt),
				zap.Duration("time_since_update", timeSinceUpdate),
			)

			// 自动修复逻辑（谨慎使用）
			if s.shouldAutoFix(orderStatus, portStatus, online, timeSinceUpdate) {
				if err := s.autoFixInconsistency(ctx, orderNo, deviceID, portNo, orderStatus, portStatus); err != nil {
					s.logger.Error("P1-4: auto-fix failed",
						zap.String("order_no", orderNo),
						zap.Error(err))
				} else {
					s.statsAutoFixed++

					// P1-4: 记录自动修复成功指标
					if s.metrics != nil && s.metrics.PortStatusAutoFixedTotal != nil {
						s.metrics.PortStatusAutoFixedTotal.Inc()
					}

					s.logger.Info("P1-4: auto-fixed inconsistency",
						zap.String("order_no", orderNo),
						zap.Int("old_order_status", orderStatus),
						zap.Intp("port_status", portStatus))
				}
			}
		}
	}

	if inconsistentCount > 0 {
		s.logger.Info("P1-4: consistency check completed",
			zap.Int("inconsistent_count", inconsistentCount),
			zap.Int64("total_mismatches", s.statsMismatches),
			zap.Int64("auto_fixed", s.statsAutoFixed))
	}
}

// getExpectedPortStatus 根据订单状态获取期望的端口状态（BKV位图格式）
func (s *PortStatusSyncer) getExpectedPortStatus(orderStatus int) int {
	// 注意：这里返回的是BKV协议位图值，不是业务枚举！
	// BKV位图: bit7=在线(0x80), bit5=充电(0x20), bit4=空载(0x10)
	switch orderStatus {
	case 2: // charging
		return 0xA0 // 0xA0 = bit7(在线) + bit5(充电)
	case 8, 9: // cancelling, stopping
		return 0xA0 // port可能还在charging，等待设备响应
	case 10: // interrupted
		return 0xA0 // port可能还在charging或已offline
	default:
		return 0x90 // 0x90 = bit7(在线) + bit4(空载) = 空闲
	}
}

// shouldAutoFix 修复: 判断是否应该自动修复（更精确的防护条件）
func (s *PortStatusSyncer) shouldAutoFix(orderStatus int, portStatus *int, online bool, timeSinceUpdate time.Duration) bool {
	// 修复: 自动修复条件增强：
	// 1. 设备离线超过5分钟（至少5个心跳周期） 且 订单更新时间>5分钟 → 标记订单为failed
	// 2. 端口是free 且 订单是charging 且 超过15分钟未更新 → 标记订单为completed
	// 3. 修复: 订单interrupted状态 且 超过1小时 → 标记为failed

	// 条件1: 设备长时间离线，订单还在charging
	// 修复: 离线超过5分钟（避免误判心跳间隔），且订单更新超过5分钟
	// 设备心跳周期1分钟，5分钟=至少错过5次心跳，确保真正离线
	if !online && timeSinceUpdate > 5*time.Minute && orderStatus == 2 {
		return true
	}

	// 条件2: 端口已free但订单还是charging，可能是充电结束事件丢失
	if portStatus != nil && *portStatus == 0x90 && orderStatus == 2 && timeSinceUpdate > 15*time.Minute {
		return true
	}

	// 条件3: 修复 - interrupted订单长期未恢复，自动标记失败
	if orderStatus == 10 && timeSinceUpdate > 1*time.Hour {
		return true
	}

	return false
}

// autoFixInconsistency D修复: 自动修复不一致状态（增强interrupted支持 + 端口状态同步）
func (s *PortStatusSyncer) autoFixInconsistency(ctx context.Context, orderNo string, deviceID int64, portNo int, orderStatus int, portStatus *int) error {
	// 根据情况选择修复策略
	var newStatus int
	var reason string

	// D修复: 支持interrupted订单自动失败
	if orderStatus == 10 {
		// interrupted订单长期未恢复，标记为failed
		newStatus = 6 // failed
		reason = "interrupted_timeout_auto_fix"
	} else if portStatus == nil || *portStatus == 0 {
		// 端口是free或缺失，订单标记为completed
		newStatus = 3 // completed
		reason = "port_status_free_auto_fix"
	} else {
		// 设备离线，订单标记为failed
		newStatus = 6 // failed
		reason = "device_offline_auto_fix"
	}

	// 使用统一的 FinalizeOrderAndPort 收敛订单和端口状态，避免分散更新逻辑
	const idleStatus = 0x90 // BKV idle: bit7在线 + bit4空载
	var reasonPtr *string
	if newStatus == 6 {
		reasonPtr = &reason
	}

	if err := s.repo.FinalizeOrderAndPort(ctx, orderNo, orderStatus, newStatus, idleStatus, reasonPtr); err != nil {
		return fmt.Errorf("finalize order and port: %w", err)
	}

	// 一致性审计: 统一的自愈日志格式
	s.logger.Info("consistency: order status auto-fixed",
		// 标准一致性字段
		zap.String("source", "port_status_syncer"),
		zap.String("scenario", "order_port_inconsistency"),
		zap.String("expected_state", fmt.Sprintf("order_status_%d_matches_port", orderStatus)),
		zap.String("actual_state", "order_port_mismatch"),
		zap.String("action", "auto_fix_order_and_port"),
		// 业务上下文
		zap.String("order_no", orderNo),
		zap.Int64("device_id", deviceID),
		zap.Int("port_no", portNo),
		zap.Int("old_order_status", orderStatus),
		zap.Int("new_order_status", newStatus),
		zap.String("fix_reason", reason),
	)

	// Prometheus 指标: 记录订单自愈事件
	if s.metrics != nil {
		s.metrics.ConsistencyEventsTotal.WithLabelValues(
			"port_status_syncer",
			"order_port_inconsistency",
			"info",
		).Inc()
		s.metrics.ConsistencyAutoFixTotal.WithLabelValues(
			"port_status_syncer",
			"order_port_inconsistency",
			"auto_fix_order_and_port",
		).Inc()
	}

	return nil
}

// Stats 获取统计信息
func (s *PortStatusSyncer) Stats() map[string]interface{} {
	return map[string]interface{}{
		"checked":        s.statsChecked,
		"mismatches":     s.statsMismatches,
		"device_queries": s.statsDeviceQuery,
		"auto_fixed":     s.statsAutoFixed,
	}
}
