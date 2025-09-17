package bkv

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
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