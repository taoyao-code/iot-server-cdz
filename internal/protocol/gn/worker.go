package gn

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"sync"
	"time"

	gnStorage "github.com/taoyao-code/iot-server/internal/storage/gn"
)

// Worker GN协议出站可靠性工作器
type Worker struct {
	mu           sync.RWMutex
	running      bool
	done         chan struct{}
	wg           sync.WaitGroup
	
	repos        *gnStorage.PostgresRepos
	sender       Sender
	
	// 配置
	scanInterval time.Duration // 扫描间隔
	retryBackoff time.Duration // 重试退避时间
	maxRetries   int           // 最大重试次数
	batchSize    int           // 批处理大小
	
	// 指标
	metrics      *WorkerMetrics
}

// Sender 发送器接口
type Sender interface {
	// Send 发送消息到设备
	Send(ctx context.Context, deviceID string, data []byte) error
}

// WorkerMetrics 工作器指标
type WorkerMetrics struct {
	SentTotal    int64 `json:"sent_total"`
	RetriesTotal int64 `json:"retries_total"`
	AcksTotal    int64 `json:"acks_total"`
	DeadTotal    int64 `json:"dead_total"`
	InFlight     int64 `json:"in_flight"`
}

// WorkerConfig 工作器配置
type WorkerConfig struct {
	ScanInterval time.Duration
	RetryBackoff time.Duration
	MaxRetries   int
	BatchSize    int
}

// DefaultWorkerConfig 默认工作器配置
func DefaultWorkerConfig() WorkerConfig {
	return WorkerConfig{
		ScanInterval: 5 * time.Second,
		RetryBackoff: 15 * time.Second,
		MaxRetries:   1,
		BatchSize:    50,
	}
}

// NewWorker 创建新的工作器
func NewWorker(repos *gnStorage.PostgresRepos, sender Sender, config WorkerConfig) *Worker {
	return &Worker{
		repos:        repos,
		sender:       sender,
		done:         make(chan struct{}),
		scanInterval: config.ScanInterval,
		retryBackoff: config.RetryBackoff,
		maxRetries:   config.MaxRetries,
		batchSize:    config.BatchSize,
		metrics:      &WorkerMetrics{},
	}
}

// Start 启动工作器
func (w *Worker) Start(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	
	if w.running {
		return fmt.Errorf("worker already running")
	}
	
	w.running = true
	
	// 启动冷启动扫描
	w.wg.Add(1)
	go w.coldStartScan(ctx)
	
	// 启动定期扫描
	w.wg.Add(1)
	go w.periodicScan(ctx)
	
	log.Printf("GN outbound worker started")
	return nil
}

// Stop 停止工作器
func (w *Worker) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	
	if !w.running {
		return
	}
	
	w.running = false
	close(w.done)
	w.wg.Wait()
	
	log.Printf("GN outbound worker stopped")
}

// IsRunning 检查是否正在运行
func (w *Worker) IsRunning() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.running
}

// GetMetrics 获取指标
func (w *Worker) GetMetrics() WorkerMetrics {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return *w.metrics
}

// coldStartScan 冷启动扫描：查找卡住的消息并重新发送
func (w *Worker) coldStartScan(ctx context.Context) {
	defer w.wg.Done()
	
	log.Printf("GN worker: starting cold-start scan")
	
	// 查找所有卡住的消息（status=1且next_ts已过期）
	stuckSince := time.Now().Add(-w.retryBackoff)
	stuckMessages, err := w.repos.Outbound.ListStuckSince(ctx, stuckSince)
	if err != nil {
		log.Printf("GN worker: cold-start scan failed: %v", err)
		return
	}
	
	log.Printf("GN worker: found %d stuck messages for cold-start", len(stuckMessages))
	
	for _, msg := range stuckMessages {
		select {
		case <-w.done:
			return
		default:
		}
		
		if err := w.processMessage(ctx, msg); err != nil {
			log.Printf("GN worker: cold-start process message %d failed: %v", msg.ID, err)
		}
	}
	
	log.Printf("GN worker: cold-start scan completed")
}

// periodicScan 定期扫描：处理到期的消息
func (w *Worker) periodicScan(ctx context.Context) {
	defer w.wg.Done()
	
	ticker := time.NewTicker(w.scanInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-w.done:
			return
		case <-ticker.C:
			w.scanAndProcess(ctx)
		}
	}
}

// scanAndProcess 扫描并处理到期消息
func (w *Worker) scanAndProcess(ctx context.Context) {
	// 获取到期的消息
	messages, err := w.repos.Outbound.DequeueDue(ctx, w.batchSize)
	if err != nil {
		log.Printf("GN worker: dequeue failed: %v", err)
		return
	}
	
	if len(messages) == 0 {
		return
	}
	
	log.Printf("GN worker: processing %d messages", len(messages))
	
	for _, msg := range messages {
		select {
		case <-w.done:
			return
		default:
		}
		
		if err := w.processMessage(ctx, msg); err != nil {
			log.Printf("GN worker: process message %d failed: %v", msg.ID, err)
		}
	}
}

// processMessage 处理单个消息
func (w *Worker) processMessage(ctx context.Context, msg gnStorage.OutboundMessage) error {
	// 检查重试次数
	if msg.Tries >= w.maxRetries {
		// 标记为死信
		err := w.repos.Outbound.MarkDead(ctx, msg.ID, "max_retries_exceeded")
		if err != nil {
			return fmt.Errorf("mark dead failed: %w", err)
		}
		
		w.mu.Lock()
		w.metrics.DeadTotal++
		w.mu.Unlock()
		
		log.Printf("GN worker: message %d marked as dead (max retries exceeded)", msg.ID)
		return nil
	}
	
	// 发送消息
	err := w.sendMessage(ctx, msg)
	if err != nil {
		log.Printf("GN worker: send message %d failed: %v", msg.ID, err)
		
		// 计算下次重试时间
		nextTS := time.Now().Add(w.retryBackoff)
		
		// 更新状态为已发送（准备重试）
		if markErr := w.repos.Outbound.MarkSent(ctx, msg.ID, nextTS); markErr != nil {
			log.Printf("GN worker: mark sent failed: %v", markErr)
		}
		
		w.mu.Lock()
		w.metrics.RetriesTotal++
		w.mu.Unlock()
		
		return err
	}
	
	// 发送成功，标记为已发送
	nextTS := time.Now().Add(w.retryBackoff)
	err = w.repos.Outbound.MarkSent(ctx, msg.ID, nextTS)
	if err != nil {
		return fmt.Errorf("mark sent failed: %w", err)
	}
	
	w.mu.Lock()
	w.metrics.SentTotal++
	w.metrics.InFlight++
	w.mu.Unlock()
	
	log.Printf("GN worker: message %d sent successfully", msg.ID)
	return nil
}

// sendMessage 发送消息
func (w *Worker) sendMessage(ctx context.Context, msg gnStorage.OutboundMessage) error {
	// 构建GN协议帧
	gwid, err := hex.DecodeString(msg.DeviceID) // 假设deviceID是网关ID的hex格式
	if err != nil || len(gwid) != 7 {
		return fmt.Errorf("invalid device ID format: %s", msg.DeviceID)
	}
	
	// 创建下行帧
	frame, err := NewFrame(uint16(msg.Cmd), uint32(msg.Seq), gwid, msg.Payload, true)
	if err != nil {
		return fmt.Errorf("create frame failed: %w", err)
	}
	
	// 编码帧
	data, err := frame.Encode()
	if err != nil {
		return fmt.Errorf("encode frame failed: %w", err)
	}
	
	// 发送数据
	return w.sender.Send(ctx, msg.DeviceID, data)
}

// AckMessage 确认消息（由路由器调用）
func (w *Worker) AckMessage(ctx context.Context, deviceID string, seq int) error {
	err := w.repos.Outbound.Ack(ctx, deviceID, seq)
	if err != nil {
		return err
	}
	
	w.mu.Lock()
	w.metrics.AcksTotal++
	if w.metrics.InFlight > 0 {
		w.metrics.InFlight--
	}
	w.mu.Unlock()
	
	log.Printf("GN worker: message acked for device %s, seq %d", deviceID, seq)
	return nil
}

// EnqueueMessage 入队消息（由业务逻辑调用）
func (w *Worker) EnqueueMessage(ctx context.Context, deviceID string, cmd int, seq int, payload []byte) (int64, error) {
	return w.repos.Outbound.Enqueue(ctx, deviceID, cmd, seq, payload)
}

// MockSender 用于测试的模拟发送器
type MockSender struct {
	sentMessages []SentMessage
	shouldFail   bool
}

type SentMessage struct {
	DeviceID string
	Data     []byte
	SentAt   time.Time
}

func NewMockSender() *MockSender {
	return &MockSender{
		sentMessages: make([]SentMessage, 0),
	}
}

func (s *MockSender) Send(ctx context.Context, deviceID string, data []byte) error {
	if s.shouldFail {
		return fmt.Errorf("mock send failure")
	}
	
	s.sentMessages = append(s.sentMessages, SentMessage{
		DeviceID: deviceID,
		Data:     data,
		SentAt:   time.Now(),
	})
	
	return nil
}

func (s *MockSender) SetShouldFail(fail bool) {
	s.shouldFail = fail
}

func (s *MockSender) GetSentMessages() []SentMessage {
	return s.sentMessages
}

func (s *MockSender) Reset() {
	s.sentMessages = s.sentMessages[:0]
}