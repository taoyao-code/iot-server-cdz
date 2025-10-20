-- P0修复: BKV设备参数持久化存储
-- 解决问题: 参数存储使用内存map，重启后数据丢失

CREATE TABLE IF NOT EXISTS device_params (
    id SERIAL PRIMARY KEY,
    device_id BIGINT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    param_id INT NOT NULL,
    param_value BYTEA,
    msg_id INT,
    status INT NOT NULL DEFAULT 0,  -- 0=待确认, 1=已确认, 2=失败
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    confirmed_at TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    error_message TEXT,
    UNIQUE(device_id, param_id)
);

-- 索引：查询待确认参数
CREATE INDEX idx_device_params_pending 
    ON device_params(device_id, param_id) 
    WHERE status = 0;

-- 索引：按设备查询
CREATE INDEX idx_device_params_device 
    ON device_params(device_id);

-- 索引：按更新时间查询（用于清理过期记录）
CREATE INDEX idx_device_params_updated 
    ON device_params(updated_at);

-- 注释
COMMENT ON TABLE device_params IS 'BKV设备参数写入记录（P0修复：持久化存储）';
COMMENT ON COLUMN device_params.device_id IS '设备ID（外键）';
COMMENT ON COLUMN device_params.param_id IS '参数ID（BKV协议定义）';
COMMENT ON COLUMN device_params.param_value IS '参数值（二进制）';
COMMENT ON COLUMN device_params.msg_id IS '消息ID（用于ACK确认）';
COMMENT ON COLUMN device_params.status IS '状态：0=待确认, 1=已确认, 2=失败';
COMMENT ON COLUMN device_params.created_at IS '创建时间';
COMMENT ON COLUMN device_params.confirmed_at IS '确认时间';
COMMENT ON COLUMN device_params.updated_at IS '更新时间';
COMMENT ON COLUMN device_params.error_message IS '错误信息（失败时记录）';
