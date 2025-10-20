-- 回滚 msg_id 字段
ALTER TABLE outbound_queue DROP COLUMN IF EXISTS msg_id;

