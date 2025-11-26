## ADDED Requirements

### Requirement: API-Friendly Status Codes

The system SHALL provide `PortStatusCode` (int) type with clear numeric values for API consumers.

#### Scenario: Port status code constants
- **WHEN** accessing `PortStatusCode` constants
- **THEN** the following are available:
  - `StatusCodeOffline` (0) - device offline
  - `StatusCodeIdle` (1) - device online and idle
  - `StatusCodeCharging` (2) - device is charging
  - `StatusCodeFault` (3) - device has fault

#### Scenario: Get status info
- **WHEN** calling `ToInfo()` on a `PortStatusCode` value
- **THEN** returns `PortStatusInfo` with:
  - `Code` (int) - the numeric code
  - `Name` (string) - English name (e.g., "offline", "idle")
  - `Description` (string) - Chinese description (e.g., "设备离线，无法通信")

#### Scenario: List all status codes
- **WHEN** calling `AllPortStatusInfo()`
- **THEN** returns `[]PortStatusInfo` containing all 4 status codes in order (0-3)

### Requirement: API-Friendly End Reason Codes

The system SHALL provide `EndReasonCode` (int) type for charging session end reasons.

#### Scenario: End reason code constants
- **WHEN** accessing `EndReasonCode` constants
- **THEN** the following are available:
  - `ReasonCodeNormal` (0) - normal completion
  - `ReasonCodeUserStop` (1) - user initiated stop
  - `ReasonCodeNoLoad` (2) - no load protection
  - `ReasonCodeOverCurrent` (3) - over current protection
  - `ReasonCodeOverTemp` (4) - over temperature protection
  - `ReasonCodeOverPower` (5) - over power protection
  - `ReasonCodePowerOff` (6) - power off or disconnect
  - `ReasonCodeFault` (7) - general fault

#### Scenario: Get end reason info
- **WHEN** calling `ToInfo()` on an `EndReasonCode` value
- **THEN** returns `EndReasonInfo` with code, name, and description

#### Scenario: List all end reason codes
- **WHEN** calling `AllEndReasonInfo()`
- **THEN** returns `[]EndReasonInfo` containing all 8 reason codes in order (0-7)

### Requirement: Status Definitions Endpoint

The system SHALL provide a function to get all status definitions for API documentation.

#### Scenario: Get all definitions
- **WHEN** calling `GetStatusDefinitions()`
- **THEN** returns `StatusDefinitions` struct with:
  - `PortStatus` ([]PortStatusInfo) - all port status definitions
  - `EndReason` ([]EndReasonInfo) - all end reason definitions

### Requirement: Raw Port Status Type

The system SHALL provide a `RawPortStatus` type (uint8) in the `coremodel` package that represents protocol-level port status as a bitmap.

#### Scenario: Type definition exists
- **WHEN** importing `coremodel` package
- **THEN** `RawPortStatus` type is available as `uint8` alias

#### Scenario: Bit mask constants defined
- **WHEN** accessing status bit constants
- **THEN** the following constants are available:
  - `StatusBitOnline` (0x80) - bit7: online status
  - `StatusBitMeterFault` (0x40) - bit6: meter fault
  - `StatusBitCharging` (0x20) - bit5: charging status
  - `StatusBitNoLoad` (0x10) - bit4: no load status
  - `StatusBitOverTemp` (0x08) - bit3: over temperature
  - `StatusBitOverCurrent` (0x04) - bit2: over current
  - `StatusBitOverPower` (0x02) - bit1: over power

### Requirement: Status Combination Constants

The system SHALL provide pre-defined status combination constants for common states.

#### Scenario: Common combinations available
- **WHEN** accessing status combination constants
- **THEN** the following are available:
  - `RawStatusOffline` (0x00) - device offline
  - `RawStatusOnlineIdle` (0x80) - online and idle
  - `RawStatusOnlineCharging` (0xA0) - online and charging
  - `RawStatusOnlineNoLoad` (0x90) - online with no load detected

### Requirement: Status Check Methods

The system SHALL provide methods on `RawPortStatus` to check individual status bits.

#### Scenario: Check online status
- **WHEN** calling `IsOnline()` on a `RawPortStatus` value
- **THEN** returns `true` if bit7 is set, `false` otherwise

#### Scenario: Check charging status
- **WHEN** calling `IsCharging()` on a `RawPortStatus` value
- **THEN** returns `true` if bit5 is set, `false` otherwise

#### Scenario: Check no-load status
- **WHEN** calling `IsNoLoad()` on a `RawPortStatus` value
- **THEN** returns `true` if bit4 is set, `false` otherwise

#### Scenario: Check fault status
- **WHEN** calling `HasFault()` on a `RawPortStatus` value
- **THEN** returns `true` if any of bits 6, 3, 2, or 1 indicate a fault condition

### Requirement: Raw End Reason Type

The system SHALL provide a `RawEndReason` type (uint8) for protocol-level end reason codes.

#### Scenario: Raw end reason constants
- **WHEN** accessing `RawEndReason` constants
- **THEN** protocol-specific numeric codes are available:
  - `RawReasonNormal` (0)
  - `RawReasonUserStop` (1)
  - `RawReasonOverCurrent` (2)
  - `RawReasonOverTemp` (3)
  - `RawReasonPowerOff` (4)
  - `RawReasonNoLoad` (8)

### Requirement: Protocol to API Conversion

The system SHALL provide conversion from protocol-level to API-level types.

#### Scenario: Raw status to status code
- **WHEN** calling `ToStatusCode()` on a `RawPortStatus` value
- **THEN** returns appropriate `PortStatusCode`:
  - Offline (bit7=0) → `StatusCodeOffline` (0)
  - Has fault → `StatusCodeFault` (3)
  - Charging (bit5=1) → `StatusCodeCharging` (2)
  - Otherwise → `StatusCodeIdle` (1)

#### Scenario: Raw end reason to reason code
- **WHEN** calling `ToEndReasonCode()` on a `RawEndReason` value
- **THEN** returns appropriate `EndReasonCode` (0-7)

### Requirement: Status Conversion Functions (Backward Compatible)

The system SHALL provide bidirectional conversion between raw and core status types for backward compatibility.

#### Scenario: Raw to core port status conversion
- **WHEN** calling `RawToPortStatus(raw RawPortStatus)`
- **THEN** returns appropriate `PortStatus` string:
  - Offline (bit7=0) → `PortStatusOffline`
  - Has fault → `PortStatusFault`
  - Charging (bit5=1) → `PortStatusCharging`
  - Otherwise → `PortStatusIdle`

#### Scenario: Core to raw port status conversion
- **WHEN** calling `PortStatusToRaw(status PortStatus)`
- **THEN** returns approximate `RawPortStatus` value

#### Scenario: Derive end reason from status
- **WHEN** calling `DeriveEndReasonFromStatus(raw RawPortStatus)`
- **THEN** returns `EndReasonCode` based on status bits:
  - Not online → `ReasonCodePowerOff` (6)
  - No load bit set → `ReasonCodeNoLoad` (2)
  - Over temp bit set → `ReasonCodeOverTemp` (4)
  - Over current bit set → `ReasonCodeOverCurrent` (3)
  - Otherwise → `ReasonCodeNormal` (0)
