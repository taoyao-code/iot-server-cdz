# E2E 充电流程测试

## 使用方法

### 编译

```bash
cd test/e2e
go build -o charge_test charge_test.go
```

### 运行

```bash
./charge_test
```

### 自定义配置

编辑 `charge_test.go` 中的常量:

- `ServerURL`: 服务器地址
- `APIKey`: API 密钥
- `DeviceID`: 设备 ID
- `PortNo`: 端口号

## 测试流程

1. ✓ 检查设备在线状态
2. ✓ 创建充电订单
3. ✓ 等待指令下发
4. 🔌 **手动插入充电器**
5. ✓ 等待订单状态变为 charging
6. ✓ 监控充电进度
7. ✓ 验证订单结果

## 注意事项

- **必须手动插入充电器**才能进入 charging 状态
- 测试时长 60 秒（可在代码中修改）
- 所有 HTTP 错误都会清晰提示
- 无 Shell 脚本依赖，纯 Go 实现
