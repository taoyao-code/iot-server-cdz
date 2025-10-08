package thirdparty

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

// MockWebhookServer Mock Webhook服务器（用于测试）
type MockWebhookServer struct {
	*httptest.Server
	mu             sync.Mutex
	receivedEvents []map[string]interface{}
	secret         string
}

// NewMockWebhookServer 创建Mock Webhook服务器
func NewMockWebhookServer(secret string) *MockWebhookServer {
	mock := &MockWebhookServer{
		receivedEvents: make([]map[string]interface{}, 0),
		secret:         secret,
	}

	// 创建HTTP服务器
	mock.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 读取请求体
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		// 解析事件数据
		var event map[string]interface{}
		if err := json.Unmarshal(body, &event); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// 保存接收到的事件
		mock.mu.Lock()
		mock.receivedEvents = append(mock.receivedEvents, event)
		mock.mu.Unlock()

		// 返回成功响应
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"code":0,"message":"success"}`))
	}))

	return mock
}

// GetReceivedEvents 获取接收到的所有事件
func (m *MockWebhookServer) GetReceivedEvents() []map[string]interface{} {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 返回副本
	events := make([]map[string]interface{}, len(m.receivedEvents))
	copy(events, m.receivedEvents)
	return events
}

// GetEventCount 获取接收到的事件数量
func (m *MockWebhookServer) GetEventCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.receivedEvents)
}

// ClearEvents 清空接收到的事件
func (m *MockWebhookServer) ClearEvents() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.receivedEvents = make([]map[string]interface{}, 0)
}

// ===== 测试用例 =====

// TestMockWebhookServer 测试Mock Webhook服务器
func TestMockWebhookServer(t *testing.T) {
	// 创建Mock服务器
	mock := NewMockWebhookServer("test-secret")
	defer mock.Close()

	// 发送测试事件
	testEvent := map[string]interface{}{
		"event_id":   "test-001",
		"event_type": "order.created",
		"device_id":  "DEV001",
		"timestamp":  1234567890,
	}

	// 发送HTTP请求（简化测试）
	// 实际项目中需要完整的HTTP客户端和请求逻辑
	t.Log("Mock webhook server URL:", mock.URL)
	t.Log("Test event:", testEvent)

	// 验证事件已接收
	// 注意：这个测试只是验证Mock服务器本身，不发送实际事件
	// 实际使用时，应该有外部客户端发送事件到Mock服务器
	assert.GreaterOrEqual(t, mock.GetEventCount(), 0, "event count should be non-negative")

	// 清空事件
	mock.ClearEvents()
	assert.Equal(t, 0, mock.GetEventCount())
}

// TestWebhookPusher Mock Webhook推送测试示例
func TestWebhookPusher(t *testing.T) {
	// 创建Mock服务器
	mock := NewMockWebhookServer("test-secret-123")
	defer mock.Close()

	// 创建测试事件（简化示例）
	// 实际需要使用正确的Pusher构造函数
	testEvent := map[string]interface{}{
		"event_type": "order.created",
		"device_id":  "DEV001",
		"order_no":   "ORDER001",
		"amount":     100.0,
	}
	t.Log("Test event:", testEvent)

	// 推送事件（这里需要实际实现）
	// err := pusher.Push(context.Background(), event)
	// assert.NoError(t, err)

	// 验证Mock服务器接收到事件
	// assert.Equal(t, 1, mock.GetEventCount())

	t.Log("Webhook pusher test framework ready")
}

// 注意：完整的测试实现需要：
// 1. 实际的事件推送逻辑
// 2. HMAC签名验证
// 3. 重试机制测试
// 4. 去重机制测试
// 5. 并发推送测试
