-- Week2: 数据库查询优化 - 添加索引
-- 目标: 将常见查询延迟降低10倍

-- 1. 设备最近心跳查询优化
-- 查询: SELECT * FROM devices WHERE last_seen_at > NOW() - INTERVAL '5 minutes'
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_devices_last_seen 
ON devices(last_seen_at DESC) 
WHERE last_seen_at IS NOT NULL;

COMMENT ON INDEX idx_devices_last_seen IS '优化在线设备查询';

-- 2. 订单查询优化（复合索引）
-- 查询: SELECT * FROM orders WHERE phy_id = 'DEV001' ORDER BY created_at DESC
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_orders_phy_created 
ON orders(phy_id, created_at DESC);

COMMENT ON INDEX idx_orders_phy_created IS '优化设备订单查询';

-- 3. 命令日志查询优化
-- 查询: SELECT * FROM cmd_logs WHERE device_id = 123 AND created_at BETWEEN ...
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_cmd_logs_device_created 
ON cmd_logs(device_id, created_at DESC);

COMMENT ON INDEX idx_cmd_logs_device_created IS '优化设备命令日志查询';

-- 4. 下行队列状态索引（如果使用PG队列）
-- 查询: SELECT * FROM outbound_queue WHERE status IN (0, 1) ORDER BY priority DESC
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_outbound_status_priority 
ON outbound_queue(status, priority DESC, created_at) 
WHERE status IN (0, 1);

COMMENT ON INDEX idx_outbound_status_priority IS '优化下行队列扫描';

-- 5. 端口状态查询优化
-- 查询: SELECT * FROM ports WHERE device_id = 123 AND port_no = 1
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ports_device_no 
ON ports(device_id, port_no);

COMMENT ON INDEX idx_ports_device_no IS '优化端口状态查询';

-- 6. 订单hex查询优化（用于重复订单检测）
-- 查询: SELECT * FROM orders WHERE order_hex = '...'
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_orders_hex 
ON orders(order_hex);

COMMENT ON INDEX idx_orders_hex IS '优化订单hex查询（防重）';
