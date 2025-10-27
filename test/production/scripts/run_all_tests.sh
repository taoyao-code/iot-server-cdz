#!/bin/bash

# 一键执行所有测试脚本
# 功能: 按顺序执行所有测试阶段，生成最终报告
# 使用: ./run_all_tests.sh

set -e

# 加载辅助函数
SCRIPT_DIR=$(dirname "$0")
source "$SCRIPT_DIR/helper_functions.sh"

# 测试日志文件
LOG_FILE="../test_run_$(date '+%Y%m%d_%H%M%S').log"
TEST_START_TIME=$(date +%s)

# 测试结果统计
TOTAL_STAGES=6
PASSED_STAGES=0
FAILED_STAGES=0

# 记录日志
log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1" | tee -a "$LOG_FILE"
}

# 运行测试阶段
run_stage() {
    local stage_num=$1
    local stage_name=$2
    local script_name=$3
    local optional=$4
    
    print_header "阶段 $stage_num: $stage_name"
    log "开始阶段 $stage_num: $stage_name"
    
    echo ""
    
    if [ "$optional" = "optional" ]; then
        read -p "是否执行此阶段? (y/n, 默认y): " run_it
        run_it=${run_it:-y}
        
        if [ "$run_it" != "y" ] && [ "$run_it" != "Y" ]; then
            print_warning "跳过阶段 $stage_num"
            log "跳过阶段 $stage_num: $stage_name"
            return 0
        fi
    fi
    
    local stage_start=$(date +%s)
    
    if [ -f "$SCRIPT_DIR/$script_name" ]; then
        if bash "$SCRIPT_DIR/$script_name" 2>&1 | tee -a "$LOG_FILE"; then
            local stage_end=$(date +%s)
            local duration=$((stage_end - stage_start))
            
            print_success "阶段 $stage_num 完成 (耗时: ${duration}秒)"
            log "阶段 $stage_num 完成，耗时: ${duration}秒"
            ((PASSED_STAGES++))
            return 0
        else
            local stage_end=$(date +%s)
            local duration=$((stage_end - stage_start))
            
            print_failure "阶段 $stage_num 失败 (耗时: ${duration}秒)"
            log "阶段 $stage_num 失败，耗时: ${duration}秒"
            ((FAILED_STAGES++))
            
            echo ""
            read -p "是否继续执行后续测试? (y/n): " continue_test
            
            if [ "$continue_test" != "y" ] && [ "$continue_test" != "Y" ]; then
                return 1
            fi
            
            return 0
        fi
    else
        print_failure "找不到测试脚本: $script_name"
        log "错误: 找不到测试脚本 $script_name"
        ((FAILED_STAGES++))
        return 0
    fi
}

# 主测试流程
print_header "IoT充电桩完整测试套件"
log "========================================="
log "测试开始"
log "========================================="

echo ""
echo "测试日志: $LOG_FILE"
echo "测试服务器: $TEST_SERVER"
echo "测试设备: $TEST_DEVICE1, $TEST_DEVICE2"
echo ""

print_warning "重要提示:"
echo "  1. 本脚本将执行所有测试阶段，预计耗时4-6小时"
echo "  2. 部分测试需要手动操作设备（如插入充电插头、断开网络等）"
echo "  3. 请确保有充足的时间完成整个测试流程"
echo "  4. 建议在独立的测试环境中执行"
echo ""

read -p "确认开始完整测试? (y/n): " start_test

if [ "$start_test" != "y" ] && [ "$start_test" != "Y" ]; then
    echo "测试已取消"
    exit 0
fi

echo ""
log "用户确认开始测试"

# ==================== 执行各阶段测试 ====================

# 阶段1: 环境验证
run_stage 1 "环境验证与准备" "01_env_check.sh"
echo ""

# 阶段2: 基础充电流程
run_stage 2 "基础充电流程测试" "02_basic_charging_test.sh"
echo ""

# 阶段3: 异常场景测试
run_stage 3 "异常场景测试" "03_exception_test.sh" "optional"
echo ""

# 阶段4: 性能测试
run_stage 4 "性能与压力测试" "04_performance_test.sh" "optional"
echo ""

# 阶段5: 数据一致性验证
run_stage 5 "数据一致性验证" "05_data_consistency_check.sh"
echo ""

# 阶段6: 监控验证
run_stage 6 "监控与可观测性验证" "06_monitoring_check.sh"
echo ""

# ==================== 生成最终报告 ====================
print_header "生成最终报告"

if bash "$SCRIPT_DIR/generate_final_report.sh" 2>&1 | tee -a "$LOG_FILE"; then
    print_success "最终报告已生成"
else
    print_warning "报告生成失败，请手动生成"
fi

echo ""

# ==================== 测试总结 ====================
TEST_END_TIME=$(date +%s)
TOTAL_DURATION=$((TEST_END_TIME - TEST_START_TIME))
HOURS=$((TOTAL_DURATION / 3600))
MINUTES=$(((TOTAL_DURATION % 3600) / 60))
SECONDS=$((TOTAL_DURATION % 60))

print_header "测试总结"

log "========================================="
log "测试结束"
log "========================================="

echo ""
echo "测试统计:"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  总阶段数: $TOTAL_STAGES"
echo "  通过阶段: $PASSED_STAGES"
echo "  失败阶段: $FAILED_STAGES"
echo "  总耗时: ${HOURS}小时${MINUTES}分${SECONDS}秒"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

log "总阶段数: $TOTAL_STAGES"
log "通过阶段: $PASSED_STAGES"
log "失败阶段: $FAILED_STAGES"
log "总耗时: ${HOURS}h ${MINUTES}m ${SECONDS}s"

if [ $FAILED_STAGES -eq 0 ]; then
    echo -e "${GREEN}✓ 所有测试阶段通过${NC}"
    log "结果: 所有测试阶段通过"
    echo ""
    echo "后续步骤:"
    echo "  1. 查看详细测试日志: $LOG_FILE"
    echo "  2. 填写最终测试报告"
    echo "  3. 提交验收审核"
    echo ""
    exit 0
else
    echo -e "${RED}✗ 有 $FAILED_STAGES 个测试阶段失败${NC}"
    log "结果: 有 $FAILED_STAGES 个测试阶段失败"
    echo ""
    echo "后续步骤:"
    echo "  1. 查看失败原因: $LOG_FILE"
    echo "  2. 修复发现的问题"
    echo "  3. 重新执行失败的测试阶段"
    echo ""
    exit 1
fi

