package bkv

import "errors"

var (
    ErrShort = errors.New("short packet")
    ErrBad   = errors.New("bad packet")
)

// Parse 最小解析：校验magic与长度字段
func Parse(b []byte) (*Frame, error) {
    if len(b) < 2+1+1+1 {
        return nil, ErrShort
    }
    if !((b[0] == magicA[0] && b[1] == magicA[1]) || (b[0] == magicB[0] && b[1] == magicB[1])) {
        return nil, ErrBad
    }
    total := int(b[2])
    if total <= 0 || total > len(b) {
        return nil, ErrBad
    }
    cmd := b[3]
    data := b[4:total-1]
    // sum占位不校验
    return &Frame{Cmd: cmd, Data: data}, nil
}

// StreamDecoder 简化版：按magic+len切分
type StreamDecoder struct{ buf []byte }

func NewStreamDecoder() *StreamDecoder { return &StreamDecoder{} }

func (d *StreamDecoder) Feed(p []byte) ([]*Frame, error) {
    d.buf = append(d.buf, p...)
    var out []*Frame
    for {
        if len(d.buf) < 4 {
            return out, nil
        }
        if !((d.buf[0] == magicA[0] && d.buf[1] == magicA[1]) || (d.buf[0] == magicB[0] && d.buf[1] == magicB[1])) {
            d.buf = d.buf[1:]
            continue
        }
        total := int(d.buf[2])
        if total <= 0 {
            d.buf = d.buf[1:]
            continue
        }
        if len(d.buf) < total {
            return out, nil
        }
        fr, err := Parse(d.buf[:total])
        if err == nil {
            out = append(out, fr)
            d.buf = d.buf[total:]
            if len(d.buf) == 0 { return out, nil }
            continue
        }
        d.buf = d.buf[1:]
    }
}


