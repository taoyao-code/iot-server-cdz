package session

import (
	"sync"
	"time"
)

// Manager 会话管理最小实现：记录设备最近心跳时间，判断是否在线
type Manager struct {
	mu       sync.RWMutex
	lastSeen map[string]time.Time // phyID -> last seen
	timeout  time.Duration
	conns    map[string]interface{}
}

func New(timeout time.Duration) *Manager {
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	return &Manager{lastSeen: make(map[string]time.Time), timeout: timeout, conns: make(map[string]interface{})}
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

// GetConn 返回绑定的连接对象
func (m *Manager) GetConn(phyID string) (interface{}, bool) {
	m.mu.RLock()
	c, ok := m.conns[phyID]
	m.mu.RUnlock()
	return c, ok
}

// IsOnline 判断设备是否在线
func (m *Manager) IsOnline(phyID string, now time.Time) bool {
	m.mu.RLock()
	ts, ok := m.lastSeen[phyID]
	m.mu.RUnlock()
	if !ok {
		return false
	}
	return now.Sub(ts) <= m.timeout
}

// OnlineCount 返回当前在线设备数量
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
