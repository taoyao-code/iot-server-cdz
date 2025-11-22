-- Migration: 移除 devices.online 冗余字段
-- Purpose: 设备在线状态仅由 SessionManager + last_seen_at 提供，删除未维护的 boolean online 列
-- Date: 2025-11-22

BEGIN;

-- 1. 如果存在，则删除 devices.online 列
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_name = 'devices'
          AND column_name = 'online'
    ) THEN
        ALTER TABLE devices DROP COLUMN online;
        RAISE NOTICE 'Dropped devices.online column';
    END IF;
END $$;

COMMIT;

