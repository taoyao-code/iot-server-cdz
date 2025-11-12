# IoT Charging Pile Server - Comprehensive Architecture Analysis

## Executive Summary

This is a distributed IoT device management server for charging pile equipment, built with Go. It implements a multi-layered architecture supporting:
- **Multiple protocol handlers** (AP3000, BKV with dual encoding standards)
- **TCP gateway** with protocol multiplexing and connection management
- **Distributed session management** using Redis
- **Asynchronous outbound messaging** via Redis-backed queues
- **Third-party webhook integration** with event deduplication
- **PostgreSQL persistence** with specialized repository pattern
- **Production-grade reliability** features (circuit breaker, rate limiting, health checks)

---

## 1. OVERALL ARCHITECTURE PATTERN

### Layered Architecture with Event-Driven Flows

```
┌─────────────────────────────────────────────────────────────────┐
│                    EXTERNAL SYSTEMS                             │
│  (Clients via TCP | Third-party APIs via HTTP | Webhooks)      │
└──────────────────┬──────────────────────────────────────────────┘
                   │
┌──────────────────────────────────────────────────────────────────┐
│                    GATEWAY LAYER                                 │
│  • TCP Server (listener, connection lifecycle)                  │
│  • Protocol Mux (AP3000 vs BKV detection)                       │
│  • Connection Context (read/write cycles)                       │
└──────────────────┬──────────────────────────────────────────────┘
                   │
┌──────────────────────────────────────────────────────────────────┐
│                 PROTOCOL LAYER                                   │
│  • AP3000 Adapter (stream decoder + routing table)              │
│  • BKV Adapter (frame parser + handler dispatch)                │
│  • Checksum validation, frame encoding/decoding                 │
└──────────────────┬──────────────────────────────────────────────┘
                   │
┌──────────────────────────────────────────────────────────────────┐
│              BUSINESS LOGIC LAYER                                │
│  • Protocol Handlers (AP3000/BKV)                               │
│  • Session Management (Redis + in-memory cache)                 │
│  • Card Service (charging business logic)                       │
│  • Outbound Adapter (message queueing)                          │
│  • Event/Webhook Integration                                    │
└──────────────────┬──────────────────────────────────────────────┘
                   │
┌──────────────────────────────────────────────────────────────────┐
│              PERSISTENCE LAYER                                   │
│  • PostgreSQL Repository (devices, orders, transactions)        │
│  • Redis Storage (sessions, outbound queues, events, dedup)    │
└──────────────────────────────────────────────────────────────────┘
```

### Data Flow - Inbound (Device → Server)

```
TCP Connection → Protocol Detection → Protocol Handler → Business Logic
                      ↓
                  Adapter.Sniff()
                  (identify magic bytes)
                      ↓
                Handler.Route()
                (dispatch by command)
                      ↓
            Handler.Handle*() methods
            (process command, update DB)
                      ↓
            Session.Bind/OnHeartbeat
            (track connection state)
```

### Data Flow - Outbound (Server → Device)

```
Business Logic (API/Event) → RedisQueue.Enqueue
                                    ↓
                       RedisWorker.Start() loop
                          (poll every N ms)
                                    ↓
                    Session.GetConn() (online check)
                                    ↓
                    ConnContext.Write() (send command)
                                    ↓
                       Device receives data
```

### Key Design Principles

1. **Separation of Concerns**: Each layer has a clear responsibility
2. **Interface-Based**: SessionManager, Repository interfaces enable testing/mocking
3. **Async-First**: Outbound messaging is queued, not synchronous
4. **Distributed-Ready**: Redis session manager supports multi-instance deployments
5. **Resilience**: Circuit breaker, rate limiting, graceful degradation
6. **Observability**: Prometheus metrics, structured logging (Zap), health checks

---

## 2. KEY INTERNAL PACKAGES & RESPONSIBILITIES

### 2.1 internal/app (Application Bootstrap & Lifecycle)

**Role**: Orchestrates startup/shutdown and manages shared infrastructure

**Key Files**:
- `bootstrap/app.go` - 9-phase startup sequence
- `db.go` - Database pool creation and migrations
- `redis.go` - Redis client initialization
- `session.go` - SessionManager instantiation
- `tcp.go` - TCP server setup
- `http.go` - HTTP server setup
- `event_queue.go` - Event queue/deduplication initialization
- `outbound_adapter.go` - Bridges Redis queue to BKV protocol

**Startup Phases**:
```
Phase 1: Metrics registry, ready flags
Phase 2: Redis client (required)
Phase 3: Session manager (Redis-backed with server ID)
Phase 4: PostgreSQL connection + migrations
Phase 5: Protocol handlers (AP3000, BKV)
Phase 6: HTTP server (health checks, APIs)
Phase 7: Outbound worker (Redis message processor)
Phase 7.5: Event queue workers
Phase 7.6: Order monitor (detect stuck orders)
Phase 7.7: Port status syncer (consistency check)
Phase 7.8: Event pusher (Outbox pattern)
Phase 8: TCP server (all deps ready first)
Phase 9: Graceful shutdown handler
```

**Critical Insights**:
- **Dependency ordering**: TCP server starts LAST to ensure all dependencies are ready
- **Redis queue required**: Outbound messaging is entirely Redis-based for speed
- **Server ID generation**: Enables multi-instance deployments with Redis sessions

---

### 2.2 internal/api (HTTP REST APIs)

**Role**: Exposes read-only and third-party management endpoints

**Key Handlers**:
- `readonly_handler.go` - Device/port status queries (public)
- `thirdparty_handler.go` - Device control, order management (authenticated)
- `network_handler.go` - Device network management
- `ota_handler.go` - OTA upgrade management

**Responsibilities**:
1. Read-only endpoints (devices, ports, orders)
2. Third-party commands (start charge, stop, reboot)
3. Order lifecycle (create, confirm, settle, cancel)
4. Authentication (API key validation, signature verification)
5. Event generation (order events queued for webhook push)

**Data Flow**:
```
HTTP Request
    ↓
Middleware.Auth (API key check)
    ↓
Handler (query/modify via repo)
    ↓
Outbound Queue (if device command needed)
    ↓
Event Queue (if order event created)
    ↓
HTTP Response
```

---

### 2.3 internal/gateway (TCP Connection Handler)

**Role**: Implements protocol detection and routes frames to handlers

**Key File**: `conn_handler.go`

**Responsibilities**:
1. Create protocol adapters (AP3000, BKV)
2. Register handler functions for each protocol command (20+ BKV commands)
3. Bind sessions when physical ID identified
4. Wrap handlers with metrics collection and logging

**Critical Logic**:
```go
// 1. Sniff first byte(s) to detect protocol
if apAdapter.Sniff(prefix) { /* AP3000 */ }
if bkvAdapter.Sniff(prefix) { /* BKV */ }

// 2. Register command handlers with closures
apAdapter.Register(0x20, func(f *ap3000.Frame) error {
    boundPhy = f.PhyID  // Bind on first frame
    sess.Bind(phyID, cc)
    return handlerSet.HandleRegister(ctx, f)
})

// 3. Wrap in metrics + error handling
bkvAdapter.Register(0x0015, wrapBKVHandler(func(ctx context.Context, f *bkv.Frame) error {
    return getBKVHandlers().HandleControl(ctx, f)
}))

// 4. Cleanup on disconnect
go func() {
    <-cc.Done()
    sess.UnbindByPhy(boundPhy)
    appm.SessionOfflineTotal.Inc()
}()
```

**BKV Protocol Command Registration** (25+ commands):
- `0x0000`: Heartbeat
- `0x0015`: Device control (start/stop/pause)
- `0x000B`: Card swipe
- `0x000C`: Charge end
- `0x000F`: Order confirm
- `0x000D-E, 0x001D`: Socket state query/response
- `0x0007`: OTA upgrade
- `0x0001-0x0004`: Parameter management
- Plus 10+ extended commands...

---

### 2.4 internal/protocol (Protocol Encoding/Decoding)

**Role**: Implements protocol-specific frame parsing and handler logic

#### 2.4.1 Protocol Adapters

**AP3000 Adapter** (`protocol/ap3000/adapter.go`):
```go
type Adapter struct {
    decoder *StreamDecoder  // Handles fragmentation
    table   *Table          // Command → Handler mapping
}

// Sniff checks magic bytes "D"N"Y" (0x445A59)
func (a *Adapter) Sniff(prefix []byte) bool {
    return prefix[0] == 'D' && prefix[1] == 'N' && prefix[2] == 'Y'
}

// ProcessBytes decodes frames and routes to handlers
func (a *Adapter) ProcessBytes(p []byte) error {
    frames, _ := a.decoder.Feed(p)  // Handle partial frames
    for _, fr := range frames {
        a.table.Route(fr)  // Dispatch to handler
    }
}
```

**BKV Adapter** (`protocol/bkv/adapter.go`):
- Parses fixed-header + variable-length data format
- Validates checksums (CRC16 with polynomial 0xA6BC)
- Handles message reassembly
- Supports dual encoding standards (binary + BCD)

#### 2.4.2 Handler Logic

**AP3000 Handlers** (`protocol/ap3000/handlers.go`):
- `HandleRegister()` - Device registration
- `HandleHeartbeat()` - Keep-alive ping
- `HandleGeneric()` - Generic data processing
- Integrates with Repository for DB writes

**BKV Handlers** (`protocol/bkv/handlers.go` - 57KB file, 1000+ lines):
- 25+ handler methods for different command types
- Card swipe → order creation → charging flow
- Power level charging with state machine
- Parameter read/write with ACK validation
- Network management (add/delete sockets)
- OTA upgrade progress tracking
- **Key integration**: Outbound sender for downstream commands

**Event-Driven Architecture**:
```go
// In BKV handlers, when order confirmed:
handler.HandleOrderConfirm(ctx, frame) {
    // 1. Validate with CardService
    // 2. Generate event
    eventQueue.Enqueue(OrderConfirmedEvent)
    // 3. Send downstream ACK via outbound
    outboundAdapter.SendDownlink(gatewayID, ...)
    // 4. Update DB order state
}
```

---

### 2.5 internal/session (Device Connection Tracking)

**Role**: Manages device online status and connection lifecycle

**Interface** (`session/interface.go`):
```go
type SessionManager interface {
    OnHeartbeat(phyID string, t time.Time)           // Update last seen
    Bind(phyID string, conn interface{})              // Link to TCP conn
    UnbindByPhy(phyID string)                         // Disconnect
    OnTCPClosed(phyID string, t time.Time)            // Record drop
    OnAckTimeout(phyID string, t time.Time)           // Record failure
    GetConn(phyID string) (interface{}, bool)         // Retrieve conn
    IsOnline(phyID string, now time.Time) bool        // Simple check
    IsOnlineWeighted(phyID, now time.Time, policy) bool // Multi-signal
}
```

**Implementation** (`session/redis_manager.go`):

Uses **Redis for distributed sessions**:
```
session:device:{phyID} → {LastSeen, LastTCPDown, LastAckTimeout}
session:conn:{connID}  → phyID
session:server:{serverID}:conns → Set[connID]
```

**Online Determination Strategy**:
```go
type WeightedPolicy struct {
    HeartbeatTimeout  time.Duration  // 5 min default
    TCPDownWindow     time.Duration  // Time since last TCP drop
    AckWindow         time.Duration  // Time since ACK timeout
    TCPDownPenalty    float64        // Weight reduction
    AckTimeoutPenalty float64        // Weight reduction
    Threshold         float64        // Overall threshold
}

// Multi-signal logic:
// online_score = base_score - (tcp_down_penalty if recent) - (ack_penalty if recent)
// isOnline = (online_score >= threshold) AND (heartbeat recent)
```

**Key Features**:
- **Multi-instance ready**: Server ID disambiguates in shared Redis
- **Weighted online detection**: Accounts for TCP drops and ACK failures
- **Local connection cache**: Reduces Redis lookups
- **Automatic cleanup**: TTL-based expiration

---

### 2.6 internal/storage (Data Persistence)

#### 2.6.1 PostgreSQL Repository (`storage/pg/repo.go`)

**Minimal ORM approach** - No Gorm, direct SQL:

```go
type Repository struct {
    Pool *pgxpool.Pool
}

// Device management
EnsureDevice(phyID) → id
TouchDeviceLastSeen(phyID, time)

// Command logging
InsertCmdLog(deviceID, msgID, cmd, direction, payload)

// Port/order state
UpsertPortState(deviceID, portNo, status, powerW)
UpsertOrderProgress(deviceID, portNo, orderNo, ...)
SettleOrder(deviceID, portNo, orderNo, durationSec, kwh)

// Ack tracking
AckOutboundByMsgID(deviceID, msgID, success)

// Parameter management
StoreParamWrite/GetParamWritePending/ConfirmParamWrite

// Extended (BKV)
UpsertGatewaySocket, CreateOTATask, GetOTATask
GetPendingOrderByPort, UpdateOrderToCharging, CompleteOrderByPort
```

**Design Philosophy**:
- No ORM overhead - direct parameterized queries
- UPSERT patterns for idempotent operations
- Minimal table locks (single-row updates)
- Prepared statements for consistency

#### 2.6.2 Redis Storage (`storage/redis/`)

**Outbound Queue** (`redis/outbound_queue.go`):
```
Redis Data Structures:
  outbound:queue         → Sorted Set (score = priority*1e12 + timestamp)
  outbound:processing:*  → Hash (messages being sent)
  outbound:dead          → List (failed messages)

States:
  pending → processing → success OR failed → (retry or dead)

Priority System (P1-6):
  1 = Urgent (heartbeat ACK)
  2 = Normal (control commands)
  5 = Low (queries)
  
Queue Throttling (P1-6):
  200 items  → reject low priority (>5)
  500 items  → reject medium priority (>2)
  1000 items → urgent only (<1)
```

**Event Queue** (`thirdparty/event_queue.go`):
```
Redis Data Structures:
  thirdparty:event:queue    → List (event entries)
  thirdparty:event:dlq      → List (dead letter queue)
  thirdparty:event:retry:*  → Key (retry counter)

Retry Logic:
  max_retries = 5
  Dead letter after exhaustion
  24-hour TTL on retry records
```

---

### 2.7 internal/service (Business Logic)

**CardService** (`service/card_service.go`):
- Validates card/balance
- Generates order numbers
- Calculates charge parameters (time/amount/power modes)
- Updates transaction state
- Publishes events

**PricingEngine** (`service/pricing.go`):
- Time-based pricing calculations
- Energy-based pricing
- Power-level pricing
- Service fee integration
- Overage handling

---

### 2.8 internal/thirdparty (External Integration)

**Event Queue** - Asynchronous webhook delivery:
```
Business Logic → Enqueue(StandardEvent)
                    ↓
              Redis List queue
                    ↓
         EventQueue.StartWorker() (N workers)
                    ↓
         Deduper.IsDuplicate() (optional)
                    ↓
         Pusher.Push(webhookURL, event)
                    ↓
         Retry on failure (max 5)
                    ↓
         Move to DLQ on exhaustion
```

**Event Types**:
- `OrderCreated` - New order initiated
- `ChargeStarted` - Charging active
- `ChargeEnded` - Charging complete
- `OrderSettled` - Final state reached
- And more...

**Deduplication** (`thirdparty/deduper.go`):
```
Uses Redis with TTL to prevent duplicate event delivery.
Hash(EventType + DeviceID + OrderNo + Timestamp)
Configurable TTL (default 30 seconds)
```

---

## 3. TCP GATEWAY & PROTOCOL HANDLERS INTERACTION

### 3.1 Connection Lifecycle

```
TCP Connection Accepted
    ↓
ConnContext created (reader/writer goroutines)
    ↓
Mux.BindToConn() installs onRead callback
    ↓
Protocol identification phase (5-second timeout)
    ├─ Sniff first bytes
    ├─ Detect AP3000 or BKV
    └─ RestoreNormalTimeout() (→ 300 seconds)
    ↓
Handler dispatch (based on protocol)
    ├─ Session.Bind(phyID, ConnContext)
    ├─ OnHeartbeat tracking
    └─ Metrics collection
    ↓
Long-lived connection
    ├─ Read loop: Protocol handler processes frames
    ├─ Write loop: Queued bytes → device
    └─ Background goroutine watches for done signal
    ↓
Connection closes (TCP reset, timeout, explicit)
    ↓
Session.UnbindByPhy() + OnTCPClosed()
```

### 3.2 Protocol Detection & Routing

**Mux Decision Tree** (`tcpserver/mux.go`):

```go
func (m *Mux) BindToConn(cc *ConnContext) {
    var decided bool
    cc.SetOnRead(func(p []byte) {
        if !decided {
            // Extract prefix for sniffing
            pref := p
            if len(pref) > 8 { pref = pref[:8] }
            
            // Try each adapter
            for _, a := range m.adapters {
                if a.Sniff(pref) {
                    // Protocol detected!
                    handler = func(b []byte) { a.ProcessBytes(b) }
                    cc.SetProtocol("ap3000" or "bkv")
                    cc.RestoreNormalTimeout()
                    decided = true
                    break
                }
            }
            
            if !decided {
                // Fall-through: try all adapters (for robustness)
                for _, a := range m.adapters {
                    _ = a.ProcessBytes(p)
                }
                return
            }
        }
        
        // Identified - use fixed handler
        if handler != nil {
            handler(p)
        }
    })
}
```

### 3.3 Command Dispatch (Example: BKV)

```
Frame arrives → BKV Adapter.ProcessBytes()
    ↓
Parser extracts: cmd (uint16), msgID, gatewayID, data
    ↓
wrapBKVHandler closure:
    ├─ Log frame receipt
    ├─ Increment metrics
    ├─ bindIfNeeded(gatewayID) - Session.Bind()
    └─ Call actual handler
    ↓
Switch by command:
    0x0000 → HandleHeartbeat
    0x0015 → HandleControl
    0x000B → HandleCardSwipe
    0x000C → HandleChargeEnd
    0x000F → HandleOrderConfirm
    0x001A → HandleBalanceQuery
    ... (20+ more)
    ↓
Handler processes:
    1. Validate input
    2. Query repo for state
    3. Apply business logic
    4. Enqueue events
    5. Queue outbound reply (if needed)
    6. Update DB
    ↓
Response sent via outbound worker
    (not synchronous - queued in Redis)
```

---

## 4. SESSION MANAGEMENT ARCHITECTURE

### 4.1 Session State Machine

```
┌────────────────────────────────────────────────────────────┐
│                   SESSION STATES                           │
└────────────────────────────────────────────────────────────┘

OFFLINE (no connection)
    ↓
    │ TCP connection accepted + protocol identified
    ↓
ONLINE (heartbeat received)
    │
    ├─ OnHeartbeat() → LastSeen updated
    │
    └─ OnTCPClosed() (TCP drop)
        ↓
POTENTIALLY_OFFLINE (connection dropped, but recent heartbeat)
    │
    ├─ If new connection → back to ONLINE
    │
    └─ If heartbeat timeout (5 min) → OFFLINE
        ↓
OFFLINE
```

### 4.2 Redis Schema for Distributed Sessions

```go
// Per device
session:device:{phyID}:data = JSON {
    phy_id: "GW001",
    conn_id: "uuid-xxx",      // Current connection
    server_id: "srv-001",     // Which server instance
    last_seen: "2024-11-12T10:30:00Z",
    last_tcp_down: "...",     // When TCP disconnected
    last_ack_timeout: "..."   // When ACK failed
}

// Connection index (reverse mapping)
session:conn:{connID} = phyID

// Server tracking (for cleanup)
session:server:{serverID}:conns = Set[connID1, connID2, ...]

// All have TTL = 1.5 × heartbeat timeout
```

### 4.3 Online Detection with Weighted Policy

```
Algorithm: IsOnlineWeighted(phyID, now, policy)

1. Load session data from Redis
2. Check heartbeat recency:
   if (now - lastSeen) > policy.HeartbeatTimeout → return false (dead)

3. Calculate score:
   score = 100.0
   
   // Factor in TCP downs
   if policy.TCPDownPenalty > 0:
       timeSinceTCPDown = now - lastTCPDown
       if timeSinceTCPDown < policy.TCPDownWindow:
           score -= policy.TCPDownPenalty
   
   // Factor in ACK failures
   if policy.AckTimeoutPenalty > 0:
       timeSinceAckTimeout = now - lastAckTimeout
       if timeSinceAckTimeout < policy.AckWindow:
           score -= policy.AckTimeoutPenalty
   
4. Compare to threshold:
   return score >= policy.Threshold

Default Policy:
  HeartbeatTimeout: 5 minutes
  TCPDownWindow: 1 minute
  AckWindow: 30 seconds
  TCPDownPenalty: 30 points
  AckTimeoutPenalty: 20 points
  Threshold: 50 points

// Example: Device with recent TCP drop → score = 100 - 30 = 70 > 50 → still online ✓
```

### 4.4 Connection Retrieval & Validation

```go
// In RedisWorker.processOne():
conn, ok := session.GetConn(phyID)
if !ok {
    // GetConn internally checks:
    // 1. ConnID from Redis
    // 2. Local cache lookup
    // 3. Heartbeat timeout verification
    markFailed(msg, "connection not available")
    return
}

// conn is now guaranteed valid for write
n, err := conn.Write(msg.Command)
```

---

## 5. DATABASE & STORAGE PATTERNS

### 5.1 PostgreSQL Schema (Minimal)

```sql
-- Core tables
CREATE TABLE devices (
    id BIGSERIAL PRIMARY KEY,
    phy_id VARCHAR UNIQUE,
    last_seen_at TIMESTAMP,
    created_at, updated_at TIMESTAMP
);

CREATE TABLE ports (
    device_id, port_no: compound PK,
    status INT,              -- 0=offline, 1=idle, 2=charging, etc
    power_w INT,
    updated_at TIMESTAMP
);

CREATE TABLE orders (
    order_no VARCHAR PRIMARY KEY,
    device_id, port_no,
    start_time, end_time TIMESTAMP,
    kwh_0p01 INT,            -- Energy in 0.01 kWh units
    status INT,              -- 0=pending, 1=confirmed, 2=complete
    updated_at TIMESTAMP
);

CREATE TABLE cmd_log (
    device_id, msg_id, cmd, direction, payload, success
);

-- Extended (BKV specific)
CREATE TABLE gateway_sockets (
    gateway_id, socket_no: compound PK,
    socket_type, protocol_version, firmware_version
);

CREATE TABLE ota_tasks (
    id BIGSERIAL PRIMARY KEY,
    device_id, target_version, status, progress, error_msg
);

-- Card charging
CREATE TABLE cards (
    card_no VARCHAR PRIMARY KEY,
    balance DECIMAL,
    status VARCHAR
);

CREATE TABLE card_transactions (
    order_no VARCHAR PRIMARY KEY,
    card_no, status, energy_kwh, total_amount
);
```

### 5.2 Repository Pattern

**Why not ORM?**
- Speed: Parameterized queries are faster than reflection
- Simplicity: No model tags, migration files, query builder
- Control: SQL is explicit and auditable

**Typical Usage**:
```go
// Ensure device exists (idempotent)
deviceID, err := repo.EnsureDevice(ctx, "GW001")

// Upsert port state (idempotent)
repo.UpsertPortState(ctx, deviceID, 1, statusCharging, &power)

// Upsert order (idempotent)
repo.UpsertOrderProgress(ctx, deviceID, 1, "ORDER_HEX_123", ...)

// Settle order (idempotent - overwrites on conflict)
repo.SettleOrder(ctx, deviceID, 1, "ORDER_HEX_123", ...)
```

All operations use `INSERT ... ON CONFLICT DO UPDATE` for idempotency.

### 5.3 Redis Patterns

**Queue Pattern** (Outbound):
```
Sorted Set for priority queue (score = priority × 1e12 + timestamp)
Dequeue via ZPOPMIN(1) (get lowest score = highest priority)
Mark processing (move to processing hash)
On success: delete from processing
On failure: add to dead letter queue
Retry: re-enqueue with same ID
```

**Session Pattern**:
```
Hash per device (physical ID)
Set per server (track owned connections)
All with TTL
```

**Event Queue Pattern**:
```
Redis List for work queue
Dequeue via LPOP
On success: delete (implicit)
On failure: move to DLQ via RPUSH
Retry via delay + re-enqueue
```

---

## 6. THIRD-PARTY INTEGRATION MECHANISMS

### 6.1 Webhook Delivery Architecture

```
┌─────────────────────────────────────┐
│   Business Logic                    │
│   (protocol handler, API)           │
└──────────────┬──────────────────────┘
               │
        eventQueue.Enqueue()
               ↓
┌──────────────────────────────────────┐
│   Redis Queue                        │
│   thirdparty:event:queue (List)      │
└──────────────┬───────────────────────┘
               │
               ├─ Worker 1 ┐
               ├─ Worker 2 ├─ Parallel consumption
               └─ Worker 3 ┘
                      ↓
            Deduper.IsDuplicate()
            (Redis check with TTL)
                      ↓
            Pusher.Push(webhookURL)
            (HTTP POST with signature)
                      ↓
        ┌──────────────┴──────────────┐
        │                             │
    Success (2xx)              Failure (4xx/5xx/timeout)
        │                             │
        ↓                             ↓
    Delete from queue           Increment retry counter
                                      │
                          ┌───────────┴───────────┐
                          │                       │
                      < max_retries          max retries exhausted
                          │                       │
                          ↓                       ↓
                    Re-enqueue after          Dead Letter Queue
                    backoff delay             (manual review)
```

### 6.2 Event Types & Payloads

```go
type StandardEvent struct {
    EventID         string    `json:"event_id"`        // UUID
    EventType       EventType `json:"event_type"`      // Order*, Charge*, etc
    DevicePhyID     string    `json:"device_phy_id"`
    OrderNo         string    `json:"order_no"`
    Timestamp       int64     `json:"timestamp"`       // Unix millis
    Data            json.Raw  `json:"data"`           // Event-specific
}

enum EventType {
    OrderCreated,
    OrderConfirmed,
    ChargingStarted,
    ChargingEnded,
    OrderSettled,
    OrderCancelled,
    PortStatusChanged,
    // ...
}
```

### 6.3 Signature Authentication

```go
// Webhook signature (HMAC-SHA256)
Signature = base64(HMAC-SHA256(JSON body, secret))

// Third-party validates:
// 1. Parse JSON
// 2. Compute Signature' = HMAC-SHA256(JSON, secret)
// 3. Compare with header X-Signature
```

### 6.4 Retry & Backoff Strategy

```
Attempt 1: Immediate
Attempt 2: 1 second delay
Attempt 3: 2 second delay
Attempt 4: 4 second delay
Attempt 5: 8 second delay

After attempt 5 failure → Dead Letter Queue
```

---

## 7. KEY DESIGN PATTERNS & ARCHITECTURAL DECISIONS

### 7.1 Adapter Pattern (Protocols)

**Problem**: Multiple device protocols (AP3000, BKV) on single TCP port

**Solution**:
```go
type Adapter interface {
    Sniff(prefix []byte) bool          // Early detection
    ProcessBytes(p []byte) error       // Frame parsing
}

// Mux tries each adapter:
for _, adapter := range [apAdapter, bkvAdapter] {
    if adapter.Sniff(firstBytes) {
        // Found match - use exclusively
    }
}
```

**Benefits**:
- Open/closed principle - new protocols via new Adapters
- Protocol isolation - parsers don't know about each other
- Early detection - single byte analysis

### 7.2 Handler Registration Pattern

```go
// AP3000
apAdapter.Register(0x20, func(f *ap3000.Frame) error {
    return handlers.HandleRegister(ctx, f)
})

// BKV
bkvAdapter.Register(0x0015, wrapBKVHandler(func(ctx context.Context, f *bkv.Frame) error {
    return handlers.HandleControl(ctx, f)
}))
```

**Benefits**:
- Closure-based dispatch (type-safe)
- Per-handler middleware wrapping (metrics, logging, auth)
- Testable - handlers are pure functions

### 7.3 Repository Pattern (Data Access)

**Goal**: Abstract database from business logic

```go
type Repository interface {
    EnsureDevice(ctx context.Context, phyID string) (int64, error)
    UpsertPortState(ctx context.Context, deviceID int64, ...) error
    // ...
}

// Implementation in storage/pg/
type pgRepository struct { Pool *pgxpool.Pool }
```

**Benefits**:
- Testable - mock Repository in unit tests
- Multi-database ready - could have Redis, MongoDB versions
- Decoupled - handlers don't know about SQL

### 7.4 Outbound Adapter Pattern

**Problem**: Protocol handlers need to send commands back to devices
**Challenge**: Handlers must not block on I/O

**Solution**:
```go
// Handler enqueues, doesn't wait
outboundAdapter.SendDownlink(gatewayID, cmd, data) {
    queue.Enqueue(message)  // Non-blocking
    return nil
}

// Worker independently sends
redisWorker.Start(ctx) {
    for {
        msg := queue.Dequeue(ctx)
        conn := session.GetConn(msg.PhyID)
        conn.Write(msg.Command)  // This CAN block per-message
    }
}
```

**Benefits**:
- Handler latency is O(Redis), not O(TCP I/O)
- Retry logic centralized in worker
- Priority-based queueing

### 7.5 Session Manager Interface

```go
type SessionManager interface {
    Bind(phyID string, conn interface{})
    GetConn(phyID string) (interface{}, bool)
    IsOnline(phyID string, now time.Time) bool
}
```

**Benefit**: Two implementations seamlessly:
1. **InMemory**: Single-instance, fast, tests
2. **Redis**: Multi-instance, distributed

Boot code chooses based on config:
```go
if config.Session.Type == "redis" {
    sess = session.NewRedisManager(...)
} else {
    sess = session.NewInMemoryManager(...)
}
```

### 7.6 Event Queue Decoupling

**Problem**: Webhook delivery shouldn't block order processing

**Solution**: Event queue + worker pool

```
Handler:
    repo.CreateOrder(...)      // Fast DB write
    eventQueue.Enqueue(...)    // Fire-and-forget
    return success

Worker (background):
    for event := range queue {
        webhook.POST(event)    // Slow I/O
        if fail && retries < 5 {
            re-enqueue(event)
        }
    }
```

**Benefits**:
- Handler latency unaffected by webhook latency
- Webhook failure doesn't fail order
- Retries managed asynchronously
- Dead-letter queue for manual review

### 7.7 Graceful Degradation

**Queue overload handling** (`redis/outbound_queue.go`):
```
Queue length > 200:  Reject low-priority commands
Queue length > 500:  Reject medium-priority commands
Queue length > 1000: Accept urgent commands only
```

**Connection limits** (`tcpserver/limiter.go`):
```
MaxConnections = N
Current = M
Acquire() {
    if M >= N {
        return ErrLimitExceeded
    }
}
```

**Circuit breaker**:
```
Open (reject all)  →  Half-Open (allow some)  →  Closed (allow all)
   ↓                         ↓                        ↑
 failure threshold    success threshold      recovered
```

### 7.8 Observability First

**Metrics** (`internal/metrics/`):
```go
type AppMetrics struct {
    TCPAccepted              prometheus.Counter
    TCPBytesReceived         prometheus.Counter
    HeartbeatTotal           prometheus.Counter
    OnlineGauge              prometheus.Gauge
    AP3000RouteTotal         *prometheus.CounterVec
    BKVRouteTotal            *prometheus.CounterVec
    ProtocolChecksumError    prometheus.Counter
    SessionOfflineTotal      *prometheus.CounterVec
    // ... 20+ more
}
```

**Logging** (Zap structured logs):
```
Every frame: protocol, command, gateway_id, status
Every handler: success/failure, latency, error
Every queue operation: enqueue, dequeue, retry, dead-letter
```

**Health Checks** (`internal/health/`):
```
GET /health → {
    tcp_server: "healthy",
    postgresql: "healthy",
    redis: "healthy",
    event_queue: "1500 pending"
}
```

---

## 8. DATA FLOW EXAMPLES

### 8.1 Complete Inbound Flow: BKV Heartbeat

```
Device → TCP (sends bytes)
    ↓
TCPServer.Start() loop: Accept connection
    ↓
ConnContext created (cc)
    ↓
Mux.BindToConn(cc) installed onRead handler
    ↓
First packet arrives:
    Mux detects BKV (sniff magic bytes)
    Calls cc.SetOnRead(bkvAdapter.ProcessBytes)
    cc.RestoreNormalTimeout()
    ↓
BKV Adapter parses frame:
    cmd = 0x0000 (heartbeat)
    gatewayID = "GW001"
    ↓
wrapBKVHandler wrapper:
    sess.Bind("GW001", cc)
    appm.BKVRouteTotal.Inc("0000")
    ↓
Handler dispatch:
    bkvAdapter[0x0000]() called
    ↓
HandleHeartbeat():
    sess.OnHeartbeat("GW001", now)     // Update Redis
    repo.TouchDeviceLastSeen("GW001")  // Update DB
    appm.HeartbeatTotal.Inc()
    appm.OnlineGauge.Set(N)
    ↓
Response queued (outbound):
    outboundAdapter.SendDownlink(
        gatewayID="GW001",
        cmd=ACK,
        data=heartbeat_response
    )
    ↓
RedisWorker loop (parallel):
    Dequeue from outbound queue
    Session.GetConn("GW001") → ConnContext
    cc.Write(ackFrame)
    ↓
Write loop (background goroutine):
    writeC ← ackFrame
    conn.Write(ackFrame to device)
```

### 8.2 Complete Outbound Flow: Start Charge Command (via 3rd-party API)

```
HTTP POST /thirdparty/devices/{device_id}/start_charge
    │
    ├─ Auth middleware: validate API key
    ├─ Parse JSON: {port_no: 1, duration: 60, amount: 1000}
    │
    ↓
ThirdPartyHandler.StartCharge():
    1. Validate input (port online?, device online?)
    2. Create order in DB:
       repo.UpsertOrderProgress(deviceID=123, portNo=1, ...)
    3. Queue outbound command:
       outboundQ.Enqueue({
           PhyID: "GW001",
           Cmd: 0x0015 (control),
           Data: [encoded charge params],
           Priority: 2 (normal)
       })
    4. Queue event:
       eventQueue.Enqueue({
           EventType: ChargeStarted,
           OrderNo: "ORD_123",
           DevicePhyID: "GW001",
           Timestamp: now
       })
    5. Return success (202 Accepted)
    ↓
RedisWorker (background, throttled by 100ms):
    msg := queue.Dequeue() → {PhyID: "GW001", Cmd: 0x0015, ...}
    conn := sess.GetConn("GW001") → ConnContext
    if !ok → mark failed, retry
    
    cc.Write(msg.Command)  // Queue write
    Wait(timeout)          // Simple ACK wait
    queue.MarkSuccess()
    ↓
Device receives start charge command
    Processes locally (enables charger)
    Sends status update (0x1017 socket state)
    ↓
Server receives socket state → Handler updates DB
    ↓
EventQueue Worker (parallel):
    Dequeue ChargeStarted event
    Dedup check (Redis SETEX with TTL)
    Pusher.Push(webhook_url, event) → HTTP POST
    ├─ Success: done
    ├─ Failure: retry queue (up to 5x)
    └─ Exhausted: move to DLQ
```

### 8.3 Multi-Instance Deployment (Redis Sessions)

```
Device GW001 connects to Server A:
    Server A registers: session:device:GW001 = {ServerID: "srv-A"}
    Server A tracks: session:server:srv-A:conns += connID_ABC
    ↓
Server B wants to send command to GW001:
    Get session: session:device:GW001 → {ServerID: "srv-A"}
    Enqueue in Redis outbound queue
    ↓
RedisWorker on Server A:
    Dequeue message for GW001
    sess.GetConn("GW001") → returns ConnContext (local)
    cc.Write()
    ↓
Server B (or C, D...) can also enqueue commands
    All in same Redis queue
    All processed by Server A's worker (owner of connection)
```

---

## SUMMARY

This architecture demonstrates **enterprise-grade IoT server design**:

1. **Clean separation** between protocol, session, persistence, and business logic
2. **Distributed-ready** with Redis-backed sessions and queues
3. **Resilient** with circuit breaker, rate limiting, graceful degradation
4. **Asynchronous** - handlers don't block on I/O or external services
5. **Observable** - comprehensive metrics and structured logging
6. **Testable** - interface-based dependencies, minimal coupling
7. **Performance-focused** - direct SQL, minimal ORM overhead, priority queues

The **key innovation** is the **Outbound Adapter + Redis Queue pattern**, which:
- Decouples handler latency from device I/O latency
- Enables priority-based message dispatch
- Supports automatic retries and dead-letter handling
- Scales to multiple server instances via shared Redis
