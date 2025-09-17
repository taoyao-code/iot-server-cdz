package bkv

import "testing"

func TestAdapter_ProcessBytes(t *testing.T) {
	a := NewAdapter()
	var gotCmds []uint16
	a.Register(0x0000, func(f *Frame) error { gotCmds = append(gotCmds, f.Cmd); return nil })
	a.Register(0x1000, func(f *Frame) error { gotCmds = append(gotCmds, f.Cmd); return nil })

	// 构造两个测试帧：心跳(0x0000)和状态上报(0x1000)
	frame1 := BuildUplink(0x0000, 0, "82200520004869", []byte{0x01})
	frame2 := BuildUplink(0x1000, 0, "82200520004869", []byte{0x02})

	raw := append([]byte{0x00}, frame1...) // 前面加噪声
	raw = append(raw, frame2...)

	if err := a.ProcessBytes(raw); err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(gotCmds) != 2 || gotCmds[0] != 0x0000 || gotCmds[1] != 0x1000 {
		t.Fatalf("unexpected cmds: %#v", gotCmds)
	}
}
