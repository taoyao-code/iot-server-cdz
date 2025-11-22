# 实施任务清单

本文档列出了统一翻译API返回信息的具体实施任务。

## 任务总览

- **总任务数**: 10项
- **预计工时**: 2.5天
- **优先级**: Phase 1 > Phase 2 > Phase 3

## Phase 1: API Handler层（优先级：高）

### ✅ Task 1.1: 翻译 thirdparty_handler.go

- **文件路径**: `internal/api/thirdparty_handler.go`
- **翻译条目数**: 约40处
- **预计耗时**: 1小时
- **依赖**: 无
- **验收标准**:
  - [ ] 所有StandardResponse的Message字段翻译为中文
  - [ ] Code字段保持不变
  - [ ] 单元测试通过（如有）
  - [ ] API手工测试通过

**主要翻译接口**:
- StartCharge: 约9处
- StopCharge: 约6处
- CancelOrder: 约4处
- GetDevice: 约3处
- GetOrder: 约2处
- ListOrders: 约2处
- SetParams: 约5处
- TriggerOTA: 约5处
- GetOrderEvents: 约2处
- 辅助函数: 约2处

---

### ✅ Task 1.2: 翻译 readonly_handler.go

- **文件路径**: `internal/api/readonly_handler.go`
- **翻译条目数**: 约7处
- **预计耗时**: 20分钟
- **依赖**: 无
- **验收标准**:
  - [ ] 所有gin.H的error字段翻译为中文
  - [ ] 单元测试通过
  - [ ] API响应格式正确

**翻译点**:
- Line 63, 82, 224: 通用错误处理
- Line 103: "device not found" → "设备不存在"
- Line 110: "query failed" → "查询失败"
- Line 186: "invalid id" → "无效的ID"
- Line 191: "not found" → "未找到"

---

### ✅ Task 1.3: 翻译 ota_handler.go

- **文件路径**: `internal/api/ota_handler.go`
- **翻译条目数**: 约9处
- **预计耗时**: 20分钟
- **依赖**: 无
- **验收标准**:
  - [ ] 所有gin.H的error/message字段翻译为中文
  - [ ] API响应正确

**翻译点**:
- Line 47, 117: "invalid device_id" → "无效的设备ID"
- Line 53: "invalid request" → "无效的请求"
- Line 58: "target_socket_no required" → "缺少target_socket_no参数"
- Line 76: "failed to create" → "创建失败"
- Line 80: "success" → "成功"
- Line 93: "invalid task_id" → "无效的任务ID"
- Line 99: "not found" → "未找到"
- Line 128: "query failed" → "查询失败"

---

### ✅ Task 1.4: 翻译 network_handler.go（如需）

- **文件路径**: `internal/api/network_handler.go`
- **翻译条目数**: 待确认
- **预计耗时**: 10分钟
- **依赖**: 无
- **说明**: 根据实际代码确认是否有需要翻译的内容

---

### ✅ Task 1.5: 翻译 testconsole_handler.go（如需）

- **文件路径**: `internal/api/testconsole_handler.go`
- **翻译条目数**: 待确认
- **预计耗时**: 10分钟
- **依赖**: 无
- **说明**: 根据实际代码确认是否有需要翻译的内容

---

## Phase 2: 中间件层（优先级：中）

### ✅ Task 2.1: 翻译 auth.go

- **文件路径**: `internal/api/middleware/auth.go`
- **翻译条目数**: 4处
- **预计耗时**: 15分钟
- **依赖**: Task 1.x 完成后开始
- **验收标准**:
  - [ ] 所有error字段翻译为中文
  - [ ] message字段保持不变（已是中文）
  - [ ] 认证流程正常工作

**翻译点**:
- Line 54: "unauthorized" → "未授权"
- Line 77: "forbidden" → "禁止访问"
- Line 149: "unauthorized" → "未授权"
- Line 172: "forbidden" → "禁止访问"

---

### ✅ Task 2.2: 翻译 thirdparty_auth.go

- **文件路径**: `internal/api/middleware/thirdparty_auth.go`
- **翻译条目数**: 2处
- **预计耗时**: 10分钟
- **依赖**: Task 1.x 完成后开始
- **验收标准**:
  - [ ] 所有message字段翻译为中文
  - [ ] 第三方认证流程正常

**翻译点**:
- Line 36: "missing api key" → "缺少API Key"
- Line 52: "invalid api key" → "无效的API Key"

---

## Phase 3: 存储层（优先级：低）

### ✅ Task 3.1: 翻译 outbound_queue.go

- **文件路径**: `internal/storage/redis/outbound_queue.go`
- **翻译条目数**: 8处
- **预计耗时**: 30分钟
- **依赖**: Task 2.x 完成后开始
- **验收标准**:
  - [ ] 所有fmt.Errorf的英文消息翻译为中文
  - [ ] 保留P1-6等技术标识
  - [ ] 队列功能正常

**翻译点**:
- Line 63: "queue overloaded" → "队列过载"
- Line 68: "queue critical" → "队列临界"
- Line 73: "queue emergency" → "队列紧急"
- Line 162: "msgID cannot be zero" → "消息ID不能为零"
- Line 170: "no processing message found" → "未找到处理中的消息"
- Line 181: "message not found in processing" → "消息未在处理队列中找到"
- Line 369: "invalid message format" → "无效的消息格式"

---

## Phase 4: 测试与验证（优先级：最高）

### ✅ Task 4.1: 单元测试验证

- **预计耗时**: 1小时
- **依赖**: 所有翻译任务完成
- **执行步骤**:
  ```bash
  # 运行所有单元测试
  make test
  
  # 或针对性测试
  go test ./internal/api/...
  go test ./internal/storage/redis/...
  ```
- **验收标准**:
  - [ ] 所有现有单元测试通过
  - [ ] 无编译错误
  - [ ] 无运行时错误

---

### ✅ Task 4.2: API集成测试

- **预计耗时**: 1小时
- **依赖**: Task 4.1 完成
- **测试场景**:
  1. 测试正常充电流程，验证成功消息为中文
  2. 测试各类错误场景，验证错误消息为中文
  3. 测试认证失败场景，验证认证消息为中文
  4. 测试OTA升级，验证返回消息为中文
  
- **测试工具**: Postman或curl
- **验收标准**:
  - [ ] 所有API返回的Message字段为中文
  - [ ] Code字段保持不变
  - [ ] JSON格式正确
  - [ ] 业务逻辑正常

---

### ✅ Task 4.3: 文档同步更新

- **预计耗时**: 30分钟
- **依赖**: Task 4.2 完成
- **更新内容**:
  1. API文档（swagger/openapi）：更新错误消息示例
  2. CHANGELOG.md：记录本次变更
  3. README.md：说明错误消息语言（如需）
  
- **验收标准**:
  - [ ] API文档示例更新为中文
  - [ ] CHANGELOG记录详细
  - [ ] 文档与代码一致

---

## 总体验收清单

在所有任务完成后，进行最终验收：

- [ ] **代码质量**
  - [ ] 所有翻译准确无误
  - [ ] Code字段未被修改
  - [ ] JSON结构完整
  - [ ] 日志消息未被翻译
  
- [ ] **测试覆盖**
  - [ ] 单元测试全部通过
  - [ ] 集成测试全部通过
  - [ ] 手工测试验证成功
  
- [ ] **文档同步**
  - [ ] API文档已更新
  - [ ] CHANGELOG已更新
  - [ ] 相关文档已同步

- [ ] **代码审查**
  - [ ] 代码已提交PR
  - [ ] 代码审查通过
  - [ ] 无遗留问题

## 时间规划

| Phase | 任务 | 预计耗时 | 累计耗时 |
|-------|------|----------|----------|
| Phase 1 | Task 1.1 - 1.5 | 2小时 | 2小时 |
| Phase 2 | Task 2.1 - 2.2 | 25分钟 | 2.5小时 |
| Phase 3 | Task 3.1 | 30分钟 | 3小时 |
| Phase 4 | Task 4.1 - 4.3 | 2.5小时 | 5.5小时 |
| **总计** | | **5.5小时** | **约1天** |

## 注意事项

1. **并行执行**: Task 1.1 - 1.5 可以并行执行，提高效率
2. **增量提交**: 每完成一个Phase，提交一次代码，便于回滚
3. **及时测试**: 每个Task完成后立即测试，避免累积问题
4. **保留备份**: 修改前备份原文件或创建分支
5. **沟通确认**: 翻译有疑问的地方及时与团队确认

## 风险应对

| 风险 | 概率 | 影响 | 应对措施 |
|------|------|------|----------|
| 翻译不准确 | 低 | 高 | 代码审查时重点检查；提供翻译对照表 |
| 破坏现有功能 | 低 | 高 | 充分测试；增量提交；保留回滚方案 |
| 影响API调用方 | 中 | 中 | 提前通知；建议使用Code字段判断 |
| 时间超期 | 低 | 低 | 合理分配任务；预留缓冲时间 |

---

**下一步**: 等待提案批准后，按此任务清单执行实施工作。
