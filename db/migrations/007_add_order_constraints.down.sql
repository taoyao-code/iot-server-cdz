-- 007_add_order_constraints.down.sql
-- Rollback order constraints migration

DROP INDEX IF EXISTS idx_orders_device_port_active;
ALTER TABLE orders DROP COLUMN IF EXISTS status_history;
DROP INDEX IF EXISTS idx_ports_device_status;
DROP TABLE IF EXISTS outbound_dead_letters;
