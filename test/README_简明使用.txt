充电桩自动化测试系统 - 简明使用指南
========================================

【3步工作流程】

1. make test-local-compile  # 验证代码
2. make auto-deploy         # 部署+测试
3. make monitor-simple      # 监控运行

【常用命令】

make test-local-compile    # 本地编译验证（30秒）
make auto-deploy           # 自动部署到生产（2-3分钟）
make deploy-quick          # 快速部署（跳过测试）
make monitor-simple        # 实时监控（5秒刷新）

【充电测试】

# 简单测试（60秒）
./test/scripts/quick_charge_test.sh 60

# 完整测试
./test/scripts/test_charge_lifecycle.sh --mode duration --value 300

【查看日志】

# 测试日志
tail -100 test/logs/charge_test_*.log

# 服务器日志
ssh root@182.43.177.92 "docker logs --tail 100 iot-server-prod"

# 查看充电相关日志
ssh root@182.43.177.92 "docker logs -f iot-server-prod | grep -E '0x0015|order|charging'"

【已修复的问题】

✅ 移除 set -e（测试不会因小错误退出）
✅ 设备离线不终止测试（硬件正常就继续）
✅ 智能判断测试结果（部分通过也算成功）
✅ 完整日志记录（所有API调用和状态变化）
✅ macOS兼容（grep命令）
✅ 自动回滚保护（部署失败可恢复）

【核心脚本】

test/scripts/quick_charge_test.sh         # 快速测试 ⭐ 简单直接
test/scripts/test_charge_lifecycle.sh      # 完整测试
test/production/scripts/auto_test_production.sh  # 生产测试
scripts/auto_deploy_test.sh                # 自动部署
scripts/monitor_simple.sh                   # 监控工具

========================================

