## 1. 建模阶段（GORM 模型）

- [x] 1.1 在 `internal/storage/models` 定义 `Device`/`Port`/`Order`/`CmdLog`/`OutboundMessage` 模型：
  - 字段命名符合 GORM 约定（ID/CreatedAt/UpdatedAt）；
  - 列名通过 `gorm:"column:xxx"` 固定，与现有 schema 兼容；
  - 不使用方言特有类型（例如 `jsonb`），统一使用 GORM 支持的基础类型。
- [ ] 1.2 使用 `AutoMigrate` 在本地创建测试数据库，验证模型生成的结构与当前 SQL 语义一致（主键/唯一索引/外键）。

## 2. 仓储抽象阶段（CoreRepo）

- [x] 2.1 在 `internal/storage` 定义精简的 `CoreRepo` 接口，覆盖中间件核心能力：
  - 设备：EnsureDevice / GetDeviceByPhyID
  - 端口：UpsertPortState / ListPortsByPhyID
  - 订单：CreateOrder / GetPendingOrderByPort / GetChargingOrderByPort / UpdateOrderStatus / SettleOrder
  - 队列/日志：EnqueueOutbound / InsertCmdLog
- [ ] 2.2 将 `ThirdPartyHandler` / BKV `Handlers` / `PortStatusSyncer` 等核心模块改为依赖 `CoreRepo` 接口，而不是直接依赖 `pgstorage.Repository` 或 `*pgxpool.Pool`。

## 3. GORM 实现阶段

- [x] 3.1 实现基于 GORM 的 `CoreRepo`：
  - 使用事务包裹跨表操作（例如创建订单 + 入队下行指令）；
  - 使用 `OnConflict` / 复合主键实现端口状态 upsert；
  - 保证所有查询使用 GORM 表达，不再拼接 SQL。
- [x] 3.2 首先迁移“启动充电”主链路：
  - StartCharge 中的设备/端口/订单/队列访问全部改为调用 `CoreRepo`（GORM 实现）；
  - BKV 控制 ACK / 充电结束上报中的订单结算和端口收敛使用 `CoreRepo`；
  - PortStatusSyncer 中对 `ports`/`orders` 的一致性检查改用 `CoreRepo`。

## 4. 去 SQL 阶段（核心路径）

- [ ] 4.1 标记并删除与核心路径重叠的原生 SQL：
  - `internal/storage/pg` 中对应的方法（例如 SettleOrder/UpsertPortState 等）的 SQL；
  - `internal/api` 和 `internal/app` 中直接调用 `Pool.Exec/Query/QueryRow` 的语句（若已被 CoreRepo 覆盖）。
- [ ] 4.2 确保在核心路径（StartCharge → 控制 ACK → 结束上报 → 一致性任务）中不再新增任何原生 SQL：
  - 在 Code Review 规则中明确“核心路径必须通过 CoreRepo + GORM 访问数据库”；
  - 只有非核心的诊断/一次性维护脚本可以保留方言 SQL（长期目标是全部迁移或移除）。

## 5. 验证与回归

- [ ] 5.1 在 GORM 实现和旧 pg 实现之间运行相同的集成测试套件，对比行为（订单状态、端口状态、事件推送）。
- [ ] 5.2 在测试环境启用 GORM CoreRepo，观察一段时间确认无异常后，再在生产环境切换。
- [ ] 5.3 清理所有不再引用的 pg 专用仓储代码，确保代码树中不再出现核心路径相关的 SQL 方言片段。
