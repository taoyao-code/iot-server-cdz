# GitHub Actions CI/CD 快速配置指南

> ⚡️ 5 分钟快速配置指南 - 让 CI/CD 立即运行起来！

## 🎯 最小配置方案

如果你只想快速启用 CI（代码检查和测试），无需配置任何 Secrets！

### ✅ CI 工作流已自动启用

以下功能**无需配置**即可使用：

1. **代码格式检查** - 自动检查代码格式
2. **静态分析** - Go vet 分析
3. **单元测试** - 自动运行所有测试（带 race 检测）
4. **测试覆盖率** - 生成覆盖率报告
5. **代码质量检查** - golangci-lint 检查
6. **构建验证** - 确保代码可编译
7. **Docker 镜像构建** - 验证 Docker 镜像可构建

### 🚀 立即使用

```bash
# 1. 创建测试分支
git checkout -b test/ci

# 2. 做一些修改
echo "测试 CI" >> README.md

# 3. 提交并推送
git add .
git commit -m "test: 测试 CI 功能"
git push origin test/ci

# 4. 在 GitHub 创建 PR
# 5. 查看 Actions 标签，CI 会自动运行！
```

---

## 📦 部署功能配置（可选）

如果你需要自动部署到服务器，需要配置以下 Secrets：

### 方案 A: 仅配置测试环境（推荐先配置）

**最少需要 3 个 Secrets：**

```bash
# 1. 生成 SSH 密钥
ssh-keygen -t ed25519 -f ~/.ssh/github_staging -N ""

# 2. 将公钥添加到服务器
ssh-copy-id -i ~/.ssh/github_staging.pub deploy@your-staging-server

# 3. 获取私钥内容
cat ~/.ssh/github_staging
```

**在 GitHub 添加 Secrets:**

进入仓库 → Settings → Secrets and variables → Actions → New repository secret

| Secret 名称 | 值 | 说明 |
|-----------|---|------|
| `STAGING_HOST` | `your-staging-server.com` | 测试服务器地址 |
| `STAGING_USER` | `deploy` | SSH 用户名 |
| `STAGING_SSH_KEY` | `<私钥完整内容>` | SSH 私钥 |

**创建 staging 环境:**

进入仓库 → Settings → Environments → New environment

- 名称: `staging`
- 保存即可（无需其他配置）

完成后，每次合并到 `main` 分支会自动部署到测试环境！

### 方案 B: 配置生产环境

**需要额外 3 个 Secrets：**

```bash
# 使用不同的密钥对
ssh-keygen -t ed25519 -f ~/.ssh/github_prod -N ""
ssh-copy-id -i ~/.ssh/github_prod.pub deploy@your-prod-server
cat ~/.ssh/github_prod
```

| Secret 名称 | 值 | 说明 |
|-----------|---|------|
| `PROD_HOST` | `your-prod-server.com` | 生产服务器地址 |
| `PROD_USER` | `deploy` | SSH 用户名 |
| `PROD_SSH_KEY` | `<私钥完整内容>` | SSH 私钥 |

**创建 production 环境（含审批）:**

进入仓库 → Settings → Environments → New environment

- 名称: `production`
- 勾选 "Required reviewers"
- 添加至少 1 个审批人
- Deployment branches: 选择 "Selected branches"
  - 添加: `main`
  - 添加: `tags/v*.*.*`

完成后，推送 tag 会触发生产部署，需要审批后才会执行！

---

## 🎬 完整工作流演示

### 1️⃣ 开发功能

```bash
git checkout -b feature/new-feature
# 开发代码...
git commit -m "feat: 添加新功能"
git push origin feature/new-feature
```

→ 创建 PR → CI 自动运行（5-10 分钟）

### 2️⃣ 合并到 main

```bash
# PR 审查通过后合并
```

→ 自动部署到测试环境（如已配置）

### 3️⃣ 发布生产版本

```bash
# 更新 CHANGELOG
vim CHANGELOG.md

# 创建版本标签
git tag -a v1.0.0 -m "Release v1.0.0"
git push origin v1.0.0
```

→ 触发：

- Release 工作流（创建 GitHub Release）
- Production 部署工作流（等待审批）

---

## 🔧 服务器准备

### 测试/生产服务器都需要执行

```bash
# 1. 创建部署用户
sudo useradd -m -s /bin/bash deploy
sudo usermod -aG docker deploy

# 2. 创建项目目录
sudo mkdir -p /opt/iot-server
sudo chown deploy:deploy /opt/iot-server

# 3. 创建备份目录（生产环境）
sudo mkdir -p /opt/backups
sudo chown deploy:deploy /opt/backups

# 4. 上传 docker-compose.yml 和配置文件
cd /opt/iot-server
# 上传你的 docker-compose.yml
# 上传你的 configs/production.yaml 或 configs/local.yaml
```

### 验证部署环境

```bash
# 以 deploy 用户登录
ssh deploy@your-server

# 检查 Docker 权限
docker ps
# 应该能正常运行，无需 sudo

# 检查目录权限
ls -la /opt/iot-server
# deploy 用户应该有读写权限
```

---

## ✅ 配置检查清单

### 基础 CI（无需配置）

- [x] ✅ 自动运行代码检查
- [x] ✅ 自动运行测试
- [x] ✅ 自动构建验证

### 测试环境自动部署

- [ ] `STAGING_HOST` Secret 已配置
- [ ] `STAGING_USER` Secret 已配置
- [ ] `STAGING_SSH_KEY` Secret 已配置
- [ ] `staging` Environment 已创建
- [ ] 服务器上已准备好部署目录
- [ ] 测试 SSH 连接成功

### 生产环境部署

- [ ] `PROD_HOST` Secret 已配置
- [ ] `PROD_USER` Secret 已配置
- [ ] `PROD_SSH_KEY` Secret 已配置（独立密钥）
- [ ] `production` Environment 已创建
- [ ] 配置了审批人
- [ ] 限制了部署分支
- [ ] 服务器上已准备好部署和备份目录

---

## 📚 延伸阅读

- **完整指南**: [CI/CD 使用指南](../../docs/CI-CD-GUIDE.md)
- **Secrets 配置**: [GitHub Secrets 详细配置](../../docs/GITHUB-SECRETS-SETUP.md)
- **工作流说明**: [Workflows README](.github/workflows/README.md)

---

## 🐛 常见问题

### Q: CI 运行失败怎么办？

1. 查看 Actions 页面的错误日志
2. 本地运行测试：`make test-all`
3. 检查代码格式：`make fmt-check`

### Q: 测试环境部署失败？

```bash
# 1. 检查 SSH 连接
ssh -i ~/.ssh/github_staging deploy@your-staging-server

# 2. 检查服务器日志
cd /opt/iot-server
docker-compose logs iot-server

# 3. 手动健康检查
curl http://localhost:7065/healthz
```

### Q: 如何跳过 CI 检查？

**不推荐！** 但如果必须：

```bash
git commit -m "fix: 紧急修复 [skip ci]"
```

---

## 🎉 完成

配置完成后：

- ✅ 每个 PR 自动运行 CI
- ✅ 合并到 `main` 自动部署测试环境（如已配置）
- ✅ 推送 tag 触发生产部署（需审批）

**享受自动化的便利吧！** 🚀

有问题？查看 [完整文档](../../docs/CI-CD-GUIDE.md) 或联系团队。
