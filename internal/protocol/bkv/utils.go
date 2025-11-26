package bkv

import (
	"fmt"
	"time"

	"github.com/taoyao-code/iot-server/internal/coremodel"
)

// extractDeviceID 从 Frame 中提取设备ID，如果为空则返回错误
func extractDeviceID(f *Frame) (string, error) {
	deviceID := f.GatewayID
	if deviceID == "" {
		return "", fmt.Errorf("missing gateway ID")
	}
	return deviceID, nil
}

// extractDeviceIDOrDefault 从 Frame 中提取设备ID，如果为空则使用默认值
func extractDeviceIDOrDefault(f *Frame) string {
	if f.GatewayID != "" {
		return f.GatewayID
	}
	return "BKV-UNKNOWN"
}

// extractDeviceIDFromPayload 从 Payload 中提取设备ID，支持回退到 Frame
func extractDeviceIDFromPayload(f *Frame, payload *BKVPayload) string {
	if payload != nil && payload.GatewayID != "" {
		return payload.GatewayID
	}
	return extractDeviceIDOrDefault(f)
}

// now 获取当前时间（便于测试时 mock）
func now() time.Time {
	return time.Now()
}

// toBCD 将整数转换为BCD编码
func toBCD(v int) byte {
	if v < 0 {
		v = 0
	}
	if v > 99 {
		v = v % 100
	}
	hi := (v / 10) & 0x0F
	lo := (v % 10) & 0x0F
	return byte(hi<<4 | lo)
}

// encodeHeartbeatAck 构造心跳ACK的payload（当前时间）
// 按协议文档使用7字节BCD时间戳: YYYYMMDDHHMMSS
func encodeHeartbeatAck(gatewayID string) []byte {
	t := now()
	year := t.Year()
	yy1 := year / 100
	yy2 := year % 100

	return []byte{
		toBCD(yy1),
		toBCD(yy2),
		toBCD(int(t.Month())),
		toBCD(t.Day()),
		toBCD(t.Hour()),
		toBCD(t.Minute()),
		toBCD(t.Second()),
	}
}

// extractEndReason 从插座状态中提取结束原因
// 委托给 coremodel.DeriveEndReasonFromStatus 实现
func extractEndReason(status uint8) int {
	reasonCode := coremodel.DeriveEndReasonFromStatus(coremodel.RawPortStatus(status))
	// 返回 coremodel 的结束原因码（int）
	return int(reasonCode)
}
