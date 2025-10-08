#!/bin/bash
set -e

# 备份脚本 - 用于生产环境数据备份

# 颜色定义
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

# 配置
BACKUP_DIR="./backups"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
COMPOSE_FILE="docker-compose.prod.yml"

# 日志函数
log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# 创建备份目录
mkdir -p "$BACKUP_DIR"

# 备份PostgreSQL数据库
backup_postgres() {
    log_info "开始备份PostgreSQL数据库..."
    
    BACKUP_FILE="$BACKUP_DIR/postgres_${TIMESTAMP}.sql"
    
    docker-compose -f "$COMPOSE_FILE" exec -T postgres \
        pg_dump -U "${POSTGRES_USER:-iot}" "${POSTGRES_DB:-iot_server}" \
        > "$BACKUP_FILE"
    
    # 压缩备份文件
    gzip "$BACKUP_FILE"
    
    log_info "PostgreSQL备份完成: ${BACKUP_FILE}.gz"
}

# 备份Redis数据
backup_redis() {
    log_info "开始备份Redis数据..."
    
    # 触发Redis保存
    docker-compose -f "$COMPOSE_FILE" exec redis \
        redis-cli -a "${REDIS_PASSWORD}" SAVE
    
    # 复制RDB文件
    BACKUP_FILE="$BACKUP_DIR/redis_${TIMESTAMP}.rdb"
    docker cp iot-redis-prod:/data/dump.rdb "$BACKUP_FILE"
    
    # 压缩
    gzip "$BACKUP_FILE"
    
    log_info "Redis备份完成: ${BACKUP_FILE}.gz"
}

# 备份配置文件
backup_configs() {
    log_info "开始备份配置文件..."
    
    BACKUP_FILE="$BACKUP_DIR/configs_${TIMESTAMP}.tar.gz"
    
    tar -czf "$BACKUP_FILE" configs/ .env 2>/dev/null || true
    
    log_info "配置文件备份完成: $BACKUP_FILE"
}

# 清理旧备份（保留最近7天）
cleanup_old_backups() {
    log_info "清理7天前的旧备份..."
    
    find "$BACKUP_DIR" -name "*.gz" -mtime +7 -delete
    find "$BACKUP_DIR" -name "*.sql" -mtime +7 -delete
    find "$BACKUP_DIR" -name "*.rdb" -mtime +7 -delete
    
    log_info "旧备份清理完成"
}

# 显示备份列表
list_backups() {
    log_info "现有备份文件："
    ls -lh "$BACKUP_DIR/"
}

# 恢复PostgreSQL
restore_postgres() {
    local backup_file=$1
    
    if [ -z "$backup_file" ]; then
        log_error "请指定备份文件"
        exit 1
    fi
    
    log_warn "即将恢复数据库，这将覆盖现有数据！"
    read -p "确认继续？(yes/no): " confirm
    
    if [ "$confirm" != "yes" ]; then
        log_info "取消恢复"
        exit 0
    fi
    
    log_info "开始恢复PostgreSQL数据库..."
    
    # 如果是压缩文件，先解压
    if [[ "$backup_file" == *.gz ]]; then
        gunzip -c "$backup_file" | docker-compose -f "$COMPOSE_FILE" exec -T postgres \
            psql -U "${POSTGRES_USER:-iot}" "${POSTGRES_DB:-iot_server}"
    else
        cat "$backup_file" | docker-compose -f "$COMPOSE_FILE" exec -T postgres \
            psql -U "${POSTGRES_USER:-iot}" "${POSTGRES_DB:-iot_server}"
    fi
    
    log_info "数据库恢复完成"
}

# 主函数
main() {
    case "${1:-backup}" in
        backup)
            # 加载环境变量
            if [ -f .env ]; then
                source .env
            fi
            
            log_info "开始完整备份..."
            backup_postgres
            backup_redis
            backup_configs
            cleanup_old_backups
            list_backups
            log_info "备份完成 ✓"
            ;;
        restore)
            if [ -z "$2" ]; then
                log_error "用法: $0 restore <backup_file>"
                exit 1
            fi
            restore_postgres "$2"
            ;;
        list)
            list_backups
            ;;
        cleanup)
            cleanup_old_backups
            ;;
        *)
            echo "用法: $0 {backup|restore|list|cleanup}"
            echo ""
            echo "命令说明："
            echo "  backup           - 完整备份（数据库+Redis+配置）"
            echo "  restore <file>   - 恢复数据库备份"
            echo "  list             - 列出所有备份"
            echo "  cleanup          - 清理旧备份"
            exit 1
            ;;
    esac
}

main "$@"

