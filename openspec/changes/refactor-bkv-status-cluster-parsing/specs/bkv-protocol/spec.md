# BKV Protocol Spec Delta

## ADDED Requirements

### Requirement: BKV Status Cluster Parsing

The system SHALL correctly parse BKV status cluster reports (command 0x1017) according to the protocol specification in `docs/协议/设备对接指引-组网设备2024(1).txt`.

The parser SHALL handle the 0x03/0x04 TLV prefix wrapper format where:
- 0x03 indicates a 1-byte value length
- 0x04 indicates a 2-byte value length

The parser SHALL extract the following socket-level fields:
- 0x4A: Socket number (1 byte)
- 0x3E: Software version (2 bytes)
- 0x07: Temperature (1 byte, in Celsius)
- 0x96: RSSI signal strength (1 byte)

The parser SHALL extract the following port-level fields from each 0x5B container:
- 0x08: Port number (1 byte, 0=A, 1=B)
- 0x09: Port status byte (1 byte, bitfield)
- 0x0A: Business number (2 bytes)
- 0x95: Voltage (2 bytes, unit: 0.1V)
- 0x0B: Instant power (2 bytes, unit: 0.1W)
- 0x0C: Instant current (2 bytes, unit: 0.001A)
- 0x0D: Energy consumed (2 bytes, unit: Wh)
- 0x0E: Charging time (2 bytes, unit: minutes)

#### Scenario: Parse protocol example successfully

- **GIVEN** a raw BKV status cluster payload matching the protocol document example
- **WHEN** ParseBKVStatusCluster is called
- **THEN** the SocketStatus struct SHALL contain:
  - SocketNo = 1
  - SoftwareVer = 0xFFFF
  - Temperature = 37 (0x25)
  - RSSI = 30 (0x1E)
  - Port A with PortNo=0, Status=0x80, Voltage=2275
  - Port B with PortNo=1, Status=0x80, Voltage=2275

#### Scenario: Handle truncated data gracefully

- **GIVEN** a truncated BKV status cluster payload
- **WHEN** ParseBKVStatusCluster is called
- **THEN** the function SHALL return an error with descriptive message
- **AND** the function SHALL NOT panic or cause undefined behavior

#### Scenario: Skip unknown tags

- **GIVEN** a BKV status cluster payload containing unknown TLV tags
- **WHEN** ParseBKVStatusCluster is called
- **THEN** the parser SHALL skip unknown tags
- **AND** the parser SHALL continue parsing remaining known fields
- **AND** the parser SHALL log the unknown tags at DEBUG level

### Requirement: TLV Tag Constants

The system SHALL define named constants for all BKV TLV tags to improve code readability and maintainability.

#### Scenario: Tag constants match protocol specification

- **GIVEN** the BKV TLV tag constants are defined
- **WHEN** compared against the protocol specification
- **THEN** all tag values SHALL match exactly:
  - TagSocketNo = 0x4A
  - TagSoftwareVer = 0x3E
  - TagTemperature = 0x07
  - TagRSSI = 0x96
  - TagPortAttr = 0x5B
  - TagPortNo = 0x08
  - TagPortStatus = 0x09
  - TagBusinessNo = 0x0A
  - TagVoltage = 0x95
  - TagPower = 0x0B
  - TagCurrent = 0x0C
  - TagEnergy = 0x0D
  - TagChargingTime = 0x0E

### Requirement: Backward Compatibility

The system SHALL maintain backward compatibility with existing code that uses the SocketStatus interface.

#### Scenario: Fallback to legacy parser

- **GIVEN** a BKV payload that cannot be parsed by the new parser
- **WHEN** GetSocketStatus is called
- **THEN** the function SHALL fallback to the legacy parsing logic
- **AND** the function SHALL return a valid SocketStatus if legacy parsing succeeds
- **AND** the function SHALL log a warning indicating fallback was used

#### Scenario: Legacy code continues to work

- **GIVEN** existing code that calls GetSocketStatus
- **WHEN** the refactored parser is deployed
- **THEN** existing functionality SHALL NOT be affected
- **AND** all existing tests SHALL continue to pass

### Requirement: Port Status Parsing

The system SHALL correctly parse the port status byte (0x09) into meaningful status flags.

The status byte format (high bit first):
- Bit 7: Online status (1=online, 0=offline)
- Bit 6: Metering status (1=normal, 0=abnormal)
- Bit 5: Charging status (1=charging, 0=idle)
- Bit 4: No-load status (1=no-load detected, 0=load present)
- Bit 3: Temperature status (1=normal, 0=over-temperature)
- Bit 2: Current status (1=normal, 0=over-current)
- Bit 1: Power status (1=normal, 0=over-power)
- Bit 0: Reserved

#### Scenario: Charging status detection

- **GIVEN** a port status byte of 0xB0 (10110000)
- **WHEN** the status is parsed
- **THEN** IsOnline SHALL be true
- **AND** IsCharging SHALL be true
- **AND** HasNoLoad SHALL be false
- **AND** IsOverTemperature SHALL be false

#### Scenario: No-load status detection

- **GIVEN** a port status byte of 0x98 (10011000)
- **WHEN** the status is parsed
- **THEN** IsOnline SHALL be true
- **AND** IsCharging SHALL be true
- **AND** HasNoLoad SHALL be true
