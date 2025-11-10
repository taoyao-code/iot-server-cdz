# 架构文档索引

欢迎阅读物联网充电桩服务器架构文档！本指南帮助你理解系统的设计、组件和数据流。

## 快速入门

从这里开始快速了解：
- **[ARCHITECTURE_SUMMARY.txt](ARCHITECTURE_SUMMARY.txt)** - 单页快速参考，包含关键模式和数据流

## 综合指南

详细架构文档：
- **[CLAUDE.md](CLAUDE.md)** - 完整的696行架构指南（从这里开始全面理解）
- **[ARCHITECTURE_DIAGRAMS.md](ARCHITECTURE_DIAGRAMS.md)** - 可视化流程图和组件层次结构

## 文档结构

### CLAUDE.md（主要参考）
权威架构指南，涵盖：

1. **系统概述** - 系统的功能和目的
2. **核心数据流**（3个部分）
   - 入站路径：设备→服务器
   - 出站路径：服务器→设备
   - 会话管理架构
3. **关键架构模式**（4个部分）
   - 协议适配器模式（可插拔协议）
   - 解耦处理器模式（非阻塞）
   - 加权在线检测（多信号）
   - 带降级的优先级队列（背压）
4. **启动序列**（9个阶段）
   - 编排式引导确保所有依赖就绪
5. **关键文件**（10个类别）
   - 50+个关键文件映射到职责
6. **消息处理示例**（3个流程）
   - 心跳→数据库持久化
   - API命令→设备传递
   - BKV帧解析
7. **性能与可靠性**（4个模式）
8. **故障排除指南**（5个场景）
9. **测试策略**
10. **扩展点**（3个领域）

### ARCHITECTURE_SUMMARY.txt（快速参考）
精简参考，包含：
- 核心数据流（可视化）
- 关键模式（每个1-2行）
- 启动序列概述
- 按类别分类的关键文件
- 消息流示例
- 性能特征
- Redis键模式
- 故障排除清单

### ARCHITECTURE_DIAGRAMS.md（可视化指南）
ASCII图表展示：
1. 系统组件层次结构
2. 入站消息流（设备→服务器）
3. 出站消息流（服务器→设备）
4. 会话生命周期
5. 多阶段引导序列
6. 协议检测与分发
7. Redis存储架构
8. 错误处理和重试流程

## 需要理解的关键概念

### 1. 多协议网关
系统接受来自使用不同协议（AP3000、BKV）的物联网设备的连接。每个协议包含：
- **适配器**：嗅探魔数，检测协议类型
- **解码器**：将原始字节转换为帧（处理帧、TCP数据包边界）
- **路由器**：命令代码→处理器查找
- **处理器**：每种消息类型的业务逻辑

### 2. 异步命令传递
命令通过按优先级排序的Redis队列流转：
- HTTP API请求立即入队（返回202）
- 后台工作线程每100ms轮询队列
- 工作线程在发送前验证设备在线
- 命令在进程重启后仍保留（持久化在Redis中）
- 失败的消息重试最多N次，然后移至死信队列

### 3. 会话管理
设备连接在Redis中跟踪：
- 物理ID（phyID）→连接映射
- 心跳时间戳用于在线检测
- 加权策略考虑TCP断开和ACK超时
- 分布式：跨多个服务器实例工作

### 4. 两层存储
- **PostgreSQL**：持久化数据（设备、订单、命令日志、审计跟踪）
- **Redis**：会话数据、命令队列、去重（高吞吐量）

### 5. 9阶段启动
精心编排确保：
1. 指标 + 状态跟踪器就绪
2. Redis客户端可用（必需）
3. 会话管理器初始化
4. 数据库连接并迁移（必需）
5. 业务处理器创建
6. HTTP服务器启动
7. 后台工作线程启动
8. **TCP服务器最后启动**（在所有依赖之后）
9. SIGTERM时优雅关闭

## 文件组织

```
internal/
├── app/
│   ├── bootstrap/app.go          ← 9阶段启动编排
│   ├── tcp.go, http.go, db.go    ← 组件工厂
│   ├── session.go, redis.go      ← 依赖创建
│   └── outbound_adapter.go       ← Redis队列桥接
│
├── tcpserver/
│   ├── server.go                 ← TCP监听器
│   ├── mux.go                    ← 协议检测
│   ├── conn.go                   ← 连接包装器
│   └── limiter.go, rate_limiter.go, circuit_breaker.go
│
├── gateway/
│   └── conn_handler.go           ← 处理器注册（300行）
│
├── protocol/
│   ├── adapter/adapter.go        ← 接口
│   ├── ap3000/
│   │   ├── adapter.go, parser.go, decoder.go
│   │   └── handlers.go
│   └── bkv/
│       ├── adapter.go, parser.go, decoder.go
│       └── handlers.go（约1500行）
│
├── session/
│   ├── interface.go              ← SessionManager契约
│   └── redis_manager.go          ← Redis实现
│
├── storage/
│   ├── redis/
│   │   ├── outbound_queue.go     ← 队列操作
│   │   └── client.go
│   └── pg/
│       ├── repo.go               ← 所有数据库查询（约700行）
│       └── pool.go
│
├── api/
│   ├── routes.go, thirdparty_routes.go  ← 路由注册
│   ├── thirdparty_handler.go    ← 命令端点
│   └── readonly_handler.go       ← 查询端点
│
├── outbound/
│   ├── redis_worker.go           ← 轮询和传递
│   └── priority.go               ← 优先级计算
│
└── health/
    ├── aggregator.go             ← 复合健康检查
    ├── checker.go, *_checker.go  ← DB、Redis、TCP检查
    └── http_routes.go
```

## 常见问题解答

**问：设备命令如何发送？**
答：HTTP POST → ThirdPartyHandler → RedisQueue.Enqueue() → 返回202 → RedisWorker轮询 → 检查设备在线 → 写入socket

**问：如果设备在命令期间离线会怎样？**
答：RedisWorker通过会话管理器检测离线 → 标记失败 → 重试最多3次 → 移至死信队列供人工审查

**问：系统如何检测设备是否在线？**
答：简单方式：5分钟内有心跳。加权方式：最近心跳+1.0，最近TCP断开-0.2，最近ACK超时-0.3，阈值0.5

**问：系统能否水平扩展？**
答：可以！会话存储在Redis中（非本地内存）。每个服务器获得一个UUID。TCP连接本地绑定，但可以通过Redis查询其他服务器的会话

**问：服务器重启时会发生什么？**
答：Redis队列中的命令持久化。会话数据清理。TCP客户端重新连接（正常流程）。死信队列保留供分析

**问：命令如何排定优先级？**
答：分数 = 优先级 × 1e12 + 纳秒。高优先级命令先出队。队列降级在队列过长时拒绝低优先级

## 后续步骤

1. **理解系统**：阅读CLAUDE.md第1-3节（数据流和模式）
2. **学习启动**：阅读CLAUDE.md第3节和ARCHITECTURE_DIAGRAMS.md（引导序列）
3. **追踪消息**：遵循CLAUDE.md第5节中的示例
4. **扩展系统**：查看CLAUDE.md第10节（扩展点）
5. **调试问题**：使用CLAUDE.md第8节（故障排除）或ARCHITECTURE_SUMMARY.txt

## 文档统计

| 文件 | 行数 | 重点 |
|------|-------|-------|
| CLAUDE.md | 696 | 完整架构指南 |
| ARCHITECTURE_SUMMARY.txt | 170 | 快速参考卡 |
| ARCHITECTURE_DIAGRAMS.md | ~400 | 可视化流程图 |
| ARCHITECTURE_INDEX.md | ~300 | 本文件 |

## 相关代码参考

查看这些文件了解实现细节：
- **入口点**：`cmd/server/main.go`
- **配置**：`internal/config/*.go`
- **迁移**：`db/migrations/*.sql`
- **测试**：`test/e2e/`、`internal/**/*_test.go`
- **示例**：`examples/gn_usage.go`

## 支持与维护

- **配置**：查看`CLAUDE.md`第7节了解所有配置选项
- **健康检查**：使用`/health`端点查看系统状态
- **指标**：Prometheus指标位于`/metrics`端点
- **日志**：检查应用程序日志以进行详细流程追踪

---

**最后更新**：2025-11-10
**如有问题**：参考CLAUDE.md或查看`internal/`包中的文件注释
