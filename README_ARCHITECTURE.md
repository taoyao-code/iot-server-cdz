# 架构文档 - 完整指南

## 已创建的文件

我为物联网充电桩服务器创建了一套完整的架构文档包：

### 1. **CLAUDE.md** (24KB, 696行) - 主要参考文档
权威架构指南，包含10个主要章节：
- 系统概述
- 核心数据流（入站、出站、会话）
- 架构模式（适配器、处理器、在线检测、队列）
- 启动序列（9个阶段）
- 关键文件（50+个文件映射到职责）
- 消息处理示例（3个完整流程）
- 性能与可靠性模式
- 故障排除指南
- 测试策略
- 扩展点

**从这里开始全面了解系统**

### 2. **ARCHITECTURE_SUMMARY.txt** (7.6KB, 170行) - 快速参考
单页参考卡，包括：
- 核心数据流（可视化）
- 关键架构模式
- 9阶段启动序列概述
- 按类别组织的关键文件
- 带步骤的消息流示例
- 性能特征
- Redis键模式
- 故障排除清单

**用于快速查找和定位**

### 3. **ARCHITECTURE_DIAGRAMS.md** (40KB, 667行) - 可视化指南
8个详细的ASCII图表，展示：
1. 系统组件层次结构
2. 入站消息流（设备→服务器）
3. 出站消息流（服务器→设备）
4. 会话生命周期及Redis存储
5. 多阶段引导序列
6. 协议检测与分发
7. Redis存储架构及所有键模式
8. 错误处理和重试流程

**在跟踪消息流或理解时序时参考这些图表**

### 4. **ARCHITECTURE_INDEX.md** (8.2KB, 217行) - 导航指南
导航和概览：
- 快速入门建议
- 文档结构
- 需要理解的关键概念
- 文件组织树
- 常见问题解答
- 后续学习步骤
- 文档统计

**将此作为所有文档的入口点**

## 你将获得什么

### 理解
- **大局观**：TCP网关、适配器、会话管理和队列如何协同工作
- **数据流**：从设备连接到命令传递的完整消息追踪
- **模式**：4个关键架构模式解释设计决策
- **启动**：为什么9阶段编排很重要，每个阶段的作用

### 代码导航
- **50+个关键文件**：每个文件都映射到特定职责
- **文件组织**：internal/包结构的可视化树
- **入口点**：针对不同任务的代码阅读起点

### 问题解决
- **故障排除指南**：5个场景及解决步骤
- **Redis模式**：完整的键架构及示例
- **错误流程**：重试、死信队列和恢复的工作原理

### 扩展
- **扩展点**：添加新协议、端点、优先级的位置
- **模式参考**：如何使用现有模式进行实现
- **测试策略**：现有测试如何演示模式

## 如何使用本文档

### 理解系统（2-3小时）
1. 阅读 ARCHITECTURE_INDEX.md（5分钟）
2. 阅读 ARCHITECTURE_SUMMARY.txt（10分钟）
3. 阅读 CLAUDE.md 第1-3节（数据流+模式）（60分钟）
4. 学习 ARCHITECTURE_DIAGRAMS.md（入站和出站流程）（30分钟）

### 特定任务（30分钟）
1. 检查 ARCHITECTURE_INDEX.md 的"常见问题"找到你的场景
2. 跳转到相关的 CLAUDE.md 章节
3. 参考 ARCHITECTURE_DIAGRAMS.md 进行可视化确认

### 调试（15分钟）
1. 转到 CLAUDE.md 第8节"故障排除指南"
2. 与 ARCHITECTURE_SUMMARY.txt 清单交叉参考
3. 使用 ARCHITECTURE_DIAGRAMS.md 第7节的Redis键模式

### 添加功能（45分钟）
1. 阅读 CLAUDE.md 第10节"扩展点"
2. 使用文件组织找到类似的现有代码
3. 遵循 CLAUDE.md 第2-4节的模式
4. 使用 ARCHITECTURE_DIAGRAMS.md 理解时序

## 关键见解

### 1. 协议适配器模式
系统通过第一帧的魔数检测协议，然后锁定该适配器。这使得AP3000、BKV和未来协议无需代码更改即可支持。

### 2. 异步命令传递
命令在Redis中排队（非阻塞），允许处理器快速返回，后台工作线程原子性地管理传递。

### 3. 分布式会话
会话存储在Redis中，包含serverID和connID，支持水平扩展 - 设备可以在服务器之间移动。

### 4. 基于优先级的背压
队列长度触发自动优先级降级 - 队列>200时拒绝低优先级命令，500时拒绝中等优先级，1000+时只接受紧急命令。

### 5. 多阶段启动
关键的顺序确保依赖在使用前准备就绪：会话前先Redis，处理器前先数据库，TCP网关前先工作线程。

## 关键文件快速参考

| 任务 | 前往 |
|------|-------|
| 理解整体架构 | CLAUDE.md 第1-2节 |
| 追踪设备消息流 | ARCHITECTURE_DIAGRAMS.md #2 |
| 追踪API命令流 | ARCHITECTURE_DIAGRAMS.md #3 |
| 查找TCP监听位置 | CLAUDE.md 第4.2节，然后 internal/tcpserver/server.go |
| 查找协议处理器注册 | CLAUDE.md 第4.6节，然后 internal/gateway/conn_handler.go |
| 理解会话管理 | CLAUDE.md 第1.3节，然后 internal/session/redis_manager.go |
| 查找出站队列实现 | CLAUDE.md 第4.5节，然后 internal/storage/redis/outbound_queue.go |
| 查看启动编排 | CLAUDE.md 第3.1节，然后 internal/app/bootstrap/app.go |
| 调试设备离线 | CLAUDE.md 第8节，然后检查 Redis session:device:{phyID} |
| 调试命令未发送 | CLAUDE.md 第8节，然后检查 Redis outbound:queue 长度 |

## 文件统计摘要

```
CLAUDE.md                   696行   24KB  完整参考
ARCHITECTURE_DIAGRAMS.md    667行   40KB  可视化流程图
ARCHITECTURE_INDEX.md       217行   8.2KB 导航指南
ARCHITECTURE_SUMMARY.txt    170行   7.6KB 快速参考
─────────────────────────────────────────────────────────────
总计                       1750行   79KB  完整包
```

## 这些文档与源代码的关系

- **系统概述** → `cmd/server/main.go` + `internal/app/bootstrap/app.go`
- **TCP网关** → `internal/tcpserver/` + `internal/gateway/conn_handler.go`
- **协议适配器** → `internal/protocol/ap3000/adapter.go` + `internal/protocol/bkv/adapter.go`
- **会话管理** → `internal/session/redis_manager.go`
- **出站队列** → `internal/storage/redis/outbound_queue.go` + `internal/outbound/redis_worker.go`
- **HTTP API** → `internal/api/thirdparty_handler.go` + `internal/api/thirdparty_routes.go`
- **数据库** → `internal/storage/pg/repo.go`

## 版本信息

- **生成日期**：2025-11-10
- **目标版本**：IoT Server v1.0.0
- **Go版本**：1.23+
- **关键依赖**：Redis、PostgreSQL、Gin、pgx/v5

## 后续步骤

1. **开始阅读**：打开 ARCHITECTURE_INDEX.md 进行引导式导航
2. **快速概览**：10分钟内阅读 ARCHITECTURE_SUMMARY.txt
3. **深入研究**：阅读 CLAUDE.md 全面理解
4. **可视化学习**：阅读代码时参考 ARCHITECTURE_DIAGRAMS.md
5. **实践**：使用文件参考探索实际实现

## 支持

关于架构的问题：
1. 检查 CLAUDE.md 目录找到相关章节
2. 搜索 ARCHITECTURE_DIAGRAMS.md 查找可视化流程
3. 使用 ARCHITECTURE_SUMMARY.txt 故障排除清单
4. 参考 ARCHITECTURE_INDEX.md 中的文件路径探索源代码
