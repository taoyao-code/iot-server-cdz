#!/bin/bash
# 服务器端重启脚本
# 功能：安全重启服务（带健康检查和自动回滚）
# 部署位置：在182.43.177.92服务器上运行
# 使用：./restart_service.sh

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# 配置
CONTAINER_NAME="${CONTAINER_NAME:-iot-server-prod}"
BINARY_PATH="${BINARY_PATH:-/app/iot-server}"
BACKUP_DIR="${BACKUP_DIR:-/tmp}"
HEALTH_CHECK_URL="${HEALTH_CHECK_URL:-http://localhost:7055/healthz}"
MAX_WAIT_TIME=30  # 最大等待时间（秒）

print_success() { echo -e "${GREEN}✓${NC} $1"; }
print_failure() { echo -e "${RED}✗${NC} $1"; }
print_warning() { echo -e "${YELLOW}⚠${NC} $1"; }
print_info() { echo -e "${BLUE}→${NC} $1"; }

# 备份当前版本
backup_current() {
    print_info "备份当前版本..."
    
    local backup_file="$BACKUP_DIR/iot-server-backup-$(date +%Y%m%d-%H%M%S)"
    
    if docker exec $CONTAINER_NAME test -f $BINARY_PATH; then
        if docker cp $CONTAINER_NAME:$BINARY_PATH $backup_file; then
            print_success "备份成功: $backup_file"
            echo "$backup_file"
            return 0
        else
            print_warning "备份失败"
            return 1
        fi
    else
        print_warning "当前版本不存在，跳过备份"
        return 1
    fi
}

# 停止容器
stop_container() {
    print_info "停止容器..."
    
    if docker stop $CONTAINER_NAME; then
        print_success "容器已停止"
        return 0
    else
        print_failure "停止容器失败"
        return 1
    fi
}

# 启动容器
start_container() {
    print_info "启动容器..."
    
    if docker start $CONTAINER_NAME; then
        print_success "容器已启动"
        return 0
    else
        print_failure "启动容器失败"
        return 1
    fi
}

# 健康检查
health_check() {
    print_info "执行健康检查..."
    
    local waited=0
    local check_interval=2
    
    # 等待服务启动
    print_info "等待服务启动..."
    sleep 5
    
    while [ $waited -lt $MAX_WAIT_TIME ]; do
        if curl -f -s $HEALTH_CHECK_URL > /dev/null 2>&1; then
            print_success "健康检查通过"
            return 0
        fi
        
        sleep $check_interval
        waited=$((waited + check_interval))
        echo -n "."
    done
    
    echo ""
    print_failure "健康检查失败（超时：${MAX_WAIT_TIME}秒）"
    return 1
}

# 回滚到备份
rollback() {
    local backup_file=$1
    
    if [ -z "$backup_file" ] || [ ! -f "$backup_file" ]; then
        print_failure "未找到备份文件，无法回滚"
        return 1
    fi
    
    print_warning "正在回滚到之前的版本..."
    
    if ! stop_container; then
        print_failure "回滚失败：无法停止容器"
        return 1
    fi
    
    if ! docker cp $backup_file $CONTAINER_NAME:$BINARY_PATH; then
        print_failure "回滚失败：无法复制备份文件"
        # 尝试启动容器
        start_container || true
        return 1
    fi
    
    if ! start_container; then
        print_failure "回滚失败：无法启动容器"
        return 1
    fi
    
    if health_check; then
        print_success "回滚成功"
        return 0
    else
        print_failure "回滚后服务仍异常"
        return 1
    fi
}

# 主函数
main() {
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "  服务重启（带健康检查和回滚）"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    echo "容器名称: $CONTAINER_NAME"
    echo "二进制路径: $BINARY_PATH"
    echo "健康检查: $HEALTH_CHECK_URL"
    echo ""
    
    # 步骤1: 备份
    local backup_file=$(backup_current)
    
    # 步骤2: 重启
    if ! stop_container; then
        print_failure "重启失败：无法停止容器"
        exit 1
    fi
    
    if ! start_container; then
        print_failure "重启失败：无法启动容器"
        
        # 尝试回滚
        if [ -n "$backup_file" ]; then
            rollback "$backup_file"
        fi
        
        exit 1
    fi
    
    # 步骤3: 健康检查
    if ! health_check; then
        print_failure "重启失败：健康检查未通过"
        
        # 自动回滚
        if [ -n "$backup_file" ]; then
            rollback "$backup_file"
        fi
        
        exit 1
    fi
    
    # 成功
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo -e "${GREEN}✓ 服务重启成功${NC}"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    
    # 清理旧备份（保留最近10个）
    if [ -n "$backup_file" ]; then
        print_info "清理旧备份..."
        ls -t $BACKUP_DIR/iot-server-backup-* 2>/dev/null | tail -n +11 | xargs -r rm -f
        print_success "旧备份已清理"
    fi
    
    exit 0
}

# 运行主函数
main

