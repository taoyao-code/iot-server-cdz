## MODIFIED Requirements
### Requirement: Driver-to-core event boundary

Protocol drivers SHALL communicate device-side changes to the middleware core exclusively via a small set of normalized events (DeviceHeartbeat, PortSnapshot, SessionStarted, SessionProgress, SessionEnded, ExceptionReported). Each event MUST now include the payload required for the middleware core to create/update orders, ports, card transactions, and third-party webhooks without direct access to protocol packages.

#### Scenario: Handling driver-side charging start
- **WHEN** a protocol driver parses a "charging start"/authorization success message from a device
- **THEN** it MUST emit a `SessionStarted` event that carries device identifier, port number, business/session identifiers, charge mode, target duration/energy, and any driver-supplied parameters needed for billing
- **AND** the middleware core MUST be solely responsible for creating/locking orders, invoking card services, and pushing third-party notifications.

#### Scenario: Handling in-session progress
- **WHEN** a driver receives in-session telemetry (energy, duration, power)
- **THEN** it MUST emit a `SessionProgress` event with the normalized measurements
- **AND** the middleware core MUST use it to update persisted progress and trigger any consistency jobs, without the driver touching the database.

#### Scenario: Handling protocol exceptions
- **WHEN** the driver detects protocol or hardware exceptions
- **THEN** it MUST emit an `ExceptionReported` event describing device/port, exception code, and diagnostic text
- **AND** the middleware core MUST decide how to persist, alert, or trigger retries; drivers MUST NOT call business services directly.

### Requirement: Core-to-driver command boundary

The middleware core SHALL instruct protocol drivers only through normalized commands (StartCharge, StopCharge, CancelSession, QueryPortStatus, SetParam, TriggerOTA, ConfigureNetwork), and MUST NOT construct or send protocol-specific frames directly.

#### Scenario: Sending configuration/maintenance commands
- **WHEN** the middleware core needs to configure network nodes, write device parameters, or trigger OTA upgrades
- **THEN** it MUST emit `ConfigureNetwork`, `SetParam`, or `TriggerOTA` commands via the driver API, containing only normalized identifiers and payloads (socket number, MAC address, URLs, etc.)
- **AND** drivers MUST translate these commands into the appropriate protocol frames and handle sequencing/ACK without exposing Redis queues or protocol builders to the core.

#### Scenario: Background tasks reconciling device state
- **WHEN** background workers (PortStatusSyncer, order monitor) require fresh device data
- **THEN** they MUST request it through Driver API commands (e.g., `QueryPortStatus`) and rely on emitted events for state updates
- **AND** they MUST NOT craft driver-specific payloads or enqueue Redis messages directly.
