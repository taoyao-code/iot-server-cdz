-- Migration: 修复devices表schema完整性
-- Purpose: 添加缺失的online字段、UNIQUE约束和索引
-- Date: 2025-01-18
-- Fixes:
--   1. BKV心跳包处理错误（ON CONFLICT需要UNIQUE约束）
--   2. P1-4端口状态同步器错误（查询online字段）

BEGIN;

-- 1. 添加online字段（如果不存在）
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name='devices' AND column_name='online'
    ) THEN
        ALTER TABLE devices ADD COLUMN online BOOLEAN NOT NULL DEFAULT false;
        RAISE NOTICE 'Added online column to devices table';
    END IF;
END $$;

-- 2. 为phy_id添加UNIQUE约束（如果不存在）
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'devices'::regclass
        AND conname = 'devices_phy_id_key'
    ) THEN
        -- 先检查是否有重复的phy_id
        IF EXISTS (
            SELECT phy_id, COUNT(*)
            FROM devices
            GROUP BY phy_id
            HAVING COUNT(*) > 1
        ) THEN
            RAISE EXCEPTION '发现重复的phy_id，需要先清理重复数据';
        END IF;

        -- 添加UNIQUE约束
        ALTER TABLE devices ADD CONSTRAINT devices_phy_id_key UNIQUE (phy_id);
        RAISE NOTICE 'Added UNIQUE constraint to phy_id';
    END IF;
END $$;

-- 3. 创建last_seen_at索引（如果不存在）
CREATE INDEX IF NOT EXISTS idx_devices_last_seen_at
ON devices(last_seen_at);

COMMENT ON INDEX idx_devices_last_seen_at IS '优化在线设备查询';

-- 4. 创建gateway_id索引（如果不存在）
CREATE INDEX IF NOT EXISTS idx_devices_gateway_id
ON devices(gateway_id)
WHERE gateway_id IS NOT NULL;

COMMENT ON INDEX idx_devices_gateway_id IS '优化网关设备查询';

-- 5. 添加字段注释
COMMENT ON COLUMN devices.online IS '设备在线状态（由会话管理器维护）';
COMMENT ON COLUMN devices.phy_id IS '设备物理ID（唯一标识）';

-- 6. 验证修复
DO $$
DECLARE
    online_exists INT;
    unique_exists INT;
    idx1_exists INT;
    idx2_exists INT;
BEGIN
    SELECT COUNT(*) INTO online_exists
    FROM information_schema.columns
    WHERE table_name='devices' AND column_name='online';

    SELECT COUNT(*) INTO unique_exists
    FROM pg_constraint
    WHERE conrelid = 'devices'::regclass
    AND conname = 'devices_phy_id_key';

    SELECT COUNT(*) INTO idx1_exists
    FROM pg_indexes
    WHERE tablename='devices' AND indexname='idx_devices_last_seen_at';

    SELECT COUNT(*) INTO idx2_exists
    FROM pg_indexes
    WHERE tablename='devices' AND indexname='idx_devices_gateway_id';

    IF online_exists = 0 OR unique_exists = 0 OR idx1_exists = 0 OR idx2_exists = 0 THEN
        RAISE EXCEPTION 'devices表schema修复失败: online=%, unique=%, idx1=%, idx2=%',
                        online_exists, unique_exists, idx1_exists, idx2_exists;
    END IF;

    RAISE NOTICE 'devices表schema修复成功: online=%, unique=%, idx1=%, idx2=%',
                  online_exists, unique_exists, idx1_exists, idx2_exists;
END $$;

COMMIT;
