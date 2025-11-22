# API Response Specification Delta

本文件是针对API响应规范的增量变更文档。

## MODIFIED Requirements

### Requirement: API responses MUST use Chinese language (API响应消息必须统一为中文)

**Requirement ID**: `api-response-language`

**描述**: 所有面向用户的API响应消息（包括成功消息和错误消息）**MUST**使用中文，以提供一致的用户体验。
Description: All end-user facing API response messages (success and error) MUST be rendered in Chinese.

**变更类型**: MODIFIED

**影响范围**:
- `internal/api/*_handler.go` - 所有API处理器
- `internal/api/middleware/*.go` - 认证中间件
- `internal/storage/redis/outbound_queue.go` - 存储层错误消息

#### Scenario: API返回成功消息

**Given**: 用户调用启动充电接口
**When**: 充电指令成功下发到设备
**Then**: API返回的StandardResponse.Message字段应为中文，如"充电指令发送成功"，而非英文"charge command sent successfully"

**验证标准**:
```json
{
  "code": 0,
  "message": "充电指令发送成功",
  "data": { ... },
  "request_id": "...",
  "timestamp": 1234567890
}
```

---

#### Scenario: API返回错误消息

**Given**: 用户调用启动充电接口，但设备离线
**When**: 系统检测到设备不在线
**Then**: API返回的StandardResponse.Message字段应为中文，如"设备离线，无法创建订单"，而非英文"device is offline, cannot create order"

**验证标准**:
```json
{
  "code": 503,
  "message": "设备离线，无法创建订单",
  "data": {
    "device_id": "82241218000382",
    "status": "offline"
  },
  "request_id": "...",
  "timestamp": 1234567890
}
```

---

#### Scenario: 认证中间件返回错误

**Given**: 用户调用API但未提供API Key
**When**: 认证中间件拦截请求
**Then**: 返回的错误消息应为中文，如"缺少API Key"，而非英文"missing api key"

**验证标准**:
```json
{
  "error": "未授权",
  "message": "请在Header中提供 X-API-Key 或 Authorization: Bearer <token>"
}
```

---

#### Scenario: 存储层错误传播到API

**Given**: Redis队列因过载拒绝新命令
**When**: API层捕获并返回错误
**Then**: 错误消息应翻译为中文，如"队列过载，拒绝低优先级命令"

**验证标准**:
- 错误码（Code字段）保持不变
- 错误消息（Message字段）使用中文
- 技术标识（如P1-6）保留

---

### Requirement: Error codes MUST remain unchanged (错误码必须保持不变)

**Requirement ID**: `api-response-error-code-stability`

**描述**: 在翻译消息的过程中，StandardResponse.Code字段**MUST**保持不变，以确保程序化错误处理不受影响。
Description: The StandardResponse.Code field MUST remain unchanged; only the human-readable Message changes.

**变更类型**: MODIFIED

**影响范围**:
- 所有API响应的Code字段

#### Scenario: 错误码不变，消息翻译

**Given**: API返回"设备不存在"错误
**When**: 修改前后对比
**Then**: 
- 修改前: `{"code": 404, "message": "device not found"}`
- 修改后: `{"code": 404, "message": "设备不存在"}`
- Code字段404保持不变

**验证标准**:
- 所有已有的错误码保持不变
- API调用方可继续使用Code字段进行错误判断

---

### Requirement: Log messages SHALL remain in English (日志消息保持英文)

**Requirement ID**: `api-log-message-english`

**描述**: Logger输出的日志消息**SHALL**保持英文不变，便于开发人员调试和国际化团队协作。
Description: All logger output messages SHALL remain in English for consistency and debugging.

**变更类型**: NOT MODIFIED (明确说明不变更)

**影响范围**:
- 所有zap.Logger相关的日志输出

#### Scenario: 日志消息不翻译

**Given**: 代码中存在日志语句
**When**: 进行消息翻译工作
**Then**: 日志语句保持英文不变

**验证标准**:
```go
// 正确：日志保持英文
h.logger.Error("failed to get device", zap.Error(err))

// 正确：API响应使用中文
c.JSON(500, StandardResponse{
    Message: "获取设备失败",
})
```

---

## ADDED Requirements

### Requirement: Translation comments MAY include original English (翻译注释可选标注英文原文)

**Requirement ID**: `api-translation-comment`

**描述**: 在翻译后的代码中，**SHALL**允许添加注释标注原英文信息，便于代码维护和理解。
Description: Translated lines SHALL allow an optional comment including original English text for maintainability.

**变更类型**: ADDED

**影响范围**:
- 翻译后的代码行

#### Scenario: 添加翻译注释

**Given**: 翻译一个英文错误消息
**When**: 完成翻译
**Then**: 可以在代码中添加注释标注原英文

**示例**:
```go
// EN: "invalid request"
Message: "无效的请求"
```

**说明**: 此注释为可选，主要用于帮助理解原始英文含义。

---

## 验收测试场景

### Test Case 1: 充电流程完整测试

**步骤**:
1. 调用 `POST /api/v1/third/devices/{device_id}/charge` 启动充电
2. 设备在线，端口可用
3. 验证返回的Message为"充电指令发送成功"

**期望结果**: ✅ Message为中文

---

### Test Case 2: 设备离线错误测试

**步骤**:
1. 调用 `POST /api/v1/third/devices/{device_id}/charge` 启动充电
2. 设备离线
3. 验证返回的Message为"设备离线，无法创建订单"

**期望结果**: ✅ Message为中文，Code为503

---

### Test Case 3: 认证失败测试

**步骤**:
1. 调用API但不提供API Key
2. 验证认证中间件返回的message为"缺少API Key"

**期望结果**: ✅ Message为中文

---

### Test Case 4: 查询订单测试

**步骤**:
1. 调用 `GET /api/v1/third/orders/{order_id}`
2. 订单不存在
3. 验证返回的Message为"订单不存在"

**期望结果**: ✅ Message为中文，Code为404

---

## 向后兼容性说明

**兼容性级别**: 部分向后兼容

**说明**:
- ✅ **API接口结构不变**：HTTP方法、路径、参数、响应格式均不变
- ✅ **错误码（Code字段）不变**：程序化错误处理不受影响
- ⚠️ **错误消息（Message字段）改变**：如果调用方通过匹配英文Message进行错误处理，需要适配

**建议**:
- API调用方应使用Code字段而非Message字段进行错误判断
- Message字段仅供人类阅读，不应用于程序逻辑

---

## 文档更新要求

- [ ] 更新API文档（Swagger/OpenAPI）中的错误示例
- [ ] 更新CHANGELOG.md，记录本次变更
- [ ] 通知API调用方消息语言变更（如需）

---

**变更日期**: 2025-11-18
**变更人**: AI Assistant
**审核状态**: 待审核
