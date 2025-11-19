ALTER TABLE orders
    ADD COLUMN IF NOT EXISTS business_no INTEGER NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_orders_business_no
    ON orders (device_id, business_no);
