## 1. Core repo & model extensions
- [ ] 1.1 Inventory handler-side repo calls and map them to new `storage.CoreRepo` methods (orders, ports, cmd logs, OTAs, gateway sockets, etc.).
- [ ] 1.2 Extend `coremodel` / `driverapi` structures with the missing session/exception payloads and command types (SetParams, TriggerOTA, ConfigureNetwork, etc.).
- [ ] 1.3 Implement the new `CoreRepo` interface surface in GORM + PG repositories, with tests covering transactions and concurrency.

## 2. DriverCore event handling
- [ ] 2.1 Inject CardService/EventQueue/third-party hooks into DriverCore and implement handlers for SessionStarted/SessionProgress/SessionEnded/Exception/PortSnapshot/Heartbeat.
- [ ] 2.2 Provide deterministic unit tests for DriverCore, covering order creation/update, port snapshot writes, third-party push and card workflows.

## 3. Protocol handler refactor
- [ ] 3.1 Rewrite `internal/protocol/bkv/handlers.go` to remove all direct repo/card/event calls, emitting `CoreEvent`s instead and consuming Driver API commands only for ACK/dup logic.
- [ ] 3.2 Delete Redis/BKV fallback code paths (SetParams, OTA, network configure, etc.) now covered by Driver API commands.
- [ ] 3.3 Update handler tests to use fake EventSink/CommandSource assertions instead of repo mocks.

## 4. Background tasks & supporting services
- [ ] 4.1 Update PortStatusSyncer, order monitor, and any other worker to rely on Driver API commands + events rather than crafting protocol payloads or touching repo directly.
- [ ] 4.2 Ensure CardService/EventQueue only interact with core abstractions (no dependency on protocol packages) and provide regressions tests.

## 5. Documentation & verification
- [ ] 5.1 Record architectural consequences (design doc) capturing the new responsibility matrix and migration plan.
- [ ] 5.2 Add regression tests (unit + integration) plus run `go test ./...` and relevant e2e suites.
- [ ] 5.3 Update runbooks/docs to remove references to Redis fallback paths and describe the new driver/core boundaries.
