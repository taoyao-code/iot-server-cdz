# 集成测试指南

## 📋 概述

集成测试验证系统各组件（PostgreSQL、Redis、应用代码）之间的交互，使用真实的数据库和缓存服务。

## 🚀 快速开始

### 运行所有集成测试

```bash
# 方式1：使用 Makefile（推荐）
make test-integration

# 方式2：直接使用 go test
go test -v ./tests/integration/...
```

### 快速迭代开发

```bash
# 1. 手动启动测试环境（只需一次）
make test-integration-setup

# 2. 运行测试（保留环境）
make test-integration-quick

# 3. 完成后清理
make test-integration-teardown
```

## 🏗️ 测试架构

### 目录结构

```
tests/
├── integration/          # 集成测试
│   ├── setup_test.go    # 测试环境初始化
│   ├── storage_test.go  # 存储层测试
│   └── session_test.go  # 会话管理测试
└── testutil/            # 测试工具库
    ├── docker.go        # Docker Compose 管理
    ├── helpers.go       # 辅助函数
    └── fixtures.go      # 测试数据构造器
```

### 测试环境

- **PostgreSQL**: `localhost:15433` (数据库: `iot_test`)
- **Redis**: `localhost:6381` (DB: 0)
- **容器名称**: `iot-postgres-test`, `iot-redis-test`

## 📝 编写集成测试

### 基本模板

```go
package integration

import (
	"testing"
	"github.com/stretchr/testify/require"
	"github.com/taoyao-code/iot-server/tests/testutil"
)

func TestYourFeature(t *testing.T) {
	// 1. 获取测试资源
	db := getTestDB(t)
	redis := getTestRedis(t)
	defer cleanupTest(t) // 测试后清理数据

	// 2. 准备测试数据
	device := testutil.CreateTestDevice(t, db, "TEST_DEVICE")

	// 3. 执行测试逻辑
	// ...

	// 4. 断言验证
	require.NoError(t, err)
}
```

### 使用测试工具

#### 创建测试数据

```go
// 创建设备
device := testutil.CreateTestDevice(t, db, "DEVICE_001")

// 创建端口
port := testutil.CreateTestPort(t, db, device.ID, 1, 0)

// 创建订单
order := testutil.CreateTestOrder(t, db, device.ID, 1, 1)
```

#### 数据清理

```go
// 清理所有测试数据
testutil.CleanDatabase(t, db)
testutil.CleanRedis(t, redis)

// 或使用 defer 自动清理
defer cleanupTest(t)
```

#### 等待异步操作

```go
testutil.WaitForCondition(t, func() bool {
	status := testutil.GetPortStatus(t, db, deviceID, portNo)
	return status == 1 // 充电中
}, 5*time.Second, "端口状态更新")
```

## 🔧 环境变量

### 跳过 Docker 启动（使用现有容器）

```bash
SKIP_DOCKER=true go test ./tests/integration/...
```

### 跳过测试清理（用于调试）

```bash
SKIP_CLEANUP=true go test ./tests/integration/...
```

### 自定义数据库连接

```bash
TEST_DB_DSN="postgres://user:pass@host:port/db" go test ./tests/integration/...
TEST_REDIS_ADDR="localhost:6379" go test ./tests/integration/...
```

## 🐛 故障排查

### 测试失败

1. **检查 Docker 是否运行**:
   ```bash
   docker ps
   ```

2. **查看容器日志**:
   ```bash
   docker logs iot-postgres-test
   docker logs iot-redis-test
   ```

3. **手动连接测试环境**:
   ```bash
   # PostgreSQL
   psql -h localhost -p 15433 -U postgres -d iot_test

   # Redis
   redis-cli -h localhost -p 6381
   ```

### 端口冲突

如果端口 5433 或 6380 被占用，修改 `docker-compose.test.yml`:

```yaml
services:
  postgres-test:
    ports:
      - "15433:5432"  # 使用其他端口
```

### 清理残留容器

```bash
# 强制清理
docker compose -f docker-compose.test.yml down -v --remove-orphans

# 清理所有测试相关容器
docker ps -a | grep iot-test | awk '{print $1}' | xargs docker rm -f
```

## 📊 测试覆盖范围

### 已实现测试

- ✅ **存储层测试** (`storage_test.go`)
  - 设备 CRUD 操作
  - 端口状态更新
  - 订单创建和结算
  - 并发更新（事务隔离）

- ✅ **会话管理测试** (`session_test.go`)
  - Redis 键值操作
  - TTL 和过期管理
  - Hash 数据结构
  - 队列操作

### 待添加测试

- ⏳ 订单全流程集成测试
- ⏳ 事件队列集成测试
- ⏳ 第三方推送集成测试

## 🔗 相关命令

```bash
# 运行所有测试（单元 + 集成）
make test-all

# 只运行单元测试
make test

# 只运行 P1 测试
make test-p1

# 生成覆盖率报告
make test-coverage
```

## 📚 最佳实践

1. **测试隔离**: 每个测试使用独立的测试数据，避免相互影响
2. **及时清理**: 使用 `defer cleanupTest(t)` 确保数据清理
3. **超时设置**: 集成测试使用较长超时（120s）
4. **并发测试**: 测试并发场景时使用 goroutine 和 channel
5. **断言清晰**: 使用有意义的错误消息

## 🤝 贡献指南

添加新集成测试时：

1. 在 `tests/integration/` 创建 `*_test.go` 文件
2. 遵循现有测试的命名和结构规范
3. 添加必要的测试数据构造器到 `testutil/fixtures.go`
4. 更新本 README 的测试覆盖范围

---

**有问题？** 查看项目根目录的 `Makefile` 和 `scripts/test-all.sh` 了解更多细节。
