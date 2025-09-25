package parser

import (
	"time"
)

// Frame 表示解析后的协议帧
type Frame struct {
	// 基础帧结构
	Header    uint16 `json:"header"`    // 包头 fcfe/fcff
	Length    uint16 `json:"length"`    // 包长
	Command   uint16 `json:"command"`   // 命令码
	Sequence  uint32 `json:"sequence"`  // 帧流水号
	Direction uint8  `json:"direction"` // 数据方向 0x00下行/0x01上行
	GatewayID string `json:"gateway_id"`// 网关ID
	
	// 数据载荷
	Payload []byte `json:"payload"` // 协议载荷
	
	// 校验信息
	Checksum         uint8 `json:"checksum"`          // 校验和
	CalculatedSum    uint8 `json:"calculated_sum"`    // 计算的校验和
	ChecksumValid    bool  `json:"checksum_valid"`    // 校验和是否正确
	Tail             uint16`json:"tail"`              // 包尾 fcee
	
	// 解析状态
	ParsedAt         time.Time `json:"parsed_at"`     // 解析时间
	FrameType        string    `json:"frame_type"`    // 帧类型描述
	Valid            bool      `json:"valid"`         // 帧是否有效
	Errors           []string  `json:"errors"`        // 解析错误
}

// BKVPayload BKV协议载荷
type BKVPayload struct {
	Command      uint16            `json:"command"`       // BKV命令
	Sequence     uint64            `json:"sequence"`      // BKV序列号
	GatewayID    string            `json:"gateway_id"`    // 网关ID
	Fields       map[uint8][]byte  `json:"fields"`        // TLV字段
	SocketNo     uint8             `json:"socket_no"`     // 插座号
	PortNo       uint8             `json:"port_no"`       // 插孔号
}

// FrameParser 协议帧解析器接口
type FrameParser interface {
	// Parse 解析协议帧
	Parse(data []byte) (*Frame, error)
	
	// ValidateFrame 验证帧结构
	ValidateFrame(frame *Frame) error
	
	// CalculateChecksum 计算校验和
	CalculateChecksum(data []byte) uint8
	
	// GetFrameType 获取帧类型
	GetFrameType(frame *Frame) string
}

// TLVParser TLV解析器接口
type TLVParser interface {
	// ParseTLV 解析TLV结构
	ParseTLV(data []byte) (map[uint8][]byte, error)
	
	// EncodeTLV 编码TLV结构
	EncodeTLV(fields map[uint8][]byte) ([]byte, error)
	
	// GetFieldValue 获取字段值
	GetFieldValue(fields map[uint8][]byte, tag uint8) ([]byte, bool)
}

// BKVParser BKV协议解析器接口
type BKVParser interface {
	// ParseBKV 解析BKV载荷
	ParseBKV(data []byte) (*BKVPayload, error)
	
	// ValidateBKV 验证BKV载荷
	ValidateBKV(payload *BKVPayload) error
	
	// GetBKVType 获取BKV类型
	GetBKVType(payload *BKVPayload) string
}

// ProtocolType 协议类型
type ProtocolType int

const (
	ProtocolTypeUnknown ProtocolType = iota
	ProtocolTypeBKV     // BKV子协议
	ProtocolTypeGN      // GN组网协议
)

// CommandType 命令类型常量
const (
	// 基础命令
	CmdHeartbeat      = 0x0000 // 心跳
	CmdStatusQuery    = 0x0015 // 状态查询
	CmdNetworkConfig  = 0x0005 // 网络配置
	CmdControl        = 0x0015 // 控制命令
	CmdOTA            = 0x0007 // OTA升级
	
	// BKV命令
	BKVCmdStatusReport    = 0x1017 // 状态上报
	BKVCmdControlDevice   = 0x1007 // 控制设备
	BKVCmdChargingEnd     = 0x1004 // 充电结束
	BKVCmdCardCharging    = 0x100B // 刷卡充电
	BKVCmdBalanceQuery    = 0x101A // 余额查询
	BKVCmdParamSet        = 0x1011 // 参数设置
	BKVCmdParamQuery      = 0x1012 // 参数查询
	BKVCmdExceptionEvent  = 0x1010 // 异常事件
)

// FrameDirection 帧方向
const (
	DirectionDownlink = 0x00 // 下行（服务器->设备）
	DirectionUplink   = 0x01 // 上行（设备->服务器）
)

// FrameHeaders 帧头常量
const (
	HeaderDownlink = 0xFCFF // 下行帧头
	HeaderUplink   = 0xFCFE // 上行帧头
	FrameTail      = 0xFCEE // 帧尾
)

// ParseResult 解析结果
type ParseResult struct {
	Frame     *Frame      `json:"frame"`
	BKV       *BKVPayload `json:"bkv,omitempty"`
	Protocol  ProtocolType`json:"protocol"`
	Success   bool        `json:"success"`
	Errors    []string    `json:"errors"`
	Warnings  []string    `json:"warnings"`
}

// ValidationRule 验证规则
type ValidationRule struct {
	Name        string      `json:"name"`
	Field       string      `json:"field"`
	Type        string      `json:"type"`        // equals, range, pattern, function
	Expected    interface{} `json:"expected"`
	Min         *int64      `json:"min,omitempty"`
	Max         *int64      `json:"max,omitempty"`
	Pattern     string      `json:"pattern,omitempty"`
	Function    string      `json:"function,omitempty"`
	Required    bool        `json:"required"`
}