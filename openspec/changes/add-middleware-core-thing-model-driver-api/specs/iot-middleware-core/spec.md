## ADDED Requirements

### Requirement: Technical thing model for devices, ports, and sessions

The middleware core SHALL model all connected charging devices using a protocol-agnostic technical thing model that consists of devices, ports, and charging sessions, and represents device capabilities as properties, events, and services. Device objects MUST track identifiers, product profile, physical connectivity, and at least the `online`, `offline`, `maintenance`, and `decommissioned` lifecycle states. Port objects MUST expose `status`, `power_w`, `current_ma`, `voltage_v`, and `temperature_c` properties, where `status` is constrained to `{unknown, offline, idle, charging, fault}`. Session objects MUST retain `SessionStatus` from `{pending, charging, stopping, completed, cancelled, interrupted}`, associated `BusinessNo`, and accumulated metrics (`energy_kwh01`, `duration_sec`) regardless of protocol.

#### Scenario: Representing a BKV charging session
- **WHEN** a BKV device starts a charging process on a specific socket/port with a business order number
- **THEN** the event MUST be normalized into a protocol-agnostic charging session object with device identifier, port number, business/session identifier, and an internal status (e.g., `pending` or `charging`)
- **AND** any session progress or end reports MUST update the same session object via the internal thing model, without exposing protocol-specific fields to upstream APIs.

#### Scenario: Mapping port status reports into properties
- **WHEN** a protocol driver receives a port status report (e.g., power, current, fault flags) from a device
- **THEN** the driver MUST translate it into a set of normalized port properties (such as status, power, voltage, temperature) in the middleware core
- **AND** the core MUST persist these properties as the single source of truth for port state, independent of the underlying protocol encoding.

#### Scenario: Recording device lifecycle without protocol fields
- **WHEN** a device transitions to offline, maintenance, or decommissioned states due to BKV-specific heartbeat loss or maintenance packets
- **THEN** the middleware core MUST record the lifecycle change using the normalized lifecycle state and timestamps only
- **AND** the protocol-specific reason (such as a BKV Tag or socket number) MAY be stored solely inside the diagnostic `RawStatus`, without introducing new columns or public API fields.

### Requirement: Driver-to-core event boundary

Protocol drivers SHALL communicate device-side changes to the middleware core exclusively via a small set of normalized events (DeviceHeartbeat, PortSnapshot, SessionStarted, SessionProgress, SessionEnded, ExceptionReported). Each event MUST include `DeviceID`, `OccurredAt`, and a typed payload (e.g., SessionStarted payload contains `PortNo`, `BusinessNo`, `target_mode`). No event payload may contain protocol opcodes, socket numbers, or TLV tags; protocol metadata is confined to `RawStatus` or `RawReason` fields for observability.

#### Scenario: Handling device-side charging end
- **WHEN** a protocol driver parses a "charging end" report from a device
- **THEN** it MUST emit a normalized `SessionEnded` event to the middleware core, including device identifier, port number, business/session identifier, energy usage, duration, and a mapped end reason code
- **AND** it MUST NOT directly update order tables, port tables, or invoke third-party APIs; those responsibilities belong to the middleware core reacting to the event.

#### Scenario: Reporting port status changes
- **WHEN** a protocol driver detects a change in port status (e.g., from charging to idle or to fault)
- **THEN** it MUST emit a `PortSnapshot` or equivalent event to the middleware core with the normalized status and measurements
- **AND** the core MUST be responsible for persisting this snapshot and reconciling it with any active sessions or consistency tasks.

#### Scenario: Emitting device heartbeat events
- **WHEN** a protocol driver validates a heartbeat or keepalive frame
- **THEN** it MUST emit a `DeviceHeartbeat` event with `status=online` (or `offline` if heartbeats were missing) and the most recent `last_seen_at`
- **AND** the middleware core MUST be responsible for any resulting availability alarms, redis session management, or third-party notifications.

#### Scenario: Capturing protocol exceptions
- **WHEN** the driver detects a fault condition such as repeated CRC errors, hardware exceptions, or policy violations in the protocol frames
- **THEN** it MUST emit an `ExceptionReported` event carrying the normalized `exception_code`, `port_no` (when applicable), and diagnostic text
- **AND** middleware core MUST decide whether to suspend the device, escalate alerts, or schedule retries, without the driver touching database rows.

### Requirement: Core-to-driver command boundary

The middleware core SHALL instruct protocol drivers only through normalized commands (StartCharge, StopCharge, CancelSession, QueryPortStatus, SetParam), and MUST NOT construct or send protocol-specific frames directly. Each command MUST contain protocol-agnostic identifiers (`DeviceID`, `PortNo`, `SessionID` or `BusinessNo`) plus intent parameters (charge mode, limits, timeout). Protocol encoding, sequence numbering, and retries belong solely to the driver implementation.

#### Scenario: Starting a new charging session
- **WHEN** an upstream system requests to start a new charging session on a given device and port with a business order number
- **THEN** the middleware core MUST create or update an internal session object and issue a `StartCharge` command to the appropriate protocol driver
- **AND** the command MUST include only protocol-agnostic fields (such as device identifier, port number, business/session identifier, and target mode/constraints), leaving protocol encoding details to the driver.

#### Scenario: Stopping or cancelling a charging session
- **WHEN** the middleware core decides to stop or cancel a charging session (for example due to a user request or timeout)
- **THEN** it MUST send a `StopCharge` or `CancelSession` command to the protocol driver
- **AND** it MUST NOT attempt to directly generate device-specific control frames or embed protocol fields in core data structures.

#### Scenario: Querying port status on demand
- **WHEN** the middleware core needs to reconcile stale port snapshots for a device/port
- **THEN** it MUST dispatch a `QueryPortStatus` command carrying only the normalized identifiers and a correlation ID
- **AND** the driver MUST translate that command into the necessary protocol query and emit the resulting status via `PortSnapshot`, without leaking the query frame back to the core.

#### Scenario: Modifying device parameters
- **WHEN** middleware operators change tariff or firmware parameters exposed as services
- **THEN** the middleware core MUST issue a `SetParam` command referencing the logical parameter key/value pair
- **AND** the driver MUST handle persistence, retries, and ACK correlation, while the core limits itself to observing the resulting events (success/failure) through the same driver API.

### Requirement: No database or upstream coupling in protocol drivers

Protocol drivers MUST NOT access the middleware database, transaction layers, or upstream business services directly, and MUST rely solely on the driver API to report state and receive commands.

#### Scenario: Preventing drivers from updating orders directly
- **WHEN** a protocol driver needs to reflect a change in charging status or completion reason
- **THEN** it MUST emit the appropriate normalized event (such as `SessionStarted`, `SessionProgress`, or `SessionEnded`) through the driver API
- **AND** all order lifecycle transitions, consistency fixes, and third-party notifications MUST be handled by middleware core modules in response to these events.

#### Scenario: Preventing drivers from querying core storage
- **WHEN** a protocol driver needs information about a device, port, or session (for example to map a device serial number to an internal identifier)
- **THEN** it MUST obtain this information through explicit driver API calls or notifications (such as device/product notify callbacks)
- **AND** it MUST NOT issue direct queries against middleware core tables or rely on ad-hoc SQL or ORM access.

### Requirement: Protocol-agnostic middleware core

The middleware core SHALL depend only on the normalized technical thing model and driver API, and MUST NOT import or parse protocol-specific frame types or encodings.

#### Scenario: Adding support for a new protocol
- **WHEN** a new device protocol is introduced (for example a second TCP or Modbus-based protocol)
- **THEN** the middleware core MUST be able to support it without changes to its persistence, state machine, or public APIs
- **AND** support for the new protocol MUST be implemented entirely by adding a new protocol driver that conforms to the same event/command and thing-model contracts.
