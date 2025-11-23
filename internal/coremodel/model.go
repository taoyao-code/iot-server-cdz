package coremodel

import "time"

// DeviceID 统一设备标识类型
type DeviceID string

// PortNo 端口编号，0-based
type PortNo int32

// SessionID 技术会话ID
type SessionID string

// BusinessNo 上游业务订单号
type BusinessNo string

// DeviceLifecycleState 设备生命周期状态
type DeviceLifecycleState string

const (
	DeviceStateUnknown        DeviceLifecycleState = "unknown"
	DeviceStateOnline         DeviceLifecycleState = "online"
	DeviceStateOffline        DeviceLifecycleState = "offline"
	DeviceStateMaintenance    DeviceLifecycleState = "maintenance"
	DeviceStateDecommissioned DeviceLifecycleState = "decommissioned"
)

// PortStatus 端口状态枚举（技术视角）
type PortStatus string

const (
	PortStatusUnknown  PortStatus = "unknown"
	PortStatusOffline  PortStatus = "offline"
	PortStatusIdle     PortStatus = "idle"
	PortStatusCharging PortStatus = "charging"
	PortStatusFault    PortStatus = "fault"
)

// SessionStatus 充电会话状态枚举
type SessionStatus string

const (
	SessionStatusPending     SessionStatus = "pending"
	SessionStatusCharging    SessionStatus = "charging"
	SessionStatusStopping    SessionStatus = "stopping"
	SessionStatusCompleted   SessionStatus = "completed"
	SessionStatusCancelled   SessionStatus = "cancelled"
	SessionStatusInterrupted SessionStatus = "interrupted"
)

// EndReason 统一结束原因编码
type EndReason string

// DeviceHeartbeatPayload 设备心跳载荷
type DeviceHeartbeatPayload struct {
	DeviceID     DeviceID
	Status       DeviceLifecycleState
	LastSeenAt   time.Time
	TemperatureC *int32
	RSSIDBm      *int32
}

// PortSnapshot 端口状态快照
type PortSnapshot struct {
	DeviceID DeviceID
	PortNo   PortNo
	Status   PortStatus
	// RawStatus 保存协议侧原始状态值（例如BKV位图），供核心直接持久化或进一步映射。
	RawStatus int32
	PowerW    *int32
	CurrentmA *int32
	VoltageV  *int32
	TempC     *int32
	At        time.Time
}

// SessionEndedPayload 充电结束报告载荷
type SessionEndedPayload struct {
	DeviceID      DeviceID
	PortNo        PortNo
	BusinessNo    BusinessNo
	EnergyKWh01   int32
	DurationSec   int32
	EndReasonCode EndReason
	InstantPowerW *int32
	AmountCent    *int64
	OccurredAt    time.Time
	// RawReason 可选保存协议原始结束原因，便于诊断
	RawReason      *int32
	NextPortStatus *int32
	RawStatus      *int32
}

// SessionStartedPayload 充电开始载荷
type SessionStartedPayload struct {
	DeviceID     DeviceID
	PortNo       PortNo
	BusinessNo   BusinessNo
	SessionID    *SessionID
	Mode         string
	ModeCode     *int32
	StartedAt    time.Time
	TargetSec    *int32
	TargetKWh01  *int32
	TargetPowerW *int32
	MaxFeeCent   *int64
	CardNo       *string
	Metadata     map[string]string
}

// SessionProgressPayload 充电进度载荷
type SessionProgressPayload struct {
	DeviceID     DeviceID
	PortNo       PortNo
	BusinessNo   BusinessNo
	SessionID    *SessionID
	EnergyKWh01  *int32
	DurationSec  *int32
	PowerW       *int32
	VoltageV     *int32
	CurrentmA    *int32
	TemperatureC *int32
	AmountCent   *int64
	RawStatus    *int32
	OccurredAt   time.Time
}

// ExceptionPayload 协议异常/硬件告警载荷
type ExceptionPayload struct {
	DeviceID   DeviceID
	PortNo     *PortNo
	Code       string
	Message    string
	Severity   string
	RawStatus  *int32
	Metadata   map[string]string
	OccurredAt time.Time
}

// CoreEventType 规范化驱动事件类型
type CoreEventType string

const (
	EventDeviceHeartbeat   CoreEventType = "DeviceHeartbeat"
	EventPortSnapshot      CoreEventType = "PortSnapshot"
	EventSessionStarted    CoreEventType = "SessionStarted"
	EventSessionProgress   CoreEventType = "SessionProgress"
	EventSessionEnded      CoreEventType = "SessionEnded"
	EventExceptionReported CoreEventType = "ExceptionReported"
	EventParamResult       CoreEventType = "ParamResult"
	EventParamSync         CoreEventType = "ParamSync"
	EventOTAProgress       CoreEventType = "OTAProgress"
	EventNetworkTopology   CoreEventType = "NetworkTopology"
)

// CoreEvent 驱动 -> 核心 的标准事件
type CoreEvent struct {
	Type            CoreEventType
	DeviceID        DeviceID
	PortNo          *PortNo
	SessionID       *SessionID
	BusinessNo      *BusinessNo
	OccurredAt      time.Time
	DeviceHeartbeat *DeviceHeartbeatPayload
	PortSnapshot    *PortSnapshot
	SessionStarted  *SessionStartedPayload
	SessionProgress *SessionProgressPayload
	SessionEnded    *SessionEndedPayload
	Exception       *ExceptionPayload
	ParamResult     *ParamResultPayload
	ParamSync       *ParamSyncPayload
	OTAProgress     *OTAProgressPayload
	NetworkTopology *NetworkTopologyPayload
	// TODO: 后续可按需扩展 SessionStarted / Progress / Exception 等载荷
}

// ParamResultPayload 参数写入/重置结果
type ParamResultPayload struct {
	DeviceID   DeviceID
	PortNo     *PortNo
	Result     string
	Message    string
	Metadata   map[string]string
	OccurredAt time.Time
}

// ParamSyncPayload 参数同步进度
type ParamSyncPayload struct {
	DeviceID   DeviceID
	Progress   int32
	Result     string
	Message    string
	Metadata   map[string]string
	OccurredAt time.Time
}

// OTAProgressPayload OTA 进度/响应
type OTAProgressPayload struct {
	DeviceID   DeviceID
	PortNo     *PortNo
	Status     string
	Progress   int32
	Message    string
	Metadata   map[string]string
	OccurredAt time.Time
}

// NetworkTopologyPayload 组网变更
type NetworkTopologyPayload struct {
	DeviceID   DeviceID
	Action     string
	SocketNo   *int32
	Result     string
	Message    string
	Metadata   map[string]string
	OccurredAt time.Time
}

// CoreCommandType 核心 -> 驱动 的命令类型
type CoreCommandType string

const (
	CommandStartCharge      CoreCommandType = "StartCharge"
	CommandStopCharge       CoreCommandType = "StopCharge"
	CommandCancelSession    CoreCommandType = "CancelSession"
	CommandQueryPortStatus  CoreCommandType = "QueryPortStatus"
	CommandSetParams        CoreCommandType = "SetParams"
	CommandTriggerOTA       CoreCommandType = "TriggerOTA"
	CommandConfigureNetwork CoreCommandType = "ConfigureNetwork"
)

// StartChargePayload 简化的开始充电命令载荷
type StartChargePayload struct {
	Mode              string
	ModeCode          *int32
	TargetDurationSec *int32
	MaxEnergyKWh01    *int32
	MaxFeeCent        *int32
	TargetPowerW      *int32
}

// StopChargePayload 停止充电命令载荷
type StopChargePayload struct {
	Reason string
}

// CancelSessionPayload 取消会话命令载荷
type CancelSessionPayload struct {
	Reason string
}

// QueryPortStatusPayload 查询端口状态命令载荷
type QueryPortStatusPayload struct {
	SocketNo *int32
}

// SetParamItem 参数写入项
type SetParamItem struct {
	ID    int32
	Value string
}

// SetParamsPayload 参数写入命令载荷
type SetParamsPayload struct {
	Params []SetParamItem
}

// TriggerOTAPayload OTA 升级命令载荷
type TriggerOTAPayload struct {
	TargetType   int32
	TargetSocket *int32
	FirmwareURL  string
	Version      string
	MD5          string
	Size         int32
}

// ConfigureNetworkPayload 组网配置命令载荷
type ConfigureNetworkPayload struct {
	Channel int32
	Nodes   []NetworkNodePayload
}

// NetworkNodePayload 组网节点
type NetworkNodePayload struct {
	SocketNo  int32
	SocketMAC string
}

// CoreCommand 核心 -> 驱动 的标准命令
type CoreCommand struct {
	Type             CoreCommandType
	CommandID        string
	DeviceID         DeviceID
	PortNo           PortNo
	SessionID        *SessionID
	BusinessNo       *BusinessNo
	IssuedAt         time.Time
	StartCharge      *StartChargePayload
	StopCharge       *StopChargePayload
	CancelSession    *CancelSessionPayload
	QueryPortStatus  *QueryPortStatusPayload
	SetParams        *SetParamsPayload
	TriggerOTA       *TriggerOTAPayload
	ConfigureNetwork *ConfigureNetworkPayload
}
