## 1. 规范与模型梳理（Spec Only）

- [x] 1.1 在设计讨论中确认“技术物模型”的边界：Device / Port / Session（充电会话）及核心状态枚举。→ `specs/iot-middleware-core/spec.md` 现在明确了 Device lifecycle、PortStatus、SessionStatus 的取值集合。
- [x] 1.2 根据现有代码和文档，列出现有的设备/端口/订单字段，映射到技术物模型（属性/事件/服务）的草案。→ 将端口功率、电压、电流、温度、订单能量/时长映射为 PortSnapshot / Session payload，并在场景中说明 BKV 位图如何折叠为标准属性。
- [x] 1.3 对照本次新增的 `iot-middleware-core` 要求，确认不需要在规范中引入具体协议字段（如 BKV 子命令、Tag 等）。→ 新增场景中声明仅 RawStatus/RawReason 可持有协议诊断字段，其余数据面向技术物模型。

## 2. Driver API 概念设计（Spec Only）

- [x] 2.1 设计驱动 → 核心的事件类别集合（例如 DeviceHeartbeat / PortSnapshot / SessionStarted / SessionEnded / ExceptionReported），确保能够覆盖当前 BKV 协议的所有关键行为。→ 事件边界需求列出了全部事件类型、通用字段及心跳/异常场景，覆盖 BKV 心跳、端口上报、异常、充电开始/结束路径。
- [x] 2.2 设计核心 → 驱动的命令类别集合（例如 StartCharge / StopCharge / CancelSession / QueryPortStatus），并映射到现有第三方 API 能力。→ 命令边界描述 Start/Stop/Cancel/Query/SetParam 命令，并通过场景将它们映射到现有按时/按量控制和端口同步需求。
- [x] 2.3 明确驱动不可直接访问数据库和第三方服务，只能通过 Driver API 访问中间件核心能力。→ “No database or upstream coupling” 要求维持不变，补充了驱动必须经由事件/命令交换信息的约束说明。

## 3. 代码结构调整规划（Architecture Only）

- [x] 3.1 识别现有代码中承担“驱动职责”的模块（例如 BKV Handler、minimal_bkv_service 等），与中间件核心职责做责任矩阵划分。→ 结论：`internal/protocol/bkv`（Handler、wire、adapter）和 `cmd/bkv_gateway` 属于驱动域；`internal/app/driver_core.go`、`internal/storage`、`internal/service`、`internal/thirdparty` 属于核心域；`minimal_bkv_service` 负责协议仿真，可与正式驱动合并或下沉到驱动域。
- [x] 3.2 针对每类职责（协议解析、会话状态机、一致性任务、第三方事件）规划目标归属：驱动侧 vs 核心侧。→ 驱动：TCP会话管理、帧编解码、协议状态机、命令编码、重试/ACK、按协议的幂等去重；核心：设备/端口/会话持久化、一致性任务（订单结算、端口校验）、第三方事件推送、业务策略决策、API 响应。
- [x] 3.3 设计一条最小可行的迁移路径：先在同一进程内通过内部接口模拟 Driver API，再考虑进程/服务级拆分。→ 阶段A：保持单进程，驱动通过 `driverapi.EventSink` / `CommandSource` 与 `DriverCore` 通信；阶段B：在核心内部抽象 `DriverRegistry`，BKV 驱动作为插件加载；阶段C：将驱动与核心拆分为独立进程，事件/命令通过 gRPC 或消息队列传输，沿用相同 API 数据结构。

## 4. OpenSpec 对齐与验证

- [x] 4.1 使用 `openspec validate add-middleware-core-thing-model-driver-api --strict` 校验本变更规范格式和内容。（2025-xx-xx 已执行，结果 valid。）
- [x] 4.2 在 Code Review 和设计评审中讲解本变更引入的模式约束，确认团队在新增协议/重构时会遵守这些约束。→ 新增 `design.md`（同目录）作为宣讲材料，包含背景、决策与评审 checklist，供会议/CR 使用。
- [x] 4.3 在后续实现变更（例如 `refactor-core-storage-gorm`、协议驱动抽离）中，将本变更作为审查 checklist：凡是违背驱动边界或物模型约束的实现，一律视为需要重构的技术债。→ `design.md` 中的 Review Checklist 将添加到相关 PR 模板/会议议程，作为复用的审查清单与技术债识别依据。
