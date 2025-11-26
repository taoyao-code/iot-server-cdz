package coremodel

import "fmt"

// ============================================================================
// API 层：对外接口使用的状态码
// ============================================================================

// PortStatusCode API 友好的端口状态码
// 核心规则：只有 StatusCodeIdle (1) 才能开始充电
type PortStatusCode int

const (
	StatusCodeOffline  PortStatusCode = 0 // 设备离线
	StatusCodeIdle     PortStatusCode = 1 // 空闲可用 - 唯一可以开始充电的状态
	StatusCodeCharging PortStatusCode = 2 // 充电中
	StatusCodeFault    PortStatusCode = 3 // 故障
)

// CanCharge 判断当前状态是否可以开始充电
// 核心业务逻辑：只有 StatusCodeIdle (1) 才能充电
func (c PortStatusCode) CanCharge() bool {
	return c == StatusCodeIdle
}

// PortStatusInfo 端口状态完整信息（用于 API 响应）
type PortStatusInfo struct {
	Code         int    `json:"code"`          // 状态码
	Name         string `json:"name"`          // 英文名称
	DisplayText  string `json:"display_text"`  // 显示文本（中文）
	Description  string `json:"description"`   // 详细描述
	CanCharge    bool   `json:"can_charge"`    // 是否可以开始充电
	DisplayColor string `json:"display_color"` // 建议显示颜色
}

// ToInfo 获取状态的完整信息
func (c PortStatusCode) ToInfo() PortStatusInfo {
	switch c {
	case StatusCodeOffline:
		return PortStatusInfo{
			Code:         0,
			Name:         "offline",
			DisplayText:  "设备离线",
			Description:  "设备离线，无法通信",
			CanCharge:    false,
			DisplayColor: "gray",
		}
	case StatusCodeIdle:
		return PortStatusInfo{
			Code:         1,
			Name:         "idle",
			DisplayText:  "空闲可用",
			Description:  "设备在线，空闲可用，可以开始充电",
			CanCharge:    true,
			DisplayColor: "green",
		}
	case StatusCodeCharging:
		return PortStatusInfo{
			Code:         2,
			Name:         "charging",
			DisplayText:  "使用中",
			Description:  "正在充电中，端口被占用",
			CanCharge:    false,
			DisplayColor: "yellow",
		}
	case StatusCodeFault:
		return PortStatusInfo{
			Code:         3,
			Name:         "fault",
			DisplayText:  "故障",
			Description:  "设备故障，需要维护",
			CanCharge:    false,
			DisplayColor: "red",
		}
	default:
		return PortStatusInfo{
			Code:         int(c),
			Name:         "unknown",
			DisplayText:  "未知",
			Description:  "未知状态",
			CanCharge:    false,
			DisplayColor: "gray",
		}
	}
}

// String 返回状态名称
func (c PortStatusCode) String() string {
	return c.ToInfo().Name
}

// AllPortStatusInfo 返回所有端口状态信息列表
func AllPortStatusInfo() []PortStatusInfo {
	return []PortStatusInfo{
		StatusCodeOffline.ToInfo(),
		StatusCodeIdle.ToInfo(),
		StatusCodeCharging.ToInfo(),
		StatusCodeFault.ToInfo(),
	}
}

// ============================================================================
// API 层：充电结束原因码
// ============================================================================

// EndReasonCode API 友好的结束原因码
type EndReasonCode int

const (
	ReasonCodeNormal      EndReasonCode = 0 // 正常结束
	ReasonCodeUserStop    EndReasonCode = 1 // 用户停止
	ReasonCodeNoLoad      EndReasonCode = 2 // 空载保护
	ReasonCodeOverCurrent EndReasonCode = 3 // 过流保护
	ReasonCodeOverTemp    EndReasonCode = 4 // 过温保护
	ReasonCodeOverPower   EndReasonCode = 5 // 过功率保护
	ReasonCodePowerOff    EndReasonCode = 6 // 断电/断开
	ReasonCodeFault       EndReasonCode = 7 // 设备故障
)

// EndReasonInfo 结束原因完整信息
type EndReasonInfo struct {
	Code        int    `json:"code"`         // 原因码
	Name        string `json:"name"`         // 英文名称
	DisplayText string `json:"display_text"` // 显示文本
	Description string `json:"description"`  // 详细描述
}

// ToInfo 获取结束原因的完整信息
func (r EndReasonCode) ToInfo() EndReasonInfo {
	switch r {
	case ReasonCodeNormal:
		return EndReasonInfo{Code: 0, Name: "normal", DisplayText: "正常结束", Description: "充电正常完成，充满或达到设定值"}
	case ReasonCodeUserStop:
		return EndReasonInfo{Code: 1, Name: "user_stop", DisplayText: "用户停止", Description: "用户主动停止充电"}
	case ReasonCodeNoLoad:
		return EndReasonInfo{Code: 2, Name: "no_load", DisplayText: "空载保护", Description: "检测到无负载，自动停止"}
	case ReasonCodeOverCurrent:
		return EndReasonInfo{Code: 3, Name: "over_current", DisplayText: "过流保护", Description: "电流超过安全限制"}
	case ReasonCodeOverTemp:
		return EndReasonInfo{Code: 4, Name: "over_temp", DisplayText: "过温保护", Description: "温度过高，自动停止"}
	case ReasonCodeOverPower:
		return EndReasonInfo{Code: 5, Name: "over_power", DisplayText: "过功率保护", Description: "功率超过安全限制"}
	case ReasonCodePowerOff:
		return EndReasonInfo{Code: 6, Name: "power_off", DisplayText: "断电", Description: "设备断电或连接断开"}
	case ReasonCodeFault:
		return EndReasonInfo{Code: 7, Name: "fault", DisplayText: "故障", Description: "设备故障导致停止"}
	default:
		return EndReasonInfo{Code: int(r), Name: "unknown", DisplayText: "未知", Description: "未知原因"}
	}
}

// String 返回原因名称
func (r EndReasonCode) String() string {
	return r.ToInfo().Name
}

// AllEndReasonInfo 返回所有结束原因信息列表
func AllEndReasonInfo() []EndReasonInfo {
	return []EndReasonInfo{
		ReasonCodeNormal.ToInfo(),
		ReasonCodeUserStop.ToInfo(),
		ReasonCodeNoLoad.ToInfo(),
		ReasonCodeOverCurrent.ToInfo(),
		ReasonCodeOverTemp.ToInfo(),
		ReasonCodeOverPower.ToInfo(),
		ReasonCodePowerOff.ToInfo(),
		ReasonCodeFault.ToInfo(),
	}
}

// ============================================================================
// 状态定义汇总（供 API 文档使用）
// ============================================================================

// StatusDefinitions 所有状态定义的汇总
type StatusDefinitions struct {
	PortStatus []PortStatusInfo `json:"port_status"` // 端口状态定义
	EndReason  []EndReasonInfo  `json:"end_reason"`  // 结束原因定义
}

// GetStatusDefinitions 获取所有状态定义（供 API 返回）
func GetStatusDefinitions() StatusDefinitions {
	return StatusDefinitions{
		PortStatus: AllPortStatusInfo(),
		EndReason:  AllEndReasonInfo(),
	}
}

// ============================================================================
// 协议层：设备通信使用的原始状态位图
// ============================================================================

// RawPortStatus 协议层端口状态（位图格式）
// BKV 协议格式：
//   - Bit7: 在线状态 (1=在线, 0=离线)
//   - Bit6: 电表故障 (1=故障, 0=正常)
//   - Bit5: 充电状态 (1=充电中, 0=未充电)
//   - Bit4: 空载状态 (1=空载, 0=有负载)
//   - Bit3: 过温 (1=过温, 0=正常)
//   - Bit2: 过流 (1=过流, 0=正常)
//   - Bit1: 过功率 (1=过功率, 0=正常)
//   - Bit0: 保留
type RawPortStatus uint8

// 状态位掩码常量
const (
	StatusBitOnline      RawPortStatus = 0x80 // bit7: 在线
	StatusBitMeterFault  RawPortStatus = 0x40 // bit6: 电表故障
	StatusBitCharging    RawPortStatus = 0x20 // bit5: 充电中
	StatusBitNoLoad      RawPortStatus = 0x10 // bit4: 空载
	StatusBitOverTemp    RawPortStatus = 0x08 // bit3: 过温
	StatusBitOverCurrent RawPortStatus = 0x04 // bit2: 过流
	StatusBitOverPower   RawPortStatus = 0x02 // bit1: 过功率
)

// 常用状态组合常量
const (
	RawStatusOffline        RawPortStatus = 0x00 // 离线
	RawStatusOnlineIdle     RawPortStatus = 0x80 // 在线空闲
	RawStatusOnlineCharging RawPortStatus = 0xA0 // 在线充电中 (0x80 | 0x20)
	RawStatusOnlineNoLoad   RawPortStatus = 0x90 // 在线空载 (0x80 | 0x10)
)

// IsOnline 检查是否在线（bit7）
func (s RawPortStatus) IsOnline() bool {
	return s&StatusBitOnline != 0
}

// IsCharging 检查是否正在充电（bit5）
func (s RawPortStatus) IsCharging() bool {
	return s&StatusBitCharging != 0
}

// IsNoLoad 检查是否空载（bit4）
func (s RawPortStatus) IsNoLoad() bool {
	return s&StatusBitNoLoad != 0
}

// HasFault 检查是否有故障（bit6, bit3, bit2, bit1 任一置位）
func (s RawPortStatus) HasFault() bool {
	faultBits := StatusBitMeterFault | StatusBitOverTemp | StatusBitOverCurrent | StatusBitOverPower
	return s&faultBits != 0
}

// ToStatusCode 将协议层状态转换为 API 状态码
// 转换规则：
//   - 不在线 → StatusCodeOffline (0)
//   - 有故障 → StatusCodeFault (3)
//   - 充电中 → StatusCodeCharging (2)
//   - 其他 → StatusCodeIdle (1)
func (s RawPortStatus) ToStatusCode() PortStatusCode {
	if !s.IsOnline() {
		return StatusCodeOffline
	}
	if s.HasFault() {
		return StatusCodeFault
	}
	if s.IsCharging() {
		return StatusCodeCharging
	}
	return StatusCodeIdle
}

// ToPortStatus 将协议层状态转换为核心层状态（兼容旧代码）
func (s RawPortStatus) ToPortStatus() PortStatus {
	switch s.ToStatusCode() {
	case StatusCodeOffline:
		return PortStatusOffline
	case StatusCodeIdle:
		return PortStatusIdle
	case StatusCodeCharging:
		return PortStatusCharging
	case StatusCodeFault:
		return PortStatusFault
	default:
		return PortStatusUnknown
	}
}

// String 返回人类可读的状态描述
func (s RawPortStatus) String() string {
	return fmt.Sprintf("0x%02X(%s)", uint8(s), s.ToStatusCode().String())
}

// ============================================================================
// 协议层：原始结束原因码
// ============================================================================

// RawEndReason 协议层结束原因码
type RawEndReason uint8

// 协议层结束原因常量（BKV 协议）
const (
	RawReasonNormal      RawEndReason = 0 // 正常结束
	RawReasonUserStop    RawEndReason = 1 // 用户停止
	RawReasonOverCurrent RawEndReason = 2 // 过流
	RawReasonOverTemp    RawEndReason = 3 // 过温
	RawReasonPowerOff    RawEndReason = 4 // 断电
	RawReasonNoLoad      RawEndReason = 8 // 空载
)

// ToEndReasonCode 将协议层原因码转换为 API 原因码
func (r RawEndReason) ToEndReasonCode() EndReasonCode {
	switch r {
	case RawReasonNormal:
		return ReasonCodeNormal
	case RawReasonUserStop:
		return ReasonCodeUserStop
	case RawReasonOverCurrent:
		return ReasonCodeOverCurrent
	case RawReasonOverTemp:
		return ReasonCodeOverTemp
	case RawReasonPowerOff:
		return ReasonCodePowerOff
	case RawReasonNoLoad:
		return ReasonCodeNoLoad
	default:
		return ReasonCodeFault
	}
}

// DeriveEndReasonFromStatus 从状态位图推导结束原因
// 用于设备未明确上报原因时的推导
func DeriveEndReasonFromStatus(status RawPortStatus) EndReasonCode {
	if !status.IsOnline() {
		return ReasonCodePowerOff
	}
	if status.IsNoLoad() {
		return ReasonCodeNoLoad
	}
	if status&StatusBitOverTemp != 0 {
		return ReasonCodeOverTemp
	}
	if status&StatusBitOverCurrent != 0 {
		return ReasonCodeOverCurrent
	}
	if status&StatusBitOverPower != 0 {
		return ReasonCodeOverPower
	}
	if status&StatusBitMeterFault != 0 {
		return ReasonCodeFault
	}
	return ReasonCodeNormal
}

// ============================================================================
// 转换辅助函数
// ============================================================================

// StatusCodeToRaw 将 API 状态码转换为协议层状态（近似值）
func StatusCodeToRaw(code PortStatusCode) RawPortStatus {
	switch code {
	case StatusCodeOffline:
		return RawStatusOffline
	case StatusCodeIdle:
		return RawStatusOnlineIdle
	case StatusCodeCharging:
		return RawStatusOnlineCharging
	case StatusCodeFault:
		return RawPortStatus(StatusBitOnline | StatusBitMeterFault)
	default:
		return RawStatusOffline
	}
}

// RawStatusToCode 将 int32 原始状态转换为 API 状态码（便捷函数）
func RawStatusToCode(rawStatus int32) PortStatusCode {
	return RawPortStatus(uint8(rawStatus)).ToStatusCode()
}

// PortStatusToCode 将核心层状态字符串转换为 API 状态码
func PortStatusToCode(status PortStatus) PortStatusCode {
	switch status {
	case PortStatusOffline:
		return StatusCodeOffline
	case PortStatusIdle:
		return StatusCodeIdle
	case PortStatusCharging:
		return StatusCodeCharging
	case PortStatusFault:
		return StatusCodeFault
	default:
		return StatusCodeOffline
	}
}
