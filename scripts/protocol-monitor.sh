#!/bin/bash

# ============================================
# 协议监控工具 - 实时查看组网设备连接和协议数据
# 支持 BKV/GN 协议（包头 fcfe/fcff）
# ============================================

set -e

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
MAGENTA='\033[0;35m'
NC='\033[0m'

log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }
log_title() { echo -e "${CYAN}========== $1 ==========${NC}"; }
log_data() { echo -e "${MAGENTA}[DATA]${NC} $1"; }

TCP_PORT=${TCP_PORT:-7054}
API_PORT=${API_PORT:-7055}

# ============================================
# 1. 实时 TCP 数据抓包
# ============================================
capture_tcp_traffic() {
    log_title "实时 TCP 数据抓包"
    
    log_info "开始抓取 TCP 端口 $TCP_PORT 的数据包..."
    log_info "提示：按 Ctrl+C 停止抓包"
    echo ""
    
    # 检查是否有 root 权限
    if [ "$EUID" -ne 0 ]; then
        log_warn "需要 root 权限进行抓包"
        log_info "请使用: sudo $0 capture"
        exit 1
    fi
    
    # 使用 tcpdump 抓包（十六进制 + ASCII）
    tcpdump -i lo0 -X -s 0 "tcp port $TCP_PORT" 2>/dev/null
}

# ============================================
# 2. 实时查看应用层日志（协议解析）
# ============================================
watch_protocol_logs() {
    local filter="${1:-}"
    
    log_title "实时协议日志"
    
    if [ -n "$filter" ]; then
        log_info "过滤条件: $filter"
        docker-compose logs -f --tail=50 iot-server | grep -i --line-buffered "$filter"
    else
        log_info "显示所有 BKV/GN 协议相关日志"
        docker-compose logs -f --tail=50 iot-server | grep -i --line-buffered -E "bkv|gn|fcfe|fcff|网关|插座|parse|decode|frame"
    fi
}

# ============================================
# 3. 查看特定设备的消息流
# ============================================
watch_device() {
    local device_id="$1"
    
    if [ -z "$device_id" ]; then
        log_error "请提供设备 ID（网关ID或插座MAC）"
        echo "用法: $0 device <设备ID>"
        exit 1
    fi
    
    log_title "监控设备: $device_id"
    
    log_info "实时跟踪设备 $device_id 的所有消息..."
    log_info "提示：按 Ctrl+C 停止监控"
    echo ""
    
    docker-compose logs -f --tail=100 iot-server | grep --line-buffered -i "$device_id"
}

# ============================================
# 4. 解析十六进制数据包（BKV/GN 协议）
# ============================================
parse_hex_packet() {
    local hex_data="$1"
    
    if [ -z "$hex_data" ]; then
        log_error "请提供十六进制数据"
        echo "用法: $0 parse-hex <十六进制数据>"
        echo "示例: $0 parse-hex fcfe002e..."
        exit 1
    fi
    
    log_title "解析协议数据包"
    
    # 使用 Python 解析（如果安装了）
    if command -v python3 &> /dev/null; then
        python3 << EOF
import sys

hex_data = "$hex_data".replace(" ", "").replace(":", "")
try:
    data = bytes.fromhex(hex_data)
except ValueError as e:
    print(f"错误：无效的十六进制数据 - {e}")
    sys.exit(1)

print("=" * 60)
print("原始数据 (十六进制):")
print(" ".join([f"{b:02X}" for b in data]))
print()
print("=" * 60)
print("协议解析:")
print("-" * 60)

# BKV/GN 协议解析（包头 fcfe 或 fcff）
if len(data) >= 2:
    header = data[0:2]
    
    if header == bytes([0xfc, 0xfe]):
        print("协议类型: BKV/GN（设备上报）")
        print(f"包头:       fcfe")
    elif header == bytes([0xfc, 0xff]):
        print("协议类型: BKV/GN（服务器下发）")
        print(f"包头:       fcff")
    else:
        print(f"协议类型: 未知（包头 {header.hex()}）")
        sys.exit(0)
    
    print()
    
    if len(data) >= 4:
        # 包长
        pkg_len = int.from_bytes(data[2:4], byteorder='big')
        print(f"包长:       {pkg_len} (0x{pkg_len:04X})")
        
        if len(data) >= 6:
            # 命令
            cmd = int.from_bytes(data[4:6], byteorder='big')
            cmd_names = {
                0x0000: "心跳",
                0x0005: "组网命令",
                0x0015: "控制命令",
                0x1000: "BKV兼容包",
            }
            cmd_name = cmd_names.get(cmd, f"未知(0x{cmd:04X})")
            print(f"命令:       0x{cmd:04X} ({cmd_name})")
            
            if len(data) >= 10:
                # 帧流水号
                frame_id = int.from_bytes(data[6:10], byteorder='big')
                print(f"帧流水号:   0x{frame_id:08X}")
                
                if len(data) >= 11:
                    # 数据方向
                    direction = data[10]
                    dir_text = "上行(设备->服务器)" if direction == 0x01 else "下行(服务器->设备)"
                    print(f"数据方向:   0x{direction:02X} ({dir_text})")
                    
                    if len(data) >= 18:
                        # 网关 ID（7字节）
                        gateway_id = data[11:18].hex().upper()
                        print(f"网关 ID:    {gateway_id}")
                        
                        # 数据体
                        if len(data) > 20:
                            payload = data[18:-2]  # 减去包尾校验
                            print(f"\n数据体 ({len(payload)} 字节):")
                            if len(payload) <= 32:
                                print(f"  Hex: {' '.join([f'{b:02X}' for b in payload])}")
                            else:
                                print(f"  Hex: {' '.join([f'{b:02X}' for b in payload[:32]])} ...")
                                print(f"  (显示前 32 字节，共 {len(payload)} 字节)")
                        
                        # 校验和
                        if len(data) >= 2:
                            checksum = data[-2]
                            print(f"\n校验和:     0x{checksum:02X}")
                        
                        # 包尾
                        if len(data) >= 2:
                            tail = data[-2:]
                            print(f"包尾:       {tail.hex().upper()}")
else:
    print("数据包太短，无法解析")

print("=" * 60)
EOF
    else
        log_warn "Python3 未安装，无法解析协议"
        echo "原始十六进制数据:"
        echo "$hex_data"
    fi
}

# ============================================
# 5. 实时统计（每秒刷新）
# ============================================
watch_stats() {
    log_title "实时协议统计"
    
    log_info "每 2 秒刷新一次统计数据..."
    log_info "提示：按 Ctrl+C 停止"
    echo ""
    
    while true; do
        clear
        echo "======================================"
        echo "  协议统计 - $(date '+%Y-%m-%d %H:%M:%S')"
        echo "======================================"
        echo ""
        
        # TCP 连接
        echo "【TCP 连接】"
        curl -s "http://localhost:$API_PORT/metrics" 2>/dev/null | grep "^tcp_accept_total" | sed 's/tcp_accept_total/  累计连接数:   /' || echo "  无数据"
        curl -s "http://localhost:$API_PORT/metrics" 2>/dev/null | grep "^tcp_bytes_received_total" | sed 's/tcp_bytes_received_total/  累计接收字节: /' || echo "  无数据"
        echo ""
        
        # BKV 协议解析
        echo "【BKV 协议解析】"
        curl -s "http://localhost:$API_PORT/metrics" 2>/dev/null | grep "bkv_parse_total" | sed 's/bkv_parse_total/  解析统计:     /' || echo "  无数据"
        echo ""
        
        # 命令路由
        echo "【命令路由】"
        curl -s "http://localhost:$API_PORT/metrics" 2>/dev/null | grep "bkv_route_total\|gn_route_total" || echo "  无数据"
        echo ""
        
        # 会话状态
        echo "【设备会话】"
        curl -s "http://localhost:$API_PORT/metrics" 2>/dev/null | grep "^session_online_count" | sed 's/session_online_count/  在线设备数:   /' || echo "  无数据"
        curl -s "http://localhost:$API_PORT/metrics" 2>/dev/null | grep "^session_heartbeat_total" | sed 's/session_heartbeat_total/  累计心跳数:   /' || echo "  无数据"
        echo ""
        
        # 出站队列
        echo "【出站队列】"
        curl -s "http://localhost:$API_PORT/metrics" 2>/dev/null | grep 'outbound_queue_size' || echo "  无数据"
        
        sleep 2
    done
}

# ============================================
# 6. 查看最近的协议消息
# ============================================
recent_messages() {
    local count="${1:-20}"
    
    log_title "最近的协议消息 (最近 $count 条)"
    
    docker-compose logs --tail="$count" iot-server | grep -i -E "bkv|gn|fcfe|fcff|parse|decode|frame|网关|插座" | while read line; do
        # 提取关键信息并高亮
        if echo "$line" | grep -qi "error"; then
            echo -e "${RED}$line${NC}"
        elif echo "$line" | grep -qi "warn"; then
            echo -e "${YELLOW}$line${NC}"
        elif echo "$line" | grep -qi "网关\|gateway"; then
            echo -e "${GREEN}$line${NC}"
        else
            echo "$line"
        fi
    done
}

# ============================================
# 7. 查看当前在线设备
# ============================================
list_online_devices() {
    log_title "当前在线设备"
    
    log_info "从 Redis 查询会话..."
    docker-compose exec redis redis-cli KEYS "session:*" 2>/dev/null | while read key; do
        if [ "$key" != "(empty array)" ] && [ -n "$key" ]; then
            # 提取设备 ID
            device_id=$(echo "$key" | sed 's/session://')
            
            # 查询会话信息
            info=$(docker-compose exec redis redis-cli HGETALL "$key" 2>/dev/null)
            
            if [ -n "$info" ]; then
                echo ""
                echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
                echo -e "${GREEN}设备: $device_id${NC}"
                echo "$info" | while read field; do
                    read value || break
                    echo "  $field: $value"
                done
            fi
        fi
    done
    
    echo ""
    log_info "从数据库查询设备..."
    docker-compose exec postgres psql -U iot -d iot_server -c \
        "SELECT phy_id, is_online, last_heartbeat, created_at 
         FROM devices 
         WHERE is_online = true 
         ORDER BY last_heartbeat DESC 
         LIMIT 20;" 2>/dev/null || log_warn "数据库查询失败"
}

# ============================================
# 8. 持续监控模式（综合视图）
# ============================================
monitor_live() {
    log_title "协议实时监控（综合视图）"
    
    # 使用 tmux 分屏（如果可用）
    if command -v tmux &> /dev/null; then
        log_info "启动 tmux 多窗口监控..."
        
        tmux new-session -d -s iot-monitor
        
        # 窗口 1: 实时日志
        tmux rename-window -t iot-monitor:0 'Logs'
        tmux send-keys -t iot-monitor:0 "cd $(pwd) && docker-compose logs -f --tail=50 iot-server | grep -i --line-buffered -E 'bkv|gn|fcfe|fcff|网关|插座'" C-m
        
        # 窗口 2: 统计数据
        tmux new-window -t iot-monitor:1 -n 'Stats'
        tmux send-keys -t iot-monitor:1 "cd $(pwd) && $0 stats" C-m
        
        # 窗口 3: 在线设备
        tmux new-window -t iot-monitor:2 -n 'Devices'
        tmux send-keys -t iot-monitor:2 "cd $(pwd) && watch -n 5 \"$0 devices\"" C-m
        
        # 附加到会话
        tmux attach-session -t iot-monitor
    else
        log_warn "tmux 未安装，使用简化监控模式"
        log_info "建议安装 tmux: brew install tmux"
        echo ""
        
        # 简化模式：只显示日志
        watch_protocol_logs
    fi
}

# ============================================
# 帮助信息
# ============================================
usage() {
    cat << EOF
协议监控工具 - 实时查看组网设备连接和协议数据

支持协议：BKV/GN（包头 fcfe/fcff）

用法: $0 <命令> [参数]

实时监控命令：
  live                          综合监控（tmux 多窗口，推荐）
  logs [过滤条件]               实时协议日志
  device <设备ID>               监控特定设备（网关ID或插座MAC）
  stats                         实时统计（每 2 秒刷新）
  devices                       查看在线设备列表

数据包分析：
  capture                       实时抓取 TCP 数据包（需要 sudo）
  parse-hex <十六进制>          解析 BKV/GN 协议数据包
  recent [条数]                 查看最近的协议消息（默认 20 条）

示例：
  # 综合监控（推荐）
  $0 live

  # 查看实时日志（过滤特定网关）
  $0 logs 82200520004869

  # 查看实时统计
  $0 stats

  # 监控特定设备
  $0 device 82200520004869

  # 抓取 TCP 数据包
  sudo $0 capture

  # 解析十六进制数据包
  $0 parse-hex fcfe002e00000000000001822005200048693839...

  # 查看最近 50 条消息
  $0 recent 50

  # 查看在线设备
  $0 devices

环境变量：
  TCP_PORT        TCP 端口（默认: 7054，BKV协议端口）
  API_PORT        API 端口（默认: 7055）

提示：
  - 使用 debug 日志级别可以看到详细的协议解析过程
  - 使用 'live' 命令获得最佳监控体验（需要 tmux）
  - 抓包命令需要 sudo 权限
  - BKV 协议包头为 fcfe（上行）或 fcff（下行）

EOF
}

# ============================================
# 主程序
# ============================================
main() {
    local cmd="${1:-help}"
    shift || true
    
    case "$cmd" in
        live|monitor)
            monitor_live
            ;;
        logs|log)
            watch_protocol_logs "$@"
            ;;
        device|dev)
            watch_device "$@"
            ;;
        stats|stat)
            watch_stats
            ;;
        devices|list)
            list_online_devices
            ;;
        capture|cap)
            capture_tcp_traffic
            ;;
        parse-hex|parse)
            parse_hex_packet "$@"
            ;;
        recent|rec)
            recent_messages "$@"
            ;;
        help|--help|-h)
            usage
            ;;
        *)
            log_error "未知命令: $cmd"
            echo ""
            usage
            exit 1
            ;;
    esac
}

main "$@"
