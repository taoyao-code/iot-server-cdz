## ADDED Requirements

### Requirement: Middleware core responsibilities

The IoT service SHALL act as a protocol-agnostic middleware whose core responsibilities are limited to device/port state synchronization, order lifecycle management, outbound command queuing, and event delivery.

#### Scenario: New upstream business requirement
- **WHEN** a new business-specific rule is requested by an upstream system
- **THEN** the rule MUST be implemented in an upstream service or protocol adapter
- **AND** it MUST NOT be added as ad-hoc business logic inside the middleware core modules.

### Requirement: Protocol adapters decoupled from core

Each supported device protocol (e.g., BKV, GN) SHALL be implemented as a protocol adapter that translates protocol frames into core events and state updates, without embedding business-specific decisions.

#### Scenario: Handling BKV heartbeat and status reports
- **WHEN** a BKV device sends heartbeats and socket status reports
- **THEN** the BKV adapter MUST parse frames and call core services to update device last-seen time, port status, and related events
- **AND** it MUST NOT directly manipulate orders or apply upstream business rules.

### Requirement: Removal of redundant and dead code

The middleware codebase SHALL be periodically analyzed to remove redundant, dead, or clearly incorrect code paths that are no longer used or contradict the current architecture and consistency specifications.

#### Scenario: Legacy patch code for a one-off incident
- **WHEN** a legacy patch was introduced to fix a one-off production incident and is no longer required under the new consistency lifecycle
- **THEN** the code path MUST be either refactored into a documented feature or removed from the runtime codebase
- **AND** any remaining scripts or SQL used for one-time fixes MUST be isolated from the main execution path.

