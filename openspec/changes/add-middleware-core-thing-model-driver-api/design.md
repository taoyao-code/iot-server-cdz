## Context

`iot-server` 同时承担协议驱动与中间件核心的职责，导致：

- BKV Handler 直接访问数据库/订单状态机，与核心职责耦合；
- 协议特有字段渗透至核心 API，阻碍扩展其他协议；
- 驱动与核心缺乏明确的接口契约，经验依赖导致评审难以统一标准。

本设计文档用于在 Code Review / 设计评审中讲解新增规范（任务 4.2），并沉淀可复用的检查清单（任务 4.3）。

## Goals / Non-Goals

- **Goals**
  - 传达技术物模型（Device / Port / Session）及事件/命令边界；
  - 给出 Driver ↔ Core 职责矩阵与最小迁移路径；
  - 制定评审时的 checklist，用于识别违反边界的实现。
- **Non-Goals**
  - 不在此文档中定义具体 HTTP API、Go 类型或 DB schema；
  - 不覆盖协议实现细节（TCP 粘包、帧解析等驱动内部实现）。

## Decisions

1. **Thing Model 与状态枚举**
   - Device：维护 `online/offline/maintenance/decommissioned` 生命周期，记录产品、实体连接等信息。
   - Port：统一 `status`（unknown/offline/idle/charging/fault）及测量属性 `power_w/current_ma/voltage_v/temperature_c`，BKV 位图仅映射到 `RawStatus` 供诊断。
   - Session：维护 `SessionStatus`（pending/charging/stopping/completed/cancelled/interrupted）、`BusinessNo`、`energy_kwh01`、`duration_sec`。

2. **Driver → Core 事件集合**
   - `DeviceHeartbeat`: 用于可用性与 last_seen。
   - `PortSnapshot`: 端口实时状态 & 测量。
   - `SessionStarted` / `SessionProgress` / `SessionEnded`: 会话生命周期。
   - `ExceptionReported`: 协议异常或硬件告警。
   - 每个事件都必须包含 `DeviceID`、`OccurredAt` 以及对应载荷；协议 Tag/子命令仅能出现在 `RawStatus`/`RawReason` 字段。

3. **Core → Driver 命令集合**
   - `StartCharge`, `StopCharge`, `CancelSession`, `QueryPortStatus`, `SetParam`。
   - 命令仅携带设备/端口/会话标识与业务意图参数；协议编码、重试由驱动负责。

4. **Driver / Core 职责矩阵**

| 职责 | Driver | Core |
| --- | --- | --- |
| TCP 连接管理 | ✅ | ❌ |
| 协议帧解析/编码、去重、ACK | ✅ | ❌ |
| 设备/端口/会话持久化 | ❌（通过事件） | ✅ |
| 订单状态机、结算、一致性任务 | ❌ | ✅ |
| 第三方事件推送/通知 | ❌ | ✅ |
| 业务策略（限功率、预约） | ❌ | ✅（通过命令参数） |
| 协议参数持久化与下发 | ✅ | ❌（仅发 `SetParam` 命令） |

5. **迁移路径**
   - **阶段A（当前）**：单进程内通过 `driverapi.EventSink` 和未来的 `CommandSource` 完成内存调用。
   - **阶段B**：引入 `DriverRegistry` 插件化加载驱动，仍运行在同一进程但隔离模块依赖。
   - **阶段C**：驱动与核心拆分为独立进程，沿用相同事件/命令结构，通过 gRPC/消息队列传输。

## Review Checklist

在 Code Review / 设计评审中使用以下问题逐条确认：

1. **事件输入**
   - 协议处理逻辑是否仅通过 `driverapi.EventSink` 上报 `DeviceHeartbeat` / `PortSnapshot` / `Session*` / `ExceptionReported`？
   - 事件载荷是否只包含技术物模型字段？若携带协议特定值，是否放入 `RawStatus`/`RawReason`？
2. **命令输出**
   - 核心是否通过 `driverapi.CommandSource` 发出 Start/Stop/Cancel/Query/SetParam，而非直接构造协议帧？
   - 命令是否包含完整的 `DeviceID`/`PortNo`/`BusinessNo`，并且没有嵌入 socket/tag？
3. **驱动边界**
   - 驱动代码是否避免直接访问 `storage`、`service`、`thirdparty` 包？若需要数据，是否由核心通过事件/命令补充？
   - 是否避免在驱动中实现订单状态机、端口一致性任务或第三方推送？
4. **核心边界**
   - 核心是否避免引用协议包（如 `internal/protocol/bkv`），仅以技术物模型/Driver API 为依赖？
   - 新增协议支持是否只在驱动层扩展，而无需修改核心状态机 / API / DB schema？
5. **迁移任务**
   - 变更是否沿阶段A→B→C的路径实施？若直接进入更高阶段，是否提供风险评估和回退策略？

评审通过条件：上述所有问题均得到肯定（或给出明确的后续整改项），否则视为违反规范，需要在当前或后续迭代中修复。

## Adoption Plan

1. **宣讲**：在下一次架构/协议周会上分享本设计及 `spec.md`，并记录 Q&A（完成任务 4.2）。
2. **代码清单**：在相关 PR 模板中引用上文 checklist，并在 `refactor-core-storage-gorm`、驱动抽离等重构中要求逐项确认（完成任务 4.3）。
3. **治理**：将 checklist 同步到团队 wiki / README，并把违反条目登记为技术债任务，方便追踪。
