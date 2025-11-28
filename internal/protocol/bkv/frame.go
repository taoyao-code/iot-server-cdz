package bkv

import (
	"encoding/hex"
	"errors"
)

// Frame BKV 协议帧结构
// 格式：fcfe/fcff(2) + len(2) + cmd(2) + msgID(4) + direction(1) + gatewayID(var) + data(var) + checksum(1) + fcee(2)
type Frame struct {
	Magic     []byte // fcfe (上行) 或 fcff (下行)
	Len       uint16 // 包长度，包含整个帧
	Cmd       uint16 // 命令码
	MsgID     uint32 // 帧流水号
	Direction uint8  // 包类型/数据方向：0x00-服务器下行，0x01-设备上行
	GatewayID string // 网关ID (变长)
	Data      []byte // 数据payload
	Checksum  uint8  // 校验和
	Tail      []byte // fcee 包尾

	// BKV子协议解析结果 (缓存)
	bkvPayload *BKVPayload
}

// IsUplink 判断是否为上行帧
func (f *Frame) IsUplink() bool {
	return f.Direction == 0x01
}

// IsDownlink 判断是否为下行帧
func (f *Frame) IsDownlink() bool {
	return f.Direction == 0x00
}

// GatewayIDBytes 返回网关ID的原始字节
func (f *Frame) GatewayIDBytes() []byte {
	if len(f.GatewayID) == 0 {
		return nil
	}
	bytes, _ := hex.DecodeString(f.GatewayID)
	return bytes
}

// GetBKVPayload 解析并返回BKV子协议载荷
func (f *Frame) GetBKVPayload() (*BKVPayload, error) {
	if f.bkvPayload != nil {
		return f.bkvPayload, nil
	}

	// 只有命令0x1000包含BKV子协议
	if f.Cmd != 0x1000 {
		return nil, errors.New("not a BKV protocol frame")
	}

	payload, err := ParseBKVPayload(f.Data)
	if err != nil {
		return nil, err
	}

	f.bkvPayload = payload
	return payload, nil
}

// IsBKVFrame 判断是否为BKV子协议帧
func (f *Frame) IsBKVFrame() bool {
	return f.Cmd == 0x1000
}

// IsHeartbeat 判断是否为心跳帧
// 协议规范：心跳命令仅为帧命令0x0000，BKV子协议没有心跳命令
// 注意：0x1000帧（BKV兼容包）不可能是心跳，即使其BKV子命令是0x1017（状态上报）
func (f *Frame) IsHeartbeat() bool {
	return f.Cmd == 0x0000 // 仅帧命令0x0000是心跳
}

var (
	magicUplink   = []byte{0xFC, 0xFE} // 设备上行
	magicDownlink = []byte{0xFC, 0xFF} // 服务器下行
	tailMagic     = []byte{0xFC, 0xEE} // 包尾
)
