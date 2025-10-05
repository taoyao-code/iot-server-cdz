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

