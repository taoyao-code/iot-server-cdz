package gn

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
)

// GN协议帧格式常量
const (
	// 帧头
	FrameHeaderUplink   = 0xFCFE // 上行帧头 (设备->服务器)
	FrameHeaderDownlink = 0xFCFF // 下行帧头 (服务器->设备)
	FrameTail           = 0xFCEE // 帧尾

	// 最小帧长度 (header+len+cmd+seq+dir+gwid+checksum+tail)
	MinFrameLength = 2 + 2 + 2 + 4 + 1 + 7 + 1 + 2 // = 21

	// 最大帧长度 (防止无限长帧)
	MaxFrameLength = 4096

	// 方向字段
	DirectionUplink   = 0x01 // 上行 (设备->服务器)
	DirectionDownlink = 0x00 // 下行 (服务器->设备)
)

// Frame 表示一个完整的GN协议帧
type Frame struct {
	Header    uint16 // 帧头 fcfe/fcff
	Length    uint16 // 包长度
	Command   uint16 // 命令字
	Sequence  uint32 // 帧流水号
	Direction uint8  // 数据方向
	GatewayID []byte // 网关ID (7字节)
	Payload   []byte // 载荷数据
	Checksum  uint8  // 校验和
	Tail      uint16 // 帧尾 fcee
}

// IsUplink 判断是否为上行帧
func (f *Frame) IsUplink() bool {
	return f.Header == FrameHeaderUplink && f.Direction == DirectionUplink
}

// IsDownlink 判断是否为下行帧
func (f *Frame) IsDownlink() bool {
	return f.Header == FrameHeaderDownlink && f.Direction == DirectionDownlink
}

// GetGatewayIDHex 获取网关ID的十六进制字符串表示
func (f *Frame) GetGatewayIDHex() string {
	return hex.EncodeToString(f.GatewayID)
}

// Encode 将帧编码为字节数组
func (f *Frame) Encode() ([]byte, error) {
	if len(f.GatewayID) != 7 {
		return nil, fmt.Errorf("gateway ID must be 7 bytes, got %d", len(f.GatewayID))
	}

	// 计算总长度: cmd + seq + dir + gwid + payload + checksum + 额外的2字节
	// 基于实际示例反推，长度字段似乎包含额外的2字节
	totalLen := 2 + 4 + 1 + 7 + len(f.Payload) + 1 + 2
	if totalLen > 0xFFFF {
		return nil, fmt.Errorf("frame too large: %d bytes", totalLen)
	}

	f.Length = uint16(totalLen)

	// 构建帧数据 (不含校验和和帧尾)
	buf := make([]byte, 0, totalLen+4) // +4 for header and tail
	
	// 帧头
	buf = append(buf, byte(f.Header>>8), byte(f.Header&0xFF))
	
	// 长度
	buf = append(buf, byte(f.Length>>8), byte(f.Length&0xFF))
	
	// 命令字
	buf = append(buf, byte(f.Command>>8), byte(f.Command&0xFF))
	
	// 序列号
	buf = append(buf, byte(f.Sequence>>24), byte(f.Sequence>>16), 
		byte(f.Sequence>>8), byte(f.Sequence&0xFF))
	
	// 方向
	buf = append(buf, f.Direction)
	
	// 网关ID
	buf = append(buf, f.GatewayID...)
	
	// 载荷
	buf = append(buf, f.Payload...)
	
	// 计算校验和 (从长度字段开始到载荷结束)
	checksum := calculateChecksum(buf[2:]) // 从长度字段开始
	f.Checksum = checksum
	buf = append(buf, checksum)
	
	// 帧尾
	buf = append(buf, byte(FrameTail>>8), byte(FrameTail&0xFF))
	
	return buf, nil
}

// calculateChecksum 计算累加校验和 (模256)
func calculateChecksum(data []byte) uint8 {
	var sum uint32
	for _, b := range data {
		sum += uint32(b)
	}
	return uint8(sum % 256)
}

// ParseFrame 解析字节数组为帧结构
func ParseFrame(data []byte) (*Frame, error) {
	if len(data) < MinFrameLength {
		return nil, fmt.Errorf("frame too short: %d bytes", len(data))
	}

	frame := &Frame{}
	offset := 0

	// 解析帧头
	frame.Header = binary.BigEndian.Uint16(data[offset:])
	offset += 2
	
	if frame.Header != FrameHeaderUplink && frame.Header != FrameHeaderDownlink {
		return nil, fmt.Errorf("invalid frame header: 0x%04X", frame.Header)
	}

	// 解析长度
	frame.Length = binary.BigEndian.Uint16(data[offset:])
	offset += 2

	// 验证帧长度: length字段 + 帧头(2) + 帧尾(2)
	expectedFrameSize := int(frame.Length) + 4
	if len(data) < expectedFrameSize {
		return nil, fmt.Errorf("incomplete frame: expected %d bytes, got %d", 
			expectedFrameSize, len(data))
	}

	// 解析命令字
	frame.Command = binary.BigEndian.Uint16(data[offset:])
	offset += 2

	// 解析序列号
	frame.Sequence = binary.BigEndian.Uint32(data[offset:])
	offset += 4

	// 解析方向
	frame.Direction = data[offset]
	offset += 1

	// 解析网关ID (7字节)
	frame.GatewayID = make([]byte, 7)
	copy(frame.GatewayID, data[offset:offset+7])
	offset += 7

	// 计算载荷长度: 实际测试显示，载荷比计算的少2字节
	// 看起来长度字段可能包含了一些不明确的部分
	// 基于实际示例调整: 载荷 = length - (cmd+seq+dir+gwid+checksum) - 2
	baseLen := 2 + 4 + 1 + 7 + 1 // cmd + seq + dir + gwid + checksum
	payloadLen := int(frame.Length) - baseLen - 2 // 减去额外的2字节
	if payloadLen < 0 {
		payloadLen = 0
	}

	// 解析载荷
	if payloadLen > 0 {
		frame.Payload = make([]byte, payloadLen)
		copy(frame.Payload, data[offset:offset+payloadLen])
		offset += payloadLen
	}

	// 解析校验和
	frame.Checksum = data[offset]
	offset += 1

	// 验证校验和: 从长度字段开始到载荷结束(不包含校验和本身)
	checksumData := data[2:offset-1]  // 从长度字段开始
	expectedChecksum := calculateChecksum(checksumData)
	if frame.Checksum != expectedChecksum {
		return nil, fmt.Errorf("checksum mismatch: expected 0x%02X, got 0x%02X (checksum at offset %d)",
			expectedChecksum, frame.Checksum, offset-1)
	}

	// 解析帧尾
	frame.Tail = binary.BigEndian.Uint16(data[offset:])
	if frame.Tail != FrameTail {
		return nil, fmt.Errorf("invalid frame tail: 0x%04X", frame.Tail)
	}

	return frame, nil
}

// NewFrame 创建新帧
func NewFrame(cmd uint16, seq uint32, gwid []byte, payload []byte, isDownlink bool) (*Frame, error) {
	if len(gwid) != 7 {
		return nil, fmt.Errorf("gateway ID must be 7 bytes")
	}

	frame := &Frame{
		Command:   cmd,
		Sequence:  seq,
		GatewayID: make([]byte, 7),
		Payload:   payload,
	}

	copy(frame.GatewayID, gwid)

	if isDownlink {
		frame.Header = FrameHeaderDownlink
		frame.Direction = DirectionDownlink
	} else {
		frame.Header = FrameHeaderUplink
		frame.Direction = DirectionUplink
	}

	return frame, nil
}