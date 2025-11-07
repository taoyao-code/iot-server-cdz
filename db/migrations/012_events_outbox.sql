-- P1-7修复: 事件推送Outbox模式
-- 创建events表用于可靠的事件推送

CREATE TABLE IF NOT EXISTS events (
    id BIGSERIAL PRIMARY KEY,
    order_no VARCHAR(32) NOT NULL,
    event_type VARCHAR(32) NOT NULL,  -- device.heartbeat, order.created, order.confirmed, charging.started, charging.ended, order.completed, etc.
    event_data JSONB NOT NULL,
    sequence_no INT NOT NULL,         -- 事件序列号（按订单递增）
    status INT NOT NULL DEFAULT 0,    -- 0:待推送, 1:已推送, 2:失败
    retry_count INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    pushed_at TIMESTAMPTZ,
    error_message TEXT,
    CONSTRAINT events_order_seq UNIQUE(order_no, sequence_no)
);

-- 索引优化
CREATE INDEX IF NOT EXISTS idx_events_status_created ON events(status, created_at) WHERE status IN (0, 2);
CREATE INDEX IF NOT EXISTS idx_events_order_no ON events(order_no);
CREATE INDEX IF NOT EXISTS idx_events_retry ON events(status, retry_count, created_at) WHERE status = 2 AND retry_count < 5;

-- 插入迁移记录
INSERT INTO schema_migrations (version, applied_at) VALUES (12, NOW())
ON CONFLICT (version) DO NOTHING;
