-- outbound_queue 由 0002_outbox.down.sql 删除
DROP INDEX IF EXISTS idx_cmdlog_msg;
DROP INDEX IF EXISTS idx_cmdlog_device_time;
DROP TABLE IF EXISTS cmd_log;
DROP INDEX IF EXISTS idx_orders_time;
DROP INDEX IF EXISTS idx_orders_device_port;
DROP TABLE IF EXISTS orders;
DROP TABLE IF EXISTS ports;
DROP INDEX IF EXISTS idx_devices_gateway_id;
DROP INDEX IF EXISTS idx_devices_last_seen_at;
DROP TABLE IF EXISTS devices;


