package bkv

import "testing"

func TestParse_Min(t *testing.T) {
    // magic fc fe | len 5 | cmd 0x10 | data[0] | sum
    raw := []byte{0xFC,0xFE, 5, 0x10, 0x01, 0x00}
    fr, err := Parse(raw)
    if err != nil || fr.Cmd != 0x10 || len(fr.Data) != 1 { t.Fatalf("bad parse") }
}

func TestStreamDecoder(t *testing.T) {
    d := NewStreamDecoder()
    raw := append([]byte{0x00,0xFC,0xFE,5,0x10,0x01,0x00}, []byte{0xFC,0xFF,5,0x11,0x02,0x00}...)
    frames, err := d.Feed(raw)
    if err != nil || len(frames) != 2 { t.Fatalf("frames=%d err=%v", len(frames), err) }
}


