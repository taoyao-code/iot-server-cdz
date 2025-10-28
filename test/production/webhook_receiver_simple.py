#!/usr/bin/env python3
# -*- coding: utf-8 -*-

"""
Simple Webhook Receiver
Purpose: Receive and log webhook events from IoT server
Usage: python3 webhook_receiver_simple.py [port]
"""

import hmac
import hashlib
import json
import time
from datetime import datetime
from flask import Flask, request, jsonify

app = Flask(__name__)

# Configuration
WEBHOOK_SECRET = "4e231cd1408070cc52525f675c2f6c053afe4afdef5b813acc9f7b74e1fd3c34"
PORT = 8989

# Event storage
received_events = []
event_ids_seen = set()


def verify_signature(body, signature, timestamp, nonce):
    """Verify HMAC-SHA256 signature"""
    try:
        # Calculate body SHA256
        body_sha256 = hashlib.sha256(body).hexdigest()
        
        # Construct canonical string
        method = request.method
        path = request.path
        canonical = f"{method}\n{path}\n{timestamp}\n{nonce}\n{body_sha256}"
        
        # Calculate HMAC-SHA256
        expected_sig = hmac.new(
            WEBHOOK_SECRET.encode(),
            canonical.encode(),
            hashlib.sha256
        ).hexdigest()
        
        # Compare signatures
        return hmac.compare_digest(signature, expected_sig)
    except Exception as e:
        print(f"[ERROR] Signature verification failed: {e}")
        return False


@app.route('/webhook', methods=['POST'])
def handle_webhook():
    """Handle webhook requests"""
    timestamp_str = datetime.now().strftime('%Y-%m-%d %H:%M:%S')
    
    # Get headers
    signature = request.headers.get('X-Signature', '')
    timestamp = request.headers.get('X-Timestamp', '')
    nonce = request.headers.get('X-Nonce', '')
    
    print("\n" + "="*60)
    print(f"[{timestamp_str}] Received Webhook Request")
    print("="*60)
    
    # Verify signature
    if WEBHOOK_SECRET and WEBHOOK_SECRET != "your-webhook-secret-key":
        body = request.get_data()
        if not verify_signature(body, signature, timestamp, nonce):
            print(f"[ERROR] Signature verification failed")
            return jsonify({"error": "invalid signature"}), 401
        print(f"[OK] Signature verified")
    else:
        print(f"[WARN] Skipped signature verification (no secret configured)")
    
    # Parse event
    try:
        event = request.get_json()
        if not event:
            print(f"[ERROR] Invalid JSON data")
            return jsonify({"error": "invalid json"}), 400
    except Exception as e:
        print(f"[ERROR] JSON parse error: {e}")
        return jsonify({"error": "json parse error"}), 400
    
    event_id = event.get("event_id", "")
    event_type = event.get("event_type", "")
    device_phy_id = event.get("device_phy_id", "")
    
    print(f"\n[EVENT INFO]")
    print(f"  Event ID: {event_id}")
    print(f"  Event Type: {event_type}")
    print(f"  Device: {device_phy_id}")
    
    # Check for duplicates
    if event_id in event_ids_seen:
        print(f"[WARN] Duplicate event (already processed)")
        return jsonify({"status": "ok", "message": "duplicate event"}), 200
    
    event_ids_seen.add(event_id)
    
    # Print event data
    print(f"\n[EVENT DATA]:")
    print(json.dumps(event, indent=2, ensure_ascii=False))
    
    # Handle different event types
    data = event.get("data", {})
    
    if event_type == "device.registered":
        print(f"\n[OK] Device Registration Event")
        print(f"  ICCID: {data.get('iccid', 'N/A')}")
        print(f"  Firmware: {data.get('firmware', 'N/A')}")
        
    elif event_type == "device.heartbeat":
        print(f"\n[OK] Device Heartbeat Event")
        print(f"  Voltage: {data.get('voltage', 'N/A')}V")
        print(f"  Temp: {data.get('temp', 'N/A')}C")
        ports = data.get('ports', [])
        if ports:
            print(f"  Ports: {len(ports)}")
            for port in ports:
                print(f"    Port{port.get('port_no')}: {port.get('state')} - {port.get('power', 0)}W")
                
    elif event_type == "order.created":
        print(f"\n[OK] Order Created Event")
        print(f"  Order No: {data.get('order_no', 'N/A')}")
        print(f"  Port: {data.get('port_no', 'N/A')}")
        print(f"  Mode: {data.get('charge_mode', 'N/A')}")
        print(f"  Duration: {data.get('duration', 'N/A')}s")
        
    elif event_type == "charging.started":
        print(f"\n[OK] Charging Started Event")
        print(f"  Order No: {data.get('order_no', 'N/A')}")
        print(f"  Port: {data.get('port_no', 'N/A')}")
        print(f"  Start Time: {data.get('start_time', 'N/A')}")
        
    elif event_type == "charging.progress":
        print(f"\n[INFO] Charging Progress Event")
        print(f"  Order No: {data.get('order_no', 'N/A')}")
        print(f"  Duration: {data.get('duration_sec', 0)}s")
        print(f"  Energy: {data.get('total_kwh', 0)}kWh")
        print(f"  Power: {data.get('current_power', 0)}W")
        
    elif event_type == "order.completed":
        print(f"\n[OK] Order Completed Event")
        print(f"  Order No: {data.get('order_no', 'N/A')}")
        print(f"  Total Duration: {data.get('duration_sec', 0)}s")
        print(f"  Total Energy: {data.get('total_kwh', 0)}kWh")
        print(f"  Total Amount: {data.get('final_amount', 0)} cents")
        print(f"  End Reason: {data.get('end_reason', 'N/A')}")
        
    elif event_type == "device.alarm":
        print(f"\n[ERROR] Device Alarm Event")
        print(f"  Alarm Type: {data.get('alarm_type', 'N/A')}")
        print(f"  Port: {data.get('port_no', 'N/A')}")
        print(f"  Fault Code: {data.get('fault_code', 'N/A')}")
        print(f"  Fault Msg: {data.get('fault_msg', 'N/A')}")
    
    # Store event
    event['received_at'] = timestamp_str
    received_events.append(event)
    
    # Keep recent 100 events only
    if len(received_events) > 100:
        received_events.pop(0)
    
    print(f"\n[OK] Event processed successfully")
    print(f"  Total events received: {len(received_events)}")
    
    # Quick response
    return jsonify({"status": "ok"}), 200


@app.route('/events', methods=['GET'])
def list_events():
    """View received events"""
    return jsonify({
        "total": len(received_events),
        "events": received_events[-20:]
    })


@app.route('/stats', methods=['GET'])
def stats():
    """Statistics"""
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
    """Clear event history"""
    global received_events, event_ids_seen
    received_events = []
    event_ids_seen = set()
    return jsonify({"status": "cleared"})


@app.route('/', methods=['GET'])
def index():
    """Homepage"""
    return f"""
    <html>
    <head><title>Webhook Receiver</title></head>
    <body>
        <h1>IoT Webhook Receiver</h1>
        <p>Webhook endpoint: <code>POST /webhook</code></p>
        <p>Events received: {len(received_events)}</p>
        <p>Unique event IDs: {len(event_ids_seen)}</p>
        <hr>
        <h2>API Endpoints:</h2>
        <ul>
            <li><a href="/events">GET /events</a> - View events</li>
            <li><a href="/stats">GET /stats</a> - View statistics</li>
            <li>POST /clear - Clear event history</li>
        </ul>
    </body>
    </html>
    """


if __name__ == '__main__':
    import sys
    
    if len(sys.argv) > 1:
        PORT = int(sys.argv[1])
    
    print("="*60)
    print("  IoT Webhook Receiver")
    print("="*60)
    print(f"\n[OK] Service started")
    print(f"  Listening on port: {PORT}")
    print(f"  Webhook URL: http://0.0.0.0:{PORT}/webhook")
    print(f"  Management page: http://0.0.0.0:{PORT}/")
    print(f"\n[CONFIG]")
    print(f"  Set in IoT server config:")
    print(f"  webhook_url: \"http://YOUR_IP:{PORT}/webhook\"")
    print(f"  secret: \"{WEBHOOK_SECRET}\"")
    print(f"\nPress Ctrl+C to stop\n")
    
    app.run(host='0.0.0.0', port=PORT, debug=False)

