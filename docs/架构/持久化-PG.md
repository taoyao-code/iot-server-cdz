## 持久化（PostgreSQL）设计

### 1. 目标

- 支撑高并发写入与审计可回放；分区治理、批量写、下行WAL、索引可控。

### 2. 表与分区

- 日分区：`cmd_log_YYYYMMDD`、`telemetry_summary_YYYYMMDD`；旧分区直接 DROP。
- 热点表：`orders`、`outbound_queue`、`sessions`；复合索引 `(phy_id, cmd, ts)` 与 BRIN(ts)。

### 3. 写入策略

- 批量：COPY/多值 INSERT；逻辑批≤1MB；重试退避。
- 事务：短事务；避免跨多表长事务；按用例提交。

### 4. 下行 WAL

- 队列：`outbound_queue(status,next_ts,tries)`；完成即清理；dead 队列审计。
- 冷启：按 `next_ts` 扫描续发；每设备顺序保障。

### 5. 维护与备份

- VACUUM/ANALYZE 周期；归档与TTL；备份/恢复演练。

### 6. 指标与验收

- 指标：写入失败率、p95 写延迟、分区大小/数量、WAL 积压。
- 验收：基准写入达标；迁移脚本幂等；恢复验证通过。

### 7. 依赖标签引用（详见 `docs/技术栈治理.md`）

- [TS-DB]、[TS-Migrate]

### 8. 配置项

- `pg.dsn`、`pg.max_open`、`pg.max_idle`、`pg.conn_max_lifetime`
- `pg.partitions.keep_days`（日志/摘要保留天数）
- `pg.batch.max_rows`、`pg.batch.max_bytes`
- `wal.scan_interval_sec`、`wal.dead_retention_days`

### 9. 运维要点

- 定期 VACUUM/ANALYZE；检查膨胀与热点索引；监控复制与磁盘空间。
- 归档与压缩策略；冷备与恢复演练（季度）。

### 10. 验证方法

- 压测写入/查询；分区轮转脚本演练；宕机恢复与WAL续发演练。
