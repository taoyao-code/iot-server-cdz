DROP INDEX IF EXISTS idx_orders_business_no;

ALTER TABLE orders
    DROP COLUMN IF EXISTS business_no;
