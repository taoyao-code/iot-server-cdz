# Storage Module - 数据持久化层

> **导航**: [← 返回根目录](../../CLAUDE.md)
> **路径**: `internal/storage/`
> **最后更新**: 2025-11-28

---

## 📋 模块职责

Storage 模块是整个应用的数据持久化层，负责：

- **数据访问抽象**: 统一的 `CoreRepo` 接口
- **PostgreSQL 存储**: 设备、端口、命令日志、出站队列
- **Redis 存储**: 会话缓存、事件队列、临时数据
- **事务管理**: 跨存储操作的原子性保证
- **数据模型**: 统一的数据库模型定义

---

## 🏗️ 模块结构

```mermaid
graph TB
    A[Storage Module] --> B[CoreRepo Interface]
    A --> C[PostgreSQL Layer]
    A --> D[Redis Layer]
    A --> E[Models]

    B --> B1[设备操作]
    B --> B2[端口操作]
    B --> B3[命令日志]
    B --> B4[出站队列]
    B --> B5[事务管理]

    C --> C1[gormrepo<br/>GORM实现]
    C --> C2[pg<br/>连接池管理]

    D --> D1[Redis Client]
    D --> D2[队列操作]
    D --> D3[缓存操作]

    E --> E1[Device Model]
    E --> E2[Port Model]
    E --> E3[CmdLog Model]
    E --> E4[OutboundMessage Model]

    style B fill:#4ecdc4
    style C fill:#ff6b6b
    style D fill:#ffd93d
```

---

## 📂 文件结构

```
storage/
├── core_repo.go           # [核心] CoreRepo 接口定义
├── models/                # 数据模型
│   ├── device.go          # 设备模型
│   ├── port.go            # 端口模型
│   ├── cmd_log.go         # 命令日志模型
│   ├── outbound.go        # 出站消息模型
│   └── gateway_socket.go  # 网关插座映射
├── gormrepo/              # GORM 实现
│   ├── repository.go      # CoreRepo 实现
│   ├── device.go          # 设备操作
│   ├── port.go            # 端口操作
│   ├── cmd_log.go         # 命令日志操作
│   └── outbound.go        # 出站队列操作
├── pg/                    # PostgreSQL 连接
│   ├── pool.go            # 连接池管理
│   └── migrations.go      # 迁移辅助
└── redis/                 # Redis 操作
    ├── client.go          # Redis 客户端
    ├── queue.go           # 队列操作
    └── cache.go           # 缓存操作
```

---

## 🔑 核心接口：CoreRepo

### 接口定义 (core_repo.go)

```go
type CoreRepo interface {
    // 事务管理
    WithTx(ctx context.Context, fn func(repo CoreRepo) error) error

    // 设备操作
    EnsureDevice(ctx context.Context, phyID string) (*models.Device, error)
    TouchDeviceLastSeen(ctx context.Context, phyID string, at time.Time) error
    GetDeviceByPhyID(ctx context.Context, phyID string) (*models.Device, error)
    ListDevices(ctx context.Context, limit, offset int) ([]models.Device, error)

    // 网关插座映射
    UpsertGatewaySocket(ctx context.Context, socket *models.GatewaySocket) error
    GetGatewaySocketByUID(ctx context.Context, uid string) (*models.GatewaySocket, error)

    // 端口操作
    UpsertPortSnapshot(ctx context.Context, deviceID int64, portNo int32, status int32, powerW *int32, updatedAt time.Time) error
    GetPort(ctx context.Context, deviceID int64, portNo int32) (*models.Port, error)

    // 命令日志
    AppendCmdLog(ctx context.Context, log *models.CmdLog) error
    ListRecentCmdLogs(ctx context.Context, deviceID int64, limit int) ([]models.CmdLog, error)

    // 出站队列
    EnqueueOutbound(ctx context.Context, msg *models.OutboundMessage) (int64, error)
    DequeuePendingForDevice(ctx context.Context, deviceID int64, limit int) ([]models.OutboundMessage, error)
    MarkOutboundSent(ctx context.Context, id int64) error
    MarkOutboundDone(ctx context.Context, id int64) error
    MarkOutboundFailed(ctx context.Context, id int64, lastError string) error
}
```

### 设计约束

1. **上层禁止直接写 SQL** - 统一通过 CoreRepo 接口访问
2. **DB-agnostic** - 接口面向模型与基础类型，不暴露具体数据库实现
3. **事务支持** - 提供 `WithTx` 保证核心路径的原子性
4. **嵌套事务** - 实现需正确处理嵌套调用，复用当前事务

---

## 📊 数据模型 (models/)

### Device 模型

```go
type Device struct {
    ID         int64     `gorm:"primaryKey;autoIncrement"`
    PhyID      string    `gorm:"uniqueIndex;size:64;not null"`  // 物理设备ID
    Protocol   string    `gorm:"size:20"`                       // ap3000/bkv/gn
    Status     string    `gorm:"size:20"`                       // online/offline
    LastSeenAt time.Time `gorm:"index"`                         // 最后心跳时间
    CreatedAt  time.Time
    UpdatedAt  time.Time
}
```

**索引**:
- `UNIQUE INDEX` on `phy_id`
- `INDEX` on `last_seen_at`

### Port 模型

```go
type Port struct {
    ID        int64     `gorm:"primaryKey;autoIncrement"`
    DeviceID  int64     `gorm:"index:idx_device_port;not null"`
    PortNo    int32     `gorm:"index:idx_device_port;not null"`
    Status    int32     `gorm:"not null"`          // API状态码
    PowerW    *int32                               // 功率(瓦)
    UpdatedAt time.Time `gorm:"index"`             // 状态更新时间
    CreatedAt time.Time
}
```

**复合索引**:
- `UNIQUE INDEX idx_device_port` on `(device_id, port_no)`

### CmdLog 模型

```go
type CmdLog struct {
    ID         int64     `gorm:"primaryKey;autoIncrement"`
    DeviceID   int64     `gorm:"index;not null"`
    Direction  string    `gorm:"size:10;not null"`  // uplink/downlink
    CommandHex string    `gorm:"type:text"`          // 命令16进制
    CreatedAt  time.Time `gorm:"index"`
}
```

### OutboundMessage 模型

```go
type OutboundMessage struct {
    ID          int64     `gorm:"primaryKey;autoIncrement"`
    DeviceID    int64     `gorm:"index:idx_device_status;not null"`
    CommandType string    `gorm:"size:50;not null"`
    Payload     string    `gorm:"type:jsonb"`        // JSON payload
    Status      int32     `gorm:"index:idx_device_status;not null;default:0"` // 0=pending,1=sent,2=done,3=failed
    Priority    int32     `gorm:"not null;default:0"`
    NotBefore   time.Time `gorm:"index"`             // 不早于此时间发送
    RetryCount  int32     `gorm:"default:0"`
    LastError   string    `gorm:"type:text"`
    CreatedAt   time.Time `gorm:"index"`
    SentAt      *time.Time
    DoneAt      *time.Time
}
```

**复合索引**:
- `INDEX idx_device_status` on `(device_id, status)`

---

## 🔧 GORM 实现 (gormrepo/)

### Repository 实现

```go
type GormRepo struct {
    db     *gorm.DB
    logger *zap.Logger
}

func NewGormRepo(db *gorm.DB, logger *zap.Logger) storage.CoreRepo {
    return &GormRepo{db: db, logger: logger}
}
```

### 事务管理

```go
func (r *GormRepo) WithTx(ctx context.Context, fn func(repo storage.CoreRepo) error) error {
    // 检查是否已在事务中
    if tx := r.db.Statement.ConnPool; tx != nil {
        // 复用当前事务
        return fn(r)
    }

    // 开启新事务
    return r.db.Transaction(func(tx *gorm.DB) error {
        txRepo := &GormRepo{db: tx, logger: r.logger}
        return fn(txRepo)
    })
}
```

### 设备操作示例

```go
func (r *GormRepo) EnsureDevice(ctx context.Context, phyID string) (*models.Device, error) {
    var device models.Device

    // 先查询
    err := r.db.WithContext(ctx).
        Where("phy_id = ?", phyID).
        First(&device).Error

    if err == nil {
        return &device, nil
    }

    if !errors.Is(err, gorm.ErrRecordNotFound) {
        return nil, err
    }

    // 不存在则创建
    device = models.Device{
        PhyID:      phyID,
        Status:     "unknown",
        LastSeenAt: time.Now(),
    }

    if err := r.db.WithContext(ctx).Create(&device).Error; err != nil {
        return nil, err
    }

    return &device, nil
}
```

### 端口操作（Upsert）

```go
func (r *GormRepo) UpsertPortSnapshot(
    ctx context.Context,
    deviceID int64,
    portNo int32,
    status int32,
    powerW *int32,
    updatedAt time.Time,
) error {
    port := models.Port{
        DeviceID:  deviceID,
        PortNo:    portNo,
        Status:    status,
        PowerW:    powerW,
        UpdatedAt: updatedAt,
    }

    // PostgreSQL: ON CONFLICT DO UPDATE
    return r.db.WithContext(ctx).
        Clauses(clause.OnConflict{
            Columns:   []clause.Column{{Name: "device_id"}, {Name: "port_no"}},
            DoUpdates: clause.AssignmentColumns([]string{"status", "power_w", "updated_at"}),
        }).
        Create(&port).Error
}
```

---

## 💾 PostgreSQL 连接池 (pg/)

### 连接池配置

```go
type PoolConfig struct {
    DSN             string
    MaxOpenConns    int
    MaxIdleConns    int
    ConnMaxLifetime time.Duration
    ConnMaxIdleTime time.Duration
}

func NewPool(cfg PoolConfig) (*gorm.DB, error) {
    db, err := gorm.Open(postgres.Open(cfg.DSN), &gorm.Config{
        Logger: logger.Default.LogMode(logger.Silent),
    })
    if err != nil {
        return nil, err
    }

    sqlDB, err := db.DB()
    if err != nil {
        return nil, err
    }

    // 配置连接池
    sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
    sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
    sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)
    sqlDB.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)

    return db, nil
}
```

### 推荐配置

```yaml
database:
  dsn: "postgres://iot:password@localhost:5432/iot_server?sslmode=disable"
  max_open_conns: 50        # 最大开放连接
  max_idle_conns: 10        # 最大空闲连接
  conn_max_lifetime: 1h     # 连接最大生命周期
  conn_max_idle_time: 10m   # 连接最大空闲时间
```

---

## 🔴 Redis 操作 (redis/)

### Redis 客户端

```go
type RedisClient struct {
    client *redis.Client
    logger *zap.Logger
}

func NewRedisClient(addr string, password string, db int) *RedisClient {
    client := redis.NewClient(&redis.Options{
        Addr:     addr,
        Password: password,
        DB:       db,
    })

    return &RedisClient{client: client}
}
```

### 事件队列操作

```go
// 推送事件到队列
func (r *RedisClient) PushEvent(ctx context.Context, event []byte) error {
    return r.client.LPush(ctx, "event_queue", event).Err()
}

// 阻塞弹出事件（BRPOP）
func (r *RedisClient) PopEvent(ctx context.Context, timeout time.Duration) ([]byte, error) {
    result, err := r.client.BRPop(ctx, timeout, "event_queue").Result()
    if err != nil {
        return nil, err
    }
    if len(result) < 2 {
        return nil, errors.New("invalid brpop result")
    }
    return []byte(result[1]), nil
}
```

### 缓存操作

```go
// 设置缓存
func (r *RedisClient) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
    return r.client.Set(ctx, key, value, expiration).Err()
}

// 获取缓存
func (r *RedisClient) Get(ctx context.Context, key string) (string, error) {
    return r.client.Get(ctx, key).Result()
}

// 删除缓存
func (r *RedisClient) Del(ctx context.Context, keys ...string) error {
    return r.client.Del(ctx, keys...).Err()
}
```

---

## 🔄 使用示例

### 示例1: 设备心跳处理

```go
func HandleHeartbeat(ctx context.Context, repo storage.CoreRepo, phyID string) error {
    // 确保设备存在
    device, err := repo.EnsureDevice(ctx, phyID)
    if err != nil {
        return fmt.Errorf("ensure device failed: %w", err)
    }

    // 更新最后心跳时间
    if err := repo.TouchDeviceLastSeen(ctx, phyID, time.Now()); err != nil {
        return fmt.Errorf("touch last seen failed: %w", err)
    }

    return nil
}
```

### 示例2: 端口状态更新（带事务）

```go
func UpdatePortStatus(ctx context.Context, repo storage.CoreRepo, phyID string, portNo int32, status int32, powerW *int32) error {
    return repo.WithTx(ctx, func(txRepo storage.CoreRepo) error {
        // 1. 获取设备
        device, err := txRepo.GetDeviceByPhyID(ctx, phyID)
        if err != nil {
            return err
        }

        // 2. 更新端口快照
        if err := txRepo.UpsertPortSnapshot(ctx, device.ID, portNo, status, powerW, time.Now()); err != nil {
            return err
        }

        // 3. 记录命令日志
        log := &models.CmdLog{
            DeviceID:   device.ID,
            Direction:  "uplink",
            CommandHex: "...",
            CreatedAt:  time.Now(),
        }
        return txRepo.AppendCmdLog(ctx, log)
    })
}
```

### 示例3: 命令下发

```go
func EnqueueCommand(ctx context.Context, repo storage.CoreRepo, phyID string, cmdType string, payload interface{}) error {
    // 1. 获取设备
    device, err := repo.GetDeviceByPhyID(ctx, phyID)
    if err != nil {
        return err
    }

    // 2. 序列化 payload
    payloadJSON, err := json.Marshal(payload)
    if err != nil {
        return err
    }

    // 3. 入队
    msg := &models.OutboundMessage{
        DeviceID:    device.ID,
        CommandType: cmdType,
        Payload:     string(payloadJSON),
        Status:      0, // pending
        Priority:    0,
        NotBefore:   time.Now(),
        CreatedAt:   time.Now(),
    }

    _, err = repo.EnqueueOutbound(ctx, msg)
    return err
}
```

---

## 🧪 测试

### 单元测试

```bash
# 测试 GORM 实现
go test ./internal/storage/gormrepo -v

# 测试 Redis 操作
go test ./internal/storage/redis -v

# 测试覆盖率
go test ./internal/storage/... -coverprofile=coverage.out
```

### 集成测试（需要真实数据库）

```go
func TestGormRepoIntegration(t *testing.T) {
    // 连接测试数据库
    db, err := gorm.Open(postgres.Open("postgres://test:test@localhost:5432/test_db"))
    require.NoError(t, err)

    // 迁移表结构
    db.AutoMigrate(&models.Device{}, &models.Port{})

    // 创建 repo
    repo := gormrepo.NewGormRepo(db, zap.NewNop())

    // 测试设备操作
    device, err := repo.EnsureDevice(context.Background(), "test_device_001")
    assert.NoError(t, err)
    assert.NotZero(t, device.ID)
}
```

---

## 📈 性能优化

### 索引优化

**已添加索引**:
- `devices.phy_id` - UNIQUE INDEX（设备查询）
- `devices.last_seen_at` - INDEX（在线状态查询）
- `ports.(device_id, port_no)` - UNIQUE INDEX（端口状态查询）
- `ports.updated_at` - INDEX（时序查询）
- `outbound_messages.(device_id, status)` - INDEX（队列查询）

### 查询优化

**批量操作**:
```go
// ❌ 避免：N+1 查询
for _, phyID := range phyIDs {
    device, _ := repo.GetDeviceByPhyID(ctx, phyID)
}

// ✅ 推荐：批量查询
devices, _ := repo.GetDevicesByPhyIDs(ctx, phyIDs)
```

**预加载关联**:
```go
// ✅ 使用 GORM Preload
db.Preload("Ports").Find(&devices)
```

### 连接池调优

**监控指标**:
```go
sqlDB, _ := db.DB()
stats := sqlDB.Stats()

// 关键指标
fmt.Printf("MaxOpenConnections: %d\n", stats.MaxOpenConnections)
fmt.Printf("OpenConnections: %d\n", stats.OpenConnections)
fmt.Printf("InUse: %d\n", stats.InUse)
fmt.Printf("Idle: %d\n", stats.Idle)
fmt.Printf("WaitCount: %d\n", stats.WaitCount)
fmt.Printf("WaitDuration: %v\n", stats.WaitDuration)
```

---

## 🚨 常见问题

### Q1: 事务嵌套问题

**问题**: 嵌套调用 `WithTx` 时事务未复用

**解决**:
```go
// 检查是否已在事务中
func (r *GormRepo) WithTx(ctx context.Context, fn func(repo storage.CoreRepo) error) error {
    if r.db.Statement.ConnPool != nil {
        // 已在事务中，直接调用
        return fn(r)
    }
    // 开启新事务
    return r.db.Transaction(func(tx *gorm.DB) error {
        txRepo := &GormRepo{db: tx, logger: r.logger}
        return fn(txRepo)
    })
}
```

### Q2: 连接池耗尽

**症状**: `too many connections` 错误

**原因**:
- 连接泄漏（未关闭 rows）
- 连接池配置过小

**解决**:
```go
// 1. 正确关闭 rows
rows, err := db.Query("...")
defer rows.Close()

// 2. 增大连接池
sqlDB.SetMaxOpenConns(100)
sqlDB.SetMaxIdleConns(20)
```

### Q3: PostgreSQL 死锁

**症状**: `deadlock detected` 错误

**原因**: 事务内操作顺序不一致

**解决**:
```go
// ✅ 统一操作顺序
repo.WithTx(ctx, func(tx storage.CoreRepo) error {
    // 总是先操作设备，再操作端口
    tx.EnsureDevice(ctx, phyID)
    tx.UpsertPortSnapshot(ctx, deviceID, portNo, ...)
    return nil
})
```

---

## 🔗 相关文档

- [App Module](../app/CLAUDE.md) - 应用引导（使用 CoreRepo）
- [Core Model](../coremodel/CLAUDE.md) - 核心数据模型
- [数据库迁移](../../db/migrations/) - 迁移脚本
- [GORM 文档](https://gorm.io/) - GORM 官方文档
- [Redis 文档](https://redis.io/) - Redis 官方文档

---

**最后更新**: 2025-11-28
**维护者**: Storage Team
