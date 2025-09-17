package bkv

import "testing"

// TestReplay 基于离线报文回放，验证解码与路由顺序
func TestReplay(t *testing.T) {
	a := NewAdapter()
	var seq []uint16
	a.Register(0x0000, func(f *Frame) error { seq = append(seq, f.Cmd); return nil })
	a.Register(0x1000, func(f *Frame) error { seq = append(seq, f.Cmd); return nil })

	// 构造离线抓包（含噪声与不同magic）
	frame1 := BuildUplink(0x0000, 0, "82200520004869", []byte{0xAA})
	frame2 := Build(0x1000, 0, "82200520004869", []byte{0xBB})

	cap := append([]byte{0x01, 0x02, 0x03}, frame1...)
	cap = append(cap, frame2...)
	cap = append(cap, []byte{0x00, 0x00}...)

	if err := a.ProcessBytes(cap); err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(seq) != 2 || seq[0] != 0x0000 || seq[1] != 0x1000 {
		t.Fatalf("bad seq: %#v", seq)
	}
}
