-- 补充 orders.end_reason 列，修复创单时缺失字段导致的数据库错误
ALTER TABLE orders
ADD COLUMN IF NOT EXISTS end_reason INT;
