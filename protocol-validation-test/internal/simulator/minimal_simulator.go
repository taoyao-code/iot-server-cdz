package simulator

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/rand"
	"sync"
	"time"
)

// MinimalSimulator 最小行为模拟器
// 仅包含最小连接/调度/错误注入能力，不做真实物理仿真
type MinimalSimulator struct {
	config      *SimulatorConfig
	connections map[string]*Connection
	tasks       map[string]*ScheduledTask
	stats       *ConcurrencyStats
	mu          sync.RWMutex
	running     bool
	cancel      context.CancelFunc
}

// Connection 模拟连接
type Connection struct {
	DeviceID    string
	ConnectedAt time.Time
	LastSeen    time.Time
	Status      string // "connected", "disconnected", "error"
	MessageChan chan []byte
	ErrorChan   chan error
}

// ScheduledTask 调度任务
type ScheduledTask struct {
	ID         string
	Type       string // "heartbeat", "timeout", "retry"
	DeviceID   string
	Interval   time.Duration
	MaxRetries int
	Retries    int
	Callback   func()
	Timer      *time.Timer
	CreatedAt  time.Time
}

// NewMinimalSimulator 创建最小模拟器
func NewMinimalSimulator(config *SimulatorConfig) *MinimalSimulator {
	if config == nil {
		config = DefaultSimulatorConfig()
	}
	
	return &MinimalSimulator{
		config:      config,
		connections: make(map[string]*Connection),
		tasks:       make(map[string]*ScheduledTask),
		stats: &ConcurrencyStats{
			Errors: make([]string, 0),
		},
		running: false,
	}
}

// Start 启动模拟器
func (s *MinimalSimulator) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if s.running {
		return fmt.Errorf("simulator already running")
	}
	
	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.running = true
	
	// 启动统计更新协程
	go s.updateStats(ctx)
	
	return nil
}

// Connect 模拟设备连接
func (s *MinimalSimulator) Connect(ctx context.Context, deviceID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if len(s.connections) >= s.config.MaxConnections {
		return fmt.Errorf("connection limit exceeded: %d", s.config.MaxConnections)
	}
	
	if _, exists := s.connections[deviceID]; exists {
		return fmt.Errorf("device %s already connected", deviceID)
	}
	
	conn := &Connection{
		DeviceID:    deviceID,
		ConnectedAt: time.Now(),
		LastSeen:    time.Now(),
		Status:      "connected",
		MessageChan: make(chan []byte, 100),
		ErrorChan:   make(chan error, 10),
	}
	
	s.connections[deviceID] = conn
	s.stats.ActiveConnections = len(s.connections)
	if s.stats.ActiveConnections > s.stats.PeakConnections {
		s.stats.PeakConnections = s.stats.ActiveConnections
	}
	
	return nil
}

// Disconnect 模拟设备断开
func (s *MinimalSimulator) Disconnect(ctx context.Context, deviceID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	conn, exists := s.connections[deviceID]
	if !exists {
		return fmt.Errorf("device %s not connected", deviceID)
	}
	
	conn.Status = "disconnected"
	close(conn.MessageChan)
	close(conn.ErrorChan)
	delete(s.connections, deviceID)
	
	s.stats.ActiveConnections = len(s.connections)
	
	return nil
}

// Send 发送数据到模拟设备
func (s *MinimalSimulator) Send(ctx context.Context, deviceID string, data []byte) error {
	s.mu.RLock()
	conn, exists := s.connections[deviceID]
	s.mu.RUnlock()
	
	if !exists {
		return fmt.Errorf("device %s not connected", deviceID)
	}
	
	// 模拟网络延迟
	select {
	case <-time.After(time.Millisecond * time.Duration(rand.Intn(10))):
	case <-ctx.Done():
		return ctx.Err()
	}
	
	// 模拟网络错误
	if s.shouldInjectNetworkError() {
		s.addError(fmt.Sprintf("network error sending to %s", deviceID))
		s.stats.FailedReqs++
		return fmt.Errorf("network error")
	}
	
	select {
	case conn.MessageChan <- data:
		conn.LastSeen = time.Now()
		s.stats.SuccessfulReqs++
		s.stats.TotalRequests++
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		s.stats.FailedReqs++
		return fmt.Errorf("message queue full")
	}
}

// Receive 从模拟设备接收数据
func (s *MinimalSimulator) Receive(ctx context.Context, deviceID string) ([]byte, error) {
	s.mu.RLock()
	conn, exists := s.connections[deviceID]
	s.mu.RUnlock()
	
	if !exists {
		return nil, fmt.Errorf("device %s not connected", deviceID)
	}
	
	select {
	case data := <-conn.MessageChan:
		return data, nil
	case err := <-conn.ErrorChan:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// GetConnectedDevices 获取已连接设备列表
func (s *MinimalSimulator) GetConnectedDevices() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	devices := make([]string, 0, len(s.connections))
	for deviceID := range s.connections {
		devices = append(devices, deviceID)
	}
	return devices
}

// ScheduleHeartbeat 调度心跳任务
func (s *MinimalSimulator) ScheduleHeartbeat(ctx context.Context, deviceID string, interval time.Duration) error {
	taskID := fmt.Sprintf("heartbeat_%s", deviceID)
	
	task := &ScheduledTask{
		ID:        taskID,
		Type:      "heartbeat",
		DeviceID:  deviceID,
		Interval:  interval,
		CreatedAt: time.Now(),
		Callback: func() {
			// 模拟心跳数据
			heartbeatData := s.generateHeartbeatFrame(deviceID)
			s.Send(ctx, deviceID, heartbeatData)
		},
	}
	
	task.Timer = time.AfterFunc(interval, func() {
		task.Callback()
		// 重新调度下一次心跳
		s.ScheduleHeartbeat(ctx, deviceID, interval)
	})
	
	s.mu.Lock()
	s.tasks[taskID] = task
	s.mu.Unlock()
	
	return nil
}

// ScheduleTimeout 调度超时任务
func (s *MinimalSimulator) ScheduleTimeout(ctx context.Context, taskID string, timeout time.Duration, callback func()) error {
	task := &ScheduledTask{
		ID:        taskID,
		Type:      "timeout",
		Interval:  timeout,
		CreatedAt: time.Now(),
		Callback:  callback,
	}
	
	task.Timer = time.AfterFunc(timeout, callback)
	
	s.mu.Lock()
	s.tasks[taskID] = task
	s.mu.Unlock()
	
	return nil
}

// ScheduleRetry 调度重试任务
func (s *MinimalSimulator) ScheduleRetry(ctx context.Context, taskID string, maxRetries int, interval time.Duration) error {
	task := &ScheduledTask{
		ID:         taskID,
		Type:       "retry",
		Interval:   interval,
		MaxRetries: maxRetries,
		Retries:    0,
		CreatedAt:  time.Now(),
	}
	
	task.Callback = func() {
		task.Retries++
		if task.Retries < task.MaxRetries {
			// 继续重试
			task.Timer = time.AfterFunc(interval, task.Callback)
		} else {
			// 重试次数用尽，删除任务
			s.CancelTask(taskID)
		}
	}
	
	task.Timer = time.AfterFunc(interval, task.Callback)
	
	s.mu.Lock()
	s.tasks[taskID] = task
	s.mu.Unlock()
	
	return nil
}

// CancelTask 取消任务
func (s *MinimalSimulator) CancelTask(taskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	task, exists := s.tasks[taskID]
	if !exists {
		return fmt.Errorf("task %s not found", taskID)
	}
	
	if task.Timer != nil {
		task.Timer.Stop()
	}
	
	delete(s.tasks, taskID)
	return nil
}

// GetActiveTasks 获取活跃任务列表
func (s *MinimalSimulator) GetActiveTasks() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	tasks := make([]string, 0, len(s.tasks))
	for taskID := range s.tasks {
		tasks = append(tasks, taskID)
	}
	return tasks
}

// Close 关闭模拟器
func (s *MinimalSimulator) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if !s.running {
		return nil
	}
	
	// 取消所有任务
	for taskID := range s.tasks {
		if task := s.tasks[taskID]; task.Timer != nil {
			task.Timer.Stop()
		}
	}
	s.tasks = make(map[string]*ScheduledTask)
	
	// 断开所有连接
	for deviceID := range s.connections {
		s.Disconnect(context.Background(), deviceID)
	}
	
	if s.cancel != nil {
		s.cancel()
	}
	
	s.running = false
	return nil
}

// Stop 停止调度器
func (s *MinimalSimulator) Stop() error {
	return s.Close()
}

// GetConcurrencyStats 获取并发统计信息
func (s *MinimalSimulator) GetConcurrencyStats() *ConcurrencyStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	// 创建统计信息副本
	stats := &ConcurrencyStats{
		ActiveConnections: s.stats.ActiveConnections,
		TotalRequests:     s.stats.TotalRequests,
		SuccessfulReqs:    s.stats.SuccessfulReqs,
		FailedReqs:        s.stats.FailedReqs,
		AverageLatency:    s.stats.AverageLatency,
		PeakConnections:   s.stats.PeakConnections,
		Errors:            make([]string, len(s.stats.Errors)),
	}
	copy(stats.Errors, s.stats.Errors)
	
	return stats
}

// 内部方法

// updateStats 更新统计信息
func (s *MinimalSimulator) updateStats(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			s.mu.Lock()
			// 计算平均延迟（简单模拟）
			if s.stats.TotalRequests > 0 {
				s.stats.AverageLatency = time.Duration(float64(time.Millisecond) * (1 + rand.Float64()*10))
			}
			s.mu.Unlock()
		case <-ctx.Done():
			return
		}
	}
}

// shouldInjectNetworkError 判断是否应该注入网络错误
func (s *MinimalSimulator) shouldInjectNetworkError() bool {
	return rand.Float64() < s.config.NetworkErrorRate
}

// addError 添加错误信息
func (s *MinimalSimulator) addError(errMsg string) {
	s.stats.Errors = append(s.stats.Errors, fmt.Sprintf("%s: %s", time.Now().Format("15:04:05"), errMsg))
	
	// 保持错误列表长度不超过100
	if len(s.stats.Errors) > 100 {
		s.stats.Errors = s.stats.Errors[1:]
	}
}

// generateHeartbeatFrame 生成心跳帧数据
func (s *MinimalSimulator) generateHeartbeatFrame(deviceID string) []byte {
	// 基于协议文档的心跳帧格式
	// fcfe002e0000000000000182200520004869...
	heartbeatHex := "fcfe002e0000000000000182200520004869383938363034363331313230373033313934313763562e31723436001fcafcee"
	data, _ := hex.DecodeString(heartbeatHex)
	
	// 可以根据deviceID调整帧内容
	return data
}