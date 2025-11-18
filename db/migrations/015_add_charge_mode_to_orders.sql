-- 添加orders表缺失的charge_mode字段
-- 用于记录订单的充电模式：1=按时长,2=按电量,3=按功率,4=充满自停

BEGIN;

-- 添加charge_mode字段
ALTER TABLE orders
ADD COLUMN IF NOT EXISTS charge_mode INTEGER DEFAULT 1;

-- 添加注释
COMMENT ON COLUMN orders.charge_mode IS '充电模式: 1=按时长, 2=按电量, 3=按功率, 4=充满自停';

-- 为现有数据设置默认值（按时长）
UPDATE orders
SET charge_mode = 1
WHERE charge_mode IS NULL;

-- 设置为NOT NULL
ALTER TABLE orders
ALTER COLUMN charge_mode SET NOT NULL;

COMMIT;
