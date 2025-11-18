-- E2E测试控制台: test_session_id贯穿链路
-- 将测试会话ID落地到核心表，便于时间线查询

ALTER TABLE IF EXISTS orders
    ADD COLUMN IF NOT EXISTS test_session_id TEXT;
CREATE INDEX IF NOT EXISTS idx_orders_test_session
    ON orders(test_session_id) WHERE test_session_id IS NOT NULL;

ALTER TABLE IF EXISTS cmd_log
    ADD COLUMN IF NOT EXISTS test_session_id TEXT;
CREATE INDEX IF NOT EXISTS idx_cmd_log_test_session
    ON cmd_log(test_session_id) WHERE test_session_id IS NOT NULL;

ALTER TABLE IF EXISTS outbound_queue
    ADD COLUMN IF NOT EXISTS test_session_id TEXT;
CREATE INDEX IF NOT EXISTS idx_outbound_queue_test_session
    ON outbound_queue(test_session_id) WHERE test_session_id IS NOT NULL;

ALTER TABLE IF EXISTS events
    ADD COLUMN IF NOT EXISTS test_session_id TEXT;
CREATE INDEX IF NOT EXISTS idx_events_test_session
    ON events(test_session_id) WHERE test_session_id IS NOT NULL;

INSERT INTO schema_migrations (version, applied_at)
VALUES (13, NOW())
ON CONFLICT (version) DO NOTHING;
