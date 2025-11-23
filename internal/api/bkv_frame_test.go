package api

import (
	"encoding/hex"
	"testing"

	"github.com/taoyao-code/iot-server/internal/protocol/bkv"
)

// TestFullBKVFrame 测试完整的BKV帧构造
func TestFullBKVFrame(t *testing.T) {
	// 1. 构造payload
	payload := bkv.EncodeStartControlPayload(1, 0, 1, 60, 0)
	t.Logf("Payload (9字节): %s", hex.EncodeToString(payload))

	// 2. 构造完整BKV帧
	gatewayID := "82241218000382"
	msgID := uint32(12345)
	frame := bkv.Build(0x0015, msgID, gatewayID, payload)

	t.Logf("完整BKV帧 (%d字节): %s", len(frame), hex.EncodeToString(frame))

	// 3. 解析帧结构
	t.Logf("\n帧结构分析:")
	t.Logf("  [0-1]   包头: %s (应为fcff)", hex.EncodeToString(frame[0:2]))
	t.Logf("  [2-3]   长度: %s (数据部分长度)", hex.EncodeToString(frame[2:4]))
	t.Logf("  [4-5]   命令: %s (应为0015)", hex.EncodeToString(frame[4:6]))
	t.Logf("  [6-9]   MsgID: %s", hex.EncodeToString(frame[6:10]))
	t.Logf("  [10]    方向: %02X (00=下行)", frame[10])
	t.Logf("  [11-17] 网关ID: %s", hex.EncodeToString(frame[11:18]))

	dataStart := 18
	dataEnd := len(frame) - 3 // 去掉校验和(1) + 包尾(2)
	t.Logf("  [%d-%d] Payload数据: %s", dataStart, dataEnd-1, hex.EncodeToString(frame[dataStart:dataEnd]))
	t.Logf("  [%d]   校验和: %02X", dataEnd, frame[dataEnd])
	t.Logf("  [%d-%d] 包尾: %s (应为fcee)", dataEnd+1, dataEnd+2, hex.EncodeToString(frame[dataEnd+1:]))

	// 4. 验证payload是否正确嵌入
	actualPayload := frame[dataStart:dataEnd]
	if len(actualPayload) != 9 {
		t.Errorf("❌ Payload长度错误: got %d, want 9", len(actualPayload))
	}

	if actualPayload[0] != 0x07 {
		t.Errorf("❌ 第一个字节不是0x07! got 0x%02X", actualPayload[0])
	}

	// 5. 计算期望的总长度
	// BKV帧 = 包头(2) + 长度(2) + 命令(2) + MsgID(4) + 方向(1) + 网关ID(7) + Payload(9) + 校验(1) + 包尾(2)
	expectedLen := 2 + 2 + 2 + 4 + 1 + 7 + 9 + 1 + 2
	t.Logf("\n期望总长度: %d 字节", expectedLen)
	t.Logf("实际总长度: %d 字节", len(frame))

	if len(frame) == expectedLen {
		t.Logf("\n✅ BKV帧长度正确！")
	} else {
		t.Errorf("\n❌ BKV帧长度错误！")
	}

	// 对比协议文档示例（第254行）
	// fcff001c0015001c9a5100860044594530050008070200010100f00000c8fcee
	// 其中 0008 是数据长度，07020001010 0f00000 是payload
	t.Logf("\n协议文档示例（插座2，A孔，240分钟）:")
	t.Logf("  完整帧: fcff001c0015001c9a5100860044594530050008070200010100f00000c8fcee")
	t.Logf("  Payload: 0702000101 00f0 0000 (9字节)")
}
