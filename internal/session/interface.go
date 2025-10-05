package session

import "time"

// SessionManager 会话管理器接口，支持内存和Redis两种实现
type SessionManager interface {
	// OnHeartbeat 更新设备最近心跳时间
	OnHeartbeat(phyID string, t time.Time)

	// Bind 绑定设备物理ID到连接对象
	Bind(phyID string, conn interface{})

	// UnbindByPhy 解除绑定
	UnbindByPhy(phyID string)

	// OnTCPClosed 记录TCP断开事件
	OnTCPClosed(phyID string, t time.Time)

	// OnAckTimeout 记录ACK超时事件
	OnAckTimeout(phyID string, t time.Time)

	// GetConn 返回绑定的连接对象
	GetConn(phyID string) (interface{}, bool)

	// IsOnline 判断设备是否在线（仅心跳）
	IsOnline(phyID string, now time.Time) bool

	// IsOnlineWeighted 按加权策略判断设备是否在线
	IsOnlineWeighted(phyID string, now time.Time, p WeightedPolicy) bool

	// OnlineCount 返回当前在线设备数量（仅心跳）
	OnlineCount(now time.Time) int

	// OnlineCountWeighted 返回按加权策略计算的在线设备数量
	OnlineCountWeighted(now time.Time, p WeightedPolicy) int
}
