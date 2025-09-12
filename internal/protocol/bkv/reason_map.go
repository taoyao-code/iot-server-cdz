package bkv

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ReasonMap 结束原因映射：BKV code -> 平台统一 code
type ReasonMap struct {
	Map map[int]int `yaml:"map"`
}

func LoadReasonMap(path string) (*ReasonMap, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read reason map: %w", err)
	}
	var m ReasonMap
	if err := yaml.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("unmarshal reason map: %w", err)
	}
	if m.Map == nil {
		m.Map = make(map[int]int)
	}
	return &m, nil
}

func (m *ReasonMap) Translate(bkvCode int) (int, bool) {
	if m == nil || m.Map == nil {
		return 0, false
	}
	v, ok := m.Map[bkvCode]
	return v, ok
}
