package e2e

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config E2E测试配置
type Config struct {
	// 服务器配置
	ServerURL string // 服务器地址
	APIKey    string // API密钥

	// 测试设备
	TestDeviceID string // 测试设备ID
	SocketUID    string // 测试插座UID（用于控制链路映射）

	// 超时配置
	RequestTimeout time.Duration // 单个请求超时
	WaitTimeout    time.Duration // 等待状态超时

	// 重试配置
	RetryAttempts int           // 重试次数
	RetryDelay    time.Duration // 重试延迟

	// 日志配置
	LogLevel string // 日志级别：debug, info, warn, error
	Verbose  bool   // 详细输出
}

// GetConfig 获取测试配置（支持环境变量覆盖）
func GetConfig() *Config {
	cfg := &Config{
		// 默认配置
		ServerURL:      getEnv("E2E_SERVER_URL", "http://localhost:7055"),
		APIKey:         getEnv("E2E_API_KEY", ""),
		TestDeviceID:   getEnv("E2E_DEVICE_ID", ""),
		SocketUID:      getEnv("E2E_SOCKET_UID", ""),
		RequestTimeout: getDurationEnv("E2E_REQUEST_TIMEOUT", 30*time.Second),
		WaitTimeout:    getDurationEnv("E2E_WAIT_TIMEOUT", 60*time.Second),
		RetryAttempts:  getIntEnv("E2E_RETRY_ATTEMPTS", 3),
		RetryDelay:     getDurationEnv("E2E_RETRY_DELAY", 1*time.Second),
		LogLevel:       getEnv("E2E_LOG_LEVEL", "info"),
		Verbose:        getBoolEnv("E2E_VERBOSE", false),
	}

	// 验证必填配置
	if cfg.APIKey == "" {
		panic("E2E_API_KEY is required")
	}
	if cfg.TestDeviceID == "" {
		panic("E2E_DEVICE_ID is required")
	}
	if cfg.SocketUID == "" {
		panic("E2E_SOCKET_UID is required")
	}

	return cfg
}

// MaskedAPIKey 返回脱敏的 API Key
func (c *Config) MaskedAPIKey() string {
	if len(c.APIKey) <= 8 {
		return "***"
	}
	return c.APIKey[:4] + "***" + c.APIKey[len(c.APIKey)-4:]
}

// String 返回配置的字符串表示（脱敏）
func (c *Config) String() string {
	return fmt.Sprintf(`E2E Test Configuration:
  Server URL: %s
  API Key: %s
  Device ID: %s
  Socket UID: %s
  Request Timeout: %s
  Wait Timeout: %s
  Retry Attempts: %d
  Retry Delay: %s
  Log Level: %s
  Verbose: %v`,
		c.ServerURL,
		c.MaskedAPIKey(),
		c.TestDeviceID,
		c.SocketUID,
		c.RequestTimeout,
		c.WaitTimeout,
		c.RetryAttempts,
		c.RetryDelay,
		c.LogLevel,
		c.Verbose,
	)
}

// 辅助函数

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getIntEnv(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}

func getDurationEnv(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if d, err := time.ParseDuration(value); err == nil {
			return d
		}
	}
	return defaultValue
}

func getBoolEnv(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if b, err := strconv.ParseBool(value); err == nil {
			return b
		}
	}
	return defaultValue
}
