# Change: 将核心存储层重构为 GORM 驱动并移除核心路径中的原生 SQL

## Why

- 当前核心读写路径（设备/端口/订单/指令队列）大量依赖 Postgres 方言 SQL 和 pgx，导致：
  - 无法平滑迁移到其他数据库（MySQL/SQLite 等）；
  - Schema 真实结构分散在多处 SQL 片段，难以维护；
  - 业务逻辑和 SQL 强耦合，难以识别哪些是真正的业务约束，哪些只是实现细节。
- 项目定位为 IoT 中间件（设备 ↔ IoT 服务 ↔ 第三方服务），应当：
  - 在存储层上保持数据库无关性（只依赖 GORM 模型和事务语义）；
  - 把核心状态（设备、端口、订单、指令队列）集中在一套清晰的数据模型中；
  - 允许未来按需替换底层数据库，而不修改业务层代码。

## What Changes

- 定义一组 GORM 模型作为唯一的结构真相源：
  - `Device` / `Port` / `Order` / `CmdLog` / `OutboundMessage`。
- 抽象出精简的 `CoreRepo` 接口，覆盖中间件核心能力：
  - 设备注册与查询、端口状态 upsert、订单生命周期、指令下行入队、指令/事件日志。
- 使用 GORM 实现 `CoreRepo`，逐步替换现有 pgx + 原生 SQL 的实现：
  - 从 StartCharge → BKV 控制 ACK/结束上报 → 端口/订单一致性检查这条主链路开始；
  - 保证所有核心路径只通过 `CoreRepo` 访问数据库。
- 在核心路径完全迁移到 GORM 后，删除对应模块中的原生 SQL 和对 `*pgxpool.Pool` 的直接依赖。

## Impact

- 核心模块（示例，不完整列举）：
  - `internal/api/thirdparty_handler.go`（StartCharge/StopCharge 等）；
  - `internal/protocol/bkv/handlers.go`（控制 ACK、充电结束上报、端口状态上报）；
  - `internal/app/port_status_syncer.go` / `internal/app/order_monitor.go` 等一致性任务。
- 存储层：
  - 新增 `internal/storage/models` 用于 GORM 模型；
  - 新增基于 GORM 的 `CoreRepo` 实现；
  - 逐步弱化并最终移除 `internal/storage/pg` 中与核心路径相关的原生 SQL。
- OpenSpec：
  - 在本变更下，为“存储核心”能力新增规范，要求核心路径完全依赖 GORM 模型和事务，不再引入新的原生 SQL。

