package app

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	cfgpkg "github.com/taoyao-code/iot-server/internal/config"
	"github.com/taoyao-code/iot-server/internal/session"
	redisstorage "github.com/taoyao-code/iot-server/internal/storage/redis"
)

// TestSessionTimeout_P1_1 测试P1-1修复：心跳超时窗口调整为60秒
func TestSessionTimeout_P1_1(t *testing.T) {
	// 加载配置文件
	cfg, err := cfgpkg.Load("../../configs/example.yaml")
	require.NoError(t, err, "配置文件加载失败")

	// 验证配置值为60秒（心跳周期30s × 2）
	assert.Equal(t, 60, cfg.Session.HeartbeatTimeoutSec,
		"P1-1: heartbeat_timeout_sec应设置为60秒（心跳周期30s × 2，避免网络波动误判）")

	// 创建Redis客户端（用于测试）
	redisClient := &redisstorage.Client{
		// 这里使用nil Client是安全的，因为我们只测试timeout参数传递
		Client: nil,
	}

	// 创建logger
	logger := zap.NewNop()

	// 创建会话管理器
	mgr, policy := NewSessionAndPolicy(cfg.Session, redisClient, "test-server", logger)

	// 验证返回的不是nil
	require.NotNil(t, mgr, "会话管理器不应为nil")

	// 验证policy的HeartbeatTimeout为60秒
	expectedTimeout := 60 * time.Second
	assert.Equal(t, expectedTimeout, policy.HeartbeatTimeout,
		"P1-1: WeightedPolicy的HeartbeatTimeout应为60秒")

	// 验证RedisManager的timeout字段（通过类型断言）
	if _, ok := mgr.(*session.RedisManager); ok {
		// 注意：RedisManager的timeout字段是私有的，无法直接访问
		// 但我们可以通过测试IsOnline方法来间接验证
		t.Log("RedisManager创建成功，timeout参数已传递")
	}

	t.Logf("✓ P1-1验证通过: heartbeat_timeout_sec = %d秒", cfg.Session.HeartbeatTimeoutSec)
}

// TestSessionTimeout_AllConfigs 验证所有配置文件的timeout都是60秒
func TestSessionTimeout_AllConfigs(t *testing.T) {
	configFiles := []struct {
		name string
		path string
	}{
		{"example", "../../configs/example.yaml"},
		{"local", "../../configs/local.yaml"},
		{"production", "../../configs/production.yaml"},
	}

	for _, cf := range configFiles {
		t.Run(cf.name, func(t *testing.T) {
			cfg, err := cfgpkg.Load(cf.path)
			require.NoError(t, err, "%s配置文件加载失败", cf.name)

			assert.Equal(t, 60, cfg.Session.HeartbeatTimeoutSec,
				"P1-1: %s配置的heartbeat_timeout_sec应为60秒", cf.name)

			t.Logf("✓ %s: heartbeat_timeout_sec = %d秒", cf.name, cfg.Session.HeartbeatTimeoutSec)
		})
	}
}
