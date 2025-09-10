package ap3000

import (
	"encoding/binary"
	"errors"
)

var (
	ErrInvalidMagic  = errors.New("invalid magic")
	ErrShortPacket   = errors.New("short packet")
	ErrBadLength     = errors.New("bad length")
	ErrBadChecksum   = errors.New("bad checksum")
	ErrProtocolLimit = errors.New("protocol length limit exceeded")
)

// checksum16 累加校验（低16位），不包含最终的校验字段本身
func checksum16(b []byte) uint16 {
	var sum uint32
	for i := 0; i < len(b); i++ {
		sum += uint32(b[i])
	}
	return uint16(sum & 0xFFFF)
}

// Parse 解析一帧（严格校验：magic、长度、checksum）
// 说明：当前实现约定 len 字段等于整个帧总长度（包含 magic 与 len 本身），与现有测试保持一致。
func Parse(raw []byte) (*Frame, error) {
	if len(raw) < 3+2+1+2+1+2 { // magic+len+phyLen+msgId+cmd+sum
		return nil, ErrShortPacket
	}
	if raw[0] != magic[0] || raw[1] != magic[1] || raw[2] != magic[2] {
		return nil, ErrInvalidMagic
	}
	totalLen := int(binary.LittleEndian.Uint16(raw[3:5]))
	if totalLen != len(raw) {
		return nil, ErrBadLength
	}
	// 校验和（小端），覆盖范围：除去末尾2字节校验本身
	got := binary.LittleEndian.Uint16(raw[len(raw)-2:])
	want := checksum16(raw[:len(raw)-2])
	if got != want {
		return nil, ErrBadChecksum
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
	return &Frame{PhyID: phy, MsgID: msgID, Cmd: cmd, Data: data}, nil
}

// StreamDecoder 处理半包/粘包的流式解码器
type StreamDecoder struct {
	buf         []byte
	maxFrameLen int // 保护上限，避免畸形数据占用过多内存
}

// NewStreamDecoder 创建流式解码器
func NewStreamDecoder(maxFrameLen int) *StreamDecoder {
	if maxFrameLen <= 0 {
		maxFrameLen = 1024 // 协议宣称<=256，这里放宽一些
	}
	return &StreamDecoder{maxFrameLen: maxFrameLen}
}

// Feed 追加数据并尽可能解出多帧
func (d *StreamDecoder) Feed(p []byte) ([]*Frame, error) {
	if len(p) == 0 {
		return nil, nil
	}
	d.buf = append(d.buf, p...)
	frames := make([]*Frame, 0, 2)

	for {
		// 查找 magic
		start := indexMagic(d.buf)
		if start < 0 {
			// 无 magic，清空缓冲避免无界增长
			if len(d.buf) > 0 {
				// 保留最后2字节以应对下一次可能的跨边界 magic
				if len(d.buf) > 2 {
					d.buf = d.buf[len(d.buf)-2:]
				}
			}
			return frames, nil
		}
		if start > 0 {
			// 丢弃无效前缀
			d.buf = d.buf[start:]
		}
		if len(d.buf) < 5 {
			// 还需要更多字节（magic+len）
			return frames, nil
		}
		totalLen := int(binary.LittleEndian.Uint16(d.buf[3:5]))
		if totalLen <= 0 || totalLen > d.maxFrameLen {
			// 明显异常的长度，丢弃1字节后继续同步
			d.buf = d.buf[1:]
			continue
		}
		if len(d.buf) < totalLen {
			// 半包，等待更多
			return frames, nil
		}

		candidate := d.buf[:totalLen]
		// 先做快速校验和检查
		got := binary.LittleEndian.Uint16(candidate[totalLen-2:])
		want := checksum16(candidate[:totalLen-2])
		if got != want {
			// 校验失败，向后滑动一个字节继续寻找同步
			d.buf = d.buf[1:]
			continue
		}
		// 解析
		fr, err := Parse(candidate)
		if err != nil {
			// 稳妥起见滑动1字节
			d.buf = d.buf[1:]
			continue
		}
		frames = append(frames, fr)
		// 消耗本帧
		d.buf = d.buf[totalLen:]
		if len(d.buf) == 0 {
			return frames, nil
		}
	}
}

// indexMagic 返回缓冲区中下一个 magic 开始位置
func indexMagic(b []byte) int {
	if len(b) < 3 {
		return -1
	}
	for i := 0; i <= len(b)-3; i++ {
		if b[i] == magic[0] && b[i+1] == magic[1] && b[i+2] == magic[2] {
			return i
		}
	}
	return -1
}
