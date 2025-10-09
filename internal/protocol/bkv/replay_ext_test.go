package bkv

import "testing"

// TestReplay_Errors 粘/半包/错误校验的容错回放
func TestReplay_Errors(t *testing.T) {
	a := NewAdapter()
	calls := 0
	a.Register(0x0000, func(f *Frame) error { calls++; return nil })

	// 构造粘包 + 半包
	frame1 := BuildUplink(0x0000, 0, "82200520004869", []byte{0x00})
	frame2 := BuildUplink(0x0000, 0, "82200520004869", []byte{0x01})

	// 第二帧只取一部分形成半包
	halfLen := len(frame2) - 3
	stream := append(frame1, frame2[:halfLen]...)

	// 第一次feed应该只解析出第一帧
	frames, err := a.decoder.Feed(stream)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(frames) != 1 {
		t.Fatalf("expected 1 frame, got %d", len(frames))
	}

	// 路由第一帧
	err = a.table.Route(frames[0])
	if err != nil {
		t.Fatalf("route err: %v", err)
	}

	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}

	// 补齐半包
	frames, err = a.decoder.Feed(frame2[halfLen:])
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
