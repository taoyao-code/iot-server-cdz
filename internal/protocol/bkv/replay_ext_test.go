package bkv

import "testing"

// TestReplay_Errors 粘/半包/错误校验的容错回放
func TestReplay_Errors(t *testing.T) {
	a := NewAdapter()
	calls := 0
	a.Register(0x10, func(f *Frame) error { calls++; return nil })
	// 粘包 + 半包 + 错误sum 字节（解析器忽略sum）
	stream := append([]byte{0xFC, 0xFE, 5, 0x10, 0x00, 0x00}, []byte{0xFC, 0xFE, 5, 0x10, 0x00}...)
	// 第二帧半包，不应触发；后续补齐
	frames, err := a.decoder.Feed(stream)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(frames) != 1 || calls != 1 {
		t.Fatalf("expected 1 frame, got %d calls %d", len(frames), calls)
	}
	// 补齐半包
	frames, err = a.decoder.Feed([]byte{0x00})
	if err != nil {
		t.Fatalf("err2: %v", err)
	}
	for _, f := range frames {
		_ = a.table.Route(f)
	}
	if calls != 2 {
		t.Fatalf("expected 2 calls, got %d", calls)
	}
}
