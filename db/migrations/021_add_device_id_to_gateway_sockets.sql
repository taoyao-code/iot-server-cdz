-- 021_add_device_id_to_gateway_sockets.sql
-- 为 gateway_sockets 表添加 device_id 列
-- 该列用于将每个socket映射到独立的device记录，替代之前的全局端口计算逻辑

-- 添加 device_id 列
ALTER TABLE gateway_sockets ADD COLUMN IF NOT EXISTS device_id BIGINT REFERENCES devices(id);

-- 添加索引以加速通过 device_id 查询
CREATE INDEX IF NOT EXISTS idx_gateway_sockets_device_id ON gateway_sockets(device_id);

-- 添加注释
COMMENT ON COLUMN gateway_sockets.device_id IS '插座对应的设备ID，每个socket是独立的device';
