package bkv

import (
	"os"
	"testing"
)

func TestReasonMap_LoadAndTranslate(t *testing.T) {
	tmp := t.TempDir() + "/rm.yaml"
	if err := os.WriteFile(tmp, []byte("map:\n  2: 102\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := LoadReasonMap(tmp)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if v, ok := m.Translate(2); !ok || v != 102 {
		t.Fatalf("translate: %v %v", v, ok)
	}
	if _, ok := m.Translate(99); ok {
		t.Fatalf("unexpected map")
	}
}
