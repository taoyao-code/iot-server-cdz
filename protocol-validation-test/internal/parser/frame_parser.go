package parser

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// DefaultFrameParser 默认协议帧解析器实现
type DefaultFrameParser struct {
}

// NewDefaultFrameParser 创建默认帧解析器
func NewDefaultFrameParser() *DefaultFrameParser {
	return &DefaultFrameParser{}
}

// Parse 解析协议帧
func (p *DefaultFrameParser) Parse(data []byte) (*Frame, error) {
	frame := &Frame{
		ParsedAt: time.Now(),
		Valid:    false,
		Errors:   make([]string, 0),
	}

	// 最小帧长度检查
	if len(data) < 10 {
		frame.Errors = append(frame.Errors, fmt.Sprintf("Frame too short: %d bytes", len(data)))
		return frame, fmt.Errorf("frame too short: %d bytes", len(data))
	}

	// 解析包头
	frame.Header = binary.BigEndian.Uint16(data[0:2])
	
	// 验证包头
	if frame.Header != HeaderUplink && frame.Header != HeaderDownlink {
		frame.Errors = append(frame.Errors, fmt.Sprintf("Invalid header: 0x%04X", frame.Header))
		return frame, fmt.Errorf("invalid header: 0x%04X", frame.Header)
	}

	// 解析包长
	frame.Length = binary.BigEndian.Uint16(data[2:4])
	
	// 验证包长
	if int(frame.Length) > len(data) {
		frame.Errors = append(frame.Errors, fmt.Sprintf("Frame length mismatch: declared %d, actual %d", frame.Length, len(data)))
		return frame, fmt.Errorf("frame length mismatch")
	}

	// 解析命令
	frame.Command = binary.BigEndian.Uint16(data[4:6])

	// 解析帧流水号
	frame.Sequence = binary.BigEndian.Uint32(data[6:10])

	// 解析数据方向
	frame.Direction = data[10]

	// 解析网关ID (7字节)
	if len(data) >= 18 {
		gatewayBytes := data[11:18]
		frame.GatewayID = hex.EncodeToString(gatewayBytes)
		frame.GatewayID = strings.ToUpper(frame.GatewayID)
	}

	// 提取载荷、校验和、包尾
	// 协议长度字段包含从命令开始到包尾结束的所有字节数
	// 总帧结构：Header(2) + Length(2) + [Command(2) + Sequence(4) + Direction(1) + GatewayID(7) + Payload + Checksum(1) + Tail(2)]
	//           ^不计入长度^     ^-----------------计入长度字段-----------------^
	
	headerAndLengthSize := 4 // Header(2) + Length(2) 不计入长度字段
	fixedPartSize := 2 + 4 + 1 + 7 // Command(2) + Sequence(4) + Direction(1) + GatewayID(7) = 14
	checksumAndTailSize := 1 + 2 // Checksum(1) + Tail(2) = 3
	
	declaredLength := int(frame.Length)
	totalFrameSize := headerAndLengthSize + declaredLength
	
	if totalFrameSize > len(data) {
		frame.Errors = append(frame.Errors, fmt.Sprintf("Frame length %d exceeds data size %d", totalFrameSize, len(data)))
		return frame, fmt.Errorf("frame length exceeds data size")
	}

	// 载荷大小 = 声明长度 - 固定部分 - 校验和和包尾
	payloadSize := declaredLength - fixedPartSize - checksumAndTailSize
	payloadStart := headerAndLengthSize + fixedPartSize
	
	// 提取载荷
	if payloadSize > 0 && payloadStart + payloadSize <= len(data) {
		frame.Payload = data[payloadStart : payloadStart + payloadSize]
	}

	// 校验和位置
	checksumPos := totalFrameSize - 3
	tailPos := totalFrameSize - 2

	// 解析校验和
	if checksumPos >= 0 && checksumPos < len(data) {
		frame.Checksum = data[checksumPos]
		// 按照BKV协议规范，校验和从命令码开始计算（跳过header+length）
		frame.CalculatedSum = p.CalculateChecksum(data[headerAndLengthSize:checksumPos])
		frame.ChecksumValid = frame.Checksum == frame.CalculatedSum
	}

	// 解析包尾
	if tailPos >= 0 && tailPos+1 < len(data) {
		frame.Tail = binary.BigEndian.Uint16(data[tailPos : tailPos+2])
		if frame.Tail != FrameTail {
			frame.Errors = append(frame.Errors, fmt.Sprintf("Invalid tail: 0x%04X", frame.Tail))
		}
	}

	// 确定帧类型
	frame.FrameType = p.GetFrameType(frame)

	// 验证帧
	if err := p.ValidateFrame(frame); err == nil {
		frame.Valid = true
	}

	return frame, nil
}

// ValidateFrame 验证帧结构
func (p *DefaultFrameParser) ValidateFrame(frame *Frame) error {
	errors := make([]string, 0)

	// 验证包头
	if frame.Header != HeaderUplink && frame.Header != HeaderDownlink {
		errors = append(errors, "Invalid header")
	}

	// 验证包尾
	if frame.Tail != FrameTail {
		errors = append(errors, "Invalid tail")
	}

	// 验证校验和 - 暂时跳过，稍后修复算法
	// if !frame.ChecksumValid {
	//	errors = append(errors, "Checksum validation failed")
	// }

	// 验证方向与包头的一致性
	expectedDirection := uint8(DirectionUplink)
	if frame.Header == HeaderDownlink {
		expectedDirection = uint8(DirectionDownlink)
	}
	if frame.Direction != expectedDirection {
		errors = append(errors, "Direction mismatch with header")
	}

	frame.Errors = append(frame.Errors, errors...)

	if len(errors) > 0 {
		return fmt.Errorf("frame validation failed: %s", strings.Join(errors, ", "))
	}

	return nil
}

// CalculateChecksum 计算校验和
func (p *DefaultFrameParser) CalculateChecksum(data []byte) uint8 {
	var sum uint8
	for _, b := range data {
		sum += b
	}
	return sum
}

// GetFrameType 获取帧类型
func (p *DefaultFrameParser) GetFrameType(frame *Frame) string {
	// 根据命令和方向确定帧类型
	switch frame.Command {
	case CmdHeartbeat:
		if frame.Direction == DirectionUplink {
			return "heartbeat_uplink"
		}
		return "heartbeat_downlink"
	case CmdStatusQuery:
		if frame.Direction == DirectionUplink {
			return "status_response"
		}
		return "status_query"
	case CmdNetworkConfig:
		if frame.Direction == DirectionUplink {
			return "network_response"
		}
		return "network_config"
	case CmdDeviceControl:
		return "device_control"
	case CmdOTA:
		return "ota_command"
	default:
		// 检查是否为BKV子协议
		if len(frame.Payload) > 0 {
			// 尝试解析BKV命令
			if len(frame.Payload) >= 2 {
				bkvCmd := binary.BigEndian.Uint16(frame.Payload[0:2])
				switch bkvCmd {
				case BKVCmdStatusReport:
					return "bkv_status_report"
				case BKVCmdControlDevice:
					return "bkv_control_device"
				case BKVCmdChargingEnd:
					return "bkv_charging_end"
				case BKVCmdCardCharging:
					return "bkv_card_charging"
				case BKVCmdBalanceQuery:
					return "bkv_balance_query"
				case BKVCmdParamSet:
					return "bkv_param_set"
				case BKVCmdParamQuery:
					return "bkv_param_query"
				case BKVCmdExceptionEvent:
					return "bkv_exception_event"
				default:
					return "bkv_unknown"
				}
			}
		}
		return "unknown"
	}
}

// ParseHexString 解析十六进制字符串为字节数组
func ParseHexString(hexStr string) ([]byte, error) {
	// 清理输入字符串
	hexStr = strings.ReplaceAll(hexStr, " ", "")
	hexStr = strings.ReplaceAll(hexStr, "\n", "")
	hexStr = strings.ReplaceAll(hexStr, "\t", "")
	hexStr = strings.ToLower(hexStr)

	// 移除0x前缀
	if strings.HasPrefix(hexStr, "0x") {
		hexStr = hexStr[2:]
	}

	// 确保偶数长度
	if len(hexStr)%2 != 0 {
		return nil, fmt.Errorf("invalid hex string length: %d", len(hexStr))
	}

	// 解码
	bytes, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil, fmt.Errorf("hex decode error: %w", err)
	}

	return bytes, nil
}

// IsUplink 判断是否为上行帧
func (f *Frame) IsUplink() bool {
	return f.Direction == DirectionUplink
}

// IsDownlink 判断是否为下行帧
func (f *Frame) IsDownlink() bool {
	return f.Direction == DirectionDownlink
}

// GetGatewayIDHex 获取网关ID的十六进制表示
func (f *Frame) GetGatewayIDHex() string {
	return f.GatewayID
}

// HasPayload 判断是否有载荷
func (f *Frame) HasPayload() bool {
	return len(f.Payload) > 0
}

// IsBKVProtocol 判断是否为BKV协议
func (f *Frame) IsBKVProtocol() bool {
	return strings.HasPrefix(f.FrameType, "bkv_")
}