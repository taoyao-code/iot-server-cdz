# API文档

## 快速开始

### 生成API文档

```bash
# 一键生成完整API文档
make api-docs
```

### 查看文档

**方式1: 在线Swagger UI**

```
启动服务后访问: http://localhost:7055/swagger/index.html
```

**方式2: 查看生成的文件**

```
api/swagger/swagger.json  - JSON格式
api/swagger/swagger.yaml  - YAML格式  
api/swagger/docs.go       - Go代码
```

**方式3: 手工维护的文档**

```
api/openapi/openapi.yaml  - OpenAPI 3.0规范
docs/api/第三方API文档.md  - 中文说明文档
docs/api/事件推送规范.md    - Webhook规范
```

---

## 第三方集成API

所有第三方平台API都带有完整的Swagger注解,自动生成文档。

### API列表

| 端点 | 方法 | 功能 | 认证 |
|------|------|------|------|
| /api/v1/third/devices/{id}/charge | POST | 启动充电 | ✅ 需要 |
| /api/v1/third/devices/{id}/stop | POST | 停止充电 | ✅ 需要 |
| /api/v1/third/devices/{id} | GET | 查询设备状态 | ✅ 需要 |
| /api/v1/third/orders/{order_no} | GET | 查询订单详情 | ✅ 需要 |
| /api/v1/third/orders | GET | 订单列表查询 | ✅ 需要 |
| /api/v1/third/devices/{id}/params | POST | 设置参数 | ✅ 需要 |
| /api/v1/third/devices/{id}/ota | POST | OTA升级 | ✅ 需要 |

---

## 认证方式

### API Key认证

所有请求需要在Header中提供:

```http
X-Api-Key: your-api-key-here
```

### HMAC签名 (可选,推荐)

```http
X-Signature: hmac-sha256-signature
X-Timestamp: unix-timestamp
X-Nonce: random-nonce
```

---

## 更新API文档

### 添加新API

1. 在Handler方法上添加Swagger注解:

```go
// @Summary 接口摘要
// @Description 详细描述
// @Tags 分组标签
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param id path string true "参数说明"
// @Success 200 {object} ResponseType
// @Router /api/path [method]
func (h *Handler) NewAPI(c *gin.Context) {
    // ...
}
```

2. 重新生成文档:

```bash
make api-docs
```

3. 提交更新:

```bash
git add api/swagger/
git commit -m "docs: 更新API文档"
```

---

## 文档规范

### Swagger注解规范

- `@Summary`: 一句话说明功能
- `@Description`: 详细描述
- `@Tags`: 分组(建议: "第三方API - XXX")
- `@Accept`: 接受的数据格式 (json)
- `@Produce`: 返回的数据格式 (json)
- `@Security`: 认证方式 (ApiKeyAuth)
- `@Param`: 参数定义
  - path/query/header/body
  - 类型: string/integer/object
  - 必填: true/false
- `@Success`: 成功响应 (状态码 {type} 说明)
- `@Failure`: 失败响应
- `@Router`: 路由路径 [方法]

---

## 常用命令

```bash
# 生成API文档
make api-docs

# 只生成不安装
make swagger-gen

# 验证OpenAPI文档
make swagger-validate

# 查看帮助
make help | grep -A 10 "API文档"
```

---

## 注意事项

1. ✅ Swagger文档自动生成,不要手动编辑
2. ✅ 修改代码后运行 `make api-docs` 更新
3. ✅ `api/swagger/` 目录在 .gitignore 中,可选提交
4. ✅ `api/openapi/openapi.yaml` 手工维护,需提交
