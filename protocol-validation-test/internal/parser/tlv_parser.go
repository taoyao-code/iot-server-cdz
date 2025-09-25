package parser

import (
	"encoding/binary"
	"fmt"
)

// DefaultTLVParser 默认TLV解析器实现
type DefaultTLVParser struct {
}

// NewDefaultTLVParser 创建默认TLV解析器
func NewDefaultTLVParser() *DefaultTLVParser {
	return &DefaultTLVParser{}
}

// ParseTLV 解析TLV结构
func (p *DefaultTLVParser) ParseTLV(data []byte) (map[uint8][]byte, error) {
	fields := make(map[uint8][]byte)
	pos := 0

	for pos < len(data) {
		// 检查剩余数据长度
		if pos+2 > len(data) {
			break
		}

		// 读取Tag (1字节)
		tag := data[pos]
		pos++

		// 读取Length (1字节)
		length := data[pos]
		pos++

		// 检查Value长度
		if pos+int(length) > len(data) {
			return nil, fmt.Errorf("TLV length exceeds data boundary at tag 0x%02X", tag)
		}

		// 读取Value
		value := make([]byte, length)
		copy(value, data[pos:pos+int(length)])
		pos += int(length)

		fields[tag] = value
	}

	return fields, nil
}

// EncodeTLV 编码TLV结构
func (p *DefaultTLVParser) EncodeTLV(fields map[uint8][]byte) ([]byte, error) {
	var result []byte

	for tag, value := range fields {
		if len(value) > 255 {
			return nil, fmt.Errorf("TLV value too long for tag 0x%02X: %d bytes", tag, len(value))
		}

		// 添加Tag
		result = append(result, tag)
		
		// 添加Length
		result = append(result, uint8(len(value)))
		
		// 添加Value
		result = append(result, value...)
	}

	return result, nil
}

// GetFieldValue 获取字段值
func (p *DefaultTLVParser) GetFieldValue(fields map[uint8][]byte, tag uint8) ([]byte, bool) {
	value, exists := fields[tag]
	return value, exists
}

// GetFieldAsUint8 获取字段值作为uint8
func (p *DefaultTLVParser) GetFieldAsUint8(fields map[uint8][]byte, tag uint8) (uint8, error) {
	value, exists := fields[tag]
	if !exists {
		return 0, fmt.Errorf("field 0x%02X not found", tag)
	}
	if len(value) != 1 {
		return 0, fmt.Errorf("field 0x%02X invalid length for uint8: %d", tag, len(value))
	}
	return value[0], nil
}

// GetFieldAsUint16 获取字段值作为uint16 (大端序)
func (p *DefaultTLVParser) GetFieldAsUint16(fields map[uint8][]byte, tag uint8) (uint16, error) {
	value, exists := fields[tag]
	if !exists {
		return 0, fmt.Errorf("field 0x%02X not found", tag)
	}
	if len(value) != 2 {
		return 0, fmt.Errorf("field 0x%02X invalid length for uint16: %d", tag, len(value))
	}
	return binary.BigEndian.Uint16(value), nil
}

// GetFieldAsUint32 获取字段值作为uint32 (大端序)
func (p *DefaultTLVParser) GetFieldAsUint32(fields map[uint8][]byte, tag uint8) (uint32, error) {
	value, exists := fields[tag]
	if !exists {
		return 0, fmt.Errorf("field 0x%02X not found", tag)
	}
	if len(value) != 4 {
		return 0, fmt.Errorf("field 0x%02X invalid length for uint32: %d", tag, len(value))
	}
	return binary.BigEndian.Uint32(value), nil
}

// GetFieldAsUint64 获取字段值作为uint64 (大端序)
func (p *DefaultTLVParser) GetFieldAsUint64(fields map[uint8][]byte, tag uint8) (uint64, error) {
	value, exists := fields[tag]
	if !exists {
		return 0, fmt.Errorf("field 0x%02X not found", tag)
	}
	if len(value) != 8 {
		return 0, fmt.Errorf("field 0x%02X invalid length for uint64: %d", tag, len(value))
	}
	return binary.BigEndian.Uint64(value), nil
}

// GetFieldAsString 获取字段值作为字符串
func (p *DefaultTLVParser) GetFieldAsString(fields map[uint8][]byte, tag uint8) (string, error) {
	value, exists := fields[tag]
	if !exists {
		return "", fmt.Errorf("field 0x%02X not found", tag)
	}
	return string(value), nil
}

// SetFieldUint8 设置uint8字段
func (p *DefaultTLVParser) SetFieldUint8(fields map[uint8][]byte, tag uint8, value uint8) {
	fields[tag] = []byte{value}
}

// SetFieldUint16 设置uint16字段 (大端序)
func (p *DefaultTLVParser) SetFieldUint16(fields map[uint8][]byte, tag uint8, value uint16) {
	buf := make([]byte, 2)
	binary.BigEndian.PutUint16(buf, value)
	fields[tag] = buf
}

// SetFieldUint32 设置uint32字段 (大端序)
func (p *DefaultTLVParser) SetFieldUint32(fields map[uint8][]byte, tag uint8, value uint32) {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, value)
	fields[tag] = buf
}

// SetFieldUint64 设置uint64字段 (大端序)
func (p *DefaultTLVParser) SetFieldUint64(fields map[uint8][]byte, tag uint8, value uint64) {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, value)
	fields[tag] = buf
}

// SetFieldString 设置字符串字段
func (p *DefaultTLVParser) SetFieldString(fields map[uint8][]byte, tag uint8, value string) {
	fields[tag] = []byte(value)
}

// SetFieldBytes 设置字节数组字段
func (p *DefaultTLVParser) SetFieldBytes(fields map[uint8][]byte, tag uint8, value []byte) {
	fields[tag] = make([]byte, len(value))
	copy(fields[tag], value)
}

// HasField 检查字段是否存在
func (p *DefaultTLVParser) HasField(fields map[uint8][]byte, tag uint8) bool {
	_, exists := fields[tag]
	return exists
}

// GetFieldLength 获取字段长度
func (p *DefaultTLVParser) GetFieldLength(fields map[uint8][]byte, tag uint8) int {
	value, exists := fields[tag]
	if !exists {
		return 0
	}
	return len(value)
}

// RemoveField 删除字段
func (p *DefaultTLVParser) RemoveField(fields map[uint8][]byte, tag uint8) {
	delete(fields, tag)
}

// GetAllTags 获取所有标签
func (p *DefaultTLVParser) GetAllTags(fields map[uint8][]byte) []uint8 {
	tags := make([]uint8, 0, len(fields))
	for tag := range fields {
		tags = append(tags, tag)
	}
	return tags
}

// GetTotalLength 计算编码后的总长度
func (p *DefaultTLVParser) GetTotalLength(fields map[uint8][]byte) int {
	totalLength := 0
	for _, value := range fields {
		totalLength += 2 + len(value) // Tag(1) + Length(1) + Value
	}
	return totalLength
}

// ValidateFields 验证字段合法性
func (p *DefaultTLVParser) ValidateFields(fields map[uint8][]byte) error {
	for tag, value := range fields {
		if len(value) > 255 {
			return fmt.Errorf("field 0x%02X value too long: %d bytes", tag, len(value))
		}
	}
	return nil
}

// CloneFields 克隆字段映射
func (p *DefaultTLVParser) CloneFields(fields map[uint8][]byte) map[uint8][]byte {
	cloned := make(map[uint8][]byte)
	for tag, value := range fields {
		cloned[tag] = make([]byte, len(value))
		copy(cloned[tag], value)
	}
	return cloned
}