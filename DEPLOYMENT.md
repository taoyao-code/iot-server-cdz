# IOT Server 生产环境部署指南

## 📋 目录

- [系统要求](#系统要求)
- [快速开始](#快速开始)
- [配置说明](#配置说明)
- [部署步骤](#部署步骤)
- [监控运维](#监控运维)
- [故障排查](#故障排查)
- [安全加固](#安全加固)

## 🔧 系统要求

### 硬件要求

- **CPU**: 4核心以上（推荐8核心）
- **内存**: 8GB以上（推荐16GB）
- **磁盘**: 100GB以上 SSD（根据数据量调整）
- **网络**: 1Gbps以上

### 软件要求

- **操作系统**: Linux (Ubuntu 20.04+, CentOS 8+, Debian 11+)
- **Docker**: 20.10+
- **Docker Compose**: 2.0+
- **Git**: 2.0+

## 🚀 快速开始

### 1. 克隆代码

```bash
git clone https://github.com/your-org/iot-server.git
cd iot-server
```

### 2. 配置环境变量

```bash
# 复制环境变量模板
cp scripts/env.example .env

# 编辑配置文件
nano .env
```

**必须修改的变量**：

```bash
# 数据库密码（强密码，至少16位）
POSTGRES_PASSWORD=your-strong-password-here

# Redis密码（强密码）
REDIS_PASSWORD=your-redis-password-here

# API密钥（至少32位随机字符串）
API_KEY=$(openssl rand -base64 32)
THIRDPARTY_API_KEY=$(openssl rand -base64 32)

# Webhook配置（如果使用第三方推送）
WEBHOOK_URL=https://your-webhook-endpoint.com/events
WEBHOOK_SECRET=$(openssl rand -base64 32)
```

### 3. 一键部署

```bash
# 执行安全部署（自动备份 + 零停机）
make deploy
```

部署脚本会自动完成：

- ✅ 数据库自动备份（后续部署）
- ✅ 智能检测（首次/更新）
- ✅ Docker镜像构建
- ✅ 零停机更新
- ✅ 健康检查
- ✅ 失败自动回滚

### 4. 验证部署

```bash
# 查看服务状态
docker-compose ps

# 测试API
curl http://localhost:7055/healthz
curl http://localhost:7055/metrics

# 查看日志
make prod-logs
```

## ⚙️ 配置说明

### 配置文件

| 文件 | 说明 |
|------|------|
| `configs/production.yaml` | 生产环境主配置 |
| `configs/bkv_reason_map.yaml` | BKV协议错误码映射 |
| `.env` | 环境变量配置（敏感信息） |
| `docker-compose.prod.yml` | 生产环境容器编排 |

### 关键配置项

#### 1. 数据库配置

```yaml
database:
  dsn: "${DATABASE_URL}"
  maxOpenConns: 100      # 根据实际负载调整
  maxIdleConns: 20
  connMaxLifetime: 2h
  autoMigrate: false     # 生产环境禁用自动迁移
```

#### 2. Redis配置

```yaml
redis:
  enabled: true
  pool_size: 100         # 根据并发量调整
  min_idle_conns: 20
```

#### 3. TCP服务配置

```yaml
tcp:
  addr: ":7000"
  maxConnections: 50000  # 最大设备连接数
  limiting:
    enabled: true
    max_connections: 50000
    rate_per_second: 500
```

#### 4. 第三方集成

```yaml
thirdparty:
  push:
    webhook_url: "${WEBHOOK_URL}"
    worker_count: 10      # 事件推送并发数
    max_retries: 5
```

## 📦 部署步骤

### 方式一：使用 Makefile（推荐）

```bash
# 安全部署（自动备份 + 零停机）
make deploy

# 构建镜像
make docker-build

# 重启服务
make prod-restart

# 查看日志
make prod-logs

# 停止服务
make prod-down
```

### 方式二：手动部署

```bash
# 1. 构建镜像
docker build -t iot-server:latest .

# 2. 启动服务
docker-compose -f docker-compose.prod.yml up -d

# 3. 查看状态
docker-compose -f docker-compose.prod.yml ps

# 4. 查看日志
docker-compose -f docker-compose.prod.yml logs -f iot-server
```

### 启用监控（可选）

```bash
# 启动Prometheus和Grafana
docker-compose -f docker-compose.prod.yml --profile monitoring up -d

# 访问监控面板
# Prometheus: http://localhost:9090
# Grafana: http://localhost:3000 (默认密码在.env中配置)
```

## 📊 监控运维

### 服务端点

| 端点 | 说明 |
|------|------|
| `http://localhost:8080/healthz` | 健康检查 |
| `http://localhost:8080/readyz` | 就绪检查 |
| `http://localhost:8080/metrics` | Prometheus指标 |
| `http://localhost:8080/api/v1/third/*` | 第三方API |

### 关键指标监控

#### 1. 系统指标

- CPU使用率 < 80%
- 内存使用率 < 85%
- 磁盘使用率 < 80%

#### 2. 应用指标

- TCP连接数
- HTTP请求QPS
- API响应时间
- 事件队列长度
- 数据库连接池状态

#### 3. 业务指标

- 在线设备数
- 充电订单数
- 事件推送成功率
- 协议解析错误率

### 日志管理

```bash
# 查看实时日志
docker-compose -f docker-compose.prod.yml logs -f iot-server

# 查看最近100行日志
docker-compose -f docker-compose.prod.yml logs --tail=100 iot-server

# 导出日志
docker cp iot-server-prod:/var/log/iot-server ./logs-backup/

# 日志轮转
# 日志会自动轮转，保留30个文件，每个最大500MB
```

## 🔍 故障排查

### 常见问题

#### 1. 服务无法启动

```bash
# 检查日志
docker-compose -f docker-compose.prod.yml logs iot-server

# 常见原因：
# - 环境变量未配置
# - 数据库连接失败
# - 端口被占用
# - 配置文件语法错误
```

#### 2. 数据库连接失败

```bash
# 检查数据库状态
docker-compose -f docker-compose.prod.yml ps postgres

# 测试数据库连接
docker-compose -f docker-compose.prod.yml exec postgres \
  psql -U iot -d iot_server -c "SELECT 1"

# 检查数据库日志
docker-compose -f docker-compose.prod.yml logs postgres
```

#### 3. Redis连接失败

```bash
# 检查Redis状态
docker-compose -f docker-compose.prod.yml ps redis

# 测试Redis连接
docker-compose -f docker-compose.prod.yml exec redis \
  redis-cli -a ${REDIS_PASSWORD} ping
```

#### 4. 设备无法连接

```bash
# 检查TCP端口
netstat -tulpn | grep 7000

# 检查防火墙
sudo ufw status
sudo firewall-cmd --list-ports

# 测试端口连通性
telnet localhost 7000
```

### 性能优化

#### 1. 数据库优化

```sql
-- 查看慢查询
SELECT * FROM pg_stat_statements 
ORDER BY total_exec_time DESC 
LIMIT 10;

-- 查看表大小
SELECT 
    schemaname,
    tablename,
    pg_size_pretty(pg_total_relation_size(schemaname||'.'||tablename)) AS size
FROM pg_tables
WHERE schemaname = 'public'
ORDER BY pg_total_relation_size(schemaname||'.'||tablename) DESC;
```

#### 2. Redis优化

```bash
# 查看内存使用
docker-compose exec redis redis-cli -a ${REDIS_PASSWORD} INFO memory

# 查看慢日志
docker-compose exec redis redis-cli -a ${REDIS_PASSWORD} SLOWLOG GET 10
```

## 🔒 安全加固

### 1. 网络安全

```bash
# 配置防火墙
sudo ufw allow 22/tcp      # SSH
sudo ufw allow 8080/tcp    # HTTP API
sudo ufw allow 7000/tcp    # TCP协议端口
sudo ufw enable

# 仅允许特定IP访问（推荐）
sudo ufw allow from YOUR_IP to any port 8080
```

### 2. API安全

- ✅ 启用API Key认证
- ✅ 使用HMAC签名验证
- ✅ 配置IP白名单
- ✅ 限流和熔断保护

### 3. 数据库安全

- ✅ 使用强密码
- ✅ 禁止root远程登录
- ✅ 定期备份
- ✅ 启用SSL连接（生产环境必须）

### 4. 容器安全

- ✅ 使用非root用户运行
- ✅ 限制容器资源
- ✅ 定期更新基础镜像
- ✅ 扫描镜像漏洞

## 🔄 更新升级

### 滚动更新

```bash
# 1. 拉取最新代码
git pull origin main

# 2. 执行安全部署（自动备份 + 零停机）
make deploy

# 部署过程：
# ✅ 自动备份数据库
# ✅ 构建新镜像（利用缓存）
# ✅ 零停机更新（仅更新应用，数据库不重启）
# ✅ 健康检查
# ✅ 失败自动回滚
```

### 数据库迁移

```bash
# 1. 备份数据库
docker-compose exec postgres pg_dump -U iot iot_server > backup.sql

# 2. 运行迁移
# (应用启动时会自动运行迁移，或手动执行迁移脚本)

# 3. 验证迁移
docker-compose exec postgres psql -U iot -d iot_server -c "\dt"
```

## 📈 容量规划

### 硬件配置推荐

| 设备规模 | CPU | 内存 | 磁盘 | 网络 |
|---------|-----|------|------|------|
| 1000台 | 4核 | 8GB | 200GB | 100Mbps |
| 5000台 | 8核 | 16GB | 500GB | 1Gbps |
| 10000台 | 16核 | 32GB | 1TB | 10Gbps |

### 数据库配置建议

```yaml
# 1000台设备
maxOpenConns: 50
maxIdleConns: 10

# 5000台设备
maxOpenConns: 100
maxIdleConns: 20

# 10000台设备
maxOpenConns: 200
maxIdleConns: 50
```

## 📞 技术支持

如有问题，请联系：

- 📧 Email: <support@example.com>
- 📱 电话: +86-xxx-xxxx-xxxx
- 💬 技术群: xxxxx

## 📄 许可证

[添加许可证信息]
