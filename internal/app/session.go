package app

import (
	"fmt"
	"time"

	cfgpkg "github.com/taoyao-code/iot-server/internal/config"
	"github.com/taoyao-code/iot-server/internal/session"
	redisstorage "github.com/taoyao-code/iot-server/internal/storage/redis"
	"go.uber.org/zap"
)

// NewSessionAndPolicy 构造会话管理器与加权策略
// Redis是必选依赖，用于支持分布式会话管理和水平扩展
func NewSessionAndPolicy(
	cfg cfgpkg.SessionConfig,
	redisClient *redisstorage.Client,
	serverID string,
	logger *zap.Logger,
) (session.SessionManager, session.WeightedPolicy) {
	timeout := time.Duration(cfg.HeartbeatTimeoutSec) * time.Second

	// Redis是生产环境必选依赖
	if redisClient == nil {
		panic(fmt.Errorf("redis is required for session management (enable redis in config)"))
	}

	// 使用Redis会话管理器（支持分布式部署）
	mgr := session.NewRedisManager(redisClient.Client, serverID, timeout)
	logger.Info("redis session manager initialized",
		zap.String("server_id", serverID),
		zap.Duration("timeout", timeout))

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
