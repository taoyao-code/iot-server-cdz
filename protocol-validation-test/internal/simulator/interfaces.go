package simulator

import (
	"context"
	"time"
)

// ConnectionSimulator 连接模拟器接口
// 仅用于验证真实时间/并发/连接行为时使用
type ConnectionSimulator interface {
	// Connect 模拟设备连接
	Connect(ctx context.Context, deviceID string) error
	
	// Disconnect 模拟设备断开
	Disconnect(ctx context.Context, deviceID string) error
	
	// Send 发送数据到模拟设备
	Send(ctx context.Context, deviceID string, data []byte) error
	
	// Receive 从模拟设备接收数据
	Receive(ctx context.Context, deviceID string) ([]byte, error)
	
	// GetConnectedDevices 获取已连接设备列表
	GetConnectedDevices() []string
	
	// Close 关闭模拟器
	Close() error
}

// SchedulerSimulator 调度模拟器接口  
// 用于测试基于时间的协议行为
type SchedulerSimulator interface {
	// ScheduleHeartbeat 调度心跳任务
	ScheduleHeartbeat(ctx context.Context, deviceID string, interval time.Duration) error
	
	// ScheduleTimeout 调度超时任务
	ScheduleTimeout(ctx context.Context, taskID string, timeout time.Duration, callback func()) error
	
	// ScheduleRetry 调度重试任务
	ScheduleRetry(ctx context.Context, taskID string, maxRetries int, interval time.Duration) error
	
	// CancelTask 取消任务
	CancelTask(taskID string) error
	
	// GetActiveTasks 获取活跃任务列表
	GetActiveTasks() []string
	
	// Stop 停止调度器
	Stop() error
}

// ErrorInjector 错误注入器接口
// 用于测试协议的容错能力
type ErrorInjector interface {
	// InjectChecksumError 注入校验和错误
	InjectChecksumError(data []byte) []byte
	
	// InjectLengthError 注入长度字段错误
	InjectLengthError(data []byte) []byte
	
	// InjectHeaderError 注入包头错误
	InjectHeaderError(data []byte) []byte
	
	// InjectTailError 注入包尾错误
	InjectTailError(data []byte) []byte
	
	// InjectTimeoutError 注入超时错误
	InjectTimeoutError(ctx context.Context, delay time.Duration) context.Context
	
	// InjectNetworkError 注入网络错误
	InjectNetworkError(probability float64) error
	
	// InjectUnknownCommand 注入未知命令
	InjectUnknownCommand(data []byte) []byte
	
	// Reset 重置错误注入器
	Reset()
}

// ConcurrencyTester 并发测试器接口
// 用于验证协议在并发场景下的行为
type ConcurrencyTester interface {
	// RunConcurrentConnections 运行并发连接测试
	RunConcurrentConnections(ctx context.Context, deviceCount int) error
	
	// RunConcurrentCommands 运行并发命令测试
	RunConcurrentCommands(ctx context.Context, deviceID string, commandCount int) error
	
	// RunLoadTest 运行负载测试
	RunLoadTest(ctx context.Context, duration time.Duration, ratePerSecond int) error
	
	// GetConcurrencyStats 获取并发统计信息
	GetConcurrencyStats() *ConcurrencyStats
	
	// Stop 停止并发测试
	Stop() error
}

// ConcurrencyStats 并发统计信息
type ConcurrencyStats struct {
	ActiveConnections int           `json:"active_connections"`
	TotalRequests     int64         `json:"total_requests"`
	SuccessfulReqs    int64         `json:"successful_requests"`
	FailedReqs        int64         `json:"failed_requests"`
	AverageLatency    time.Duration `json:"average_latency"`
	PeakConnections   int           `json:"peak_connections"`
	Errors            []string      `json:"errors"`
}

// SimulatorConfig 模拟器配置
type SimulatorConfig struct {
	// 连接相关配置
	MaxConnections    int           `yaml:"max_connections"`
	ConnectionTimeout time.Duration `yaml:"connection_timeout"`
	ReadTimeout       time.Duration `yaml:"read_timeout"`
	WriteTimeout      time.Duration `yaml:"write_timeout"`
	
	// 调度相关配置
	HeartbeatInterval time.Duration `yaml:"heartbeat_interval"`
	RetryInterval     time.Duration `yaml:"retry_interval"`
	MaxRetries        int           `yaml:"max_retries"`
	
	// 错误注入配置
	ErrorRate         float64 `yaml:"error_rate"`
	NetworkErrorRate  float64 `yaml:"network_error_rate"`
	ChecksumErrorRate float64 `yaml:"checksum_error_rate"`
	
	// 并发测试配置
	MaxConcurrency    int           `yaml:"max_concurrency"`
	LoadTestDuration  time.Duration `yaml:"load_test_duration"`
	RequestRateLimit  int           `yaml:"request_rate_limit"`
}

// DefaultSimulatorConfig 返回默认模拟器配置
func DefaultSimulatorConfig() *SimulatorConfig {
	return &SimulatorConfig{
		MaxConnections:    100,
		ConnectionTimeout: 30 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		HeartbeatInterval: 60 * time.Second,
		RetryInterval:     5 * time.Second,
		MaxRetries:        3,
		ErrorRate:         0.01,
		NetworkErrorRate:  0.005,
		ChecksumErrorRate: 0.002,
		MaxConcurrency:    50,
		LoadTestDuration:  60 * time.Second,
		RequestRateLimit:  100,
	}
}