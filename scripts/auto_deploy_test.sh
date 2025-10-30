#!/bin/bash
# 自动部署+测试脚本
# 功能：本地验证 → 编译 → 上传 → 重启 → 健康检查 → 自动测试
# 使用：./auto_deploy_test.sh

# 注意：保留 set -e，部署失败应该立即停止
set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

# 配置
SERVER="${SERVER:-182.43.177.92}"
SERVER_USER="${SERVER_USER:-root}"
SSH_KEY="${SSH_KEY:-$HOME/.ssh/id_rsa}"
SERVER_PATH="${SERVER_PATH:-/dataDisk/wwwroot/iot/iot-server/iot-server}"
CONTAINER_NAME="${CONTAINER_NAME:-iot-server-prod}"
HTTP_PORT="${HTTP_PORT:-7055}"

# 选项
SKIP_TEST=false
SKIP_BACKUP=false
AUTO_ROLLBACK=true

# 打印函数
print_header() {
    echo ""
    echo -e "${BOLD}${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${BOLD}${BLUE}  $1${NC}"
    echo -e "${BOLD}${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
}

print_step() {
    echo ""
    echo -e "${CYAN}▶ $1${NC}"
}

print_success() {
    echo -e "${GREEN}✓${NC} $1"
}

print_failure() {
    echo -e "${RED}✗${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}⚠${NC} $1"
}

print_info() {
    echo -e "  $1"
}

# 解析参数
parse_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            --skip-test)
                SKIP_TEST=true
                shift
                ;;
            --skip-backup)
                SKIP_BACKUP=true
                shift
                ;;
            --no-rollback)
                AUTO_ROLLBACK=false
                shift
                ;;
            --help)
                show_usage
                exit 0
                ;;
            *)
                echo "未知参数: $1"
                show_usage
                exit 1
                ;;
        esac
    done
}

show_usage() {
    cat << EOF
使用方法: $0 [选项]

选项:
  --skip-test       跳过自动测试
  --skip-backup     跳过远程备份
  --no-rollback     失败时不自动回滚
  --help            显示帮助信息

示例:
  # 完整部署（含测试）
  $0

  # 快速部署（不测试）
  $0 --skip-test

  # 无备份快速部署
  $0 --skip-test --skip-backup
EOF
}

# 步骤1: 本地编译验证
local_compile_check() {
    print_step "[1/8] 本地编译验证"
    
    local script_dir=$(cd "$(dirname "$0")" && pwd)
    local compile_script="$script_dir/../test/local/compile_check.sh"
    
    if [ ! -f "$compile_script" ]; then
        print_warning "编译验证脚本不存在，跳过本地验证"
        return 0
    fi
    
    if bash "$compile_script"; then
        print_success "本地验证通过"
        return 0
    else
        print_failure "本地验证失败"
        return 1
    fi
}

# 步骤2: 编译生产版本
build_production() {
    print_step "[2/8] 编译生产版本"
    
    local project_root=$(cd "$(dirname "$0")/.." && pwd)
    cd "$project_root"
    
    print_info "编译Linux版本..."
    
    if CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/iot-server-linux ./cmd/server; then
        local size=$(ls -lh bin/iot-server-linux | awk '{print $5}')
        print_success "编译成功 (大小: $size)"
        return 0
    else
        print_failure "编译失败"
        return 1
    fi
}

# 步骤3: 上传到服务器
upload_to_server() {
    print_step "[3/8] 上传到服务器"
    
    if [ ! -f "$SSH_KEY" ]; then
        print_failure "SSH密钥不存在: $SSH_KEY"
        return 1
    fi
    
    local project_root=$(cd "$(dirname "$0")/.." && pwd)
    local binary="$project_root/bin/iot-server-linux"
    
    if [ ! -f "$binary" ]; then
        print_failure "二进制文件不存在: $binary"
        return 1
    fi
    
    print_info "上传到 $SERVER_USER@$SERVER:$SERVER_PATH/bin/"
    
    if scp -i "$SSH_KEY" "$binary" "$SERVER_USER@$SERVER:$SERVER_PATH/bin/iot-server-new" 2>&1; then
        print_success "上传成功"
        return 0
    else
        print_failure "上传失败"
        return 1
    fi
}

# 步骤4: 远程备份
remote_backup() {
    if [ "$SKIP_BACKUP" = "true" ]; then
        print_step "[4/8] 远程备份 (已跳过)"
        return 0
    fi
    
    print_step "[4/8] 远程备份当前版本"
    
    ssh -i "$SSH_KEY" "$SERVER_USER@$SERVER" << 'EOF'
        if docker exec iot-server-prod test -f /app/iot-server; then
            docker cp iot-server-prod:/app/iot-server /tmp/iot-server-backup-$(date +%Y%m%d-%H%M%S)
            echo "备份成功"
        else
            echo "当前版本不存在，跳过备份"
        fi
EOF
    
    print_success "远程备份完成"
}

# 步骤5: 替换并重启服务
restart_service() {
    print_step "[5/8] 替换并重启服务"
    
    print_info "停止容器..."
    if ! ssh -i "$SSH_KEY" "$SERVER_USER@$SERVER" "docker stop $CONTAINER_NAME" 2>&1; then
        print_failure "停止容器失败"
        return 1
    fi
    
    print_info "替换二进制文件..."
    if ! ssh -i "$SSH_KEY" "$SERVER_USER@$SERVER" \
        "docker cp $SERVER_PATH/bin/iot-server-new $CONTAINER_NAME:/app/iot-server" 2>&1; then
        print_failure "替换失败"
        print_warning "正在恢复容器..."
        ssh -i "$SSH_KEY" "$SERVER_USER@$SERVER" "docker start $CONTAINER_NAME" 2>&1 || true
        return 1
    fi
    
    print_info "启动容器..."
    if ! ssh -i "$SSH_KEY" "$SERVER_USER@$SERVER" "docker start $CONTAINER_NAME" 2>&1; then
        print_failure "启动容器失败"
        return 1
    fi
    
    print_success "服务已重启"
    return 0
}

# 步骤6: 健康检查
health_check() {
    print_step "[6/8] 健康检查"
    
    print_info "等待服务启动 (10秒)..."
    sleep 10
    
    local max_attempts=5
    local attempt=1
    
    while [ $attempt -le $max_attempts ]; do
        print_info "检查健康状态 (尝试 $attempt/$max_attempts)..."
        
        local response=$(curl -s -w "\n%{http_code}" "http://$SERVER:$HTTP_PORT/healthz" 2>&1 || echo "ERROR")
        local http_code=$(echo "$response" | tail -1)
        
        if [ "$http_code" = "200" ]; then
            print_success "服务健康检查通过"
            return 0
        fi
        
        attempt=$((attempt + 1))
        sleep 5
    done
    
    print_failure "健康检查失败"
    return 1
}

# 步骤7: 自动测试
run_auto_test() {
    if [ "$SKIP_TEST" = "true" ]; then
        print_step "[7/8] 自动测试 (已跳过)"
        return 0
    fi
    
    print_step "[7/8] 运行自动化测试"
    
    local script_dir=$(cd "$(dirname "$0")" && pwd)
    local test_script="$script_dir/../test/production/scripts/auto_test_production.sh"
    
    if [ ! -f "$test_script" ]; then
        print_warning "测试脚本不存在，跳过测试"
        return 0
    fi
    
    print_info "执行生产环境测试（快速模式）..."
    
    if bash "$test_script" --quick; then
        print_success "自动化测试通过"
        return 0
    else
        print_failure "自动化测试失败"
        return 1
    fi
}

# 步骤8: 回滚（如果需要）
rollback() {
    if [ "$AUTO_ROLLBACK" = "false" ]; then
        print_warning "自动回滚已禁用"
        return 1
    fi
    
    print_step "[回滚] 恢复之前的版本"
    
    print_info "查找最近的备份..."
    local backup=$(ssh -i "$SSH_KEY" "$SERVER_USER@$SERVER" \
        "ls -t /tmp/iot-server-backup-* 2>/dev/null | head -1" || echo "")
    
    if [ -z "$backup" ]; then
        print_failure "未找到备份文件"
        return 1
    fi
    
    print_info "找到备份: $backup"
    print_info "停止容器..."
    ssh -i "$SSH_KEY" "$SERVER_USER@$SERVER" "docker stop $CONTAINER_NAME" 2>&1 || true
    
    print_info "恢复备份..."
    ssh -i "$SSH_KEY" "$SERVER_USER@$SERVER" \
        "docker cp $backup $CONTAINER_NAME:/app/iot-server" 2>&1 || {
        print_failure "恢复失败"
        return 1
    }
    
    print_info "启动容器..."
    ssh -i "$SSH_KEY" "$SERVER_USER@$SERVER" "docker start $CONTAINER_NAME" 2>&1 || {
        print_failure "启动失败"
        return 1
    }
    
    print_success "已回滚到之前的版本"
    
    # 验证回滚后的健康状态
    sleep 10
    local response=$(curl -s -w "\n%{http_code}" "http://$SERVER:$HTTP_PORT/healthz" 2>&1 || echo "ERROR")
    local http_code=$(echo "$response" | tail -1)
    
    if [ "$http_code" = "200" ]; then
        print_success "回滚成功，服务正常"
        return 0
    else
        print_failure "回滚后服务仍异常"
        return 1
    fi
}

# 主函数
main() {
    parse_args "$@"
    
    print_header "自动部署 + 测试"
    echo "部署目标: $SERVER_USER@$SERVER"
    echo "容器名称: $CONTAINER_NAME"
    echo "跳过测试: $([ "$SKIP_TEST" = "true" ] && echo "是" || echo "否")"
    echo "自动回滚: $([ "$AUTO_ROLLBACK" = "true" ] && echo "是" || echo "否")"
    echo ""
    
    # 执行部署流程
    if ! local_compile_check; then
        print_failure "❌ 部署失败：本地验证未通过"
        exit 1
    fi
    
    if ! build_production; then
        print_failure "❌ 部署失败：编译失败"
        exit 1
    fi
    
    if ! upload_to_server; then
        print_failure "❌ 部署失败：上传失败"
        exit 1
    fi
    
    remote_backup || true
    
    if ! restart_service; then
        print_failure "❌ 部署失败：重启服务失败"
        exit 1
    fi
    
    if ! health_check; then
        print_failure "❌ 部署失败：健康检查未通过"
        
        if [ "$AUTO_ROLLBACK" = "true" ]; then
            print_warning "正在自动回滚..."
            rollback || true
        fi
        
        exit 1
    fi
    
    # 运行测试（不强制要求成功）
    if ! run_auto_test; then
        print_warning "⚠️  部署完成，但自动测试未完全通过"
        print_info "可能原因："
        print_info "  - 设备暂时离线（Redis会话超时）"
        print_info "  - 需要人工插入充电插头"
        print_info "  - Webhook接收器未启动"
        echo ""
        
        read -p "是否回滚? (y/n, 默认n): " should_rollback
        should_rollback=${should_rollback:-n}
        
        if [ "$should_rollback" = "y" ] || [ "$should_rollback" = "Y" ]; then
            rollback || true
            exit 1
        else
            print_info "保持当前部署，建议手动验证"
            print_info "运行: make monitor-simple"
            # 继续，不退出
        fi
    fi
    
    # 部署成功
    print_header "部署成功"
    echo ""
    echo -e "${GREEN}✓ 所有步骤完成${NC}"
    echo ""
    echo "后续操作:"
    echo "  - 查看日志: docker logs -f $CONTAINER_NAME"
    echo "  - 监控系统: make monitor"
    echo "  - 运行测试: cd test/scripts && ./test_charge_lifecycle.sh"
    echo ""
    
    exit 0
}

# 运行主函数
main "$@"

