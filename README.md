# IOT Server - 充电桩物联网服务器

[![Go Version](https://img.shields.io/badge/Go-1.24-blue.svg)](https://golang.org)
[![Docker](https://img.shields.io/badge/Docker-ready-brightgreen.svg)](https://www.docker.com)
[![License](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

## 📋 项目简介

IOT Server 是一个专为充电桩设备设计的高性能物联网服务器，支持多种设备通信协议，提供设备管理、数据采集、远程控制等核心功能。

### 核心特性

- 🚀 **高性能**：支持 50,000+ 并发设备连接
- 🔌 **多协议支持**：AP3000、BKV、GN 等主流充电桩协议
- 💾 **数据持久化**：PostgreSQL + Redis 双存储
- 📡 **实时通信**：TCP 长连接 + WebSocket
- 🔐 **安全可靠**：API 认证、数据加密、会话管理
- 📊 **监控完善**：Prometheus 指标、健康检查、日志记录
- 🔄 **第三方集成**：Webhook 事件推送、API 对接
- 🛡️ **高可用**：熔断保护、限流、自动重连

---

## 🚀 快速开始

### 系统要求

- Docker 20.10+
- Docker Compose 2.0+
- 8GB+ RAM
- 100GB+ 磁盘空间

### 一键部署

```bash
# 1. 克隆项目
git clone <repository-url>
cd iot-server

# 2. 配置环境变量
cp scripts/env.example .env
vim .env  # 修改数据库密码、API密钥等配置

# 3. 执行部署
make deploy

# 4. 验证服务
curl http://localhost:7055/healthz
```

**就这么简单！** 🎉

部署脚本会自动：

- ✅ 检查环境依赖
- ✅ 构建 Docker 镜像
- ✅ 启动所有服务
- ✅ 初始化数据库
- ✅ 执行健康检查

---

## 📖 文档导航

### 快速入门

- [部署指南](DEPLOYMENT.md) - 完整的部署说明
- [配置说明](configs/example.yaml) - 配置文件详解
- [API 文档](docs/api/) - HTTP API 接口文档

### 开发指南

- [项目架构](docs/架构/项目架构设计.md) - 系统架构设计
- [协议文档](docs/协议/) - 设备通信协议
- [数据安全说明](docs/数据安全与部署说明.md) - 数据持久化机制

### 运维指南

- [CI/CD 指南](docs/CI-CD-GUIDE.md) - 自动化部署
- [监控运维](DEPLOYMENT.md#监控运维) - 监控和告警配置
- [故障排查](DEPLOYMENT.md#故障排查) - 常见问题解决

---

## 🏗️ 项目结构

```
iot-server/
├── cmd/                    # 应用入口
│   └── server/            # 主程序
├── internal/              # 内部包
│   ├── api/              # HTTP API 路由和处理器
│   ├── app/              # 应用引导和依赖注入
│   ├── gateway/          # TCP 网关
│   ├── protocol/         # 协议解析器
│   │   ├── ap3000/      # AP3000 协议
│   │   ├── bkv/         # BKV 协议
│   │   └── gn/          # GN 组网协议
│   ├── session/         # 会话管理
│   ├── storage/         # 数据存储
│   └── thirdparty/      # 第三方集成
├── configs/              # 配置文件
├── db/migrations/        # 数据库迁移
├── scripts/             # 部署脚本
├── docs/                # 文档
└── docker-compose.yml   # 容器编排
```

---

## 🔧 开发

### 本地开发

```bash
# 安装依赖
go mod download

# 运行测试
make test

# 代码检查
make lint

# 启动开发环境
make compose-up

# 启动服务器
make run
```

### 代码规范

```bash
# 格式化代码
make fmt

# 静态分析
make vet

# 运行所有检查
make ci-check
```

---

## 📊 性能指标

| 指标 | 数值 |
|------|------|
| 最大并发连接 | 50,000+ |
| HTTP API QPS | 10,000+ |
| TCP 消息吞吐 | 100,000+/秒 |
| 平均响应时间 | < 50ms |
| 内存占用 | < 2GB |

---

## 🔒 安全特性

- ✅ API Key 认证
- ✅ HMAC 签名验证
- ✅ TLS/SSL 加密传输
- ✅ 限流和熔断保护
- ✅ SQL 注入防护
- ✅ XSS 防护

---

## 📈 监控和可观测

### Prometheus 指标

- 设备连接数
- HTTP 请求 QPS
- 协议消息计数
- 错误率统计
- 响应时间分布

### 健康检查

```bash
# 存活检查
curl http://localhost:7055/healthz

# 就绪检查
curl http://localhost:7055/readyz

# Prometheus 指标
curl http://localhost:7055/metrics
```

---

## 🚀 部署选项

### Docker Compose（推荐）

适合单机或小规模部署：

```bash
make deploy
```

### CI/CD 自动化

支持 GitHub Actions 自动化部署：

```bash
# 测试环境：提交到 main 分支自动部署
git push origin main

# 生产环境：创建版本标签触发部署（需审批）
git tag -a v1.2.3 -m "Release v1.2.3"
git push origin v1.2.3
```

查看 [CI/CD 指南](docs/CI-CD-GUIDE.md) 了解详情。

---

## 🛠️ 常用命令

### 部署相关

```bash
make deploy          # 安全部署（自动备份+零停机）
make docker-build    # 构建 Docker 镜像
make prod-restart    # 重启服务
make prod-logs       # 查看日志
make backup          # 备份数据
```

### 开发相关

```bash
make build           # 构建应用
make test            # 运行测试
make test-coverage   # 测试覆盖率
make fmt             # 格式化代码
make lint            # 代码检查
```

### 环境管理

```bash
make compose-up      # 启动开发环境
make compose-down    # 停止开发环境
make clean           # 清理构建文件
make clean-all       # 深度清理（包括数据）
```

完整命令列表：`make help`

---

## 🔄 更新部署

### 日常更新

```bash
# 1. 拉取最新代码
git pull origin main

# 2. 安全部署（零停机）
make deploy

# 部署特性：
# ✅ 自动备份数据库
# ✅ 智能检测（首次/更新）
# ✅ 零停机更新
# ✅ 失败自动回滚
```

### 版本发布

```bash
# 1. 更新 CHANGELOG.md
vim CHANGELOG.md

# 2. 创建版本标签
git tag -a v1.2.3 -m "Release v1.2.3"
git push origin v1.2.3

# 3. 自动触发 CI/CD 部署（如已配置）
```

---

## 🐛 故障排查

### 查看日志

```bash
# 应用日志
make prod-logs

# 数据库日志
docker-compose logs postgres

# Redis 日志
docker-compose logs redis
```

### 健康检查

```bash
# 检查服务状态
docker-compose ps

# 健康检查
curl http://localhost:7055/healthz

# 详细诊断
curl http://localhost:7055/readyz
```

### 数据备份恢复

```bash
# 备份
make backup

# 恢复
make restore

# 手动备份
docker-compose exec postgres pg_dump -U iot iot_server > backup.sql
```

更多问题请查看 [故障排查文档](DEPLOYMENT.md#故障排查)

---

## 🤝 贡献指南

欢迎贡献代码、报告问题或提出建议！

1. Fork 本仓库
2. 创建特性分支 (`git checkout -b feature/amazing-feature`)
3. 提交更改 (`git commit -m 'feat: Add amazing feature'`)
4. 推送到分支 (`git push origin feature/amazing-feature`)
5. 创建 Pull Request

请遵循 [代码规范](docs/CODE_STYLE.md) 和 [提交规范](https://www.conventionalcommits.org/)

---

## 📞 技术支持

- 📧 Email: <support@example.com>
- 📖 文档: [完整文档](docs/)
- 🐛 Issues: [GitHub Issues](https://github.com/your-org/iot-server/issues)

---

## 📄 许可证

本项目采用 MIT 许可证 - 查看 [LICENSE](LICENSE) 文件了解详情

---

## 🙏 致谢

感谢所有贡献者和开源项目的支持！

---

**Made with ❤️ by IOT Team**
