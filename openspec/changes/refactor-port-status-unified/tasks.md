# Tasks: Port Status Unified Management

## 1. Phase 1 - Add New Definitions (Non-Breaking)

- [x] 1.1 Create `internal/coremodel/port_status.go` with:

  **API 层类型**：
  - [x] 1.1.1 `PortStatusCode` (int) 类型定义和常量（0-3）
  - [x] 1.1.2 `PortStatusInfo` 结构体 `{Code, Name, Description, CanCharge, DisplayText, DisplayColor}`
  - [x] 1.1.3 `PortStatusCode.ToInfo()` 方法
  - [x] 1.1.4 `AllPortStatusInfo()` 函数 - 返回所有状态信息列表
  - [x] 1.1.5 `EndReasonCode` (int) 类型定义和常量（0-7）
  - [x] 1.1.6 `EndReasonInfo` 结构体 `{Code, Name, Description, DisplayText}`
  - [x] 1.1.7 `EndReasonCode.ToInfo()` 方法
  - [x] 1.1.8 `AllEndReasonInfo()` 函数 - 返回所有结束原因列表
  - [x] 1.1.9 `StatusDefinitions` 结构体和 `GetStatusDefinitions()` 函数
  - [x] 1.1.10 `PortStatusCode.CanCharge()` 方法 - 核心业务逻辑

  **协议层类型**：
  - [x] 1.1.11 `RawPortStatus` type definition (uint8)
  - [x] 1.1.12 Status bit mask constants (`StatusBitOnline`, `StatusBitCharging`, etc.)
  - [x] 1.1.13 Common status combination constants (`RawStatusOnlineIdle`, etc.)
  - [x] 1.1.14 Methods: `IsOnline()`, `IsCharging()`, `IsNoLoad()`, `HasFault()`, `String()`
  - [x] 1.1.15 `RawEndReason` type and constants

  **转换函数**：
  - [x] 1.1.16 `RawPortStatus.ToStatusCode()` - 协议层 → API 层
  - [x] 1.1.17 `StatusCodeToRaw()` - API 层 → 协议层
  - [x] 1.1.18 `RawStatusToCode()` - 便捷函数（int32 → PortStatusCode）
  - [x] 1.1.19 `RawEndReason.ToEndReasonCode()` - 结束原因转换
  - [x] 1.1.20 `DeriveEndReasonFromStatus()` function

- [x] 1.2 Create `internal/coremodel/port_status_test.go` with:
  - [x] 1.2.1 Test `PortStatusCode.CanCharge()` for all status codes
  - [x] 1.2.2 Test `PortStatusCode.ToInfo()` for all status codes
  - [x] 1.2.3 Test `GetStatusDefinitions()` returns complete list
  - [x] 1.2.4 Test `IsOnline()` for all relevant status values
  - [x] 1.2.5 Test `IsCharging()` for all relevant status values
  - [x] 1.2.6 Test `IsNoLoad()` for all relevant status values
  - [x] 1.2.7 Test `HasFault()` for fault conditions
  - [x] 1.2.8 Test `RawPortStatus.ToStatusCode()` conversion
  - [x] 1.2.9 Test `DeriveEndReasonFromStatus()` derivation
  - [x] 1.2.10 Test edge cases (0x00, 0xFF, common device values like 0x98)

- [x] 1.3 Verify build passes: `go build ./...`
- [x] 1.4 Verify tests pass: `go test ./internal/coremodel/...`

## 2. Phase 2 - API Integration

- [x] 2.1 Update `internal/api/thirdparty_handler.go`:
  - [x] 2.1.1 Replace `portMappingStatus()` to use `coremodel.RawStatusToCode()`
  - [x] 2.1.2 Replace `isBKVChargingStatus()` to use `coremodel.RawPortStatus.IsCharging()`
  - [x] 2.1.3 Add `GetStatusDefinitions()` API endpoint

- [x] 2.2 Update `internal/api/thirdparty_routes.go`:
  - [x] 2.2.1 Register `/api/v1/third/status/definitions` route

## 3. Phase 3 - Frontend Update

- [x] 3.1 Update `web/static/js/app.js`:
  - [x] 3.1.1 Add `PORT_STATUS` constants (OFFLINE=0, IDLE=1, CHARGING=2, FAULT=3)
  - [x] 3.1.2 Update `getPortStatusText()` to use new status codes
  - [x] 3.1.3 Add `getPortStatusColor()` function
  - [x] 3.1.4 Update `canStartCharge` to check `status === PORT_STATUS.IDLE`
  - [x] 3.1.5 Update `canStopCharge` to check `status === PORT_STATUS.CHARGING`
  - [x] 3.1.6 Update `getDeviceStatusText()` to handle all status codes

- [x] 3.2 Enhance API port data response:
  - [x] 3.2.1 Create `buildPortData()` helper function for complete port info
  - [x] 3.2.2 Update `GetDevice()` to return ports with: status, status_name, status_text, can_charge, display_color
  - [x] 3.2.3 Update `ListDevices()` to return ports with: status, status_name, status_text, can_charge, display_color

## 4. Phase 4 - Fix BKV Port Status Parsing (Critical Bug)

**问题描述**：
设备发送 0x1000 帧时，BKV Payload 中的 Cmd 字段可能是 0x1013（而非预期的 0x1017）。
当前 `IsStatusReport()` 仅检查 `Cmd == 0x1017`，导致端口状态更新逻辑从未执行。

**根因分析**：
1. 协议文档中 0x1017 = "插座状态上报"，但某些设备固件使用 0x1013
2. `HandleBKVStatus` 调用 `payload.IsStatusReport()` 判断是否为状态上报
3. `IsStatusReport()` 返回 `p.Cmd == 0x1017`，对于 Cmd=0x1013 的帧返回 false
4. 端口状态数据（tag 0x65 + value 0x94）存在但未被处理
5. 最终只有通过 0x0015 充电结束帧才会更新单个端口，导致只显示 1 个端口

**修复方案**：
不依赖 BKV Cmd 判断，改为尝试解析状态字段（tag 0x65 + value 0x94），若成功则处理为状态上报。

- [x] 4.1 Update `internal/protocol/bkv/handlers.go`:
  - [x] 4.1.1 Modify `HandleBKVStatus()` to check both `IsStatusReport()` and `HasSocketStatusFields()`
  - [x] 4.1.2 Add comments explaining the fix for BKV Cmd mismatch

- [x] 4.2 Update `internal/protocol/bkv/tlv.go`:
  - [x] 4.2.1 Update `IsStatusReport()` to also accept Cmd 0x1013
  - [x] 4.2.2 Add `HasSocketStatusFields()` method for robust detection

- [x] 4.3 Verify fix:
  - [x] 4.3.1 Build passes
  - [x] 4.3.2 coremodel tests pass

## 5. Phase 5 - Replace Magic Numbers (Completed)

- [x] 5.1 `internal/protocol/bkv/handlers.go`:
  - [x] 5.1.1 Replace `port.Status & 0x20` with `coremodel.RawPortStatus(port.Status).IsCharging()`
  - [x] 5.1.2 Replace `int32(0x90)` with `int32(coremodel.RawStatusOnlineNoLoad)`
  - [x] 5.1.3 Replace second `int32(0x90)` with `int32(coremodel.RawStatusOnlineNoLoad)`
  - [x] 5.1.4 Removed orphan comment for extractEndReason

- [x] 5.2 `internal/protocol/bkv/handlers_helper.go`:
  - [x] 5.2.1 Fixed bug: `status&0x10` was wrong (空载), changed to `IsCharging()` and `HasFault()`
  - [x] 5.2.2 Refactored `mapSocketStatusToRaw()` to use coremodel constants

- [x] 5.3 `internal/protocol/bkv/tlv.go`:
  - [x] 5.3.1 Updated `deriveEndReasonFromStatus()` to call `coremodel.DeriveEndReasonFromStatus()`

- [x] 5.4 `internal/protocol/bkv/utils.go`:
  - [x] 5.4.1 Fixed `extractEndReason()` - had wrong bit masks (0x08/0x04 instead of proper status bits)
  - [x] 5.4.2 Now delegates to `coremodel.DeriveEndReasonFromStatus()`

- [x] 5.5 Fix duplicate endReasonMsg logic:
  - [x] 5.5.1 Added `getEndReasonDescription()` helper in `handlers_helper.go`
  - [x] 5.5.2 Updated `handlers.go` to use helper instead of hardcoded incomplete logic
  - [x] 5.5.3 Updated `handlers_helper.go` to use helper instead of hardcoded incomplete logic
  - [x] 5.5.4 Helper uses `ReasonMap.GetReasonDescription()` for complete reason coverage

## 6. Phase 6 - Cleanup (Completed)

- [x] 6.1 Removed redundant methods from `bkv.PortStatus` (IsCharging, IsIdle, IsOnline)
- [x] 6.2 Kept `deriveEndReasonFromStatus` as adapter (maps coremodel.EndReasonCode → ChargingEndReason)

## Completion Criteria

**已完成**：
- [x] 统一状态模块 `coremodel/port_status.go` 创建完成
- [x] 所有状态码测试通过
- [x] API 使用统一状态转换函数
- [x] 前端使用统一状态码常量
- [x] 状态定义 API 端点可用 (`/api/v1/third/status/definitions`)
- [x] 设备端口数据包含完整状态信息 (status, status_name, status_text, can_charge, display_color)
- [x] **修复 BKV Cmd 不匹配导致端口状态不更新的问题 (Phase 4)**
- [x] **替换 BKV handlers 中的魔数为 coremodel 常量 (Phase 5)**
- [x] **修复重复且不完整的 endReasonMsg 逻辑 (Phase 5.5)**
- [x] **清理冗余代码 (Phase 6)**

**全部完成** ✅
