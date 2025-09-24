# 阶段1详细实施计划 - 第三方API完善

**时间范围**: 4-5周  
**优先级**: P0 (必须完成)  
**目标**: 为第三方平台提供完整的设备控制和查询API

## 🎯 阶段目标

完善第三方API体系，使第三方平台能够：
- 通过API完整控制充电桩设备（启停充、参数设置、远程重启）
- 查询设备实时状态和历史数据
- 获取统计分析数据支撑业务决策
- 实现高效的批量操作

## 📋 详细任务清单

### Week 1-2: 设备控制API

#### 🔌 充电控制API
**目标**: 实现完整的充电控制接口

**核心API设计**:
```go
// 启动充电
POST /api/third/devices/{phyId}/charge/start
{
    "port_no": 1,
    "mode": 1,           // 1:按时间 2:按电量 3:按功率
    "time_limit": 120,   // 分钟（按时间模式）
    "energy_limit": 50,  // kWh（按电量模式）  
    "power_limit": 7000, // W（按功率模式）
    "order_no": "ORD20240113001", // 第三方订单号（必填）
    "user_id": "USER001"          // 第三方用户ID（可选）
}

// 停止充电
POST /api/third/devices/{phyId}/charge/stop
{
    "port_no": 1,
    "reason": "user_stop", // 停止原因: user_stop/timeout/error/emergency
    "order_no": "ORD20240113001"
}

// 查询充电状态
GET /api/third/devices/{phyId}/charge/status?port_no=1
```

**详细任务**:
- [ ] **数据模型扩展**
  ```sql
  -- 扩展现有订单表，支持第三方订单号
  ALTER TABLE orders ADD COLUMN third_party_order_no VARCHAR(100);
  ALTER TABLE orders ADD COLUMN third_party_user_id VARCHAR(100);
  ALTER TABLE orders ADD COLUMN api_request_id VARCHAR(100);
  
  -- 创建API请求日志表
  CREATE TABLE api_requests (
      id BIGSERIAL PRIMARY KEY,
      request_id VARCHAR(100) UNIQUE NOT NULL,
      endpoint VARCHAR(200) NOT NULL,
      method VARCHAR(10) NOT NULL,
      device_phy_id VARCHAR(50),
      request_body JSONB,
      response_body JSONB,
      status_code INT,
      duration_ms INT,
      client_ip VARCHAR(45),
      user_agent VARCHAR(500),
      created_at TIMESTAMP DEFAULT NOW()
  );
  ```

- [ ] **充电控制服务实现**
  ```go
  // internal/api/third/charging.go
  type ChargingController struct {
      repo     *Repository
      outbound *OutboundService
      validator *Validator
  }
  
  func (c *ChargingController) StartCharging(ctx *gin.Context) {
      var req StartChargingRequest
      if err := ctx.ShouldBindJSON(&req); err != nil {
          c.respondError(ctx, ErrInvalidRequest, err.Error())
          return
      }
      
      // 1. 验证设备和端口状态
      if err := c.validateChargingRequest(&req); err != nil {
          c.respondError(ctx, ErrValidationFailed, err.Error())
          return
      }
      
      // 2. 创建订单记录
      order := c.createChargingOrder(&req)
      if err := c.repo.CreateOrder(ctx, order); err != nil {
          c.respondError(ctx, ErrDatabaseError, "创建订单失败")
          return
      }
      
      // 3. 发送设备控制指令
      if err := c.sendChargingCommand(order); err != nil {
          c.respondError(ctx, ErrDeviceControlFailed, "设备控制失败")
          return
      }
      
      c.respondSuccess(ctx, gin.H{
          "order_id": order.ID,
          "status": "commanding",
          "estimated_start_time": time.Now().Add(3 * time.Second),
      })
  }
  ```

- [ ] **设备指令集成**
  ```go
  // 集成现有的协议处理器
  func (c *ChargingController) sendChargingCommand(order *Order) error {
      switch order.DeviceProtocol {
      case "bkv":
          return c.sendBKVChargingCommand(order)
      case "gn":
          return c.sendGNChargingCommand(order)
      case "ap3000":
          return c.sendAP3000ChargingCommand(order)
      default:
          return fmt.Errorf("unsupported protocol: %s", order.DeviceProtocol)
      }
  }
  
  func (c *ChargingController) sendBKVChargingCommand(order *Order) error {
      // 构造BKV控制指令
      payload := bkv.BuildControlPayload(order.PortNo, order.Mode, order.TimeLimit, order.EnergyLimit)
      
      // 加入出站队列
      return c.outbound.EnqueueCommand(order.DeviceID, bkv.CmdControl, payload, order.ID)
  }
  ```

#### ⚙️ 设备管理API
**目标**: 提供设备参数配置和管理功能

**核心API设计**:
```go
// 设备参数设置
POST /api/third/devices/{phyId}/params
{
    "params": {
        "max_current": 32,        // 最大电流(A)
        "voltage_threshold": 220, // 电压阈值(V)  
        "temp_threshold": 60,     // 温度阈值(°C)
        "power_threshold": 7000,  // 功率阈值(W)
        "auto_stop_time": 480     // 自动停止时间(分钟)
    },
    "apply_immediately": true    // 是否立即应用
}

// 设备参数查询
GET /api/third/devices/{phyId}/params

// 远程重启
POST /api/third/devices/{phyId}/reboot
{
    "reason": "maintenance",     // 重启原因
    "delay_seconds": 30         // 延迟时间
}

// 固件升级
POST /api/third/devices/{phyId}/upgrade  
{
    "firmware_url": "https://example.com/firmware.bin",
    "version": "v2.1.0",
    "checksum": "sha256:abc123...",
    "force_upgrade": false
}
```

**详细任务**:
- [ ] **参数管理服务**
  ```go
  type DeviceParamsController struct {
      repo      *Repository
      validator *ParamsValidator
      outbound  *OutboundService
  }
  
  // 参数配置
  func (c *DeviceParamsController) SetParams(ctx *gin.Context) {
      // 1. 验证参数合法性
      // 2. 保存参数配置记录
      // 3. 发送参数设置指令
      // 4. 记录操作日志
  }
  
  // 参数验证器
  type ParamsValidator struct {
      rules map[string]ParamRule
  }
  
  type ParamRule struct {
      Min     interface{} `json:"min"`
      Max     interface{} `json:"max"`
      Type    string      `json:"type"`
      Required bool       `json:"required"`
  }
  ```

- [ ] **批量操作API**
  ```go
  // 批量设备控制
  POST /api/third/devices/batch/control
  {
      "devices": [
          {"phy_id": "DEV001", "port_no": 1, "action": "start", "params": {...}},
          {"phy_id": "DEV002", "port_no": 2, "action": "stop", "params": {...}}
      ],
      "max_concurrent": 10,        // 最大并发数
      "timeout_seconds": 30        // 超时时间
  }
  
  // 批量状态查询
  POST /api/third/devices/batch/status
  {
      "device_phy_ids": ["DEV001", "DEV002", "DEV003"],
      "include_ports": true,
      "include_realtime": true
  }
  ```

**Week 1-2 交付物**:
- 完整的充电控制API（启动/停止/状态查询）
- 设备参数配置API
- 批量操作API
- API请求日志和审计功能
- 单元测试覆盖率 > 80%

### Week 3-4: 设备查询API

#### 📊 设备状态API
**目标**: 提供全面的设备信息查询能力

**核心API设计**:
```go
// 设备列表查询
GET /api/third/devices?online=true&type=AC&region=beijing&page=1&size=20&sort=last_seen&order=desc

// 设备详情查询
GET /api/third/devices/{phyId}
{
    "code": 0,
    "data": {
        "phy_id": "DEV001",
        "device_type": "AC",
        "protocol": "bkv",
        "status": "online",
        "last_seen_at": "2024-01-13T10:00:00Z",
        "location": {"lat": 39.9042, "lng": 116.4074},
        "firmware_version": "v1.2.3",
        "hardware_version": "v2.0",
        "total_ports": 4,
        "available_ports": 2,
        "current_power": 14500, // W
        "daily_sessions": 15,
        "daily_energy": 250.5   // kWh
    }
}

// 端口状态查询
GET /api/third/devices/{phyId}/ports
{
    "code": 0,  
    "data": {
        "device_phy_id": "DEV001",
        "ports": [
            {
                "port_no": 1,
                "status": "charging",    // idle/charging/fault/offline
                "current_power": 7200,  // W
                "voltage": 220.5,       // V
                "current": 32.7,        // A
                "temperature": 45,      // °C
                "session_id": "SES001",
                "start_time": "2024-01-13T09:30:00Z",
                "duration": 1800,       // 秒
                "energy_consumed": 3.6  // kWh
            }
        ]
    }
}

// 实时数据查询
GET /api/third/devices/{phyId}/realtime
```

**详细任务**:
- [ ] **设备查询服务**
  ```go
  type DeviceQueryController struct {
      repo  *Repository
      cache *redis.Client
  }
  
  // 设备列表查询（支持复杂过滤）
  func (c *DeviceQueryController) ListDevices(ctx *gin.Context) {
      filter := c.parseDeviceFilter(ctx)
      devices, total, err := c.repo.ListDevicesWithFilter(ctx, filter)
      if err != nil {
          c.respondError(ctx, ErrDatabaseError, err.Error())
          return
      }
      
      c.respondPaginated(ctx, devices, total, filter.Page, filter.Size)
  }
  
  type DeviceFilter struct {
      Online     *bool   `form:"online"`
      DeviceType string  `form:"type"`
      Region     string  `form:"region"`
      Status     string  `form:"status"`
      Page       int     `form:"page"`
      Size       int     `form:"size"`
      Sort       string  `form:"sort"`
      Order      string  `form:"order"`
  }
  ```

- [ ] **实时数据缓存**
  ```go
  // Redis缓存实时设备数据
  type DeviceRealtimeCache struct {
      redis  *redis.Client
      ttl    time.Duration
  }
  
  func (c *DeviceRealtimeCache) GetRealtimeData(phyID string) (*RealtimeData, error) {
      key := fmt.Sprintf("realtime:device:%s", phyID)
      data, err := c.redis.Get(key).Result()
      if err != nil {
          return nil, err
      }
      
      var realtime RealtimeData
      err = json.Unmarshal([]byte(data), &realtime)
      return &realtime, err
  }
  
  func (c *DeviceRealtimeCache) UpdateRealtimeData(phyID string, data *RealtimeData) error {
      key := fmt.Sprintf("realtime:device:%s", phyID)
      jsonData, _ := json.Marshal(data)
      return c.redis.Set(key, jsonData, c.ttl).Err()
  }
  ```

#### 📈 统计分析API
**目标**: 提供设备和业务的统计分析数据

**核心API设计**:
```go
// 设备统计
GET /api/third/stats/devices?time_range=24h&group_by=type,region
{
    "code": 0,
    "data": {
        "summary": {
            "total_devices": 1000,
            "online_devices": 950,
            "offline_devices": 50,
            "utilization_rate": 0.75
        },
        "by_type": {
            "AC": {"count": 600, "online": 570, "utilization": 0.72},
            "DC": {"count": 400, "online": 380, "utilization": 0.78}
        },
        "by_region": {
            "beijing": {"count": 300, "online": 285},
            "shanghai": {"count": 250, "online": 240}
        }
    }
}

// 使用统计
GET /api/third/stats/usage?start_time=2024-01-01&end_time=2024-01-13&granularity=day
{
    "code": 0,
    "data": {
        "total_sessions": 5000,
        "total_energy": 120000,    // kWh
        "total_duration": 450000,  // 分钟
        "avg_session_duration": 90, // 分钟
        "peak_concurrent": 150,
        "daily_breakdown": [
            {
                "date": "2024-01-01",
                "sessions": 380,
                "energy": 9200,
                "avg_duration": 85
            }
        ]
    }
}

// 性能统计
GET /api/third/stats/performance?time_range=7d
{
    "code": 0,
    "data": {
        "api_performance": {
            "avg_response_time": 45,  // ms
            "p95_response_time": 120, // ms
            "success_rate": 0.999
        },
        "device_performance": {
            "command_success_rate": 0.985,
            "avg_command_latency": 2.5,  // 秒
            "heartbeat_success_rate": 0.998
        }
    }
}
```

**详细任务**:
- [ ] **统计数据聚合**
  ```go
  type StatsService struct {
      repo    *Repository
      cache   *redis.Client
      metrics *prometheus.Registry
  }
  
  // 设备统计聚合
  func (s *StatsService) AggregateDeviceStats(timeRange string) (*DeviceStats, error) {
      // 1. 从数据库聚合基础数据
      // 2. 计算派生指标
      // 3. 缓存结果
      // 4. 返回统计数据
  }
  
  // 定时聚合任务
  func (s *StatsService) StartPeriodicAggregation() {
      // 每分钟聚合实时数据
      go s.scheduleTask(time.Minute, s.aggregateRealtimeStats)
      
      // 每小时聚合历史数据
      go s.scheduleTask(time.Hour, s.aggregateHourlyStats)
      
      // 每天聚合日统计
      go s.scheduleTask(24*time.Hour, s.aggregateDailyStats)
  }
  ```

- [ ] **历史数据API**
  ```go
  // 历史数据查询
  GET /api/third/devices/{phyId}/history?type=charging&start_time=xxx&end_time=xxx&page=1&size=50
  
  // 命令日志查询
  GET /api/third/devices/{phyId}/logs?cmd_type=control&status=success&page=1&size=50
  
  // 告警历史查询
  GET /api/third/devices/{phyId}/alarms?level=warning&status=resolved&page=1&size=50
  ```

**Week 3-4 交付物**:
- 完整的设备状态查询API
- 实时数据缓存机制
- 统计分析API和数据聚合服务
- 历史数据查询API
- 性能优化和缓存策略

### Week 5: 集成测试与优化

#### 🧪 API集成测试
**目标**: 确保API功能完整性和稳定性

**详细任务**:
- [ ] **端到端测试**
  ```go
  // 完整充电流程测试
  func TestChargingWorkflow(t *testing.T) {
      // 1. 查询设备状态 -> 确认在线且端口空闲
      // 2. 启动充电 -> 验证指令下发成功
      // 3. 查询充电状态 -> 确认充电进行中
      // 4. 停止充电 -> 验证停止成功
      // 5. 查询历史记录 -> 确认记录完整
  }
  
  // 批量操作测试
  func TestBatchOperations(t *testing.T) {
      // 测试批量设备控制
      // 测试批量状态查询
      // 测试并发限制
      // 测试错误处理
  }
  ```

- [ ] **压力测试**
  ```bash
  # API压力测试脚本
  #!/bin/bash
  
  # 设备状态查询压力测试
  hey -n 10000 -c 100 -H "Authorization: Bearer xxx" \
      http://localhost:8080/api/third/devices
  
  # 充电控制压力测试  
  hey -n 1000 -c 50 -m POST -H "Content-Type: application/json" \
      -d '{"port_no":1,"mode":1,"time_limit":60,"order_no":"TEST001"}' \
      http://localhost:8080/api/third/devices/TEST_DEVICE/charge/start
  ```

#### 🔧 性能优化
**目标**: 优化API响应时间和系统性能

**详细任务**:
- [ ] **数据库优化**
  ```sql
  -- 添加必要索引
  CREATE INDEX idx_devices_status_type ON devices(status, device_type);
  CREATE INDEX idx_orders_third_party ON orders(third_party_order_no);
  CREATE INDEX idx_api_requests_endpoint_time ON api_requests(endpoint, created_at);
  CREATE INDEX idx_device_ports_device_status ON device_ports(device_id, status);
  
  -- 分区表优化（大数据量场景）
  CREATE TABLE api_requests_y2024m01 PARTITION OF api_requests 
  FOR VALUES FROM ('2024-01-01') TO ('2024-02-01');
  ```

- [ ] **缓存策略**
  ```go
  // Redis缓存配置
  type CacheConfig struct {
      DeviceStatus    time.Duration // 设备状态缓存 30秒
      DeviceList      time.Duration // 设备列表缓存 2分钟  
      Statistics      time.Duration // 统计数据缓存 5分钟
      RealtimeData    time.Duration // 实时数据缓存 10秒
  }
  
  // 缓存键设计
  const (
      CacheKeyDeviceStatus = "device:status:%s"           // 设备状态
      CacheKeyDeviceList   = "devices:list:%s"            // 设备列表（按过滤条件）
      CacheKeyStats        = "stats:%s:%s"                // 统计数据（类型:时间范围）
      CacheKeyRealtime     = "realtime:device:%s"         // 实时数据
  )
  ```

- [ ] **API响应优化**
  ```go
  // 分页查询优化
  type PaginatedResponse struct {
      Code       int         `json:"code"`
      Data       interface{} `json:"data"`
      Pagination Pagination  `json:"pagination"`
      Meta       ResponseMeta `json:"meta,omitempty"`
  }
  
  type Pagination struct {
      Page     int   `json:"page"`
      Size     int   `json:"size"`
      Total    int64 `json:"total"`
      HasNext  bool  `json:"has_next"`
      HasPrev  bool  `json:"has_prev"`
  }
  
  type ResponseMeta struct {
      ResponseTime string `json:"response_time"`
      RequestID    string `json:"request_id"`
      Version      string `json:"version"`
  }
  ```

**Week 5 交付物**:
- 完整的API测试套件
- 性能测试报告
- API文档更新
- 系统优化和缓存策略
- 生产环境部署就绪

## 🛠️ 技术实施要点

### API认证和安全
```go
// API Key认证中间件
func APIKeyAuthMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        apiKey := c.GetHeader("X-API-Key")
        if apiKey == "" {
            c.JSON(401, gin.H{"error": "missing api key"})
            c.Abort()
            return
        }
        
        // 验证API Key合法性
        if !isValidAPIKey(apiKey) {
            c.JSON(401, gin.H{"error": "invalid api key"})
            c.Abort()
            return
        }
        
        // 设置客户端信息
        c.Set("client_id", getClientID(apiKey))
        c.Next()
    }
}

// 请求限流中间件
func RateLimitMiddleware(limit int, window time.Duration) gin.HandlerFunc {
    limiter := rate.NewLimiter(rate.Every(window/time.Duration(limit)), limit)
    
    return func(c *gin.Context) {
        if !limiter.Allow() {
            c.JSON(429, gin.H{"error": "rate limit exceeded"})
            c.Abort()
            return
        }
        c.Next()
    }
}
```

### 错误处理规范
```go
// 统一错误码定义
const (
    ErrCodeSuccess          = 0
    ErrCodeInvalidRequest   = 1001
    ErrCodeUnauthorized     = 1002
    ErrCodeDeviceNotFound   = 2001
    ErrCodeDeviceOffline    = 2002
    ErrCodePortBusy         = 2003
    ErrCodeCommandFailed    = 2004
)

// 统一响应格式
type APIResponse struct {
    Code    int         `json:"code"`
    Message string      `json:"message"`
    Data    interface{} `json:"data,omitempty"`
    TraceID string      `json:"trace_id,omitempty"`
}

func RespondError(c *gin.Context, code int, message string) {
    c.JSON(200, APIResponse{
        Code:    code,
        Message: message,
        TraceID: getTraceID(c),
    })
}

func RespondSuccess(c *gin.Context, data interface{}) {
    c.JSON(200, APIResponse{
        Code:    ErrCodeSuccess,
        Message: "success",
        Data:    data,
        TraceID: getTraceID(c),
    })
}
```

### 数据库设计扩展
```sql
-- API访问控制表
CREATE TABLE api_clients (
    id BIGSERIAL PRIMARY KEY,
    client_id VARCHAR(50) UNIQUE NOT NULL,
    client_name VARCHAR(100) NOT NULL,
    api_key VARCHAR(100) UNIQUE NOT NULL,
    secret VARCHAR(100) NOT NULL,
    permissions JSONB,               -- 权限列表
    rate_limit_per_minute INT DEFAULT 1000,
    ip_whitelist JSONB,             -- IP白名单
    status SMALLINT DEFAULT 1,       -- 1:启用 0:禁用
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- 第三方订单扩展
ALTER TABLE orders ADD COLUMN IF NOT EXISTS api_client_id VARCHAR(50);
ALTER TABLE orders ADD COLUMN IF NOT EXISTS request_params JSONB;
ALTER TABLE orders ADD COLUMN IF NOT EXISTS response_data JSONB;

-- API统计表
CREATE TABLE api_stats (
    id BIGSERIAL PRIMARY KEY,
    date DATE NOT NULL,
    client_id VARCHAR(50) NOT NULL,
    endpoint VARCHAR(200) NOT NULL,
    method VARCHAR(10) NOT NULL,
    request_count BIGINT DEFAULT 0,
    success_count BIGINT DEFAULT 0,
    avg_response_time INT DEFAULT 0,   -- 毫秒
    created_at TIMESTAMP DEFAULT NOW(),
    UNIQUE(date, client_id, endpoint, method)
);
```

## ✅ 验收标准

### 功能验收
- [ ] 第三方平台可通过API启动/停止充电，成功率 > 99%
- [ ] 设备状态查询准确性 > 99.9%
- [ ] 批量操作支持100台设备并发
- [ ] 统计数据实时性 < 1分钟延迟
- [ ] API文档完整，示例可直接运行

### 性能验收
- [ ] API响应时间 P95 < 100ms
- [ ] 支持1000 QPS并发访问
- [ ] 数据库查询优化，复杂查询 < 50ms
- [ ] 缓存命中率 > 90%

### 安全验收
- [ ] API Key认证和权限控制
- [ ] 请求限流和IP白名单
- [ ] 敏感数据加密和脱敏
- [ ] 操作审计日志完整

## 🚀 后续阶段预览

**阶段2预告**: 数据统计分析
- 深度数据挖掘和趋势分析
- 自定义报表和数据导出
- 设备健康度评估

**阶段3预告**: 告警通知系统  
- 智能告警规则引擎
- 多渠道告警推送
- 告警升级和处理流程

---

**准备开始**: 确认需求后即可启动设备控制API开发