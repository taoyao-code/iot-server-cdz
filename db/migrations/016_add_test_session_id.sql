-- Migration: 添加E2E测试会话ID字段到多个表
-- Purpose: 支持E2E测试数据隔离和追踪
-- Date: 2025-01-18

BEGIN;

-- 1. events表：添加test_session_id字段
ALTER TABLE events
ADD COLUMN IF NOT EXISTS test_session_id TEXT;

-- 为test_session_id创建索引（提升E2E测试查询性能）
CREATE INDEX IF NOT EXISTS idx_events_test_session
ON events(test_session_id) WHERE test_session_id IS NOT NULL;

COMMENT ON COLUMN events.test_session_id IS 'E2E测试会话标识，用于隔离测试数据';


-- 2. cmd_log表：添加test_session_id字段
ALTER TABLE cmd_log
ADD COLUMN IF NOT EXISTS test_session_id TEXT;

CREATE INDEX IF NOT EXISTS idx_cmd_log_test_session
ON cmd_log(test_session_id) WHERE test_session_id IS NOT NULL;

COMMENT ON COLUMN cmd_log.test_session_id IS 'E2E测试会话标识，用于隔离测试数据';


-- 3. outbound_queue表：添加test_session_id字段
ALTER TABLE outbound_queue
ADD COLUMN IF NOT EXISTS test_session_id TEXT;

CREATE INDEX IF NOT EXISTS idx_outbound_queue_test_session
ON outbound_queue(test_session_id) WHERE test_session_id IS NOT NULL;

COMMENT ON COLUMN outbound_queue.test_session_id IS 'E2E测试会话标识，用于隔离测试数据';


-- 验证添加成功
DO $$
DECLARE
    events_count INT;
    cmd_log_count INT;
    outbound_count INT;
BEGIN
    SELECT COUNT(*) INTO events_count
    FROM information_schema.columns
    WHERE table_name='events' AND column_name='test_session_id';

    SELECT COUNT(*) INTO cmd_log_count
    FROM information_schema.columns
    WHERE table_name='cmd_log' AND column_name='test_session_id';

    SELECT COUNT(*) INTO outbound_count
    FROM information_schema.columns
    WHERE table_name='outbound_queue' AND column_name='test_session_id';

    IF events_count = 0 OR cmd_log_count = 0 OR outbound_count = 0 THEN
        RAISE EXCEPTION 'test_session_id字段添加失败';
    END IF;

    RAISE NOTICE 'test_session_id字段添加成功: events=%, cmd_log=%, outbound_queue=%',
                  events_count, cmd_log_count, outbound_count;
END $$;

COMMIT;
