package bkv

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/taoyao-code/iot-server/internal/coremodel"
)

var (
	ErrTLVShort   = errors.New("TLV data too short")
	ErrTLVInvalid = errors.New("invalid TLV format")
)

// TLVField BKV协议中的TLV字段
type TLVField struct {
	Tag    uint8  // 标签
	Length uint8  // 长度 (如果有)
	Value  []byte // 值
}

// BKVPayload BKV子协议数据载荷
type BKVPayload struct {
	Cmd       uint16     // BKV命令 (如0x1017)
	FrameSeq  uint64     // 设备上报帧序列号 (8字节)
	GatewayID string     // 网关ID
	Fields    []TLVField // TLV字段列表
}

// ParseBKVPayload 解析BKV子协议数据
// 重新分析格式：04 01 01 [cmd] 0a 01 02 [8-byte seq] 09 01 03 [7-byte gateway] [remaining TLV data]
func ParseBKVPayload(data []byte) (*BKVPayload, error) {
	if len(data) < 21 { // 最小长度检查: 5 + 11 + 10 = 26, but let's check step by step
		return nil, ErrTLVShort
	}

	payload := &BKVPayload{}
	pos := 0

	// 解析 04 01 01 [cmd 2bytes]
	if pos+5 > len(data) || data[pos] != 0x04 || data[pos+1] != 0x01 || data[pos+2] != 0x01 {
		return nil, ErrTLVInvalid
	}
	payload.Cmd = binary.BigEndian.Uint16(data[pos+3 : pos+5])
	pos += 5

	// 解析 0a 01 02 [frameSeq 8bytes]
	if pos+11 > len(data) || data[pos] != 0x0a || data[pos+1] != 0x01 || data[pos+2] != 0x02 {
		return nil, ErrTLVInvalid
	}
	payload.FrameSeq = binary.BigEndian.Uint64(data[pos+3 : pos+11])
	pos += 11

	// 解析 09 01 03 [gatewayID 7bytes]
	if pos+10 > len(data) || data[pos] != 0x09 || data[pos+1] != 0x01 || data[pos+2] != 0x03 {
		return nil, ErrTLVInvalid
	}
	payload.GatewayID = hex.EncodeToString(data[pos+3 : pos+10])
	pos += 10

	// 剩余部分按照实际的TLV格式处理
	// 从调试信息看，下一个应该是 65 01 94 (tag=65, len=01, value=94)
	for pos < len(data) {
		if pos+2 > len(data) {
			break
		}

		tag := data[pos]
		length := data[pos+1]
		pos += 2

		if pos+int(length) > len(data) {
			break
		}

		value := data[pos : pos+int(length)]
		pos += int(length)

		field := TLVField{
			Tag:    tag,
			Length: length,
			Value:  value,
		}

		payload.Fields = append(payload.Fields, field)
	}

	return payload, nil
}

// SocketStatus 插座状态信息
type SocketStatus struct {
	SocketNo    uint8  // 插座序号
	SoftwareVer uint16 // 软件版本
	Temperature uint8  // 温度
	RSSI        uint8  // 信号强度
	PortA       *PortStatus
	PortB       *PortStatus
}

// PortStatus 插孔状态信息

// BKV协议控制指令详细结构定义
// 基于设备对接指引-组网设备2024(1).txt协议文档

// ChargingMode 充电模式
type ChargingMode uint8

const (
	ChargingModeByTime   ChargingMode = 1 // 按时长充电
	ChargingModeByEnergy ChargingMode = 0 // 按电量充电
	ChargingModeByLevel  ChargingMode = 3 // 按功率充电
)

// SwitchState 开关状态
type SwitchState uint8

const (
	SwitchOff SwitchState = 0 // 关闭
	SwitchOn  SwitchState = 1 // 开启
)

// PortType 插孔类型
type PortType uint8

const (
	PortA PortType = 0 // A孔
	PortB PortType = 1 // B孔
)

// BKVControlCommand BKV控制指令结构（cmd=0x0015）
type BKVControlCommand struct {
	SocketNo   uint8        // 插座号
	Port       PortType     // 插孔号 (0=A孔, 1=B孔)
	Switch     SwitchState  // 开关状态 (1=开, 0=关)
	Mode       ChargingMode // 充电模式 (1=按时, 0=按量, 3=按功率)
	Duration   uint16       // 充电时长，单位分钟(1-900分钟)
	Energy     uint16       // 充电电量，单位wh
	BusinessNo uint16       // 业务号

	// 按功率充电专用字段
	PaymentAmount uint16       // 支付金额(分)
	LevelCount    uint8        // 挡位个数(最多5档)
	PowerLevels   []PowerLevel // 功率挡位信息
}

// PowerLevel 功率挡位信息（按功率充电模式）
type PowerLevel struct {
	Power    uint16 // 功率值(W)
	Price    uint16 // 价格(分)
	Duration uint16 // 时长(分钟)
}

// ChargingEndReason 充电结束原因
type ChargingEndReason uint8

const (
	ReasonNormal      ChargingEndReason = 0 // 正常结束
	ReasonUserStop    ChargingEndReason = 1 // 用户停止
	ReasonOverCurrent ChargingEndReason = 2 // 过流保护
	ReasonOverTemp    ChargingEndReason = 3 // 过温保护
	ReasonPowerOff    ChargingEndReason = 4 // 断电/异常
	ReasonNoLoad      ChargingEndReason = 8 // 空载结束（从状态位推导）
)

// BKVChargingEnd 充电结束上报结构（cmd=0x0015方向为设备->平台）
type BKVChargingEnd struct {
	SocketNo       uint8             // 插座号
	SoftwareVer    uint16            // 插座版本
	Temperature    uint8             // 插座温度
	RSSI           uint8             // 信号强度
	Port           PortType          // 插孔号
	Status         uint8             // 插座状态位
	BusinessNo     uint16            // 业务号
	InstantPower   uint16            // 瞬时功率 (单位W，实际值/10)
	InstantCurrent uint16            // 瞬时电流 (单位A，实际值/1000)
	EnergyUsed     uint16            // 用电量 (单位0.01kWh)
	ChargingTime   uint16            // 充电时间 (分钟)
	EndReason      ChargingEndReason // 结束原因

	// 按功率充电专用字段
	EndTime        string   // 结束时间 (格式: YYYYMMDDHHMISS)
	AmountSpent    uint16   // 花费金额(分)
	SettlingPower  uint16   // 结算功率(单位0.1W)
	LevelCount     uint8    // 挡位数
	LevelDurations []uint16 // 各挡位充电时间
}

// CardChargingRequest 刷卡充电请求（BKV子协议）
type CardChargingRequest struct {
	SocketNo      uint8    // 插座号
	Port          PortType // 插孔号
	BusinessNo    uint16   // 业务号
	Status        uint8    // 插座状态
	CardNo        []byte   // 卡号（6字节）
	OfflineParams []byte   // 离线卡参数（补0）
}

// CardChargingEnd 刷卡充电结束（BKV子协议）
type CardChargingEnd struct {
	SocketNo       uint8    // 插座号
	SoftwareVer    uint16   // 软件版本
	Temperature    uint8    // 温度
	RSSI           uint8    // 信号强度
	Port           PortType // 插孔号
	Status         uint8    // 插座状态
	BusinessNo     uint16   // 业务号
	InstantPower   uint16   // 瞬时功率
	InstantCurrent uint16   // 瞬时电流
	EnergyUsed     uint16   // 用电量
	ChargingTime   uint16   // 充电时间
	CardNo         []byte   // 卡号
	OnlineCard     bool     // 在线卡标志
	BillingMode    uint8    // 计费模式 (1=按时, 2=按量, 3=按功率)
	AmountSpent    uint16   // 花费金额（仅按功率）
	SettlingPower  uint16   // 结算功率（仅按功率）
	LevelCount     uint8    // 挡位数（仅按功率）
	LevelDurations []uint16 // 各挡位时间（仅按功率）
}

// ExceptionEvent 异常事件上报（BKV子协议）
type ExceptionEvent struct {
	SocketNo            uint8  // 插座号
	SocketEventReason   uint8  // 插座事件原因
	SocketEventStatus   uint8  // 插座事件状态
	Port1EventReason    uint8  // 插孔1事件原因
	Port1EventStatus    uint8  // 插孔1事件状态
	Port2EventReason    uint8  // 插孔2事件原因
	Port2EventStatus    uint8  // 插孔2事件状态
	OverVoltage         uint16 // 过压值
	UnderVoltage        uint16 // 欠压值
	Port1LeakCurrent    uint16 // 插孔1漏电流
	Port2LeakCurrent    uint16 // 插孔2漏电流
	Port1OverTemp       uint8  // 插孔1过温值
	Port2OverTemp       uint8  // 插孔2过温值
	Port1ChargingStatus uint8  // 插孔1充电状态
	Port2ChargingStatus uint8  // 插孔2充电状态
}

// ParameterQuery 参数查询（BKV子协议）
type ParameterQuery struct {
	SocketNo             uint8  // 插座号
	FullPowerThreshold   uint16 // 充满功率阈值（单位0.1W）
	TrickleThreshold     uint8  // 涓流阈值（单位%）
	FullContinueTime     uint16 // 充满续充时间（单位s）
	NoLoadPowerThreshold uint16 // 空载功率阈值（单位0.1W）
	NoLoadDelayTime      uint16 // 空载延时时间（单位s）
	MaxChargingTime      uint16 // 最大充电时间（单位min）
	HighTempThreshold    uint8  // 高温阈值
	PowerLimit           uint16 // 功率限值（单位0.1W）
	OverCurrentLimit     uint16 // 过流限值（单位0.001A）
	KeyBaseAmount        uint16 // 按键基础金额
	AntiPulseTime        uint16 // 防脉冲时间
}

// PortStatus 插孔状态信息
type PortStatus struct {
	PortNo       uint8  // 插孔号
	Status       uint8  // 插座状态
	BusinessNo   uint16 // 业务号
	Voltage      uint16 // 电压 (单位0.1V)
	Power        uint16 // 瞬时功率 (单位0.1W)
	Current      uint16 // 瞬时电流 (单位0.001A)
	Energy       uint16 // 用电量 (单位0.01kWh)
	ChargingTime uint16 // 充电时间 (分钟)
}

// 注：IsCharging/IsIdle/IsOnline 方法已迁移到 coremodel.RawPortStatus
// 使用示例: coremodel.RawPortStatus(port.Status).IsCharging()

// ParseSocketStatus 解析插座状态上报 (0x94字段)
func ParseSocketStatus(data []byte) (*SocketStatus, error) {
	if len(data) < 10 {
		return nil, ErrTLVShort
	}

	status := &SocketStatus{}
	pos := 0

	// 解析TLV字段
	for pos < len(data) {
		if pos+2 >= len(data) {
			break
		}

		tag := data[pos]
		length := data[pos+1]
		pos += 2

		if pos+int(length) > len(data) {
			break
		}

		value := data[pos : pos+int(length)]
		pos += int(length)

		switch tag {
		case 0x4A: // 插座序号
			if length >= 1 {
				status.SocketNo = value[0]
			}
		case 0x3E: // 软件版本
			if length >= 2 {
				status.SoftwareVer = binary.BigEndian.Uint16(value[0:2])
			}
		case 0x07: // 温度
			if length >= 1 {
				status.Temperature = value[0]
			}
		case 0x96: // RSSI
			if length >= 1 {
				status.RSSI = value[0]
			}
		case 0x5B: // 插孔属性
			port := parsePortStatus(value)
			if port != nil {
				if port.PortNo == 0 {
					status.PortA = port
				} else if port.PortNo == 1 {
					status.PortB = port
				}
			}
		}
	}

	return status, nil
}

// parsePortStatus 解析插孔状态
func parsePortStatus(data []byte) *PortStatus {
	if len(data) < 20 {
		return nil
	}

	port := &PortStatus{}
	pos := 0

	for pos < len(data) {
		if pos+2 >= len(data) {
			break
		}

		tag := data[pos]
		length := data[pos+1]
		pos += 2

		if pos+int(length) > len(data) {
			break
		}

		value := data[pos : pos+int(length)]
		pos += int(length)

		switch tag {
		case 0x08: // 插孔号
			if length >= 1 {
				port.PortNo = value[0]
			}
		case 0x09: // 插座状态
			if length >= 1 {
				port.Status = value[0]
			}
		case 0x0A: // 业务号
			if length >= 2 {
				port.BusinessNo = binary.BigEndian.Uint16(value[0:2])
			}
		case 0x95: // 电压
			if length >= 2 {
				port.Voltage = binary.BigEndian.Uint16(value[0:2])
			}
		case 0x0B: // 瞬时功率
			if length >= 2 {
				port.Power = binary.BigEndian.Uint16(value[0:2])
			}
		case 0x0C: // 瞬时电流
			if length >= 2 {
				port.Current = binary.BigEndian.Uint16(value[0:2])
			}
		case 0x0D: // 用电量
			if length >= 2 {
				port.Energy = binary.BigEndian.Uint16(value[0:2])
			}
		case 0x0E: // 充电时间
			if length >= 2 {
				port.ChargingTime = binary.BigEndian.Uint16(value[0:2])
			}
		}
	}

	return port
}

// GetSocketStatus 从BKV载荷中解析插座状态
func (p *BKVPayload) GetSocketStatus() (*SocketStatus, error) {
	for _, field := range p.Fields {
		if field.Tag == 0x65 && len(field.Value) > 0 && field.Value[0] == 0x94 {
			// 找到插座状态标识，后续的字段都是插座状态相关的TLV
			return parseSocketStatusFields(p.Fields)
		}
	}
	return nil, errors.New("no socket status found")
}

// parseSocketStatusFields 从所有字段中解析插座状态
func parseSocketStatusFields(fields []TLVField) (*SocketStatus, error) {
	status := &SocketStatus{}

	for _, field := range fields {
		switch field.Tag {
		case 0x4A: // 插座序号
			if len(field.Value) >= 1 {
				status.SocketNo = field.Value[0]
			}
		case 0x3E: // 软件版本
			if len(field.Value) >= 2 {
				status.SoftwareVer = binary.BigEndian.Uint16(field.Value[0:2])
			}
		case 0x07: // 温度
			if len(field.Value) >= 1 {
				status.Temperature = field.Value[0]
			}
		case 0x96: // RSSI信号强度
			if len(field.Value) >= 1 {
				status.RSSI = field.Value[0]
			}
		case 0x28: // 端口信息容器
			// Value 应该包含 Tag 0x5B 和端口数据
			if len(field.Value) >= 1 && field.Value[0] == 0x5B {
				// 解析端口数据（跳过 0x5B 标识字节）
				port := parsePortStatusFromFields(fields, field)
				if port != nil {
					if port.PortNo == 0 {
						status.PortA = port
					} else if port.PortNo == 1 {
						status.PortB = port
					}
				}
			}
		}
	}

	// 如果通过 0x28 没有解析到端口，尝试从 0x08 字段直接解析
	if status.PortA == nil && status.PortB == nil {
		status.PortA, status.PortB = parsePortsFromFields(fields)
	}

	return status, nil
}

// parsePortStatusFromFields 从字段列表中解析单个端口状态
// 查找 0x28 标记后的端口相关字段
func parsePortStatusFromFields(fields []TLVField, containerField TLVField) *PortStatus {
	port := &PortStatus{}
	found := false

	// 找到容器字段在列表中的位置
	containerIdx := -1
	for i, f := range fields {
		if f.Tag == containerField.Tag && len(f.Value) > 0 && f.Value[0] == containerField.Value[0] {
			containerIdx = i
			break
		}
	}

	if containerIdx < 0 {
		return nil
	}

	// 解析容器后面的端口字段（直到下一个 0x28 或结束）
	for i := containerIdx + 1; i < len(fields); i++ {
		f := fields[i]
		if f.Tag == 0x28 { // 下一个端口容器
			break
		}

		switch f.Tag {
		case 0x08: // 插孔号
			if len(f.Value) >= 1 {
				portNo := f.Value[0]
				// 只接受有效端口号 (0=A孔, 1=B孔)
				if portNo > 1 {
					return nil // 无效端口号
				}
				port.PortNo = portNo
				found = true
			}
		case 0x09: // 状态
			if len(f.Value) >= 1 {
				port.Status = f.Value[0]
			}
		case 0x0A: // 业务号
			if len(f.Value) >= 2 {
				port.BusinessNo = binary.BigEndian.Uint16(f.Value[0:2])
			}
		case 0x95: // 电压（某些协议版本）或功率
			if len(f.Value) >= 2 {
				port.Voltage = binary.BigEndian.Uint16(f.Value[0:2])
			}
		case 0x0B: // 瞬时功率
			if len(f.Value) >= 2 {
				port.Power = binary.BigEndian.Uint16(f.Value[0:2])
			}
		case 0x0C: // 瞬时电流
			if len(f.Value) >= 2 {
				port.Current = binary.BigEndian.Uint16(f.Value[0:2])
			}
		case 0x0D: // 用电量
			if len(f.Value) >= 2 {
				port.Energy = binary.BigEndian.Uint16(f.Value[0:2])
			}
		case 0x0E: // 充电时间
			if len(f.Value) >= 2 {
				port.ChargingTime = binary.BigEndian.Uint16(f.Value[0:2])
			}
		}
	}

	if !found {
		return nil
	}
	return port
}

// parsePortsFromFields 直接从字段列表解析端口（兼容不同协议格式）
// BKV嵌套格式：Tag=0x03/0x04 表示类型指示符，Value[0]是实际字段标签，后续字段包含数据
func parsePortsFromFields(fields []TLVField) (*PortStatus, *PortStatus) {
	var portA, portB *PortStatus
	var currentPort *PortStatus
	portAStarted := false
	portBStarted := false

	// 用于处理嵌套格式的临时变量
	var pendingTag uint8
	var pendingIsSingle bool // true=0x03单字节, false=0x04多字节

	for _, f := range fields {
		// 处理嵌套格式：0x03=单字节值，0x04=多字节值
		if f.Tag == 0x03 && len(f.Value) >= 1 {
			pendingTag = f.Value[0]
			pendingIsSingle = true
			continue
		}
		if f.Tag == 0x04 && len(f.Value) >= 1 {
			pendingTag = f.Value[0]
			pendingIsSingle = false
			continue
		}

		// 如果有待处理的嵌套标签，当前字段的Tag本身可能是数据值
		if pendingTag != 0 && currentPort != nil {
			// 在嵌套格式中，紧跟在 03/04 01 [tag] 之后的字节是值
			// 但TLV解析器把它当作了新的Tag，所以我们用f.Tag作为值
			var dataValue []byte
			if pendingIsSingle {
				// 单字节值：f.Tag本身就是值
				dataValue = []byte{f.Tag}
			} else {
				// 多字节值：f.Tag是高字节，f.Length是低字节（被误解析为Tag和Length）
				dataValue = []byte{f.Tag, f.Length}
			}

			if len(dataValue) > 0 {
				switch pendingTag {
				case 0x08: // 插孔号
					portNo := dataValue[0]
					if portNo <= 1 {
						currentPort.PortNo = portNo
						if portNo == 0 {
							portA = currentPort
						} else {
							portB = currentPort
						}
					}
				case 0x09: // 状态
					currentPort.Status = dataValue[0]
				case 0x0A: // 业务号
					if len(dataValue) >= 2 {
						currentPort.BusinessNo = binary.BigEndian.Uint16(dataValue[0:2])
					}
				case 0x95: // 电压
					if len(dataValue) >= 2 {
						currentPort.Voltage = binary.BigEndian.Uint16(dataValue[0:2])
					}
				case 0x0B: // 瞬时功率
					if len(dataValue) >= 2 {
						currentPort.Power = binary.BigEndian.Uint16(dataValue[0:2])
					}
				case 0x0C: // 瞬时电流
					if len(dataValue) >= 2 {
						currentPort.Current = binary.BigEndian.Uint16(dataValue[0:2])
					}
				case 0x0D: // 用电量
					if len(dataValue) >= 2 {
						currentPort.Energy = binary.BigEndian.Uint16(dataValue[0:2])
					}
				case 0x0E: // 充电时间
					if len(dataValue) >= 2 {
						currentPort.ChargingTime = binary.BigEndian.Uint16(dataValue[0:2])
					}
				}
			}

			// 检查f.Value中是否有打包的后续字段
			// TLV解析器错误地将多个嵌套字段打包到同一个Value中
			// 格式: [前缀字节, 标签, 值, 前缀字节, 标签, 值, ...]
			if pendingIsSingle && len(f.Value) >= 3 {
				// 尝试提取打包的字段
				for j := 0; j+1 < len(f.Value); {
					// 跳过前缀字节 (通常是 01)
					if f.Value[j] == 0x01 || f.Value[j] == 0x03 || f.Value[j] == 0x04 {
						j++
						continue
					}

					tag := f.Value[j]
					value := f.Value[j+1]

					switch tag {
					case 0x09: // 状态
						currentPort.Status = value
					case 0x08: // 端口号
						if value <= 1 {
							currentPort.PortNo = value
						}
					}
					j += 2
				}
			}

			pendingTag = 0
			continue
		}

		// 标准格式处理
		switch f.Tag {
		case 0x28: // 端口容器开始
			if !portAStarted {
				portA = &PortStatus{}
				currentPort = portA
				portAStarted = true
			} else if !portBStarted {
				portB = &PortStatus{}
				currentPort = portB
				portBStarted = true
			}
		case 0x08: // 插孔号（标准格式）
			if currentPort != nil && len(f.Value) >= 1 {
				portNo := f.Value[0]
				if portNo > 1 {
					continue
				}
				currentPort.PortNo = portNo
				if portNo == 0 {
					portA = currentPort
				} else if portNo == 1 {
					portB = currentPort
				}
			}
		case 0x09: // 状态（标准格式）
			if currentPort != nil && len(f.Value) >= 1 {
				currentPort.Status = f.Value[0]
			}
		case 0x0A: // 业务号（标准格式）
			if currentPort != nil && len(f.Value) >= 2 {
				currentPort.BusinessNo = binary.BigEndian.Uint16(f.Value[0:2])
			}
		case 0x95: // 电压/功率
			if currentPort != nil && len(f.Value) >= 2 {
				currentPort.Voltage = binary.BigEndian.Uint16(f.Value[0:2])
			}
		case 0x0B: // 瞬时功率
			if currentPort != nil && len(f.Value) >= 2 {
				currentPort.Power = binary.BigEndian.Uint16(f.Value[0:2])
			}
		case 0x0C: // 瞬时电流
			if currentPort != nil && len(f.Value) >= 2 {
				currentPort.Current = binary.BigEndian.Uint16(f.Value[0:2])
			}
		case 0x0D: // 用电量
			if currentPort != nil && len(f.Value) >= 2 {
				currentPort.Energy = binary.BigEndian.Uint16(f.Value[0:2])
			}
		case 0x0E: // 充电时间
			if currentPort != nil && len(f.Value) >= 2 {
				currentPort.ChargingTime = binary.BigEndian.Uint16(f.Value[0:2])
			}
		}
	}

	return portA, portB
}

// IsHeartbeat 判断是否为心跳命令
func (p *BKVPayload) IsHeartbeat() bool {
	return p.Cmd == 0x1017
}

// IsStatusReport 判断是否为状态上报
// 协议规范（BKV子命令码，非帧命令码）：
//
//	0x1017 = 插座状态上报（包含 tag=0x65 + value=0x94）
//	0x1013 = 参数设置ACK（不包含状态字段！）
//	0x1004 = BKV充电结束上报（见 IsChargingEnd）
//
// 注意：仅0x1017是状态上报命令，0x1013是参数设置ACK，不应包含在此
// 注意：0x0018是帧层命令码（按功率充电结束），与BKV子命令码0x1004不同！
func (p *BKVPayload) IsStatusReport() bool {
	return p.Cmd == 0x1017
}

// HasSocketStatusFields 检查载荷是否包含插座状态字段（tag 0x65 + value 0x94）
// 这是一种更可靠的检测方式，不依赖 BKV Cmd
func (p *BKVPayload) HasSocketStatusFields() bool {
	for _, field := range p.Fields {
		if field.Tag == 0x65 && len(field.Value) > 0 && field.Value[0] == 0x94 {
			return true
		}
	}
	return false
}

// IsChargingEnd 判断是否为充电结束上报
func (p *BKVPayload) IsChargingEnd() bool {
	return p.Cmd == 0x1004 // BKV充电结束上报使用1004命令
}

// IsControlCommand 判断是否为BKV控制命令
func (p *BKVPayload) IsControlCommand() bool {
	return p.Cmd == 0x1007 // BKV控制命令使用1007命令
}

// IsExceptionReport 判断是否为异常事件上报
func (p *BKVPayload) IsExceptionReport() bool {
	return p.Cmd == 0x1010 // 异常事件上报使用1010命令
}

// IsParameterQuery 判断是否为参数查询
func (p *BKVPayload) IsParameterQuery() bool {
	return p.Cmd == 0x1012 // 参数查询使用1012命令
}

// IsCardCharging 判断是否为刷卡充电相关
func (p *BKVPayload) IsCardCharging() bool {
	// 检查是否包含刷卡相关字段（卡号、余额等）
	for _, field := range p.Fields {
		// 检查常见的刷卡字段标签
		if field.Tag == 0x68 { // 余额相关
			return true
		}
	}
	return false
}

// ParseBKVControlCommand 解析BKV控制指令（0x0015）
// 根据协议文档支持按时/按量/按功率三种模式
func ParseBKVControlCommand(data []byte) (*BKVControlCommand, error) {
	if len(data) < 6 {
		return nil, ErrTLVShort
	}

	cmd := &BKVControlCommand{
		SocketNo: data[0],                            // 插座号
		Port:     PortType(data[1]),                  // 插孔号
		Switch:   SwitchState(data[2]),               // 开关状态
		Mode:     ChargingMode(data[3]),              // 充电模式
		Duration: binary.BigEndian.Uint16(data[4:6]), // 充电时长
	}

	if len(data) >= 8 {
		cmd.Energy = binary.BigEndian.Uint16(data[6:8]) // 充电电量
	}

	// 按功率充电模式有额外字段
	if cmd.Mode == ChargingModeByLevel && len(data) >= 13 {
		pos := 8
		cmd.PaymentAmount = binary.BigEndian.Uint16(data[pos : pos+2]) // 支付金额
		pos += 2
		cmd.LevelCount = data[pos] // 挡位个数
		pos++

		// 解析各挡位信息 (每个挡位6字节: 功率2+价格2+时长2)
		expectedLen := pos + int(cmd.LevelCount)*6
		if len(data) >= expectedLen {
			for i := uint8(0); i < cmd.LevelCount && i < 5; i++ { // 最多5档
				level := PowerLevel{
					Power:    binary.BigEndian.Uint16(data[pos : pos+2]),
					Price:    binary.BigEndian.Uint16(data[pos+2 : pos+4]),
					Duration: binary.BigEndian.Uint16(data[pos+4 : pos+6]),
				}
				cmd.PowerLevels = append(cmd.PowerLevels, level)
				pos += 6
			}
		}
	}

	return cmd, nil
}

// ParseBKVChargingEnd 解析BKV充电结束上报
// 支持普通充电结束和按功率充电结束两种格式
//
// 兼容两种输入：
//  1. 直接从插座号开始的裸数据：02 50 36 30 20 00 98 00 68 ...
//  2. 0x0015 控制帧中的 data 段：0011 02 02 50 36 30 20 00 98 00 68 ...
//     前 2 字节为帧长，后跟 1 字节子命令(0x02/0x18)，再是插座号等字段。
func ParseBKVChargingEnd(data []byte) (*BKVChargingEnd, error) {
	if len(data) < 15 {
		return nil, ErrTLVShort
	}

	// 【方案三：修复长度处理逻辑】
	// 兼容 0x0015 data：0011 02 01 ffff...
	// 判定条件：
	//   - 至少包含 length(2) + cmd(1) + 最小字段(15)
	//   - 第3字节为 0x02(普通结束) 或 0x18(按功率结束)
	//   - 放宽长度匹配条件：允许尾部有额外字节（如校验和）
	//   协议示例：0x0011(17) + 2(length本身) + 1(subCmd) = 20字节基础长度
	if len(data) >= 18 {
		subCmd := data[2]
		if subCmd == 0x02 || subCmd == 0x18 {
			declLen := binary.BigEndian.Uint16(data[0:2])
			expectedMinLen := int(declLen) + 3 // length + 2字节length自身 + 1字节subCmd

			// 修复：改为大于等于判断，容忍尾部额外字节（如校验和）
			// 原逻辑：严格等于，导致有校验和时不跳过前导字节，字段错位
			// 新逻辑：只要实际长度 >= 预期最小长度，就认为格式正确
			if len(data) >= expectedMinLen && expectedMinLen >= 18 {
				data = data[3:] // 跳过 length(2字节) + subCmd(1字节)，使 data[0] 对齐为插座号
			} else if len(data) < expectedMinLen {
				// 数据长度不足，记录详细错误信息
				return nil, fmt.Errorf("data length mismatch: declared=%d, expected_min=%d, actual=%d",
					declLen, expectedMinLen, len(data))
			}
		}
	}

	if len(data) < 15 {
		return nil, ErrTLVShort
	}

	end := &BKVChargingEnd{
		SocketNo:       data[0],                              // 插座号
		SoftwareVer:    binary.BigEndian.Uint16(data[1:3]),   // 软件版本
		Temperature:    data[3],                              // 温度
		RSSI:           data[4],                              // 信号强度
		Port:           PortType(data[5]),                    // 插孔号
		Status:         data[6],                              // 插座状态
		BusinessNo:     binary.BigEndian.Uint16(data[7:9]),   // 业务号
		InstantPower:   binary.BigEndian.Uint16(data[9:11]),  // 瞬时功率
		InstantCurrent: binary.BigEndian.Uint16(data[11:13]), // 瞬时电流
		EnergyUsed:     binary.BigEndian.Uint16(data[13:15]), // 用电量
	}

	if len(data) >= 17 {
		end.ChargingTime = binary.BigEndian.Uint16(data[15:17]) // 充电时间
	}

	// 根据插座状态推导结束原因
	end.EndReason = deriveEndReasonFromStatus(end.Status)

	// 按功率充电有额外字段
	if len(data) >= 26 { // 基础17 + 结束时间7 + 结束原因1 + 金额2 = 27
		pos := 17
		if pos+7 <= len(data) {
			// 解析结束时间 (7字节: 年2+月1+日1+时1+分1+秒1)
			if data[pos] != 0 || data[pos+1] != 0 { // 非零时间
				year := binary.BigEndian.Uint16(data[pos : pos+2])
				month := data[pos+2]
				day := data[pos+3]
				hour := data[pos+4]
				minute := data[pos+5]
				second := data[pos+6]
				end.EndTime = fmt.Sprintf("%04d%02d%02d%02d%02d%02d", year, month, day, hour, minute, second)
			}
			pos += 7
		}

		if pos+1 <= len(data) {
			end.EndReason = ChargingEndReason(data[pos]) // 明确的结束原因
			pos++
		}

		if pos+2 <= len(data) {
			end.AmountSpent = binary.BigEndian.Uint16(data[pos : pos+2]) // 花费金额
			pos += 2
		}

		if pos+2 <= len(data) {
			end.SettlingPower = binary.BigEndian.Uint16(data[pos : pos+2]) // 结算功率
			pos += 2
		}

		if pos+1 <= len(data) {
			end.LevelCount = data[pos] // 挡位数
			pos++

			// 解析各挡位时间 (每个2字节)
			for i := uint8(0); i < end.LevelCount && i < 5 && pos+2 <= len(data); i++ {
				duration := binary.BigEndian.Uint16(data[pos : pos+2])
				end.LevelDurations = append(end.LevelDurations, duration)
				pos += 2
			}
		}
	}

	return end, nil
}

// deriveEndReasonFromStatus 从插座状态位推导结束原因
// 委托给 coremodel.DeriveEndReasonFromStatus 实现，并映射回 BKV 协议原因码
func deriveEndReasonFromStatus(status uint8) ChargingEndReason {
	reasonCode := coremodel.DeriveEndReasonFromStatus(coremodel.RawPortStatus(status))

	// 将 coremodel.EndReasonCode 映射回 BKV 协议的 ChargingEndReason
	switch reasonCode {
	case coremodel.ReasonCodeNormal:
		return ReasonNormal
	case coremodel.ReasonCodeUserStop:
		return ReasonUserStop
	case coremodel.ReasonCodeNoLoad:
		return ReasonNoLoad
	case coremodel.ReasonCodeOverCurrent:
		return ReasonOverCurrent
	case coremodel.ReasonCodeOverTemp:
		return ReasonOverTemp
	case coremodel.ReasonCodeOverPower:
		return ReasonOverCurrent // BKV 协议无单独过功率码，映射为过流
	case coremodel.ReasonCodePowerOff:
		return ReasonPowerOff
	case coremodel.ReasonCodeFault:
		return ReasonPowerOff // BKV 协议无单独故障码，映射为断电
	default:
		return ReasonNormal
	}
}

// GetControlCommandType 判断控制指令类型
func GetControlCommandType(data []byte) string {
	if len(data) < 4 {
		return "unknown"
	}

	mode := ChargingMode(data[3])
	switch mode {
	case ChargingModeByTime:
		return "charging_by_time"
	case ChargingModeByEnergy:
		return "charging_by_energy"
	case ChargingModeByLevel:
		return "charging_by_power_level"
	default:
		return "unknown"
	}
}

// ParseBKVExceptionEvent 解析BKV异常事件上报
func ParseBKVExceptionEvent(payload *BKVPayload) (*ExceptionEvent, error) {
	event := &ExceptionEvent{}

	for _, field := range payload.Fields {
		switch field.Tag {
		case 0x4A: // 插座号
			if len(field.Value) >= 1 {
				event.SocketNo = field.Value[0]
			}
		case 0x54: // 插座事件原因
			if len(field.Value) >= 1 {
				event.SocketEventReason = field.Value[0]
			}
		case 0x4B: // 插座事件状态
			if len(field.Value) >= 1 {
				event.SocketEventStatus = field.Value[0]
			}
		case 0x55: // 插孔1事件原因
			if len(field.Value) >= 1 {
				event.Port1EventReason = field.Value[0]
			}
		case 0x4C: // 插孔1事件状态
			if len(field.Value) >= 1 {
				event.Port1EventStatus = field.Value[0]
			}
		case 0x56: // 插孔2事件原因
			if len(field.Value) >= 1 {
				event.Port2EventReason = field.Value[0]
			}
		case 0x4D: // 插孔2事件状态
			if len(field.Value) >= 1 {
				event.Port2EventStatus = field.Value[0]
			}
		case 0x4E: // 过压值
			if len(field.Value) >= 2 {
				event.OverVoltage = binary.BigEndian.Uint16(field.Value[0:2])
			}
		case 0x4F: // 欠压值
			if len(field.Value) >= 2 {
				event.UnderVoltage = binary.BigEndian.Uint16(field.Value[0:2])
			}
		case 0x50: // 插孔1漏电流
			if len(field.Value) >= 2 {
				event.Port1LeakCurrent = binary.BigEndian.Uint16(field.Value[0:2])
			}
		case 0x51: // 插孔2漏电流
			if len(field.Value) >= 2 {
				event.Port2LeakCurrent = binary.BigEndian.Uint16(field.Value[0:2])
			}
		case 0x52: // 插孔1过温值
			if len(field.Value) >= 1 {
				event.Port1OverTemp = field.Value[0]
			}
		case 0x53: // 插孔2过温值
			if len(field.Value) >= 1 {
				event.Port2OverTemp = field.Value[0]
			}
		case 0x57: // 插孔1充电状态
			if len(field.Value) >= 1 {
				event.Port1ChargingStatus = field.Value[0]
			}
		case 0x58: // 插孔2充电状态
			if len(field.Value) >= 1 {
				event.Port2ChargingStatus = field.Value[0]
			}
		}
	}

	return event, nil
}

// ParseBKVParameterQuery 解析BKV参数查询
func ParseBKVParameterQuery(payload *BKVPayload) (*ParameterQuery, error) {
	param := &ParameterQuery{}

	for _, field := range payload.Fields {
		switch field.Tag {
		case 0x4A: // 插座号
			if len(field.Value) >= 1 {
				param.SocketNo = field.Value[0]
			}
		case 0x23: // 充满功率阈值
			if len(field.Value) >= 2 {
				param.FullPowerThreshold = binary.BigEndian.Uint16(field.Value[0:2])
			}
		case 0x60: // 涓流阈值
			if len(field.Value) >= 1 {
				param.TrickleThreshold = field.Value[0]
			}
		case 0x21: // 充满续充时间
			if len(field.Value) >= 2 {
				param.FullContinueTime = binary.BigEndian.Uint16(field.Value[0:2])
			}
		case 0x24: // 空载功率阈值
			if len(field.Value) >= 2 {
				param.NoLoadPowerThreshold = binary.BigEndian.Uint16(field.Value[0:2])
			}
		case 0x22: // 空载延时时间
			if len(field.Value) >= 2 {
				param.NoLoadDelayTime = binary.BigEndian.Uint16(field.Value[0:2])
			}
		case 0x59: // 最大充电时间
			if len(field.Value) >= 2 {
				param.MaxChargingTime = binary.BigEndian.Uint16(field.Value[0:2])
			}
		case 0x25: // 高温阈值
			if len(field.Value) >= 1 {
				param.HighTempThreshold = field.Value[0]
			}
		case 0x11: // 功率限值
			if len(field.Value) >= 2 {
				param.PowerLimit = binary.BigEndian.Uint16(field.Value[0:2])
			}
		case 0x10: // 过流限值
			if len(field.Value) >= 2 {
				param.OverCurrentLimit = binary.BigEndian.Uint16(field.Value[0:2])
			}
		case 0x68: // 按键基础金额
			if len(field.Value) >= 2 {
				param.KeyBaseAmount = binary.BigEndian.Uint16(field.Value[0:2])
			}
		case 0x93: // 防脉冲时间
			if len(field.Value) >= 2 {
				param.AntiPulseTime = binary.BigEndian.Uint16(field.Value[0:2])
			}
		}
	}

	return param, nil
}
