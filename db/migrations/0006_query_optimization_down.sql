-- Week2: 回滚数据库查询优化

DROP INDEX CONCURRENTLY IF EXISTS idx_devices_last_seen;
DROP INDEX CONCURRENTLY IF EXISTS idx_orders_phy_created;
DROP INDEX CONCURRENTLY IF EXISTS idx_cmd_logs_device_created;
DROP INDEX CONCURRENTLY IF EXISTS idx_outbound_status_priority;
DROP INDEX CONCURRENTLY IF EXISTS idx_ports_device_no;
DROP INDEX CONCURRENTLY IF EXISTS idx_orders_hex;
