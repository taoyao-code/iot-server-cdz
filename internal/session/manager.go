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
}

func New(timeout time.Duration) *Manager {
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	return &Manager{lastSeen: make(map[string]time.Time), timeout: timeout}
}

// OnHeartbeat 更新设备最近心跳时间
func (m *Manager) OnHeartbeat(phyID string, t time.Time) {
	m.mu.Lock()
	m.lastSeen[phyID] = t
	m.mu.Unlock()
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
