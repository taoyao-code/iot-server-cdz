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

// PortStatusSyncer
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
	statsDeviceQuery int64
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

// Stats 获取统计信息
func (s *PortStatusSyncer) Stats() map[string]interface{} {
	return map[string]interface{}{
		"checked":        s.statsChecked,
		"device_queries": s.statsDeviceQuery,
	}
}
