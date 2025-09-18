package gn

import (
	"encoding/binary"
	"fmt"
)

// TLV表示一个Tag-Length-Value结构
type TLV struct {
	Tag    uint8  // 标签
	Length uint8  // 长度
	Value  []byte // 值
}

// TLVList 表示TLV列表
type TLVList []TLV

// ParseTLVs 解析TLV数据
func ParseTLVs(data []byte) (TLVList, error) {
	var tlvs TLVList
	offset := 0

	for offset < len(data) {
		if offset+2 > len(data) {
			break // 不够解析tag和length
		}

		tlv := TLV{
			Tag:    data[offset],
			Length: data[offset+1],
		}
		offset += 2

		// 检查是否有足够的数据
		if offset+int(tlv.Length) > len(data) {
			return nil, fmt.Errorf("TLV length %d exceeds remaining data %d at offset %d", 
				tlv.Length, len(data)-offset, offset-2)
		}

		// 复制值
		if tlv.Length > 0 {
			tlv.Value = make([]byte, tlv.Length)
			copy(tlv.Value, data[offset:offset+int(tlv.Length)])
			offset += int(tlv.Length)
		}

		tlvs = append(tlvs, tlv)
	}

	return tlvs, nil
}

// FindByTag 查找指定标签的TLV
func (list TLVList) FindByTag(tag uint8) *TLV {
	for i := range list {
		if list[i].Tag == tag {
			return &list[i]
		}
	}
	return nil
}

// GetUint8 获取单字节数值
func (tlv *TLV) GetUint8() uint8 {
	if len(tlv.Value) >= 1 {
		return tlv.Value[0]
	}
	return 0
}

// GetUint16 获取双字节数值(大端序)
func (tlv *TLV) GetUint16() uint16 {
	if len(tlv.Value) >= 2 {
		return binary.BigEndian.Uint16(tlv.Value)
	}
	return 0
}

// GetUint32 获取四字节数值(大端序)
func (tlv *TLV) GetUint32() uint32 {
	if len(tlv.Value) >= 4 {
		return binary.BigEndian.Uint32(tlv.Value)
	}
	return 0
}

// GetString 获取字符串值
func (tlv *TLV) GetString() string {
	return string(tlv.Value)
}

// GetBytes 获取原始字节值
func (tlv *TLV) GetBytes() []byte {
	return tlv.Value
}

// Encode 编码TLV为字节数组
func (tlv *TLV) Encode() []byte {
	result := make([]byte, 2+len(tlv.Value))
	result[0] = tlv.Tag
	result[1] = uint8(len(tlv.Value))
	copy(result[2:], tlv.Value)
	return result
}

// EncodeTLVs 编码TLV列表为字节数组
func EncodeTLVs(tlvs TLVList) []byte {
	var result []byte
	for _, tlv := range tlvs {
		result = append(result, tlv.Encode()...)
	}
	return result
}

// NewTLV 创建新的TLV
func NewTLV(tag uint8, value []byte) TLV {
	return TLV{
		Tag:    tag,
		Length: uint8(len(value)),
		Value:  value,
	}
}

// NewTLVUint8 创建单字节数值TLV
func NewTLVUint8(tag uint8, value uint8) TLV {
	return NewTLV(tag, []byte{value})
}

// NewTLVUint16 创建双字节数值TLV
func NewTLVUint16(tag uint8, value uint16) TLV {
	buf := make([]byte, 2)
	binary.BigEndian.PutUint16(buf, value)
	return NewTLV(tag, buf)
}

// NewTLVUint32 创建四字节数值TLV
func NewTLVUint32(tag uint8, value uint32) TLV {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, value)
	return NewTLV(tag, buf)
}

// NewTLVString 创建字符串TLV
func NewTLVString(tag uint8, value string) TLV {
	return NewTLV(tag, []byte(value))
}

// GN协议中的主要TLV标签常量
const (
	// 设备信息相关
	TagSocketNumber   = 0x4A // 插座序号
	TagSoftwareVer    = 0x3E // 插座软件版本
	TagTemperature    = 0x07 // 温度
	TagRSSI           = 0x96 // RSSI信号强度
	TagSocketAttr     = 0x5B // 插孔属性

	// 插孔属性相关
	TagPortNumber     = 0x08 // 插孔号
	TagPortStatus     = 0x09 // 插座状态
	TagBusinessNumber = 0x0A // 业务号
	TagVoltage        = 0x95 // 电压
	TagPower          = 0x0B // 瞬时功率
	TagCurrent        = 0x0C // 瞬时电流
	TagEnergy         = 0x0D // 用电量
	TagDuration       = 0x0E // 充电时间

	// 网关信息相关
	TagGatewayID      = 0x03 // 网关ID
	TagICCID          = 0x01 // ICCID
	TagDeviceID       = 0x02 // 设备ID
)