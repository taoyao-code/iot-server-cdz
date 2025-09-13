package app

import (
	"time"

	cfgpkg "github.com/taoyao-code/iot-server/internal/config"
	"github.com/taoyao-code/iot-server/internal/session"
)

// NewSessionAndPolicy 构造会话管理器与加权策略
func NewSessionAndPolicy(cfg cfgpkg.SessionConfig) (*session.Manager, session.WeightedPolicy) {
	mgr := session.New(time.Duration(cfg.HeartbeatTimeoutSec) * time.Second)
	policy := session.WeightedPolicy{
		Enabled:           cfg.WeightedEnabled,
		HeartbeatTimeout:  time.Duration(cfg.HeartbeatTimeoutSec) * time.Second,
		TCPDownWindow:     time.Duration(cfg.TCPDownWindowSec) * time.Second,
		AckWindow:         time.Duration(cfg.AckWindowSec) * time.Second,
		TCPDownPenalty:    cfg.TCPDownPenalty,
		AckTimeoutPenalty: cfg.AckTimeoutPenalty,
		Threshold:         cfg.Threshold,
	}
	return mgr, policy
}
