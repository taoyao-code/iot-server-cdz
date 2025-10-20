# API文档自动生成说明

## 使用Swagger自动生成API文档

### 1. 安装工具

```bash
make swagger-init
```

### 2. 生成API文档

```bash
make api-docs
```

### 3. 查看文档

生成后的文档位置:

- `api/swagger/swagger.json` - JSON格式
- `api/swagger/swagger.yaml` - YAML格式
- `api/swagger/docs.go` - Go代码

### 4. 在线查看

启动服务后访问:

```
http://localhost:7055/swagger/index.html
```

---

## 第三方平台API清单

### 1. 充电控制API

#### POST /api/v1/third/devices/{id}/charge

**功能:** 启动充电

**请求示例:**

```json
{
  "port_no": 1,
  "charge_mode": 1,
  "duration": 3600,
  "amount": 1000,
  "price_per_kwh": 150,
  "service_fee": 100
}
```

#### POST /api/v1/third/devices/{id}/stop

**功能:** 停止充电

---

### 2. 设备查询API

#### GET /api/v1/third/devices/{id}

**功能:** 查询设备状态

**响应示例:**

```json
{
  "code": 0,
  "data": {
    "device_phy_id": "82210225000520",
    "online": true,
    "last_seen_at": 1729407600
  }
}
```

---

### 3. 订单查询API

#### GET /api/v1/third/orders/{order_no}

**功能:** 查询订单详情

#### GET /api/v1/third/orders

**功能:** 订单列表查询

---

### 4. 参数设置API

#### POST /api/v1/third/devices/{id}/params

**功能:** 设置设备参数

---

### 5. OTA升级API

#### POST /api/v1/third/devices/{id}/ota

**功能:** 触发OTA升级

---

## 完整API注解

所有API已添加Swagger注解,包括:

- @Summary - 接口摘要
- @Description - 详细描述  
- @Tags - 分组标签
- @Accept/@Produce - 数据格式
- @Security - 认证方式
- @Param - 参数定义
- @Success/@Failure - 响应定义
- @Router - 路由路径

运行 `make api-docs` 自动生成完整文档!
