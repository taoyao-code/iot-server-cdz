# API Module - HTTP API层

> **导航**: [← 返回根目录](../../CLAUDE.md)
> **路径**: `internal/api/`
> **最后更新**: 2025-11-28

---

## 📋 模块职责

API 模块提供 HTTP RESTful 接口，包括：

- **只读 API**: 设备查询、端口状态查询
- **第三方 API**: 命令下发、Webhook 管理
- **认证鉴权**: API Key + HMAC 签名
- **API 文档**: Swagger/OpenAPI 自动生成

---

## 🏗️ 模块结构

```
api/
├── middleware/              # 中间件
│   ├── auth.go             # API Key 认证
│   ├── signature.go        # HMAC 签名验证
│   ├── rate_limit.go       # 限流
│   └── logging.go          # 请求日志
├── readonly_routes.go      # 只读路由定义
├── readonly_handler.go     # 只读处理器
├── thirdparty_routes.go    # 第三方路由定义
└── thirdparty_handler.go   # 第三方处理器
```

---

## 🌐 API 路由

### 只读 API (readonly_routes.go)

**端点**: `/api/v1/readonly/*`
**认证**: 不需要（或基础 API Key）

```go
// GET /api/v1/readonly/devices - 查询设备列表
// GET /api/v1/readonly/devices/:id - 查询设备详情
// GET /api/v1/readonly/ports - 查询端口状态
// GET /api/v1/readonly/ports/:id - 查询端口详情
```

### 第三方 API (thirdparty_routes.go)

**端点**: `/api/v1/thirdparty/*`
**认证**: API Key + HMAC 签名

```go
// POST /api/v1/thirdparty/command - 下发控制命令
// GET  /api/v1/thirdparty/command/:id - 查询命令状态
// POST /api/v1/thirdparty/webhook/register - 注册 Webhook
// DELETE /api/v1/thirdparty/webhook/:id - 删除 Webhook
```

### 健康检查

```go
// GET /healthz - 存活检查（Liveness）
// GET /readyz - 就绪检查（Readiness）
// GET /metrics - Prometheus 指标
```

---

## 🔑 认证机制

### 1. API Key 认证 (middleware/auth.go)

**请求头**: `X-Api-Key: <your-api-key>`

```go
func ApiKeyMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        apiKey := c.GetHeader("X-Api-Key")
        if !validateApiKey(apiKey) {
            c.AbortWithStatusJSON(401, gin.H{"error": "invalid api key"})
            return
        }
        c.Next()
    }
}
```

### 2. HMAC 签名验证 (middleware/signature.go)

**请求头**: `X-Signature: <hmac-sha256-signature>`

**签名计算**:
```
Signature = HMAC-SHA256(SecretKey, Timestamp + Method + Path + Body)
```

**示例**:
```go
timestamp := "1701234567"
method := "POST"
path := "/api/v1/thirdparty/command"
body := `{"device_id":"dev123","command":"start"}`

message := timestamp + method + path + body
signature := hmac.Sum256([]byte(message), secretKey)

// Header: X-Signature: <hex(signature)>
// Header: X-Timestamp: 1701234567
```

### 3. 限流 (middleware/rate_limit.go)

使用 Redis 实现令牌桶算法：

```go
// 每分钟 60 次请求
rateLimiter := NewRateLimiter(redis, 60, time.Minute)
```

---

## 📦 请求/响应格式

### 通用响应格式

```json
{
  "code": 0,
  "message": "success",
  "data": { ... },
  "timestamp": 1701234567
}
```

**错误响应**:
```json
{
  "code": 400,
  "message": "invalid parameters",
  "error": "device_id is required",
  "timestamp": 1701234567
}
```

---

## 🔧 核心处理器

### Readonly Handler (readonly_handler.go)

```go
type ReadonlyHandler struct {
    repo   storage.CoreRepo
    logger *zap.Logger
}

// GET /api/v1/readonly/devices/:id
func (h *ReadonlyHandler) GetDevice(c *gin.Context) {
    deviceID := c.Param("id")

    device, err := h.repo.GetDevice(c.Request.Context(), deviceID)
    if err != nil {
        c.JSON(404, gin.H{"error": "device not found"})
        return
    }

    c.JSON(200, gin.H{"data": device})
}
```

**主要方法**:
- `GetDevice()` - 获取设备详情
- `ListDevices()` - 设备列表（分页）
- `GetPort()` - 获取端口状态
- `ListPorts()` - 端口列表（分页、过滤）

### Thirdparty Handler (thirdparty_handler.go)

```go
type ThirdpartyHandler struct {
    queue  outbound.Queue
    repo   storage.CoreRepo
    logger *zap.Logger
}

// POST /api/v1/thirdparty/command
func (h *ThirdpartyHandler) SendCommand(c *gin.Context) {
    var req CommandRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(400, gin.H{"error": "invalid request"})
        return
    }

    // 验证设备存在
    device, err := h.repo.GetDevice(c.Request.Context(), req.DeviceID)
    if err != nil {
        c.JSON(404, gin.H{"error": "device not found"})
        return
    }

    // 入队命令
    cmd := &coremodel.CoreCommand{
        DeviceID: req.DeviceID,
        Type:     req.CommandType,
        Payload:  req.Payload,
    }

    if err := h.queue.Enqueue(c.Request.Context(), cmd); err != nil {
        c.JSON(500, gin.H{"error": "failed to enqueue command"})
        return
    }

    c.JSON(200, gin.H{"data": gin.H{"command_id": cmd.ID}})
}
```

**主要方法**:
- `SendCommand()` - 下发控制命令
- `GetCommandStatus()` - 查询命令状态
- `RegisterWebhook()` - 注册 Webhook URL
- `DeleteWebhook()` - 删除 Webhook

---

## 🧪 API 测试

### 手动测试

**只读 API**:
```bash
# 查询设备列表
curl http://localhost:7055/api/v1/readonly/devices

# 查询特定设备
curl http://localhost:7055/api/v1/readonly/devices/dev123

# 查询端口状态
curl http://localhost:7055/api/v1/readonly/ports?device_id=dev123
```

**第三方 API（需要签名）**:
```bash
# 计算签名
timestamp=$(date +%s)
message="${timestamp}POST/api/v1/thirdparty/command{\"device_id\":\"dev123\",\"command\":\"start\"}"
signature=$(echo -n "$message" | openssl dgst -sha256 -hmac "your-secret-key" | awk '{print $2}')

# 发送请求
curl -X POST http://localhost:7055/api/v1/thirdparty/command \
  -H "X-Api-Key: your-api-key" \
  -H "X-Signature: $signature" \
  -H "X-Timestamp: $timestamp" \
  -H "Content-Type: application/json" \
  -d '{"device_id":"dev123","command":"start"}'
```

### 自动化测试

```bash
# 使用 Postman/Insomnia 导入 Swagger 文档
# Swagger JSON: http://localhost:7055/swagger/doc.json

# 使用测试脚本
go test ./internal/api/... -v
```

---

## 📊 Swagger 文档

### 访问 Swagger UI

启动服务后访问：
```
http://localhost:7055/swagger/index.html
```

### 生成 Swagger 文档

```bash
# 安装 swag
go install github.com/swaggo/swag/cmd/swag@latest

# 生成文档
make swagger

# 输出: docs/swagger.json, docs/swagger.yaml
```

### Swagger 注解示例

```go
// GetDevice godoc
// @Summary 获取设备详情
// @Description 根据设备ID获取设备详细信息
// @Tags devices
// @Accept json
// @Produce json
// @Param id path string true "设备ID"
// @Success 200 {object} DeviceResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/v1/readonly/devices/{id} [get]
func (h *ReadonlyHandler) GetDevice(c *gin.Context) { ... }
```

---

## 🚨 错误处理

### 标准错误码

| HTTP状态 | 错误码 | 说明 |
|---------|--------|------|
| 200 | 0 | 成功 |
| 400 | 1001 | 参数错误 |
| 401 | 1002 | 认证失败 |
| 403 | 1003 | 无权限 |
| 404 | 1004 | 资源不存在 |
| 429 | 1005 | 请求过于频繁 |
| 500 | 2001 | 服务器内部错误 |
| 503 | 2002 | 服务不可用 |

### 错误处理示例

```go
// 统一错误响应
type ErrorResponse struct {
    Code      int    `json:"code"`
    Message   string `json:"message"`
    Error     string `json:"error,omitempty"`
    Timestamp int64  `json:"timestamp"`
}

// 错误处理中间件
func ErrorHandler() gin.HandlerFunc {
    return func(c *gin.Context) {
        c.Next()

        if len(c.Errors) > 0 {
            err := c.Errors.Last()
            c.JSON(500, ErrorResponse{
                Code:      2001,
                Message:   "internal server error",
                Error:     err.Error(),
                Timestamp: time.Now().Unix(),
            })
        }
    }
}
```

---

## 🔍 监控与日志

### 请求日志 (middleware/logging.go)

```go
func LoggingMiddleware(logger *zap.Logger) gin.HandlerFunc {
    return func(c *gin.Context) {
        start := time.Now()

        c.Next()

        logger.Info("http request",
            zap.String("method", c.Request.Method),
            zap.String("path", c.Request.URL.Path),
            zap.Int("status", c.Writer.Status()),
            zap.Duration("latency", time.Since(start)),
            zap.String("client_ip", c.ClientIP()),
        )
    }
}
```

### Metrics

```go
var (
    httpRequestsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "http_requests_total",
            Help: "Total HTTP requests",
        },
        []string{"method", "path", "status"},
    )

    httpRequestDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name: "http_request_duration_seconds",
            Help: "HTTP request duration",
        },
        []string{"method", "path"},
    )
)
```

---

## 🔗 相关文档

- [App Module](../app/CLAUDE.md) - 应用引导
- [Storage Module](../storage/CLAUDE.md) - 存储层
- [Outbound Module](../outbound/CLAUDE.md) - 出站队列
- [事件推送规范](../../docs/api/事件推送规范.md) - Webhook 文档

---

**最后更新**: 2025-11-28
**维护者**: API Team
