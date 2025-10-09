package bkv

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
)

var (
	ErrShort    = errors.New("short packet")
	ErrBadMagic = errors.New("bad magic")
	ErrBadLen   = errors.New("bad length")
	ErrBadTail  = errors.New("bad tail")
)

// Parse 解析BKV协议帧
// 格式：fcfe/fcff(2) + len(2) + [len字节的数据，包含cmd+msgID+direction+gatewayID+data+checksum+fcee]
func Parse(b []byte) (*Frame, error) {
	if len(b) < 4 { // 至少需要magic+len
		return nil, ErrShort
	}

	// 检查包头magic
	var magic []byte
	if b[0] == magicUplink[0] && b[1] == magicUplink[1] {
		magic = magicUplink
	} else if b[0] == magicDownlink[0] && b[1] == magicDownlink[1] {
		magic = magicDownlink
	} else {
		return nil, ErrBadMagic
	}

	// 解析包长度 (长度字段后面的数据长度)
	dataLen := binary.BigEndian.Uint16(b[2:4])
	totalLen := 4 + int(dataLen) // magic(2) + len(2) + data(dataLen)

	if len(b) < totalLen {
		return nil, ErrBadLen
	}

	// 检查包尾 (data部分的最后2字节应该是fcee)
	if dataLen < 2 || b[totalLen-2] != tailMagic[0] || b[totalLen-1] != tailMagic[1] {
		return nil, ErrBadTail
	}

	frame := &Frame{
		Magic: magic,
		Len:   uint16(totalLen), // 整个帧的长度
		Tail:  tailMagic,
	}

	pos := 4                // 跳过magic和len
	dataEnd := totalLen - 2 // 排除包尾fcee

	// 解析命令码 (2字节)
	if pos+2 > dataEnd {
		return nil, ErrShort
	}
	frame.Cmd = binary.BigEndian.Uint16(b[pos : pos+2])
	pos += 2

	// 解析帧流水号 (4字节)
	if pos+4 > dataEnd {
		return nil, ErrShort
	}
	frame.MsgID = binary.BigEndian.Uint32(b[pos : pos+4])
	pos += 4

	// 解析数据方向 (1字节)
	if pos+1 > dataEnd {
		return nil, ErrShort
	}
	frame.Direction = b[pos]
	pos += 1

	// 网关ID是变长的，从协议文档看通常是7字节
	gatewayIDLen := 7
	if pos+gatewayIDLen > dataEnd-1 { // 减去1字节checksum
		return nil, ErrShort
	}
	frame.GatewayID = hex.EncodeToString(b[pos : pos+gatewayIDLen])
	pos += gatewayIDLen

	// 数据部分 (到校验和之前)
	checksumPos := dataEnd - 1
	if pos <= checksumPos {
		frame.Data = b[pos:checksumPos]
	}

	// 校验和
	frame.Checksum = b[checksumPos]

	return frame, nil
}

// StreamDecoder BKV协议流式解码器，处理粘包/半包
type StreamDecoder struct {
	buf []byte
}

func NewStreamDecoder() *StreamDecoder {
	return &StreamDecoder{}
}

func (d *StreamDecoder) Feed(p []byte) ([]*Frame, error) {
	d.buf = append(d.buf, p...)
	var frames []*Frame

	for {
		if len(d.buf) < 4 { // 至少需要magic+len
			break
		}

		// 寻找frame开始位置
		start := d.findFrameStart()
		if start == -1 {
			// 没有找到有效的magic，丢弃所有数据
			d.buf = d.buf[:0]
			break
		}

		if start > 0 {
			// 丢弃无效数据
			d.buf = d.buf[start:]
		}

		if len(d.buf) < 4 {
			break
		}

		// 读取数据长度
		dataLen := binary.BigEndian.Uint16(d.buf[2:4])
		totalLen := 4 + int(dataLen) // magic(2) + len(2) + data(dataLen)

		if dataLen < 2 { // 至少需要包尾fcee
			// 长度无效，跳过这个magic
			d.buf = d.buf[1:]
			continue
		}

		if len(d.buf) < totalLen {
			// 数据不够一帧，等待更多数据
			break
		}

		// 尝试解析帧
		frame, err := Parse(d.buf[:totalLen])
		if err != nil {
			// 解析失败，跳过这个magic
			d.buf = d.buf[1:]
			continue
		}

		frames = append(frames, frame)
		d.buf = d.buf[totalLen:]
	}

	return frames, nil
}

// findFrameStart 寻找下一个有效的frame开始位置
func (d *StreamDecoder) findFrameStart() int {
	for i := 0; i < len(d.buf)-1; i++ {
		if (d.buf[i] == magicUplink[0] && d.buf[i+1] == magicUplink[1]) ||
			(d.buf[i] == magicDownlink[0] && d.buf[i+1] == magicDownlink[1]) {
			return i
		}
	}
	return -1
}
