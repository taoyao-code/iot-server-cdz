-- Week4: 回滚刷卡充电系统

-- 删除表（按依赖关系逆序）
DROP TABLE IF EXISTS card_balance_logs;
DROP TABLE IF EXISTS card_transactions;
DROP TABLE IF EXISTS cards;

-- 注意：索引会随着表自动删除

