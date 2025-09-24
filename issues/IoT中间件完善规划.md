# IoT充电桩中间件完善规划

**更新时间**: 2025-01-13  
**项目定位**: IoT设备交互中间件（非C端业务平台）  
**核心职责**: 设备通信、数据推送、第三方API、运营支撑

## 🎯 项目定位澄清

### 系统架构流程
```
第三方平台(小程序/App) 
    ↓ API调用
IoT中间件(当前系统)
    ↓ 协议通信  
充电桩设备
    ↓ 数据推送
第三方平台(业务处理)
```

### 核心职责
- **设备通信**: 与充电桩设备的协议交互（BKV/GN/AP3000）
- **API服务**: 为第三方平台提供设备控制API 
- **数据推送**: 将设备状态/事件推送到第三方平台
- **运营支撑**: 设备统计、告警通知、数据分析等中间件功能

### 非核心职责（由第三方实现）
- 用户管理和认证
- 计费和支付系统
- C端用户界面
- 订单业务逻辑

## 📊 现状分析

### ✅ 已完成的核心能力
- **协议支持**: BKV(100%)、GN(100%)、AP3000协议完整实现
- **设备管理**: 注册、心跳、在线判定、会话管理
- **基础通信**: TCP网关、消息队列、ACK重试
- **数据存储**: PostgreSQL、设备状态、命令日志
- **第三方集成**: HTTP推送、签名验证、回调机制
- **监控观测**: Prometheus指标、健康检查

### ❌ 缺失的重要功能

#### 1. 第三方API完善
- **设备控制API**: 启动/停止充电、参数设置、远程重启
- **设备查询API**: 实时状态、历史数据、统计信息
- **批量操作API**: 批量设备控制、状态查询
- **告警管理API**: 告警规则配置、历史告警查询

#### 2. 数据统计分析
- **设备统计**: 在线/离线设备数、使用率统计
- **端口统计**: 空闲/使用中/故障端口状态
- **充电统计**: 充电次数、用电量、时长统计
- **性能统计**: 响应时间、成功率、错误率

#### 3. 告警通知系统
- **设备告警**: 离线、故障、异常检测
- **业务告警**: 充电异常、通信超时、队列积压
- **系统告警**: 服务状态、资源使用、性能指标
- **通知渠道**: 多渠道告警推送（HTTP/邮件/短信）

#### 4. 运营管理功能
- **设备配置**: 参数批量配置、固件升级管理
- **数据导出**: 设备数据、统计报表、日志导出
- **系统管理**: 配置管理、日志管理、性能监控

## 🚀 实施规划

### 阶段1: 第三方API完善 (4-5周)

#### Week 1-2: 设备控制API
**目标**: 完善第三方平台控制设备的API接口

**详细任务**:
- [ ] **充电控制API**
  ```go
  POST /api/third/devices/{phyId}/charge/start    // 启动充电
  POST /api/third/devices/{phyId}/charge/stop     // 停止充电
  GET  /api/third/devices/{phyId}/charge/status   // 充电状态
  ```

- [ ] **设备管理API**
  ```go
  POST /api/third/devices/{phyId}/reboot          // 远程重启
  POST /api/third/devices/{phyId}/params          // 参数设置
  GET  /api/third/devices/{phyId}/params          // 参数查询
  POST /api/third/devices/{phyId}/upgrade         // 固件升级
  ```

- [ ] **批量操作API**
  ```go
  POST /api/third/devices/batch/control           // 批量控制
  POST /api/third/devices/batch/status            // 批量状态查询
  GET  /api/third/devices/batch/summary           // 批量摘要
  ```

**技术要点**:
- 统一的错误码和响应格式
- 请求参数验证和业务逻辑检查
- 操作日志记录和审计
- 集成现有的出站队列和重试机制

#### Week 3-4: 设备查询API
**目标**: 为第三方平台提供完整的设备信息查询能力

**详细任务**:
- [ ] **设备状态API**
  ```go
  GET /api/third/devices                          // 设备列表
  GET /api/third/devices/{phyId}                  // 设备详情
  GET /api/third/devices/{phyId}/ports            // 端口状态
  GET /api/third/devices/{phyId}/realtime         // 实时数据
  ```

- [ ] **历史数据API**
  ```go
  GET /api/third/devices/{phyId}/history          // 历史数据
  GET /api/third/devices/{phyId}/logs             // 命令日志
  GET /api/third/devices/{phyId}/alarms           // 告警历史
  ```

- [ ] **统计分析API**
  ```go
  GET /api/third/stats/devices                    // 设备统计
  GET /api/third/stats/usage                      // 使用统计
  GET /api/third/stats/performance                // 性能统计
  ```

**Week 1-2交付物**:
- 完整的第三方控制API
- API文档和测试用例
- 集成测试验证

### 阶段2: 数据统计分析 (3-4周)

#### Week 5-6: 实时统计系统
**目标**: 提供设备和业务的实时统计能力

**详细任务**:
- [ ] **设备状态统计**
  - 在线/离线设备数量统计
  - 设备类型分布统计
  - 地理位置分布统计
  - 设备健康度评估

- [ ] **端口使用统计**
  - 空闲/使用中/故障端口统计
  - 端口使用率趋势分析
  - 端口异常频率统计

- [ ] **充电业务统计**
  - 充电次数和时长统计
  - 用电量统计和趋势
  - 充电成功率统计
  - 异常结束原因分析

**技术实现**:
```go
// 实时统计服务
type StatsService struct {
    repo     *Repository
    cache    *redis.Client
    interval time.Duration
}

// 统计数据结构
type DeviceStats struct {
    OnlineCount    int64            `json:"online_count"`
    OfflineCount   int64            `json:"offline_count"`
    TotalCount     int64            `json:"total_count"`
    TypeDistribution map[string]int64 `json:"type_distribution"`
    RegionStats    []RegionStat     `json:"region_stats"`
}

type PortStats struct {
    IdleCount      int64   `json:"idle_count"`
    ChargingCount  int64   `json:"charging_count"`
    FaultCount     int64   `json:"fault_count"`
    UtilizationRate float64 `json:"utilization_rate"`
}
```

#### Week 7-8: 历史数据分析
**目标**: 提供历史数据的分析和报表功能

**详细任务**:
- [ ] **历史数据聚合**
  - 按小时/天/周/月的数据聚合
  - 设备使用趋势分析
  - 故障模式识别
  - 性能基线建立

- [ ] **报表生成**
  - 设备运行报表
  - 业务统计报表
  - 告警统计报表
  - 自定义报表支持

### 阶段3: 告警通知系统 (3-4周)

#### Week 9-10: 告警规则引擎
**目标**: 建立完整的告警检测和规则管理系统

**详细任务**:
- [ ] **告警规则定义**
  ```yaml
  # 设备离线告警
  device_offline:
    condition: "device_last_seen > 5m"
    level: "warning"
    message: "设备{{.PhyID}}已离线{{.Duration}}"
    
  # 充电异常告警  
  charging_error:
    condition: "charging_error_rate > 10%"
    level: "critical"
    message: "设备{{.PhyID}}充电异常率过高"
  ```

- [ ] **告警检测引擎**
  - 实时监控和规则匹配
  - 告警去重和抑制
  - 告警升级和恢复
  - 告警历史记录

- [ ] **告警管理API**
  ```go
  GET  /api/third/alarms                          // 告警列表
  POST /api/third/alarms/rules                    // 创建告警规则
  PUT  /api/third/alarms/rules/{id}              // 更新告警规则
  POST /api/third/alarms/{id}/ack                // 告警确认
  ```

#### Week 11-12: 通知系统
**目标**: 实现多渠道告警通知和推送

**详细任务**:
- [ ] **通知渠道实现**
  - HTTP Webhook推送
  - 邮件通知支持
  - 短信通知集成
  - 企业微信/钉钉集成

- [ ] **通知策略管理**
  - 通知规则配置
  - 通知频率控制
  - 值班表管理
  - 通知模板定制

### 阶段4: 运营管理功能 (2-3周)

#### Week 13-14: 运营工具
**目标**: 提供完整的运营管理和维护工具

**详细任务**:
- [ ] **设备配置管理**
  - 配置模板管理
  - 批量配置下发
  - 配置版本管理
  - 配置变更审计

- [ ] **数据导出功能**
  - 设备数据导出
  - 统计报表导出
  - 日志数据导出
  - 自定义查询导出

- [ ] **系统管理工具**
  - 系统配置管理
  - 日志查询和分析
  - 性能监控面板
  - 运维操作记录

#### Week 15: 测试和优化
**目标**: 系统集成测试和性能优化

**详细任务**:
- [ ] **集成测试**
  - 完整业务流程测试
  - API压力测试
  - 告警系统测试
  - 故障恢复测试

- [ ] **性能优化**
  - 数据库查询优化
  - 缓存策略优化
  - API响应时间优化
  - 系统资源使用优化

## 🛠️ 技术实现方案

### 统计系统架构
```go
// 统计服务接口
type StatsCollector interface {
    CollectDeviceStats(ctx context.Context) (*DeviceStats, error)
    CollectPortStats(ctx context.Context) (*PortStats, error)
    CollectChargingStats(ctx context.Context, timeRange TimeRange) (*ChargingStats, error)
    ExportReport(ctx context.Context, req *ReportRequest) (*Report, error)
}

// 实时统计实现
type RealTimeStatsCollector struct {
    repo    *Repository
    cache   *redis.Client
    metrics *prometheus.Registry
}

// 定时聚合任务
func (c *RealTimeStatsCollector) StartAggregation(interval time.Duration) {
    ticker := time.NewTicker(interval)
    go func() {
        for range ticker.C {
            c.aggregateStats()
        }
    }()
}
```

### 告警系统架构
```go
// 告警规则引擎
type AlertEngine struct {
    rules    []AlertRule
    detector *MetricsDetector
    notifier *NotificationService
}

// 告警规则定义
type AlertRule struct {
    ID          string              `json:"id"`
    Name        string              `json:"name"`
    Condition   string              `json:"condition"`
    Level       AlertLevel          `json:"level"`
    Message     string              `json:"message"`
    Channels    []NotificationChannel `json:"channels"`
    Throttle    time.Duration       `json:"throttle"`
}

// 通知服务
type NotificationService struct {
    channels map[string]NotificationChannel
    templates map[string]*template.Template
}
```

### API扩展规范
```yaml
# API设计规范
第三方API前缀: /api/third/
内部管理API: /api/internal/
监控API: /api/metrics/

# 统一响应格式
成功: {"code": 0, "data": {...}, "message": "success"}
失败: {"code": 1001, "data": null, "message": "设备不存在"}

# 分页格式  
请求: ?page=1&size=20&sort=created_at&order=desc
响应: {"code": 0, "data": {...}, "pagination": {...}}
```

## 📋 API设计详情

### 设备控制API
```go
// 启动充电
POST /api/third/devices/{phyId}/charge/start
{
    "port_no": 1,
    "mode": 1,           // 1:按时间 2:按电量 3:按功率
    "time_limit": 120,   // 分钟（按时间模式）
    "energy_limit": 50,  // kWh（按电量模式）
    "order_no": "ORD001" // 第三方订单号
}

// 停止充电
POST /api/third/devices/{phyId}/charge/stop
{
    "port_no": 1,
    "reason": "user_stop", // 停止原因
    "order_no": "ORD001"   // 对应订单号
}

// 设备参数设置
POST /api/third/devices/{phyId}/params
{
    "params": {
        "max_current": 32,     // 最大电流
        "voltage_threshold": 220, // 电压阈值
        "temp_threshold": 60    // 温度阈值
    }
}
```

### 统计查询API
```go
// 设备统计
GET /api/third/stats/devices?time_range=24h&group_by=type
{
    "code": 0,
    "data": {
        "online_count": 150,
        "offline_count": 10,
        "total_count": 160,
        "utilization_rate": 0.75,
        "type_distribution": {
            "AC": 120,
            "DC": 40
        }
    }
}

// 充电统计
GET /api/third/stats/charging?start_time=2024-01-01&end_time=2024-01-31
{
    "code": 0,
    "data": {
        "total_sessions": 5000,
        "total_energy": 120000,  // kWh
        "avg_duration": 45,      // 分钟
        "success_rate": 0.98
    }
}
```

### 告警API
```go
// 告警列表
GET /api/third/alarms?level=warning&status=active&page=1&size=20
{
    "code": 0,
    "data": {
        "items": [
            {
                "id": "alarm_001",
                "device_phy_id": "DEV001",
                "level": "warning",
                "message": "设备DEV001已离线5分钟",
                "status": "active",
                "created_at": "2024-01-13T10:00:00Z"
            }
        ],
        "pagination": {
            "page": 1,
            "size": 20,
            "total": 100
        }
    }
}

// 创建告警规则
POST /api/third/alarms/rules
{
    "name": "设备离线告警",
    "condition": "device_offline > 5m",
    "level": "warning",
    "message": "设备{{.PhyID}}已离线{{.Duration}}",
    "channels": ["webhook", "email"],
    "throttle": "10m"
}
```

## 🎯 验收标准

### 功能验收
- [ ] 第三方平台可通过API完整控制设备（启停充、参数设置）
- [ ] 提供完整的设备状态和统计数据查询
- [ ] 告警系统能及时检测和通知设备异常
- [ ] 支持批量操作和数据导出功能
- [ ] 具备完善的运营管理工具

### 技术验收
- [ ] API响应时间 P95 < 100ms
- [ ] 支持1000+设备并发监控
- [ ] 告警延迟 < 30秒
- [ ] 统计数据准确率 > 99.9%
- [ ] 系统可用性 > 99.9%

### 运营验收
- [ ] 设备在线率统计准确
- [ ] 告警通知及时有效
- [ ] 数据导出功能完整
- [ ] 运维操作便捷高效

## 📈 预期收益

### 业务价值
- **运营效率**: 提升50%的设备管理效率
- **故障响应**: 减少70%的故障处理时间  
- **数据洞察**: 提供完整的设备运营数据支撑
- **服务质量**: 提升第三方平台的集成体验

### 技术价值
- **稳定性**: 完善的监控和告警体系
- **可扩展**: 支持更多设备和第三方接入
- **可维护**: 完整的运营工具和数据支撑
- **标准化**: 规范的API和数据格式

---

**实施建议**: 优先实施阶段1和阶段3（API和告警），快速提升第三方集成体验和运营能力，然后补充数据统计和管理工具功能。