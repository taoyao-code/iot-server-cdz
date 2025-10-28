package thirdparty

import (
	"fmt"
	"time"
)

// EventType 事件类型
type EventType string

const (
	// EventDeviceRegistered 设备注册事件
	EventDeviceRegistered EventType = "device.registered"

	// EventDeviceHeartbeat 设备心跳事件
	EventDeviceHeartbeat EventType = "device.heartbeat"

	// EventOrderCreated 订单创建事件
	EventOrderCreated EventType = "order.created"

	// EventOrderConfirmed 订单确认事件（设备确认接收）
	EventOrderConfirmed EventType = "order.confirmed"

	// EventOrderCompleted 订单完成事件
	EventOrderCompleted EventType = "order.completed"

	// EventChargingStarted 充电开始事件
	EventChargingStarted EventType = "charging.started"

	// EventChargingProgress 充电进度事件 (P0修复: 新增)
	EventChargingProgress EventType = "charging.progress"

	// EventChargingEnded 充电结束事件
	EventChargingEnded EventType = "charging.ended"

	// EventDeviceAlarm 设备告警事件
	EventDeviceAlarm EventType = "device.alarm"

	// EventSocketStateChanged 插座状态变更事件
	EventSocketStateChanged EventType = "socket.state_changed"

	// EventOTAProgressUpdate OTA升级进度更新事件
	EventOTAProgressUpdate EventType = "ota.progress_update"
)

// StandardEvent 标准事件结构
type StandardEvent struct {
	// 基础字段
	EventID     string    `json:"event_id"`      // 事件唯一ID（用于去重）
	EventType   EventType `json:"event_type"`    // 事件类型
	DevicePhyID string    `json:"device_phy_id"` // 设备物理ID
	Timestamp   int64     `json:"timestamp"`     // 事件时间戳（Unix秒）
	Nonce       string    `json:"nonce"`         // 随机数（用于签名）

	// 业务数据
	Data map[string]interface{} `json:"data"` // 具体事件数据
}

// NewEvent 创建标准事件
func NewEvent(eventType EventType, devicePhyID string, data map[string]interface{}) *StandardEvent {
	now := time.Now()
	return &StandardEvent{
		EventID:     fmt.Sprintf("%s-%s-%d", eventType, devicePhyID, now.UnixNano()),
		EventType:   eventType,
		DevicePhyID: devicePhyID,
		Timestamp:   now.Unix(),
		Nonce:       fmt.Sprintf("%08x", uint32(now.UnixNano())),
		Data:        data,
	}
}

// DeviceRegisteredData 设备注册事件数据
type DeviceRegisteredData struct {
	ICCID        string `json:"iccid"`         // SIM卡号
	IMEI         string `json:"imei"`          // 设备IMEI
	DeviceType   string `json:"device_type"`   // 设备类型
	Firmware     string `json:"firmware"`      // 固件版本
	PortCount    int    `json:"port_count"`    // 端口数量
	RegisteredAt int64  `json:"registered_at"` // 注册时间
}

// DeviceHeartbeatData 设备心跳事件数据
type DeviceHeartbeatData struct {
	Voltage  float64                `json:"voltage"`            // 电压(V)
	RSSI     int                    `json:"rssi"`               // 信号强度
	Temp     float64                `json:"temp"`               // 温度(℃)
	Ports    []PortStatus           `json:"ports"`              // 端口状态列表
	Metadata map[string]interface{} `json:"metadata,omitempty"` // 其他元数据
}

// PortStatus 端口状态
type PortStatus struct {
	PortNo int     `json:"port_no"` // 端口号
	State  string  `json:"state"`   // 状态：idle/charging/fault
	Power  float64 `json:"power"`   // 功率(W)
}

// OrderCreatedData 订单创建事件数据
type OrderCreatedData struct {
	OrderNo     string  `json:"order_no"`              // 订单号
	PortNo      int     `json:"port_no"`               // 端口号
	ChargeMode  string  `json:"charge_mode"`           // 充电模式：time/kwh/power/card
	Duration    int     `json:"duration,omitempty"`    // 时长(秒)
	KwhLimit    float64 `json:"kwh_limit,omitempty"`   // 电量限制(kWh)
	PowerLevel  int     `json:"power_level,omitempty"` // 功率档位(1-5)
	PricePerKwh float64 `json:"price_per_kwh"`         // 单价(元/kWh)
	CreatedAt   int64   `json:"created_at"`            // 创建时间
}

// OrderConfirmedData 订单确认事件数据
type OrderConfirmedData struct {
	OrderNo     string `json:"order_no"`              // 订单号
	PortNo      int    `json:"port_no"`               // 端口号
	Result      string `json:"result"`                // 结果：success/failed
	FailReason  string `json:"fail_reason,omitempty"` // 失败原因
	ConfirmedAt int64  `json:"confirmed_at"`          // 确认时间
}

// OrderCompletedData 订单完成事件数据
type OrderCompletedData struct {
	OrderNo      string  `json:"order_no"`       // 订单号
	PortNo       int     `json:"port_no"`        // 端口号
	Duration     int     `json:"duration"`       // 实际时长(秒)
	TotalKwh     float64 `json:"total_kwh"`      // 总电量(kWh)
	PeakPower    float64 `json:"peak_power"`     // 峰值功率(W)
	AvgPower     float64 `json:"avg_power"`      // 平均功率(W)
	TotalAmount  float64 `json:"total_amount"`   // 总金额(元)
	EndReason    string  `json:"end_reason"`     // 结束原因
	EndReasonMsg string  `json:"end_reason_msg"` // 结束原因说明
	CompletedAt  int64   `json:"completed_at"`   // 完成时间
}

// ChargingStartedData 充电开始事件数据
type ChargingStartedData struct {
	OrderNo   string `json:"order_no"`   // 订单号
	PortNo    int    `json:"port_no"`    // 端口号
	StartedAt int64  `json:"started_at"` // 开始时间
}

// ChargingEndedData 充电结束事件数据
type ChargingEndedData struct {
	OrderNo      string  `json:"order_no"`       // 订单号
	PortNo       int     `json:"port_no"`        // 端口号
	Duration     int     `json:"duration"`       // 时长(秒)
	TotalKwh     float64 `json:"total_kwh"`      // 总电量(kWh)
	EndReason    string  `json:"end_reason"`     // 结束原因
	EndReasonMsg string  `json:"end_reason_msg"` // 结束原因说明
	EndedAt      int64   `json:"ended_at"`       // 结束时间
}

// DeviceAlarmData 设备告警事件数据
type DeviceAlarmData struct {
	AlarmType string                 `json:"alarm_type"`         // 告警类型
	PortNo    int                    `json:"port_no,omitempty"`  // 端口号（如适用）
	Level     string                 `json:"level"`              // 告警级别：info/warning/error/critical
	Message   string                 `json:"message"`            // 告警消息
	Metadata  map[string]interface{} `json:"metadata,omitempty"` // 其他元数据
	AlarmAt   int64                  `json:"alarm_at"`           // 告警时间
}

// SocketStateChangedData 插座状态变更事件数据
type SocketStateChangedData struct {
	PortNo      int    `json:"port_no"`                // 端口号
	OldState    string `json:"old_state"`              // 旧状态
	NewState    string `json:"new_state"`              // 新状态
	StateReason string `json:"state_reason,omitempty"` // 状态变更原因
	ChangedAt   int64  `json:"changed_at"`             // 变更时间
}

// OTAProgressUpdateData OTA升级进度更新事件数据
type OTAProgressUpdateData struct {
	TaskID    string `json:"task_id"`              // OTA任务ID
	Version   string `json:"version"`              // 目标版本
	Progress  int    `json:"progress"`             // 进度(0-100)
	Status    string `json:"status"`               // 状态：downloading/installing/completed/failed
	StatusMsg string `json:"status_msg,omitempty"` // 状态说明
	ErrorMsg  string `json:"error_msg,omitempty"`  // 错误消息（如失败）
	UpdatedAt int64  `json:"updated_at"`           // 更新时间
}

// ToMap 将事件数据转换为map（用于创建StandardEvent）
func (d *DeviceRegisteredData) ToMap() map[string]interface{} {
	return map[string]interface{}{
		"iccid":         d.ICCID,
		"imei":          d.IMEI,
		"device_type":   d.DeviceType,
		"firmware":      d.Firmware,
		"port_count":    d.PortCount,
		"registered_at": d.RegisteredAt,
	}
}

func (d *DeviceHeartbeatData) ToMap() map[string]interface{} {
	return map[string]interface{}{
		"voltage":  d.Voltage,
		"rssi":     d.RSSI,
		"temp":     d.Temp,
		"ports":    d.Ports,
		"metadata": d.Metadata,
	}
}

func (d *OrderCreatedData) ToMap() map[string]interface{} {
	m := map[string]interface{}{
		"order_no":      d.OrderNo,
		"port_no":       d.PortNo,
		"charge_mode":   d.ChargeMode,
		"price_per_kwh": d.PricePerKwh,
		"created_at":    d.CreatedAt,
	}
	if d.Duration > 0 {
		m["duration"] = d.Duration
	}
	if d.KwhLimit > 0 {
		m["kwh_limit"] = d.KwhLimit
	}
	if d.PowerLevel > 0 {
		m["power_level"] = d.PowerLevel
	}
	return m
}

func (d *OrderConfirmedData) ToMap() map[string]interface{} {
	m := map[string]interface{}{
		"order_no":     d.OrderNo,
		"port_no":      d.PortNo,
		"result":       d.Result,
		"confirmed_at": d.ConfirmedAt,
	}
	if d.FailReason != "" {
		m["fail_reason"] = d.FailReason
	}
	return m
}

func (d *OrderCompletedData) ToMap() map[string]interface{} {
	return map[string]interface{}{
		"order_no":       d.OrderNo,
		"port_no":        d.PortNo,
		"duration":       d.Duration,
		"total_kwh":      d.TotalKwh,
		"peak_power":     d.PeakPower,
		"avg_power":      d.AvgPower,
		"total_amount":   d.TotalAmount,
		"end_reason":     d.EndReason,
		"end_reason_msg": d.EndReasonMsg,
		"completed_at":   d.CompletedAt,
	}
}

func (d *ChargingStartedData) ToMap() map[string]interface{} {
	return map[string]interface{}{
		"order_no":   d.OrderNo,
		"port_no":    d.PortNo,
		"started_at": d.StartedAt,
	}
}

func (d *ChargingEndedData) ToMap() map[string]interface{} {
	return map[string]interface{}{
		"order_no":       d.OrderNo,
		"port_no":        d.PortNo,
		"duration":       d.Duration,
		"total_kwh":      d.TotalKwh,
		"end_reason":     d.EndReason,
		"end_reason_msg": d.EndReasonMsg,
		"ended_at":       d.EndedAt,
	}
}

func (d *DeviceAlarmData) ToMap() map[string]interface{} {
	m := map[string]interface{}{
		"alarm_type": d.AlarmType,
		"level":      d.Level,
		"message":    d.Message,
		"alarm_at":   d.AlarmAt,
	}
	if d.PortNo > 0 {
		m["port_no"] = d.PortNo
	}
	if d.Metadata != nil {
		m["metadata"] = d.Metadata
	}
	return m
}

func (d *SocketStateChangedData) ToMap() map[string]interface{} {
	m := map[string]interface{}{
		"port_no":    d.PortNo,
		"old_state":  d.OldState,
		"new_state":  d.NewState,
		"changed_at": d.ChangedAt,
	}
	if d.StateReason != "" {
		m["state_reason"] = d.StateReason
	}
	return m
}

func (d *OTAProgressUpdateData) ToMap() map[string]interface{} {
	m := map[string]interface{}{
		"task_id":    d.TaskID,
		"version":    d.Version,
		"progress":   d.Progress,
		"status":     d.Status,
		"updated_at": d.UpdatedAt,
	}
	if d.StatusMsg != "" {
		m["status_msg"] = d.StatusMsg
	}
	if d.ErrorMsg != "" {
		m["error_msg"] = d.ErrorMsg
	}
	return m
}
