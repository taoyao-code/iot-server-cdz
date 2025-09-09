package ap3000

import "testing"

func TestTable_RegisterAndRoute(t *testing.T) {
	tbl := NewTable()
	called := false
	tbl.Register(0x20, func(f *Frame) error { called = true; return nil })
	_ = tbl.Route(&Frame{Cmd: 0x20})
	if !called {
		t.Fatalf("handler not called")
	}
}
