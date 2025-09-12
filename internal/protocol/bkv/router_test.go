package bkv

import "testing"

func TestTable_RegisterAndRoute(t *testing.T) {
	tbl := NewTable()
	called := false
	tbl.Register(0x10, func(f *Frame) error { called = true; return nil })
	_ = tbl.Route(&Frame{Cmd: 0x10})
	if !called {
		t.Fatalf("handler not called")
	}
}
