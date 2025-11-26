# Design: Port Status Unified Management

## Context

The IoT server handles multiple charging protocols (BKV, GN, AP3000), each with its own status representation. The protocol-level status is a bitmap (uint8) where each bit represents a specific condition. The core model uses string enums for a cleaner abstraction.

### Current State

```
Protocol Layer                    Core Layer                API Layer
─────────────────────────────────────────────────────────────────────────
BKV:  PortStatus.Status (uint8)  →  coremodel.PortStatus  →  ???
GN:   PortInfo.StatusBits (int)  →  coremodel.PortStatus  →  ???
AP3000: PortStatus.Status (int)  →  coremodel.PortStatus  →  ???
```

**Problems**:
1. Each protocol handler has its own magic number definitions
2. API consumers cannot understand what status values mean
3. No unified way to expose status definitions

### Protocol Status Bitmap (BKV Reference)

```
Bit 7: Online     (1=online, 0=offline)
Bit 6: Meter      (1=fault, 0=normal)
Bit 5: Charging   (1=charging, 0=not charging)
Bit 4: No-Load    (1=no load, 0=has load)
Bit 3: Over-Temp  (1=over temp, 0=normal)
Bit 2: Over-Curr  (1=over current, 0=normal)
Bit 1: Over-Power (1=over power, 0=normal)
Bit 0: Reserved
```

## Goals / Non-Goals

### Goals
- Centralize all status bit definitions in `coremodel`
- Provide type-safe methods for status checking
- Support bidirectional conversion (protocol ↔ API)
- Maintain backward compatibility during migration
- **Provide API-friendly status codes with descriptions**
- **Enable API documentation endpoint for status definitions**

### Non-Goals
- Change database schema (status is stored as int32)
- Modify external API contracts (backward compatible)
- Change protocol wire format

## Decisions

### Decision 1: Three-Layer Status Model

**What**: Introduce a three-layer architecture for status management.

```
┌─────────────────────────────────────────────────────────────────┐
│                        API Layer                                 │
│  PortStatusCode (int): 0=offline, 1=idle, 2=charging, 3=fault  │
│  PortStatusInfo: { code, name, description }                    │
│  GetStatusDefinitions() → all status/reason definitions         │
└─────────────────────────────────────────────────────────────────┘
                              ↑↓ ToInfo() / ToStatusCode()
┌─────────────────────────────────────────────────────────────────┐
│                       Core Layer                                 │
│  PortStatus (string): "offline", "idle", "charging", "fault"   │
│  EndReason (string): "normal", "user_stop", etc.               │
│  (Internal use, backward compatible)                            │
└─────────────────────────────────────────────────────────────────┘
                              ↑↓ conversion functions
┌─────────────────────────────────────────────────────────────────┐
│                     Protocol Layer                               │
│  RawPortStatus (uint8): bitmap with individual bits             │
│  RawEndReason (uint8): protocol-level reason codes              │
│  Methods: IsOnline(), IsCharging(), IsNoLoad(), HasFault()      │
└─────────────────────────────────────────────────────────────────┘
```

**Why**:
- API consumers get simple numeric codes (0, 1, 2, 3)
- Each code has clear name and description
- Protocol handlers use bitmap for efficient bit operations
- Core layer maintains backward compatibility

### Decision 2: API-Friendly Status Codes

**What**: Define `PortStatusCode` as simple integers with `ToInfo()` method.

```go
type PortStatusCode int

const (
    StatusCodeOffline  PortStatusCode = 0  // 离线
    StatusCodeIdle     PortStatusCode = 1  // 空闲
    StatusCodeCharging PortStatusCode = 2  // 充电中
    StatusCodeFault    PortStatusCode = 3  // 故障
)

type PortStatusInfo struct {
    Code        int    `json:"code"`
    Name        string `json:"name"`
    Description string `json:"description"`
}

func (c PortStatusCode) ToInfo() PortStatusInfo
```

**Why**:
- Simple integer codes are easy to use in APIs
- ToInfo() provides full context for documentation
- JSON-friendly structure for API responses

### Decision 3: Status Definition Endpoint

**What**: Provide `GetStatusDefinitions()` returning all status and reason definitions.

```go
type StatusDefinitions struct {
    PortStatus []PortStatusInfo `json:"port_status"`
    EndReason  []EndReasonInfo  `json:"end_reason"`
}

func GetStatusDefinitions() StatusDefinitions
```

**Why**:
- API documentation can call this to generate status reference
- External systems can cache and display status meanings
- Single source of truth for all status codes

### Decision 4: Typed Protocol-Level Status

**What**: Introduce `RawPortStatus uint8` with typed bit constants.

```go
const StatusBitOnline RawPortStatus = 0x80
```

**Why**:
- Compiler catches type mismatches
- IDE autocomplete shows relevant constants
- Self-documenting code

### Decision 5: Method-Based Status Checking

**What**: Use methods like `IsCharging()` instead of exposing bit operations.

```go
// Preferred
if status.IsCharging() { ... }

// Discouraged (but still possible for flexibility)
if status&StatusBitCharging != 0 { ... }
```

**Why**:
- Encapsulates bit layout details
- Easier to read and maintain
- Matches existing BKV pattern (just moved to coremodel)

### Decision 6: Keep Protocol-Specific Structs

**What**: Keep `bkv.PortStatus`, `ap3000.PortStatus` structs but add conversion methods.

```go
func (p *bkv.PortStatus) ToRawStatus() coremodel.RawPortStatus {
    return coremodel.RawPortStatus(p.Status)
}
```

**Why**:
- Minimal code change
- Backward compatible
- Gradual migration possible

## Type Hierarchy

```
                    API Layer
                    ─────────
            PortStatusCode (int: 0,1,2,3)
                    │
            ToInfo() │ ToStatusCode()
                    ↓
            ─────────────────
            PortStatusInfo
            - Code: int
            - Name: string
            - Description: string
                    │
                    │ conversion
                    ↓
                Core Layer
                ──────────
            PortStatus (string)
            - "offline"
            - "idle"
            - "charging"
            - "fault"
                    │
                    │ conversion
                    ↓
              Protocol Layer
              ──────────────
            RawPortStatus (uint8)
                    │
     ┌──────────────┼──────────────┐
     │              │              │
  BKV           GN             AP3000
PortStatus   PortInfo        PortStatus
.Status      .StatusBits     .Status
     │              │              │
     └──────┬───────┴──────┬───────┘
            │              │
    ToRawStatus()   ToRawStatus()
```

## File Structure

```
internal/coremodel/
├── model.go              # Existing (keep EndReason type)
├── port_status.go        # NEW: Three-layer status model
└── port_status_test.go   # NEW: Unit tests

internal/protocol/bkv/
├── tlv.go                # MODIFY: Remove IsCharging/IsIdle/IsOnline, add ToRawStatus
├── handlers.go           # MODIFY: Replace magic numbers
└── handlers_helper.go    # MODIFY: Replace magic numbers

internal/protocol/gn/
└── router.go             # MODIFY: Add GetRawStatus/GetPortStatus

internal/protocol/ap3000/
└── decode.go             # MODIFY: Add GetRawStatus/GetEndReason
```

## API Response Example

```json
{
  "status_definitions": {
    "port_status": [
      {"code": 0, "name": "offline", "description": "设备离线，无法通信"},
      {"code": 1, "name": "idle", "description": "设备在线，空闲可用"},
      {"code": 2, "name": "charging", "description": "正在充电中"},
      {"code": 3, "name": "fault", "description": "设备故障，需要维护"}
    ],
    "end_reason": [
      {"code": 0, "name": "normal", "description": "正常结束，充满或达到设定值"},
      {"code": 1, "name": "user_stop", "description": "用户主动停止充电"},
      {"code": 2, "name": "no_load", "description": "空载保护，检测到无负载"},
      {"code": 3, "name": "over_current", "description": "过流保护，电流超限"},
      {"code": 4, "name": "over_temp", "description": "过温保护，温度过高"},
      {"code": 5, "name": "over_power", "description": "过功率保护"},
      {"code": 6, "name": "power_off", "description": "断电或连接断开"},
      {"code": 7, "name": "fault", "description": "设备故障"}
    ]
  }
}
```

## Risks / Trade-offs

| Risk | Impact | Mitigation |
|------|--------|------------|
| Type conversion overflow | Status incorrectly parsed | Unit tests for all bit combinations |
| Missed replacement site | Inconsistent behavior | grep scan + code review |
| Breaking existing tests | CI failure | Run tests after each file change |
| Performance regression | Unlikely (methods are inline-able) | Benchmark if concerned |
| API 兼容性 | 旧客户端可能不识别新字段 | 新增字段为可选，不影响现有接口 |

## Migration Plan

### Phase 1: Add New Definitions (Non-Breaking)

1. Create `internal/coremodel/port_status.go`
   - Define `PortStatusCode`, `PortStatusInfo`
   - Define `RawPortStatus` with bit constants
   - Define `EndReasonCode`, `EndReasonInfo`
   - Implement `GetStatusDefinitions()`
   - Implement `AllPortStatusInfo()`, `AllEndReasonInfo()`
2. Create `internal/coremodel/port_status_test.go`
3. Run `go build ./...` - should pass
4. Run `go test ./internal/coremodel/...` - should pass

### Phase 2: Add Conversion Methods (Compatible)

1. Add `ToRawStatus()` to `bkv.PortStatus`
2. Add `GetRawStatus()` to `gn.PortInfo`
3. Add `GetRawStatus()` to `ap3000.PortStatus`
4. Add `ToCoreEndReason()` to `bkv.ChargingEndReason`
5. Run all tests - should pass

### Phase 3: Replace Magic Numbers

Per-file replacement:
1. `bkv/handlers.go`: Replace `0x90`, `0x20` with constants
2. `bkv/handlers_helper.go`: Replace `0x10`, `0x90`, `0xA0`, `0x00` with constants
3. `bkv/tlv.go`: Update `deriveEndReasonFromStatus` to use coremodel
4. Run tests after each file

### Phase 4: Cleanup

1. Remove redundant `bkv.PortStatus.IsCharging/IsIdle/IsOnline` methods
2. Remove `deriveEndReasonFromStatus` from `bkv/tlv.go`
3. Update any callers to use `coremodel.RawPortStatus` methods
4. Final test run

### Phase 5: API Integration

1. Add status definitions endpoint to API layer
2. Update API documentation
3. Verify external clients can fetch definitions

### Rollback

Each phase is a separate commit. If issues found:
- Phase 1-2: Safe to keep, no behavior change
- Phase 3: Revert specific file changes
- Phase 4-5: Revert cleanup/API commit

## Open Questions

1. **Q**: Should we expose bit constants publicly or keep them internal?
   **A**: Public - allows external tools to interpret status values

2. **Q**: Should `RawPortStatus.String()` return human-readable or hex?
   **A**: Human-readable ("charging", "idle") for debugging; hex available via `fmt.Sprintf("0x%02X", status)`

3. **Q**: How to handle protocol-specific status bits not in BKV spec?
   **A**: Add as needed with clear naming (e.g., `StatusBitAP3000Specific`)

4. **Q**: 是否需要支持国际化（i18n）的状态描述？
   **A**: 当前使用中文描述，如需国际化可扩展 `PortStatusInfo` 增加 `description_en` 字段
