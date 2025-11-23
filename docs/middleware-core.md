## IoT 中间件核心设计

本文件描述 IOT Server 作为“协议无关中间件核心”的职责边界、模块划分以及协议适配模式，对应 OpenSpec 变更 `refactor-middleware-core`。

### 1. 核心职责与非核心职责

**核心职责（Middleware Core）**

- 设备与端口状态管理  
  - 基于 `SessionManager` 维护在线会话，提供 `IsOnline` 等接口作为在线状态真相源；  
  - 使用 `CoreRepo` 访问 `devices` / `ports` 表，持久化 `last_seen_at` 与端口 BKV 位图快照。  
- 订单生命周期管理  
  - 通过 `CoreRepo` 对 `orders` 表进行创建、状态流转（pending/confirmed/charging/…）以及结算；  
  - 保证订单状态与端口状态在 30–60s 窗口内收敛，与 `consistency-lifecycle` 规范一致。  
- 下行指令队列  
  - 通过 Redis 队列与 `outbound_queue`/`cmd_log` 协同，实现可靠下行与 ACK 处理；  
  - 不关心上游业务语义，仅保证“命令已下发 / 已确认 / 失败”的技术状态。  
- 事件投递  
  - 构造标准化事件（订单/设备/端口），写入事件队列，由事件推送器异步推送到第三方；  
  - 不包含上游业务对事件的解释逻辑。

**非核心职责（上游业务 / 辅助模块）**

- 面向具体业务的计费、优惠、风控等规则；  
- 只读/调试控制台的 UI 与场景模拟逻辑；  
- 一次性迁移脚本、历史修复 SQL、专用诊断工具。

### 2. 模块划分和数据流

- **API 层（internal/api）**  
  - 第三方 API（`thirdparty_handler.go`）：收敛 HTTP 请求，调用 `SessionManager` 与 `CoreRepo`，不直接写 SQL；  
  - 内部控制台 API（`testconsole_handler.go`）：复用第三方处理器能力，但限定为调试用途；  
  - 只读 API（`readonly_handler.go`）：暴露查询能力，不修改核心状态。
- **协议适配层（internal/protocol）**  
  - BKV/GN/AP3000 处理器负责：  
    - 解析协议帧 → 得到结构化事件（心跳、端口状态、充电结束等）；  
    - 调用 `CoreRepo` 和一致性任务接口更新 DB 快照与订单；  
    - 通过 Outbound 适配器发送协议级 ACK 或控制命令。  
  - 不负责：  
    - 上游业务决策（是否允许某种充电策略）；  
    - 订单金额结算规则；  
    - 会话策略配置（由 `SessionManager` 控制）。
- **核心模块**  
  - `SessionManager`（internal/session）：维护 TCP 会话与在线状态，是设备在线真相源；  
  - `CoreRepo`（internal/storage/core_repo.go + gormrepo）：封装 DB 操作，提供 DB-agnostic 核心存储接口；  
  - 一致性任务（`PortStatusSyncer` / `OrderMonitor` / `EventPusher`）：基于上述抽象定期收敛状态、推送事件。

### 3. 协议适配模式

- 协议入口由 Gateway 层根据前缀选择对应 Handler（BKV/GN/AP3000）；  
- 每个协议 Handler：  
  - 实现解析与验证（帧边界、CRC、字段合法性）；  
  - 将协议事件映射为“核心动作”：更新设备心跳、更新端口状态、推进订单状态机、写入命令日志；  
  - 对于需要 ACK 的命令，通过 Outbound 适配器构造并发送协议应答。
- 适配层不得：  
  - 直接访问 `pgxpool.Pool` 执行 SQL；  
  - 引入与特定第三方平台强耦合的业务逻辑；  
  - 绕过 `SessionManager` 判断在线状态。

### 4. 冗余/错误代码的处理原则

- 所有“只为一次事故添加且与当前架构/规范冲突”的补丁代码必须：  
  - 要么提升为有文档支持的正式特性（迁移到核心模块或监控层）；  
  - 要么退出主执行路径，仅保留为脚本或运维工具。  
- 直接违反以下约束的代码被视为不合理，应优先清理：  
  - 在线状态依赖 `devices.online` 或任意 DB 布尔字段；  
  - 端口状态使用“0/1/2 等业务枚举”而非 BKV 位图；  
  - 协议 Handler 直接拼接 SQL 或管理事务。

### 5. 验证与回归建议

- 任何针对中间件核心的结构重构，应至少通过：  
  - 单元测试：协议 Handler、API Handler、一致性任务的行为不变；  
  - 集成测试：订单生命周期、端口收敛、自愈逻辑与既有 E2E 场景一致；  
  - `go test ./...` 全量通过。  
- 对外行为变化必须在 OpenSpec 中标记为 BREAKING，并在 CHANGELOG 中说明。

