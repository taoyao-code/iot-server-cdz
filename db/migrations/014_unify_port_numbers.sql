-- 统一端口号：将orders表的API端口号(1,2)迁移为协议端口号(0,1)
-- 目标：全系统统一使用协议端口号，消除转换逻辑

BEGIN;

-- 1. 备份当前数据（可选，用于回滚）
CREATE TABLE IF NOT EXISTS orders_port_migration_backup AS
SELECT order_no, port_no, device_id, status, created_at
FROM orders
WHERE port_no >= 1;

-- 2. 迁移port_no: 1→0, 2→1
-- 只迁移port_no=1或2的订单（API端口号）
-- port_no=0的订单已经是协议端口号，无需迁移
UPDATE orders
SET port_no = port_no - 1
WHERE port_no IN (1, 2);

-- 3. 验证迁移结果
DO $$
DECLARE
    v_count_0 INTEGER;
    v_count_1 INTEGER;
    v_count_invalid INTEGER;
BEGIN
    -- 统计迁移后的端口号分布
    SELECT COUNT(*) INTO v_count_0 FROM orders WHERE port_no = 0;
    SELECT COUNT(*) INTO v_count_1 FROM orders WHERE port_no = 1;
    SELECT COUNT(*) INTO v_count_invalid FROM orders WHERE port_no NOT IN (0, 1);

    -- 输出统计信息
    RAISE NOTICE '迁移完成统计:';
    RAISE NOTICE 'port_no=0 (A端口): % 条', v_count_0;
    RAISE NOTICE 'port_no=1 (B端口): % 条', v_count_1;
    RAISE NOTICE '异常port_no: % 条', v_count_invalid;

    -- 如果有异常值，警告但不回滚
    IF v_count_invalid > 0 THEN
        RAISE WARNING '存在异常port_no值，请手动检查';
    END IF;
END $$;

COMMIT;

-- 迁移说明：
-- 1. 迁移前：ports表使用0,1（协议号）；orders表使用1,2（API号）
-- 2. 迁移后：全部统一使用0,1（协议号）
-- 3. 显示层映射：0→A端口，1→B端口
-- 4. 备份表orders_port_migration_backup可用于回滚
