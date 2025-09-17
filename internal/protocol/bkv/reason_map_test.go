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

func TestDefaultReasonMap(t *testing.T) {
	m := DefaultReasonMap()
	
	// 测试一些基本的映射
	testCases := []struct {
		bkvCode    int
		expectedOk bool
		desc       string
	}{
		{0, true, "正常结束"},
		{1, true, "空载结束"},
		{8, true, "欠压保护"},
		{99, false, "未知原因(99)"},
	}
	
	for _, tc := range testCases {
		v, ok := m.Translate(tc.bkvCode)
		if ok != tc.expectedOk {
			t.Errorf("code %d: expected ok=%v, got %v", tc.bkvCode, tc.expectedOk, ok)
		}
		if ok && v != tc.bkvCode {
			t.Errorf("code %d: expected value=%d, got %d", tc.bkvCode, tc.bkvCode, v)
		}
		
		desc := m.GetReasonDescription(tc.bkvCode)
		if desc != tc.desc {
			t.Errorf("code %d: expected desc=%s, got %s", tc.bkvCode, tc.desc, desc)
		}
	}
}

func TestReasonMap_Merge(t *testing.T) {
	m1 := DefaultReasonMap()
	m2 := &ReasonMap{
		Map: map[int]int{
			1:  101, // 覆盖已有的
			12: 112, // 新增的
		},
	}
	
	m1.Merge(m2)
	
	// 验证覆盖
	if v, ok := m1.Translate(1); !ok || v != 101 {
		t.Errorf("merge override failed: got %v, %v", v, ok)
	}
	
	// 验证新增
	if v, ok := m1.Translate(12); !ok || v != 112 {
		t.Errorf("merge add failed: got %v, %v", v, ok)
	}
	
	// 验证原有未被影响的
	if v, ok := m1.Translate(0); !ok || v != 0 {
		t.Errorf("merge preserved failed: got %v, %v", v, ok)
	}
}
