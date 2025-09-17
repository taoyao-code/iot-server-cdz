package bkv

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
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
	Cmd        uint16     // BKV命令 (如0x1017)
	FrameSeq   uint64     // 设备上报帧序列号 (8字节)
	GatewayID  string     // 网关ID
	Fields     []TLVField // TLV字段列表
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
	SocketNo      uint8  // 插座序号
	SoftwareVer   uint16 // 软件版本
	Temperature   uint8  // 温度
	RSSI          uint8  // 信号强度
	PortA         *PortStatus
	PortB         *PortStatus
}

// PortStatus 插孔状态信息

// BKV协议控制指令详细结构定义
// 基于设备对接指引-组网设备2024(1).txt协议文档

// ChargingMode 充电模式
type ChargingMode uint8

const (
	ChargingModeByTime  ChargingMode = 1 // 按时长充电
	ChargingModeByPower ChargingMode = 0 // 按电量充电  
	ChargingModeByLevel ChargingMode = 3 // 按功率充电
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
	SocketNo     uint8        // 插座号
	Port         PortType     // 插孔号 (0=A孔, 1=B孔)
	Switch       SwitchState  // 开关状态 (1=开, 0=关)
	Mode         ChargingMode // 充电模式 (1=按时, 0=按量, 3=按功率)
	Duration     uint16       // 充电时长，单位分钟(1-900分钟)
	Energy       uint16       // 充电电量，单位wh
	BusinessNo   uint16       // 业务号
	
	// 按功率充电专用字段
	PaymentAmount uint16         // 支付金额(分)
	LevelCount    uint8          // 挡位个数(最多5档)
	PowerLevels   []PowerLevel   // 功率挡位信息
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
	SocketNo        uint8             // 插座号
	SoftwareVer     uint16            // 插座版本
	Temperature     uint8             // 插座温度
	RSSI            uint8             // 信号强度
	Port            PortType          // 插孔号
	Status          uint8             // 插座状态位
	BusinessNo      uint16            // 业务号
	InstantPower    uint16            // 瞬时功率 (单位W，实际值/10)
	InstantCurrent  uint16            // 瞬时电流 (单位A，实际值/1000)
	EnergyUsed      uint16            // 用电量 (单位0.01kWh)
	ChargingTime    uint16            // 充电时间 (分钟)
	EndReason       ChargingEndReason // 结束原因
	
	// 按功率充电专用字段
	EndTime         string            // 结束时间 (格式: YYYYMMDDHHMISS)
	AmountSpent     uint16            // 花费金额(分)
	SettlingPower   uint16            // 结算功率(单位0.1W)
	LevelCount      uint8             // 挡位数
	LevelDurations  []uint16          // 各挡位充电时间
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
		case 0x03:
			if len(field.Value) >= 1 {
				// 根据位置判断是插座序号还是其他
				if field.Value[0] == 0x4A {
					status.SocketNo = field.Value[0]
				}
			}
		case 0x01:
			if len(field.Value) >= 4 && field.Value[0] == 0x01 {
				// 软件版本：01 3e ff ff 格式
				status.SoftwareVer = binary.BigEndian.Uint16(field.Value[1:3])
			}
		// 其他字段的解析...
		}
	}
	
	return status, nil
}

// IsHeartbeat 判断是否为心跳命令
func (p *BKVPayload) IsHeartbeat() bool {
	return p.Cmd == 0x1017
}

// IsStatusReport 判断是否为状态上报
func (p *BKVPayload) IsStatusReport() bool {
	return p.Cmd == 0x1017 // 状态上报也使用1017命令，通过0x94字段区分
}

// IsChargingEnd 判断是否为充电结束上报
func (p *BKVPayload) IsChargingEnd() bool {
	// 检查是否包含充电结束的关键字段
	for _, field := range p.Fields {
		// 0x2E: 充电结束时间
		// 0x2F: 结束原因
		// 0xA: 订单号
		if field.Tag == 0x2E || field.Tag == 0x2F {
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
		SocketNo: data[0],                                                // 插座号
		Port:     PortType(data[1]),                                      // 插孔号
		Switch:   SwitchState(data[2]),                                   // 开关状态
		Mode:     ChargingMode(data[3]),                                  // 充电模式
		Duration: binary.BigEndian.Uint16(data[4:6]),                     // 充电时长
	}

	if len(data) >= 8 {
		cmd.Energy = binary.BigEndian.Uint16(data[6:8]) // 充电电量
	}

	// 按功率充电模式有额外字段
	if cmd.Mode == ChargingModeByLevel && len(data) >= 13 {
		pos := 8
		cmd.PaymentAmount = binary.BigEndian.Uint16(data[pos:pos+2]) // 支付金额
		pos += 2
		cmd.LevelCount = data[pos] // 挡位个数
		pos++

		// 解析各挡位信息 (每个挡位6字节: 功率2+价格2+时长2)
		expectedLen := pos + int(cmd.LevelCount)*6
		if len(data) >= expectedLen {
			for i := uint8(0); i < cmd.LevelCount && i < 5; i++ { // 最多5档
				level := PowerLevel{
					Power:    binary.BigEndian.Uint16(data[pos:pos+2]),
					Price:    binary.BigEndian.Uint16(data[pos+2:pos+4]),
					Duration: binary.BigEndian.Uint16(data[pos+4:pos+6]),
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
func ParseBKVChargingEnd(data []byte) (*BKVChargingEnd, error) {
	if len(data) < 15 {
		return nil, ErrTLVShort
	}

	end := &BKVChargingEnd{
		SocketNo:       data[0],                                      // 插座号
		SoftwareVer:    binary.BigEndian.Uint16(data[1:3]),           // 软件版本
		Temperature:    data[3],                                      // 温度
		RSSI:           data[4],                                      // 信号强度
		Port:           PortType(data[5]),                            // 插孔号
		Status:         data[6],                                      // 插座状态
		BusinessNo:     binary.BigEndian.Uint16(data[7:9]),           // 业务号
		InstantPower:   binary.BigEndian.Uint16(data[9:11]),          // 瞬时功率
		InstantCurrent: binary.BigEndian.Uint16(data[11:13]),         // 瞬时电流
		EnergyUsed:     binary.BigEndian.Uint16(data[13:15]),         // 用电量
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
// 状态位格式: bit7-在线 bit6-计量正常 bit5-充电 bit4-空载 bit3-温度正常 bit2-电流正常 bit1-功率正常 bit0-预留
func deriveEndReasonFromStatus(status uint8) ChargingEndReason {
	// 检查离线 (bit7 = 0表示离线) - 最高优先级
	if (status>>7)&0x01 == 0 {
		return ReasonPowerOff
	}

	// 检查空载位 (bit4)
	if (status>>4)&0x01 == 1 {
		return ReasonNoLoad
	}
	
	// 检查温度异常 (bit3 = 0表示异常)
	if (status>>3)&0x01 == 0 {
		return ReasonOverTemp
	}
	
	// 检查电流异常 (bit2 = 0表示异常)
	if (status>>2)&0x01 == 0 {
		return ReasonOverCurrent
	}
	
	// 默认正常结束
	return ReasonNormal
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
	case ChargingModeByPower:
		return "charging_by_energy"
	case ChargingModeByLevel:
		return "charging_by_power_level"
	default:
		return "unknown"
	}
}