# 变更提案：统一翻译API返回信息为中文

## 元数据

- **变更ID**: `translate-api-messages`
- **提案人**: AI Assistant
- **创建日期**: 2025-11-18
- **状态**: 待批准 (Pending)
- **优先级**: 中
- **预计工作量**: 2-3天

## Why

### 业务价值

统一API返回消息的语言，为国内用户提供一致的中文体验，提升产品的专业性和用户满意度。

### 技术必要性

当前代码库中API响应消息中英文混杂，与Service层和Protocol层已有的中文消息风格不一致，造成维护困难。通过统一翻译，可以：
- 降低代码维护成本
- 提高代码可读性
- 改善用户体验

### 对现有系统的影响

- 不影响API接口结构和错误码
- 仅改变面向用户的Message字段内容
- 日志和内部调试信息保持英文

## 1. 背景与动机

### 1.1 当前问题

当前项目中的API返回信息存在中英文混杂的情况：

1. **API层**（`internal/api`）大量使用英文错误消息，如：
   - `"invalid request"`
   - `"device not found"`
   - `"port is busy"`
   - `"failed to get device"`

2. **中间件层**（`internal/api/middleware`）使用英文：
   - `"missing api key"`
   - `"invalid api key"`
   - `"unauthorized"`

3. **存储层**（`internal/storage`）使用英文：
   - `"queue overloaded"`
   - `"invalid message format"`

4. **Service层**和**Protocol层**已大量使用中文，如：
   - `"设备不存在"`
   - `"金额不足最低消费"`
   - `"成功"` / `"失败"`

### 1.2 问题影响

- **用户体验不一致**：第三方API调用方收到的错误信息中英文混杂，影响阅读体验
- **维护困难**：团队需要同时维护中英文两套表述，增加认知负担
- **文档不统一**：API文档需要额外解释部分英文错误码的含义

### 1.3 变更目标

将项目中所有面向用户的返回信息（API响应、错误信息、提示信息）统一翻译为中文，实现：

1. **用户体验一致性**：所有API返回的Message字段使用中文
2. **代码风格统一**：与现有Service层、Protocol层的中文信息保持一致
3. **国内用户友好**：符合国内项目和用户的语言习惯

## 2. 变更范围

### 2.1 涵盖模块

1. **API Handler层**：
   - `internal/api/thirdparty_handler.go` - 第三方API处理器（主要）
   - `internal/api/readonly_handler.go` - 只读查询接口
   - `internal/api/ota_handler.go` - OTA管理接口
   - `internal/api/network_handler.go` - 网络管理接口
   - `internal/api/testconsole_handler.go` - 测试控制台

2. **中间件层**：
   - `internal/api/middleware/auth.go` - 认证中间件
   - `internal/api/middleware/thirdparty_auth.go` - 第三方认证

3. **存储层**（部分）：
   - `internal/storage/redis/outbound_queue.go` - 出站队列错误

### 2.2 不涵盖的部分

以下内容**不**纳入本次翻译范围：

- **日志消息**：Logger输出的英文消息保持不变，便于开发调试
- **代码注释**：保持现有注释语言
- **Swagger文档注释**：保持现有英文注释，便于国际化
- **测试代码**：测试用例中的消息保持不变
- **数据库字段**：数据库表结构和字段名不变
- **协议常量**：BKV协议相关的常量定义不变

## 3. 技术方案

### 3.1 翻译策略

采用**直接替换法**，将英文字符串直接替换为对应的中文表述：

```go
// Before:
Message: "invalid request"

// After:
Message: "无效的请求"
```

### 3.2 翻译原则

1. **准确性**：确保中文翻译准确传达原英文含义
2. **简洁性**：使用简洁明了的中文表述，避免冗长
3. **一致性**：相同的英文信息使用统一的中文翻译
4. **专业性**：使用行业标准术语，如"设备"、"端口"、"订单"

### 3.3 兼容性保障

1. **保留错误码**：StandardResponse的Code字段保持不变，程序化处理不受影响
2. **保留英文注释**：在代码中添加英文注释，标注原英文信息
3. **向后兼容**：API接口结构不变，仅Message字段内容改变

## 4. 影响分析

### 4.1 积极影响

- ✅ 提升用户体验，统一语言风格
- ✅ 降低维护成本，减少语言切换
- ✅ 符合国内项目习惯
- ✅ 与现有代码风格一致

### 4.2 潜在风险

- ⚠️ **API调用方兼容性**：如果现有调用方通过匹配英文Message进行错误处理，需要更新
  - **缓解措施**：建议调用方使用Code字段而非Message进行判断；提前通知变更
- ⚠️ **文档同步**：需要同步更新API文档
  - **缓解措施**：在实施阶段同步更新文档

### 4.3 回滚方案

如需回滚，可以：
1. 通过Git恢复到变更前的版本
2. 或创建反向翻译脚本，将中文恢复为英文

## 5. 实施计划

### 5.1 阶段划分

1. **Phase 1 - API Handler层**（优先级高）
   - 翻译`thirdparty_handler.go`中的所有英文Message
   - 翻译其他handler文件中的英文Message

2. **Phase 2 - 中间件层**
   - 翻译认证中间件的错误信息

3. **Phase 3 - 存储层**（优先级低）
   - 翻译Redis队列相关的错误信息

### 5.2 时间估算

- Phase 1: 1天
- Phase 2: 0.5天
- Phase 3: 0.5天
- 测试验证: 0.5天
- **总计**: 2.5天

## 6. 验收标准

### 6.1 功能验收

- [ ] 所有API端点返回的Message字段均为中文
- [ ] 中间件返回的错误消息均为中文
- [ ] 错误码（Code字段）保持不变

### 6.2 测试验收

- [ ] 现有单元测试全部通过
- [ ] 现有集成测试全部通过
- [ ] 手工测试各API端点，验证错误消息正确

### 6.3 文档验收

- [ ] API文档同步更新
- [ ] CHANGELOG记录变更内容

## 7. 参考资料

- 现有代码中的中文错误信息（`internal/service/pricing.go`、`internal/protocol/bkv/handlers.go`等）
- OpenSpec项目规范（`openspec/project.md`）

## 8. 讨论事项

- [ ] 是否需要提前通知API调用方？
- [ ] 是否需要同步更新Swagger文档的错误示例？
- [ ] 是否需要创建多语言支持预留接口？（未来扩展）

---

**请审阅并批准此提案，批准后将进入实施阶段。**
