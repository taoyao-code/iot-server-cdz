package app

import (
	"context"
	"time"

	pgstorage "github.com/taoyao-code/iot-server/internal/storage/pg"
	"go.uber.org/zap"
)

// PortStatusSyncer P1-4: 端口状态定期同步器
// 定期查询设备端口状态，确保与数据库一致
type PortStatusSyncer struct {
	repo   *pgstorage.Repository
	logger *zap.Logger

	checkInterval time.Duration // 检查间隔

	// 统计
	statsChecked     int64
	statsMismatches  int64
	statsDeviceQuery int64
}

// NewPortStatusSyncer 创建端口状态同步器
func NewPortStatusSyncer(repo *pgstorage.Repository, logger *zap.Logger) *PortStatusSyncer {
	return &PortStatusSyncer{
		repo:          repo,
		logger:        logger,
		checkInterval: 5 * time.Minute, // 每5分钟检查一次
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

	// TODO P1-4: 完整实现需要以下步骤
	// 1. 查询所有在线设备（last_seen_at < 60秒）
	// 2. 对每个设备下发0x1012查询命令
	// 3. 等待响应（超时5秒）
	// 4. 比对设备返回的端口状态与数据库状态
	// 5. 记录不一致情况，触发告警
	// 6. 可选：自动修正数据库状态

	// 当前仅记录日志，实际实现需要0x1012命令支持
	s.logger.Debug("P1-4: periodic port status sync (placeholder - full implementation requires 0x1012 support)")

	// 简化版实现：检查charging订单与端口状态的一致性
	s.checkChargingOrdersConsistency(ctx)
}

// checkChargingOrdersConsistency 检查charging订单与端口状态的一致性
func (s *PortStatusSyncer) checkChargingOrdersConsistency(ctx context.Context) {
	// 查询所有charging状态的订单
	query := `
		SELECT o.order_no, o.device_id, o.port_no, p.status as port_status
		FROM orders o
		LEFT JOIN ports p ON o.device_id = p.device_id AND o.port_no = p.port_no
		WHERE o.status = 2  -- charging
	`

	rows, err := s.repo.Pool.Query(ctx, query)
	if err != nil {
		s.logger.Error("P1-4: failed to query charging orders", zap.Error(err))
		return
	}
	defer rows.Close()

	for rows.Next() {
		var orderNo string
		var deviceID int64
		var portNo int
		var portStatus *int

		if err := rows.Scan(&orderNo, &deviceID, &portNo, &portStatus); err != nil {
			s.logger.Error("P1-4: failed to scan row", zap.Error(err))
			continue
		}

		// 如果订单是charging，端口状态应该也是charging(2)
		if portStatus == nil {
			s.logger.Warn("P1-4: charging order has no port record",
				zap.String("order_no", orderNo),
				zap.Int64("device_id", deviceID),
				zap.Int("port_no", portNo))
			s.statsMismatches++
		} else if *portStatus != 2 {
			s.logger.Warn("P1-4: charging order port status mismatch",
				zap.String("order_no", orderNo),
				zap.Int64("device_id", deviceID),
				zap.Int("port_no", portNo),
				zap.Int("expected_status", 2),
				zap.Int("actual_status", *portStatus),
				zap.String("action", "requires manual intervention or 0x1012 query"))
			s.statsMismatches++
		}
	}
}

// Stats 获取统计信息
func (s *PortStatusSyncer) Stats() map[string]interface{} {
	return map[string]interface{}{
		"checked":        s.statsChecked,
		"mismatches":     s.statsMismatches,
		"device_queries": s.statsDeviceQuery,
	}
}
