## ADDED Requirements

### Requirement: Core storage MUST be DB-agnostic

The core device/port/order/queue storage for IoT middleware SHALL be implemented against GORM models and transactions, without relying on database-specific SQL in the business path.

#### Scenario: Start charge request
- **WHEN** a third-party client calls the start-charge API for a device and port
- **THEN** the system MUST create or look up the device, lock and validate the port, create an order, and enqueue the control command using GORM-based repositories
- **AND** it MUST NOT execute raw SQL statements in the API handler or protocol handlers for this path.

### Requirement: CoreRepo abstraction for middleware responsibilities

The middleware core SHALL define a minimal `CoreRepo` interface that encapsulates device, port, order, and command-log operations, and all protocol handlers and APIs SHALL depend only on this abstraction.

#### Scenario: BKV control acknowledgment handling
- **WHEN** the IoT service receives a BKV control acknowledgement for a device and port
- **THEN** the handler MUST use `CoreRepo` methods to update the corresponding order status and port status
- **AND** the handler MUST NOT access the underlying database connection or execute driver-specific queries directly.

### Requirement: No new raw SQL in core paths

Any new functionality added to the core device/port/order/queue paths SHALL NOT introduce raw SQL; instead, it MUST be expressed in terms of GORM queries over the shared models.

#### Scenario: Extending order lifecycle
- **WHEN** a new intermediate order state or lifecycle rule is introduced
- **THEN** the implementation MUST update the GORM models and `CoreRepo` methods accordingly
- **AND** it MUST avoid adding new SQL literals in API handlers, protocol handlers, or background consistency tasks.

