package simulator

import (
	"context"
	"encoding/binary"
	"math/rand"
	"time"
)

// BasicErrorInjector 基础错误注入器实现
type BasicErrorInjector struct {
	config       *SimulatorConfig
	errorHistory []string
}

// NewBasicErrorInjector 创建基础错误注入器
func NewBasicErrorInjector(config *SimulatorConfig) *BasicErrorInjector {
	if config == nil {
		config = DefaultSimulatorConfig()
	}
	
	return &BasicErrorInjector{
		config:       config,
		errorHistory: make([]string, 0),
	}
}

// InjectChecksumError 注入校验和错误
func (e *BasicErrorInjector) InjectChecksumError(data []byte) []byte {
	if len(data) < 4 {
		return data
	}
	
	// 协议帧格式: 包头(2) + 长度(2) + ... + 校验和(1) + 包尾(2)
	if len(data) >= 3 {
		// 故意修改校验和字节（倒数第3个字节）
		corrupted := make([]byte, len(data))
		copy(corrupted, data)
		
		checksumPos := len(corrupted) - 3
		if checksumPos >= 0 && checksumPos < len(corrupted) {
			// 随机修改校验和
			corrupted[checksumPos] = byte(rand.Intn(256))
			e.logError("checksum_error", "modified checksum at position %d", checksumPos)
		}
		
		return corrupted
	}
	
	return data
}

// InjectLengthError 注入长度字段错误
func (e *BasicErrorInjector) InjectLengthError(data []byte) []byte {
	if len(data) < 4 {
		return data
	}
	
	// 修改长度字段（第3-4字节，大端序）
	corrupted := make([]byte, len(data))
	copy(corrupted, data)
	
	// 故意设置错误的长度
	wrongLength := uint16(len(data) + rand.Intn(10) - 5)
	binary.BigEndian.PutUint16(corrupted[2:4], wrongLength)
	
	e.logError("length_error", "modified length from %d to %d", len(data), wrongLength)
	
	return corrupted
}

// InjectHeaderError 注入包头错误
func (e *BasicErrorInjector) InjectHeaderError(data []byte) []byte {
	if len(data) < 2 {
		return data
	}
	
	corrupted := make([]byte, len(data))
	copy(corrupted, data)
	
	// 正常包头是 0xFCFE 或 0xFCFF，故意修改
	headers := [][]byte{{0xFC, 0xFD}, {0xFB, 0xFE}, {0x00, 0x00}}
	wrongHeader := headers[rand.Intn(len(headers))]
	
	copy(corrupted[0:2], wrongHeader)
	e.logError("header_error", "modified header to %02x%02x", wrongHeader[0], wrongHeader[1])
	
	return corrupted
}

// InjectTailError 注入包尾错误
func (e *BasicErrorInjector) InjectTailError(data []byte) []byte {
	if len(data) < 2 {
		return data
	}
	
	corrupted := make([]byte, len(data))
	copy(corrupted, data)
	
	// 正常包尾是 0xFCEE，故意修改
	tails := [][]byte{{0xFC, 0xED}, {0xFB, 0xEE}, {0x00, 0x00}}
	wrongTail := tails[rand.Intn(len(tails))]
	
	tailPos := len(corrupted) - 2
	copy(corrupted[tailPos:], wrongTail)
	e.logError("tail_error", "modified tail to %02x%02x", wrongTail[0], wrongTail[1])
	
	return corrupted
}

// InjectTimeoutError 注入超时错误
func (e *BasicErrorInjector) InjectTimeoutError(ctx context.Context, delay time.Duration) context.Context {
	// 创建一个会提前超时的context
	timeoutCtx, _ := context.WithTimeout(ctx, delay/2)
	e.logError("timeout_error", "injected timeout with delay %v", delay)
	return timeoutCtx
}

// InjectNetworkError 注入网络错误
func (e *BasicErrorInjector) InjectNetworkError(probability float64) error {
	if rand.Float64() < probability {
		e.logError("network_error", "simulated network failure")
		return &NetworkError{Message: "simulated network failure"}
	}
	return nil
}

// InjectUnknownCommand 注入未知命令
func (e *BasicErrorInjector) InjectUnknownCommand(data []byte) []byte {
	if len(data) < 6 {
		return data
	}
	
	corrupted := make([]byte, len(data))
	copy(corrupted, data)
	
	// 修改命令字段（第5-6字节），设置为未知命令0xFFFF
	binary.BigEndian.PutUint16(corrupted[4:6], 0xFFFF)
	e.logError("unknown_command", "injected unknown command 0xFFFF")
	
	return corrupted
}

// Reset 重置错误注入器
func (e *BasicErrorInjector) Reset() {
	e.errorHistory = make([]string, 0)
}

// GetErrorHistory 获取错误历史
func (e *BasicErrorInjector) GetErrorHistory() []string {
	return e.errorHistory
}

// logError 记录错误信息
func (e *BasicErrorInjector) logError(errorType, format string, args ...interface{}) {
	timestamp := time.Now().Format("15:04:05.000")
	msg := timestamp + " [" + errorType + "] " + format
	
	// 格式化参数
	if len(args) > 0 {
		// 简单的字符串格式化
		for i, arg := range args {
			if i < len(args) {
				switch v := arg.(type) {
				case int:
					// 替换 %d
					for j := 0; j < len(msg); j++ {
						if j < len(msg)-1 && msg[j] == '%' && msg[j+1] == 'd' {
							msg = msg[:j] + intToString(v) + msg[j+2:]
							break
						}
					}
				case uint16:
					// 替换 %d
					for j := 0; j < len(msg); j++ {
						if j < len(msg)-1 && msg[j] == '%' && msg[j+1] == 'd' {
							msg = msg[:j] + intToString(int(v)) + msg[j+2:]
							break
						}
					}
				case byte:
					// 替换 %02x
					for j := 0; j < len(msg); j++ {
						if j < len(msg)-3 && msg[j:j+4] == "%02x" {
							msg = msg[:j] + byteToHex(v) + msg[j+4:]
							break
						}
					}
				case time.Duration:
					// 替换 %v
					for j := 0; j < len(msg); j++ {
						if j < len(msg)-1 && msg[j] == '%' && msg[j+1] == 'v' {
							msg = msg[:j] + v.String() + msg[j+2:]
							break
						}
					}
				}
			}
		}
	}
	
	e.errorHistory = append(e.errorHistory, msg)
	
	// 保持历史记录不超过50条
	if len(e.errorHistory) > 50 {
		e.errorHistory = e.errorHistory[1:]
	}
}

// NetworkError 网络错误类型
type NetworkError struct {
	Message string
}

func (e *NetworkError) Error() string {
	return e.Message
}

// 辅助函数

// intToString 整数转字符串
func intToString(n int) string {
	if n == 0 {
		return "0"
	}
	
	var result []byte
	negative := n < 0
	if negative {
		n = -n
	}
	
	for n > 0 {
		result = append([]byte{byte('0'+n%10)}, result...)
		n /= 10
	}
	
	if negative {
		result = append([]byte{'-'}, result...)
	}
	
	return string(result)
}

// byteToHex 字节转十六进制字符串
func byteToHex(b byte) string {
	const hexChars = "0123456789abcdef"
	return string([]byte{hexChars[b>>4], hexChars[b&0x0f]})
}