package bkv

import "testing"

// TestReplay 基于离线报文回放，验证解码与路由顺序
func TestReplay(t *testing.T) {
	a := NewAdapter()
	var seq []byte
	a.Register(0x10, func(f *Frame) error { seq = append(seq, f.Cmd); return nil })
	a.Register(0x11, func(f *Frame) error { seq = append(seq, f.Cmd); return nil })

	// 构造离线抓包（含噪声与不同magic）
	cap := append([]byte{0x01, 0x02, 0x03}, []byte{0xFC, 0xFE, 5, 0x10, 0xAA, 0x00}...)
	cap = append(cap, []byte{0xFC, 0xFF, 5, 0x11, 0xBB, 0x00}...)
	cap = append(cap, []byte{0x00, 0x00}...)

	if err := a.ProcessBytes(cap); err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(seq) != 2 || seq[0] != 0x10 || seq[1] != 0x11 {
		t.Fatalf("bad seq: %#v", seq)
	}
}
