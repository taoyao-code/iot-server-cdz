package api

import (
	"encoding/hex"
	"testing"

	"github.com/taoyao-code/iot-server/internal/protocol/bkv"
)

// TestEncodeStartControlPayload 测试开始充电命令编码
// 根据《设备对接指引-组网设备2024》和 docs/协议/BKV设备对接总结.md 2.1节
// 命令格式：[0x07][插座1B][插孔1B][开关1B][模式1B][时长2B][业务号2B]
func TestEncodeStartControlPayload(t *testing.T) {
	// 测试用例1: 插座2，A孔，按时长，240分钟
	payload := bkv.EncodeStartControlPayload(
		2,      // socketNo: 2号插座
		0,      // port: A孔 (0=A, 1=B)
		1,      // mode: 1=按时长
		240,    // durationMin: 240分钟
		0x0068, // businessNo: 业务号
	)

	// 期望的payload（根据协议文档 docs/协议/BKV设备对接总结.md）：
	// [0] 0x07 - BKV子命令（控制命令）
	// [1] 0x02 - 插座号=2
	// [2] 0x00 - 插孔号=0 (A孔)
	// [3] 0x01 - 开关=1 (开)
	// [4] 0x01 - 模式=1 (按时长)
	// [5] 0x00 - 时长高字节 (240 = 0x00F0)
	// [6] 0xF0 - 时长低字节
	// [7] 0x00 - 业务号高字节 (0x0068)
	// [8] 0x68 - 业务号低字节
	expected := []byte{0x07, 0x02, 0x00, 0x01, 0x01, 0x00, 0xF0, 0x00, 0x68}

	if len(payload) != len(expected) {
		t.Errorf("payload长度错误: got %d, want %d", len(payload), len(expected))
		t.Logf("实际payload: %s", hex.EncodeToString(payload))
		t.Logf("期望payload: %s", hex.EncodeToString(expected))
		return
	}

	for i := 0; i < len(expected); i++ {
		if payload[i] != expected[i] {
			t.Errorf("payload[%d]错误: got 0x%02X, want 0x%02X", i, payload[i], expected[i])
		}
	}

	t.Logf("✅ 开始充电命令编码正确")
	t.Logf("Payload (hex): %s", hex.EncodeToString(payload))
	t.Logf("Payload (详细):")
	t.Logf("  [0] 0x%02X - BKV子命令", payload[0])
	t.Logf("  [1] 0x%02X - 插座号", payload[1])
	t.Logf("  [2] 0x%02X - 插孔号 (0=A, 1=B)", payload[2])
	t.Logf("  [3] 0x%02X - 开关 (1=开, 0=关)", payload[3])
	t.Logf("  [4] 0x%02X - 模式 (1=按时, 0=按量)", payload[4])
	t.Logf("  [5-6] 0x%02X%02X - 时长 (%d分钟)", payload[5], payload[6], uint16(payload[5])<<8|uint16(payload[6]))
	t.Logf("  [7-8] 0x%02X%02X - 业务号", payload[7], payload[8])
}

// TestEncodeStopControlPayload 测试停止充电命令编码
func TestEncodeStopControlPayload(t *testing.T) {
	// 测试用例: 插座2，A孔
	payload := bkv.EncodeStopControlPayload(
		2,      // socketNo: 2号插座
		0,      // port: A孔
		0x0068, // businessNo: 业务号
	)

	// 期望的payload（根据协议文档 docs/协议/BKV设备对接总结.md）：
	// [0] 0x07 - BKV子命令
	// [1] 0x02 - 插座号=2
	// [2] 0x00 - 插孔号=0
	// [3] 0x00 - 开关=0 (关)
	// [4] 0x01 - 模式=1 (停止时无意义)
	// [5-6] 0x00 0x00 - 时长=0
	// [7-8] 0x00 0x68 - 业务号
	expected := []byte{0x07, 0x02, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x68}

	if len(payload) != len(expected) {
		t.Errorf("payload长度错误: got %d, want %d", len(payload), len(expected))
		t.Logf("实际payload: %s", hex.EncodeToString(payload))
		t.Logf("期望payload: %s", hex.EncodeToString(expected))
		return
	}

	for i := 0; i < len(expected); i++ {
		if payload[i] != expected[i] {
			t.Errorf("payload[%d]错误: got 0x%02X, want 0x%02X", i, payload[i], expected[i])
		}
	}

	t.Logf("✅ 停止充电命令编码正确")
	t.Logf("Payload (hex): %s", hex.EncodeToString(payload))
}

// TestRealWorldExample 测试真实案例：插座1，A孔，按时长60分钟
func TestRealWorldExample(t *testing.T) {
	payload := bkv.EncodeStartControlPayload(
		1,  // socketNo: 1号插座
		0,  // port: A孔 (第一个插孔)
		1,  // mode: 1=按时长
		60, // durationMin: 60分钟
		0,  // businessNo
	)

	// 协议文档示例（docs/协议/BKV设备对接总结.md）：
	// 格式：[长度2B][0x07][插座1B][插孔1B][开关1B][模式1B][时长2B][业务号2B]
	// 示例：0008 07 00 00 01 01 003c 0000
	// 我们的是：插座1，60分钟，所以应该是：
	// 07 01 00 01 01 003C 0000
	expected := []byte{0x07, 0x01, 0x00, 0x01, 0x01, 0x00, 0x3C, 0x00, 0x00}

	t.Logf("真实测试用例: 插座1, A孔, 按时长60分钟")
	t.Logf("实际payload: %s", hex.EncodeToString(payload))
	t.Logf("期望payload: %s", hex.EncodeToString(expected))

	if len(payload) != 9 {
		t.Fatalf("❌ payload长度错误: got %d, want 9", len(payload))
	}

	if payload[0] != 0x07 {
		t.Errorf("❌ 缺少BKV子命令0x07! payload[0] = 0x%02X", payload[0])
	}

	for i := 0; i < len(expected); i++ {
		if payload[i] != expected[i] {
			t.Errorf("payload[%d]错误: got 0x%02X, want 0x%02X", i, payload[i], expected[i])
		}
	}

	if t.Failed() {
		t.Logf("\n❌ 测试失败！命令格式不正确！")
	} else {
		t.Logf("\n✅ 测试通过！命令格式正确！")
	}
}
