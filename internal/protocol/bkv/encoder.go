package bkv

import (
	"encoding/binary"
	"encoding/hex"
)

// Build 构造BKV下行帧
// 格式：fcff(2) + len(2) + [len字节数据: cmd(2) + msgID(4) + direction(1) + gatewayID(var) + data(var) + checksum(1) + fcee(2)]
func Build(cmd uint16, msgID uint32, gatewayID string, data []byte) []byte {
	gatewayIDBytes, _ := hex.DecodeString(gatewayID)
	if len(gatewayIDBytes) == 0 {
		gatewayIDBytes = make([]byte, 7) // 默认7字节网关ID
	}

	// 计算数据部分长度：cmd(2) + msgID(4) + direction(1) + gatewayID + data + checksum(1) + fcee(2)
	dataLen := 2 + 4 + 1 + len(gatewayIDBytes) + len(data) + 1 + 2
	totalLen := 4 + dataLen // magic(2) + len(2) + data
	
	buf := make([]byte, 0, totalLen)

	// 包头 (下行用 fcff)
	buf = append(buf, magicDownlink...)

	// 数据长度
	lenBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(lenBytes, uint16(dataLen))
	buf = append(buf, lenBytes...)

	// 命令码
	cmdBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(cmdBytes, cmd)
	buf = append(buf, cmdBytes...)

	// 帧流水号
	msgIDBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(msgIDBytes, msgID)
	buf = append(buf, msgIDBytes...)

	// 数据方向 (下行为0x00)
	buf = append(buf, 0x00)

	// 网关ID
	buf = append(buf, gatewayIDBytes...)

	// 数据
	buf = append(buf, data...)

	// 校验和 (简单累加校验，从命令码开始)
	checksum := calculateChecksum(buf[4:]) // 从数据部分开始校验
	buf = append(buf, checksum)

	// 包尾
	buf = append(buf, tailMagic...)

	return buf
}

// BuildUplink 构造上行帧 (用于测试)
func BuildUplink(cmd uint16, msgID uint32, gatewayID string, data []byte) []byte {
	gatewayIDBytes, _ := hex.DecodeString(gatewayID)
	if len(gatewayIDBytes) == 0 {
		gatewayIDBytes = make([]byte, 7)
	}

	dataLen := 2 + 4 + 1 + len(gatewayIDBytes) + len(data) + 1 + 2
	totalLen := 4 + dataLen
	
	buf := make([]byte, 0, totalLen)

	// 包头 (上行用 fcfe)
	buf = append(buf, magicUplink...)

	// 数据长度
	lenBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(lenBytes, uint16(dataLen))
	buf = append(buf, lenBytes...)

	// 命令码
	cmdBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(cmdBytes, cmd)
	buf = append(buf, cmdBytes...)

	// 帧流水号
	msgIDBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(msgIDBytes, msgID)
	buf = append(buf, msgIDBytes...)

	// 数据方向 (上行为0x01)
	buf = append(buf, 0x01)

	// 网关ID
	buf = append(buf, gatewayIDBytes...)

	// 数据
	buf = append(buf, data...)

	// 校验和
	checksum := calculateChecksum(buf[4:])
	buf = append(buf, checksum)

	// 包尾
	buf = append(buf, tailMagic...)

	return buf
}

// calculateChecksum 计算校验和 (简单累加)
func calculateChecksum(data []byte) uint8 {
	var sum uint8
	for _, b := range data {
		sum += b
	}
	return sum
}


