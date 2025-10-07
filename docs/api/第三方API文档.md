# 第三方API文档

> **版本**: v1.0.0  
> **更新日期**: 2025-01-03  
> **Base URL**: `/api/v1/third`

---

## 认证

所有第三方API请求需要在Header中提供API Key：

```http
X-Api-Key: your-api-key-here
```

可选的HMAC签名验证（推荐）：

```http
X-Signature: hmac-sha256-signature
X-Timestamp: unix-timestamp
X-Nonce: random-nonce
```

---

## 设备控制API

### 1. 启动充电

```http
POST /api/v1/third/devices/:id/charge
```

**请求参数**:

```json
{
  "port_no": 1,
  "mode": "time",
  "duration": 3600,
  "amount": 10.0
}
```

**响应**:

```json
{
  "code": 0,
  "message": "charge started successfully",
  "data": {
    "order_id": "ORDER123456",
    "device_id": "device-001"
  },
  "request_id": "req-123",
  "timestamp": 1704067200
}
```

### 2. 停止充电

```http
POST /api/v1/third/devices/:id/stop
```

### 3. 查询设备状态

```http
GET /api/v1/third/devices/:id
```

---

## 订单查询API

### 4. 查询订单详情

```http
GET /api/v1/third/orders/:id
```

### 5. 订单列表

```http
GET /api/v1/third/orders?page=1&size=20&status=completed
```

---

## 参数和OTA API

### 6. 设置参数

```http
POST /api/v1/third/devices/:id/params
```

### 7. 触发OTA升级

```http
POST /api/v1/third/devices/:id/ota
```

---

## 响应格式

### 标准响应

所有API返回统一格式：

```json
{
  "code": 0,              // 0=成功, >0=错误码
  "message": "success",   // 消息
  "data": {},             // 业务数据
  "request_id": "uuid",   // 请求追踪ID
  "timestamp": 1234567890 // Unix时间戳
}
```

### 错误码

| Code | 说明 |
|------|------|
| 0 | 成功 |
| 400 | 请求参数错误 |
| 401 | 认证失败 |
| 404 | 资源不存在 |
| 429 | 请求过于频繁 |
| 500 | 服务器内部错误 |

---

## 事件推送

### Webhook配置

服务器会主动推送事件到配置的Webhook URL。

**推送格式**:

```json
{
  "event_id": "unique-event-id",
  "event_type": "order.completed",
  "device_phy_id": "device-001",
  "timestamp": 1704067200,
  "nonce": "random-nonce",
  "data": {
    // 事件具体数据
  }
}
```

**签名验证**:

```
canonical = method\npath\ntimestamp\nnonce\nbody_sha256
signature = HMAC-SHA256(secret, canonical)
```

### 事件类型

| 事件类型 | 说明 |
|---------|------|
| device.registered | 设备注册 |
| device.heartbeat | 设备心跳 |
| order.created | 订单创建 |
| order.confirmed | 订单确认 |
| order.completed | 订单完成 |
| charging.started | 充电开始 |
| charging.ended | 充电结束 |
| device.alarm | 设备告警 |
| socket.state_changed | 插座状态变更 |
| ota.progress_update | OTA进度更新 |

---

## 限流说明

- 每个API Key限制：100请求/分钟
- 全局限制：1000请求/分钟
- 超出限制返回429错误

---

## 示例代码

### Python

```python
import requests
import hmac
import hashlib
import time

api_key = "your-api-key"
secret = "your-secret"
base_url = "http://localhost:8080/api/v1/third"

headers = {
    "X-Api-Key": api_key,
    "Content-Type": "application/json"
}

# 启动充电
response = requests.post(
    f"{base_url}/devices/device-001/charge",
    json={"port_no": 1, "mode": "time", "duration": 3600},
    headers=headers
)

print(response.json())
```

### Go

```go
package main

import (
    "bytes"
    "encoding/json"
    "net/http"
)

func main() {
    apiKey := "your-api-key"
    url := "http://localhost:8080/api/v1/third/devices/device-001/charge"
    
    payload := map[string]interface{}{
        "port_no": 1,
        "mode": "time",
        "duration": 3600,
    }
    
    body, _ := json.Marshal(payload)
    req, _ := http.NewRequest("POST", url, bytes.NewBuffer(body))
    req.Header.Set("X-Api-Key", apiKey)
    req.Header.Set("Content-Type", "application/json")
    
    client := &http.Client{}
    resp, _ := client.Do(req)
    defer resp.Body.Close()
}
```

---

## 联系支持

如有问题，请联系技术支持团队。
