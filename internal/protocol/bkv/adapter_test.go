package bkv

import "testing"

func TestAdapter_ProcessBytes(t *testing.T) {
	a := NewAdapter()
	var gotCmds []byte
	a.Register(0x10, func(f *Frame) error { gotCmds = append(gotCmds, f.Cmd); return nil })
	a.Register(0x11, func(f *Frame) error { gotCmds = append(gotCmds, f.Cmd); return nil })
	raw := append([]byte{0x00}, []byte{0xFC, 0xFE, 5, 0x10, 0x01, 0x00}...)
	raw = append(raw, []byte{0xFC, 0xFF, 5, 0x11, 0x02, 0x00}...)
	if err := a.ProcessBytes(raw); err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(gotCmds) != 2 || gotCmds[0] != 0x10 || gotCmds[1] != 0x11 {
		t.Fatalf("unexpected cmds: %#v", gotCmds)
	}
}
