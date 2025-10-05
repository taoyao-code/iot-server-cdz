package session

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 使用测试用Redis客户端（需要真实Redis实例或mock）
func setupTestRedis(t *testing.T) *redis.Client {
	client := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
		DB:   15, // 使用测试专用数据库
	})

	// 测试连接
	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Skip("Redis not available, skipping test")
		return nil
	}

	// 清空测试数据库
	client.FlushDB(ctx)

	t.Cleanup(func() {
		client.FlushDB(ctx)
		client.Close()
	})

	return client
}

func TestRedisManager_Basic(t *testing.T) {
	client := setupTestRedis(t)
	if client == nil {
		return
	}

	mgr := NewRedisManager(client, "test-server-1", 5*time.Minute)
	require.NotNil(t, mgr)

	// 测试心跳
	now := time.Now()
	mgr.OnHeartbeat("device-001", now)

	// 检查在线状态
	assert.True(t, mgr.IsOnline("device-001", now.Add(1*time.Minute)))
	assert.False(t, mgr.IsOnline("device-001", now.Add(10*time.Minute)))
}

func TestRedisManager_Bind(t *testing.T) {
	client := setupTestRedis(t)
	if client == nil {
		return
	}

	mgr := NewRedisManager(client, "test-server-1", 5*time.Minute)

	// 绑定连接
	mockConn := &struct{ id string }{id: "conn-1"}
	mgr.Bind("device-001", mockConn)

	// 获取连接
	conn, ok := mgr.GetConn("device-001")
	assert.True(t, ok)
	assert.Equal(t, mockConn, conn)

	// 解绑
	mgr.UnbindByPhy("device-001")
	_, ok = mgr.GetConn("device-001")
	assert.False(t, ok)
}

func TestRedisManager_MultiSignal(t *testing.T) {
	client := setupTestRedis(t)
	if client == nil {
		return
	}

	mgr := NewRedisManager(client, "test-server-1", 5*time.Minute)

	now := time.Now()
	phyID := "device-001"

	// 设置心跳
	mgr.OnHeartbeat(phyID, now)

	// 记录TCP断开
	mgr.OnTCPClosed(phyID, now)

	// 记录ACK超时
	mgr.OnAckTimeout(phyID, now)

	// 测试加权策略
	policy := WeightedPolicy{
		Enabled:           true,
		HeartbeatTimeout:  5 * time.Minute,
		TCPDownWindow:     1 * time.Minute,
		AckWindow:         1 * time.Minute,
		TCPDownPenalty:    0.5,
		AckTimeoutPenalty: 0.3,
		Threshold:         0.5,
	}

	// 心跳新鲜，但有TCP断开和ACK超时惩罚
	// score = 1.0 (心跳) - 0.5 (TCP) - 0.3 (ACK) = 0.2 < 0.5
	isOnline := mgr.IsOnlineWeighted(phyID, now.Add(10*time.Second), policy)
	assert.False(t, isOnline)

	// 等待惩罚窗口过期
	future := now.Add(2 * time.Minute)
	isOnline = mgr.IsOnlineWeighted(phyID, future, policy)
	assert.True(t, isOnline)
}

func TestRedisManager_OnlineCount(t *testing.T) {
	client := setupTestRedis(t)
	if client == nil {
		return
	}

	mgr := NewRedisManager(client, "test-server-1", 5*time.Minute)

	now := time.Now()

	// 添加多个设备
	mgr.OnHeartbeat("device-001", now)
	mgr.OnHeartbeat("device-002", now)
	mgr.OnHeartbeat("device-003", now.Add(-10*time.Minute)) // 已过期

	// 检查在线数量
	count := mgr.OnlineCount(now)
	assert.Equal(t, 2, count)
}

func TestRedisManager_MultiServer(t *testing.T) {
	client := setupTestRedis(t)
	if client == nil {
		return
	}

	// 模拟两个服务器实例
	mgr1 := NewRedisManager(client, "server-1", 5*time.Minute)
	mgr2 := NewRedisManager(client, "server-2", 5*time.Minute)

	now := time.Now()

	// 服务器1绑定设备A
	mockConn1 := &struct{ id string }{id: "conn-1"}
	mgr1.Bind("device-A", mockConn1)
	mgr1.OnHeartbeat("device-A", now)

	// 服务器2绑定设备B
	mockConn2 := &struct{ id string }{id: "conn-2"}
	mgr2.Bind("device-B", mockConn2)
	mgr2.OnHeartbeat("device-B", now)

	// 服务器1只能获取自己的连接
	conn, ok := mgr1.GetConn("device-A")
	assert.True(t, ok)
	assert.Equal(t, mockConn1, conn)

	// 服务器1无法获取服务器2的连接
	_, ok = mgr1.GetConn("device-B")
	assert.False(t, ok)

	// 但都能查看在线状态
	assert.True(t, mgr1.IsOnline("device-B", now))
	assert.True(t, mgr2.IsOnline("device-A", now))

	// 在线数量应该是2
	count1 := mgr1.OnlineCount(now)
	count2 := mgr2.OnlineCount(now)
	assert.Equal(t, 2, count1)
	assert.Equal(t, 2, count2)
}

func TestRedisManager_Cleanup(t *testing.T) {
	client := setupTestRedis(t)
	if client == nil {
		return
	}

	mgr := NewRedisManager(client, "test-server-1", 5*time.Minute)

	// 绑定多个设备
	mockConn1 := &struct{ id string }{id: "conn-1"}
	mockConn2 := &struct{ id string }{id: "conn-2"}
	mgr.Bind("device-001", mockConn1)
	mgr.Bind("device-002", mockConn2)

	// 清理
	err := mgr.Cleanup()
	assert.NoError(t, err)

	// 验证连接已清理
	_, ok := mgr.GetConn("device-001")
	assert.False(t, ok)
	_, ok = mgr.GetConn("device-002")
	assert.False(t, ok)
}

func TestRedisManager_ServerIDGeneration(t *testing.T) {
	client := setupTestRedis(t)
	if client == nil {
		return
	}

	// 不提供serverID，应该自动生成
	mgr := NewRedisManager(client, "", 5*time.Minute)
	assert.NotEmpty(t, mgr.serverID)
}

func TestRedisManager_Interface(t *testing.T) {
	client := setupTestRedis(t)
	if client == nil {
		return
	}

	// 验证RedisManager实现了SessionManager接口
	var _ SessionManager = NewRedisManager(client, "test-server", 5*time.Minute)
}
