package ap3000

import "errors"

// PortStatus 表示端口状态（最小字段）
type PortStatus struct {
	Port   int
	Status int
	PowerW *int
}

var ErrBadPayload = errors.New("bad payload")

// Decode20or21 解析 0x20/0x21 的最小负载：
// 约定：data[0]=port(1..n), data[1]=status，data[2..3]=powerW(le, 可选)
func Decode20or21(data []byte) (*PortStatus, error) {
	if len(data) < 2 {
		return nil, ErrBadPayload
	}
	ps := &PortStatus{Port: int(data[0]), Status: int(data[1])}
	if len(data) >= 4 {
		pw := int(int16(int(data[2]) | (int(data[3]) << 8)))
		ps.PowerW = &pw
	}
	return ps, nil
}
