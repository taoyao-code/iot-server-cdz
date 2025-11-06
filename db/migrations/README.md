# 数据库迁移

## 文件

- `full_schema.sql` - 完整Schema(434行,包含所有版本)

## 使用

```bash
# 新环境
docker exec -i iot-postgres-prod psql -U iot -d iot_server < db/migrations/full_schema.sql

# 远程部署
make remote-migrate

# 完整部署
make auto-deploy
```

## 版本

已包含: 1, 2, 3, 5, 6, 7, 8, 9, 11
