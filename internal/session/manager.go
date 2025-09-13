package session

import (
	"sync"
	"time"
)

// WeightedPolicy 多信号在线判定策略（权重/时间窗/阈值）
type WeightedPolicy struct {
	Enabled           bool
	HeartbeatTimeout  time.Duration
	TCPDownWindow     time.Duration
	AckWindow         time.Duration
	TCPDownPenalty    float64
	AckTimeoutPenalty float64
	Threshold         float64
}

// Manager 会话管理最小实现：记录设备最近心跳时间，判断是否在线
type Manager struct {
	mu       sync.RWMutex
	lastSeen map[string]time.Time // phyID -> last seen
	timeout  time.Duration
	conns    map[string]interface{}
	// 多信号占位
	lastTCPDown    map[string]time.Time // 最近 TCP 断开时间
	lastAckTimeout map[string]time.Time // 最近 ACK 超时时间
}

func New(timeout time.Duration) *Manager {
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	return &Manager{lastSeen: make(map[string]time.Time), timeout: timeout, conns: make(map[string]interface{}), lastTCPDown: make(map[string]time.Time), lastAckTimeout: make(map[string]time.Time)}
}

// OnHeartbeat 更新设备最近心跳时间
func (m *Manager) OnHeartbeat(phyID string, t time.Time) {
	m.mu.Lock()
	m.lastSeen[phyID] = t
	m.mu.Unlock()
}

// Bind 绑定设备物理ID到连接对象（opaque），重复绑定将覆盖
func (m *Manager) Bind(phyID string, conn interface{}) {
	m.mu.Lock()
	m.conns[phyID] = conn
	m.mu.Unlock()
}

// UnbindByPhy 解除绑定
func (m *Manager) UnbindByPhy(phyID string) {
	m.mu.Lock()
	delete(m.conns, phyID)
	m.mu.Unlock()
}

// OnTCPClosed 记录 TCP 断开事件（多信号离线判定占位）
func (m *Manager) OnTCPClosed(phyID string, t time.Time) {
	m.mu.Lock()
	m.lastTCPDown[phyID] = t
	m.mu.Unlock()
}

// OnAckTimeout 记录 ACK 超时事件（多信号离线判定占位）
func (m *Manager) OnAckTimeout(phyID string, t time.Time) {
	m.mu.Lock()
	m.lastAckTimeout[phyID] = t
	m.mu.Unlock()
}

// GetConn 返回绑定的连接对象
func (m *Manager) GetConn(phyID string) (interface{}, bool) {
	m.mu.RLock()
	c, ok := m.conns[phyID]
	m.mu.RUnlock()
	return c, ok
}

// IsOnline 判断设备是否在线（仅心跳）
func (m *Manager) IsOnline(phyID string, now time.Time) bool {
	m.mu.RLock()
	ts, ok := m.lastSeen[phyID]
	m.mu.RUnlock()
	if !ok {
		return false
	}
	return now.Sub(ts) <= m.timeout
}

// IsOnlineWeighted 按加权策略判断设备是否在线
func (m *Manager) IsOnlineWeighted(phyID string, now time.Time, p WeightedPolicy) bool {
	if !p.Enabled {
		return m.IsOnline(phyID, now)
	}
	m.mu.RLock()
	ls, ok := m.lastSeen[phyID]
	ltcp := m.lastTCPDown[phyID]
	lack := m.lastAckTimeout[phyID]
	m.mu.RUnlock()
	if !ok {
		return false
	}
	// 基础分：心跳新鲜则+1
	score := 0.0
	if now.Sub(ls) <= p.HeartbeatTimeout {
		score += 1.0
	}
	// 近期 TCP down 惩罚
	if !ltcp.IsZero() && p.TCPDownWindow > 0 && now.Sub(ltcp) <= p.TCPDownWindow {
		score -= p.TCPDownPenalty
	}
	// 近期 ACK timeout 惩罚
	if !lack.IsZero() && p.AckWindow > 0 && now.Sub(lack) <= p.AckWindow {
		score -= p.AckTimeoutPenalty
	}
	return score >= p.Threshold
}

// OnlineCount 返回当前在线设备数量（仅心跳）
func (m *Manager) OnlineCount(now time.Time) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	count := 0
	for _, ts := range m.lastSeen {
		if now.Sub(ts) <= m.timeout {
			count++
		}
	}
	return count
}

// OnlineCountWeighted 返回按加权策略计算的在线设备数量
func (m *Manager) OnlineCountWeighted(now time.Time, p WeightedPolicy) int {
	if !p.Enabled {
		return m.OnlineCount(now)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	count := 0
	for phy, ts := range m.lastSeen {
		score := 0.0
		if now.Sub(ts) <= p.HeartbeatTimeout {
			score += 1.0
		}
		if ltcp, ok := m.lastTCPDown[phy]; ok && !ltcp.IsZero() && p.TCPDownWindow > 0 && now.Sub(ltcp) <= p.TCPDownWindow {
			score -= p.TCPDownPenalty
		}
		if lack, ok := m.lastAckTimeout[phy]; ok && !lack.IsZero() && p.AckWindow > 0 && now.Sub(lack) <= p.AckWindow {
			score -= p.AckTimeoutPenalty
		}
		if score >= p.Threshold {
			count++
		}
	}
	return count
}
