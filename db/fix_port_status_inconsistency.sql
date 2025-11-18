-- 修复端口状态不一致问题
-- 执行时间: 2025-11-18
-- 原因: 订单状态码修复后，清理历史遗留的端口状态不一致

-- ===== 第一部分: 修复历史订单的错误状态码 =====

-- 1.1 查看需要修复的历史订单
SELECT
  order_no,
  port_no,
  status,
  CASE status
    WHEN 1 THEN 'old_confirmed (need fix)'
    WHEN 2 THEN 'old_completed (need fix)'
    ELSE 'unknown'
  END as status_issue,
  start_time,
  end_time
FROM orders
WHERE (status = 1 AND start_time IS NOT NULL)  -- 已开始充电但状态=1，应该是2
   OR (status = 2 AND end_time IS NOT NULL AND end_time < NOW());  -- 已结束但状态=2，应该是3

-- 1.2 修复已开始充电但使用旧status=1的订单 → 改为charging(2)
UPDATE orders
SET status = 2, updated_at = NOW()
WHERE status = 1
  AND start_time IS NOT NULL
  AND end_time IS NULL;

-- 1.3 修复已结束但使用旧status=2的订单 → 改为completed(3)
UPDATE orders
SET status = 3, updated_at = NOW()
WHERE status = 2
  AND end_time IS NOT NULL
  AND end_time < NOW();

-- ===== 第二部分: 修复端口状态不一致 =====

-- 2.1 查看当前不一致的端口（有occupied/charging状态但没有对应活跃订单）
SELECT
  d.phy_id,
  p.port_no,
  p.status,
  CASE p.status
    WHEN 0 THEN 'idle'
    WHEN 1 THEN 'occupied'
    WHEN 2 THEN 'charging'
    WHEN 3 THEN 'fault'
  END as port_status_text,
  COUNT(o.id) as active_orders_count
FROM ports p
JOIN devices d ON p.device_id = d.id
LEFT JOIN orders o ON o.device_id = p.device_id
  AND o.port_no = p.port_no
  AND o.status IN (0, 2, 8, 9, 10)  -- pending, charging, cancelling, stopping, interrupted （不再包含1）
WHERE p.status IN (1, 2)  -- occupied or charging
GROUP BY d.phy_id, p.port_no, p.status
HAVING COUNT(o.id) = 0;

-- 2.2 修复不一致的端口状态（将没有活跃订单的occupied/charging端口重置为idle）
UPDATE ports p
SET status = 0, updated_at = NOW()
FROM devices d
WHERE p.device_id = d.id
  AND p.status IN (1, 2)  -- occupied or charging
  AND NOT EXISTS (
    SELECT 1 FROM orders o
    WHERE o.device_id = p.device_id
      AND o.port_no = p.port_no
      AND o.status IN (0, 2, 8, 9, 10)  -- pending, charging, cancelling, stopping, interrupted （不再包含1）
  );

-- ===== 第三部分: 验证修复结果 =====

-- 3.1 验证端口状态
SELECT
  d.phy_id,
  p.port_no,
  CASE p.status
    WHEN 0 THEN 'idle'
    WHEN 1 THEN 'occupied'
    WHEN 2 THEN 'charging'
    WHEN 3 THEN 'fault'
  END as port_status,
  COUNT(o.id) as active_orders
FROM ports p
JOIN devices d ON p.device_id = d.id
LEFT JOIN orders o ON o.device_id = p.device_id
  AND o.port_no = p.port_no
  AND o.status IN (0, 2, 8, 9, 10)
GROUP BY d.phy_id, p.port_no, p.status
ORDER BY d.phy_id, p.port_no;

-- 3.2 验证订单状态分布
SELECT
  status,
  CASE status
    WHEN 0 THEN 'pending'
    WHEN 2 THEN 'charging'
    WHEN 3 THEN 'completed'
    WHEN 5 THEN 'cancelled'
    WHEN 6 THEN 'failed'
    WHEN 8 THEN 'cancelling'
    WHEN 9 THEN 'stopping'
    WHEN 10 THEN 'interrupted'
    ELSE 'unknown/legacy'
  END as status_text,
  COUNT(*) as count
FROM orders
GROUP BY status
ORDER BY status;
