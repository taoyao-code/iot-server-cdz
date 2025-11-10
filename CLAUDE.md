# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 项目概述

这是一个**充电桩物联网服务器**，支持 AP3000、BKV、GN 等多种协议，采用 TCP 网关 + HTTP API 的架构，支持 50,000+ 并发设备连接。

### 核心特性
- **TCP 网关**：多协议支持（AP3000、BKV、GN），自动协议检测，目前重点bkv协议
- **分布式会话**：基于 Redis 的会话管理，支持水平扩展
- **异步下行队列**：Redis 优先级队列，支持自动重试和降级
- **事件推送**：Webhook 方式推送设备事件到第三方平台
- **监控完善**：Prometheus 指标、健康检查、结构化日志

---

## 常用命令

### 本地开发
```bash
# 一键启动（推荐）
make dev-all          # 启动依赖服务 + 应用服务器

# 分步启动
make dev-up           # 启动 PostgreSQL（复用本地 Redis）
make dev-run          # 启动应用服务器
make dev-down         # 停止依赖服务
make dev-clean        # 清理环境（包括数据）

# 独立运行（需先启动依赖服务）
IOT_CONFIG=./configs/local.yaml go run ./cmd/server
```

### 测试相关
```bash
make test             # 运行所有测试（带竞态检测）
make test-verbose     # 详细输出
make test-coverage    # 生成覆盖率报告（coverage.html）

# 运行单个测试
go test -v -race ./internal/protocol/bkv -run TestHandleHeartbeat
go test -v ./internal/session/...

# E2E 测试
make test-e2e
```

### 代码规范
```bash
make fmt              # 格式化代码（自动修复）
make fmt-check        # 检查格式（CI 模式）
make vet              # 静态分析
make lint             # Lint 检查
make install-hooks    # 安装 Git pre-commit hooks
```

### 部署相关
```bash
make auto-deploy      # 自动化部署（构建 + 迁移 + 部署）⭐
make remote-migrate   # 远程数据库迁移
make quick-deploy     # 快速部署（仅替换二进制，30秒完成）
make deploy           # 标准部署（测试环境）
make deploy-full      # 完整部署（带备份，生产环境）

# Docker 镜像
make docker-build     # 构建 Docker 镜像
```

### 监控调试
```bash
make monitor          # 完整诊断（推荐）
make monitor-logs     # 实时日志
make monitor-errors   # 错误日志
make monitor-metrics  # 业务指标

# TCP 模块测试
make tcp-check        # 检查 TCP 端口
make tcp-metrics      # TCP 指标

# 协议监控
make protocol-logs    # 实时协议日志
make protocol-stats   # 实时统计
```

### API 文档生成
```bash
make api-docs         # 生成 Swagger API 文档
# 文档位置：api/swagger/swagger.json
# 访问：http://localhost:7055/swagger/index.html
```

---

## 核心架构

### 数据流向

#### 上行流（设备 → 服务器）
```
TCP 连接 → TCPServer(6000)
  → Mux（协议检测）
    → AP3000Adapter/BKVAdapter.Sniff()
    → StreamDecoder 解析帧
    → RouterTable 路由到 Handler
      → Session.Bind（绑定会话）
      → Session.OnHeartbeat（更新心跳）
      → 业务处理（数据持久化、事件推送）
```

**关键文件**：
- `internal/tcpserver/server.go` - TCP 监听器
- `internal/tcpserver/mux.go` - 协议检测（23-84 行）
- `internal/gateway/conn_handler.go` - Handler 注册
- `internal/protocol/ap3000/adapter.go` - AP3000 协议
- `internal/protocol/bkv/adapter.go` - BKV 协议

#### 下行流（服务器 → 设备）
```
HTTP API 请求
  → ThirdPartyHandler.StartCharge()
    → 验证设备在线
    → 创建订单（PostgreSQL）
    → 编码命令帧
      → RedisOutboundQueue.Enqueue()（优先级队列）
        → RedisWorker 轮询
          → Session.GetConn() 获取连接
          → ConnContext.Write() 发送到设备
            → 等待 ACK
              → DatabaseRepository.AckOutboundByMsgID()
```

**关键文件**：
- `internal/api/thirdparty_handler.go` - API 入口
- `internal/app/outbound_adapter.go` - 队列适配器
- `internal/storage/redis/outbound_queue.go` - Redis 队列
- `internal/outbound/redis_worker.go` - 轮询发送
- `internal/session/redis_manager.go` - 会话管理

### 启动流程（9 个阶段）

**文件**: `internal/app/bootstrap/app.go` Run() 函数（29-222 行）

```
1. 基础组件    → Metrics、Ready 状态、Server UUID
2. Redis 客户端 → 必需，会话和队列依赖
3. 会话管理器   → RedisManager + WeightedPolicy
4. PostgreSQL  → 连接 + 迁移（db/migrations/*.sql）
5. 业务处理器   → Repository、BKV handlers、事件队列、下行适配器
6. HTTP 服务器 → Gin 路由、健康检查、API 端点（非阻塞启动）
7. 后台 Worker → Redis 下行队列轮询、事件队列、订单监控、端口同步
8. TCP 服务器  → 最后启动，确保所有依赖就绪（阻塞监听）
9. 优雅关闭    → SIGINT/SIGTERM 信号处理、10 秒超时
```

**关键决策**：
- 数据库必须在 Handler 之前就绪
- Redis 必须在会话管理器之前就绪
- TCP 最后启动，避免接收连接时 Handler 未初始化
- 所有 Worker 必须在 TCP 之前运行

### 协议适配器模式

**设计**：基于注册表的可插拔协议处理器

```
┌─────────────────────────────────┐
│   Protocol Adapter Interface    │
│  - Sniff([]byte) bool           │
│  - ProcessBytes([]byte) error   │
└─────────────────────────────────┘
       ↑                    ↑
       │                    │
  AP3000Adapter        BKVAdapter
  + StreamDecoder      + StreamDecoder
  + RouterTable        + RouterTable
```

**协议检测机制**：
- AP3000: magic = 0x44, 0x22, 0x4E ("D\"N")
- BKV: magic = 0xFC, 0xFE 或 0xFC, 0xFF
- 首帧检测后，后续帧使用同一适配器

**Handler 注册示例**：
```go
// AP3000: 命令码 → Handler
apAdapter.Register(0x20, func(f *Frame) error {
    bindIfNeeded(f.PhyID)
    sess.OnHeartbeat(f.PhyID, time.Now())
    return handlerSet.HandleRegister(ctx, f)
})

// BKV: 命令码（uint16）→ Handler
bkvAdapter.Register(0x0000, wrapBKVHandler(func(ctx, f) error {
    return getBKVHandlers().HandleHeartbeat(ctx, f)
}))
```

### 会话管理

**Redis 存储结构**：
```
session:device:{phyID}           → {connID, serverID, lastSeen}  (Hash)
session:conn:{connID}            → phyID                         (String)
session:server:{serverID}:conns  → Set<connID>                   (Set)
session:tcp_down:{phyID}         → timestamp                     (String, TTL)
session:ack_timeout:{phyID}      → timestamp                     (String, TTL)
```

**在线检测策略**：

1. **简单检测**：`IsOnline(phyID, now)` - 心跳超时判断
2. **加权检测**：`IsOnlineWeighted(phyID, now, policy)` - 多信号评分
   ```
   score = 0.0
   if (now - lastHeartbeat) ≤ HeartbeatTimeout:
       score += 1.0
   if (now - lastTCPDown) ≤ TCPDownWindow:
       score -= TCPDownPenalty (0.2)
   if (now - lastAckTimeout) ≤ AckWindow:
       score -= AckTimeoutPenalty (0.3)
   return score ≥ Threshold (0.5)
   ```

**文件**：`internal/session/redis_manager.go`（215-244 行）

### 优先级队列与降级策略

**队列长度监控**：
```
正常 (0-200)      → 接受所有命令
警告 (200-500)    → 拒绝优先级 > 5（低优先级）
严重 (500-1000)   → 拒绝优先级 > 2（仅普通/高优先级）
紧急 (>1000)      → 仅接受优先级 ≤ 1（紧急命令）
```

**优先级值**：
- 1 = 紧急（心跳 ACK、紧急停止）
- 2 = 高（启动充电、停止充电）
- 5 = 普通（参数查询、状态查询）
- 9 = 低（分析、慢速操作）

**文件**：
- `internal/outbound/priority.go` - 优先级计算
- `internal/storage/redis/outbound_queue.go`（54-73 行）- 队列长度检查

---

## BKV 协议开发指南

**重要**：BKV 协议任务必须先阅读：`docs/协议/设备对接指引-组网设备2024(1).txt`

**关注章节**：
- 2.1 心跳
- 2.2 网络节点
- 2.2.8 控制
- 刷卡充电
- 异常事件
- OTA

**命令号映射**：
- 0x0000: 心跳
- 0x1007: 启动充电
- 0x1004: 停止充电
- 0x1010: 查询状态
- 0x1011: 参数设置

**关键文件**：
- `internal/protocol/bkv/adapter.go` - 命令注册
- `internal/protocol/bkv/handlers.go` - 业务逻辑（~1500 行）
- `internal/protocol/bkv/parser.go` - 帧解析
- `internal/protocol/bkv/*_test.go` - 单元测试
- `internal/protocol/bkv/testdata/` - 测试帧样本

**影响范围**：
- `session.Manager` - 会话绑定
- `outbound` - ACK 跟踪
- `internal/storage/pg/repo.go` - 数据库操作
- `outbound_queue` - 下行队列表
- `devices` - 设备表
- `orders` - 订单表

---

## 关键模块文件

### 启动与配置
| 文件 | 职责 |
|------|------|
| `cmd/server/main.go` | 入口，配置加载 |
| `internal/app/bootstrap/app.go` | 9 阶段初始化编排 |
| `internal/config/*.go` | 配置结构 |

### TCP 网关层
| 文件 | 职责 |
|------|------|
| `internal/tcpserver/server.go` | TCP 监听器、连接生命周期 |
| `internal/tcpserver/conn.go` | ConnContext 包装、写缓冲 |
| `internal/tcpserver/mux.go` | 协议检测、适配器分发 |
| `internal/gateway/conn_handler.go` | Handler 注册（~300 行）|

### 协议适配器
| 文件 | 职责 |
|------|------|
| `internal/protocol/adapter/adapter.go` | 适配器接口 |
| `internal/protocol/ap3000/adapter.go` | AP3000 实现 |
| `internal/protocol/ap3000/handlers.go` | AP3000 业务逻辑 |
| `internal/protocol/bkv/adapter.go` | BKV 实现 |
| `internal/protocol/bkv/handlers.go` | BKV 业务逻辑（~1500 行）|

### 下行队列
| 文件 | 职责 |
|------|------|
| `internal/storage/redis/outbound_queue.go` | Redis 队列操作 |
| `internal/outbound/redis_worker.go` | 轮询发送 |
| `internal/outbound/priority.go` | 优先级计算 |
| `internal/app/outbound_adapter.go` | Handler → Redis 桥接 |

### HTTP API
| 文件 | 职责 |
|------|------|
| `internal/api/thirdparty_handler.go` | 命令 API（充电、停止、OTA）|
| `internal/api/readonly_handler.go` | 查询 API |
| `internal/api/network_handler.go` | 组网管理 |
| `internal/api/ota_handler.go` | OTA 升级 |

### 数据持久化
| 文件 | 职责 |
|------|------|
| `internal/storage/pg/pool.go` | PostgreSQL 连接池 |
| `internal/storage/pg/repo.go` | 数据库查询（~700 行）|
| `internal/app/db.go` | 数据库工厂与迁移 |
| `db/migrations/*.sql` | 数据库迁移脚本 |

### 监控与健康检查
| 文件 | 职责 |
|------|------|
| `internal/health/aggregator.go` | 复合健康状态 |
| `internal/health/database_checker.go` | 数据库探针 |
| `internal/health/redis_checker.go` | Redis 探针 |
| `internal/health/tcp_checker.go` | TCP 服务器探针 |
| `internal/metrics/metrics.go` | Prometheus 指标 |

---

## 配置说明

**关键配置项**：
- `tcp.addr` - TCP 网关绑定地址（默认: :6000）
- `tcp.max_connections` - 连接限制（默认: 10000）
- `redis.addr` - Redis 服务器（必需）
- `database.dsn` - PostgreSQL 连接串（必需）
- `protocols.ap3000.enabled` - 启用 AP3000 协议
- `protocols.bkv.enabled` - 启用 BKV 协议
- `session.heartbeat_timeout` - 会话超时（默认: 5m）
- `gateway.throttle_ms` - Worker 轮询间隔（默认: 100ms）
- `gateway.retry_max` - 消息最大重试次数（默认: 3）

**配置文件**：
- `configs/example.yaml` - 示例配置
- `configs/local.yaml` - 本地开发配置
- `configs/production.yaml` - 生产环境配置

---

## 故障排查

### 设备离线检测
1. 检查 Redis 会话：`HGETALL session:device:{phyID}`
2. 验证心跳时间：`LastSeen` 时间戳
3. 检查加权策略：`WeightedPolicy.Enabled` 和评分计算

### 下行队列堆积
1. 检查队列长度：`ZCARD outbound:queue`
2. 检查处理中：`SCAN outbound:processing:*`
3. 检查死信队列：`LLEN outbound:dead`
4. 查看日志："write to device failed", "device connection not available"

### 协议不匹配
1. 检查原始流的前 8 字节
2. 验证 magic bytes 是否匹配预期协议
3. 查看 `mux_test.go` 中的帧示例
4. 检查 `adapter.Sniff()` 是否匹配实际帧

### 命令未执行
1. 验证设备在线：`sess.IsOnline(phyID, now)`
2. 检查下行队列：消息是否入队？
3. 检查 Worker 日志："downlink message sent" vs "write failed"
4. 验证会话：connContext 是否仍然有效？

---

## 测试策略

### 单元测试
```bash
# 协议层测试
go test -v ./internal/protocol/ap3000/...
go test -v ./internal/protocol/bkv/...

# 会话管理测试
go test -v ./internal/session/...

# 队列测试
go test -v ./internal/storage/redis/...

# 适配器测试
go test -v ./internal/protocol/*/adapter_test.go
```

### 集成测试
```bash
# E2E 测试（完整流程）
make test-e2e
cd test/e2e && go test -v -timeout 10m ./...
```

### 负载测试
```bash
# 连接限制测试
go test -v ./internal/tcpserver/limiter_test.go

# 队列饱和测试
go test -v ./internal/outbound/...
```

---

## 扩展指南

### 添加新协议
1. 创建 `internal/protocol/{proto}/adapter.go` 实现 Adapter 接口
2. 创建 parser/decoder 用于帧格式解析
3. 创建 handlers 处理命令类型
4. 在 `gateway/conn_handler.go` 注册
5. 添加到 `config.Protocols`

### 添加新 API 端点
1. 在 `api/thirdparty_handler.go` 创建 handler 方法
2. 在 `api/thirdparty_routes.go` 注册路由
3. 在 `storage/pg/repo.go` 实现数据库查询
4. 在 `api/*_test.go` 编写测试

### 自定义下行优先级
1. 修改 `outbound/priority.go` GetCommandPriority()
2. 更新 handler 逻辑中的优先级映射
3. 测试队列降级场景

---

## 开发工作流模式

根据 `.github/copilot-instructions.md` 规范，开发过程遵循 **研究 → 构思 → 计划 → 执行 → 评审** 流程：

### [模式：研究]
- BKV 协议任务优先阅读协议文档
- 建立架构全局图：main.go → bootstrap → TCP/HTTP/会话/存储/队列
- 确认数据流：TCP → gateway → 协议适配器 → 持久化/会话/推送

### [模式：构思]
- 提出至少两种实现思路
- 说明对 session.Manager、outbound、PG 仓储的影响
- 使用 Context7 获取最新 API（如需第三方库）

### [模式：计划]
- 拆解到文件级别
- 列出预期命令/帧字段校验
- 说明数据库表影响（outbound_queue, devices, orders）
- 指明文档更新位置

### [模式：执行]
- 代码改动紧贴计划
- BKV 命令注册在 `adapter.go`，业务逻辑在 `handlers.go`
- 构建运行：`IOT_CONFIG=./configs/example.yaml go run ./cmd/server`
- 测试：`go test -race ./...`
- 更新指标同步调整 `metrics.go` 并在 bootstrap 注入

### [模式：评审]
- 对照计划检查：命令码、会话绑定、PG 写入、下行 ACK、指标、文档
- 汇总测试结果，指出剩余风险

---

## 性能指标

| 指标 | 数值 |
|------|------|
| 最大并发连接 | 50,000+ |
| HTTP API QPS | 10,000+ |
| TCP 消息吞吐 | 100,000+/秒 |
| 平均响应时间 | < 50ms |
| 内存占用 | < 2GB |

---

## 监控端点

```bash
# 存活检查
curl http://localhost:7055/healthz

# 就绪检查
curl http://localhost:7055/readyz

# Prometheus 指标
curl http://localhost:7055/metrics
```

---

## 提交规范

遵循 [Conventional Commits](https://www.conventionalcommits.org/)：

```
feat: 添加 BKV 0x1007 启动充电命令
fix: 修复会话管理器在高并发下的竞态条件
perf: 优化下行队列轮询性能
docs: 更新 BKV 协议文档
test: 添加端口状态同步测试用例
refactor: 重构协议适配器注册机制
```

---

## 关键依赖

- Go 1.24+
- PostgreSQL 14+
- Redis 6+
- Docker 20.10+ (可选)
- Docker Compose 2.0+ (可选)
