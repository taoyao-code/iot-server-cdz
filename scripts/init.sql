-- IOT Server 数据库初始化脚本
-- 生产环境数据库初始化

-- 设置时区
SET timezone = 'Asia/Shanghai';

-- 启用必要的扩展
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pg_stat_statements";

-- 创建数据库用户（如果不存在）
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_user WHERE usename = 'iot') THEN
        CREATE USER iot WITH PASSWORD 'CHANGE_ME_IN_PRODUCTION';
    END IF;
END
$$;

-- 授予权限
GRANT ALL PRIVILEGES ON DATABASE iot_server TO iot;

-- 创建schema
CREATE SCHEMA IF NOT EXISTS public;
GRANT ALL ON SCHEMA public TO iot;

-- 说明：实际的表结构由应用的migration系统管理
-- 此文件仅用于基础数据库和用户初始化

-- 创建健康检查表（用于监控）
CREATE TABLE IF NOT EXISTS health_check (
    id SERIAL PRIMARY KEY,
    last_check TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 插入初始记录
INSERT INTO health_check (last_check) VALUES (CURRENT_TIMESTAMP)
ON CONFLICT DO NOTHING;

-- 创建索引优化（应用启动后会自动创建表，这里预留索引优化）
-- 实际索引应在migration中创建

