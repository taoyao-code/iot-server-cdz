package e2e

import "time"

// OrderStatus 订单状态
type OrderStatus string

const (
	OrderStatusPending   OrderStatus = "pending"   // 等待设备响应
	OrderStatusCharging  OrderStatus = "charging"  // 充电中
	OrderStatusCompleted OrderStatus = "completed" // 已完成
	OrderStatusFailed    OrderStatus = "failed"    // 失败
	OrderStatusCancelled OrderStatus = "cancelled" // 已取消
)

// ChargeMode 充电模式
type ChargeMode int

const (
	ChargeModeByDuration ChargeMode = 1 // 按时长
	ChargeModeByAmount   ChargeMode = 2 // 按电量
	ChargeModeByPower    ChargeMode = 3 // 按功率
	ChargeModeAutoStop   ChargeMode = 4 // 充满自停
)

// StandardResponse API标准响应
type StandardResponse struct {
	Code      int         `json:"code"`
	Message   string      `json:"message"`
	Data      interface{} `json:"data,omitempty"`
	RequestID string      `json:"request_id"`
	Timestamp int64       `json:"timestamp"`
}

// DeviceInfo 设备信息
type DeviceInfo struct {
	DeviceDBID   int          `json:"device_db_id"`
	DeviceID     string       `json:"device_id"` // 设备物理ID
	Name         string       `json:"name"`
	Model        string       `json:"model"`
	Online       bool         `json:"online"`
	Status       string       `json:"status"`
	LastSeenAt   int64        `json:"last_seen_at"`  // Unix 时间戳
	RegisteredAt int64        `json:"registered_at"` // Unix 时间戳
	ActiveOrder  *ActiveOrder `json:"active_order"`
	Ports        []PortInfo   `json:"ports"`
	CreatedAt    int64        `json:"created_at"` // Unix 时间戳
	UpdatedAt    int64        `json:"updated_at"` // Unix 时间戳
}

// PhysicalID 返回设备物理ID（兼容方法）
func (d *DeviceInfo) PhysicalID() string {
	return d.DeviceID
}

// ActiveOrder 活跃订单
type ActiveOrder struct {
	OrderNo string `json:"order_no"`
	PortNo  int    `json:"port_no"`
	Status  string `json:"status"`
}

// PortInfo 端口信息
type PortInfo struct {
	PortNo int    `json:"port_no"`
	Status string `json:"status"`
	InUse  bool   `json:"in_use"`
}

// OrderInfo 订单详情
type OrderInfo struct {
	OrderNo         string      `json:"order_no"`
	DeviceID        int         `json:"device_id"` // 数据库ID
	PhysicalID      string      `json:"physical_id"`
	PortNo          int         `json:"port_no"`
	Status          OrderStatus `json:"status"`
	ChargeMode      ChargeMode  `json:"charge_mode"`
	Amount          int         `json:"amount"`           // 金额（分）
	DurationMinutes int         `json:"duration_minutes"` // 时长（分钟）
	Power           int         `json:"power"`            // 功率（瓦）
	PricePerKwh     int         `json:"price_per_kwh"`    // 电价（分/度）
	ServiceFee      int         `json:"service_fee"`      // 服务费率（千分比）
	EnergyConsumed  float64     `json:"energy_consumed"`  // 已消耗电量（度）
	ActualAmount    int         `json:"actual_amount"`    // 实际金额（分）
	StartTime       int64       `json:"start_time"`       // 开始时间（Unix时间戳）
	EndTime         int64       `json:"end_time"`         // 结束时间（Unix时间戳）
	CreatedAt       int64       `json:"created_at"`       // Unix时间戳
	UpdatedAt       int64       `json:"updated_at"`       // Unix时间戳
}

// StartChargeRequest 启动充电请求
type StartChargeRequest struct {
	PortNo          int        `json:"port_no"`
	ChargeMode      ChargeMode `json:"charge_mode"`
	Amount          int        `json:"amount"`
	DurationMinutes int        `json:"duration_minutes,omitempty"`
	Power           int        `json:"power,omitempty"`
	PricePerKwh     int        `json:"price_per_kwh,omitempty"`
	ServiceFee      int        `json:"service_fee,omitempty"`
}

// ChargeResponse 启动充电响应
type ChargeResponse struct {
	OrderNo string `json:"order_no"`
}

// StopChargeRequest 停止充电请求
type StopChargeRequest struct {
	PortNo int `json:"port_no"`
}

// ListDevicesParams 设备列表查询参数
type ListDevicesParams struct {
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
	Online   *bool `json:"online,omitempty"`
}

// ListOrdersParams 订单列表查询参数
type ListOrdersParams struct {
	Page      int          `json:"page"`
	PageSize  int          `json:"page_size"`
	DeviceID  string       `json:"device_id,omitempty"`
	Status    *OrderStatus `json:"status,omitempty"`
	StartTime *time.Time   `json:"start_time,omitempty"`
	EndTime   *time.Time   `json:"end_time,omitempty"`
}

// ErrorConflict 端口冲突错误
type ErrorConflict struct {
	CurrentOrder string `json:"current_order"`
	PortNo       int    `json:"port_no"`
}
