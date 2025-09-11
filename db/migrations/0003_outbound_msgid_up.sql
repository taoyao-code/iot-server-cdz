-- Add msg_id column for outbound_queue to correlate device ACK (by msgID echo)
ALTER TABLE IF EXISTS outbound_queue
  ADD COLUMN IF NOT EXISTS msg_id INT;

CREATE INDEX IF NOT EXISTS idx_outbound_queue_device_msg
  ON outbound_queue(device_id, msg_id) WHERE msg_id IS NOT NULL;


