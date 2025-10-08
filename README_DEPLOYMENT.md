# 🚀 IOT Server 生产环境快速部署指南

## 📋 部署概览

本指南提供IOT服务器生产环境的快速部署步骤。

## ⚡ 5分钟快速部署

### 1. 准备工作（2分钟）

```bash
# 克隆代码
git clone https://github.com/your-org/iot-server.git
cd iot-server

# 复制环境变量模板
cp scripts/env.example .env
```

### 2. 配置环境变量（2分钟）

编辑 `.env` 文件，修改以下必需项：

```bash
# 数据库密码
POSTGRES_PASSWORD=你的强密码

# Redis密码
REDIS_PASSWORD=你的Redis密码

# API密钥（生成强密钥）
API_KEY=$(openssl rand -base64 32)
THIRDPARTY_API_KEY=$(openssl rand -base64 32)
```

### 3. 一键部署（1分钟）

```bash
# 执行自动化部署脚本
make deploy
```

部署脚本自动完成：
✅ 环境检查  
✅ Docker镜像构建  
✅ 数据库初始化  
✅ 服务启动  
✅ 健康检查  

### 4. 验证部署

```bash
# 检查服务状态
curl http://localhost:8080/healthz

# 查看服务状态
make prod-status
```

## 📚 完整文档

- [详细部署指南](./DEPLOYMENT.md)
- [部署检查清单](./DEPLOYMENT_CHECKLIST.md)
- [API文档](./api/openapi/openapi.yaml)

## 🔧 常用命令

```bash
# 构建镜像
make docker-build

# 启动服务
make prod-up

# 查看日志
make prod-logs

# 重启服务
make prod-restart

# 停止服务
make prod-down

# 备份数据
make backup

# 查看帮助
make help
```

## 🆘 获取帮助

遇到问题？查看：
1. [故障排查](./DEPLOYMENT.md#故障排查)
2. [部署检查清单](./DEPLOYMENT_CHECKLIST.md)
3. 联系技术支持

---

**快速开始生产环境部署！** 🎉
