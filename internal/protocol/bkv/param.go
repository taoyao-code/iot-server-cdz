package bkv

// Control 帧最小解析结果（占位）：port 与 action（1=start,2=stop）
type Control struct {
	Port   int
	Action int
}

// DecodeControl 解析控制帧的最小字段：data[0]=port, data[1]=action（不足则使用默认）
func DecodeControl(b []byte) *Control {
	c := &Control{Port: 1, Action: 0}
	if len(b) >= 1 {
		c.Port = int(b[0])
	}
	if len(b) >= 2 {
		c.Action = int(b[1])
	}
	return c
}

// ParamWrite 最小解析结果（占位）：paramId 与原始值字节
type ParamWrite struct {
	ParamID int
	Value   []byte
}

// DecodeParamWrite 解析 0x83/0x84 写入类负载：data[0]=paramId, data[1]=len, data[2..] 值
// 若长度不足则返回占位，但 Value 可能为空。
func DecodeParamWrite(b []byte) *ParamWrite {
	pw := &ParamWrite{ParamID: 0, Value: nil}
	if len(b) == 0 {
		return pw
	}
	pw.ParamID = int(b[0])
	if len(b) >= 2 {
		l := int(b[1])
		if l > 0 && len(b) >= 2+l {
			pw.Value = make([]byte, l)
			copy(pw.Value, b[2:2+l])
		}
	}
	return pw
}

// ParamReadback 最小解析结果（占位）：paramId 与回读值
type ParamReadback struct {
	ParamID int
	Value   []byte
}

// DecodeParamReadback 解析 0x85 回读：data[0]=paramId, data[1]=len, data[2..] 值
func DecodeParamReadback(b []byte) *ParamReadback {
	pr := &ParamReadback{ParamID: 0, Value: nil}
	if len(b) == 0 {
		return pr
	}
	pr.ParamID = int(b[0])
	if len(b) >= 2 {
		l := int(b[1])
		if l > 0 && len(b) >= 2+l {
			pr.Value = make([]byte, l)
			copy(pr.Value, b[2:2+l])
		}
	}
	return pr
}
