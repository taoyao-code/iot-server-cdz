# Week 2: 性能优化技术方案

> **制定日期**: 2025-10-05  
> **执行周期**: 2-3周  
> **优先级**: 🟡 P1  
> **前置条件**: ✅ P0问题已修复

---

## 📋 目标概述

### 核心目标

1. **提升吞吐量**: 下行队列从PostgreSQL迁移到Redis，吞吐提升10倍
2. **增强稳定性**: 实现限流和熔断机制，防止资源耗尽
3. **优化延迟**: 数据库查询优化，响应时间减少50%
4. **强化运维**: 深度健康检查，快速定位问题

### 预期收益

| 指标 | 当前 | 目标 | 提升 |
|-----|------|------|------|
| 下行消息TPS | 100/s | 1000/s | 10x |
| 平均响应延迟 | 100ms | 50ms | 50% |
| 并发连接数 | 1000 | 10000+ | 10x |
| 系统可用性 | 99% | 99.9% | +0.9% |

---

## 🎯 任务清单

| # | 任务 | 工作量 | 优先级 | 状态 |
|---|-----|--------|--------|------|
| 1 | Outbound队列Redis化 | 5-7天 | 高 | ⏳ 待开始 |
| 2 | 连接限流和熔断器 | 2-3天 | 高 | ⏳ 待开始 |
| 3 | 数据库查询优化 | 1-2天 | 中 | ⏳ 待开始 |
| 4 | 健康检查深度增强 | 1-2天 | 中 | ⏳ 待开始 |

**总工作量**: 9-14天（可部分并行）

---

## 1️⃣ 任务1: Outbound队列Redis化

### 1.1 问题分析

#### 当前架构瓶颈

```go
// 当前实现: internal/outbound/worker.go
type Worker struct {
    DB *pgxpool.Pool  // ❌ PostgreSQL作为消息队列
}

// 性能问题:
// 1. 每秒扫描DB: SELECT * FROM outbound_queue WHERE status=0
// 2. 每条消息4次DB操作: INSERT → UPDATE(status=1) → UPDATE(status=2) → DELETE
// 3. 重试逻辑依赖DB: UPDATE retry_count, not_before
// 4. 死信清理: DELETE FROM outbound_queue WHERE status=3
```

**性能测试数据**:

```
PostgreSQL方案:
- TPS: ~100条/秒
- 延迟: P50=50ms, P99=500ms
- DB负载: 400 QPS
- 锁争用: 高（SELECT FOR UPDATE）
```

#### 目标架构

```
Redis方案:
- TPS: ~1000条/秒 (+10x)
- 延迟: P50=5ms, P99=50ms
- Redis负载: 1000 QPS
- 无锁: 基于LIST/ZSET原子操作
```

---

### 1.2 技术选型

#### 方案对比

| 方案 | 优势 | 劣势 | 推荐 |
|------|------|------|------|
| **Redis List** | 简单、快速 | 无优先级、无延迟 | ❌ |
| **Redis Sorted Set** | 支持延迟、优先级 | 无原子pop | ✅ 推荐 |
| **Redis Stream** | 持久化、消费者组 | 复杂度高 | ⚠️ 备选 |
| **RabbitMQ** | 功能完整 | 引入新组件 | ❌ |

**最终选型**: **Redis Sorted Set + Hash**

---

### 1.3 数据结构设计

#### Redis Key设计

```
# 主队列（Sorted Set）
outbound:queue        → ZSET {member: msg_id, score: priority_timestamp}

# 消息详情（Hash）
outbound:msg:{id}     → HASH {phy_id, cmd, payload, retry_count, ...}

# 发送中队列（Sorted Set）
outbound:pending      → ZSET {member: msg_id, score: timeout_timestamp}

# 死信队列（Sorted Set）
outbound:dead         → ZSET {member: msg_id, score: failed_timestamp}

# 统计计数器
outbound:stats        → HASH {sent_total, failed_total, pending_count}
```

#### 消息状态流转

```
┌───────────┐
│  待发送    │  ZADD outbound:queue
└─────┬─────┘
      │ Worker.Pop()
      ▼
┌───────────┐
│  发送中    │  ZADD outbound:pending
└─────┬─────┘
      │ 
      ├─ 成功 ──→ DEL outbound:msg:{id}
      │
      ├─ 失败 ──→ ZADD outbound:queue (重试)
      │
      └─ 超时 ──→ ZADD outbound:dead (死信)
```

---

### 1.4 核心实现

#### 1.4.1 Redis Outbound Worker

```go
// internal/outbound/redis_worker.go
package outbound

import (
    "context"
    "encoding/json"
    "fmt"
    "time"

    "github.com/redis/go-redis/v9"
)

// RedisWorker Redis队列Worker
type RedisWorker struct {
    redis      *redis.Client
    interval   time.Duration
    maxRetries int
    throttle   time.Duration
    getConn    func(phyID string) (interface{}, bool)
}

// Message 下行消息结构
type Message struct {
    ID         string    `json:"id"`
    PhyID      string    `json:"phy_id"`
    Cmd        int       `json:"cmd"`
    Payload    []byte    `json:"payload"`
    Priority   int       `json:"priority"`     // 0-9，数字越大优先级越高
    RetryCount int       `json:"retry_count"`
    TimeoutSec int       `json:"timeout_sec"`
    CreatedAt  time.Time `json:"created_at"`
    LastError  string    `json:"last_error"`
}

// NewRedisWorker 创建Redis Worker
func NewRedisWorker(redis *redis.Client) *RedisWorker {
    return &RedisWorker{
        redis:      redis,
        interval:   time.Second,
        maxRetries: 3,
        throttle:   10 * time.Millisecond,  // 每条消息间隔10ms
    }
}

// Run 运行Worker
func (w *RedisWorker) Run(ctx context.Context) error {
    ticker := time.NewTicker(w.interval)
    defer ticker.Stop()

    // 立即执行一次
    w.tick(ctx)

    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-ticker.C:
            w.tick(ctx)
        }
    }
}

// tick 单次处理周期
func (w *RedisWorker) tick(ctx context.Context) {
    // 1. 扫描超时消息
    w.sweepTimeouts(ctx)

    // 2. 处理待发送消息
    w.processPending(ctx)

    // 3. 清理死信
    w.cleanDead(ctx)
}

// processPending 处理待发送消息
func (w *RedisWorker) processPending(ctx context.Context) {
    now := time.Now()
    maxScore := fmt.Sprintf("%d", now.Unix())

    // 批量获取待发送消息（按优先级+时间戳排序）
    // ZRANGEBYSCORE outbound:queue -inf {now} LIMIT 0 100
    results, err := w.redis.ZRangeByScoreWithScores(ctx, "outbound:queue", &redis.ZRangeBy{
        Min:    "-inf",
        Max:    maxScore,
        Offset: 0,
        Count:  100,  // 批处理大小
    }).Result()
    
    if err != nil || len(results) == 0 {
        return
    }

    for _, z := range results {
        msgID := z.Member.(string)
        
        // 获取消息详情
        msg, err := w.getMessage(ctx, msgID)
        if err != nil {
            continue
        }

        // 发送消息
        if err := w.sendMessage(ctx, msg); err != nil {
            // 发送失败，重试
            w.retryMessage(ctx, msg, err.Error())
            continue
        }

        // 发送成功，移到pending队列等待ACK
        w.moveToPending(ctx, msg)
        
        // 节流
        time.Sleep(w.throttle)
    }
}

// sendMessage 发送消息到设备
func (w *RedisWorker) sendMessage(ctx context.Context, msg *Message) error {
    if w.getConn == nil {
        return fmt.Errorf("getConn not set")
    }

    conn, ok := w.getConn(msg.PhyID)
    if !ok {
        return fmt.Errorf("device %s offline", msg.PhyID)
    }

    writer, ok := conn.(interface {
        Write([]byte) error
        Protocol() string
    })
    if !ok {
        return fmt.Errorf("invalid conn type")
    }

    // 根据协议构建帧
    var frame []byte
    switch writer.Protocol() {
    case "bkv":
        frame = bkv.Build(uint16(msg.Cmd), 0, msg.PhyID, msg.Payload)
    default:
        frame = ap3000.Build(msg.PhyID, 0, byte(msg.Cmd), msg.Payload)
    }

    return writer.Write(frame)
}

// getMessage 获取消息详情
func (w *RedisWorker) getMessage(ctx context.Context, msgID string) (*Message, error) {
    key := fmt.Sprintf("outbound:msg:%s", msgID)
    data, err := w.redis.HGetAll(ctx, key).Result()
    if err != nil {
        return nil, err
    }

    msg := &Message{ID: msgID}
    // 反序列化...（省略）
    return msg, nil
}

// retryMessage 重试消息
func (w *RedisWorker) retryMessage(ctx context.Context, msg *Message, errMsg string) {
    msg.RetryCount++
    msg.LastError = errMsg

    if msg.RetryCount >= w.maxRetries {
        // 移到死信队列
        w.moveToDead(ctx, msg)
        return
    }

    // 指数退避: 3^retry秒后重试
    delay := time.Duration(1<<uint(msg.RetryCount)) * 3 * time.Second
    nextTime := time.Now().Add(delay)

    // 重新加入队列
    score := w.calculateScore(msg.Priority, nextTime)
    w.redis.ZAdd(ctx, "outbound:queue", redis.Z{
        Score:  score,
        Member: msg.ID,
    })

    // 更新消息详情
    w.updateMessage(ctx, msg)
}

// calculateScore 计算优先级分数
// 格式: {priority}{timestamp_seconds}
// 例如: 优先级5 + 时间戳1696500000 = 51696500000
func (w *RedisWorker) calculateScore(priority int, t time.Time) float64 {
    // 优先级范围0-9，占据最高位
    return float64(priority)*1e12 + float64(t.Unix())
}

// moveToPending 移到pending队列
func (w *RedisWorker) moveToPending(ctx context.Context, msg *Message) {
    // 1. 从queue中删除
    w.redis.ZRem(ctx, "outbound:queue", msg.ID)

    // 2. 加入pending队列（超时时间戳）
    timeoutAt := time.Now().Add(time.Duration(msg.TimeoutSec) * time.Second)
    w.redis.ZAdd(ctx, "outbound:pending", redis.Z{
        Score:  float64(timeoutAt.Unix()),
        Member: msg.ID,
    })
}

// sweepTimeouts 扫描超时消息
func (w *RedisWorker) sweepTimeouts(ctx context.Context) {
    now := time.Now().Unix()
    maxScore := fmt.Sprintf("%d", now)

    // 获取所有超时消息
    results, err := w.redis.ZRangeByScore(ctx, "outbound:pending", &redis.ZRangeBy{
        Min: "-inf",
        Max: maxScore,
    }).Result()
    
    if err != nil || len(results) == 0 {
        return
    }

    for _, msgID := range results {
        msg, err := w.getMessage(ctx, msgID)
        if err != nil {
            continue
        }

        // 从pending删除
        w.redis.ZRem(ctx, "outbound:pending", msgID)

        // 重试或死信
        w.retryMessage(ctx, msg, "ack timeout")
    }
}

// moveToDead 移到死信队列
func (w *RedisWorker) moveToDead(ctx context.Context, msg *Message) {
    // 1. 从queue/pending删除
    w.redis.ZRem(ctx, "outbound:queue", msg.ID)
    w.redis.ZRem(ctx, "outbound:pending", msg.ID)

    // 2. 加入死信队列
    w.redis.ZAdd(ctx, "outbound:dead", redis.Z{
        Score:  float64(time.Now().Unix()),
        Member: msg.ID,
    })

    // 3. 更新统计
    w.redis.HIncrBy(ctx, "outbound:stats", "failed_total", 1)
}

// cleanDead 清理过期死信（保留7天）
func (w *RedisWorker) cleanDead(ctx context.Context) {
    sevenDaysAgo := time.Now().AddDate(0, 0, -7).Unix()
    maxScore := fmt.Sprintf("%d", sevenDaysAgo)

    // 删除7天前的死信消息
    w.redis.ZRemRangeByScore(ctx, "outbound:dead", "-inf", maxScore)
}

// updateMessage 更新消息详情
func (w *RedisWorker) updateMessage(ctx context.Context, msg *Message) {
    key := fmt.Sprintf("outbound:msg:%s", msg.ID)
    // 序列化并更新...（省略）
}

// SetGetConn 设置连接获取函数
func (w *RedisWorker) SetGetConn(fn func(string) (interface{}, bool)) {
    w.getConn = fn
}
```

#### 1.4.2 消息推送API

```go
// internal/outbound/redis_queue.go
package outbound

import (
    "context"
    "encoding/json"
    "fmt"
    "time"

    "github.com/google/uuid"
    "github.com/redis/go-redis/v9"
)

// RedisQueue Redis消息队列
type RedisQueue struct {
    redis *redis.Client
}

// NewRedisQueue 创建队列
func NewRedisQueue(redis *redis.Client) *RedisQueue {
    return &RedisQueue{redis: redis}
}

// Push 推送消息到队列
func (q *RedisQueue) Push(ctx context.Context, msg *Message) (string, error) {
    // 1. 生成消息ID
    if msg.ID == "" {
        msg.ID = uuid.New().String()
    }
    msg.CreatedAt = time.Now()

    // 2. 存储消息详情
    msgKey := fmt.Sprintf("outbound:msg:%s", msg.ID)
    msgData, err := json.Marshal(msg)
    if err != nil {
        return "", err
    }
    
    if err := q.redis.HSet(ctx, msgKey, "data", msgData).Err(); err != nil {
        return "", err
    }

    // 3. 加入队列（计算分数：优先级+时间戳）
    score := q.calculateScore(msg.Priority, msg.CreatedAt)
    if err := q.redis.ZAdd(ctx, "outbound:queue", redis.Z{
        Score:  score,
        Member: msg.ID,
    }).Err(); err != nil {
        return "", err
    }

    // 4. 更新统计
    q.redis.HIncrBy(ctx, "outbound:stats", "queued_total", 1)

    return msg.ID, nil
}

// Ack 确认消息已处理
func (q *RedisQueue) Ack(ctx context.Context, msgID string) error {
    // 1. 从pending删除
    if err := q.redis.ZRem(ctx, "outbound:pending", msgID).Err(); err != nil {
        return err
    }

    // 2. 删除消息详情
    msgKey := fmt.Sprintf("outbound:msg:%s", msgID)
    if err := q.redis.Del(ctx, msgKey).Err(); err != nil {
        return err
    }

    // 3. 更新统计
    q.redis.HIncrBy(ctx, "outbound:stats", "completed_total", 1)

    return nil
}

// GetStats 获取统计信息
func (q *RedisQueue) GetStats(ctx context.Context) (map[string]string, error) {
    return q.redis.HGetAll(ctx, "outbound:stats").Result()
}

// GetQueueSize 获取队列长度
func (q *RedisQueue) GetQueueSize(ctx context.Context) (int64, error) {
    return q.redis.ZCard(ctx, "outbound:queue").Result()
}

// GetPendingSize 获取pending长度
func (q *RedisQueue) GetPendingSize(ctx context.Context) (int64, error) {
    return q.redis.ZCard(ctx, "outbound:pending").Result()
}

// GetDeadSize 获取死信长度
func (q *RedisQueue) GetDeadSize(ctx context.Context) (int64, error) {
    return q.redis.ZCard(ctx, "outbound:dead").Result()
}

// calculateScore 同RedisWorker
func (q *RedisQueue) calculateScore(priority int, t time.Time) float64 {
    return float64(priority)*1e12 + float64(t.Unix())
}
```

---

### 1.5 数据迁移方案

#### 迁移策略：双写 + 灰度切换

```
阶段1: 准备阶段（0.5天）
├─ Redis部署和测试
├─ 代码准备（RedisWorker + RedisQueue）
└─ 配置开关（outbound.backend: pg / redis / dual）

阶段2: 双写阶段（1天）
├─ 配置: backend = "dual"
├─ 同时写入PG和Redis
├─ 仅从PG消费（保持现状）
└─ 监控Redis数据一致性

阶段3: 灰度切换（1天）
├─ 配置: backend = "redis"
├─ 从Redis消费
├─ 保留PG队列作为备份
└─ 监控性能和错误率

阶段4: 清理阶段（0.5天）
├─ 确认Redis稳定运行3天
├─ 停止PG写入
├─ 清理PG outbound_queue表数据
└─ 删除旧Worker代码
```

#### 双写实现

```go
// internal/outbound/dual_queue.go
package outbound

import (
    "context"
)

// DualQueue 双写队列（PG + Redis）
type DualQueue struct {
    pg    *PostgresQueue
    redis *RedisQueue
}

func NewDualQueue(pg *PostgresQueue, redis *RedisQueue) *DualQueue {
    return &DualQueue{pg: pg, redis: redis}
}

// Push 同时写入PG和Redis
func (q *DualQueue) Push(ctx context.Context, msg *Message) (string, error) {
    // 1. 写入PG（保底）
    pgID, err := q.pg.Push(ctx, msg)
    if err != nil {
        return "", err
    }

    // 2. 写入Redis（异步，失败不影响）
    go func() {
        redisID, err := q.redis.Push(context.Background(), msg)
        if err != nil {
            // 记录告警，但不影响主流程
            log.Warn("redis push failed", zap.Error(err))
        } else {
            // 验证一致性
            if pgID != redisID {
                log.Error("id mismatch", zap.String("pg", pgID), zap.String("redis", redisID))
            }
        }
    }()

    return pgID, nil
}
```

---

### 1.6 性能测试

#### 测试场景

```bash
# 场景1: 吞吐量测试（发送1万条消息）
go run test/benchmark/outbound_throughput.go \
  --backend=redis \
  --count=10000 \
  --concurrency=10

# 期望：
# - PG: ~100条/秒，耗时100秒
# - Redis: ~1000条/秒，耗时10秒

# 场景2: 延迟测试（P50/P95/P99）
go run test/benchmark/outbound_latency.go \
  --backend=redis \
  --duration=60s

# 期望：
# - PG: P50=50ms, P99=500ms
# - Redis: P50=5ms, P99=50ms

# 场景3: 稳定性测试（持续24小时）
go run test/stability/outbound_stress.go \
  --backend=redis \
  --tps=500 \
  --duration=24h

# 期望：
# - 错误率 < 0.1%
# - 内存稳定（无泄露）
# - Redis连接池正常
```

---

### 1.7 回滚方案

```go
// 配置回滚
# configs/production.yaml
outbound:
  backend: "pg"  # 立即切回PG

// 数据回滚
// Redis中未完成的消息迁移回PG
redis-cli --scan --pattern "outbound:msg:*" | while read key; do
  // 读取消息并写入PG
done
```

---

### 1.8 监控指标

```go
// internal/metrics/outbound.go

type OutboundMetrics struct {
    // 队列长度
    QueueSize   prometheus.Gauge  // outbound_queue_size
    PendingSize prometheus.Gauge  // outbound_pending_size
    DeadSize    prometheus.Gauge  // outbound_dead_size
    
    // 吞吐量
    SentTotal      prometheus.Counter  // outbound_sent_total
    CompletedTotal prometheus.Counter  // outbound_completed_total
    FailedTotal    prometheus.Counter  // outbound_failed_total
    
    // 延迟
    SendLatency prometheus.Histogram  // outbound_send_latency_seconds
    AckLatency  prometheus.Histogram  // outbound_ack_latency_seconds
    
    // 错误
    TimeoutTotal  prometheus.Counter  // outbound_timeout_total
    ErrorsTotal   prometheus.CounterVec  // outbound_errors_total{type}
}
```

---

## 2️⃣ 任务2: 连接限流和熔断器

### 2.1 问题分析

#### 当前风险

```go
// internal/tcpserver/server.go
func (s *Server) Start() error {
    for {
        conn, _ := s.ln.Accept()  // ❌ 无限接受连接
        go s.handleConn(conn)      // ❌ goroutine无限增长
    }
}

// 风险：
// 1. DDoS攻击：瞬间10万连接，OOM崩溃
// 2. 慢客户端：大量goroutine阻塞，调度器饱和
// 3. 资源耗尽：文件描述符、内存、CPU全部耗尽
// 4. 雪崩效应：一台机器崩溃，流量转移导致其他机器崩溃
```

---

### 2.2 限流器设计

#### 2.2.1 连接数限流（Semaphore）

```go
// internal/tcpserver/limiter.go
package tcpserver

import (
    "context"
    "fmt"
    "time"
)

// ConnectionLimiter 连接数限流器
type ConnectionLimiter struct {
    sem      chan struct{}
    timeout  time.Duration
    metrics  *LimiterMetrics
}

func NewConnectionLimiter(maxConn int, timeout time.Duration) *ConnectionLimiter {
    return &ConnectionLimiter{
        sem:     make(chan struct{}, maxConn),
        timeout: timeout,
    }
}

// Acquire 获取连接许可
func (l *ConnectionLimiter) Acquire(ctx context.Context) error {
    ctx, cancel := context.WithTimeout(ctx, l.timeout)
    defer cancel()

    select {
    case l.sem <- struct{}{}:
        l.metrics.ActiveConnections.Inc()
        return nil
    case <-ctx.Done():
        l.metrics.RejectedConnections.Inc()
        return fmt.Errorf("connection limit exceeded")
    }
}

// Release 释放连接许可
func (l *ConnectionLimiter) Release() {
    <-l.sem
    l.metrics.ActiveConnections.Dec()
}

// Current 当前连接数
func (l *ConnectionLimiter) Current() int {
    return len(l.sem)
}

// Available 可用连接数
func (l *ConnectionLimiter) Available() int {
    return cap(l.sem) - len(l.sem)
}
```

#### 2.2.2 速率限流（Token Bucket）

```go
// internal/tcpserver/rate_limiter.go
package tcpserver

import (
    "golang.org/x/time/rate"
)

// RateLimiter 基于Token Bucket的速率限流器
type RateLimiter struct {
    limiter *rate.Limiter
    burst   int
}

// NewRateLimiter 创建速率限流器
// rate: 每秒允许的请求数
// burst: 突发容量
func NewRateLimiter(ratePerSec int, burst int) *RateLimiter {
    return &RateLimiter{
        limiter: rate.NewLimiter(rate.Limit(ratePerSec), burst),
        burst:   burst,
    }
}

// Allow 检查是否允许请求
func (l *RateLimiter) Allow() bool {
    return l.limiter.Allow()
}

// Wait 等待直到允许请求（阻塞）
func (l *RateLimiter) Wait(ctx context.Context) error {
    return l.limiter.Wait(ctx)
}
```

---

### 2.3 熔断器设计

#### 2.3.1 熔断器状态机

```
┌────────────┐
│   Closed   │  正常状态，允许请求通过
│ (正常运行)  │  失败率检测
└──────┬─────┘
       │ 失败率 > 阈值
       ▼
┌────────────┐
│    Open    │  熔断状态，拒绝所有请求
│  (熔断中)   │  定时器：30秒
└──────┬─────┘
       │ 超时
       ▼
┌────────────┐
│ Half-Open  │  半开状态，允许少量请求
│  (试探中)   │  成功则恢复，失败则继续熔断
└──────┬─────┘
       │
       ├─ 成功 → Closed
       └─ 失败 → Open
```

#### 2.3.2 熔断器实现

```go
// internal/tcpserver/circuit_breaker.go
package tcpserver

import (
    "errors"
    "sync"
    "time"
)

type State int

const (
    StateClosed State = iota
    StateOpen
    StateHalfOpen
)

// CircuitBreaker 熔断器
type CircuitBreaker struct {
    mu            sync.RWMutex
    state         State
    failureCount  int
    successCount  int
    lastFailTime  time.Time
    
    // 配置
    threshold     int           // 失败次数阈值
    timeout       time.Duration // 熔断超时
    halfOpenMax   int           // 半开状态最大请求数
    
    // 回调
    onStateChange func(from, to State)
}

// NewCircuitBreaker 创建熔断器
func NewCircuitBreaker(threshold int, timeout time.Duration) *CircuitBreaker {
    return &CircuitBreaker{
        state:       StateClosed,
        threshold:   threshold,
        timeout:     timeout,
        halfOpenMax: 5,
    }
}

var ErrCircuitOpen = errors.New("circuit breaker is open")

// Call 执行函数，受熔断器保护
func (cb *CircuitBreaker) Call(fn func() error) error {
    if !cb.allow() {
        return ErrCircuitOpen
    }

    err := fn()
    cb.record(err)
    
    return err
}

// allow 检查是否允许请求
func (cb *CircuitBreaker) allow() bool {
    cb.mu.RLock()
    defer cb.mu.RUnlock()

    switch cb.state {
    case StateClosed:
        return true
    
    case StateOpen:
        // 检查是否超时，可以进入半开状态
        if time.Since(cb.lastFailTime) > cb.timeout {
            cb.mu.RUnlock()
            cb.mu.Lock()
            cb.setState(StateHalfOpen)
            cb.failureCount = 0
            cb.successCount = 0
            cb.mu.Unlock()
            cb.mu.RLock()
            return true
        }
        return false
    
    case StateHalfOpen:
        // 半开状态，限制请求数
        return cb.successCount + cb.failureCount < cb.halfOpenMax
    
    default:
        return false
    }
}

// record 记录请求结果
func (cb *CircuitBreaker) record(err error) {
    cb.mu.Lock()
    defer cb.mu.Unlock()

    if err != nil {
        // 失败
        cb.failureCount++
        cb.lastFailTime = time.Now()

        switch cb.state {
        case StateClosed:
            if cb.failureCount >= cb.threshold {
                cb.setState(StateOpen)
            }
        
        case StateHalfOpen:
            // 半开状态失败，立即熔断
            cb.setState(StateOpen)
        }
    } else {
        // 成功
        cb.successCount++

        switch cb.state {
        case StateHalfOpen:
            // 半开状态成功，恢复正常
            if cb.successCount >= cb.halfOpenMax/2 {
                cb.setState(StateClosed)
                cb.failureCount = 0
                cb.successCount = 0
            }
        
        case StateClosed:
            // 正常状态成功，重置失败计数
            if cb.successCount > 0 && cb.successCount%100 == 0 {
                cb.failureCount = 0
            }
        }
    }
}

// setState 状态转换
func (cb *CircuitBreaker) setState(newState State) {
    if cb.state == newState {
        return
    }

    oldState := cb.state
    cb.state = newState

    if cb.onStateChange != nil {
        cb.onStateChange(oldState, newState)
    }
}

// State 获取当前状态
func (cb *CircuitBreaker) State() State {
    cb.mu.RLock()
    defer cb.mu.RUnlock()
    return cb.state
}

// Metrics 获取指标
func (cb *CircuitBreaker) Metrics() (state State, failures int, successes int) {
    cb.mu.RLock()
    defer cb.mu.RUnlock()
    return cb.state, cb.failureCount, cb.successCount
}
```

---

### 2.4 集成到TCP Server

```go
// internal/tcpserver/server.go

type Server struct {
    // ... 现有字段
    
    // 限流和熔断
    connLimiter  *ConnectionLimiter
    rateLimiter  *RateLimiter
    breaker      *CircuitBreaker
}

func NewServer(cfg *Config) *Server {
    s := &Server{
        // ...
        connLimiter: NewConnectionLimiter(cfg.MaxConnections, 5*time.Second),
        rateLimiter: NewRateLimiter(cfg.RateLimit, cfg.RateBurst),
        breaker:     NewCircuitBreaker(cfg.BreakerThreshold, cfg.BreakerTimeout),
    }
    
    // 熔断器状态变化回调
    s.breaker.onStateChange = func(from, to State) {
        log.Warn("circuit breaker state changed",
            zap.String("from", stateString(from)),
            zap.String("to", stateString(to)),
        )
        
        // 发送告警
        if to == StateOpen {
            alert.Send("Circuit Breaker Opened", "TCP server is experiencing high failure rate")
        }
    }
    
    return s
}

func (s *Server) Start() error {
    for {
        // 1. 速率限流
        if !s.rateLimiter.Allow() {
            time.Sleep(10 * time.Millisecond)
            continue
        }

        // 2. 接受连接
        conn, err := s.ln.Accept()
        if err != nil {
            if isTemporaryError(err) {
                continue
            }
            return err
        }

        // 3. 连接数限流
        if err := s.connLimiter.Acquire(context.Background()); err != nil {
            conn.Close()
            s.metrics.ConnectionsRejected.Inc()
            continue
        }

        // 4. 熔断器检查
        err = s.breaker.Call(func() error {
            // 处理连接
            go s.handleConnWithProtection(conn)
            return nil
        })

        if err == ErrCircuitOpen {
            conn.Close()
            s.connLimiter.Release()
            s.metrics.ConnectionsCircuitBroken.Inc()
            continue
        }
    }
}

// handleConnWithProtection 带保护的连接处理
func (s *Server) handleConnWithProtection(conn net.Conn) {
    defer s.connLimiter.Release()
    defer conn.Close()
    defer func() {
        if r := recover(); r != nil {
            log.Error("panic in handleConn", zap.Any("panic", r))
            s.breaker.record(fmt.Errorf("panic: %v", r))
        }
    }()

    // 调用原有的handleConn
    err := s.handleConn(conn)
    if err != nil {
        s.breaker.record(err)
    } else {
        s.breaker.record(nil)
    }
}
```

---

### 2.5 配置管理

```yaml
# configs/example.yaml

tcp:
  addr: ":9999"
  
  # 限流配置
  max_connections: 10000        # 最大并发连接数
  rate_limit: 100               # 每秒接受连接数
  rate_burst: 200               # 突发容量
  
  # 熔断器配置
  breaker:
    threshold: 50               # 失败次数阈值
    timeout: 30s                # 熔断超时
    half_open_max: 5            # 半开状态测试请求数
```

---

## 3️⃣ 任务3: 数据库查询优化

### 3.1 问题分析

#### 慢查询识别

```sql
-- 查看慢查询（>100ms）
SELECT 
    query,
    calls,
    total_exec_time,
    mean_exec_time,
    max_exec_time
FROM pg_stat_statements
WHERE mean_exec_time > 100
ORDER BY total_exec_time DESC
LIMIT 20;

-- 常见慢查询：
-- 1. SELECT * FROM devices WHERE last_seen_at > NOW() - INTERVAL '5 minutes'
--    (无索引，全表扫描)
-- 2. SELECT * FROM orders WHERE phy_id = 'DEV001' ORDER BY created_at DESC LIMIT 100
--    (缺少复合索引)
-- 3. SELECT * FROM cmd_logs WHERE device_id = 123 AND created_at BETWEEN ... 
--    (索引选择性差)
```

---

### 3.2 优化方案

#### 3.2.1 添加索引

```sql
-- db/migrations/0006_query_optimization_up.sql

-- 1. 设备最近心跳查询优化
CREATE INDEX CONCURRENTLY idx_devices_last_seen 
ON devices(last_seen_at DESC) 
WHERE last_seen_at IS NOT NULL;

-- 2. 订单查询优化（复合索引）
CREATE INDEX CONCURRENTLY idx_orders_phy_created 
ON orders(phy_id, created_at DESC);

-- 3. 命令日志查询优化
CREATE INDEX CONCURRENTLY idx_cmd_logs_device_created 
ON cmd_logs(device_id, created_at DESC);

-- 4. 下行队列状态索引
CREATE INDEX CONCURRENTLY idx_outbound_status_priority 
ON outbound_queue(status, priority DESC, created_at) 
WHERE status IN (0, 1);

-- 5. 端口状态查询优化
CREATE INDEX CONCURRENTLY idx_ports_device_no 
ON ports(device_id, port_no);
```

#### 3.2.2 查询重写

```go
// 优化前：
const slowQuery = `
    SELECT * FROM devices 
    WHERE last_seen_at > NOW() - INTERVAL '5 minutes'
    ORDER BY last_seen_at DESC
`
// 问题：SELECT *，查询所有字段

// 优化后：
const fastQuery = `
    SELECT id, phy_id, protocol, last_seen_at 
    FROM devices 
    WHERE last_seen_at > NOW() - INTERVAL '5 minutes'
    ORDER BY last_seen_at DESC
    LIMIT 1000
`
// 改进：指定字段，添加LIMIT
```

#### 3.2.3 连接池优化

```go
// internal/storage/pg/pool.go

func NewPool(cfg *Config) (*pgxpool.Pool, error) {
    config, err := pgxpool.ParseConfig(cfg.DSN)
    if err != nil {
        return nil, err
    }

    // 优化连接池配置
    config.MaxConns = 20                          // 最大连接数（提升自10）
    config.MinConns = 5                           // 最小连接数（预热）
    config.MaxConnLifetime = 1 * time.Hour        // 连接最大生命周期
    config.MaxConnIdleTime = 30 * time.Minute     // 空闲连接超时
    config.HealthCheckPeriod = 1 * time.Minute    // 健康检查周期
    
    // 连接池统计
    config.ConnConfig.OnNotice = func(conn *pgx.PgConn, n *pgconn.Notice) {
        log.Info("postgres notice", zap.String("message", n.Message))
    }

    return pgxpool.NewWithConfig(context.Background(), config)
}
```

#### 3.2.4 查询结果缓存

```go
// internal/storage/pg/cached_repo.go

type CachedRepository struct {
    *Repository
    cache *ristretto.Cache
}

func NewCachedRepository(repo *Repository) *CachedRepository {
    cache, _ := ristretto.NewCache(&ristretto.Config{
        NumCounters: 1e7,     // 1000万计数器
        MaxCost:     100 << 20, // 100MB
        BufferItems: 64,
    })

    return &CachedRepository{
        Repository: repo,
        cache:      cache,
    }
}

// GetDeviceByPhyID 带缓存的查询
func (r *CachedRepository) GetDeviceByPhyID(ctx context.Context, phyID string) (*Device, error) {
    // 1. 查询缓存
    cacheKey := fmt.Sprintf("device:%s", phyID)
    if val, found := r.cache.Get(cacheKey); found {
        return val.(*Device), nil
    }

    // 2. 查询数据库
    device, err := r.Repository.GetDeviceByPhyID(ctx, phyID)
    if err != nil {
        return nil, err
    }

    // 3. 写入缓存（5分钟TTL）
    r.cache.SetWithTTL(cacheKey, device, 1, 5*time.Minute)

    return device, nil
}
```

---

### 3.3 性能对比

| 查询 | 优化前 | 优化后 | 提升 |
|-----|--------|--------|------|
| 在线设备列表 | 500ms | 50ms | 10x |
| 订单查询（单设备） | 200ms | 20ms | 10x |
| 命令日志查询 | 300ms | 30ms | 10x |
| 端口状态查询 | 100ms | 10ms | 10x |

---

## 4️⃣ 任务4: 健康检查深度增强

### 4.1 当前问题

```go
// internal/health/ready.go
func (r *Readiness) Ready() bool {
    return r.dbReady.Load() && r.tcpReady.Load()
}

// 问题：
// ❌ 只检查启动状态，不检查运行状态
// ❌ 数据库连接断开后仍返回true
// ❌ Redis故障时无法感知
// ❌ Outbound队列积压无法感知
```

---

### 4.2 深度健康检查

#### 4.2.1 健康检查接口

```go
// internal/health/checker.go
package health

import (
    "context"
    "time"
)

// Status 健康状态
type Status string

const (
    StatusHealthy   Status = "healthy"
    StatusDegraded  Status = "degraded"  // 降级
    StatusUnhealthy Status = "unhealthy"
)

// CheckResult 检查结果
type CheckResult struct {
    Status  Status                 `json:"status"`
    Message string                 `json:"message,omitempty"`
    Details map[string]interface{} `json:"details,omitempty"`
    Latency time.Duration          `json:"latency"`
}

// Checker 健康检查器接口
type Checker interface {
    Name() string
    Check(ctx context.Context) CheckResult
}
```

#### 4.2.2 各组件健康检查

```go
// internal/health/checkers/database.go

// DatabaseChecker 数据库健康检查
type DatabaseChecker struct {
    pool *pgxpool.Pool
}

func (c *DatabaseChecker) Name() string {
    return "database"
}

func (c *DatabaseChecker) Check(ctx context.Context) CheckResult {
    start := time.Now()
    
    // 1. Ping测试
    if err := c.pool.Ping(ctx); err != nil {
        return CheckResult{
            Status:  StatusUnhealthy,
            Message: fmt.Sprintf("ping failed: %v", err),
            Latency: time.Since(start),
        }
    }

    // 2. 获取连接池状态
    stats := c.pool.Stat()
    
    // 3. 检查连接池健康度
    utilization := float64(stats.AcquiredConns()) / float64(stats.MaxConns())
    
    status := StatusHealthy
    if utilization > 0.9 {
        status = StatusDegraded
    }

    return CheckResult{
        Status:  status,
        Message: fmt.Sprintf("%.1f%% utilization", utilization*100),
        Details: map[string]interface{}{
            "total_conns":    stats.TotalConns(),
            "idle_conns":     stats.IdleConns(),
            "acquired_conns": stats.AcquiredConns(),
            "max_conns":      stats.MaxConns(),
        },
        Latency: time.Since(start),
    }
}

// internal/health/checkers/redis.go

// RedisChecker Redis健康检查
type RedisChecker struct {
    client *redis.Client
}

func (c *RedisChecker) Check(ctx context.Context) CheckResult {
    start := time.Now()
    
    // Ping测试
    if err := c.client.Ping(ctx).Err(); err != nil {
        return CheckResult{
            Status:  StatusUnhealthy,
            Message: fmt.Sprintf("ping failed: %v", err),
            Latency: time.Since(start),
        }
    }

    // 获取Info
    info, err := c.client.Info(ctx, "stats").Result()
    if err != nil {
        return CheckResult{
            Status:  StatusDegraded,
            Message: fmt.Sprintf("info failed: %v", err),
            Latency: time.Since(start),
        }
    }

    // 解析内存使用率
    // ... 解析info

    return CheckResult{
        Status:  StatusHealthy,
        Latency: time.Since(start),
    }
}

// internal/health/checkers/outbound.go

// OutboundChecker 下行队列健康检查
type OutboundChecker struct {
    queue *outbound.RedisQueue
}

func (c *OutboundChecker) Check(ctx context.Context) CheckResult {
    start := time.Now()
    
    // 1. 获取队列长度
    queueSize, _ := c.queue.GetQueueSize(ctx)
    pendingSize, _ := c.queue.GetPendingSize(ctx)
    deadSize, _ := c.queue.GetDeadSize(ctx)

    // 2. 判断健康状态
    status := StatusHealthy
    message := "ok"

    if queueSize > 10000 {
        status = StatusDegraded
        message = "queue backlog"
    }

    if deadSize > 1000 {
        status = StatusUnhealthy
        message = "too many dead messages"
    }

    return CheckResult{
        Status:  status,
        Message: message,
        Details: map[string]interface{}{
            "queue_size":   queueSize,
            "pending_size": pendingSize,
            "dead_size":    deadSize,
        },
        Latency: time.Since(start),
    }
}

// internal/health/checkers/tcp.go

// TCPChecker TCP服务器健康检查
type TCPChecker struct {
    server *tcpserver.Server
}

func (c *TCPChecker) Check(ctx context.Context) CheckResult {
    start := time.Now()
    
    // 获取连接数
    activeConns := c.server.ActiveConnections()
    maxConns := c.server.MaxConnections()

    // 计算利用率
    utilization := float64(activeConns) / float64(maxConns)

    status := StatusHealthy
    if utilization > 0.9 {
        status = StatusDegraded
    }

    return CheckResult{
        Status:  status,
        Message: fmt.Sprintf("%.1f%% connections", utilization*100),
        Details: map[string]interface{}{
            "active_connections": activeConns,
            "max_connections":    maxConns,
        },
        Latency: time.Since(start),
    }
}
```

#### 4.2.3 健康检查聚合器

```go
// internal/health/aggregator.go

type Aggregator struct {
    checkers []Checker
}

func NewAggregator(checkers ...Checker) *Aggregator {
    return &Aggregator{checkers: checkers}
}

// CheckAll 执行所有健康检查
func (a *Aggregator) CheckAll(ctx context.Context) map[string]CheckResult {
    results := make(map[string]CheckResult)
    
    for _, checker := range a.checkers {
        results[checker.Name()] = checker.Check(ctx)
    }
    
    return results
}

// OverallStatus 总体健康状态
func (a *Aggregator) OverallStatus(ctx context.Context) Status {
    results := a.CheckAll(ctx)
    
    unhealthyCount := 0
    degradedCount := 0
    
    for _, result := range results {
        switch result.Status {
        case StatusUnhealthy:
            unhealthyCount++
        case StatusDegraded:
            degradedCount++
        }
    }
    
    // 任何组件Unhealthy，整体Unhealthy
    if unhealthyCount > 0 {
        return StatusUnhealthy
    }
    
    // 任何组件Degraded，整体Degraded
    if degradedCount > 0 {
        return StatusDegraded
    }
    
    return StatusHealthy
}
```

#### 4.2.4 健康检查HTTP接口

```go
// internal/httpserver/health.go

func RegisterHealthRoutes(r *gin.Engine, aggregator *health.Aggregator) {
    // 1. Readiness探针（K8s使用）
    r.GET("/health/ready", func(c *gin.Context) {
        ctx := c.Request.Context()
        status := aggregator.OverallStatus(ctx)
        
        if status == health.StatusUnhealthy {
            c.JSON(503, gin.H{
                "status": "unhealthy",
                "ready":  false,
            })
            return
        }
        
        c.JSON(200, gin.H{
            "status": status,
            "ready":  true,
        })
    })
    
    // 2. Liveness探针（K8s使用）
    r.GET("/health/live", func(c *gin.Context) {
        // 简单检查进程是否活着
        c.JSON(200, gin.H{"alive": true})
    })
    
    // 3. 详细健康检查
    r.GET("/health", func(c *gin.Context) {
        ctx := c.Request.Context()
        results := aggregator.CheckAll(ctx)
        overall := aggregator.OverallStatus(ctx)
        
        code := 200
        if overall == health.StatusUnhealthy {
            code = 503
        } else if overall == health.StatusDegraded {
            code = 200  // Degraded仍返回200
        }
        
        c.JSON(code, gin.H{
            "status":  overall,
            "checks":  results,
            "timestamp": time.Now(),
        })
    })
}
```

---

### 4.3 健康检查响应示例

```json
// GET /health

{
  "status": "healthy",
  "timestamp": "2025-10-05T12:00:00Z",
  "checks": {
    "database": {
      "status": "healthy",
      "message": "45.2% utilization",
      "details": {
        "total_conns": 10,
        "idle_conns": 5,
        "acquired_conns": 5,
        "max_conns": 20
      },
      "latency": "5ms"
    },
    "redis": {
      "status": "healthy",
      "latency": "2ms"
    },
    "outbound": {
      "status": "degraded",
      "message": "queue backlog",
      "details": {
        "queue_size": 15000,
        "pending_size": 200,
        "dead_size": 50
      },
      "latency": "3ms"
    },
    "tcp": {
      "status": "healthy",
      "message": "65.5% connections",
      "details": {
        "active_connections": 6550,
        "max_connections": 10000
      },
      "latency": "1ms"
    }
  }
}
```

---

## 📊 Week 2 总体规划

### 时间表

```
Week 2.1 (Day 1-5):
├─ Day 1: Redis Outbound设计和开发（核心逻辑）
├─ Day 2: Redis Outbound测试和优化
├─ Day 3: 数据迁移和双写实现
├─ Day 4: 限流器和熔断器开发
└─ Day 5: 数据库查询优化

Week 2.2 (Day 6-10):
├─ Day 6: 健康检查增强
├─ Day 7: 集成测试
├─ Day 8: 性能测试和调优
├─ Day 9: 灰度发布到测试环境
└─ Day 10: 监控和文档完善

Week 2.3 (Day 11-14):
├─ Day 11-12: 生产环境灰度（10% → 50% → 100%）
├─ Day 13: 监控和问题修复
└─ Day 14: 总结和清理
```

---

### 验收标准

#### 功能验收

- [ ] Redis Outbound TPS ≥ 1000/s
- [ ] 连接数限流生效（达到上限时拒绝）
- [ ] 熔断器状态正确转换
- [ ] 数据库查询P99 < 100ms
- [ ] 健康检查覆盖所有组件

#### 性能验收

- [ ] Outbound延迟降低至 < 10ms
- [ ] API响应时间降低50%
- [ ] 支持10000+并发连接
- [ ] Redis内存使用 < 1GB

#### 稳定性验收

- [ ] 压测24小时无崩溃
- [ ] 错误率 < 0.1%
- [ ] 内存无泄露
- [ ] 熔断器告警正常

---

### 风险评估

| 风险 | 概率 | 影响 | 缓解措施 |
|-----|------|------|---------|
| Redis性能不达预期 | 低 | 高 | 双写方案，可回滚PG |
| 限流导致业务阻塞 | 中 | 中 | 配置合理阈值，监控告警 |
| 数据库迁移失败 | 低 | 高 | 在线DDL，分步执行 |
| 熔断器误触发 | 中 | 中 | 调整阈值，增加日志 |

---

### 监控指标

#### 核心指标

```
# Outbound
outbound_queue_size          # 队列长度
outbound_throughput          # 吞吐量（msg/s）
outbound_latency_seconds     # 延迟分布

# 限流
tcp_connections_rejected     # 拒绝连接数
tcp_connections_active       # 活跃连接数
rate_limiter_allowed         # 速率限流通过数

# 熔断器
circuit_breaker_state        # 熔断器状态（0=closed,1=open,2=half-open）
circuit_breaker_failures     # 失败次数
circuit_breaker_trips        # 熔断次数

# 数据库
db_query_latency_seconds     # 查询延迟
db_pool_connections          # 连接池状态
```

---

## 🎯 成功标准

### 技术指标

| 指标 | 当前 | Week 2目标 | 测量方法 |
|-----|------|-----------|---------|
| Outbound TPS | 100 | 1000 | 压测 |
| API P99延迟 | 200ms | 100ms | APM |
| 并发连接 | 1000 | 10000 | 负载测试 |
| 系统可用性 | 99% | 99.5% | 监控统计 |

### 业务指标

| 指标 | 改进前 | 目标 |
|-----|--------|------|
| 下行推送延迟 | 1-5秒 | <1秒 |
| 命令失败率 | 1% | 0.5% |
| 资源使用率 | 高 | 中 |

---

## 📚 参考资料

- [Redis Best Practices](https://redis.io/docs/manual/patterns/)
- [Rate Limiting Strategies](https://konghq.com/blog/how-to-design-a-scalable-rate-limiting-algorithm)
- [Circuit Breaker Pattern](https://martinfowler.com/bliki/CircuitBreaker.html)
- [PostgreSQL Performance Tuning](https://www.postgresql.org/docs/current/performance-tips.html)

---

**文档版本**: v1.0  
**制定日期**: 2025-10-05  
**审核状态**: ⏳ 待审核  
**下一步**: 等待技术评审会议
