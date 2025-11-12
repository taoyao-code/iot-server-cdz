# IoT Charging Pile Server - Architecture Quick Reference

## High-Level View

```
TCP Devices → TCP Gateway (protocol mux) → Protocol Handlers (AP3000/BKV)
                                                ↓
                                    Session Manager (Redis)
                                                ↓
                        ┌───────────────────────┴────────────────┐
                        ↓                                         ↓
                  Business Logic                         Outbound Worker
                (order, charging, events)               (Redis queue)
                        ↓                                         ↓
                   Database Layer                          Device Write
            (PostgreSQL + Redis)                        (via connection)
                        
HTTP APIs → Repository → Database
         → Outbound Queue → Worker
         → Event Queue → Webhooks
```

## Component Relationships

| Component | Role | Key Dependencies |
|-----------|------|------------------|
| **TCPServer** | Listen, accept connections | - |
| **Mux** | Detect protocol (AP3000/BKV) | Adapters |
| **AP3000/BKV Adapters** | Parse frames, route commands | handlers |
| **Handlers** | Business logic | repo, session, outbound, events |
| **SessionManager** | Track online status | Redis |
| **Repository** | Data access (device, port, order) | PostgreSQL |
| **OutboundQueue** | Queue commands to devices | Redis |
| **RedisWorker** | Send queued commands | session, outbound queue |
| **EventQueue** | Queue webhook events | Redis |
| **Pusher** | Send webhooks to 3rd party | HTTP |
| **HTTP APIs** | REST endpoints | repo, session, outbound |

## Data Flow Examples

### Inbound: Device Heartbeat
```
TCP Packet → Adapter.Sniff() → Protocol detected
    ↓
Adapter.ProcessBytes() → Parse frame
    ↓
Handler (route by cmd) → Handle heartbeat
    ↓
Session.OnHeartbeat() → Update Redis
    ↓
Repo.TouchDeviceLastSeen() → Update PostgreSQL
```

### Outbound: Start Charging (via 3rd-party API)
```
HTTP POST /start_charge
    ↓
Middleware.Auth() → Validate API key
    ↓
Handler.StartCharge()
    ├─ Repo.UpsertOrderProgress() → Update DB
    ├─ OutboundQueue.Enqueue() → Queue command
    └─ EventQueue.Enqueue() → Queue event
    ↓
Response 202 Accepted (async)
    ↓
RedisWorker (background)
    ├─ Dequeue command
    ├─ Session.GetConn() → Get device TCP connection
    └─ Conn.Write() → Send to device
    ↓
EventWorker (background)
    ├─ Dequeue event
    ├─ Deduper.Check() → Skip duplicate
    └─ Pusher.POST() → Send webhook

Device receives → processes locally
    ↓
Device sends status update
    ↓
Server receives → Handler updates DB
```

## Key Design Patterns

1. **Adapter Pattern** - Multiple protocols detected by first byte
2. **Repository Pattern** - All DB access via interface
3. **Session Manager Interface** - Redis or in-memory backend
4. **Handler Registration** - Command dispatch via closure map
5. **Async Queuing** - Outbound & event messaging non-blocking
6. **Graceful Degradation** - Queue throttling, rate limiting, circuit breaker

## Startup Sequence (9 Phases)

1. Metrics + ready flags
2. Redis client (required)
3. Session manager (Redis-backed)
4. PostgreSQL + migrations
5. Protocol handlers (AP3000, BKV)
6. HTTP server
7. Redis outbound worker
8. Event workers
9. TCP server (LAST - all deps ready)

**Critical**: TCP server starts after all other services to avoid accepting connections before handlers exist.

## Redis Data Structures

```
Session:
  session:device:{phyID} → {LastSeen, ConnID, ServerID}
  session:conn:{connID} → phyID
  session:server:{serverID}:conns → Set[connID]

Outbound Queue:
  outbound:queue → Sorted Set (by priority + time)
  outbound:processing:{devId} → current message
  outbound:dead → failed messages

Event Queue:
  thirdparty:event:queue → List
  thirdparty:event:dlq → dead events
  thirdparty:event:retry:{id} → retry counter
```

## PostgreSQL Tables (Minimal)

```
devices (id, phy_id, last_seen_at)
ports (device_id, port_no, status, power_w)
orders (order_no, device_id, port_no, start_time, end_time, kwh, status)
cmd_log (device_id, msg_id, cmd, direction, payload)
gateway_sockets (gateway_id, socket_no, socket_type, ...)
ota_tasks (id, device_id, target_version, status, progress)
cards (card_no, balance, status)
card_transactions (order_no, card_no, status, energy, amount)
```

All operations use `INSERT ... ON CONFLICT DO UPDATE` for idempotency.

## API Endpoints

**Public (Read-only)**:
- `GET /devices/{id}` - Device info
- `GET /devices/{id}/ports` - Port statuses
- `GET /orders/{order_no}` - Order status

**Third-party (Authenticated)**:
- `POST /devices/{id}/start_charge` - Start charging
- `POST /devices/{id}/stop_charge` - Stop charging
- `POST /devices/{id}/reboot` - Reboot device
- `GET /devices/{id}/online` - Check if online

**Monitoring**:
- `GET /health` - Health check
- `GET /metrics` - Prometheus metrics

## Protocol Commands (BKV Example)

```
0x0000 - Heartbeat
0x0015 - Device control (start/stop/pause)
0x000B - Card swipe
0x000C - Charge end
0x000F - Order confirm
0x000D - Socket state query
0x001D - Socket state response
0x0007 - OTA upgrade
0x0001-0004 - Parameter management
... 20+ more
```

## Multi-Instance Deployment

With Redis session manager:
1. Each server instance has unique ServerID
2. Device connections to any server tracked in Redis
3. Commands queued in shared Redis
4. Owner server's worker processes queue
5. Stateless API calls to any server

```
Client A → Server 1 → starts charge → outbound queue (Redis)
                                          ↓
Client B → Server 2 → query status → reads from cache (Redis)
                                          ↓
Device → Server 3 → receives command → processes

All via shared Redis + PostgreSQL
```

## Error Handling & Resilience

- **Circuit Breaker** - Auto-reject if failure rate too high
- **Rate Limiter** - Throttle connections/requests
- **Connection Limiter** - Max concurrent connections
- **Queue Throttling** - Reject low-priority when overloaded
- **Dead-letter Queue** - Move failed messages after retries
- **Health Checks** - Monitor all components
- **Graceful Shutdown** - Drain in-flight messages

## Performance Optimizations

- Direct SQL (no ORM overhead)
- Redis for sessions (not DB queries)
- Outbound queue in Redis (faster than polling DB)
- Priority-based command dispatch
- Local connection cache in session manager
- Protocol identification timeout (5s) then normal (300s)
- TCP keepalive to prevent NAT timeouts

## Key Metrics

- TCP connections accepted/active
- Heartbeats processed
- Online gauge (weighted)
- Commands by protocol/type
- Queue depth (outbound/events)
- Webhook delivery success rate
- Order processing latency
- Database query latency
