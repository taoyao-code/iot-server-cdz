## 1. 代码与测试清理（已部分完成）

- [x] 搜索并移除所有对 `devices.online` 的代码依赖（查询、更新、模型字段）。
- [x] 更新单元测试中通过 `online` 字段模拟离线的逻辑，仅依赖 `last_seen_at`。
- [ ] 检查脚本和工具（如运维 SQL、诊断脚本）是否使用 `devices.online`，必要时改为基于 `last_seen_at`。

## 2. Schema 变更与迁移

- [ ] 在新的迁移文件中从 `devices` 表删除 `online` 列，并保证向后滚动安全方案（如仅在列存在时删除）。
- [ ] 更新 `db/migrations/full_schema.sql`，移除 `online` 列定义及相关注释。
- [ ] 如有必要，更新 `db/public.sql` 或导出的参考 Schema，使其与迁移后的结构一致。

## 3. 规范与文档更新

- [ ] 在 `consistency-lifecycle` 相关规范中补充说明：设备在线状态以 `SessionManager` 为真相源，DB 仅保留 `last_seen_at` 缓存。
- [ ] 更新内部架构文档中关于设备在线状态的描述，删除对 `devices.online` 的引用。

## 4. 验证与发布

- [ ] 运行全部单元测试与集成测试，确认删除列后无运行时错误或 SQL 失败。
- [ ] 在测试环境执行迁移，验证旧数据上的行为与线上监控指标无异常。
- [ ] 更新发布说明，标记为 Schema 级变更，并给出外部报表/查询的迁移建议。

