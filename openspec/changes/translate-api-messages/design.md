# 技术设计：API返回信息中英翻译

## 1. 翻译映射表

本节提供完整的英文->中文翻译映射表，按文件分组。

### 1.1 通用错误信息

| 英文 | 中文 | 使用场景 |
|------|------|----------|
| `invalid request` | `无效的请求` | 请求参数验证失败 |
| `invalid request body` | `请求体格式错误` | JSON解析失败 |
| `failed to get device` | `获取设备失败` | 数据库查询设备失败 |
| `device not found` | `设备不存在` | 设备ID不存在 |
| `device is offline, cannot create order` | `设备离线，无法创建订单` | 设备不在线 |
| `database error` | `数据库错误` | 通用数据库错误 |
| `failed to create order` | `创建订单失败` | 订单创建失败 |
| `failed to update port status` | `更新端口状态失败` | 端口状态更新失败 |
| `port is busy` | `端口正在使用中` | 端口被占用 |
| `port state mismatch, port may be in use` | `端口状态不一致，端口可能正在使用中` | 端口状态冲突 |
| `port state inconsistent, please retry` | `端口状态不一致，请重试` | 端口状态数据不一致 |
| `port is in fault state` | `端口故障` | 端口处于故障状态 |
| `no active charging session found` | `未找到活动的充电会话` | 无充电中订单 |
| `failed to stop order` | `停止订单失败` | 订单停止失败 |
| `order status has changed, cannot stop` | `订单状态已变更，无法停止` | 订单状态冲突 |
| `stop command sent, order will be stopped in 30 seconds` | `停止指令已发送，订单将在30秒内停止` | 停止指令已下发 |
| `order not found` | `订单不存在` | 订单查询失败 |
| `charging状态订单无法直接取消,请先调用停止充电接口` | （已是中文）| 充电中订单无法取消 |
| `cancel command sent, order will be cancelled in 30 seconds` | `取消指令已发送，订单将在30秒内取消` | 取消指令已下发 |
| `charge command sent successfully` | `充电指令发送成功` | 充电指令已下发 |
| `success` | `成功` | 通用成功消息 |
| `failed to query orders` | `查询订单失败` | 订单列表查询失败 |
| `param command sent successfully` | `参数指令发送成功` | 参数设置成功 |
| `failed to send param command` | `参数指令发送失败` | 参数下发失败 |
| `ota command sent successfully` | `OTA指令发送成功` | OTA指令已下发 |
| `failed to send ota command` | `OTA指令发送失败` | OTA下发失败 |
| `failed to get order events` | `获取订单事件失败` | 事件查询失败 |

### 1.2 internal/api/thirdparty_handler.go

#### 1.2.1 StartCharge接口

```go
// Line 109: 请求体验证失败
Message: fmt.Sprintf("invalid request: %v", err)
→ Message: fmt.Sprintf("无效的请求: %v", err)

// Line 130: 获取设备失败
Message: "failed to get device"
→ Message: "获取设备失败"

// Line 145: 设备离线
Message: "device is offline, cannot create order"
→ Message: "设备离线，无法创建订单"

// Line 190: 端口状态不匹配
Message: "port state mismatch, port may be in use"
→ Message: "端口状态不一致，端口可能正在使用中"

// Line 213: 端口状态不一致
Message: "port state inconsistent, please retry"
→ Message: "端口状态不一致，请重试"

// Line 239: 端口故障
Message: "port is in fault state"
→ Message: "端口故障"

// Line 268: 创建订单失败
Message: "failed to create order"
→ Message: "创建订单失败"

// Line 282: 更新端口状态失败
Message: "failed to update port status"
→ Message: "更新端口状态失败"

// Line 296: 数据库错误
Message: "database error"
→ Message: "数据库错误"

// Line 379: 充电指令发送成功
Message: "charge command sent successfully"
→ Message: "充电指令发送成功"
```

#### 1.2.2 StopCharge接口

```go
// Line 404: 无效的请求
Message: fmt.Sprintf("invalid request: %v", err)
→ Message: fmt.Sprintf("无效的请求: %v", err)

// Line 417: 获取设备失败
Message: "failed to get device"
→ Message: "获取设备失败"

// Line 435: 无活动充电会话
Message: "no active charging session found"
→ Message: "未找到活动的充电会话"

// Line 449: 停止订单失败
Message: "failed to stop order"
→ Message: "停止订单失败"

// Line 458: 订单状态已变更
Message: "order status has changed, cannot stop"
→ Message: "订单状态已变更，无法停止"

// Line 490: 停止指令已发送
Message: "stop command sent, order will be stopped in 30 seconds"
→ Message: "停止指令已发送，订单将在30秒内停止"
```

#### 1.2.3 CancelOrder接口

```go
// Line 522: 订单不存在
Message: "order not found"
→ Message: "订单不存在"

// Line 571: 取消订单失败
Message: "failed to cancel order"
→ Message: "取消订单失败"

// Line 583: 取消指令已发送
Message: "cancel command sent, order will be cancelled in 30 seconds"
→ Message: "取消指令已发送，订单将在30秒内取消"
```

#### 1.2.4 GetDevice接口

```go
// Line 609: 获取设备失败
Message: "failed to get device"
→ Message: "获取设备失败"

// Line 625: 设备不存在
Message: "device not found"
→ Message: "设备不存在"
```

#### 1.2.5 GetOrder接口

```go
// Line 682: 订单不存在
Message: "order not found"
→ Message: "订单不存在"
```

#### 1.2.6 ListOrders接口

```go
// Line 753: 查询订单失败
Message: "failed to query orders"
→ Message: "查询订单失败"
```

#### 1.2.7 SetParams接口

```go
// Line 821: 无效的请求
Message: fmt.Sprintf("invalid request: %v", err)
→ Message: fmt.Sprintf("无效的请求: %v", err)

// Line 833: 获取设备失败
Message: "failed to get device"
→ Message: "获取设备失败"

// Line 870: 参数指令发送失败
Message: "failed to send param command"
→ Message: "参数指令发送失败"

// Line 879: 参数指令发送成功
Message: "param command sent successfully"
→ Message: "参数指令发送成功"
```

#### 1.2.8 TriggerOTA接口

```go
// Line 905: 无效的请求
Message: fmt.Sprintf("invalid request: %v", err)
→ Message: fmt.Sprintf("无效的请求: %v", err)

// Line 919: 获取设备失败
Message: "failed to get device"
→ Message: "获取设备失败"

// Line 953: OTA指令发送失败
Message: "failed to send ota command"
→ Message: "OTA指令发送失败"

// Line 962: OTA指令发送成功
Message: "ota command sent successfully"
→ Message: "OTA指令发送成功"
```

#### 1.2.9 GetOrderEvents接口

```go
// Line 1492: 获取订单事件失败
Message: "failed to get order events"
→ Message: "获取订单事件失败"
```

### 1.3 internal/api/readonly_handler.go

```go
// Line 63: 错误信息
c.JSON(500, gin.H{"error": err.Error()})
→ c.JSON(500, gin.H{"error": "查询失败", "details": err.Error()})

// Line 82: 错误信息
c.JSON(500, gin.H{"error": err.Error()})
→ c.JSON(500, gin.H{"error": "查询失败", "details": err.Error()})

// Line 103: 设备不存在
c.JSON(http.StatusNotFound, gin.H{"error": "device not found"})
→ c.JSON(http.StatusNotFound, gin.H{"error": "设备不存在"})

// Line 110: 查询失败
c.JSON(http.StatusInternalServerError, gin.H{"error": "query failed"})
→ c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})

// Line 186: 无效ID
c.JSON(400, gin.H{"error": "invalid id"})
→ c.JSON(400, gin.H{"error": "无效的ID"})

// Line 191: 未找到
c.JSON(404, gin.H{"error": "not found"})
→ c.JSON(404, gin.H{"error": "未找到"})

// Line 224: 错误信息
c.JSON(500, gin.H{"error": err.Error()})
→ c.JSON(500, gin.H{"error": "查询失败", "details": err.Error()})
```

### 1.4 internal/api/ota_handler.go

```go
// Line 47: 无效设备ID
c.JSON(http.StatusBadRequest, gin.H{"error": "invalid device_id"})
→ c.JSON(http.StatusBadRequest, gin.H{"error": "无效的设备ID"})

// Line 53: 无效请求
c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
→ c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求"})

// Line 58: 缺少参数
c.JSON(http.StatusBadRequest, gin.H{"error": "target_socket_no required"})
→ c.JSON(http.StatusBadRequest, gin.H{"error": "缺少target_socket_no参数"})

// Line 76: 创建失败
c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create"})
→ c.JSON(http.StatusInternalServerError, gin.H{"error": "创建失败"})

// Line 80: 成功
c.JSON(http.StatusCreated, gin.H{"message": "success", "task_id": taskID})
→ c.JSON(http.StatusCreated, gin.H{"message": "成功", "task_id": taskID})

// Line 93: 无效任务ID
c.JSON(http.StatusBadRequest, gin.H{"error": "invalid task_id"})
→ c.JSON(http.StatusBadRequest, gin.H{"error": "无效的任务ID"})

// Line 99: 未找到
c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
→ c.JSON(http.StatusNotFound, gin.H{"error": "未找到"})

// Line 117: 无效设备ID
c.JSON(http.StatusBadRequest, gin.H{"error": "invalid device_id"})
→ c.JSON(http.StatusBadRequest, gin.H{"error": "无效的设备ID"})

// Line 128: 查询失败
c.JSON(http.StatusInternalServerError, gin.H{"error": "query failed"})
→ c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
```

### 1.5 internal/api/middleware/auth.go

```go
// Line 54-55: 未授权
"error":   "unauthorized",
"message": "请在Header中提供 X-API-Key 或 Authorization: Bearer <token>",
→ （message已是中文，error改为中文）
"error":   "未授权",
"message": "请在Header中提供 X-API-Key 或 Authorization: Bearer <token>",

// Line 77-78: 禁止访问
"error":   "forbidden",
"message": "无效的API Key",
→ （message已是中文，error改为中文）
"error":   "禁止访问",
"message": "无效的API Key",

// Line 149-150: 未授权
"error":   "unauthorized",
"message": "内部接口需要认证：请在Header中提供 X-Internal-API-Key",
→ （message已是中文，error改为中文）
"error":   "未授权",
"message": "内部接口需要认证：请在Header中提供 X-Internal-API-Key",

// Line 172-173: 禁止访问
"error":   "forbidden",
"message": "无效的内部API Key",
→ （message已是中文，error改为中文）
"error":   "禁止访问",
"message": "无效的内部API Key",
```

### 1.6 internal/api/middleware/thirdparty_auth.go

```go
// Line 36: 缺少API Key
"message": "missing api key",
→ "message": "缺少API Key",

// Line 52: 无效API Key
"message": "invalid api key",
→ "message": "无效的API Key",
```

### 1.7 internal/storage/redis/outbound_queue.go

```go
// Line 63: 队列过载
return fmt.Errorf("P1-6: queue overloaded (len=%d), rejecting low priority command (priority=%d)", ...)
→ return fmt.Errorf("P1-6: 队列过载 (len=%d)，拒绝低优先级命令 (priority=%d)", ...)

// Line 68: 队列临界
return fmt.Errorf("P1-6: queue critical (len=%d), rejecting non-urgent command (priority=%d)", ...)
→ return fmt.Errorf("P1-6: 队列临界 (len=%d)，拒绝非紧急命令 (priority=%d)", ...)

// Line 73: 队列紧急
return fmt.Errorf("P1-6: queue emergency (len=%d), only accepting urgent commands (priority<=1)", ...)
→ return fmt.Errorf("P1-6: 队列紧急 (len=%d)，仅接受紧急命令 (priority<=1)", ...)

// Line 162: 消息ID不能为零
return fmt.Errorf("msgID cannot be zero")
→ return fmt.Errorf("消息ID不能为零")

// Line 170: 未找到处理中的消息
return fmt.Errorf("no processing message found for phy_id=%s msg_id=%d", phyID, msgID)
→ return fmt.Errorf("未找到处理中的消息 phy_id=%s msg_id=%d", phyID, msgID)

// Line 181: 消息未找到
return fmt.Errorf("message %s not found in processing", messageID)
→ return fmt.Errorf("消息 %s 未在处理队列中找到", messageID)

// Line 369: 无效的消息格式
return nil, fmt.Errorf("invalid message format")
→ return nil, fmt.Errorf("无效的消息格式")
```

## 2. 实施细节

### 2.1 替换方法

使用 `multi_replace_string_in_file` 工具批量替换，确保：
1. 保留上下文代码（前后3-5行）
2. 精确匹配字符串（包括引号）
3. 保持代码缩进和格式

### 2.2 注释标注

在翻译后的代码附近添加注释，标注原英文信息：

```go
// EN: "invalid request"
Message: "无效的请求"
```

### 2.3 测试验证

每个文件修改后需要：
1. 运行对应的单元测试
2. 检查编译错误
3. 验证API返回格式正确

## 3. 风险控制

### 3.1 代码审查要点

- [ ] 确保翻译准确无误
- [ ] 确保Code字段未被修改
- [ ] 确保JSON结构完整
- [ ] 确保日志消息未被翻译

### 3.2 测试覆盖

- [ ] API接口测试：验证各端点返回中文消息
- [ ] 错误场景测试：触发各类错误，验证消息正确
- [ ] 集成测试：验证整体功能正常

## 4. 翻译统计

- **API Handler层**: 约60处翻译
- **中间件层**: 约8处翻译
- **存储层**: 约8处翻译
- **总计**: 约76处翻译

## 5. 后续优化建议

1. **错误码标准化**：建立统一的错误码常量定义
2. **多语言支持**：为未来国际化预留接口
3. **错误消息模板**：使用模板化错误消息，便于维护
