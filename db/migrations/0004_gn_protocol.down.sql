-- Rollback GN Protocol Tables

-- Drop GN-specific tables
DROP TABLE IF EXISTS reasons_map;
DROP TABLE IF EXISTS params_pending;
DROP TABLE IF EXISTS inbound_logs;

-- Remove GN-specific indexes from outbound_queue
DROP INDEX IF EXISTS uk_outbound_queue_device_seq;
DROP INDEX IF EXISTS idx_outbound_queue_worker_scan;

-- Remove GN-specific columns from outbound_queue
ALTER TABLE IF EXISTS outbound_queue
  DROP COLUMN IF EXISTS seq,
  DROP COLUMN IF EXISTS next_ts,
  DROP COLUMN IF EXISTS tries;

-- Remove GN-specific constraints from ports
ALTER TABLE ports DROP CONSTRAINT IF EXISTS uk_ports_device_port;

-- Remove GN-specific columns from ports
ALTER TABLE IF EXISTS ports
  DROP COLUMN IF EXISTS status_bits,
  DROP COLUMN IF EXISTS biz_no,
  DROP COLUMN IF EXISTS voltage,
  DROP COLUMN IF EXISTS current,
  DROP COLUMN IF EXISTS power,
  DROP COLUMN IF EXISTS energy,
  DROP COLUMN IF EXISTS duration;

-- Remove GN-specific indexes and columns from devices
DROP INDEX IF EXISTS idx_devices_gateway_id;

ALTER TABLE IF EXISTS devices
  DROP COLUMN IF EXISTS gateway_id,
  DROP COLUMN IF EXISTS rssi,
  DROP COLUMN IF EXISTS fw_ver;