# Change: Refactor driver/core boundary for BKV protocol

## Why
- Protocol handlers still operate directly on orders, ports, third-party push, and card services, which contradicts the iot-middleware-core spec and blocks multi-protocol reuse.
- DriverCore currently handles only heartbeat/port snapshot/session end, so business logic is fragmented and difficult to reason about or test.
- Background tasks (PortStatusSyncer, order monitor) and third-party APIs still depend on Redis/BKV payload fallbacks, preventing us from proving the "device ↔ IoT ↔ third-party" separation required by the platform.

## What Changes
- Extend `CoreRepo` and DriverCore so that all Session* / Port / Exception events are processed in the core, including card-service interactions, third-party event queue writes, and consistency updates.
- Rewrite `internal/protocol/bkv` handlers so they only parse frames, dedupe/ACK at the protocol layer, and emit/consume Driver API events/commands—no direct DB or business service access.
- Update background tasks (PortStatusSyncer, order monitor) and any remaining command senders to use Driver API abstractions instead of crafting BKV payloads or touching Redis queues.
- Provide design documentation + tests proving the new boundary and removing dead code paths (Redis fallbacks, handler-side business logic, duplicated card-service flows).

## Impact
- Affects `internal/coremodel`, `internal/app/driver_core`, `internal/storage/{pg,gormrepo}`, `internal/protocol/bkv/*`, background tasks, and their tests.
- Requires new migrations or schema alignment only if CoreRepo needs additional columns/indexes (to be confirmed in design).
- After this change, feature development for new protocols or third-party flows can rely on DriverCore + Driver API invariants, and any deviation becomes testable debt.
