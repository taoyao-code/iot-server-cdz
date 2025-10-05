package app

import (
	cfgpkg "github.com/taoyao-code/iot-server/internal/config"
	"github.com/taoyao-code/iot-server/internal/tcpserver"
	"go.uber.org/zap"
)

// NewTCPServer 根据配置创建 TCP 服务器
// Week2: 支持限流和熔断配置
func NewTCPServer(cfg cfgpkg.TCPConfig, logger *zap.Logger) *tcpserver.Server {
	srv := tcpserver.New(cfg)
	srv.SetLogger(logger)

	// Week2: 启用限流和熔断（如果配置启用）
	if cfg.Limiting.Enabled {
		srv.EnableLimiting(
			cfg.Limiting.MaxConnections,
			cfg.Limiting.RatePerSecond,
			cfg.Limiting.RateBurst,
			cfg.Limiting.BreakerThreshold,
			cfg.Limiting.BreakerTimeout,
		)
		logger.Info("tcp limiting enabled",
			zap.Int("max_connections", cfg.Limiting.MaxConnections),
			zap.Int("rate_per_second", cfg.Limiting.RatePerSecond),
			zap.Int("rate_burst", cfg.Limiting.RateBurst),
			zap.Int("breaker_threshold", cfg.Limiting.BreakerThreshold),
			zap.Duration("breaker_timeout", cfg.Limiting.BreakerTimeout),
		)
	}

	return srv
}
