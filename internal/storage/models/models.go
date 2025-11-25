package models

import (
	"time"
)

// 注意：
// - 保持与 db/migrations/full_schema.sql 完全对齐
// - 不使用 gorm.Model，显式声明每个字段，避免隐式 DeletedAt

// Device 映射 devices 表
type Device struct {
	// 主键
	ID int64 `gorm:"column:id;primaryKey;autoIncrement"`
	// 物理设备唯一标识
	PhyID string `gorm:"column:phy_id;type:text;not null;uniqueIndex"`
	// GN 协议网关 ID，可空
	GatewayID *string `gorm:"column:gateway_id;type:text"`
	// 通信/设备标识，可空
	ICCID       *string `gorm:"column:iccid;type:text"`
	IMEI        *string `gorm:"column:imei;type:text"`
	Model       *string `gorm:"column:model;type:text"`
	FwVer       *string `gorm:"column:fw_ver;type:text"`
	FirmwareVer *string `gorm:"column:firmware_ver;type:text"`
	// 信号强度，可空
	RSSI *int32 `gorm:"column:rssi"`
	// 最近一次心跳
	LastSeenAt *time.Time `gorm:"column:last_seen_at"`
	// 审计字段
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (Device) TableName() string { return "devices" }

// GatewaySocket 映射 gateway_sockets 表
type GatewaySocket struct {
	ID             int64      `gorm:"column:id;primaryKey;autoIncrement"`
	GatewayID      string     `gorm:"column:gateway_id;type:varchar(50);not null"`
	SocketNo       int32      `gorm:"column:socket_no;not null"`
	SocketMAC      string     `gorm:"column:socket_mac;type:varchar(20);not null"`
	SocketUID      *string    `gorm:"column:socket_uid;type:varchar(20)"`
	Channel        *int32     `gorm:"column:channel"`
	Status         *int32     `gorm:"column:status"`
	SignalStrength *int32     `gorm:"column:signal_strength"`
	LastSeenAt     *time.Time `gorm:"column:last_seen_at"`
	CreatedAt      time.Time  `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt      time.Time  `gorm:"column:updated_at;autoUpdateTime"`
}

func (GatewaySocket) TableName() string { return "gateway_sockets" }

// Port 映射 ports 表（复合主键：device_id + port_no）
type Port struct {
	DeviceID int64 `gorm:"column:device_id;primaryKey"`
	PortNo   int32 `gorm:"column:port_no;primaryKey"`
	// 端口状态（与协议位图一致）
	Status    int32     `gorm:"column:status;not null;default:0"`
	PowerW    *int32    `gorm:"column:power_w"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (Port) TableName() string { return "ports" }

// CmdLog 映射 cmd_log 表（上下行指令日志）
type CmdLog struct {
	ID            int64     `gorm:"column:id;primaryKey;autoIncrement"`
	DeviceID      int64     `gorm:"column:device_id;not null;index:idx_cmdlog_device_time,priority:1"`
	MsgID         *int32    `gorm:"column:msg_id"`
	Cmd           int32     `gorm:"column:cmd;not null;index:idx_cmdlog_msg,priority:2"`
	Direction     int16     `gorm:"column:direction;not null"` // 0=UP, 1=DOWN
	Payload       []byte    `gorm:"column:payload"`
	Success       *bool     `gorm:"column:success"`
	ErrCode       *int32    `gorm:"column:err_code"`
	DurationMs    *int32    `gorm:"column:duration_ms"`
	CreatedAt     time.Time `gorm:"column:created_at;autoCreateTime;index:idx_cmdlog_device_time,priority:2,sort:desc"`
	TestSessionID *string   `gorm:"column:test_session_id;type:text;index:idx_cmd_log_test_session,where:test_session_id IS NOT NULL"`
}

func (CmdLog) TableName() string { return "cmd_log" }

// OutboundMessage 映射 outbound_queue 表
type OutboundMessage struct {
	ID       int64 `gorm:"column:id;primaryKey;autoIncrement"`
	DeviceID int64 `gorm:"column:device_id;not null;index"`
	// 直发场景保留 phy_id（与 device_id 二选一，当前 schema 两者可共存）
	PhyID         *string    `gorm:"column:phy_id;type:text"`
	PortNo        *int32     `gorm:"column:port_no"`
	Cmd           int32      `gorm:"column:cmd;not null"`
	Payload       []byte     `gorm:"column:payload"`
	Priority      int32      `gorm:"column:priority;not null;default:100"`
	Status        int32      `gorm:"column:status;not null;default:0"` // 0=pending,1=sent,2=done,3=failed
	RetryCount    int32      `gorm:"column:retry_count;not null;default:0"`
	Retries       int32      `gorm:"column:retries;not null;default:0"` // 兼容旧字段
	NotBefore     *time.Time `gorm:"column:not_before"`
	TimeoutSec    int32      `gorm:"column:timeout_sec;not null;default:15"`
	CorrelationID *string    `gorm:"column:correlation_id;type:text"`
	LastError     *string    `gorm:"column:last_error;type:text"`
	CreatedAt     time.Time  `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt     time.Time  `gorm:"column:updated_at;autoUpdateTime"`
	TestSessionID *string    `gorm:"column:test_session_id;type:text"`
}

func (OutboundMessage) TableName() string { return "outbound_queue" }
