# Outbound Module - 出站队列

> **导航**: [← 返回根目录](../../CLAUDE.md)
> **路径**: `internal/outbound/`
> **最后更新**: 2025-11-28

---

## 📋 模块职责

Outbound 模块负责命令下发的队列管理和调度：

- **优先级队列**: 按优先级排序的命令队列
- **Redis 实现**: 基于 Redis Sorted Set
- **Worker 机制**: 多 worker 并发处理
- **重试策略**: 失败自动重试，指数退避
- **状态追踪**: pending/sent/done/failed

---

## 📂 文件结构

```
outbound/
├── priority.go       # 优先级定义
└── redis_worker.go   # Redis 队列 Worker
```

---

## 🔑 核心组件

### 优先级定义

```go
const (
    PriorityHigh   = 100  // 高优先级（紧急命令）
    PriorityNormal = 50   // 普通优先级
    PriorityLow    = 10   // 低优先级（批量任务）
)
```

### Worker 机制

```go
type RedisWorker struct {
    redis      *redis.Client
    handlers   map[string]CommandHandler
    workerCount int
}

func (w *RedisWorker) Start(ctx context.Context) {
    for i := 0; i < w.workerCount; i++ {
        go w.worker(ctx, i)
    }
}
```

---

## 🔄 队列操作

### 入队

```go
// 使用 ZADD 存储，score 为优先级
func Enqueue(deviceID string, cmd *Command) error {
    score := float64(cmd.Priority)
    return redis.ZAdd(ctx, queueKey, &redis.Z{
        Score:  score,
        Member: cmd.ID,
    }).Err()
}
```

### 出队

```go
// 使用 ZPOPMAX 按优先级弹出
func Dequeue() (*Command, error) {
    result, err := redis.ZPopMax(ctx, queueKey, 1).Result()
    if err != nil {
        return nil, err
    }
    return parseCommand(result[0].Member)
}
```

---

## 🔗 相关文档

- [Storage Module](../storage/CLAUDE.md)
- [DriverAPI Module](../driverapi/CLAUDE.md)

---

**最后更新**: 2025-11-28
