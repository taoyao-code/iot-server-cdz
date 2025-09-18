-- GN Protocol Tables for 组网设备
-- Based on requirements from PR2 problem statement

-- Update devices table for GN protocol support
-- Add columns for gateway_id, rssi, fw_ver
ALTER TABLE IF EXISTS devices 
  ADD COLUMN IF NOT EXISTS gateway_id TEXT,
  ADD COLUMN IF NOT EXISTS rssi INT,
  ADD COLUMN IF NOT EXISTS fw_ver TEXT;

-- Create index for gateway_id queries
CREATE INDEX IF NOT EXISTS idx_devices_gateway_id 
  ON devices(gateway_id) WHERE gateway_id IS NOT NULL;

-- Extend ports table for GN protocol fields
-- Add new fields for GN protocol port status
ALTER TABLE IF EXISTS ports
  ADD COLUMN IF NOT EXISTS status_bits INT,
  ADD COLUMN IF NOT EXISTS biz_no TEXT,
  ADD COLUMN IF NOT EXISTS voltage DECIMAL(10,3),
  ADD COLUMN IF NOT EXISTS current DECIMAL(10,3),
  ADD COLUMN IF NOT EXISTS power DECIMAL(10,3),
  ADD COLUMN IF NOT EXISTS energy DECIMAL(10,3),
  ADD COLUMN IF NOT EXISTS duration INT;

-- Create unique constraint for device_id + port_no (if not exists)
ALTER TABLE ports DROP CONSTRAINT IF EXISTS uk_ports_device_port;
ALTER TABLE ports ADD CONSTRAINT uk_ports_device_port 
  UNIQUE(device_id, port_no);

-- Inbound logs table for GN protocol
CREATE TABLE IF NOT EXISTS inbound_logs (
    id BIGSERIAL PRIMARY KEY,
    device_id BIGINT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    cmd INT NOT NULL,
    seq INT NOT NULL,
    payload_hex TEXT,
    parsed_ok BOOLEAN NOT NULL DEFAULT FALSE,
    reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_inbound_logs_device_time 
  ON inbound_logs(device_id, created_at DESC);

-- Modify outbound_queue for GN protocol support (add seq field)
ALTER TABLE IF EXISTS outbound_queue
  ADD COLUMN IF NOT EXISTS seq INT;

-- Add unique constraint for device_id + seq
CREATE UNIQUE INDEX IF NOT EXISTS uk_outbound_queue_device_seq
  ON outbound_queue(device_id, seq) WHERE seq IS NOT NULL;

-- Add index for status + next_ts (for worker scanning)
ALTER TABLE IF EXISTS outbound_queue
  ADD COLUMN IF NOT EXISTS next_ts TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_outbound_queue_worker_scan
  ON outbound_queue(status, next_ts) WHERE status IN (0, 1);

-- Add tries column (rename from retry_count for consistency)
ALTER TABLE IF EXISTS outbound_queue
  ADD COLUMN IF NOT EXISTS tries INT DEFAULT 0;

-- Parameters pending table for GN protocol
CREATE TABLE IF NOT EXISTS params_pending (
    id BIGSERIAL PRIMARY KEY,
    device_id BIGINT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    param_id INT NOT NULL,
    value TEXT NOT NULL,
    seq INT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_params_pending_device 
  ON params_pending(device_id);

-- Reasons map table for error code mapping
CREATE TABLE IF NOT EXISTS reasons_map (
    reason_code INT PRIMARY KEY,
    reason_name TEXT NOT NULL,
    category TEXT
);

-- Insert some default reason codes
INSERT INTO reasons_map (reason_code, reason_name, category) VALUES 
(0, 'Success', 'OK'),
(1, 'Invalid Command', 'ERROR'),
(2, 'Invalid Parameters', 'ERROR'),
(3, 'Device Busy', 'WARNING'),
(4, 'Timeout', 'ERROR'),
(5, 'Unknown Error', 'ERROR')
ON CONFLICT (reason_code) DO NOTHING;