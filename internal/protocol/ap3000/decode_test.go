package ap3000

import "testing"

func TestDecode20or21_Minimal(t *testing.T) {
	ps, err := Decode20or21([]byte{2, 1})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if ps.Port != 2 || ps.Status != 1 || ps.PowerW != nil {
		t.Fatalf("unexpected: %+v", ps)
	}
}

func TestDecode20or21_WithPower(t *testing.T) {
	ps, err := Decode20or21([]byte{1, 3, 0x10, 0x00})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if ps.PowerW == nil || *ps.PowerW != 16 {
		t.Fatalf("power: %+v", ps)
	}
}

func TestDecode20or21_Bad(t *testing.T) {
	if _, err := Decode20or21([]byte{1}); err == nil {
		t.Fatalf("expected error")
	}
}
