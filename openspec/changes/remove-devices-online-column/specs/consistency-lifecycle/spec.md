## MODIFIED Requirements

### Requirement: Device Online Status Source

The system SHALL use the session manager (`SessionManager.IsOnline`) as the single source of truth for device online status, and the database SHALL NOT store an additional boolean online flag for devices.

#### Scenario: Check device online status
- **WHEN** any component needs to determine whether a device is online
- **THEN** it MUST call `SessionManager.IsOnline` (or weighted variants) and MAY use `devices.last_seen_at` only as a cached timestamp for diagnostics
- **AND** it MUST NOT read or rely on a `devices.online` column.

### Requirement: Device Schema Minimality

The device schema SHALL keep only fields that are either required for business behavior or for observability, avoiding redundant state that duplicates information available from other subsystems.

#### Scenario: Remove redundant online column
- **WHEN** the database schema defines a `devices.online` boolean column that duplicates the session manager's online state
- **THEN** the column MUST be removed from the canonical schema and migrations
- **AND** all code paths MUST be updated to rely on `SessionManager` + `last_seen_at` instead.

