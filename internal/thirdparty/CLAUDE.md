# Thirdparty Module - 第三方集成

> **导航**: [← 返回根目录](../../CLAUDE.md)
> **路径**: `internal/thirdparty/`
> **最后更新**: 2025-11-28

---

## 📋 模块职责

Thirdparty 模块负责与第三方系统的集成和事件推送：

- **Webhook 推送**: HTTP POST 事件到第三方
- **事件队列**: Redis 队列缓冲
- **去重机制**: 防止重复推送
- **签名验证**: HMAC-SHA256 签名
- **熔断保护**: 失败熔断机制
- **指标监控**: Prometheus 指标

---

## 📂 文件结构

```
thirdparty/
├── pusher.go          # Webhook 推送器
├── pusher_test.go     # 推送器测试
├── event_queue.go     # 事件队列
├── events.go          # 事件定义
├── events_test.go     # 事件测试
├── deduper.go         # 去重器
├── signer.go          # HMAC 签名
├── signer_test.go     # 签名测试
└── metrics.go         # 指标定义
```

---

## 🔑 核心组件

### EventPusher

```go
type EventPusher struct {
    queue      *EventQueue
    httpClient *http.Client
    webhookURL string
    signer     *Signer
    deduper    *Deduper
    metrics    *Metrics
}

func (ep *EventPusher) Start(ctx context.Context) {
    for i := 0; i < workerCount; i++ {
        go ep.worker(ctx)
    }
}
```

### Webhook 签名

```go
// HMAC-SHA256 签名
func Sign(secretKey, payload []byte) string {
    h := hmac.New(sha256.New, secretKey)
    h.Write(payload)
    return hex.EncodeToString(h.Sum(nil))
}
```

---

## 📊 事件类型

```go
const (
    EventTypeDeviceOnline  = "device.online"
    EventTypeDeviceOffline = "device.offline"
    EventTypePortStatus    = "port.status"
    EventTypeSessionStart  = "session.start"
    EventTypeSessionEnd    = "session.end"
)
```

---

## 🔒 去重机制

```go
type Deduper struct {
    redis *redis.Client
    ttl   time.Duration
}

func (d *Deduper) IsDuplicate(eventID string) bool {
    key := fmt.Sprintf("dedup:%s", eventID)
    return d.redis.SetNX(ctx, key, 1, d.ttl).Val() == false
}
```

---

## 🔗 相关文档

- [App Module](../app/CLAUDE.md)
- [API Module](../api/CLAUDE.md)
- [事件推送规范](../../docs/api/事件推送规范.md)

---

**最后更新**: 2025-11-28
