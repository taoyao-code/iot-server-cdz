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

// DefaultReasonMap 返回默认的结束原因映射
func DefaultReasonMap() *ReasonMap {
	return &ReasonMap{
		Map: map[int]int{
			// BKV 协议结束原因 -> 平台统一错误码
			0:  0,  // 正常结束
			1:  1,  // 空载结束
			2:  2,  // 满额结束
			3:  3,  // 超时结束
			4:  4,  // 人工结束
			5:  5,  // 系统停止
			6:  6,  // 过流保护
			7:  7,  // 过压保护
			8:  8,  // 欠压保护
			9:  9,  // 过温保护
			10: 10, // 设备故障
			11: 11, // 通信故障
		},
	}
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

// GetReasonDescription 获取结束原因的描述
func (m *ReasonMap) GetReasonDescription(bkvCode int) string {
	descriptions := map[int]string{
		0:  "正常结束",
		1:  "空载结束",
		2:  "满额结束",
		3:  "超时结束",
		4:  "人工结束",
		5:  "系统停止",
		6:  "过流保护",
		7:  "过压保护",
		8:  "欠压保护",
		9:  "过温保护",
		10: "设备故障",
		11: "通信故障",
	}

	if desc, ok := descriptions[bkvCode]; ok {
		return desc
	}
	return fmt.Sprintf("未知原因(%d)", bkvCode)
}

// Merge 合并另一个ReasonMap的映射规则
func (m *ReasonMap) Merge(other *ReasonMap) {
	if m == nil || m.Map == nil || other == nil || other.Map == nil {
		return
	}
	for k, v := range other.Map {
		m.Map[k] = v
	}
}
