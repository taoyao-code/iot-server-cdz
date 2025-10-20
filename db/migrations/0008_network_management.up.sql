-- Week 6: 组网管理功能
-- 创建网关插座管理表

CREATE TABLE IF NOT EXISTS gateway_sockets (
    id SERIAL PRIMARY KEY,
    gateway_id VARCHAR(50) NOT NULL,
    socket_no SMALLINT NOT NULL,
    socket_mac VARCHAR(20) NOT NULL,
    socket_uid VARCHAR(20),
    channel SMALLINT,
    status SMALLINT DEFAULT 0,  -- 0=离线, 1=在线, 2=故障
    signal_strength SMALLINT,   -- 信号强度
    last_seen_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(gateway_id, socket_no)
);

-- 索引优化
CREATE INDEX idx_gateway_sockets_gateway ON gateway_sockets(gateway_id);
CREATE INDEX idx_gateway_sockets_mac ON gateway_sockets(socket_mac);
CREATE INDEX idx_gateway_sockets_status ON gateway_sockets(gateway_id, status);

-- 注释
COMMENT ON TABLE gateway_sockets IS 'BKV网关插座管理表';
COMMENT ON COLUMN gateway_sockets.gateway_id IS '网关物理ID';
COMMENT ON COLUMN gateway_sockets.socket_no IS '插座编号(1-250)';
COMMENT ON COLUMN gateway_sockets.socket_mac IS '插座MAC地址';
COMMENT ON COLUMN gateway_sockets.socket_uid IS '插座唯一标识';
COMMENT ON COLUMN gateway_sockets.channel IS '信道(1-15)';
COMMENT ON COLUMN gateway_sockets.status IS '状态: 0=离线, 1=在线, 2=故障';
COMMENT ON COLUMN gateway_sockets.signal_strength IS '信号强度(RSSI)';

