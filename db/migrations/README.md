# 数据库迁移文件

## 迁移顺序

| 版本 | 文件名 | 说明 | 依赖 |
|------|--------|------|------|
| 0001 | init | 初始化基础表结构 | 无 |
| 0002 | outbox | 下行任务队列表 | 0001 |
| 0003 | outbound_msgid | 为下行队列添加msg_id字段 | 0002 |
| 0004 | reserved | 预留(暂无操作) | - |
| 0005 | device_params | BKV设备参数持久化 | 0001 |
| 0006 | query_optimization | 查询优化索引 | 0001-0005 |
| 0007 | card_system | 刷卡充电系统 | 0001 |
| 0008 | network_management | 组网管理(网关插座) | 0001 |
| 0009 | ota_upgrade | OTA升级功能 | 0001 |

## 表结构概览

### 核心表

- `devices` - 设备基础信息
- `ports` - 设备端口状态
- `orders` - 充电订单
- `cmd_log` - 指令日志
- `outbound_queue` - 下行任务队列

### 功能表

- `device_params` - 设备参数记录
- `cards` - 充电卡信息
- `card_transactions` - 刷卡交易记录
- `card_balance_logs` - 余额变更日志
- `gateway_sockets` - 网关插座管理
- `ota_tasks` - OTA升级任务

## 执行迁移

### 初始化数据库

```bash
# 按顺序执行所有迁移
for f in db/migrations/*_up.sql; do
    echo "执行: $f"
    docker exec -i iot-postgres-prod psql -U iot -d iot_server < "$f"
done
```

### 回滚数据库

```bash
# 按逆序执行回滚
for f in $(ls -r db/migrations/*_down.sql); do
    echo "回滚: $f"
    docker exec -i iot-postgres-prod psql -U iot -d iot_server < "$f"
done
```

### 重建数据库(测试环境)

```bash
# 1. 删除所有表
docker exec -it iot-postgres-prod psql -U iot -d iot_server -c "
DROP TABLE IF EXISTS ota_tasks CASCADE;
DROP TABLE IF EXISTS gateway_sockets CASCADE;
DROP TABLE IF EXISTS card_balance_logs CASCADE;
DROP TABLE IF EXISTS card_transactions CASCADE;
DROP TABLE IF EXISTS cards CASCADE;
DROP TABLE IF EXISTS device_params CASCADE;
DROP TABLE IF EXISTS outbound_queue CASCADE;
DROP TABLE IF EXISTS cmd_log CASCADE;
DROP TABLE IF EXISTS orders CASCADE;
DROP TABLE IF EXISTS ports CASCADE;
DROP TABLE IF EXISTS devices CASCADE;
"

# 2. 重新执行所有迁移
for f in db/migrations/*_up.sql; do
    docker exec -i iot-postgres-prod psql -U iot -d iot_server < "$f"
done

# 3. 重启应用
docker restart iot-server-prod
```

## 迁移规范

### 文件命名

- 格式: `{version}_{name}.{up|down}.sql`
- 版本号: 4位数字,从0001开始
- 名称: 小写字母+下划线,简洁描述
- 方向: `up` = 应用迁移, `down` = 回滚迁移

### SQL规范

1. 使用 `IF NOT EXISTS` 避免重复创建
2. 使用 `IF EXISTS` 避免删除不存在的对象
3. 添加注释说明表和字段用途
4. 创建必要的索引优化查询
5. 外键约束使用 `ON DELETE CASCADE/RESTRICT`

### 注意事项

- ⚠️ **生产环境**: 不要直接删表重建,使用 `ALTER TABLE` 修改
- ⚠️ **测试环境**: 可以删表重建,数据无需保留
- ⚠️ **依赖关系**: 必须按顺序执行,不能跳过版本
- ⚠️ **回滚操作**: 谨慎使用,会丢失数据

## 字段变更历史

### devices 表

- 0001: 初始创建(添加 gateway_id, rssi, fw_ver, last_seen_at)
- 修复: 补充缺失字段以支持GN协议

### outbound_queue 表  

- 0001: 基础版本(已移除,避免重复)
- 0002: 完整版本(添加更多字段和触发器)
- 0003: 添加 msg_id 字段

## 常见问题

### Q: 为什么有 0004_reserved?

A: 占位文件,保持版本号连续性。

### Q: 迁移执行失败怎么办?

A:

1. 检查依赖是否满足
2. 查看错误日志定位问题
3. 测试环境可以重建数据库
4. 生产环境需要手动修复后继续

### Q: 如何验证迁移是否成功?

A:

```sql
-- 检查表是否存在
SELECT tablename FROM pg_tables WHERE schemaname='public' ORDER BY tablename;

-- 检查字段是否存在
SELECT column_name, data_type FROM information_schema.columns 
WHERE table_name='devices' ORDER BY ordinal_position;
```
