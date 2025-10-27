# IoT充电桩生产环境测试指南

> **测试环境:** 182.43.177.92  
> **测试日期:** 请填写 ___________  
> **测试人员:** 请填写 ___________  

## 📋 测试概述

本测试方案旨在验证IoT充电桩系统在真实生产环境下的完整业务流程，包括：

- ✅ 设备连接与心跳
- ✅ 充电指令下发
- ✅ 实时数据上报
- ✅ 第三方平台对接
- ✅ 异常场景处理
- ✅ 性能与可靠性

## 🎯 测试目标

验证系统可以安全、稳定地投入生产环境使用，满足以下标准：

- 功能完整性 ≥ 100%
- 数据准确性误差 < 5%
- API响应时间 < 200ms
- 指令下发成功率 > 99%
- 设备重连时间 < 10秒

## 📂 测试资源

- **测试脚本:** `test/production/scripts/`
- **监控脚本:** `test/production/scripts/monitor_*.sh`
- **测试数据记录:** `test/production/test_results.md`
- **问题追踪:** `test/production/issues.md`

## ⚙️ 测试前准备

### 1. 环境访问权限

确保您有以下权限：

- [ ] SSH访问服务器 182.43.177.92
- [ ] Docker命令执行权限
- [ ] 数据库查询权限
- [ ] Redis访问权限

### 2. 物理设备准备

- [ ] 设备1 (82210225000520) 已上电并连接网络
- [ ] 设备2 (82241218000382) 已上电并连接网络
- [ ] 准备至少1个充电插头（电动车充电器）

### 3. 第三方平台准备

- [ ] Webhook接收端点已部署
- [ ] Webhook密钥已配置
- [ ] 事件接收日志已准备

### 4. 测试工具安装

```bash
# 安装必要工具
sudo apt-get update
sudo apt-get install -y curl jq apache2-utils postgresql-client

# 验证工具安装
curl --version
jq --version
ab -V
psql --version
```

## 🚀 快速开始

### Step 1: 克隆测试脚本到本地

```bash
# 在本地机器执行
cd /Users/zhanghai/code/充电桩/iot-server/test/production
chmod +x scripts/*.sh
```

### Step 2: 执行环境检查

```bash
# 执行环境验证脚本
./scripts/01_env_check.sh
```

如果所有检查通过，继续下一步。

### Step 3: 开始基础测试

```bash
# 执行基础充电流程测试
./scripts/02_basic_charging_test.sh
```

### Step 4: 执行异常场景测试

```bash
# 执行异常场景测试（需要物理操作设备）
./scripts/03_exception_test.sh
```

### Step 5: 性能压力测试

```bash
# 执行性能测试
./scripts/04_performance_test.sh
```

### Step 6: 生成测试报告

```bash
# 生成完整测试报告
./scripts/generate_report.sh
```

## 📊 测试执行顺序

建议按照以下顺序执行测试：

```
阶段1: 环境验证 (30分钟)
  ↓
阶段2: 基础充电流程 (1小时)
  ↓
阶段3: 异常场景测试 (1.5小时)
  ↓
阶段4: 性能压力测试 (1小时)
  ↓
阶段5: 数据一致性验证 (30分钟)
  ↓
阶段6: 监控验证 (30分钟)
  ↓
最终验收 (30分钟)
```

**总计时间:** 约5-6小时

## 📝 测试记录

### 测试环境信息

| 项目 | 值 | 备注 |
|------|-----|------|
| 服务器IP | 182.43.177.92 | |
| HTTP端口 | 7055 | |
| TCP端口 | 7065 | BKV协议 |
| 设备1 | 82210225000520 | |
| 设备2 | 82241218000382 | |
| API Key | sk_test_1234567890 | |
| 测试开始时间 | __________ | |
| 测试结束时间 | __________ | |

### 快速命令参考

```bash
# 查看服务健康
curl http://182.43.177.92:7055/healthz

# 查看设备在线状态
./scripts/check_device_online.sh 82210225000520

# 实时监控日志
./scripts/monitor_logs.sh

# 查看活跃订单
./scripts/check_active_orders.sh

# 查看Redis队列
./scripts/check_redis_queue.sh
```

## 🆘 故障排查

### 问题1: 设备显示离线

**排查步骤:**

1. 检查设备网络连接
2. 查看服务器日志: `docker logs --tail 50 iot-server-prod`
3. 验证TCP端口: `telnet 182.43.177.92 7065`

### 问题2: API返回401

**排查步骤:**

1. 检查API Key是否正确
2. 查看配置文件: `configs/production.yaml`
3. 重启服务: `docker restart iot-server-prod`

### 问题3: Webhook未收到推送

**排查步骤:**

1. 检查webhook_url配置
2. 验证网络连通性: `curl -X POST webhook_url`
3. 查看事件队列: `docker exec -it iot-redis-prod redis-cli -a 123456 LLEN "thirdparty:event_queue"`

### 问题4: 订单创建失败

**排查步骤:**

1. 检查端口状态是否空闲
2. 查看数据库连接
3. 查看服务器错误日志

## 📞 支持联系

- **技术支持:** 请记录问题详情并联系项目负责人
- **紧急问题:** 如果测试中发现严重bug，立即停止测试并报告

## ✅ 验收标准

测试完成后，确认以下所有项目：

### 功能验收

- [ ] 设备上线和心跳正常
- [ ] 充电指令下发成功
- [ ] 订单创建和确认
- [ ] 充电进度实时上报
- [ ] 订单正常结算
- [ ] 远程停止功能
- [ ] Webhook事件推送
- [ ] 异常场景恢复

### 性能验收

- [ ] API响应时间 < 200ms
- [ ] 指令下发成功率 > 99%
- [ ] 设备重连时间 < 10秒
- [ ] 支持100+ QPS

### 数据验收

- [ ] 充电时长误差 < 5秒
- [ ] 电量计量准确
- [ ] 金额计算正确
- [ ] 数据无丢失

---

**准备好了吗？让我们开始测试！** 🚀
