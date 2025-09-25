package validator

import (
	"fmt"
	"time"

	"github.com/taoyao-code/protocol-validation-test/internal/parser"
)

// TestCase 表示一个测试用例
type TestCase struct {
	ID           string                 `yaml:"id" json:"id"`
	Name         string                 `yaml:"name" json:"name"`
	Description  string                 `yaml:"description" json:"description"`
	Category     string                 `yaml:"category" json:"category"`
	Type         string                 `yaml:"type" json:"type"`          // uplink, downlink, bidirectional
	Input        TestInput              `yaml:"input" json:"input"`
	Expected     ExpectedResult         `yaml:"expected" json:"expected"`
	Assertions   []Assertion            `yaml:"assertions" json:"assertions"`
	Metadata     map[string]interface{} `yaml:"metadata,omitempty" json:"metadata,omitempty"`
}

// TestInput 测试输入
type TestInput struct {
	HexFrame    string            `yaml:"hex_frame" json:"hex_frame"`
	RawBytes    []byte            `yaml:"raw_bytes,omitempty" json:"raw_bytes,omitempty"`
	Scenario    string            `yaml:"scenario,omitempty" json:"scenario,omitempty"`
	Context     map[string]interface{} `yaml:"context,omitempty" json:"context,omitempty"`
}

// ExpectedResult 期望结果
type ExpectedResult struct {
	Command     uint16                 `yaml:"command" json:"command"`
	Direction   uint8                  `yaml:"direction" json:"direction"`
	GatewayID   string                 `yaml:"gateway_id" json:"gateway_id"`
	FrameType   string                 `yaml:"frame_type" json:"frame_type"`
	Valid       bool                   `yaml:"valid" json:"valid"`
	Payload     map[string]interface{} `yaml:"payload,omitempty" json:"payload,omitempty"`
	BKVFields   map[uint8]interface{}  `yaml:"bkv_fields,omitempty" json:"bkv_fields,omitempty"`
	ACKBehavior string                 `yaml:"ack_behavior,omitempty" json:"ack_behavior,omitempty"`
}

// Assertion 断言
type Assertion struct {
	Field    string      `yaml:"field" json:"field"`
	Type     string      `yaml:"type" json:"type"`        // equals, not_equals, range, pattern, valid, invalid, contains
	Value    interface{} `yaml:"value,omitempty" json:"value,omitempty"`
	Min      *int64      `yaml:"min,omitempty" json:"min,omitempty"`
	Max      *int64      `yaml:"max,omitempty" json:"max,omitempty"`
	Pattern  string      `yaml:"pattern,omitempty" json:"pattern,omitempty"`
	Message  string      `yaml:"message,omitempty" json:"message,omitempty"`
	Required bool        `yaml:"required" json:"required"`
}

// TestResult 测试结果
type TestResult struct {
	TestCase       *TestCase              `json:"test_case"`
	ParseResult    *parser.ParseResult    `json:"parse_result"`
	Success        bool                   `json:"success"`
	Passed         bool                   `json:"passed"`
	Failed         bool                   `json:"failed"`
	Skipped        bool                   `json:"skipped"`
	Errors         []ValidationError      `json:"errors"`
	Warnings       []ValidationWarning    `json:"warnings"`
	Duration       time.Duration          `json:"duration"`
	ExecutedAt     time.Time              `json:"executed_at"`
	AssertionResults []AssertionResult    `json:"assertion_results"`
}

// ValidationError 验证错误
type ValidationError struct {
	Field       string    `json:"field"`
	Type        string    `json:"type"`
	Expected    interface{} `json:"expected"`
	Actual      interface{} `json:"actual"`
	Message     string    `json:"message"`
	Severity    string    `json:"severity"` // error, critical
}

// ValidationWarning 验证警告
type ValidationWarning struct {
	Field    string `json:"field"`
	Message  string `json:"message"`
	Hint     string `json:"hint"`
}

// AssertionResult 断言结果
type AssertionResult struct {
	Assertion *Assertion `json:"assertion"`
	Passed    bool       `json:"passed"`
	Error     string     `json:"error,omitempty"`
	ActualValue interface{} `json:"actual_value,omitempty"`
}

// Engine 验证引擎
type Engine struct {
	frameParser parser.FrameParser
	bkvParser   parser.BKVParser
	tlvParser   parser.TLVParser
	
	// 统计信息
	totalTests    int
	passedTests   int
	failedTests   int
	skippedTests  int
}

// NewEngine 创建验证引擎
func NewEngine(frameParser parser.FrameParser, bkvParser parser.BKVParser, tlvParser parser.TLVParser) *Engine {
	return &Engine{
		frameParser: frameParser,
		bkvParser:   bkvParser,
		tlvParser:   tlvParser,
	}
}

// ValidateTestCase 验证单个测试用例
func (e *Engine) ValidateTestCase(testCase *TestCase) *TestResult {
	startTime := time.Now()
	
	result := &TestResult{
		TestCase:   testCase,
		ExecutedAt: startTime,
		Success:    true,
		Passed:     true,
	}

	// 解析输入帧
	hexData := testCase.Input.HexFrame
	rawBytes, err := hexToBytes(hexData)
	if err != nil {
		result.addError("input", "hex_decode", nil, nil, fmt.Sprintf("Failed to decode hex frame: %v", err))
		result.Duration = time.Since(startTime)
		return result
	}

	// 使用框架解析器解析帧
	frame, err := e.frameParser.Parse(rawBytes)
	if err != nil {
		result.addError("parse", "frame_parse", nil, nil, fmt.Sprintf("Failed to parse frame: %v", err))
		result.Duration = time.Since(startTime)
		return result
	}

	result.ParseResult = &parser.ParseResult{
		Frame:   frame,
		Success: frame.Valid,
		Errors:  frame.Errors,
	}

	// 如果是BKV协议，进一步解析BKV载荷
	if len(frame.Payload) > 0 && e.bkvParser != nil {
		bkvPayload, err := e.bkvParser.ParseBKV(frame.Payload)
		if err == nil {
			result.ParseResult.BKV = bkvPayload
			result.ParseResult.Protocol = parser.ProtocolTypeBKV
		}
	}

	// 执行断言
	e.executeAssertions(testCase, result, frame)

	result.Duration = time.Since(startTime)
	
	// 更新统计
	e.totalTests++
	if result.Passed {
		e.passedTests++
	} else {
		e.failedTests++
	}
	
	return result
}

// executeAssertions 执行断言
func (e *Engine) executeAssertions(testCase *TestCase, result *TestResult, frame *parser.Frame) {
	for _, assertion := range testCase.Assertions {
		assertionResult := e.executeAssertion(&assertion, frame, result.ParseResult)
		result.AssertionResults = append(result.AssertionResults, assertionResult)
		
		if !assertionResult.Passed {
			result.Passed = false
			result.Success = false
		}
	}

	// 验证期望结果
	e.validateExpectedResult(testCase, result, frame)
}

// executeAssertion 执行单个断言
func (e *Engine) executeAssertion(assertion *Assertion, frame *parser.Frame, parseResult *parser.ParseResult) AssertionResult {
	result := AssertionResult{
		Assertion: assertion,
		Passed:    false,
	}

	actualValue := e.getFieldValue(assertion.Field, frame, parseResult)
	result.ActualValue = actualValue

	switch assertion.Type {
	case "equals":
		result.Passed = e.assertEquals(actualValue, assertion.Value)
	case "not_equals":
		result.Passed = !e.assertEquals(actualValue, assertion.Value)
	case "range":
		result.Passed = e.assertRange(actualValue, assertion.Min, assertion.Max)
	case "valid":
		result.Passed = e.assertValid(assertion.Field, frame, parseResult)
	case "invalid":
		result.Passed = !e.assertValid(assertion.Field, frame, parseResult)
	case "contains":
		result.Passed = e.assertContains(actualValue, assertion.Value)
	default:
		result.Error = fmt.Sprintf("Unknown assertion type: %s", assertion.Type)
	}

	if !result.Passed && result.Error == "" {
		result.Error = fmt.Sprintf("Assertion failed: expected %v, got %v", assertion.Value, actualValue)
	}

	return result
}

// getFieldValue 获取字段值
func (e *Engine) getFieldValue(field string, frame *parser.Frame, parseResult *parser.ParseResult) interface{} {
	switch field {
	case "header":
		return frame.Header
	case "length":
		return frame.Length
	case "command":
		return frame.Command
	case "sequence":
		return frame.Sequence
	case "direction":
		return frame.Direction
	case "gateway_id":
		return frame.GatewayID
	case "checksum":
		return frame.Checksum
	case "checksum_valid":
		return frame.ChecksumValid
	case "valid":
		return frame.Valid
	case "frame_type":
		return frame.FrameType
	default:
		// 尝试从BKV载荷中获取
		if parseResult.BKV != nil {
			return e.getBKVFieldValue(field, parseResult.BKV)
		}
		return nil
	}
}

// getBKVFieldValue 从BKV载荷中获取字段值
func (e *Engine) getBKVFieldValue(field string, bkv *parser.BKVPayload) interface{} {
	switch field {
	case "bkv_command":
		return bkv.Command
	case "bkv_sequence":
		return bkv.Sequence
	case "bkv_gateway_id":
		return bkv.GatewayID
	case "socket_no":
		return bkv.SocketNo
	case "port_no":
		return bkv.PortNo
	default:
		// 尝试从TLV字段中获取
		// TODO: 实现TLV字段访问
		return nil
	}
}

// validateExpectedResult 验证期望结果
func (e *Engine) validateExpectedResult(testCase *TestCase, result *TestResult, frame *parser.Frame) {
	expected := testCase.Expected
	
	// 验证命令
	if expected.Command != 0 && frame.Command != expected.Command {
		result.addError("command", "mismatch", expected.Command, frame.Command, "Command mismatch")
	}
	
	// 验证方向
	if frame.Direction != expected.Direction {
		result.addError("direction", "mismatch", expected.Direction, frame.Direction, "Direction mismatch")
	}
	
	// 验证网关ID
	if expected.GatewayID != "" && frame.GatewayID != expected.GatewayID {
		result.addError("gateway_id", "mismatch", expected.GatewayID, frame.GatewayID, "Gateway ID mismatch")
	}
	
	// 验证帧类型
	if expected.FrameType != "" && frame.FrameType != expected.FrameType {
		result.addError("frame_type", "mismatch", expected.FrameType, frame.FrameType, "Frame type mismatch")
	}
	
	// 验证有效性
	if frame.Valid != expected.Valid {
		result.addError("valid", "mismatch", expected.Valid, frame.Valid, "Frame validity mismatch")
	}
}

// 辅助方法
func (e *Engine) assertEquals(actual, expected interface{}) bool {
	return actual == expected
}

func (e *Engine) assertRange(actual interface{}, min, max *int64) bool {
	val, ok := actual.(int64)
	if !ok {
		return false
	}
	
	if min != nil && val < *min {
		return false
	}
	if max != nil && val > *max {
		return false
	}
	return true
}

func (e *Engine) assertValid(field string, frame *parser.Frame, parseResult *parser.ParseResult) bool {
	switch field {
	case "checksum":
		return frame.ChecksumValid
	case "frame":
		return frame.Valid
	default:
		return true
	}
}

func (e *Engine) assertContains(actual, expected interface{}) bool {
	// TODO: 实现包含断言
	return false
}

// addError 添加错误到结果
func (r *TestResult) addError(field, errorType string, expected, actual interface{}, message string) {
	r.Success = false
	r.Passed = false
	r.Errors = append(r.Errors, ValidationError{
		Field:    field,
		Type:     errorType,
		Expected: expected,
		Actual:   actual,
		Message:  message,
		Severity: "error",
	})
}

// GetStats 获取统计信息
func (e *Engine) GetStats() (total, passed, failed, skipped int) {
	return e.totalTests, e.passedTests, e.failedTests, e.skippedTests
}

// hexToBytes 将十六进制字符串转换为字节数组
func hexToBytes(hexStr string) ([]byte, error) {
	// 移除可能的空格和前缀
	hexStr = fmt.Sprintf("%s", hexStr) // 简化处理，实际实现需要更完善
	
	if len(hexStr)%2 != 0 {
		return nil, fmt.Errorf("invalid hex string length")
	}
	
	bytes := make([]byte, len(hexStr)/2)
	for i := 0; i < len(hexStr); i += 2 {
		// 简化实现，实际需要使用hex.DecodeString
		// bytes[i/2] = hexToByte(hexStr[i:i+2])
	}
	
	return bytes, nil
}