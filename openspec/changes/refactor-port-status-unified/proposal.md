# Change: Refactor Port Status Unified Management

## Why

Currently, the port status definition is scattered across multiple protocol implementations:
- `coremodel.PortStatus` - string enum for abstraction layer
- `bkv.PortStatus` - struct for BKV protocol
- `ap3000.PortStatus` - struct for AP3000 protocol
- `gn.PortInfo.StatusBits` - int field for GN protocol

This leads to:
1. **Duplicated definitions** - Same status bit meanings defined in multiple places
2. **Magic numbers** - `0x80`, `0x20`, `0x10` scattered throughout handlers
3. **Inconsistent naming** - Multiple `PortStatus` types with different semantics
4. **Missing conversions** - No standard way to convert between protocol-layer and core-layer status
5. **Incomplete core model** - `EndReason` type exists but has no constants
6. **API 不友好** - 外部系统无法清晰理解状态码含义

## What Changes

### 新增三层状态模型

**API 层（对外接口）**：
- **ADDED** `PortStatusCode` (int) - API 友好的状态码（0=离线, 1=空闲, 2=充电中, 3=故障）
- **ADDED** `PortStatusInfo` 结构体 - 包含 code/name/description 的完整状态信息
- **ADDED** `EndReasonCode` (int) - API 友好的结束原因码（0=正常, 1=用户停止, ...）
- **ADDED** `EndReasonInfo` 结构体 - 包含结束原因的完整信息
- **ADDED** `GetStatusDefinitions()` - 返回所有状态定义，供 API 文档使用
- **ADDED** `AllPortStatusInfo()` / `AllEndReasonInfo()` - 枚举所有有效值

**协议层（设备通信）**：
- **ADDED** `RawPortStatus` (uint8) - 协议级位图状态
- **ADDED** Status bit constants: `StatusBitOnline`, `StatusBitCharging`, `StatusBitNoLoad`, etc.
- **ADDED** Common status combinations: `RawStatusOnlineIdle`, `RawStatusOnlineCharging`, etc.
- **ADDED** Methods on `RawPortStatus`: `IsOnline()`, `IsCharging()`, `IsNoLoad()`, `HasFault()`
- **ADDED** `RawEndReason` (uint8) - 协议级结束原因码

**转换函数**：
- **ADDED** `RawToStatusCode()` - 协议层 → API 层
- **ADDED** `StatusCodeToRaw()` - API 层 → 协议层
- **ADDED** `RawToEndReasonCode()` - 结束原因转换
- **ADDED** `DeriveEndReasonFromStatus()` - 从状态位推导结束原因

### 代码清理

- **MODIFIED** Protocol handlers to use unified constants instead of magic numbers
- **REMOVED** Redundant `IsCharging()`, `IsIdle()`, `IsOnline()` methods from `bkv.PortStatus`
- **REMOVED** `deriveEndReasonFromStatus()` from `bkv/tlv.go`（移至 coremodel）
- **REMOVED** `mapSocketStatusToRaw()` 中的魔数（使用常量替代）

## Impact

- Affected specs: coremodel (new capability)
- Affected code:
  - `internal/coremodel/port_status.go` - **New file** with unified definitions
  - `internal/protocol/bkv/tlv.go` - Remove redundant methods, add conversion
  - `internal/protocol/bkv/handlers.go` - Replace magic numbers
  - `internal/protocol/bkv/handlers_helper.go` - Replace magic numbers
  - `internal/protocol/gn/router.go` - Add conversion methods
  - `internal/protocol/ap3000/decode.go` - Add conversion methods

## Benefits

1. **Single Source of Truth** - All status definitions in one place
2. **Self-Documenting Code** - `IsCharging()` vs `& 0x20 != 0`
3. **Compile-Time Safety** - Type system prevents mixing status types
4. **Easier Maintenance** - Change meaning in one place
5. **Protocol Agnostic** - Unified interface for all protocols
6. **API Friendly** - 外部系统可通过 `GetStatusDefinitions()` 获取所有状态定义
7. **High Cohesion, Low Coupling** - 状态管理集中在 coremodel，协议层仅做转换

## Risk Assessment

| Risk | Mitigation |
|------|------------|
| Type conversion errors | Phased rollout with parallel validation |
| Missed replacement sites | grep scan before each phase |
| Test coverage gaps | Add comprehensive unit tests first |
| API 兼容性 | 保留原有 `PortStatus` string 类型作为内部使用 |

## Migration Strategy

Five-phase approach:
1. **Phase 1**: Add new definitions (non-breaking)
2. **Phase 2**: Add conversion methods to each protocol (compatible)
3. **Phase 3**: Replace magic numbers with constants
4. **Phase 4**: Remove redundant code and cleanup
5. **Phase 5**: Add API documentation endpoint for status definitions
