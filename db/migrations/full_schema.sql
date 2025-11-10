-- =============================================
-- IoT Server 完整数据库Schema
-- 包含初始化 + 所有迁移的完整版本
-- =============================================

-- 设置时区
SET timezone = 'Asia/Shanghai';

-- 启用必要的扩展
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pg_stat_statements";

-- 重建schema_migrations表
DROP TABLE IF EXISTS schema_migrations CASCADE;
CREATE TABLE schema_migrations (
    version BIGINT PRIMARY KEY,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- devices: 设备基础信息
CREATE TABLE IF NOT EXISTS devices (
    id              BIGSERIAL PRIMARY KEY,
    phy_id          TEXT NOT NULL UNIQUE,
    gateway_id      TEXT,                -- 网关ID(GN协议使用)
    iccid           TEXT,
    imei            TEXT,
    model           TEXT,
    firmware_ver    TEXT,
    rssi            INTEGER,             -- 信号强度
    fw_ver          TEXT,                -- 固件版本(GN协议使用)
    online          BOOLEAN DEFAULT FALSE, -- 设备在线状态
    last_seen_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_devices_last_seen_at ON devices(last_seen_at);
CREATE INDEX IF NOT EXISTS idx_devices_gateway_id ON devices(gateway_id) WHERE gateway_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_devices_online ON devices(online) WHERE online = TRUE;

-- ports: 设备端口快照
CREATE TABLE IF NOT EXISTS ports (
    device_id       BIGINT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    port_no         INT NOT NULL,
    status          INT NOT NULL DEFAULT 0,
    power_w         INT,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY(device_id, port_no)
);

-- orders: 订单
CREATE TABLE IF NOT EXISTS orders (
    id              BIGSERIAL PRIMARY KEY,
    device_id       BIGINT NOT NULL REFERENCES devices(id) ON DELETE RESTRICT,
    port_no         INT NOT NULL,
    order_no        TEXT NOT NULL,
    order_hex       TEXT,        -- 订单hex字符串（用于防重）
    start_time      TIMESTAMPTZ,
    end_time        TIMESTAMPTZ,
    end_reason      INT,         -- 结束原因代码
    kwh_0p01        BIGINT,      -- 以 0.01kWh 为单位
    amount_cent     BIGINT,      -- 分
    status          INT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(order_no)
);
CREATE INDEX IF NOT EXISTS idx_orders_device_port ON orders(device_id, port_no);
CREATE INDEX IF NOT EXISTS idx_orders_time ON orders(start_time, end_time);
CREATE INDEX IF NOT EXISTS idx_orders_order_hex ON orders(order_hex) WHERE order_hex IS NOT NULL;

-- cmd_log: 指令日志（上下行）
CREATE TABLE IF NOT EXISTS cmd_log (
    id              BIGSERIAL PRIMARY KEY,
    device_id       BIGINT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    msg_id          INT,
    cmd             INT NOT NULL,
    direction       SMALLINT NOT NULL, -- 0=UP,1=DOWN
    payload         BYTEA,
    success         BOOLEAN,
    err_code        INT,
    duration_ms     INT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_cmdlog_device_time ON cmd_log(device_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_cmdlog_msg ON cmd_log(msg_id, cmd);

-- cmd_logs: 别名视图，兼容使用 cmd_logs 名称的代码
CREATE OR REPLACE VIEW cmd_logs AS SELECT * FROM cmd_log;

-- outbound_queue: 下行任务队列(由 0002_outbox.up.sql 创建)
-- 此处已移除,避免重复创建


-- Outbox for downlink commands
CREATE TABLE IF NOT EXISTS outbound_queue (
    id BIGSERIAL PRIMARY KEY,
    device_id BIGINT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    phy_id TEXT,
    port_no INT,
    cmd INT NOT NULL,
    payload BYTEA,
    priority INT NOT NULL DEFAULT 100,
    status INT NOT NULL DEFAULT 0,         -- 0=pending,1=sent,2=done,3=failed
    retry_count INT NOT NULL DEFAULT 0,
    retries INT NOT NULL DEFAULT 0,        -- 别名,兼容旧代码
    not_before TIMESTAMPTZ,
    timeout_sec INT NOT NULL DEFAULT 15,
    correlation_id TEXT,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_outbound_queue_status_notbefore
    ON outbound_queue(status, not_before, priority, created_at);

CREATE INDEX IF NOT EXISTS idx_outbound_queue_device
    ON outbound_queue(device_id);

CREATE UNIQUE INDEX IF NOT EXISTS uid_outbound_correlation
    ON outbound_queue(correlation_id) WHERE correlation_id IS NOT NULL;

-- trigger to update updated_at
CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = NOW();
  RETURN NEW;
END; $$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_outbound_updated_at ON outbound_queue;
CREATE TRIGGER trg_outbound_updated_at
BEFORE UPDATE ON outbound_queue
FOR EACH ROW EXECUTE PROCEDURE set_updated_at();

-- Add msg_id column for outbound_queue to correlate device ACK (by msgID echo)
ALTER TABLE IF EXISTS outbound_queue
  ADD COLUMN IF NOT EXISTS msg_id INT;

CREATE INDEX IF NOT EXISTS idx_outbound_queue_device_msg
  ON outbound_queue(device_id, msg_id) WHERE msg_id IS NOT NULL;


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
-- Week2: 数据库查询优化 - 添加索引
-- 目标: 将常见查询延迟降低10倍

-- 1. 设备最近心跳查询优化
-- 查询: SELECT * FROM devices WHERE last_seen_at > NOW() - INTERVAL '5 minutes'
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_devices_last_seen 
ON devices(last_seen_at DESC) 
WHERE last_seen_at IS NOT NULL;

COMMENT ON INDEX idx_devices_last_seen IS '优化在线设备查询';

-- 2. 订单查询优化（复合索引）
-- 注意: orders表没有phy_id列，而是通过device_id关联到devices.phy_id
-- 因此不需要此索引，已有的 idx_orders_device_port 索引足够
-- CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_orders_phy_created 
-- ON orders(phy_id, created_at DESC);
-- COMMENT ON INDEX idx_orders_phy_created IS '优化设备订单查询';

-- 3. 命令日志查询优化
-- 注意: 实际表名是cmd_log，不是cmd_logs（cmd_logs是视图）
-- 已在上面创建了 idx_cmdlog_device_time 索引，无需重复创建
-- CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_cmd_logs_device_created 
-- ON cmd_logs(device_id, created_at DESC);
-- COMMENT ON INDEX idx_cmd_logs_device_created IS '优化设备命令日志查询';

-- 4. 下行队列状态索引（如果使用PG队列）
-- 查询: SELECT * FROM outbound_queue WHERE status IN (0, 1) ORDER BY priority DESC
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_outbound_status_priority 
ON outbound_queue(status, priority DESC, created_at) 
WHERE status IN (0, 1);

COMMENT ON INDEX idx_outbound_status_priority IS '优化下行队列扫描';

-- 5. 端口状态查询优化
-- 查询: SELECT * FROM ports WHERE device_id = 123 AND port_no = 1
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ports_device_no 
ON ports(device_id, port_no);

COMMENT ON INDEX idx_ports_device_no IS '优化端口状态查询';

-- 6. 订单hex查询优化（用于重复订单检测）
-- 注意: order_hex列已在orders表创建时添加，并在上面创建了 idx_orders_order_hex 索引
-- 无需重复创建
-- CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_orders_hex 
-- ON orders(order_hex);
-- COMMENT ON INDEX idx_orders_hex IS '优化订单hex查询（防重）';
-- Week4: 刷卡充电系统数据库设计
-- 创建卡片表和交易表，支持刷卡充电业务

-- 卡片表：存储充电卡信息
CREATE TABLE IF NOT EXISTS cards (
    id BIGSERIAL PRIMARY KEY,
    card_no VARCHAR(32) UNIQUE NOT NULL,         -- 卡号（唯一）
    balance DECIMAL(10,2) DEFAULT 0 NOT NULL,    -- 余额（元）
    status VARCHAR(20) DEFAULT 'active' NOT NULL, -- 状态：active/inactive/blocked
    user_id BIGINT,                               -- 关联用户ID（预留）
    description TEXT,                             -- 备注信息
    created_at TIMESTAMP DEFAULT NOW() NOT NULL,
    updated_at TIMESTAMP DEFAULT NOW() NOT NULL
);

-- 卡片表索引
CREATE INDEX idx_cards_card_no ON cards(card_no);
CREATE INDEX idx_cards_status ON cards(status);
CREATE INDEX idx_cards_user_id ON cards(user_id);

-- 刷卡交易表：记录刷卡充电交易
CREATE TABLE IF NOT EXISTS card_transactions (
    id BIGSERIAL PRIMARY KEY,
    card_no VARCHAR(32) NOT NULL,                 -- 卡号
    device_id VARCHAR(64) NOT NULL,               -- 设备ID
    phy_id VARCHAR(16) NOT NULL,                  -- 物理ID
    order_no VARCHAR(64) UNIQUE NOT NULL,         -- 订单号（唯一）
    
    -- 充电模式：1=按时长, 2=按电量, 3=按功率, 4=充满自停
    charge_mode INT NOT NULL,
    
    -- 充电参数
    amount DECIMAL(10,2),                         -- 消费金额（元）
    duration_minutes INT,                         -- 充电时长（分钟）
    power_watts INT,                              -- 充电功率（瓦）
    energy_kwh DECIMAL(10,3),                     -- 充电电量（度）
    
    -- 订单状态：pending/charging/completed/cancelled/failed
    status VARCHAR(20) DEFAULT 'pending' NOT NULL,
    
    -- 时间记录
    start_time TIMESTAMP,                         -- 充电开始时间
    end_time TIMESTAMP,                           -- 充电结束时间
    created_at TIMESTAMP DEFAULT NOW() NOT NULL,  -- 订单创建时间
    updated_at TIMESTAMP DEFAULT NOW() NOT NULL,  -- 订单更新时间
    
    -- 失败原因（当status=failed时）
    failure_reason TEXT,
    
    -- 计费信息
    price_per_kwh DECIMAL(10,4),                  -- 电价（元/度）
    service_fee_rate DECIMAL(5,4),                -- 服务费率（0-1）
    total_amount DECIMAL(10,2),                   -- 实际消费金额（元）
    
    -- 外键约束（可选）
    CONSTRAINT fk_card_transactions_card FOREIGN KEY (card_no) REFERENCES cards(card_no) ON DELETE CASCADE
);

-- 交易表索引
CREATE INDEX idx_card_transactions_card_no ON card_transactions(card_no);
CREATE INDEX idx_card_transactions_device_id ON card_transactions(device_id);
CREATE INDEX idx_card_transactions_phy_id ON card_transactions(phy_id);
CREATE INDEX idx_card_transactions_order_no ON card_transactions(order_no);
CREATE INDEX idx_card_transactions_status ON card_transactions(status);
CREATE INDEX idx_card_transactions_created_at ON card_transactions(created_at DESC);

-- 复合索引：按设备和状态查询
CREATE INDEX idx_card_transactions_device_status ON card_transactions(device_id, status);

-- 卡片余额变更记录表（可选，用于审计）
CREATE TABLE IF NOT EXISTS card_balance_logs (
    id BIGSERIAL PRIMARY KEY,
    card_no VARCHAR(32) NOT NULL,
    transaction_id BIGINT,                        -- 关联交易ID
    change_type VARCHAR(20) NOT NULL,             -- 变更类型：recharge/consume/refund/adjust
    amount DECIMAL(10,2) NOT NULL,                -- 变更金额（正数=充值，负数=扣款）
    balance_before DECIMAL(10,2) NOT NULL,        -- 变更前余额
    balance_after DECIMAL(10,2) NOT NULL,         -- 变更后余额
    description TEXT,                             -- 变更说明
    created_at TIMESTAMP DEFAULT NOW() NOT NULL,
    
    CONSTRAINT fk_balance_logs_card FOREIGN KEY (card_no) REFERENCES cards(card_no) ON DELETE CASCADE
);

-- 余额变更记录索引
CREATE INDEX idx_balance_logs_card_no ON card_balance_logs(card_no);
CREATE INDEX idx_balance_logs_created_at ON card_balance_logs(created_at DESC);
CREATE INDEX idx_balance_logs_transaction_id ON card_balance_logs(transaction_id);

-- 插入测试数据（开发环境）
INSERT INTO cards (card_no, balance, status, description) VALUES
    ('1000000001', 100.00, 'active', '测试卡片1'),
    ('1000000002', 50.00, 'active', '测试卡片2'),
    ('1000000003', 0.00, 'inactive', '测试卡片3（未激活）'),
    ('1000000004', 200.00, 'active', '测试卡片4')
ON CONFLICT (card_no) DO NOTHING;

-- 添加注释
COMMENT ON TABLE cards IS '充电卡片信息表';
COMMENT ON TABLE card_transactions IS '刷卡充电交易记录表';
COMMENT ON TABLE card_balance_logs IS '卡片余额变更记录表';

COMMENT ON COLUMN cards.card_no IS '卡号，唯一标识';
COMMENT ON COLUMN cards.balance IS '卡片余额，单位：元';
COMMENT ON COLUMN cards.status IS '卡片状态：active=正常, inactive=未激活, blocked=已冻结';

COMMENT ON COLUMN card_transactions.charge_mode IS '充电模式：1=按时长, 2=按电量, 3=按功率, 4=充满自停';
COMMENT ON COLUMN card_transactions.status IS '订单状态：pending=待确认, charging=充电中, completed=已完成, cancelled=已取消, failed=失败';
COMMENT ON COLUMN card_transactions.order_no IS '订单号，唯一标识，格式：CARD{timestamp}{random}';

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

-- P0-2修复: 添加interrupted(10)状态支持
-- 用于charging订单的设备断线场景

-- 1. 更新orders表status字段注释
COMMENT ON COLUMN orders.status IS 
'订单状态: 0=pending, 1=confirmed, 2=charging, 3=timeout, 4=cancelled, 5=completed, 6=failed, 7=stopped, 8=cancelling, 9=stopping, 10=interrupted';

-- 2. 添加failure_reason字段(如果不存在)
DO $$ 
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name='orders' AND column_name='failure_reason'
    ) THEN
        ALTER TABLE orders ADD COLUMN failure_reason VARCHAR(255);
    END IF;
END $$;

-- 3. 为interrupted状态创建索引,提升监控任务查询性能
CREATE INDEX IF NOT EXISTS idx_orders_interrupted 
ON orders(device_id, status, updated_at) 
WHERE status = 10;

-- 4. 为设备离线超时检测创建索引
CREATE INDEX IF NOT EXISTS idx_orders_charging_by_device
ON orders(device_id, status, updated_at)
WHERE status = 2;

-- 5. 添加interrupted状态迁移逻辑(幂等)
-- 将当前charging状态且设备离线超过60秒的订单标记为interrupted
-- 注意: 需要配合设备last_seen字段判断,这里仅创建索引,实际逻辑在代码中


-- P1-7修复: 事件推送Outbox模式
-- events: 事件推送表
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

CREATE INDEX IF NOT EXISTS idx_events_status_created ON events(status, created_at) WHERE status IN (0, 2);
CREATE INDEX IF NOT EXISTS idx_events_order_no ON events(order_no);
CREATE INDEX IF NOT EXISTS idx_events_retry ON events(status, retry_count, created_at) WHERE status = 2 AND retry_count < 5;

COMMENT ON TABLE events IS 'P1-7: 事件推送Outbox模式，确保事件可靠推送';
COMMENT ON COLUMN events.status IS '0=待推送, 1=已推送, 2=失败';
COMMENT ON COLUMN events.sequence_no IS '同一订单的事件序列号，确保顺序';

-- 记录所有迁移版本
INSERT INTO schema_migrations(version) VALUES (1), (2), (3), (5), (6), (7), (8), (9), (11), (12);
