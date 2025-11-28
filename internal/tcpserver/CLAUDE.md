# TCPServer Module - TCP 服务器

> **导航**: [← 返回根目录](../../CLAUDE.md)
> **路径**: `internal/tcpserver/`
> **最后更新**: 2025-11-28

---

## 📋 模块职责

TCPServer 模块提供高性能 TCP 服务器基础设施：

- **TCP 监听**: 多端口监听（7011/7012/7013）
- **连接管理**: 连接池、超时管理
- **流量控制**: 限流、熔断保护
- **协议多路复用**: 路由到不同协议处理器
- **性能优化**: 连接复用、零拷贝

---

## 📂 文件结构

```
tcpserver/
├── server.go              # TCP 服务器主体
├── conn.go                # 连接管理
├── mux.go                 # 协议多路复用器
├── mux_test.go            # 多路复用测试
├── rate_limiter.go        # 限流器
├── limiter.go             # 限流实现
├── limiter_test.go        # 限流测试
├── circuit_breaker.go     # 熔断器
└── circuit_breaker_test.go # 熔断测试
```

---

## 🔑 核心组件

### TCPServer

```go
type TCPServer struct {
    addr     string
    listener net.Listener
    handler  ConnectionHandler
    limiter  *RateLimiter
    breaker  *CircuitBreaker
}

func (s *TCPServer) Start() error {
    listener, err := net.Listen("tcp", s.addr)
    if err != nil {
        return err
    }

    for {
        conn, err := listener.Accept()
        if err != nil {
            continue
        }
        go s.handleConn(conn)
    }
}
```

### 限流器

```go
type RateLimiter struct {
    rate  int           // 每秒最大连接数
    burst int           // 突发容量
    limiter *rate.Limiter
}

func (rl *RateLimiter) Allow() bool {
    return rl.limiter.Allow()
}
```

### 熔断器

```go
type CircuitBreaker struct {
    maxFailures int
    timeout     time.Duration
    state       State  // Closed/Open/HalfOpen
}

func (cb *CircuitBreaker) Call(fn func() error) error {
    if cb.state == Open {
        return ErrCircuitOpen
    }
    return fn()
}
```

---

## 🔒 保护机制

### 连接限流

```yaml
tcp:
  rate_limit:
    connections_per_second: 100
    burst: 200
```

### 熔断保护

```yaml
tcp:
  circuit_breaker:
    max_failures: 10
    timeout: 30s
```

---

## 🔗 相关文档

- [Gateway Module](../gateway/CLAUDE.md)
- [Session Module](../session/CLAUDE.md)

---

**最后更新**: 2025-11-28
