package session

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// RedisManager Redis版本的会话管理器，支持分布式部署
type RedisManager struct {
	client   *redis.Client
	serverID string        // 当前服务器实例ID
	timeout  time.Duration // 心跳超时时间

	// 本地连接缓存 (connID -> connection object)
	mu        sync.RWMutex
	localConn map[string]interface{}
}

// sessionData Redis存储的会话数据结构
type sessionData struct {
	PhyID          string    `json:"phy_id"`
	ConnID         string    `json:"conn_id"`
	ServerID       string    `json:"server_id"`
	LastSeen       time.Time `json:"last_seen"`
	LastTCPDown    time.Time `json:"last_tcp_down,omitempty"`
	LastAckTimeout time.Time `json:"last_ack_timeout,omitempty"`
}

// Redis Key设计
const (
	// session:device:{phyID} -> sessionData JSON
	keyDevicePrefix = "session:device:"

	// session:conn:{connID} -> phyID
	keyConnPrefix = "session:conn:"

	// session:server:{serverID}:conns -> Set[connID]
	keyServerConnsPrefix = "session:server:"
)

// NewRedisManager 创建Redis会话管理器
func NewRedisManager(client *redis.Client, serverID string, timeout time.Duration) *RedisManager {
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	if serverID == "" {
		serverID = uuid.New().String()
	}
	return &RedisManager{
		client:    client,
		serverID:  serverID,
		timeout:   timeout,
		localConn: make(map[string]interface{}),
	}
}

// OnHeartbeat 更新设备最近心跳时间
func (m *RedisManager) OnHeartbeat(phyID string, t time.Time) {
	ctx := context.Background()

	// 读取现有数据
	data, err := m.getSessionData(ctx, phyID)
	if err != nil {
		// 如果不存在，创建新的会话数据
		data = &sessionData{
			PhyID:    phyID,
			LastSeen: t,
		}
	} else {
		data.LastSeen = t
	}

	// 保存到Redis
	m.setSessionData(ctx, phyID, data)
}

// Bind 绑定设备物理ID到连接对象
func (m *RedisManager) Bind(phyID string, conn interface{}) {
	ctx := context.Background()

	// 生成唯一的连接ID
	connID := uuid.New().String()

	// 保存本地连接缓存
	m.mu.Lock()
	m.localConn[connID] = conn
	m.mu.Unlock()

	// 创建会话数据
	data := &sessionData{
		PhyID:    phyID,
		ConnID:   connID,
		ServerID: m.serverID,
		LastSeen: time.Now(),
	}

	// 保存到Redis
	m.setSessionData(ctx, phyID, data)

	// 保存连接ID映射: connID -> phyID
	m.client.Set(ctx, keyConnPrefix+connID, phyID, m.timeout*2)

	// 添加到服务器连接集合
	m.client.SAdd(ctx, m.serverConnsKey(), connID)
}

// UnbindByPhy 解除设备绑定
func (m *RedisManager) UnbindByPhy(phyID string) {
	ctx := context.Background()

	// 获取会话数据
	data, err := m.getSessionData(ctx, phyID)
	if err != nil {
		return
	}

	// 🔥 关键修复: 通知TCP层关闭连接，避免僵尸连接
	if data.ConnID != "" {
		// 获取连接对象
		m.mu.RLock()
		conn, ok := m.localConn[data.ConnID]
		m.mu.RUnlock()

		// 尝试关闭连接
		if ok {
			if closer, ok := conn.(interface{ Close() error }); ok {
				_ = closer.Close()
			}
		}

		// 删除本地连接缓存
		m.mu.Lock()
		delete(m.localConn, data.ConnID)
		m.mu.Unlock()

		// 删除连接映射
		m.client.Del(ctx, keyConnPrefix+data.ConnID)

		// 从服务器连接集合中移除
		m.client.SRem(ctx, m.serverConnsKey(), data.ConnID)
	}

	// 删除会话数据
	m.client.Del(ctx, keyDevicePrefix+phyID)
}

// OnTCPClosed 记录TCP断开事件
func (m *RedisManager) OnTCPClosed(phyID string, t time.Time) {
	ctx := context.Background()

	data, err := m.getSessionData(ctx, phyID)
	if err != nil {
		return
	}

	data.LastTCPDown = t
	m.setSessionData(ctx, phyID, data)
}

// OnAckTimeout 记录ACK超时事件
func (m *RedisManager) OnAckTimeout(phyID string, t time.Time) {
	ctx := context.Background()

	data, err := m.getSessionData(ctx, phyID)
	if err != nil {
		return
	}

	data.LastAckTimeout = t
	m.setSessionData(ctx, phyID, data)
}

// GetConn 获取绑定的连接对象（仅限本地连接）
// 增强版：验证连接有效性，包括心跳超时检查
func (m *RedisManager) GetConn(phyID string) (interface{}, bool) {
	ctx := context.Background()

	// 获取会话数据
	data, err := m.getSessionData(ctx, phyID)
	if err != nil {
		return nil, false
	}

	// 检查连接是否在本地服务器
	if data.ServerID != m.serverID {
		return nil, false
	}

	// 检查心跳是否超时（关键修复点1：防止使用僵尸连接）
	if time.Since(data.LastSeen) > m.timeout {
		// 心跳已超时，主动清理Session
		m.UnbindByPhy(phyID)
		return nil, false
	}

	// 从本地缓存获取连接
	m.mu.RLock()
	conn, ok := m.localConn[data.ConnID]
	m.mu.RUnlock()

	if !ok {
		// 连接已被清理，从Redis也清理
		m.UnbindByPhy(phyID)
		return nil, false
	}

	return conn, true
}

// IsOnline 判断设备是否在线（仅心跳）
func (m *RedisManager) IsOnline(phyID string, now time.Time) bool {
	ctx := context.Background()

	data, err := m.getSessionData(ctx, phyID)
	if err != nil {
		return false
	}

	return now.Sub(data.LastSeen) <= m.timeout
}

// IsOnlineWeighted 按加权策略判断设备是否在线
func (m *RedisManager) IsOnlineWeighted(phyID string, now time.Time, p WeightedPolicy) bool {
	if !p.Enabled {
		return m.IsOnline(phyID, now)
	}

	ctx := context.Background()
	data, err := m.getSessionData(ctx, phyID)
	if err != nil {
		return false
	}

	// 基础分：心跳新鲜则+1
	score := 0.0
	if now.Sub(data.LastSeen) <= p.HeartbeatTimeout {
		score += 1.0
	}

	// 近期 TCP down 惩罚
	if !data.LastTCPDown.IsZero() && p.TCPDownWindow > 0 && now.Sub(data.LastTCPDown) <= p.TCPDownWindow {
		score -= p.TCPDownPenalty
	}

	// 近期 ACK timeout 惩罚
	if !data.LastAckTimeout.IsZero() && p.AckWindow > 0 && now.Sub(data.LastAckTimeout) <= p.AckWindow {
		score -= p.AckTimeoutPenalty
	}

	return score >= p.Threshold
}

// OnlineCount 返回当前在线设备数量（仅心跳）
func (m *RedisManager) OnlineCount(now time.Time) int {
	ctx := context.Background()

	// 扫描所有设备会话
	var cursor uint64
	count := 0

	for {
		keys, nextCursor, err := m.client.Scan(ctx, cursor, keyDevicePrefix+"*", 100).Result()
		if err != nil {
			break
		}

		for _, key := range keys {
			phyID := key[len(keyDevicePrefix):]
			if m.IsOnline(phyID, now) {
				count++
			}
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	return count
}

// OnlineCountWeighted 返回按加权策略计算的在线设备数量
func (m *RedisManager) OnlineCountWeighted(now time.Time, p WeightedPolicy) int {
	if !p.Enabled {
		return m.OnlineCount(now)
	}

	ctx := context.Background()

	// 扫描所有设备会话
	var cursor uint64
	count := 0

	for {
		keys, nextCursor, err := m.client.Scan(ctx, cursor, keyDevicePrefix+"*", 100).Result()
		if err != nil {
			break
		}

		for _, key := range keys {
			phyID := key[len(keyDevicePrefix):]
			if m.IsOnlineWeighted(phyID, now, p) {
				count++
			}
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	return count
}

// --- 辅助方法 ---

func (m *RedisManager) getSessionData(ctx context.Context, phyID string) (*sessionData, error) {
	key := keyDevicePrefix + phyID
	val, err := m.client.Get(ctx, key).Result()
	if err != nil {
		return nil, err
	}

	var data sessionData
	if err := json.Unmarshal([]byte(val), &data); err != nil {
		return nil, err
	}

	return &data, nil
}

func (m *RedisManager) setSessionData(ctx context.Context, phyID string, data *sessionData) error {
	key := keyDevicePrefix + phyID

	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	// 设置过期时间为心跳超时的2倍
	return m.client.Set(ctx, key, jsonData, m.timeout*2).Err()
}

func (m *RedisManager) serverConnsKey() string {
	return fmt.Sprintf("%s%s:conns", keyServerConnsPrefix, m.serverID)
}

// Cleanup 清理本服务器的所有会话数据（用于优雅关闭）
func (m *RedisManager) Cleanup() error {
	ctx := context.Background()

	// 获取本服务器的所有连接ID
	connIDs, err := m.client.SMembers(ctx, m.serverConnsKey()).Result()
	if err != nil {
		return err
	}

	// 清理每个连接
	for _, connID := range connIDs {
		// 获取phyID
		phyID, err := m.client.Get(ctx, keyConnPrefix+connID).Result()
		if err != nil {
			continue
		}

		// 解绑设备
		m.UnbindByPhy(phyID)
	}

	// 删除服务器连接集合
	m.client.Del(ctx, m.serverConnsKey())

	return nil
}
