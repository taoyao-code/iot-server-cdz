# Session Module - 会话管理

> **导航**: [← 返回根目录](../../CLAUDE.md)
> **路径**: `internal/session/`
> **最后更新**: 2025-11-28

---

## 📋 模块职责

Session 模块负责管理设备的 TCP 连接会话和在线状态判定：

- **会话绑定**: 设备物理ID与TCP连接的映射
- **心跳追踪**: 记录设备最后心跳时间
- **在线判定**: 基于心跳和连接状态的在线判断
- **加权策略**: 多信号综合判定（心跳+TCP+ACK）
- **Redis存储**: 分布式会话共享

---

## 📂 文件结构

```
session/
├── interface.go           # SessionManager 接口定义
├── redis_manager.go       # Redis 实现
└── redis_manager_test.go  # 测试
```

---

## 🔑 核心接口

### SessionManager

```go
type SessionManager interface {
    // 心跳管理
    OnHeartbeat(phyID string, t time.Time)

    // 连接绑定
    Bind(phyID string, conn interface{})
    UnbindByPhy(phyID string)
    GetConn(phyID string) (interface{}, bool)

    // 事件记录
    OnTCPClosed(phyID string, t time.Time)
    OnAckTimeout(phyID string, t time.Time)

    // 在线判定
    IsOnline(phyID string, now time.Time) bool
    IsOnlineWeighted(phyID string, now time.Time, p WeightedPolicy) bool

    // 统计
    OnlineCount(now time.Time) int
    OnlineCountWeighted(now time.Time, p WeightedPolicy) int
}
```

---

## 🎯 加权策略

### WeightedPolicy 结构

```go
type WeightedPolicy struct {
    Enabled           bool
    HeartbeatTimeout  time.Duration
    TCPDownWindow     time.Duration
    AckWindow         time.Duration
    TCPDownPenalty    float64
    AckTimeoutPenalty float64
    Threshold         float64
}
```

### 在线判定算法

```
score = 1.0
if heartbeat_timeout:
    score = 0.0
if tcp_closed_in_window:
    score -= TCPDownPenalty
if ack_timeout_in_window:
    score -= AckTimeoutPenalty

online = score >= Threshold
```

**示例配置**:
```yaml
session:
  heartbeat_timeout: 300s
  tcp_down_window: 60s
  ack_window: 30s
  tcp_down_penalty: 0.3
  ack_timeout_penalty: 0.2
  threshold: 0.6
```

---

## 💾 Redis 实现

### 数据结构

**Hash 键**:
- `session:heartbeat:{phyID}` - 最后心跳时间戳
- `session:tcp_closed:{phyID}` - TCP 关闭时间戳
- `session:ack_timeout:{phyID}` - ACK 超时时间戳

**内存存储**:
- `connections map[string]interface{}` - 连接对象映射

---

## 🔗 相关文档

- [App Module](../app/CLAUDE.md)
- [TCP Server](../tcpserver/CLAUDE.md)

---

**最后更新**: 2025-11-28
