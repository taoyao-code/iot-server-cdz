#!/usr/bin/env python3

"""
Webhook接收端示例
功能: 接收IoT服务器推送的事件，验证签名，记录事件
使用: python3 webhook_receiver.py [port]
"""

import hmac
import hashlib
import json
import time
from datetime import datetime
from flask import Flask, request, jsonify

app = Flask(__name__)

# 配置
WEBHOOK_SECRET = "your-webhook-secret-key"  # 与IoT服务器配置保持一致
PORT = 8888

# 事件存储（内存）
received_events = []
event_ids_seen = set()

# ANSI颜色
GREEN = '\033[92m'
YELLOW = '\033[93m'
RED = '\033[91m'
BLUE = '\033[94m'
CYAN = '\033[96m'
NC = '\033[0m'


def verify_signature(body, signature, timestamp, nonce):
    """验证HMAC-SHA256签名"""
    try:
        # 计算body的SHA256
        body_sha256 = hashlib.sha256(body).hexdigest()
        
        # 构造canonical string
        method = request.method
        path = request.path
        canonical = f"{method}\n{path}\n{timestamp}\n{nonce}\n{body_sha256}"
        
        # 计算HMAC-SHA256
        expected_sig = hmac.new(
            WEBHOOK_SECRET.encode(),
            canonical.encode(),
            hashlib.sha256
        ).hexdigest()
        
        # 比对签名
        return hmac.compare_digest(signature, expected_sig)
    except Exception as e:
        print(f"{RED}✗ 签名验证错误: {e}{NC}")
        return False


@app.route('/webhook', methods=['POST'])
def handle_webhook():
    """处理webhook请求"""
    timestamp_str = datetime.now().strftime('%Y-%m-%d %H:%M:%S')
    
    # 1. 获取请求头
    signature = request.headers.get('X-Signature', '')
    timestamp = request.headers.get('X-Timestamp', '')
    nonce = request.headers.get('X-Nonce', '')
    
    print(f"\n{BLUE}═══════════════════════════════════════════════════════════{NC}")
    print(f"{CYAN}[{timestamp_str}] 收到Webhook请求{NC}")
    print(f"{BLUE}═══════════════════════════════════════════════════════════{NC}")
    
    # 2. 验证签名
    if WEBHOOK_SECRET and WEBHOOK_SECRET != "your-webhook-secret-key":
        body = request.get_data()
        if not verify_signature(body, signature, timestamp, nonce):
            print(f"{RED}✗ 签名验证失败{NC}")
            print(f"  Signature: {signature[:20]}...")
            print(f"  Timestamp: {timestamp}")
            print(f"  Nonce: {nonce}")
            return jsonify({"error": "invalid signature"}), 401
        print(f"{GREEN}✓ 签名验证通过{NC}")
    else:
        print(f"{YELLOW}⚠ 跳过签名验证（未配置密钥）{NC}")
    
    # 3. 解析事件
    try:
        event = request.get_json()
        if not event:
            print(f"{RED}✗ 无效的JSON数据{NC}")
            return jsonify({"error": "invalid json"}), 400
    except Exception as e:
        print(f"{RED}✗ JSON解析失败: {e}{NC}")
        return jsonify({"error": "json parse error"}), 400
    
    event_id = event.get("event_id", "")
    event_type = event.get("event_type", "")
    device_phy_id = event.get("device_phy_id", "")
    
    print(f"\n{YELLOW}📨 事件信息{NC}")
    print(f"  Event ID: {event_id}")
    print(f"  Event Type: {event_type}")
    print(f"  Device: {device_phy_id}")
    
    # 4. 幂等性检查
    if event_id in event_ids_seen:
        print(f"{YELLOW}⚠ 重复事件（已处理）{NC}")
        return jsonify({"status": "ok", "message": "duplicate event"}), 200
    
    event_ids_seen.add(event_id)
    
    # 5. 打印事件数据
    print(f"\n{CYAN}📄 事件数据:{NC}")
    print(json.dumps(event, indent=2, ensure_ascii=False))
    
    # 6. 处理不同类型的事件
    data = event.get("data", {})
    
    if event_type == "device.registered":
        print(f"\n{GREEN}✓ 设备注册事件{NC}")
        print(f"  ICCID: {data.get('iccid', 'N/A')}")
        print(f"  固件版本: {data.get('firmware', 'N/A')}")
        
    elif event_type == "device.heartbeat":
        print(f"\n{GREEN}✓ 设备心跳事件{NC}")
        print(f"  电压: {data.get('voltage', 'N/A')}V")
        print(f"  温度: {data.get('temp', 'N/A')}°C")
        ports = data.get('ports', [])
        if ports:
            print(f"  端口数: {len(ports)}")
            for port in ports:
                print(f"    端口{port.get('port_no')}: {port.get('state')} - {port.get('power', 0)}W")
                
    elif event_type == "order.created":
        print(f"\n{GREEN}✓ 订单创建事件{NC}")
        print(f"  订单号: {data.get('order_no', 'N/A')}")
        print(f"  端口: {data.get('port_no', 'N/A')}")
        print(f"  充电模式: {data.get('charge_mode', 'N/A')}")
        print(f"  时长: {data.get('duration', 'N/A')}秒")
        
    elif event_type == "charging.started":
        print(f"\n{GREEN}✓ 充电开始事件{NC}")
        print(f"  订单号: {data.get('order_no', 'N/A')}")
        print(f"  端口: {data.get('port_no', 'N/A')}")
        print(f"  开始时间: {data.get('start_time', 'N/A')}")
        
    elif event_type == "charging.progress":
        print(f"\n{CYAN}→ 充电进度事件{NC}")
        print(f"  订单号: {data.get('order_no', 'N/A')}")
        print(f"  时长: {data.get('duration_sec', 0)}秒")
        print(f"  电量: {data.get('total_kwh', 0)}度")
        print(f"  功率: {data.get('current_power', 0)}W")
        
    elif event_type == "order.completed":
        print(f"\n{GREEN}✓ 订单完成事件{NC}")
        print(f"  订单号: {data.get('order_no', 'N/A')}")
        print(f"  总时长: {data.get('duration_sec', 0)}秒")
        print(f"  总电量: {data.get('total_kwh', 0)}度")
        print(f"  总金额: {data.get('final_amount', 0)}分")
        print(f"  结束原因: {data.get('end_reason', 'N/A')}")
        
    elif event_type == "device.alarm":
        print(f"\n{RED}✗ 设备告警事件{NC}")
        print(f"  告警类型: {data.get('alarm_type', 'N/A')}")
        print(f"  端口: {data.get('port_no', 'N/A')}")
        print(f"  故障码: {data.get('fault_code', 'N/A')}")
        print(f"  故障信息: {data.get('fault_msg', 'N/A')}")
    
    # 7. 存储事件
    event['received_at'] = timestamp_str
    received_events.append(event)
    
    # 保持最近100个事件
    if len(received_events) > 100:
        received_events.pop(0)
    
    print(f"\n{GREEN}✓ 事件处理完成{NC}")
    print(f"  已接收事件总数: {len(received_events)}")
    
    # 8. 快速响应
    return jsonify({"status": "ok"}), 200


@app.route('/events', methods=['GET'])
def list_events():
    """查看已接收的事件列表"""
    return jsonify({
        "total": len(received_events),
        "events": received_events[-20:]  # 返回最近20个
    })


@app.route('/stats', methods=['GET'])
def stats():
    """统计信息"""
    event_types = {}
    for event in received_events:
        event_type = event.get('event_type', 'unknown')
        event_types[event_type] = event_types.get(event_type, 0) + 1
    
    return jsonify({
        "total_events": len(received_events),
        "unique_event_ids": len(event_ids_seen),
        "event_type_counts": event_types
    })


@app.route('/clear', methods=['POST'])
def clear_events():
    """清空事件历史"""
    global received_events, event_ids_seen
    received_events = []
    event_ids_seen = set()
    return jsonify({"status": "cleared"})


@app.route('/', methods=['GET'])
def index():
    """首页"""
    return f"""
    <html>
    <head><title>Webhook接收端</title></head>
    <body>
        <h1>IoT Webhook接收端</h1>
        <p>Webhook端点: <code>POST /webhook</code></p>
        <p>已接收事件: {len(received_events)}</p>
        <p>唯一事件ID: {len(event_ids_seen)}</p>
        <hr>
        <h2>API端点:</h2>
        <ul>
            <li><a href="/events">GET /events</a> - 查看事件列表</li>
            <li><a href="/stats">GET /stats</a> - 查看统计信息</li>
            <li>POST /clear - 清空事件历史</li>
        </ul>
    </body>
    </html>
    """


if __name__ == '__main__':
    import sys
    
    if len(sys.argv) > 1:
        PORT = int(sys.argv[1])
    
    print(f"{BLUE}═══════════════════════════════════════════════════════════{NC}")
    print(f"{CYAN}  IoT Webhook接收端{NC}")
    print(f"{BLUE}═══════════════════════════════════════════════════════════{NC}")
    print(f"\n{GREEN}✓ 服务启动{NC}")
    print(f"  监听端口: {PORT}")
    print(f"  Webhook URL: http://0.0.0.0:{PORT}/webhook")
    print(f"  管理页面: http://0.0.0.0:{PORT}/")
    print(f"\n{YELLOW}配置提示:{NC}")
    print(f"  请在IoT服务器配置文件中设置:")
    print(f"  webhook_url: \"http://YOUR_IP:{PORT}/webhook\"")
    print(f"  secret: \"{WEBHOOK_SECRET}\"")
    print(f"\n{CYAN}按 Ctrl+C 停止服务{NC}\n")
    
    app.run(host='0.0.0.0', port=PORT, debug=False)

