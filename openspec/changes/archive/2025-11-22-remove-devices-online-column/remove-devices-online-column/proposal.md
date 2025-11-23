# Change: 移除 devices.online 冗余在线状态字段

## Why

- 生产代码已经统一以 `SessionManager.IsOnline` 作为设备在线状态的单一真相源，数据库中的 `devices.online` 字段长期未被可靠维护，存在语义误导和状态不一致风险。
- 当前与设备在线相关的逻辑只依赖 Redis 会话和 `last_seen_at` 缓存时间戳，保留 `devices.online` 只会增加维护成本和误用可能。

## What Changes

- 从数据库 Schema 中删除 `devices.online` 字段，并更新全量 Schema（`full_schema.sql`）及历史迁移。
- 确认所有代码路径不再读取或写入 `devices.online`，包括测试用例和维护脚本。
- 在一致性规范中明确设备在线状态的数据来源：Redis 会话 + `last_seen_at` 缓存，数据库不再存储额外布尔在线字段。

## Impact

- 影响表：`devices`（删除一列），需要运行迁移脚本。
- 影响规范：`consistency-lifecycle` 能力中关于“设备在线状态”的描述需要强调 DB 不再有 `online` 布尔列。
- 向后兼容性：依赖旧列的外部查询或报表需要改为基于 `last_seen_at` 或 API 的 `online` 字段。

