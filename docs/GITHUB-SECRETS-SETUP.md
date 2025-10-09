# GitHub Secrets 配置详细指南

## 📋 概述

本文档详细说明如何配置 GitHub Actions 所需的所有 Secrets 和 Environments。

---

## 🔐 配置 GitHub Secrets

### 步骤 1: 进入 Secrets 配置页面

1. 打开 GitHub 仓库页面
2. 点击 **Settings** 标签
3. 左侧菜单选择 **Secrets and variables** → **Actions**
4. 点击 **New repository secret** 按钮

### 步骤 2: 配置必需的 Secrets

#### 1️⃣ Docker Registry 配置

##### DOCKER_USERNAME

- **名称**: `DOCKER_USERNAME`
- **值**: 你的 Docker Hub 用户名
- **示例**: `myusername`

##### DOCKER_PASSWORD

- **名称**: `DOCKER_PASSWORD`
- **值**: Docker Hub Access Token（不是密码！）

**如何获取 Docker Hub Token:**

```bash
# 方法 1: Web 界面
1. 登录 https://hub.docker.com/
2. 点击右上角头像 → Account Settings
3. 左侧菜单选择 Security
4. 点击 "New Access Token"
5. Token description: "github-actions-ci"
6. Access permissions: Read & Write
7. Generate → 复制 Token（只显示一次！）

# 方法 2: CLI 命令
docker login
# 输入用户名和密码后，token 会保存在 ~/.docker/config.json
```

**如果使用阿里云镜像仓库:**

```yaml
DOCKER_USERNAME: 阿里云账号完整名称
DOCKER_PASSWORD: 阿里云镜像服务密码（独立密码）
DOCKER_REGISTRY: registry.cn-hangzhou.aliyuncs.com
```

#### 2️⃣ 测试环境（Staging）配置

##### STAGING_HOST

- **名称**: `STAGING_HOST`
- **值**: 测试服务器 IP 或域名
- **示例**:
  - `192.168.1.100` (内网 IP)
  - `staging.example.com` (域名)

##### STAGING_USER

- **名称**: `STAGING_USER`
- **值**: SSH 登录用户名
- **推荐**: `deploy` (专用部署用户)

##### STAGING_SSH_KEY

- **名称**: `STAGING_SSH_KEY`
- **值**: SSH 私钥（完整内容）

**如何生成和配置 SSH 密钥:**

```bash
# 1. 生成密钥对（在你的本地机器）
ssh-keygen -t ed25519 -C "github-actions-staging" -f ~/.ssh/github_staging_key
# 提示输入密码时直接回车（不设密码）

# 2. 查看公钥（需要添加到服务器）
cat ~/.ssh/github_staging_key.pub

# 3. 查看私钥（需要添加到 GitHub Secrets）
cat ~/.ssh/github_staging_key

# 4. 将公钥添加到测试服务器
# 方法 A: 使用 ssh-copy-id
ssh-copy-id -i ~/.ssh/github_staging_key.pub deploy@staging-server

# 方法 B: 手动添加
ssh deploy@staging-server
mkdir -p ~/.ssh
chmod 700 ~/.ssh
vim ~/.ssh/authorized_keys
# 粘贴公钥内容
chmod 600 ~/.ssh/authorized_keys
exit

# 5. 测试连接
ssh -i ~/.ssh/github_staging_key deploy@staging-server
```

**私钥格式示例:**

```
-----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtz
c2gtZWQyNTUxOQAAACDGvX2YL3vMvhJ2wNjCqB5rJB3qX4kF3nLV6xJ2VqzYmA
...（更多内容）...
AAACBAgMCAQIDBAMEBAIDBAABAg==
-----END OPENSSH PRIVATE KEY-----
```

⚠️ **重要事项:**

- 必须包含完整的 `-----BEGIN` 和 `-----END` 行
- 保留所有换行符
- 不要添加任何额外的空格或注释

##### STAGING_PORT（可选）

- **名称**: `STAGING_PORT`
- **值**: SSH 端口号
- **默认**: `22`
- **仅在使用非标准端口时配置**

#### 3️⃣ 生产环境（Production）配置

##### PROD_HOST

- **名称**: `PROD_HOST`
- **值**: 生产服务器 IP 或域名
- **示例**: `192.168.1.200` 或 `prod.example.com`

##### PROD_USER

- **名称**: `PROD_USER`
- **值**: SSH 登录用户名
- **推荐**: `deploy`

##### PROD_SSH_KEY

- **名称**: `PROD_SSH_KEY`
- **值**: SSH 私钥（完整内容）

⚠️ **安全建议:** 生产环境应使用与测试环境不同的 SSH 密钥

```bash
# 为生产环境生成单独的密钥对
ssh-keygen -t ed25519 -C "github-actions-production" -f ~/.ssh/github_prod_key
```

##### PROD_PORT（可选）

- **名称**: `PROD_PORT`
- **值**: SSH 端口号
- **默认**: `22`

##### PROD_DOMAIN（可选）

- **名称**: `PROD_DOMAIN`
- **值**: 生产环境访问域名（用于通知和访问链接）
- **示例**: `iot.example.com`

---

## 🌍 配置 GitHub Environments

Environments 用于配置部署审批流程和环境特定的 secrets。

### 步骤 1: 进入 Environments 配置页面

1. 打开 GitHub 仓库页面
2. 点击 **Settings** 标签
3. 左侧菜单选择 **Environments**

### 步骤 2: 创建 Staging 环境

1. 点击 **New environment**
2. Name: `staging`
3. 点击 **Configure environment**

**配置项:**

- ✅ **Environment protection rules**: 不勾选（测试环境无需审批）
- ✅ **Deployment branches**: All branches（允许所有分支部署）
- ✅ **Environment secrets**: 可选，用于测试环境特定配置

### 步骤 3: 创建 Production 环境

1. 点击 **New environment**
2. Name: `production`
3. 点击 **Configure environment**

**配置项:**

#### ✅ Required reviewers（必需审批人）

- 勾选 **Required reviewers**
- 添加至少 1 个审批人（团队 Lead 或技术负责人）
- **最多可添加 6 人**

**审批流程:**

- 当部署到生产环境时，会自动暂停
- 发送通知给审批人
- 审批人必须在 GitHub 页面手动批准
- 批准后才会继续部署

#### ✅ Wait timer（等待时间）

- 设置为 `0` minutes
- 或根据需要设置等待时间（例如 15 分钟观察期）

#### ✅ Deployment branches（部署分支限制）

- 选择 **Selected branches**
- 添加规则:
  - `main` (主分支)
  - `tags/v*.*.*` (版本标签)

**为什么限制分支:**

- 防止误操作从错误的分支部署到生产环境
- 确保只有经过测试的代码才能部署

#### ✅ Environment secrets（可选）

如果生产环境有特定配置，可以在这里添加：

- `DATABASE_URL`: 生产数据库连接串
- `REDIS_URL`: 生产 Redis 地址
- `WEBHOOK_URL`: 生产环境 Webhook
- 等等...

**Environment secrets 优先级高于 Repository secrets**

---

## 🔍 验证配置

### 1. 验证 Docker 登录

```bash
# 使用你的凭证测试
echo "$DOCKER_PASSWORD" | docker login -u "$DOCKER_USERNAME" --password-stdin

# 应该显示: Login Succeeded
```

### 2. 验证 SSH 连接

```bash
# 测试环境
ssh -i ~/.ssh/github_staging_key deploy@$STAGING_HOST

# 生产环境
ssh -i ~/.ssh/github_prod_key deploy@$PROD_HOST

# 测试 Docker 权限
docker ps
```

### 3. 验证服务器目录

```bash
# 在测试服务器上
ls -la /opt/iot-server
ls -la /opt/backups  # 生产环境

# 确保 deploy 用户有写权限
```

### 4. 运行测试 Workflow

创建一个测试 PR 触发 CI:

```bash
git checkout -b test/ci-setup
echo "test" >> README.md
git commit -am "test: 测试 CI 配置"
git push origin test/ci-setup
```

然后在 GitHub 创建 PR，查看 Actions 是否正常运行。

---

## 📝 配置清单

在配置完成后，使用此清单验证：

### Docker Registry

- [ ] ✅ DOCKER_USERNAME 已配置
- [ ] ✅ DOCKER_PASSWORD 已配置
- [ ] ✅ 本地测试登录成功

### 测试环境

- [ ] ✅ STAGING_HOST 已配置
- [ ] ✅ STAGING_USER 已配置
- [ ] ✅ STAGING_SSH_KEY 已配置
- [ ] ✅ STAGING_PORT 已配置（如果需要）
- [ ] ✅ SSH 连接测试成功
- [ ] ✅ deploy 用户在 docker 组
- [ ] ✅ /opt/iot-server 目录存在
- [ ] ✅ docker-compose.yml 已上传

### 生产环境

- [ ] ✅ PROD_HOST 已配置
- [ ] ✅ PROD_USER 已配置
- [ ] ✅ PROD_SSH_KEY 已配置（独立密钥）
- [ ] ✅ PROD_PORT 已配置（如果需要）
- [ ] ✅ PROD_DOMAIN 已配置（可选）
- [ ] ✅ SSH 连接测试成功
- [ ] ✅ 防火墙规则已配置
- [ ] ✅ 备份目录已创建

### GitHub Environments

- [ ] ✅ staging 环境已创建
- [ ] ✅ production 环境已创建
- [ ] ✅ production 配置了审批人
- [ ] ✅ production 限制了部署分支

### 测试验证

- [ ] ✅ CI workflow 可以正常运行
- [ ] ✅ Docker 镜像可以成功构建
- [ ] ✅ 测试可以通过

---

## ⚠️ 常见问题

### Q1: SSH 连接失败 - Permission denied

**症状:**

```
Permission denied (publickey).
```

**解决方案:**

1. 检查私钥格式是否完整
2. 确保服务器 `authorized_keys` 包含对应公钥
3. 检查文件权限:

   ```bash
   chmod 700 ~/.ssh
   chmod 600 ~/.ssh/authorized_keys
   ```

4. 查看服务器 SSH 日志:

   ```bash
   sudo tail -f /var/log/auth.log  # Ubuntu/Debian
   sudo tail -f /var/log/secure     # CentOS/RHEL
   ```

### Q2: Docker 登录失败

**症状:**

```
Error response from daemon: Get https://registry-1.docker.io/v2/: unauthorized
```

**解决方案:**

1. 确认使用的是 Access Token 而不是密码
2. 检查 Token 权限是否包含 Read & Write
3. Token 是否已过期
4. 用户名是否正确（Docker Hub 用户名区分大小写）

### Q3: 环境审批人无法收到通知

**解决方案:**

1. 确保审批人已启用 GitHub 通知
2. 检查 Settings → Notifications → Actions
3. 审批人需要有仓库的写权限

### Q4: 部署失败 - 无法拉取镜像

**解决方案:**

1. 检查服务器是否登录了 Docker Registry
2. 检查镜像名称是否正确
3. 检查网络连接
4. 手动测试:

   ```bash
   docker pull username/iot-server:staging-latest
   ```

---

## 🔒 安全最佳实践

### 1. SSH 密钥管理

- ✅ 为 CI/CD 创建专用密钥对
- ✅ 生产和测试环境使用不同密钥
- ✅ 定期轮换密钥（建议每季度）
- ✅ 密钥不设置密码（GitHub Actions 无法输入密码）
- ✅ 使用 ed25519 算法（比 RSA 更安全更快）

### 2. Docker Credentials

- ✅ 使用 Access Token 而不是密码
- ✅ 限制 Token 权限（只授予必要的权限）
- ✅ 定期更新 Token
- ✅ 不要在代码中硬编码凭证

### 3. 服务器安全

- ✅ 创建专用的 deploy 用户
- ✅ 限制 deploy 用户权限（最小权限原则）
- ✅ 使用防火墙限制 SSH 访问
- ✅ 启用 SSH 密钥认证，禁用密码登录
- ✅ 定期审查 `authorized_keys` 文件

### 4. GitHub Secrets

- ✅ 只添加必要的 Secrets
- ✅ 使用 Environment secrets 隔离环境
- ✅ 定期审计 Secrets 使用情况
- ✅ 离职员工及时移除审批权限

---

## 📚 相关文档

- [GitHub Actions Secrets 官方文档](https://docs.github.com/en/actions/security-guides/encrypted-secrets)
- [GitHub Environments 官方文档](https://docs.github.com/en/actions/deployment/targeting-different-environments/using-environments-for-deployment)
- [Docker Hub Token 文档](https://docs.docker.com/docker-hub/access-tokens/)
- [SSH 密钥管理最佳实践](https://www.ssh.com/academy/ssh/keygen)

---

## 💬 需要帮助？

如果遇到问题：

1. 查看本文档的 [常见问题](#常见问题) 部分
2. 查看 [CI/CD 使用指南](./CI-CD-GUIDE.md)
3. 搜索 GitHub Actions 日志中的错误信息
4. 联系团队运维人员

配置完成后，你就可以享受自动化部署的便利了！🚀
