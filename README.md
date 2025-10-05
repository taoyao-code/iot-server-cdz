# 物联网充电桩平台

当前项目一个专注于物联网充电桩管理和监控的平台，旨在提供一个高效、可靠的解决方案，帮助用户轻松管理和监控充电桩设备。
当前的项目重点在于与设备之间的通信和数据处理，确保充电桩的稳定运行和高效管理。
当前项目是一个类似于中间件的形式，主要负责充电桩设备与用户之间的交互和数据传输。

## 数据流

1. 充电桩设备通过物联网协议（如 TCP、MQTT、HTTP 等）将数据发送到平台，平台接收并处理这些数据(当前重点实现 TCP 模块)。
2. 平台将处理后的数据存储在数据库中，确保数据的完整性和可追溯性。
3. 用户通过平台的前端界面或 API 接口访问和管理充电桩设备，进行充电操作和数据查询。
4. 平台实时监控充电桩的状态，及时响应设备的异常情况，确保充电桩的稳定运行。
5. 平台定期生成充电数据报告，帮助用户分析充电行为和优化充电策略。
6. 平台支持多种充电桩型号和协议，确保广泛适用性和兼容性。
7. 平台提供详细的文档和教程，帮助用户快速上手和高效使用平台功能。
8. 平台持续优化性能，提升用户体验和系统稳定性。
9. 平台支持扩展和升级，满足未来的功能需求和技术发展。

## 打造一个物联网充电桩平台，包含以下功能

- 设备管理：支持充电桩的注册、配置和监控。
- 用户管理：支持用户注册、登录和权限管理。
- 充电管理：支持充电桩的启动、停止和计费。
- 数据分析：支持充电数据的统计和分析。
- 报警管理：支持充电桩的故障报警和处理。
- API 接口：提供开放的 API 接口，方便第三方系统集成。
- 可扩展性：支持平台的扩展和升级，满足未来需求。
- 实时监控：支持充电桩的实时状态监控和远程控制。
- 文档支持：提供详细的文档和教程，帮助用户快速上手。
- 兼容性：支持多种充电桩型号和协议，确保广泛适用。
- 性能优化：持续优化平台性能，提升用户体验。
- 充电历史：记录用户的充电历史，方便查询和管理。
- 充电桩状态通知：支持充电桩状态的实时通知，提升用户体验。
- 充电桩日志管理：记录充电桩的操作日志，方便问题排查。
- 充电桩健康监测：实时监测充电桩的健康状态，预防故障。

## 技术栈

- 后端：Golang
- 数据库：PostgreSQL
- 缓存：Redis
- 物联网协议：TCP、MQTT、HTTP

## 架构特性

### ✅ P0任务已完成 (2025-10-05)

- **启动顺序优化**: 确保数据库初始化后再启动TCP服务
- **参数持久化**: 设备参数存储到PostgreSQL
- **API认证**: API Key认证保护HTTP端点
- **会话Redis化**: 支持分布式会话管理和水平扩展

### 部署模式

#### 单实例模式

适合开发/测试环境或小规模部署（<10K设备）

```yaml
# config.yaml
redis:
  enabled: false  # 使用内存会话管理
```

```bash
./iot-server --config config.yaml
```

#### 多实例模式

适合生产环境，支持水平扩展（10K+设备）

```yaml
# config.yaml
redis:
  enabled: true
  addr: "redis:6379"
  pool_size: 100
```

```bash
# 启动多个实例
SERVER_ID=iot-server-1 ./iot-server --config config.yaml
SERVER_ID=iot-server-2 ./iot-server --config config.yaml
SERVER_ID=iot-server-3 ./iot-server --config config.yaml
```

### 性能优化 (Week 2)

- **连接限流**: 最大10000并发连接
- **速率限流**: 100连接/秒
- **熔断器**: 自动故障检测和恢复
- **数据库优化**: 6个新索引提升查询性能
- **Redis队列**: 10倍吞吐提升

### 监控与健康检查

```bash
# 健康检查
curl http://localhost:8080/health

# Prometheus指标
curl http://localhost:9090/metrics

# 在线设备查询
curl -H "X-API-Key: your-api-key" http://localhost:8080/api/devices/online
```

## 快速开始

### 1. 环境准备

```bash
# 安装依赖
go mod download

# 启动PostgreSQL
docker run -d -p 5432:5432 \
  -e POSTGRES_DB=iotdb \
  -e POSTGRES_USER=iot \
  -e POSTGRES_PASSWORD=secret \
  postgres:15

# （可选）启动Redis - 多实例部署需要
docker run -d -p 6379:6379 redis:7-alpine
```

### 2. 配置

复制并编辑配置文件：

```bash
cp configs/example.yaml configs/config.yaml
# 编辑 configs/config.yaml
```

### 3. 数据库迁移

```bash
# 自动执行迁移
./iot-server --config configs/config.yaml
# 或手动执行
psql -U iot -d iotdb -f db/migrations/0001_init_up.sql
```

### 4. 运行

```bash
# 单实例模式
./iot-server --config configs/config.yaml

# 多实例模式
SERVER_ID=server-1 ./iot-server --config configs/config.yaml
```

## 文档

- [项目架构设计](./docs/架构/项目架构设计.md)
- [Redis会话管理](./docs/架构/Redis会话管理.md)
- [协议对接指引](./docs/协议/)
- [P0任务完成报告](./P0任务完成报告.md)
- [下一步任务规划](./下一步任务规划.md)

## 项目状态

**当前版本**: v1.0 (P0完成)  
**架构完成度**: 100% (生产级)  
**协议完成度**: 43% (5/21命令)

详见 [项目状态报告](./项目状态报告.md)

## 开发路线图

- ✅ Week 1-3: P0任务（架构基础）
- 🔄 Week 4-5: 刷卡充电功能
- ⏳ Week 6+: 协议补全和生产部署

详见 [下一步任务规划](./下一步任务规划.md)
