-- 007_add_order_constraints.up.sql
-- 修复订单状态竞态条件，防止同一端口并发创建多个活跃订单

-- 1. 防止同一端口同时有多个活跃订单（pending/charging/cancelling等）
-- 使用部分唯一索引，仅对活跃状态生效
CREATE UNIQUE INDEX IF NOT EXISTS idx_orders_device_port_active 
ON orders(device_id, port_no) 
WHERE status IN (0, 1, 2, 8, 9, 10);

COMMENT ON INDEX idx_orders_device_port_active IS 
'Prevent concurrent active orders on the same port: 0=pending,1=confirmed,2=charging,8=cancelling,9=stopping,10=interrupted';

-- 2. 订单状态流转审计（可选，用于调试状态机问题）
ALTER TABLE orders ADD COLUMN IF NOT EXISTS status_history jsonb DEFAULT '[]';
COMMENT ON COLUMN orders.status_history IS 
'Track order status transitions: [{status: 0, timestamp: "...", reason: "..."}, ...]';

-- 3. 端口状态快照索引（优化status查询）
CREATE INDEX IF NOT EXISTS idx_ports_device_status 
ON ports(device_id, status, updated_at DESC);

-- 4. 死信队列持久化表（Redis数据丢失时的兜底）
CREATE TABLE IF NOT EXISTS outbound_dead_letters (
    id BIGSERIAL PRIMARY KEY,
    message_id VARCHAR(255) NOT NULL,
    device_id BIGINT NOT NULL,
    phy_id VARCHAR(255) NOT NULL,
    command BYTEA NOT NULL,
    priority INT NOT NULL,
    retries INT NOT NULL,
    last_error TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    failed_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_dead_letters_device 
ON outbound_dead_letters(device_id, failed_at DESC);

CREATE INDEX IF NOT EXISTS idx_dead_letters_phy_id 
ON outbound_dead_letters(phy_id, failed_at DESC);

COMMENT ON TABLE outbound_dead_letters IS 
'Persist dead-letter queue messages for debugging and recovery';
