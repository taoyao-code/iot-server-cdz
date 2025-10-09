# CI/CD 使用指南

## 📚 目录

- [概述](#概述)
- [工作流程](#工作流程)
- [环境配置](#环境配置)
- [使用说明](#使用说明)
- [故障排查](#故障排查)
- [最佳实践](#最佳实践)

---

## 概述

本项目使用 **GitHub Actions** 实现完整的 CI/CD 流水线，包括：

| 工作流 | 触发条件 | 说明 |
|--------|---------|------|
| **CI - 持续集成** | Push/PR 到 main/develop | 代码检查、测试、构建验证 |
| **Deploy - 测试环境** | Push 到 main 分支 | 自动部署到 staging 环境 |
| **Deploy - 生产环境** | 创建版本标签 (v*.*.*) | 审批后部署到生产环境 |
| **Release - 版本发布** | 创建版本标签 | 自动创建 GitHub Release |

---

## 工作流程

### 🔄 开发流程

```mermaid
graph LR
    A[开发分支] -->|提交代码| B[创建 PR]
    B -->|自动触发| C[CI 检查]
    C -->|通过| D[代码审查]
    D -->|合并| E[main 分支]
    E -->|自动触发| F[部署测试环境]
    F -->|验证通过| G[打版本标签]
    G -->|触发| H[等待审批]
    H -->|批准| I[部署生产环境]
```

### 📝 详细步骤

#### 1️⃣ 开发阶段

```bash
# 创建功能分支
git checkout -b feature/new-feature

# 开发并提交代码
git add .
git commit -m "feat: 添加新功能"
git push origin feature/new-feature

# 创建 Pull Request
# 👉 GitHub 会自动运行 CI 检查
```

**CI 会执行：**

- ✅ 代码格式检查 (gofmt, golangci-lint)
- ✅ 静态分析 (go vet)
- ✅ 单元测试 (go test)
- ✅ 测试覆盖率检查
- ✅ 构建验证
- ✅ Docker 镜像构建
- ✅ 安全漏洞扫描

#### 2️⃣ 合并到 main

```bash
# PR 通过审查后合并
# 👉 自动触发测试环境部署
```

**部署流程：**

1. 构建 Docker 镜像（带 `staging-*` 标签）
2. 推送到镜像仓库
3. SSH 连接到测试服务器
4. 拉取最新镜像
5. 滚动更新服务
6. 执行健康检查
7. 发送部署通知

#### 3️⃣ 发布生产版本

```bash
# 确保 main 分支代码已在测试环境验证

# 创建版本标签（语义化版本）
git tag -a v1.2.3 -m "Release v1.2.3"
git push origin v1.2.3

# 👉 触发生产部署流程（需要人工审批）
```

**生产部署流程：**

1. ✅ 预检查（版本号、CHANGELOG）
2. ✅ 构建生产镜像（带版本号标签）
3. ✅ 安全扫描（严格模式）
4. ✅ 冒烟测试
5. ⏸️ **等待人工审批**
6. ✅ 备份当前环境
7. ✅ 蓝绿部署
8. ✅ 健康检查
9. ✅ 监控指标验证
10. ✅ 创建 GitHub Release
11. ✅ 清理旧版本

---

## 环境配置

### 1. GitHub Secrets 配置

进入仓库 **Settings** → **Secrets and variables** → **Actions** → **New repository secret**

#### 必需配置（8个）

| Secret 名称 | 说明 | 获取方式 |
|------------|------|---------|
| `DOCKER_USERNAME` | Docker Hub 用户名 | Docker Hub 账号 |
| `DOCKER_PASSWORD` | Docker Hub Token | [生成方法](#docker-token) |
| `STAGING_HOST` | 测试服务器地址 | 服务器 IP 或域名 |
| `STAGING_USER` | 测试服务器用户 | SSH 用户名 |
| `STAGING_SSH_KEY` | 测试服务器密钥 | [生成方法](#ssh-key) |
| `PROD_HOST` | 生产服务器地址 | 服务器 IP 或域名 |
| `PROD_USER` | 生产服务器用户 | SSH 用户名 |
| `PROD_SSH_KEY` | 生产服务器密钥 | [生成方法](#ssh-key) |

#### 可选配置

| Secret 名称 | 说明 |
|------------|------|
| `STAGING_PORT` | SSH 端口（默认 22） |
| `PROD_PORT` | SSH 端口（默认 22） |
| `PROD_DOMAIN` | 生产域名（用于通知） |
| `CODECOV_TOKEN` | 代码覆盖率上传 |
| `SLACK_WEBHOOK_URL` | Slack 通知 |

### 2. GitHub Environments 配置

#### 创建环境

进入 **Settings** → **Environments** → **New environment**

**创建两个环境：**

##### staging（测试环境）

- ✅ Environment protection rules: 无
- ✅ Environment secrets: 可选

##### production（生产环境）

- ✅ **Required reviewers**: 添加团队 Lead（至少1人）
- ✅ **Wait timer**: 0 minutes
- ✅ **Deployment branches**: Selected branches
  - 允许: `main` 和 `tags/v*.*.*`
- ✅ Environment secrets: 生产环境专用配置

### 3. 服务器准备

#### 测试服务器

```bash
# 1. 创建部署用户
sudo useradd -m -s /bin/bash deploy
sudo usermod -aG docker deploy

# 2. 设置 SSH 密钥认证
sudo -u deploy mkdir -p /home/deploy/.ssh
sudo -u deploy vim /home/deploy/.ssh/authorized_keys
# 粘贴 GitHub Actions 使用的公钥
sudo chmod 700 /home/deploy/.ssh
sudo chmod 600 /home/deploy/.ssh/authorized_keys

# 3. 创建项目目录
sudo mkdir -p /opt/iot-server
sudo chown deploy:deploy /opt/iot-server

# 4. 部署配置文件
cd /opt/iot-server
# 上传 docker-compose.yml 和 .env 文件
```

#### 生产服务器

```bash
# 同测试服务器，额外配置：

# 1. 设置防火墙
sudo ufw allow 22/tcp
sudo ufw allow 7054/tcp  # HTTP API
sudo ufw allow 7055/tcp  # TCP 设备端口
sudo ufw enable

# 2. 配置备份目录
sudo mkdir -p /opt/backups
sudo chown deploy:deploy /opt/backups

# 3. 设置日志轮转
sudo vim /etc/logrotate.d/iot-server
```

---

## 使用说明

### 日常开发

#### 查看 CI 状态

```bash
# 方法 1: GitHub 网页
# 进入仓库 → Actions 标签

# 方法 2: GitHub CLI
gh run list
gh run view <run-id>
```

#### 本地预检查

```bash
# 在提交前本地运行检查
make fmt      # 格式化代码
make vet      # 静态分析
make test     # 运行测试
make lint     # 代码检查
make build    # 构建验证

# 一键执行所有检查
make all
```

### 部署操作

#### 手动触发测试环境部署

```bash
# 方法 1: GitHub 网页
# Actions → Deploy - 测试环境 → Run workflow

# 方法 2: GitHub CLI
gh workflow run deploy-staging.yml
```

#### 生产环境发布流程

```bash
# 1. 确保代码已合并到 main 并在测试环境验证
# 2. 更新 CHANGELOG.md
vim CHANGELOG.md

# 3. 创建版本标签
git tag -a v1.2.3 -m "Release v1.2.3

- 新增功能 A
- 修复问题 B
- 优化性能 C
"

# 4. 推送标签（触发部署流程）
git push origin v1.2.3

# 5. 在 GitHub Actions 页面等待审批
# 6. 审批后自动部署到生产环境
# 7. 部署完成后验证服务
curl https://iot.example.com/healthz
```

#### 生产环境审批

1. 进入 **Actions** → 找到等待审批的部署
2. 点击 **Review deployments**
3. 选择 **production** 环境
4. 查看部署详情和测试结果
5. 点击 **Approve and deploy** 或 **Reject**

### 回滚操作

#### 自动回滚

如果部署后健康检查失败，系统会**自动回滚**到上一个版本。

#### 手动回滚

```bash
# 方法 1: 通过标签重新部署旧版本
# GitHub Actions → Deploy - 生产环境 → Run workflow
# 输入: version = v1.2.2 (旧版本)

# 方法 2: SSH 到服务器手动回滚
ssh deploy@prod-server

# 查看可用版本
docker images iot-server

# 回滚到指定版本
docker tag iot-server:v1.2.2 iot-server:latest
cd /opt/iot-server
docker-compose up -d --no-deps iot-server

# 验证
curl http://localhost:8080/healthz
```

---

## 故障排查

### CI 失败常见问题

#### 1. 测试失败

```bash
# 查看失败的测试
# Actions → 失败的 workflow → test job → 展开失败步骤

# 本地重现
go test -v ./...

# 查看具体测试
go test -v -run TestFunctionName ./path/to/package
```

#### 2. 代码格式问题

```bash
# 查看需要格式化的文件
gofmt -l .

# 自动格式化
make fmt

# 重新提交
git add .
git commit --amend --no-edit
git push -f
```

#### 3. 构建失败

```bash
# 检查依赖问题
go mod tidy
go mod verify

# 清理缓存
go clean -cache -modcache

# 重新构建
make build
```

### 部署失败常见问题

#### 1. SSH 连接失败

**症状：** `Permission denied (publickey)`

**解决：**

```bash
# 1. 验证 Secret 中的 SSH 密钥格式正确
# 2. 确保服务器 authorized_keys 包含对应公钥
# 3. 检查服务器 SSH 配置

# 测试 SSH 连接
ssh -i ~/.ssh/github_actions deploy@staging-server
```

#### 2. Docker 镜像拉取失败

**症状：** `Error response from daemon: pull access denied`

**解决：**

```bash
# 1. 验证 DOCKER_USERNAME 和 DOCKER_PASSWORD 正确
# 2. 检查镜像是否已成功推送

# 在服务器上手动拉取测试
docker login
docker pull username/iot-server:staging-latest
```

#### 3. 健康检查超时

**症状：** `健康检查失败`

**解决：**

```bash
# SSH 到服务器
ssh deploy@server

# 查看容器日志
docker-compose logs iot-server

# 检查容器状态
docker-compose ps

# 手动测试健康接口
curl http://localhost:8080/healthz
```

#### 4. 数据库迁移失败

**症状：** `migration failed`

**解决：**

```bash
# 1. 检查迁移脚本
# 2. 手动运行迁移

docker-compose exec postgres psql -U iot iot_server

# 检查迁移记录
SELECT * FROM schema_migrations;

# 手动执行迁移
# 根据具体错误调整
```

---

## 最佳实践

### 1. 分支管理

```
main          ← 生产环境代码（受保护）
  ├── develop ← 开发环境代码
  └── feature/* ← 功能分支
```

**规则：**

- ✅ `main` 分支受保护，只能通过 PR 合并
- ✅ PR 必须通过 CI 检查才能合并
- ✅ 建议至少 1 人 review
- ✅ 定期从 `main` 同步到 `develop`

### 2. 提交规范

使用 [Conventional Commits](https://www.conventionalcommits.org/)：

```bash
feat: 添加新功能
fix: 修复 Bug
docs: 文档更新
style: 代码格式调整
refactor: 代码重构
test: 测试相关
chore: 构建/工具链相关
perf: 性能优化
```

**示例：**

```bash
git commit -m "feat(api): 添加设备批量查询接口"
git commit -m "fix(protocol): 修复 BKV 协议解析问题 #123"
git commit -m "docs: 更新 API 文档"
```

### 3. 版本管理

遵循 [语义化版本](https://semver.org/lang/zh-CN/)：

```
MAJOR.MINOR.PATCH  (例如: 1.2.3)

MAJOR: 不兼容的 API 修改
MINOR: 向下兼容的功能新增
PATCH: 向下兼容的问题修正
```

**示例：**

```bash
v1.0.0  # 首个正式版本
v1.1.0  # 新增功能
v1.1.1  # Bug 修复
v2.0.0  # 重大更新（不兼容）
```

### 4. CHANGELOG 维护

每次发版前更新 `CHANGELOG.md`：

```markdown
## [1.2.3] - 2024-03-15

### Added
- 新增设备批量查询接口
- 支持自定义充电策略

### Changed
- 优化数据库查询性能
- 更新第三方依赖版本

### Fixed
- 修复协议解析内存泄漏
- 修复并发场景下的竞态条件

### Security
- 修复 SQL 注入漏洞
```

### 5. 测试覆盖率

**目标：**

- ✅ 核心业务逻辑：> 80%
- ✅ 公共库和工具：> 90%
- ✅ 整体覆盖率：> 60%

**实践：**

```bash
# 查看覆盖率
go test -cover ./...

# 生成详细报告
make test-coverage
open coverage.html
```

### 6. 部署时机

**测试环境：**

- ✅ 每次 PR 合并后自动部署
- ✅ 随时可以部署最新代码

**生产环境：**

- ✅ 选择业务低峰期（如深夜）
- ✅ 避开周五和节假日前
- ✅ 确保有人值班监控
- ✅ 准备回滚预案

### 7. 监控和告警

**部署后必做：**

```bash
# 1. 观察关键指标 15-30 分钟
- CPU 使用率
- 内存使用率
- 错误率
- 响应时间
- 活跃连接数

# 2. 验证核心业务
- 设备连接测试
- 充电订单测试
- 第三方回调测试

# 3. 查看日志
docker-compose logs -f --tail=100 iot-server
```

---

## 附录

### <a name="docker-token"></a>生成 Docker Hub Token

1. 登录 [Docker Hub](https://hub.docker.com/)
2. Account Settings → Security
3. New Access Token
4. 描述: `github-actions-ci`
5. 权限: **Read & Write**
6. 复制 Token（只显示一次）

### <a name="ssh-key"></a>生成 SSH 密钥

```bash
# 生成密钥对
ssh-keygen -t ed25519 -C "github-actions-deploy" -f github_actions_key

# 查看公钥（添加到服务器）
cat github_actions_key.pub

# 查看私钥（添加到 GitHub Secrets）
cat github_actions_key

# ⚠️ 注意：完整复制私钥内容，包括：
# -----BEGIN OPENSSH PRIVATE KEY-----
# ...密钥内容...
# -----END OPENSSH PRIVATE KEY-----
```

### 相关资源

- [GitHub Actions 文档](https://docs.github.com/actions)
- [Docker Hub 文档](https://docs.docker.com/docker-hub/)
- [语义化版本规范](https://semver.org/lang/zh-CN/)
- [Conventional Commits](https://www.conventionalcommits.org/)

---

## 联系支持

如有问题，请：

1. 查看 [故障排查](#故障排查) 章节
2. 搜索 [GitHub Issues](https://github.com/your-org/iot-server/issues)
3. 联系运维团队

**保持持续改进！** 🚀
