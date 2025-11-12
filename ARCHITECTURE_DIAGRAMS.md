# IoT Charging Pile Server - Architecture Diagrams

## 1. Complete System Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           EXTERNAL SYSTEMS                                  │
├─────────────────────────────────────────────────────────────────────────────┤
│  Charging Devices (TCP)    │    3rd-party APIs (HTTP)    │  Webhook URLs    │
│  (AP3000/BKV protocols)    │    (REST endpoints)         │  (async push)     │
└────────────┬───────────────────────┬──────────────────────────┬─────────────┘
             │                       │                          │
             ↓                       ↓                          ↓
┌────────────────────────┐  ┌──────────────────┐  ┌──────────────────────┐
│   TCP Gateway Layer    │  │   HTTP Server    │  │  Webhook Receivers   │
│  (tcpserver/)          │  │   (Gin Router)   │  │  (3rd-party systems) │
├────────────────────────┤  ├──────────────────┤  ├──────────────────────┤
│ • TCP Listener         │  │ • API Routes     │  │ • Event delivery     │
│ • Connection mgmt      │  │ • Auth middleware│  │ • Signature verify   │
│ • Rate limiting        │  │ • Response fmt   │  │ • Webhook retry      │
└────────┬───────────────┘  └────────┬─────────┘  └──────────────────────┘
         │                           │
         ↓                           ↓
┌─────────────────────────────────────────────────────────────────────────────┐
│                      PROTOCOL LAYER                                         │
├─────────────────────────────────────────────────────────────────────────────┤
│  AP3000 Protocol            │         BKV Protocol                          │
│  (protocol/ap3000/)         │         (protocol/bkv/)                       │
├─────────────────────────────┼──────────────────────────────────────────────┤
│ • Stream Decoder            │ • Frame Parser (CRC16)                        │
│ • Handler Registry          │ • Command Router (25+ cmds)                   │
│ • Frame Routing             │ • Dual encoding (binary+BCD)                  │
│ • Magic byte detection      │ • Parameter management                        │
│                             │ • Card swipe processing                       │
│ Commands: 0x20, 0x21, etc   │ • OTA upgrade mgmt                            │
│                             │ • Network management                          │
│                             │ Commands: 0x0000-0x1017                      │
└────────┬───────────────────┴──────────────────────┬──────────────────────┘
         │                                          │
         └──────────────────┬───────────────────────┘
                            ↓
┌──────────────────────────────────────────────────────────────────────────────┐
│                      BUSINESS LOGIC LAYER                                    │
├──────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌──────────────────────┐    ┌──────────────────────┐    ┌───────────────┐ │
│  │   Session Manager    │    │    Repository        │    │ Card Service  │ │
│  │  (session/)          │    │   (storage/pg/)      │    │ (service/)    │ │
│  ├──────────────────────┤    ├──────────────────────┤    ├───────────────┤ │
│  │ • Bind connection    │    │ • EnsureDevice()     │    │ • Validate    │ │
│  │ • Heartbeat tracking │    │ • UpsertPortState()  │    │   card/balance│ │
│  │ • Online detection   │    │ • UpsertOrder()      │    │ • Create order│ │
│  │ • Connection pool    │    │ • SettleOrder()      │    │ • Price calc  │ │
│  │ • Weighted policy    │    │ • InsertCmdLog()     │    │ • Event gen   │ │
│  └──────────────────────┘    └──────────────────────┘    └───────────────┘ │
│                                                                              │
│  ┌──────────────────────┐    ┌──────────────────────┐                      │
│  │ Outbound Adapter     │    │  Event Queue         │                      │
│  │ (app/outbound_...)   │    │  (thirdparty/)       │                      │
│  ├──────────────────────┤    ├──────────────────────┤                      │
│  │ • Build BKV frames   │    │ • Enqueue event      │                      │
│  │ • Priority dispatch  │    │ • Event types        │                      │
│  │ • Retry logic        │    │ • Deduplication      │                      │
│  │ • Queue to Redis     │    │ • Webhook delivery   │                      │
│  └──────────────────────┘    └──────────────────────┘                      │
│                                                                              │
└──────┬───────────────────────────────────────────────────────┬──────────────┘
       │                                                        │
       │                                    ┌───────────────────┘
       │                                    │
       ↓                                    ↓
┌──────────────────────────┐      ┌─────────────────────────┐
│   Redis Outbound Queue   │      │  Redis Event Queue      │
│   (storage/redis/)       │      │  (thirdparty/events)    │
├──────────────────────────┤      ├─────────────────────────┤
│ Type: Sorted Set         │      │ Type: List              │
│ Key: outbound:queue      │      │ Key: event:queue        │
│ Score: priority + time   │      │ Size: dynamic           │
│ States: pending, process,│      │ Workers: N (configurable)│
│         success, dead    │      │ Retry: exponential      │
└──────┬───────────────────┘      └────────┬────────────────┘
       │                                   │
       ↓                                   ↓
┌──────────────────────────┐      ┌─────────────────────────┐
│   Redis Worker (1)       │      │  Event Workers (N)      │
│   (outbound/worker.go)   │      │  (thirdparty)           │
├──────────────────────────┤      ├─────────────────────────┤
│ • Dequeue by priority    │      │ • Dequeue event         │
│ • Get device connection  │      │ • Check duplicate       │
│ • Write to TCP socket    │      │ • HTTP POST webhook     │
│ • Mark success/failure   │      │ • Retry on failure      │
│ • Throttle: 100ms loop   │      │ • Move to DLQ           │
└──────┬───────────────────┘      └─────────────────────────┘
       │
       ↓
    ┌─────────────────────┐
    │  Device TCP Socket  │
    │  (connected client) │
    └─────────────────────┘
            ↓
    Device processes command
    and responds
```

## 2. Inbound Message Flow (TCP → Database)

```
Device connects to TCP:port
    ├─ TCP.Accept() → new net.Conn
    ├─ ConnContext created (id, read/write buffers)
    └─ Mux.BindToConn() → install onRead handler
         │
         ├─ Sniff first 8 bytes
         │   ├─ prefix[0:3] == "DYN"? → AP3000
         │   ├─ else parse as BKV frame? → BKV
         │   └─ else unknown (try all)
         │
         └─ RestoreNormalTimeout() (5s → 300s)
             │
             ↓
Device sends Frame 1: 0x0000 (Heartbeat)
    │
    ├─ ConnContext.onRead(bytes)
    ├─ BKVAdapter.ProcessBytes()
    │   ├─ Parse: cmd=0x0000, msgID=123, gatewayID="GW001", data=[...]
    │   └─ Call registered handler
    │
    ├─ wrapBKVHandler wrapper
    │   ├─ Log frame (protocol, cmd, size)
    │   ├─ Metrics: BKVRouteTotal.Inc("0000")
    │   ├─ Session.Bind("GW001", ConnContext)
    │   ├─ Session.OnHeartbeat("GW001", now)
    │   └─ Call actual handler: HandleHeartbeat
    │
    ├─ Handler: HandleHeartbeat(ctx, frame)
    │   ├─ Session.OnHeartbeat("GW001") → Update Redis lastSeen
    │   ├─ Repo.TouchDeviceLastSeen("GW001", time.Now())
    │   ├─ Repo.EnsureDevice("GW001") → Get/create device ID
    │   ├─ Metrics collection
    │   │   ├─ HeartbeatTotal++
    │   │   ├─ OnlineGauge = sess.OnlineCountWeighted(policy)
    │   │   └─ ProtocolChecksumError (if failed)
    │   │
    │   └─ Return status (success/error)
    │
    └─ Queue response in outbound worker
        (async, not synchronous)

Database State Updated:
  devices.last_seen_at = now
  session:device:GW001 = {LastSeen: now, ConnID: ..., ServerID: ...}
  metrics.heartbeat_total++
```

## 3. Outbound Message Flow (API → Device)

```
Third-party calls HTTP API:
  POST /thirdparty/devices/123/start_charge
  Authorization: X-Api-Key: secret123
  Body: {port_no: 1, duration: 60, amount: 1000}
    │
    ├─ Middleware.Auth() → Verify API key
    ├─ Gin.Bind() → Parse JSON
    │
    └─ ThirdPartyHandler.StartCharge(ctx, req)
         │
         ├─ 1. Validation
         │  ├─ Repo.GetDevice(123) → Check exists
         │  ├─ Repo.GetPort(123, 1) → Check online
         │  └─ Error if not online → HTTP 503
         │
         ├─ 2. Order Creation
         │  ├─ OrderNo = GenerateOrderNo(cardNo, phyID)
         │  ├─ Repo.UpsertOrderProgress(
         │  │    deviceID=123, portNo=1,
         │  │    orderNo="ORD_XXX", status=PENDING,
         │  │    kwh=0, durationSec=3600)
         │  └─ DB updated: orders table
         │
         ├─ 3. Queue Outbound Command
         │  └─ OutboundAdapter.SendDownlink(
         │       gatewayID="GW001",
         │       cmd=0x0015 (control),
         │       msgID=next++,
         │       data=[charge params])
         │      │
         │      ├─ Build BKV Frame
         │      ├─ Repo.EnsureDevice() → Get device ID
         │      │
         │      └─ OutboundQueue.Enqueue({
         │           ID: "bkv_0x0015_1731234567890",
         │           DeviceID: 123,
         │           PhyID: "GW001",
         │           Command: [BKV encoded bytes],
         │           Priority: 2 (normal),
         │           MaxRetry: 1,
         │           Timeout: 3000ms,
         │           CreatedAt: now
         │         })
         │         Redis: ZADD outbound:queue 2000000000000 JSON
         │
         ├─ 4. Queue Event
         │  ├─ StandardEvent{
         │  │    EventID: uuid,
         │  │    EventType: "OrderCreated",
         │  │    DevicePhyID: "GW001",
         │  │    OrderNo: "ORD_XXX",
         │  │    Timestamp: unixMS,
         │  │    Data: {...charging params...}
         │  │  }
         │  └─ EventQueue.Enqueue(event)
         │     Redis: RPUSH thirdparty:event:queue JSON
         │
         └─ 5. Return Success
            HTTP 202 Accepted {
              code: 0,
              request_id: "req_123",
              data: {order_no: "ORD_XXX"}
            }

=== ASYNC PROCESSING (Background) ===

Redis Worker Loop (every 100ms):
    │
    ├─ QueueLength := ZCARD outbound:queue
    ├─ if length > 1000 → reject low-priority (only accept urgent)
    │
    ├─ msg := ZPOPMIN(outbound:queue, 1)
    │  → Get: {ID, PhyID, Command, Priority, ...}
    │
    ├─ HSET outbound:processing:123 ID msg (mark processing)
    │
    ├─ conn := Session.GetConn("GW001")
    │  if !ok → mark failed, move to DLQ
    │  (checks: connection exists, heartbeat recent)
    │
    ├─ n, err := conn.Write(msg.Command)
    │  (writes to device TCP socket)
    │
    ├─ if success:
    │  └─ HDEL outbound:processing:123 ID
    │     (removed from processing)
    │
    └─ if failed:
       ├─ Increment retry count
       ├─ if retries < MaxRetry:
       │  └─ Re-ZADD to queue with delayed score
       └─ if exhausted:
          └─ RPUSH outbound:dead msg (dead letter)

Event Worker Loop (parallel):
    │
    ├─ event := LPOP thirdparty:event:queue
    │
    ├─ if Deduper.IsDuplicate(eventID):
    │  └─ Continue (skip)
    │
    ├─ Signature := HMAC-SHA256(JSON, secret)
    │
    ├─ err := HTTP.POST(webhookURL, {
    │       headers: {X-Signature: Signature},
    │       body: JSON(event)
    │     })
    │
    ├─ if success (2xx):
    │  └─ Continue (event processed)
    │
    └─ if failure:
       ├─ INCR thirdparty:event:retry:{eventID}
       ├─ retries := GET thirdparty:event:retry:{eventID}
       ├─ if retries < 5:
       │  └─ Sleep(exponential backoff) + re-enqueue
       └─ if exhausted:
          └─ RPUSH thirdparty:event:dlq event (dead letter)

Device receives start_charge command
    └─ Processes locally (enables charger)
       Sends status update frames...

Server receives status updates
    └─ Handlers process as inbound flow...
```

## 4. Session Management (Redis)

```
SESSION STATE IN REDIS:

Device GW001 connects to Server A:
    │
    ├─ Session.Bind("GW001", ConnContext)
    │   ├─ connID := uuid.New() → "abc123"
    │   │
    │   └─ SET session:device:GW001 {
    │        phy_id: "GW001",
    │        conn_id: "abc123",
    │        server_id: "srv-A",
    │        last_seen: 2024-11-12T10:30:00Z,
    │        last_tcp_down: null,
    │        last_ack_timeout: null
    │      } EX 450s (1.5×5min timeout)
    │
    ├─ SET session:conn:abc123 "GW001" EX 450s
    │
    └─ SADD session:server:srv-A:conns "abc123"

Periodic heartbeats from device:
    │
    └─ Session.OnHeartbeat("GW001", now)
       └─ GET session:device:GW001
          SET updated.last_seen = now
          SET updated.updated_at = now
          SETEX session:device:GW001 450s JSON(updated)

TCP Connection drops (network failure):
    │
    ├─ Session.OnTCPClosed("GW001", now)
    │  └─ GET session:device:GW001
    │     SET updated.last_tcp_down = now
    │     SETEX session:device:GW001 450s JSON(updated)
    │
    └─ Session.UnbindByPhy("GW001")
       └─ DEL session:conn:abc123
          SREM session:server:srv-A:conns abc123

Check if online:
    │
    ├─ IsOnlineWeighted("GW001", now, WeightedPolicy)
    │   │
    │   ├─ GET session:device:GW001 → {LastSeen, LastTCPDown, ...}
    │   │
    │   ├─ if (now - LastSeen) > policy.HeartbeatTimeout
    │   │  └─ return false (heartbeat too old)
    │   │
    │   ├─ score := 100.0
    │   │
    │   ├─ if (now - LastTCPDown) < policy.TCPDownWindow
    │   │  └─ score -= policy.TCPDownPenalty (e.g., 30)
    │   │
    │   ├─ if (now - LastAckTimeout) < policy.AckWindow
    │   │  └─ score -= policy.AckTimeoutPenalty (e.g., 20)
    │   │
    │   └─ return score >= policy.Threshold (e.g., 50)
    │
    └─ return bool (true = online, false = offline)

Get connection for writing:
    │
    └─ GetConn("GW001") → (ConnContext, ok)
       │
       ├─ GET session:device:GW001
       ├─ connID := session.ConnID
       ├─ Check local cache: m.localConn[connID]
       └─ if exists and online → return it

Multi-instance scenario:
    │
    ├─ Server A: conn to GW001
    │  SET session:device:GW001 {server_id: "srv-A", ...}
    │
    ├─ Server B: wants to send command
    │  GET session:device:GW001 → {server_id: "srv-A"}
    │  Enqueue in Redis → outbound:queue
    │
    └─ Server A's worker (owns the connection)
       Dequeue from Redis
       Session.GetConn("GW001") → finds it (local cache)
       Writes to TCP socket
```

## 5. Protocol Handler Dispatch Tree

```
TCP Packet received
    │
    ├─ Mux.BindToConn() → sniff first bytes
    │   │
    │   ├─ AP3000 protocol detected (prefix = "DYN")
    │   │  │
    │   │  └─ AP3000Adapter.ProcessBytes()
    │   │     ├─ StreamDecoder.Feed() → handle partial frames
    │   │     └─ For each complete frame:
    │   │        │
    │   │        └─ Router.Route(frame)
    │   │           ├─ cmd = frame.Cmd (0x20, 0x21, etc)
    │   │           │
    │   │           ├─ if cmd == 0x20:
    │   │           │  └─ RegisteredHandler(frame)
    │   │           │     └─ AP3000Handlers.HandleRegister()
    │   │           │
    │   │           ├─ if cmd == 0x21:
    │   │           │  └─ AP3000Handlers.HandleHeartbeat()
    │   │           │
    │   │           └─ if cmd == 0x22:
    │   │              └─ AP3000Handlers.HandleGeneric()
    │   │
    │   └─ BKV protocol detected (parse successfully)
    │      │
    │      └─ BKVAdapter.ProcessBytes()
    │         ├─ Parser.Parse(bytes) → Frame
    │         │  ├─ Extract: header, length, payload, checksum
    │         │  ├─ Validate CRC16
    │         │  └─ Reassemble if fragmented
    │         │
    │         └─ Router.Route(frame)
    │            ├─ cmd = frame.Cmd (uint16)
    │            │
    │            ├─ wrapBKVHandler() wrapper
    │            │  ├─ Log frame details
    │            │  ├─ Metrics.Inc()
    │            │  ├─ Session.Bind()
    │            │  └─ Call actual handler
    │            │
    │            ├─ if cmd == 0x0000:
    │            │  └─ BKVHandlers.HandleHeartbeat()
    │            │
    │            ├─ if cmd == 0x000B:
    │            │  └─ BKVHandlers.HandleCardSwipe()
    │            │
    │            ├─ if cmd == 0x000C:
    │            │  └─ BKVHandlers.HandleChargeEnd()
    │            │
    │            ├─ if cmd == 0x000F:
    │            │  └─ BKVHandlers.HandleOrderConfirm()
    │            │
    │            ├─ if cmd == 0x0015:
    │            │  └─ BKVHandlers.HandleControl()
    │            │
    │            ├─ ... (20+ more commands)
    │            │
    │            └─ if cmd == unknown:
    │               └─ BKVHandlers.HandleGeneric()
    │
    └─ Handler execution
       ├─ Validate input
       ├─ Query DB (Repository)
       ├─ Apply business logic
       ├─ Update DB
       ├─ Queue events
       ├─ Queue outbound replies
       └─ Return status
```

## 6. Startup Dependency Graph

```
main.go
  │
  ├─ Config.Load() ──────────────────────┐
  │                                      │
  └─ bootstrap.Run(cfg, logger)          │
                                         │
     Phase 1: Init Infrastructure        │
     ├─ Metrics.NewRegistry()            │
     ├─ Ready flags                      │
     └─ Logging setup ◄────────────────┘
           │
           └─→ Phase 2: Redis Client ⟵────── Required
                ├─ NewClient(cfg.Redis)
                ├─ Ping()
                └─ Error → Fatal exit
                     │
                     └─→ Phase 3: Session Manager
                          ├─ ServerID = GenerateServerID()
                          ├─ NewRedisManager(redis, serverID)
                          └─ NewWeightedPolicy()
                               │
                               └─→ Phase 4: PostgreSQL ⟵────── Required
                                    ├─ pgxpool.New()
                                    ├─ RunMigrations()
                                    └─ Error → Fatal exit
                                         │
                                         └─→ Phase 5: Business Setup
                                              ├─ LoadProtocolHandlers(AP3000, BKV)
                                              ├─ NewCardService()
                                              ├─ NewOutboundAdapter()
                                              └─ NewEventQueue()
                                                   │
                                                   └─→ Phase 6: HTTP Server
                                                        ├─ RegisterRoutes()
                                                        ├─ Start() [non-blocking]
                                                        └─ Health checks
                                                             │
                                                             └─→ Phase 7: Workers
                                                                  ├─ RedisWorker.Start()
                                                                  ├─ EventWorker.Start()
                                                                  ├─ OrderMonitor.Start()
                                                                  └─ PortStatusSyncer.Start()
                                                                       │
                                                                       └─→ Phase 8: TCP Server
                                                                            ├─ SetConnHandler()
                                                                            ├─ Start() [blocking]
                                                                            └─ Ready=true
                                                                                 │
                                                                                 └─→ Phase 9: Shutdown
                                                                                      ├─ SIGINT/SIGTERM
                                                                                      ├─ HTTP.Shutdown()
                                                                                      ├─ TCP.Shutdown()
                                                                                      └─ Cleanup

KEY INSIGHT: TCP server starts LAST to ensure all dependencies ready first!
             If TCP started early, it could accept connections before handlers exist.
```

## 7. Multi-Protocol Connection Decision Flow

```
TCP Connection receives first packet with bytes [0x46, 0x47, ...]
    │
    └─ Sniff(prefix[0:8])
       │
       ├─ AP3000Adapter.Sniff(prefix)
       │  │
       │  └─ Check: prefix[0:3] == [0x44, 0x59, 0x4E] ("DYN")
       │     ├─ true → MATCH → "ap3000"
       │     └─ false → continue
       │
       ├─ BKVAdapter.Sniff(prefix)
       │  │
       │  └─ Parse as BKV frame header
       │     ├─ Try to extract length field
       │     ├─ Validate structure
       │     └─ If parseable → MATCH → "bkv"
       │
       └─ No match → try all adapters fallback
          (for robustness - may match on second frame)

Selected adapter → Install its ProcessBytes() as handler
      │
      └─ All future packets → adapter.ProcessBytes()
         (no re-sniffing, protocol locked)

ConnContext.Protocol() → "ap3000" or "bkv" or ""
```

## 8. Error Handling & Recovery Flows

```
Outbound Message Delivery:

Send → Success ✓
   │
   └─ Mark completed

Send → Failure (device offline, write error, timeout)
   │
   ├─ Get current retries count
   │
   ├─ if retries < maxRetries (default: 1)
   │  ├─ Increment retry counter
   │  ├─ Add exponential backoff delay
   │  └─ Re-enqueue to outbound:queue
   │     (will be retried in next cycle)
   │
   └─ if retries >= maxRetries
      ├─ Log error
      └─ Move to outbound:dead (dead letter queue)
         (operator manual review required)

Webhook Event Delivery:

Push → 2xx Success ✓
   │
   └─ Continue (event processed)

Push → 4xx/5xx Error
   │
   ├─ Get retry count from thirdparty:event:retry:{eventID}
   │
   ├─ if retries < 5
   │  ├─ Increment counter
   │  ├─ Backoff: 1s, 2s, 4s, 8s (exponential)
   │  └─ Re-enqueue to queue
   │
   └─ if retries >= 5
      ├─ Log to DLQ
      └─ Move to thirdparty:event:dlq
         (operator can retry manually)

Connection Loss Recovery:

Device disconnects
   │
   ├─ Session.OnTCPClosed(phyID, now)
   │  └─ Mark last_tcp_down timestamp
   │
   └─ Subsequent online check:
      │
      ├─ if (now - lastTCPDown) < 1 minute:
      │  └─ Still considered online (weighted policy)
      │     (commands queued, awaits reconnection)
      │
      └─ if (now - lastTCPDown) > 5 minutes:
         └─ Offline (commands fail)

Database Transaction Failure:

Insert order → CONFLICT
   │
   └─ ON CONFLICT DO UPDATE
      (idempotent - update if exists)

Write parameter → Conflict
   │
   └─ Stored in parameter:pending with TTL
      (will be retried on next device contact)
```

---

## Summary

- **Layered design**: Gateway → Protocol → Business Logic → Persistence
- **Async-first**: Outbound and events non-blocking
- **Distributed-ready**: Redis sessions support multi-instance
- **Resilient**: Circuit breaker, rate limiting, dead-letter queues
- **Observable**: Metrics, logs, health checks on all components
- **Protocol-agnostic**: New protocols added via Adapter interface
- **Interface-driven**: SessionManager, Repository for testability

