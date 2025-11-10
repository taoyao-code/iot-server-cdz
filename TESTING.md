# 测试指南

本文档说明如何运行项目测试。

## 🚀 快速开始

### 方式1：GitHub Actions 自动化测试（推荐）

**无需本地配置，推送代码即可自动测试！**

```bash
# 1. 推送代码到GitHub
git push origin main

# 2. 访问 GitHub Actions 查看测试结果
# https://github.com/YOUR_ORG/iot-server/actions
```

**特点**：
- ✅ 零配置，自动运行
- ✅ PostgreSQL + Redis 环境自动准备
- ✅ 覆盖率报告自动生成
- ✅ PR自动检查

---

### 方式2：Docker Compose 本地测试

**适合本地快速测试**

```bash
# 1. 启动测试环境（自动初始化数据库）
docker-compose -f docker-compose.test.yml up -d

# 2. 运行测试
export TEST_DATABASE_URL="postgres://postgres:postgres@localhost:5433/iot_test?sslmode=disable"
go test ./internal/... -v

# 3. 清理环境
docker-compose -f docker-compose.test.yml down
```

**特点**：
- ✅ 一键启动测试环境
- ✅ 数据库自动初始化
- ✅ 本地快速调试

---

### 方式3：使用测试脚本（推荐本地使用）

```bash
# 一键运行所有测试 + 生成覆盖率报告
bash scripts/test-coverage.sh
```

**脚本自动**：
- 检测Docker环境
- 启动测试服务（如果需要）
- 运行所有测试
- 生成覆盖率报告
- 生成HTML报告
- 清理测试环境

---

## 📊 测试覆盖详情

### 新增测试文件

| 文件 | 测试数量 | 覆盖内容 |
|------|---------|---------|
| `internal/storage/pg/repo_test.go` | 12个 | 订单状态流转 |
| `internal/storage/pg/order_exception_test.go` | 11个 | 异常场景 |
| `internal/storage/pg/device_check_test.go` | 10个 | 设备检查 |
| **总计** | **33个** | **核心业务逻辑** |

### 测试内容

#### ✅ 订单状态流转（12个测试）

```
pending → charging → completed
pending → cancelling → cancelled  
charging → interrupted → charging/failed
charging → stopping → stopped
```

#### ✅ 异常场景（11个测试）

- P0-1: 设备离线拒绝创建订单
- P0-2: 充电中断恢复/超时
- P1-2: 延迟ACK拒绝
- P1-3: 端口并发冲突
- P1-5: cancelling拒绝启动
- P2-1: 监控任务CAS更新
- 超时处理（10秒、30秒、60秒）
- 竞态条件处理

#### ✅ 设备检查（10个测试）

- 设备在线判定（60秒阈值）
- 心跳超时检测
- 端口状态检查
- 中间态端口占用
- 并发访问

---

## 🎯 测试覆盖率目标

| 模块 | 原覆盖率 | 目标 | CI阈值 |
|------|---------|------|--------|
| **整体** | 28.9% | **≥50%** | 50% (必须) |
| **storage/pg** | 0% | **≥60%** | 60% (警告) |
| 订单逻辑 | 0% | ≥80% | - |
| 异常场景 | 0% | ≥70% | - |
| 设备检查 | 0% | ≥75% | - |

---

## 🧪 运行特定测试

### 运行单个测试文件

```bash
go test ./internal/storage/pg/repo_test.go -v
```

### 运行特定测试

```bash
go test ./internal/storage/pg/... -run TestOrderStatusTransition_PendingToCharging -v
```

### 运行特定模块

```bash
# Storage/PG模块
go test ./internal/storage/pg/... -v

# API模块
go test ./internal/api/... -v

# 协议模块
go test ./internal/protocol/... -v
```

### 并发测试

```bash
# 启用竞态检测
go test ./internal/... -race -v

# 并行运行
go test ./internal/... -parallel 4 -v
```

---

## 📈 查看覆盖率

### 终端查看

```bash
go test ./internal/... -coverprofile=coverage.out
go tool cover -func=coverage.out
```

### HTML报告

```bash
go test ./internal/... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

### 按模块查看

```bash
# Storage/PG模块覆盖率
go test ./internal/storage/pg/... -coverprofile=storage.out
go tool cover -func=storage.out
```

---

## 🐛 测试调试

### 详细输出

```bash
go test ./internal/... -v -test.v
```

### 失败时停止

```bash
go test ./internal/... -failfast
```

### 超时设置

```bash
go test ./internal/... -timeout 30s
```

### 输出到文件

```bash
go test ./internal/... -v 2>&1 | tee test_output.log
```

---

## ⚙️ 环境变量

### 测试数据库

```bash
export TEST_DATABASE_URL="postgres://postgres:postgres@localhost:5433/iot_test?sslmode=disable"
```

### Redis (可选)

```bash
export REDIS_URL="redis://localhost:6380/0"
```

### 跳过数据库测试

如果未设置 `TEST_DATABASE_URL`，需要数据库的测试会自动跳过。

---

## 🔍 CI/CD集成

### GitHub Actions

推送代码后自动运行：

```yaml
on:
  push:
    branches: [ main, develop ]
  pull_request:
    branches: [ main, develop ]
```

### 查看CI结果

1. 访问 GitHub Actions: https://github.com/YOUR_ORG/iot-server/actions
2. 点击最新的workflow run
3. 查看各Job的执行结果
4. 在Summary标签查看覆盖率报告

### CI通过条件

- ✅ 所有测试通过
- ✅ 总覆盖率 ≥ 50%
- ✅ 无竞态条件
- ✅ 代码可编译
- ✅ Lint检查通过

---

## 📝 编写新测试

### 测试文件命名

```
<filename>_test.go
```

### 测试函数命名

```go
func Test<Category>_<Scenario>(t *testing.T) {
    // ...
}
```

### 使用辅助函数

```go
func TestExample(t *testing.T) {
    repo := setupTestRepo(t)
    defer cleanupTestData(t, repo, "TEST_DEVICE_ID")
    
    // 测试逻辑
    deviceID := createTestDevice(t, repo, "TEST_DEVICE_ID")
    createTestOrder(t, repo, deviceID, 1, "ORDER_001")
    
    // 断言
    assert.Equal(t, expected, actual)
}
```

### 表驱动测试

```go
func TestMultipleScenarios(t *testing.T) {
    testCases := []struct {
        name     string
        input    int
        expected int
    }{
        {"case1", 1, 1},
        {"case2", 2, 4},
    }
    
    for _, tc := range testCases {
        t.Run(tc.name, func(t *testing.T) {
            result := function(tc.input)
            assert.Equal(t, tc.expected, result)
        })
    }
}
```

---

## 🚨 常见问题

### Q: 测试失败：连接数据库超时

**A**: 确保Docker服务启动完成

```bash
docker-compose -f docker-compose.test.yml ps
# 等待PostgreSQL状态变为healthy
```

### Q: 测试失败：表不存在

**A**: 数据库迁移未执行

```bash
# Docker会自动初始化，如果手动测试：
psql -h localhost -p 5433 -U postgres -d iot_test -f db/migrations/full_schema.sql
```

### Q: 部分测试被跳过

**A**: 未设置 `TEST_DATABASE_URL`，数据库测试自动跳过

```bash
export TEST_DATABASE_URL="postgres://postgres:postgres@localhost:5433/iot_test"
```

### Q: 覆盖率低于预期

**A**: 
1. 查看覆盖率报告找到未覆盖代码
2. 补充测试用例
3. 重新运行测试

### Q: CI失败但本地测试通过

**A**: 
1. 检查Go版本一致性
2. 清理本地缓存：`go clean -testcache`
3. 确保所有依赖已提交

---

## 📚 参考文档

- [测试覆盖率报告](internal/storage/pg/TEST_README.md)
- [GitHub Actions配置](.github/workflows/README.md)
- [技术规范](docs/IoT中间件技术规范.md)

---

## ✅ 最佳实践

### 1. 测试隔离

- 每个测试使用唯一的测试数据
- 测试结束后清理数据
- 不依赖测试执行顺序

### 2. 命名规范

- 测试名称清晰描述场景
- 使用 `Test<Module>_<Scenario>` 格式
- 测试文件与源文件对应

### 3. 断言清晰

```go
// Good
assert.Equal(t, 2, order.Status, "订单状态应为charging(2)")

// Bad
assert.Equal(t, 2, order.Status)
```

### 4. 测试覆盖

- 正常流程
- 边界条件
- 异常场景
- 并发情况

### 5. 持续维护

- 代码变更时更新测试
- 新功能必须有测试
- 保持覆盖率不降低

---

**最后更新**: 2025-11-10

**预期效果**:
- ✅ 测试覆盖率从 28.9% 提升至 >50%
- ✅ 核心模块覆盖率达到 >60%
- ✅ 关键业务逻辑全覆盖
- ✅ CI/CD自动化测试就绪

