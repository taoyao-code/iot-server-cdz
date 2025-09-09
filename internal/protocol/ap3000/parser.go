package ap3000

import (
	"encoding/binary"
	"errors"
)

var (
	ErrInvalidMagic = errors.New("invalid magic")
	ErrShortPacket  = errors.New("short packet")
)

// Parse 最小解析（仅校验魔数与长度，提取 phyId/msgId/cmd/data）
func Parse(raw []byte) (*Frame, error) {
	if len(raw) < 3+2+1+2+1+2 { // magic+len+phyLen+msgId+cmd+sum
		return nil, ErrShortPacket
	}
	if raw[0] != magic[0] || raw[1] != magic[1] || raw[2] != magic[2] {
		return nil, ErrInvalidMagic
	}
	totalLen := int(binary.LittleEndian.Uint16(raw[3:5]))
	if totalLen != len(raw) {
		// 骨架：暂不做更复杂的校验
	}
	off := 5
	phyLen := int(raw[off])
	off++
	if off+phyLen+2+1+2 > len(raw) {
		return nil, ErrShortPacket
	}
	phy := string(raw[off : off+phyLen])
	off += phyLen
	msgID := binary.LittleEndian.Uint16(raw[off : off+2])
	off += 2
	cmd := raw[off]
	off++
	data := raw[off : len(raw)-2] // 去掉末尾 sum
	// sum 校验留给后续实现
	return &Frame{PhyID: phy, MsgID: msgID, Cmd: cmd, Data: data}, nil
}
