package app

import (
	"time"

	cfgpkg "github.com/taoyao-code/iot-server/internal/config"
	"github.com/taoyao-code/iot-server/internal/session"
	redisstorage "github.com/taoyao-code/iot-server/internal/storage/redis"
	"go.uber.org/zap"
)

// NewSessionAndPolicy 构造会话管理器与加权策略
// 如果Redis客户端可用，则使用Redis会话管理器，否则使用内存会话管理器
func NewSessionAndPolicy(
	cfg cfgpkg.SessionConfig,
	redisClient *redisstorage.Client,
	serverID string,
	logger *zap.Logger,
) (session.SessionManager, session.WeightedPolicy) {
	timeout := time.Duration(cfg.HeartbeatTimeoutSec) * time.Second

	var mgr session.SessionManager

	// 如果Redis可用，使用Redis会话管理器
	if redisClient != nil {
		mgr = session.NewRedisManager(redisClient.Client, serverID, timeout)
		logger.Info("using redis session manager",
			zap.String("server_id", serverID),
			zap.Duration("timeout", timeout))
	} else {
		// 否则使用内存会话管理器
		mgr = session.New(timeout)
		logger.Info("using memory session manager",
			zap.Duration("timeout", timeout))
	}

	policy := session.WeightedPolicy{
		Enabled:           cfg.WeightedEnabled,
		HeartbeatTimeout:  timeout,
		TCPDownWindow:     time.Duration(cfg.TCPDownWindowSec) * time.Second,
		AckWindow:         time.Duration(cfg.AckWindowSec) * time.Second,
		TCPDownPenalty:    cfg.TCPDownPenalty,
		AckTimeoutPenalty: cfg.AckTimeoutPenalty,
		Threshold:         cfg.Threshold,
	}
	return mgr, policy
}
