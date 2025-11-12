# Architecture Documentation Index

## Overview

This directory contains comprehensive architecture documentation for the IoT Charging Pile Server. Three complementary documents provide different levels of detail for different audiences.

## Documents

### 1. [ARCHITECTURE.md](./ARCHITECTURE.md) - Main Technical Reference
**Purpose**: Deep-dive technical analysis for architects and senior developers  
**Length**: 1243 lines (~37 KB)  
**Audience**: Technical leads, system designers, senior engineers

**Contents**:
- Executive summary with system overview
- Overall architecture pattern (layered + event-driven)
- Detailed breakdown of all 8 internal packages
- TCP gateway and protocol multiplexing deep-dive
- Session management with weighted online detection
- Database patterns and storage architecture
- Third-party webhook integration mechanisms
- 7 key design patterns and architectural decisions
- Real-world data flow examples

**Best for**:
- Understanding how everything fits together
- Learning design rationale and trade-offs
- Deep-diving into specific components
- Implementation details of complex features

---

### 2. [ARCHITECTURE_SUMMARY.md](./ARCHITECTURE_SUMMARY.md) - Quick Reference Guide
**Purpose**: Fast lookup and component relationships  
**Length**: 221 lines (~7 KB)  
**Audience**: All developers, new team members, API consumers

**Contents**:
- High-level system diagram
- Component relationships table
- Inbound/outbound data flow summaries
- Key design patterns (6 main patterns)
- 9-phase startup sequence
- Redis data structures and keys
- PostgreSQL table schemas
- API endpoint listing
- Multi-instance deployment guide
- Error handling summary

**Best for**:
- Quick reference while coding
- Understanding component relationships
- Onboarding new team members
- API consumption and integration

---

### 3. [ARCHITECTURE_DIAGRAMS.md](./ARCHITECTURE_DIAGRAMS.md) - Visual Flowcharts
**Purpose**: Visual representation of complex flows  
**Length**: 621 lines (~29 KB)  
**Audience**: All team members, business stakeholders

**Contents**:
- Complete system architecture diagram
- Inbound message flow (device → DB)
- Outbound message flow (API → device)
- Session management Redis interactions
- Protocol handler dispatch tree
- Startup dependency graph
- Multi-protocol detection flow
- Error handling and recovery flows

**Best for**:
- Understanding message flows
- Debugging connection issues
- Explaining system to stakeholders
- Understanding failure scenarios

---

## Quick Navigation

### By Role

**System Architect / Tech Lead**
1. Start with ARCHITECTURE.md (Executive Summary)
2. Review key design patterns (Section 7)
3. Check ARCHITECTURE_DIAGRAMS.md for visual confirmation

**Backend Developer (New)**
1. Read ARCHITECTURE_SUMMARY.md (entire document)
2. Review relevant section in ARCHITECTURE.md
3. Check ARCHITECTURE_DIAGRAMS.md for flows

**Backend Developer (Experienced)**
1. Use ARCHITECTURE_SUMMARY.md for quick lookups
2. Deep-dive into ARCHITECTURE.md as needed
3. Reference ARCHITECTURE_DIAGRAMS.md for debugging

**DevOps / Infrastructure**
1. Check ARCHITECTURE_SUMMARY.md (startup sequence, multi-instance)
2. Review ARCHITECTURE.md section 2.1 (app bootstrap)
3. Use ARCHITECTURE_DIAGRAMS.md for dependency graph

### By Topic

**Understanding the TCP Gateway**
- ARCHITECTURE.md Section 2.3
- ARCHITECTURE_DIAGRAMS.md Sections 1, 7

**Session Management**
- ARCHITECTURE.md Section 4
- ARCHITECTURE_SUMMARY.md (Redis data structures)
- ARCHITECTURE_DIAGRAMS.md Section 4

**Protocol Handling**
- ARCHITECTURE.md Section 2.4
- ARCHITECTURE_DIAGRAMS.md Section 5

**Database & Storage**
- ARCHITECTURE.md Section 5
- ARCHITECTURE_SUMMARY.md (table schemas)

**Async Message Flows**
- ARCHITECTURE.md Section 2.1 (startup)
- ARCHITECTURE_DIAGRAMS.md Sections 2, 3, 8

**Third-Party Integration**
- ARCHITECTURE.md Section 6
- ARCHITECTURE_SUMMARY.md (API endpoints)

**Design Patterns**
- ARCHITECTURE.md Section 7

---

## Key Architectural Concepts

### 1. Layered Architecture
```
TCP → Protocol Detection → Business Logic → Database
     (AP3000/BKV)       (Handlers, Services) (PG + Redis)
```

### 2. Async Message Queuing
- Commands: Redis Sorted Set (priority-based)
- Events: Redis List (webhook delivery)
- Both support retries and dead-letter queues

### 3. Distributed Session Management
- Redis-backed (multi-instance support)
- Weighted online detection (multi-signal)
- Connection pooling with TTL

### 4. Protocol Multiplexing
- First-byte detection (AP3000 vs BKV)
- Adapter pattern for extensibility
- Per-command handler registration

### 5. Event-Driven Design
- Handlers don't block on I/O
- Outbound commands queued
- Webhook delivery asynchronous

---

## Critical Design Decisions

1. **TCP Server starts LAST**: Ensures all dependencies ready before accepting connections
2. **Direct SQL (no ORM)**: Balances speed with control
3. **Redis-only sessions**: Enables multi-instance deployment
4. **Priority-based outbound queue**: Urgent commands (heartbeat ACK) bypass congestion
5. **Weighted online detection**: Accounts for TCP drops and ACK failures
6. **Event deduplication**: Prevents duplicate webhook delivery
7. **Graceful degradation**: Queue throttling rejects low-priority under load

---

## System Components (At a Glance)

| Component | Type | Responsibility | Tech |
|-----------|------|-----------------|------|
| TCPServer | Gateway | Listen, accept connections | Go net |
| Mux | Gateway | Protocol detection | Custom |
| AP3000/BKV Adapters | Protocol | Frame parsing | Custom |
| Handlers | Business | Order/device logic | Custom |
| SessionManager | Infrastructure | Online tracking | Redis |
| Repository | Persistence | Data access | PostgreSQL |
| OutboundQueue | Queue | Command queueing | Redis |
| EventQueue | Queue | Webhook events | Redis |
| RedisWorker | Worker | Send queued commands | Go goroutine |
| EventWorker | Worker | Deliver webhooks | Go goroutine |

---

## Common Workflows

### Adding a New BKV Command Handler
1. Define handler in `protocol/bkv/handlers.go`
2. Register in `gateway/conn_handler.go` with command ID
3. Implement business logic (validate, update DB, queue outbound)
4. Add metrics collection

### Adding a New API Endpoint
1. Define handler in `api/thirdparty_handler.go`
2. Register route in `api/thirdparty_routes.go`
3. Queue outbound command if device action needed
4. Queue event if webhook notification needed

### Investigating a Device Connection Issue
1. Check Redis session: `session:device:{phyID}`
2. Check online detection (weighted policy)
3. Check outbound queue for pending commands
4. Check TCP connection state in logs

### Debugging Message Delivery Failure
1. Check outbound:queue (message pending?)
2. Check outbound:dead (dead-letter queue)
3. Check Redis worker logs
4. Verify device connection still active

---

## Performance Notes

- **Outbound latency**: ~100-500ms (queued, not synchronous)
- **Session lookup**: O(1) Redis hash lookup + local cache
- **Protocol detection**: O(1) magic byte check
- **Queue processing**: ~100ms throttle (tunable)
- **Webhook retry**: Exponential backoff (1s, 2s, 4s, 8s)

---

## Further Reading

For implementation details on specific components, refer to source files:

```
internal/
  ├── app/bootstrap/app.go           ← Startup sequence
  ├── gateway/conn_handler.go        ← Protocol detection
  ├── protocol/bkv/handlers.go       ← BKV commands (1000+ lines)
  ├── session/redis_manager.go       ← Session management
  ├── storage/pg/repo.go             ← Database operations
  ├── service/card_service.go        ← Business logic
  ├── outbound/redis_worker.go       ← Message delivery
  ├── thirdparty/event_queue.go      ← Webhook delivery
  └── tcpserver/                     ← TCP gateway implementation
```

---

## Questions?

- **Architecture overview**: See ARCHITECTURE.md Section 1
- **Component responsibilities**: See ARCHITECTURE.md Section 2
- **How messages flow**: See ARCHITECTURE_DIAGRAMS.md Sections 2-3
- **How to extend**: See ARCHITECTURE.md Section 7 (design patterns)
- **Multi-instance setup**: See ARCHITECTURE_SUMMARY.md (multi-instance deployment)

---

**Last Updated**: 2024-11-12  
**Document Version**: 1.0  
**Codebase Version**: Latest (as of analysis)
