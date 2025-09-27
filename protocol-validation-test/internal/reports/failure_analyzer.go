package reports

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// FailureAnalyzer 失败帧分析器
type FailureAnalyzer struct {
	outputDir string
}

// NewFailureAnalyzer 创建失败帧分析器
func NewFailureAnalyzer(outputDir string) *FailureAnalyzer {
	return &FailureAnalyzer{
		outputDir: outputDir,
	}
}

// FrameFailure 帧失败信息
type FrameFailure struct {
	TestCaseID    string            `json:"test_case_id"`
	FrameHex      string            `json:"frame_hex"`
	ErrorType     string            `json:"error_type"`
	ErrorMessage  string            `json:"error_message"`
	FailedAt      time.Time         `json:"failed_at"`
	Analysis      *FailureAnalysis  `json:"analysis"`
	Suggestions   []string          `json:"suggestions"`
}

// FailureAnalysis 失败分析详情
type FailureAnalysis struct {
	FrameLength       int                    `json:"frame_length"`
	ExpectedLength    int                    `json:"expected_length,omitempty"`
	ParsedFields      map[string]interface{} `json:"parsed_fields"`
	ErrorLocation     *ErrorLocation         `json:"error_location,omitempty"`
	ProtocolViolation *ProtocolViolation     `json:"protocol_violation,omitempty"`
	RepairAttempt     *RepairAttempt         `json:"repair_attempt,omitempty"`
}

// ErrorLocation 错误位置信息
type ErrorLocation struct {
	ByteOffset    int    `json:"byte_offset"`
	FieldName     string `json:"field_name"`
	ExpectedValue string `json:"expected_value"`
	ActualValue   string `json:"actual_value"`
	Description   string `json:"description"`
}

// ProtocolViolation 协议违规信息
type ProtocolViolation struct {
	ViolationType string   `json:"violation_type"`
	Severity      string   `json:"severity"` // "critical", "warning", "info"
	Description   string   `json:"description"`
	References    []string `json:"references"`
}

// RepairAttempt 修复尝试
type RepairAttempt struct {
	Strategy      string `json:"strategy"`
	RepairedHex   string `json:"repaired_hex"`
	Success       bool   `json:"success"`
	Changes       []FieldChange `json:"changes"`
}

// FieldChange 字段变更
type FieldChange struct {
	FieldName string `json:"field_name"`
	Offset    int    `json:"offset"`
	OldValue  string `json:"old_value"`
	NewValue  string `json:"new_value"`
	Reason    string `json:"reason"`
}

// FailureReport 失败报告
type FailureReport struct {
	GeneratedAt     time.Time       `json:"generated_at"`
	TotalFailures   int             `json:"total_failures"`
	FailuresByType  map[string]int  `json:"failures_by_type"`
	FailuresByField map[string]int  `json:"failures_by_field"`
	Failures        []FrameFailure  `json:"failures"`
	Summary         *FailureSummary `json:"summary"`
}

// FailureSummary 失败摘要
type FailureSummary struct {
	MostCommonErrors   []ErrorFrequency `json:"most_common_errors"`
	ProblematicFields  []FieldFrequency `json:"problematic_fields"`
	RepairSuccessRate  float64          `json:"repair_success_rate"`
	CriticalViolations int              `json:"critical_violations"`
	Recommendations    []string         `json:"recommendations"`
}

// ErrorFrequency 错误频率
type ErrorFrequency struct {
	ErrorType string `json:"error_type"`
	Count     int    `json:"count"`
	Percentage float64 `json:"percentage"`
}

// FieldFrequency 字段频率
type FieldFrequency struct {
	FieldName string `json:"field_name"`
	Count     int    `json:"count"`
	Percentage float64 `json:"percentage"`
}

// AnalyzeFrame 分析单个失败帧
func (fa *FailureAnalyzer) AnalyzeFrame(testCaseID, frameHex, errorType, errorMessage string) *FrameFailure {
	failure := &FrameFailure{
		TestCaseID:   testCaseID,
		FrameHex:     frameHex,
		ErrorType:    errorType,
		ErrorMessage: errorMessage,
		FailedAt:     time.Now(),
		Suggestions:  make([]string, 0),
	}

	// 解码帧数据
	frameData, err := hex.DecodeString(frameHex)
	if err != nil {
		failure.Analysis = &FailureAnalysis{
			FrameLength: len(frameHex) / 2,
			ParsedFields: map[string]interface{}{
				"hex_decode_error": err.Error(),
			},
		}
		failure.Suggestions = append(failure.Suggestions, "检查帧数据的十六进制格式是否正确")
		return failure
	}

	// 执行详细分析
	failure.Analysis = fa.performDetailedAnalysis(frameData, errorType, errorMessage)
	failure.Suggestions = fa.generateSuggestions(failure.Analysis, errorType)

	return failure
}

// performDetailedAnalysis 执行详细分析
func (fa *FailureAnalyzer) performDetailedAnalysis(frameData []byte, errorType, errorMessage string) *FailureAnalysis {
	analysis := &FailureAnalysis{
		FrameLength:  len(frameData),
		ParsedFields: make(map[string]interface{}),
	}

	// 基础帧结构解析
	if len(frameData) >= 4 {
		analysis.ParsedFields["header"] = fmt.Sprintf("%02x%02x", frameData[0], frameData[1])
		analysis.ParsedFields["length"] = fmt.Sprintf("%02x%02x", frameData[2], frameData[3])
		
		// 解析长度字段
		if len(frameData) >= 4 {
			expectedLength := int(frameData[2])<<8 + int(frameData[3])
			analysis.ExpectedLength = expectedLength
			
			if expectedLength != len(frameData) {
				analysis.ErrorLocation = &ErrorLocation{
					ByteOffset:    2,
					FieldName:     "length",
					ExpectedValue: fmt.Sprintf("%04x", len(frameData)),
					ActualValue:   fmt.Sprintf("%04x", expectedLength),
					Description:   "帧长度字段与实际长度不符",
				}
			}
		}
	}

	// 解析更多字段
	if len(frameData) >= 6 {
		analysis.ParsedFields["command"] = fmt.Sprintf("%02x%02x", frameData[4], frameData[5])
	}
	if len(frameData) >= 10 {
		analysis.ParsedFields["sequence"] = fmt.Sprintf("%02x%02x%02x%02x", 
			frameData[6], frameData[7], frameData[8], frameData[9])
	}
	if len(frameData) >= 11 {
		analysis.ParsedFields["direction"] = fmt.Sprintf("%02x", frameData[10])
	}

	// 检查包头和包尾
	fa.checkProtocolCompliance(frameData, analysis)

	// 尝试修复
	analysis.RepairAttempt = fa.attemptRepair(frameData, errorType)

	return analysis
}

// checkProtocolCompliance 检查协议合规性
func (fa *FailureAnalyzer) checkProtocolCompliance(frameData []byte, analysis *FailureAnalysis) {
	violations := make([]*ProtocolViolation, 0)

	// 检查包头
	if len(frameData) >= 2 {
		header := fmt.Sprintf("%02x%02x", frameData[0], frameData[1])
		if header != "fcfe" && header != "fcff" {
			violations = append(violations, &ProtocolViolation{
				ViolationType: "invalid_header",
				Severity:      "critical",
				Description:   fmt.Sprintf("无效的包头: %s，期望 fcfe 或 fcff", header),
				References:    []string{"设备对接指引-组网设备2024(1).txt 第3.1节"},
			})
		}
	}

	// 检查包尾
	if len(frameData) >= 2 {
		tail := fmt.Sprintf("%02x%02x", frameData[len(frameData)-2], frameData[len(frameData)-1])
		if tail != "fcee" {
			violations = append(violations, &ProtocolViolation{
				ViolationType: "invalid_tail",
				Severity:      "critical",
				Description:   fmt.Sprintf("无效的包尾: %s，期望 fcee", tail),
				References:    []string{"设备对接指引-组网设备2024(1).txt 第3.1节"},
			})
		}
	}

	// 检查最小长度
	if len(frameData) < 12 {
		violations = append(violations, &ProtocolViolation{
			ViolationType: "frame_too_short",
			Severity:      "critical",
			Description:   fmt.Sprintf("帧长度过短: %d字节，最小需要12字节", len(frameData)),
			References:    []string{"设备对接指引-组网设备2024(1).txt 第3.2节"},
		})
	}

	// 如果有违规，记录第一个
	if len(violations) > 0 {
		analysis.ProtocolViolation = violations[0]
	}
}

// attemptRepair 尝试修复帧
func (fa *FailureAnalyzer) attemptRepair(frameData []byte, errorType string) *RepairAttempt {
	attempt := &RepairAttempt{
		Strategy: "auto_repair",
		Changes:  make([]FieldChange, 0),
		Success:  false,
	}

	repaired := make([]byte, len(frameData))
	copy(repaired, frameData)
	hasChanges := false

	switch errorType {
	case "checksum_error":
		// 尝试修复校验和
		if len(repaired) >= 3 {
			// 重新计算校验和
			newChecksum := fa.calculateChecksum(repaired[:len(repaired)-3])
			oldChecksum := repaired[len(repaired)-3]
			repaired[len(repaired)-3] = newChecksum
			
			if newChecksum != oldChecksum {
				attempt.Changes = append(attempt.Changes, FieldChange{
					FieldName: "checksum",
					Offset:    len(repaired) - 3,
					OldValue:  fmt.Sprintf("%02x", oldChecksum),
					NewValue:  fmt.Sprintf("%02x", newChecksum),
					Reason:    "重新计算校验和",
				})
				hasChanges = true
			}
		}

	case "header_error":
		// 尝试修复包头
		if len(repaired) >= 2 {
			oldHeader := fmt.Sprintf("%02x%02x", repaired[0], repaired[1])
			repaired[0] = 0xFC
			repaired[1] = 0xFE // 默认使用上行包头
			
			attempt.Changes = append(attempt.Changes, FieldChange{
				FieldName: "header",
				Offset:    0,
				OldValue:  oldHeader,
				NewValue:  "fcfe",
				Reason:    "修正为标准包头",
			})
			hasChanges = true
		}

	case "tail_error":
		// 尝试修复包尾
		if len(repaired) >= 2 {
			oldTail := fmt.Sprintf("%02x%02x", repaired[len(repaired)-2], repaired[len(repaired)-1])
			repaired[len(repaired)-2] = 0xFC
			repaired[len(repaired)-1] = 0xEE
			
			attempt.Changes = append(attempt.Changes, FieldChange{
				FieldName: "tail",
				Offset:    len(repaired) - 2,
				OldValue:  oldTail,
				NewValue:  "fcee",
				Reason:    "修正为标准包尾",
			})
			hasChanges = true
		}

	case "length_error":
		// 尝试修复长度字段
		if len(repaired) >= 4 {
			oldLength := fmt.Sprintf("%02x%02x", repaired[2], repaired[3])
			correctLength := len(repaired)
			repaired[2] = byte(correctLength >> 8)
			repaired[3] = byte(correctLength & 0xFF)
			
			attempt.Changes = append(attempt.Changes, FieldChange{
				FieldName: "length",
				Offset:    2,
				OldValue:  oldLength,
				NewValue:  fmt.Sprintf("%04x", correctLength),
				Reason:    "修正为实际帧长度",
			})
			hasChanges = true
		}
	}

	if hasChanges {
		attempt.RepairedHex = hex.EncodeToString(repaired)
		attempt.Success = true
	}

	return attempt
}

// calculateChecksum 计算校验和（简化版本）
func (fa *FailureAnalyzer) calculateChecksum(data []byte) byte {
	var sum byte
	for _, b := range data {
		sum += b
	}
	return sum
}

// generateSuggestions 生成修复建议
func (fa *FailureAnalyzer) generateSuggestions(analysis *FailureAnalysis, errorType string) []string {
	suggestions := make([]string, 0)

	switch errorType {
	case "checksum_error":
		suggestions = append(suggestions, "检查数据传输过程中是否存在干扰")
		suggestions = append(suggestions, "验证校验和计算算法是否正确")
		if analysis.RepairAttempt != nil && analysis.RepairAttempt.Success {
			suggestions = append(suggestions, "可尝试使用修复后的校验和值")
		}

	case "length_error":
		suggestions = append(suggestions, "检查帧长度字段的字节序（大端/小端）")
		suggestions = append(suggestions, "验证是否包含了所有必需的字段")

	case "header_error", "tail_error":
		suggestions = append(suggestions, "检查帧的包头包尾是否符合协议规范")
		suggestions = append(suggestions, "确认上行帧使用fcfe包头，下行帧使用fcff包头")

	case "parse_error":
		suggestions = append(suggestions, "检查帧的整体结构是否符合协议格式")
		suggestions = append(suggestions, "验证所有必需字段是否都存在")

	default:
		suggestions = append(suggestions, "请参考协议文档检查帧格式")
		suggestions = append(suggestions, "使用帧分析工具进行详细诊断")
	}

	// 基于分析结果的通用建议
	if analysis.ProtocolViolation != nil {
		if analysis.ProtocolViolation.Severity == "critical" {
			suggestions = append(suggestions, "存在严重协议违规，需要重点修复")
		}
	}

	if analysis.ErrorLocation != nil {
		suggestions = append(suggestions, fmt.Sprintf("重点检查字段 %s（偏移%d）", 
			analysis.ErrorLocation.FieldName, analysis.ErrorLocation.ByteOffset))
	}

	return suggestions
}

// GenerateReport 生成失败分析报告
func (fa *FailureAnalyzer) GenerateReport(failures []FrameFailure) (*FailureReport, error) {
	report := &FailureReport{
		GeneratedAt:     time.Now(),
		TotalFailures:   len(failures),
		FailuresByType:  make(map[string]int),
		FailuresByField: make(map[string]int),
		Failures:        failures,
	}

	// 统计失败类型
	for _, failure := range failures {
		report.FailuresByType[failure.ErrorType]++
		
		if failure.Analysis != nil && failure.Analysis.ErrorLocation != nil {
			report.FailuresByField[failure.Analysis.ErrorLocation.FieldName]++
		}
	}

	// 生成摘要
	report.Summary = fa.generateFailureSummary(report)

	return report, nil
}

// generateFailureSummary 生成失败摘要
func (fa *FailureAnalyzer) generateFailureSummary(report *FailureReport) *FailureSummary {
	summary := &FailureSummary{
		MostCommonErrors:  make([]ErrorFrequency, 0),
		ProblematicFields: make([]FieldFrequency, 0),
		Recommendations:   make([]string, 0),
	}

	// 统计最常见错误
	for errorType, count := range report.FailuresByType {
		percentage := float64(count) / float64(report.TotalFailures) * 100
		summary.MostCommonErrors = append(summary.MostCommonErrors, ErrorFrequency{
			ErrorType:  errorType,
			Count:      count,
			Percentage: percentage,
		})
	}

	// 统计问题字段
	for fieldName, count := range report.FailuresByField {
		percentage := float64(count) / float64(report.TotalFailures) * 100
		summary.ProblematicFields = append(summary.ProblematicFields, FieldFrequency{
			FieldName:  fieldName,
			Count:      count,
			Percentage: percentage,
		})
	}

	// 计算修复成功率
	repairAttempts := 0
	successfulRepairs := 0
	criticalViolations := 0

	for _, failure := range report.Failures {
		if failure.Analysis != nil {
			if failure.Analysis.RepairAttempt != nil {
				repairAttempts++
				if failure.Analysis.RepairAttempt.Success {
					successfulRepairs++
				}
			}
			if failure.Analysis.ProtocolViolation != nil && 
				failure.Analysis.ProtocolViolation.Severity == "critical" {
				criticalViolations++
			}
		}
	}

	if repairAttempts > 0 {
		summary.RepairSuccessRate = float64(successfulRepairs) / float64(repairAttempts) * 100
	}
	summary.CriticalViolations = criticalViolations

	// 生成建议
	summary.Recommendations = fa.generateReportRecommendations(report, summary)

	return summary
}

// generateReportRecommendations 生成报告建议
func (fa *FailureAnalyzer) generateReportRecommendations(report *FailureReport, summary *FailureSummary) []string {
	recommendations := make([]string, 0)

	if len(summary.MostCommonErrors) > 0 {
		topError := summary.MostCommonErrors[0]
		recommendations = append(recommendations, 
			fmt.Sprintf("优先修复最常见的错误类型：%s (占%.1f%%)", topError.ErrorType, topError.Percentage))
	}

	if len(summary.ProblematicFields) > 0 {
		topField := summary.ProblematicFields[0]
		recommendations = append(recommendations, 
			fmt.Sprintf("重点关注问题字段：%s (出现%d次)", topField.FieldName, topField.Count))
	}

	if summary.CriticalViolations > 0 {
		recommendations = append(recommendations, 
			fmt.Sprintf("有%d个严重协议违规需要立即处理", summary.CriticalViolations))
	}

	if summary.RepairSuccessRate < 50 {
		recommendations = append(recommendations, "修复成功率较低，建议检查修复算法")
	} else if summary.RepairSuccessRate > 80 {
		recommendations = append(recommendations, "修复成功率良好，可考虑自动修复机制")
	}

	if len(recommendations) == 0 {
		recommendations = append(recommendations, "失败率较低，继续保持测试质量")
	}

	return recommendations
}

// SaveJSON 保存JSON格式报告
func (fa *FailureAnalyzer) SaveJSON(report *FailureReport, filename string) error {
	if err := os.MkdirAll(fa.outputDir, 0755); err != nil {
		return err
	}

	filePath := filepath.Join(fa.outputDir, filename)
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}