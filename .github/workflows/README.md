# GitHub Actions 工作流说明

本目录包含所有 GitHub Actions CI/CD 工作流配置文件。

## 📋 工作流列表

### 1. CI - 持续集成 (`ci.yml`)

**触发条件：**

- Push 到 `main` 或 `develop` 分支
- 针对 `main` 或 `develop` 的 Pull Request
- 手动触发

**执行内容：**

- ✅ 代码格式检查 (gofmt)
- ✅ 静态分析 (go vet)
- ✅ 单元测试 (带 race 检测)
- ✅ 测试覆盖率报告
- ✅ 代码质量检查 (golangci-lint)
- ✅ 构建验证
- ✅ Docker 镜像构建

**依赖服务：**

- PostgreSQL 15
- Redis 7

### 2. Deploy - 测试环境 (`deploy-staging.yml`)

**触发条件：**

- Push 到 `main` 分支（自动）
- 手动触发（可指定版本）

**执行内容：**

- 构建 Linux 二进制文件
- 部署到测试服务器
- 自动健康检查
- 失败自动回滚

**所需 Secrets：**

- `STAGING_HOST` - 测试服务器地址
- `STAGING_USER` - SSH 用户名
- `STAGING_SSH_KEY` - SSH 私钥
- `STAGING_PORT` - SSH 端口（可选，默认 22）

### 3. Deploy - 生产环境 (`deploy-production.yml`)

**触发条件：**

- Push 版本标签 (`v*.*.*`)
- 手动触发（需指定版本）

**执行内容：**

- 版本号验证
- 运行完整测试套件
- 构建生产版本
- 创建备份
- 部署到生产服务器
- 健康检查（6 次重试）
- 失败自动回滚

**所需 Secrets：**

- `PROD_HOST` - 生产服务器地址
- `PROD_USER` - SSH 用户名
- `PROD_SSH_KEY` - SSH 私钥
- `PROD_PORT` - SSH 端口（可选，默认 22）

**Environment：**

- 需要配置 `production` 环境
- 建议启用人工审批

### 4. Release - 版本发布 (`release.yml`)

**触发条件：**

- Push 版本标签 (`v*.*.*`)

**执行内容：**

- 构建多平台二进制文件：
  - Linux (amd64, arm64)
  - macOS (amd64, arm64)
- 生成 SHA256 校验和
- 从 CHANGELOG.md 提取发布说明
- 创建 GitHub Release
- 上传构建产物

## 🚀 使用指南

### 日常开发流程

1. **创建功能分支**

   ```bash
   git checkout -b feature/new-feature
   ```

2. **开发并提交代码**

   ```bash
   git add .
   git commit -m "feat: 添加新功能"
   git push origin feature/new-feature
   ```

3. **创建 Pull Request**
   - CI 会自动运行所有检查
   - 通过后即可合并到 `main`

4. **合并到 main**
   - 自动触发测试环境部署

### 发布生产版本

1. **确保代码已在测试环境验证**

2. **更新 CHANGELOG.md**

   ```bash
   vim CHANGELOG.md
   git add CHANGELOG.md
   git commit -m "docs: 更新 CHANGELOG v1.2.3"
   git push
   ```

3. **创建版本标签**

   ```bash
   git tag -a v1.2.3 -m "Release v1.2.3"
   git push origin v1.2.3
   ```

4. **等待工作流执行**
   - Release 工作流创建 GitHub Release
   - Production 部署等待审批（如已配置）

5. **审批生产部署**（如已配置）
   - 进入 Actions → 选择对应的 workflow run
   - Review deployments → Approve

## ⚙️ 配置说明

### GitHub Secrets 配置

进入仓库 **Settings** → **Secrets and variables** → **Actions**

#### 必需的 Secrets

测试环境：

```
STAGING_HOST=your-staging-server.com
STAGING_USER=deploy
STAGING_SSH_KEY=<SSH私钥内容>
```

生产环境：

```
PROD_HOST=your-production-server.com
PROD_USER=deploy
PROD_SSH_KEY=<SSH私钥内容>
```

### GitHub Environments 配置

#### 创建 `production` 环境

1. 进入 **Settings** → **Environments** → **New environment**
2. 名称：`production`
3. 配置：
   - ✅ Required reviewers: 添加至少 1 个审批人
   - ✅ Wait timer: 0 minutes（可选）
   - ✅ Deployment branches: 限制为 `main` 分支和 `v*.*.*` 标签

#### 创建 `staging` 环境（可选）

1. 名称：`staging`
2. 配置：
   - 不需要审批人
   - 允许所有分支

### 服务器准备

在部署服务器上执行：

```bash
# 1. 创建部署用户
sudo useradd -m -s /bin/bash deploy
sudo usermod -aG docker deploy

# 2. 设置 SSH 密钥认证
sudo -u deploy mkdir -p /home/deploy/.ssh
# 将 GitHub Actions 的公钥添加到 authorized_keys
sudo -u deploy vim /home/deploy/.ssh/authorized_keys
sudo chmod 700 /home/deploy/.ssh
sudo chmod 600 /home/deploy/.ssh/authorized_keys

# 3. 创建项目目录
sudo mkdir -p /opt/iot-server
sudo mkdir -p /opt/backups
sudo chown deploy:deploy /opt/iot-server /opt/backups

# 4. 部署 docker-compose.yml 和配置文件
cd /opt/iot-server
# 上传 docker-compose.yml, configs/production.yaml 等
```

## 🔧 本地测试

在推送前本地验证：

```bash
# 代码格式检查
make fmt-check

# 运行测试
make test-all

# 构建验证
make build
```

## 📊 工作流状态查看

### 通过 GitHub 网页

1. 进入仓库 → **Actions** 标签
2. 查看工作流运行历史
3. 点击具体的 run 查看详情

### 通过 GitHub CLI

```bash
# 查看最近的工作流运行
gh run list

# 查看具体工作流详情
gh run view <run-id>

# 查看工作流日志
gh run view <run-id> --log
```

## 🐛 故障排查

### CI 失败

**1. 测试失败**

- 查看 Actions 日志中的具体错误
- 本地运行 `make test-all` 重现问题

**2. 代码格式问题**

```bash
# 自动修复
make fmt

# 重新提交
git add .
git commit --amend --no-edit
git push -f
```

**3. 构建失败**

```bash
# 清理缓存
go clean -cache -modcache

# 重新构建
make build
```

### 部署失败

**1. SSH 连接失败**

- 检查 Secret 中的 SSH_KEY 格式（需包含完整的 BEGIN/END 标记）
- 验证服务器上的 authorized_keys 配置

**2. 健康检查失败**

- SSH 到服务器查看容器日志：

  ```bash
  ssh deploy@server
  cd /opt/iot-server
  docker-compose logs iot-server
  ```

**3. 回滚**

- 系统会自动回滚
- 如需手动回滚，SSH 到服务器：

  ```bash
  cd /opt/iot-server
  cp /opt/backups/latest/iot-server.backup ./iot-server
  docker-compose restart iot-server
  ```

## 📝 最佳实践

### 提交规范

使用 [Conventional Commits](https://www.conventionalcommits.org/)：

```
feat: 添加新功能
fix: 修复 Bug
docs: 文档更新
style: 代码格式调整
refactor: 代码重构
test: 测试相关
chore: 构建/工具链相关
```

### 版本管理

遵循 [语义化版本](https://semver.org/)：

```
MAJOR.MINOR.PATCH
1.2.3

MAJOR: 不兼容的 API 修改
MINOR: 向下兼容的功能新增
PATCH: 向下兼容的问题修正
```

### 部署时机

- **测试环境**: 每次合并到 `main` 自动部署
- **生产环境**: 选择业务低峰期，避开周五和节假日

## 🔗 相关资源

- [CI/CD 完整指南](../../docs/CI-CD-GUIDE.md)
- [GitHub Secrets 配置指南](../../docs/GITHUB-SECRETS-SETUP.md)
- [GitHub Actions 官方文档](https://docs.github.com/actions)

## 📞 支持

如有问题，请：

1. 查看 Actions 日志
2. 检查本文档的故障排查章节
3. 联系开发团队
