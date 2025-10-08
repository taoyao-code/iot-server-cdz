# 📋 生产环境部署检查清单

## ⚠️ 部署前必查项

### 1. 环境准备 ✓

- [ ] 服务器系统已更新到最新版本
- [ ] Docker 20.10+ 已安装
- [ ] Docker Compose 2.0+ 已安装
- [ ] 防火墙规则已配置
  - [ ] TCP 22 (SSH)
  - [ ] TCP 8080 (HTTP API)
  - [ ] TCP 7000 (设备连接)
- [ ] SSL证书已准备（如果使用HTTPS）
- [ ] 域名DNS已配置

### 2. 配置文件 ✓

- [ ] 已复制 `scripts/env.example` 为 `.env`
- [ ] 已配置数据库密码（至少16位强密码）
- [ ] 已配置Redis密码（至少16位强密码）
- [ ] 已配置API密钥（至少32位）
- [ ] 已配置Webhook地址和密钥
- [ ] 已审查 `configs/production.yaml`
- [ ] 已验证 `configs/bkv_reason_map.yaml`

### 3. 安全检查 ✓

- [ ] 所有默认密码已修改
- [ ] API认证已启用 (`api.auth.enabled: true`)
- [ ] HMAC签名验证已启用
- [ ] IP白名单已配置（可选）
- [ ] `.env` 文件权限设置为 `600`
- [ ] 数据库密码已加密存储
- [ ] 日志中不包含敏感信息

### 4. 资源配置 ✓

- [ ] 服务器CPU: 4核以上
- [ ] 服务器内存: 8GB以上
- [ ] 磁盘空间: 100GB以上
- [ ] 数据库连接池已配置
- [ ] Redis连接池已配置
- [ ] Docker资源限制已设置

### 5. 监控告警 ✓

- [ ] Prometheus已配置（可选）
- [ ] Grafana已配置（可选）
- [ ] 告警规则已设置
- [ ] 日志收集已配置
- [ ] 健康检查端点正常

## 📦 部署步骤检查清单

### 步骤 1: 代码准备

```bash
# 克隆代码
- [ ] git clone完成
- [ ] 切换到正确的分支/标签
- [ ] 检查代码版本
```

### 步骤 2: 配置文件

```bash
# 配置环境变量
- [ ] cp scripts/env.example .env
- [ ] 编辑.env并填写所有必需值
- [ ] chmod 600 .env
- [ ] 验证配置正确性
```

### 步骤 3: 构建镜像

```bash
# 构建Docker镜像
- [ ] make docker-build 成功
- [ ] 镜像标签正确
- [ ] 镜像大小合理（< 50MB）
```

### 步骤 4: 数据库准备

```bash
# 数据库初始化
- [ ] PostgreSQL容器启动成功
- [ ] 数据库连接正常
- [ ] 初始化脚本执行成功
- [ ] 备份策略已配置
```

### 步骤 5: 启动服务

```bash
# 启动应用
- [ ] make prod-up 成功
- [ ] 所有容器状态健康
- [ ] 日志无错误信息
- [ ] 端口监听正常
```

### 步骤 6: 健康检查

```bash
# 验证服务
- [ ] curl http://localhost:8080/healthz 返回200
- [ ] curl http://localhost:8080/readyz 返回200
- [ ] curl http://localhost:8080/metrics 返回指标
- [ ] TCP端口7000可访问
```

### 步骤 7: 功能测试

```bash
# API功能测试
- [ ] 设备连接测试
- [ ] 第三方API测试
- [ ] 事件推送测试
- [ ] 数据库读写测试
- [ ] Redis连接测试
```

## 🔍 部署后验证清单

### 1. 服务状态验证

```bash
# 检查服务状态
docker-compose -f docker-compose.prod.yml ps

预期结果：
- [ ] iot-server-prod: Up (healthy)
- [ ] iot-postgres-prod: Up (healthy)
- [ ] iot-redis-prod: Up (healthy)
```

### 2. 日志检查

```bash
# 查看应用日志
docker-compose -f docker-compose.prod.yml logs iot-server | tail -50

检查项：
- [ ] 无ERROR级别日志
- [ ] 数据库连接成功
- [ ] Redis连接成功
- [ ] TCP服务器启动成功
- [ ] HTTP服务器启动成功
```

### 3. API功能验证

```bash
# 健康检查
- [ ] GET /healthz → 200 OK

# 就绪检查
- [ ] GET /readyz → 200 OK

# 指标端点
- [ ] GET /metrics → 200 OK, Prometheus格式

# 第三方API（需要API Key）
- [ ] POST /api/v1/third/devices/:id/charge
- [ ] GET /api/v1/third/devices/:id
- [ ] GET /api/v1/third/orders
```

### 4. 数据库验证

```bash
# 连接数据库
docker-compose -f docker-compose.prod.yml exec postgres \
  psql -U iot -d iot_server

检查项：
- [ ] 表结构正确创建
- [ ] 索引已创建
- [ ] 数据可正常读写
```

### 5. 性能验证

```bash
检查项：
- [ ] CPU使用率 < 50%（空载）
- [ ] 内存使用率 < 50%（空载）
- [ ] API响应时间 < 100ms
- [ ] TCP连接延迟 < 50ms
```

## 🔧 常见问题检查

### 问题1: 服务无法启动

```bash
检查步骤：
- [ ] 检查端口是否被占用: netstat -tulpn | grep 8080
- [ ] 检查环境变量是否正确: cat .env
- [ ] 检查配置文件语法: cat configs/production.yaml
- [ ] 查看详细日志: docker-compose logs iot-server
```

### 问题2: 数据库连接失败

```bash
检查步骤：
- [ ] 数据库容器是否运行: docker ps | grep postgres
- [ ] 数据库密码是否正确: 检查.env中的密码
- [ ] 网络连接是否正常: docker network ls
- [ ] 数据库日志: docker-compose logs postgres
```

### 问题3: Redis连接失败

```bash
检查步骤：
- [ ] Redis容器是否运行: docker ps | grep redis
- [ ] Redis密码是否正确: 检查.env中的密码
- [ ] 测试连接: docker exec iot-redis-prod redis-cli -a PASSWORD ping
```

### 问题4: 设备无法连接

```bash
检查步骤：
- [ ] TCP端口是否监听: netstat -tulpn | grep 7000
- [ ] 防火墙是否开放: sudo ufw status
- [ ] 设备网络是否正常
- [ ] 协议版本是否匹配
```

## 📊 监控指标

### 关键指标阈值

```
应用指标：
- [ ] TCP连接数 < maxConnections的80%
- [ ] HTTP QPS < 1000/s
- [ ] API P99延迟 < 200ms
- [ ] 事件队列长度 < 10000

系统指标：
- [ ] CPU使用率 < 80%
- [ ] 内存使用率 < 85%
- [ ] 磁盘使用率 < 80%
- [ ] 网络带宽 < 80%

数据库指标：
- [ ] 连接数 < maxOpenConns的80%
- [ ] 慢查询 < 10/min
- [ ] 死锁 = 0
- [ ] 复制延迟 < 1s（如有主从）
```

## 🔐 安全加固检查

### 系统安全

- [ ] SSH密钥登录已启用
- [ ] 密码登录已禁用
- [ ] fail2ban已安装配置
- [ ] 系统更新已安装
- [ ] 不必要的服务已禁用

### 应用安全

- [ ] API Key长度 >= 32位
- [ ] 密码复杂度符合要求
- [ ] HTTPS已启用（如适用）
- [ ] 跨域访问已限制
- [ ] 请求大小已限制

### 网络安全

- [ ] 防火墙规则最小化
- [ ] 仅必要端口对外开放
- [ ] DDoS防护已启用
- [ ] 速率限制已配置

## 📝 运维检查

### 日常运维

- [ ] 日志轮转已配置
- [ ] 备份脚本已测试
- [ ] 监控告警已配置
- [ ] 应急预案已准备
- [ ] 联系人清单已更新

### 文档完整性

- [ ] 部署文档已更新
- [ ] API文档已同步
- [ ] 故障处理文档已准备
- [ ] 变更记录已维护

## ✅ 最终确认

```
部署负责人: ________________
审核人: ________________
部署时间: ________________
版本号: ________________

确认签字:
部署负责人: ________________ 日期: ________
技术负责人: ________________ 日期: ________
```

## 🚀 部署完成后的下一步

1. **性能基准测试**
   - 压力测试
   - 并发测试
   - 长时间稳定性测试

2. **灰度发布**
   - 小流量测试
   - 逐步扩大流量
   - 监控关键指标

3. **备份验证**
   - 执行首次备份
   - 测试恢复流程

4. **监控配置**
   - 配置告警规则
   - 测试告警通知
   - 设置监控大盘

5. **文档归档**
   - 部署记录
   - 配置备份
   - 变更日志
