package gn

import (
	"context"
	"sync"
	"testing"
	"time"

	gnStorage "github.com/taoyao-code/iot-server/internal/storage/gn"
)

// MockOutboundQueueRepo 模拟出站队列仓储
type MockOutboundQueueRepo struct {
	mu       sync.RWMutex
	messages []gnStorage.OutboundMessage
	nextID   int64
}

func NewMockOutboundQueueRepo() *MockOutboundQueueRepo {
	return &MockOutboundQueueRepo{
		messages: make([]gnStorage.OutboundMessage, 0),
		nextID:   1,
	}
}

func (r *MockOutboundQueueRepo) Enqueue(ctx context.Context, deviceID string, cmd int, seq int, payload []byte) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	msg := gnStorage.OutboundMessage{
		ID:       r.nextID,
		DeviceID: deviceID,
		Cmd:      cmd,
		Seq:      seq,
		Payload:  payload,
		Status:   0, // pending
		Tries:    0,
		NextTS:   time.Now(),
	}

	r.messages = append(r.messages, msg)
	r.nextID++

	return msg.ID, nil
}

func (r *MockOutboundQueueRepo) DequeueDue(ctx context.Context, limit int) ([]gnStorage.OutboundMessage, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var dueMessages []gnStorage.OutboundMessage
	now := time.Now()

	count := 0
	for _, msg := range r.messages {
		if count >= limit {
			break
		}

		if (msg.Status == 0 || msg.Status == 1) && msg.NextTS.Before(now) {
			dueMessages = append(dueMessages, msg)
			count++
		}
	}

	return dueMessages, nil
}

func (r *MockOutboundQueueRepo) MarkSent(ctx context.Context, id int64, nextTS time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i := range r.messages {
		if r.messages[i].ID == id {
			r.messages[i].Status = 1 // sent
			r.messages[i].Tries++
			r.messages[i].NextTS = nextTS
			break
		}
	}
	return nil
}

func (r *MockOutboundQueueRepo) Ack(ctx context.Context, deviceID string, seq int) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i := range r.messages {
		if r.messages[i].DeviceID == deviceID && r.messages[i].Seq == seq {
			r.messages[i].Status = 2 // acked
			break
		}
	}
	return nil
}

func (r *MockOutboundQueueRepo) MarkDead(ctx context.Context, id int64, reason string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i := range r.messages {
		if r.messages[i].ID == id {
			r.messages[i].Status = 3 // dead
			break
		}
	}
	return nil
}

func (r *MockOutboundQueueRepo) ListStuckSince(ctx context.Context, ts time.Time) ([]gnStorage.OutboundMessage, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var stuckMessages []gnStorage.OutboundMessage

	for _, msg := range r.messages {
		if msg.Status == 1 && msg.NextTS.Before(ts) {
			stuckMessages = append(stuckMessages, msg)
		}
	}

	return stuckMessages, nil
}

func (r *MockOutboundQueueRepo) GetMessageByID(id int64) *gnStorage.OutboundMessage {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, msg := range r.messages {
		if msg.ID == id {
			// 返回副本以避免外部修改
			msgCopy := msg
			return &msgCopy
		}
	}
	return nil
}

func TestWorker_BasicFlow(t *testing.T) {
	// 创建模拟依赖
	outboundRepo := NewMockOutboundQueueRepo()
	sender := NewMockSender()

	// 创建模拟的repos（只需要outbound）
	repos := &gnStorage.PostgresRepos{
		Outbound: outboundRepo,
	}

	// 创建worker
	config := WorkerConfig{
		ScanInterval: 100 * time.Millisecond,
		RetryBackoff: 50 * time.Millisecond,
		MaxRetries:   1,
		BatchSize:    10,
	}

	worker := NewWorker(repos, sender, config)

	ctx := context.Background()

	// 入队一条消息
	deviceID := "82200520004869"
	cmd := 0x0000
	seq := 12345
	payload := []byte("test_payload")

	msgID, err := worker.EnqueueMessage(ctx, deviceID, cmd, seq, payload)
	if err != nil {
		t.Fatalf("EnqueueMessage failed: %v", err)
	}

	t.Logf("Enqueued message ID: %d", msgID)

	// 启动worker
	err = worker.Start(ctx)
	if err != nil {
		t.Fatalf("Start worker failed: %v", err)
	}
	defer worker.Stop()

	// 等待消息被处理
	time.Sleep(200 * time.Millisecond)

	// 验证消息被发送
	sentMessages := sender.GetSentMessages()
	if len(sentMessages) != 1 {
		t.Fatalf("Expected 1 sent message, got %d", len(sentMessages))
	}

	sentMsg := sentMessages[0]
	if sentMsg.DeviceID != deviceID {
		t.Errorf("DeviceID mismatch: expected %s, got %s", deviceID, sentMsg.DeviceID)
	}

	// 验证指标
	metrics := worker.GetMetrics()
	if metrics.SentTotal != 1 {
		t.Errorf("Expected 1 sent message in metrics, got %d", metrics.SentTotal)
	}
	if metrics.InFlight != 1 {
		t.Errorf("Expected 1 in-flight message, got %d", metrics.InFlight)
	}

	// 确认消息
	err = worker.AckMessage(ctx, deviceID, seq)
	if err != nil {
		t.Fatalf("AckMessage failed: %v", err)
	}

	// 验证ACK后的指标
	metrics = worker.GetMetrics()
	if metrics.AcksTotal != 1 {
		t.Errorf("Expected 1 acked message in metrics, got %d", metrics.AcksTotal)
	}
	if metrics.InFlight != 0 {
		t.Errorf("Expected 0 in-flight messages after ACK, got %d", metrics.InFlight)
	}

	t.Logf("Final metrics: %+v", metrics)
}

func TestWorker_RetryLogic(t *testing.T) {
	outboundRepo := NewMockOutboundQueueRepo()
	sender := NewMockSender()

	repos := &gnStorage.PostgresRepos{
		Outbound: outboundRepo,
	}

	config := WorkerConfig{
		ScanInterval: 100 * time.Millisecond,
		RetryBackoff: 50 * time.Millisecond,
		MaxRetries:   5, // 设置足够的重试次数
		BatchSize:    10,
	}

	worker := NewWorker(repos, sender, config)
	ctx := context.Background()

	// 入队消息
	deviceID := "82200520004869"
	_, err := worker.EnqueueMessage(ctx, deviceID, 0x0000, 12345, []byte("test"))
	if err != nil {
		t.Fatalf("EnqueueMessage failed: %v", err)
	}

	// 启动worker
	err = worker.Start(ctx)
	if err != nil {
		t.Fatalf("Start worker failed: %v", err)
	}
	defer worker.Stop()

	// 设置发送器失败，等待第一次发送失败
	sender.SetShouldFail(true)
	time.Sleep(150 * time.Millisecond)

	// 验证重试计数增加
	metrics1 := worker.GetMetrics()
	if metrics1.RetriesTotal == 0 {
		t.Error("Expected some retry attempts")
	}

	// 修复发送器
	sender.SetShouldFail(false)

	// 等待重试成功
	time.Sleep(200 * time.Millisecond)

	// 验证最终指标
	finalMetrics := worker.GetMetrics()

	// 无论成功或失败，都应该有重试尝试
	if finalMetrics.RetriesTotal == 0 {
		t.Error("Expected retry attempts")
	}

	// 检查消息状态：应该成功发送或者最终死信
	if finalMetrics.SentTotal == 0 && finalMetrics.DeadTotal == 0 {
		t.Error("Expected either successful send or dead message")
	}

	t.Logf("Retry test metrics: %+v", finalMetrics)
	t.Logf("Sent messages count: %d", len(sender.GetSentMessages()))
}

func TestWorker_MaxRetriesExceeded(t *testing.T) {
	outboundRepo := NewMockOutboundQueueRepo()
	sender := NewMockSender()

	repos := &gnStorage.PostgresRepos{
		Outbound: outboundRepo,
	}

	config := WorkerConfig{
		ScanInterval: 50 * time.Millisecond,
		RetryBackoff: 50 * time.Millisecond,
		MaxRetries:   1, // 只允许1次重试
		BatchSize:    10,
	}

	worker := NewWorker(repos, sender, config)
	ctx := context.Background()

	// 设置发送器始终失败
	sender.SetShouldFail(true)

	// 入队消息
	deviceID := "82200520004869"
	msgID, err := worker.EnqueueMessage(ctx, deviceID, 0x0000, 12345, []byte("test"))
	if err != nil {
		t.Fatalf("EnqueueMessage failed: %v", err)
	}

	// 手动设置消息已达到最大重试次数
	msg := outboundRepo.GetMessageByID(msgID)
	if msg != nil {
		msg.Tries = config.MaxRetries
	}

	// 启动worker
	err = worker.Start(ctx)
	if err != nil {
		t.Fatalf("Start worker failed: %v", err)
	}
	defer worker.Stop()

	// 等待消息被标记为死信
	time.Sleep(200 * time.Millisecond)

	// 验证消息被标记为死信
	finalMsg := outboundRepo.GetMessageByID(msgID)
	if finalMsg == nil {
		t.Fatal("Message not found")
	}
	if finalMsg.Status != 3 { // 3 = dead
		t.Errorf("Expected message status 3 (dead), got %d", finalMsg.Status)
	}

	// 验证死信指标
	metrics := worker.GetMetrics()
	if metrics.DeadTotal != 1 {
		t.Errorf("Expected 1 dead message, got %d", metrics.DeadTotal)
	}

	t.Logf("Dead message test metrics: %+v", metrics)
}

func TestWorker_ColdStart(t *testing.T) {
	outboundRepo := NewMockOutboundQueueRepo()
	sender := NewMockSender()

	repos := &gnStorage.PostgresRepos{
		Outbound: outboundRepo,
	}

	config := WorkerConfig{
		ScanInterval: 500 * time.Millisecond, // 增加扫描间隔，避免与冷启动重复
		RetryBackoff: 50 * time.Millisecond,
		MaxRetries:   2,
		BatchSize:    10,
	}

	// 预先创建一个"卡住"的消息（status=1, next_ts已过期）
	stuckMsg := gnStorage.OutboundMessage{
		ID:       1,
		DeviceID: "82200520004869",
		Cmd:      0x1000,
		Seq:      54321,
		Payload:  []byte("stuck_message"),
		Status:   1, // sent but stuck
		Tries:    0,
		NextTS:   time.Now().Add(-100 * time.Millisecond), // 过期时间
	}

	outboundRepo.messages = append(outboundRepo.messages, stuckMsg)
	outboundRepo.nextID = 2

	worker := NewWorker(repos, sender, config)
	ctx := context.Background()

	// 启动worker (会触发冷启动扫描)
	err := worker.Start(ctx)
	if err != nil {
		t.Fatalf("Start worker failed: %v", err)
	}
	defer worker.Stop()

	// 等待冷启动扫描处理卡住的消息
	time.Sleep(200 * time.Millisecond)

	// 验证卡住的消息被重新发送
	sentMessages := sender.GetSentMessages()
	if len(sentMessages) == 0 {
		t.Fatal("Expected at least 1 sent message from cold-start")
	}

	// 找到包含我们预期设备的消息
	found := false
	for _, sentMsg := range sentMessages {
		if sentMsg.DeviceID == stuckMsg.DeviceID {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("Expected to find message for device %s", stuckMsg.DeviceID)
	}

	// 验证指标
	metrics := worker.GetMetrics()
	if metrics.SentTotal == 0 {
		t.Error("Expected at least 1 sent message from cold-start")
	}

	t.Logf("Cold-start test metrics: %+v", metrics)
	t.Logf("Sent %d messages during cold-start", len(sentMessages))
}

func TestMockSender(t *testing.T) {
	sender := NewMockSender()
	ctx := context.Background()

	// 测试正常发送
	err := sender.Send(ctx, "device1", []byte("test_data"))
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// 验证消息被记录
	sentMessages := sender.GetSentMessages()
	if len(sentMessages) != 1 {
		t.Fatalf("Expected 1 sent message, got %d", len(sentMessages))
	}

	msg := sentMessages[0]
	if msg.DeviceID != "device1" {
		t.Errorf("DeviceID mismatch: expected device1, got %s", msg.DeviceID)
	}
	if string(msg.Data) != "test_data" {
		t.Errorf("Data mismatch: expected test_data, got %s", string(msg.Data))
	}

	// 测试失败模式
	sender.SetShouldFail(true)
	err = sender.Send(ctx, "device2", []byte("test_data2"))
	if err == nil {
		t.Error("Expected send to fail")
	}

	// 验证失败时没有新消息
	sentMessages = sender.GetSentMessages()
	if len(sentMessages) != 1 {
		t.Errorf("Expected still 1 sent message after failure, got %d", len(sentMessages))
	}

	// 重置发送器
	sender.Reset()
	sentMessages = sender.GetSentMessages()
	if len(sentMessages) != 0 {
		t.Errorf("Expected 0 sent messages after reset, got %d", len(sentMessages))
	}
}
