package ap3000

import (
    "encoding/hex"
    "errors"
)

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

// Settlement03 结算(0x03)最小解析结果
type Settlement03 struct {
    Port        int
    DurationSec int
    Kwh01       int // 0.01kWh 单位
    Reason      int
    OrderHex    string // 16字节十六进制大写（不带0x）
}

// Decode03 按文档解析 0x03 的关键字段：时长、耗电量、端口、停止原因、订单号
// 兼容不同固件在部分扩展字段上的差异，采用顺序解析并基于最小长度校验。
func Decode03(b []byte) (*Settlement03, error) {
    // 最小需要: dur(2)+maxP(2)+kwh(2)+port(1)+mode(1)+card(4)+reason(1)+order(16) = 29
    if len(b) < 29 {
        return nil, ErrBadPayload
    }
    off := 0
    dur := int(uint16(int(b[off]) | (int(b[off+1]) << 8)))
    off += 2
    // skip maxPower
    off += 2
    kwh := int(uint16(int(b[off]) | (int(b[off+1]) << 8)))
    off += 2
    port := int(b[off])
    off += 1
    // skip mode
    off += 1
    // skip card/code (4B)
    off += 4
    reason := int(b[off])
    off += 1
    if off+16 > len(b) {
        return nil, ErrBadPayload
    }
    ord := make([]byte, 16)
    copy(ord, b[off:off+16])
    off += 16
    return &Settlement03{
        Port:        port,
        DurationSec: dur,
        Kwh01:       kwh,
        Reason:      reason,
        OrderHex:    hex.EncodeToString(ord),
    }, nil
}

// Power06 充电时功率心跳(0x06)最小解析结果
type Power06 struct {
    Port        int
    Status      int
    DurationSec int
    Kwh01       int // 累计 0.01kWh
    PowerW01    int // 0.1W 单位（可选）
    OrderHex    string
}

// Decode06 解析 0x06 的关键字段：端口、状态、累计耗电、时长、订单号、功率
func Decode06(b []byte) (*Power06, error) {
    // 最小需要: port(1)+status(1)+dur(2)+kwh(2)+mode(1)+pwr(2)+max(2)+min(2)+avg(2)+order(16) = 31
    if len(b) < 31 {
        return nil, ErrBadPayload
    }
    off := 0
    port := int(b[off])
    off += 1
    status := int(b[off])
    off += 1
    dur := int(uint16(int(b[off]) | (int(b[off+1]) << 8)))
    off += 2
    kwh := int(uint16(int(b[off]) | (int(b[off+1]) << 8)))
    off += 2
    // skip mode
    off += 1
    pwr := int(uint16(int(b[off]) | (int(b[off+1]) << 8)))
    off += 2
    // skip max/min/avg (6B)
    off += 6
    if off+16 > len(b) {
        return nil, ErrBadPayload
    }
    ord := make([]byte, 16)
    copy(ord, b[off:off+16])
    return &Power06{
        Port:        port,
        Status:      status,
        DurationSec: dur,
        Kwh01:       kwh,
        PowerW01:    pwr,
        OrderHex:    hex.EncodeToString(ord),
    }, nil
}
