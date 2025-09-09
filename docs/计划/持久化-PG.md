## 持久化（PG）· 实施清单

### TODO

- [ ] 迁移脚本：核心表、日分区模板、索引、角色/权限
- [ ] 批量写通道（COPY/多值 insert）与重试退避策略
- [ ] outbound_queue：status/next_ts/tries、冷启续发、dead 清理
- [ ] VACUUM/ANALYZE/备份与恢复演练计划

### 验收

- 迁移幂等；基准写入达标；恢复演练通过

### Owner

- 待指派（数据负责人）

### 依赖

- 技术栈治理 [TS-DB]/[TS-Migrate]、领域服务用例
