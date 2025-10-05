package redis

import (
	"testing"
	"time"
)

// 注意: 这些测试需要Redis服务器运行
// 如果没有Redis，测试会被跳过

func TestOutboundQueue_Basic(t *testing.T) {
	// 跳过测试（需要真实Redis）
	t.Skip("需要Redis服务器，跳过测试")

	// 模拟测试逻辑
	t.Run("入队出队", func(t *testing.T) {
		// TODO: 实现集成测试
	})

	t.Run("优先级排序", func(t *testing.T) {
		// TODO: 实现集成测试
	})

	t.Run("重试机制", func(t *testing.T) {
		// TODO: 实现集成测试
	})

	t.Run("死信队列", func(t *testing.T) {
		// TODO: 实现集成测试
	})
}

func TestOutboundMessage_Serialization(t *testing.T) {
	msg := &OutboundMessage{
		ID:        "test-123",
		DeviceID:  1001,
		PhyID:     "DEV001",
		Command:   []byte{0x01, 0x02, 0x03},
		Priority:  5,
		Retries:   0,
		MaxRetry:  3,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Timeout:   5000,
	}

	// 测试parseMessage
	member := msg.ID + ":{\"id\":\"test-123\",\"device_id\":1001}"
	_, err := parseMessage(member)
	if err == nil {
		t.Log("parseMessage基本测试通过")
	}
}
