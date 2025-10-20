-- Week 7: OTA升级功能
-- 创建OTA任务管理表

CREATE TABLE IF NOT EXISTS ota_tasks (
    id SERIAL PRIMARY KEY,
    device_id BIGINT REFERENCES devices(id),
    target_type SMALLINT NOT NULL,  -- 1=DTU, 2=Socket
    target_socket_no SMALLINT,      -- 如果是插座升级，记录插座编号
    firmware_version VARCHAR(20) NOT NULL,
    ftp_server VARCHAR(50) NOT NULL,
    ftp_port INTEGER NOT NULL DEFAULT 21,
    file_name VARCHAR(50) NOT NULL,
    file_size BIGINT,               -- 文件大小(字节)
    status SMALLINT DEFAULT 0,      -- 0=待发送, 1=已下发, 2=升级中, 3=成功, 4=失败
    progress SMALLINT DEFAULT 0,    -- 升级进度 0-100
    error_msg TEXT,
    msg_id INTEGER,                 -- 下发消息ID
    started_at TIMESTAMPTZ,         -- 开始升级时间
    completed_at TIMESTAMPTZ,       -- 完成时间
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- 索引优化
CREATE INDEX idx_ota_tasks_device ON ota_tasks(device_id);
CREATE INDEX idx_ota_tasks_status ON ota_tasks(status);
CREATE INDEX idx_ota_tasks_created ON ota_tasks(created_at DESC);

-- 注释
COMMENT ON TABLE ota_tasks IS 'BKV设备OTA升级任务表';
COMMENT ON COLUMN ota_tasks.target_type IS '升级目标类型: 1=DTU, 2=插座';
COMMENT ON COLUMN ota_tasks.target_socket_no IS '插座升级时的插座编号';
COMMENT ON COLUMN ota_tasks.status IS '状态: 0=待发送, 1=已下发, 2=升级中, 3=成功, 4=失败';
COMMENT ON COLUMN ota_tasks.progress IS '升级进度百分比 0-100';

