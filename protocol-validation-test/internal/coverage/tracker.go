package coverage

import (
	"encoding/json"
	"time"

	"github.com/taoyao-code/protocol-validation-test/internal/validator"
)

// ScenarioInfo 场景信息
type ScenarioInfo struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Category     string    `json:"category"`
	Priority     string    `json:"priority"`
	Covered      bool      `json:"covered"`
	TestCases    []string  `json:"test_cases"`
	PassedCases  int       `json:"passed_cases"`
	TotalCases   int       `json:"total_cases"`
	PassRate     float64   `json:"pass_rate"`
	LastTested   time.Time `json:"last_tested"`
}

// CommandInfo 命令覆盖信息
type CommandInfo struct {
	Command     uint16   `json:"command"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Covered     bool     `json:"covered"`
	TestCount   int      `json:"test_count"`
	Scenarios   []string `json:"scenarios"`
}

// FrameTypeInfo 帧类型覆盖信息
type FrameTypeInfo struct {
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Covered     bool     `json:"covered"`
	TestCount   int      `json:"test_count"`
	Examples    []string `json:"examples"`
}

// FieldInfo 字段验证信息
type FieldInfo struct {
	Field       string   `json:"field"`
	Type        string   `json:"type"`
	Validations []string `json:"validations"`
	Covered     bool     `json:"covered"`
	TestCount   int      `json:"test_count"`
}

// CoverageSummary 覆盖度摘要
type CoverageSummary struct {
	TotalScenarios      int     `json:"total_scenarios"`
	CoveredScenarios    int     `json:"covered_scenarios"`
	ScenarioCoverage    float64 `json:"scenario_coverage"`
	TotalTestCases      int     `json:"total_test_cases"`
	PassedTestCases     int     `json:"passed_test_cases"`
	FailedTestCases     int     `json:"failed_test_cases"`
	SkippedTestCases    int     `json:"skipped_test_cases"`
	TestPassRate        float64 `json:"test_pass_rate"`
	TotalCommands       int     `json:"total_commands"`
	CoveredCommands     int     `json:"covered_commands"`
	CommandCoverage     float64 `json:"command_coverage"`
	TotalFrameTypes     int     `json:"total_frame_types"`
	CoveredFrameTypes   int     `json:"covered_frame_types"`
	FrameTypeCoverage   float64 `json:"frame_type_coverage"`
	GeneratedAt         time.Time `json:"generated_at"`
}

// CoverageMatrix 覆盖矩阵
type CoverageMatrix struct {
	Version     string              `json:"version"`
	Scenarios   map[string]*ScenarioInfo `json:"scenarios"`
	Commands    map[uint16]*CommandInfo  `json:"commands"`
	FrameTypes  map[string]*FrameTypeInfo `json:"frame_types"`
	Fields      map[string]*FieldInfo    `json:"fields"`
	TestResults []*validator.TestResult  `json:"test_results"`
	Summary     *CoverageSummary         `json:"summary"`
}

// Tracker 覆盖度追踪器
type Tracker struct {
	matrix *CoverageMatrix
}

// NewTracker 创建覆盖度追踪器
func NewTracker() *Tracker {
	return &Tracker{
		matrix: &CoverageMatrix{
			Version:     "1.0",
			Scenarios:   make(map[string]*ScenarioInfo),
			Commands:    make(map[uint16]*CommandInfo),
			FrameTypes:  make(map[string]*FrameTypeInfo),
			Fields:      make(map[string]*FieldInfo),
			TestResults: make([]*validator.TestResult, 0),
		},
	}
}

// InitializeScenarios 初始化31个协议场景
func (t *Tracker) InitializeScenarios() {
	// 基础场景 (9个)
	basicScenarios := []struct {
		id, name, category string
	}{
		{"heartbeat", "心跳上报/回复", "basic"},
		{"data_exchange", "服务器下发数据/模块上报数据", "basic"},
		{"socket_status_report", "插座状态上报", "basic"},
		{"socket_status_query", "查询插座状态", "basic"},
		{"network_refresh", "下发网络节点列表-刷新列表", "basic"},
		{"network_add_socket", "下发网络节点列表-添加单个插座", "basic"},
		{"network_delete_socket", "下发网络节点列表-删除单个插座", "basic"},
		{"control_time_power", "控制设备按时/按电量充电", "basic"},
		{"charging_end_basic", "充电结束上报按时/按电量", "basic"},
	}

	// 进阶场景 (10个)
	advancedScenarios := []struct {
		id, name, category string
	}{
		{"control_by_power", "按功率下发充电命令", "advanced"},
		{"charging_end_power", "按功率充电结束上报", "advanced"},
		{"card_charging", "刷卡充电(在线卡)", "advanced"},
		{"card_balance_query", "刷卡查询余额", "advanced"},
		{"voice_time_setting", "设置设备允许语音播报的时间", "advanced"},
		{"socket_param_set", "插座系统参数设置", "advanced"},
		{"socket_param_query", "插座系统参数查询", "advanced"},
		{"exception_event", "异常事件上报", "advanced"},
		{"ota_upgrade", "OTA升级", "advanced"},
		{"fee_service_charging", "按电费+服务费充电", "advanced"},
	}

	// 验证场景 (12个)
	validationScenarios := []struct {
		id, name, category string
	}{
		{"checksum_valid", "校验和正确验证", "validation"},
		{"checksum_invalid", "校验和错误验证", "validation"},
		{"sequence_consistency", "序列号一致性验证", "validation"},
		{"frame_length_validation", "帧长度验证", "validation"},
		{"header_tail_validation", "包头包尾验证", "validation"},
		{"gateway_id_validation", "网关ID验证", "validation"},
		{"business_id_consistency", "业务号一致性验证", "validation"},
		{"ack_behavior", "ACK行为验证", "validation"},
		{"bad_frame_1", "错误帧测试(坏帧1)", "validation"},
		{"bad_frame_2", "错误帧测试(坏帧2)", "validation"},
		{"unknown_command", "未知命令测试", "validation"},
		{"boundary_values", "边界值测试", "validation"},
	}

	// 添加所有场景
	for _, s := range basicScenarios {
		t.AddScenario(s.id, s.name, s.category, "high")
	}
	for _, s := range advancedScenarios {
		t.AddScenario(s.id, s.name, s.category, "medium")
	}
	for _, s := range validationScenarios {
		t.AddScenario(s.id, s.name, s.category, "high")
	}
}

// AddScenario 添加场景
func (t *Tracker) AddScenario(id, name, category, priority string) {
	t.matrix.Scenarios[id] = &ScenarioInfo{
		ID:         id,
		Name:       name,
		Category:   category,
		Priority:   priority,
		Covered:    false,
		TestCases:  make([]string, 0),
		PassedCases: 0,
		TotalCases:  0,
		PassRate:   0.0,
	}
}

// AddCommand 添加命令
func (t *Tracker) AddCommand(command uint16, name, description string) {
	t.matrix.Commands[command] = &CommandInfo{
		Command:     command,
		Name:        name,
		Description: description,
		Covered:     false,
		TestCount:   0,
		Scenarios:   make([]string, 0),
	}
}

// AddFrameType 添加帧类型
func (t *Tracker) AddFrameType(frameType, description string) {
	t.matrix.FrameTypes[frameType] = &FrameTypeInfo{
		Type:        frameType,
		Description: description,
		Covered:     false,
		TestCount:   0,
		Examples:    make([]string, 0),
	}
}

// AddField 添加字段
func (t *Tracker) AddField(field, fieldType string, validations []string) {
	t.matrix.Fields[field] = &FieldInfo{
		Field:       field,
		Type:        fieldType,
		Validations: validations,
		Covered:     false,
		TestCount:   0,
	}
}

// RecordTestResult 记录测试结果
func (t *Tracker) RecordTestResult(result *validator.TestResult) {
	t.matrix.TestResults = append(t.matrix.TestResults, result)

	// 更新场景覆盖
	testCase := result.TestCase
	scenarioID := testCase.Category + "_" + testCase.ID
	
	if scenario, exists := t.matrix.Scenarios[scenarioID]; exists {
		scenario.TestCases = append(scenario.TestCases, testCase.ID)
		scenario.TotalCases++
		scenario.Covered = true
		scenario.LastTested = result.ExecutedAt
		
		if result.Passed {
			scenario.PassedCases++
		}
		
		if scenario.TotalCases > 0 {
			scenario.PassRate = float64(scenario.PassedCases) / float64(scenario.TotalCases)
		}
	}

	// 更新命令覆盖
	if result.ParseResult != nil && result.ParseResult.Frame != nil {
		command := result.ParseResult.Frame.Command
		if cmdInfo, exists := t.matrix.Commands[command]; exists {
			cmdInfo.Covered = true
			cmdInfo.TestCount++
			cmdInfo.Scenarios = append(cmdInfo.Scenarios, scenarioID)
		}

		// 更新帧类型覆盖
		frameType := result.ParseResult.Frame.FrameType
		if frameTypeInfo, exists := t.matrix.FrameTypes[frameType]; exists {
			frameTypeInfo.Covered = true
			frameTypeInfo.TestCount++
			frameTypeInfo.Examples = append(frameTypeInfo.Examples, testCase.ID)
		}
	}

	// 更新字段覆盖
	for _, assertionResult := range result.AssertionResults {
		field := assertionResult.Assertion.Field
		if fieldInfo, exists := t.matrix.Fields[field]; exists {
			fieldInfo.Covered = true
			fieldInfo.TestCount++
		}
	}
}

// GenerateSummary 生成覆盖度摘要
func (t *Tracker) GenerateSummary() *CoverageSummary {
	summary := &CoverageSummary{
		GeneratedAt: time.Now(),
	}

	// 计算场景覆盖度
	summary.TotalScenarios = len(t.matrix.Scenarios)
	for _, scenario := range t.matrix.Scenarios {
		if scenario.Covered {
			summary.CoveredScenarios++
		}
		summary.TotalTestCases += scenario.TotalCases
		summary.PassedTestCases += scenario.PassedCases
	}
	
	summary.FailedTestCases = summary.TotalTestCases - summary.PassedTestCases
	
	if summary.TotalScenarios > 0 {
		summary.ScenarioCoverage = float64(summary.CoveredScenarios) / float64(summary.TotalScenarios)
	}
	
	if summary.TotalTestCases > 0 {
		summary.TestPassRate = float64(summary.PassedTestCases) / float64(summary.TotalTestCases)
	}

	// 计算命令覆盖度
	summary.TotalCommands = len(t.matrix.Commands)
	for _, command := range t.matrix.Commands {
		if command.Covered {
			summary.CoveredCommands++
		}
	}
	
	if summary.TotalCommands > 0 {
		summary.CommandCoverage = float64(summary.CoveredCommands) / float64(summary.TotalCommands)
	}

	// 计算帧类型覆盖度
	summary.TotalFrameTypes = len(t.matrix.FrameTypes)
	for _, frameType := range t.matrix.FrameTypes {
		if frameType.Covered {
			summary.CoveredFrameTypes++
		}
	}
	
	if summary.TotalFrameTypes > 0 {
		summary.FrameTypeCoverage = float64(summary.CoveredFrameTypes) / float64(summary.TotalFrameTypes)
	}

	t.matrix.Summary = summary
	return summary
}

// GetMatrix 获取覆盖矩阵
func (t *Tracker) GetMatrix() *CoverageMatrix {
	t.GenerateSummary()
	return t.matrix
}

// ExportJSON 导出为JSON
func (t *Tracker) ExportJSON() ([]byte, error) {
	return json.MarshalIndent(t.GetMatrix(), "", "  ")
}

// GetScenarioCoverage 获取场景覆盖度
func (t *Tracker) GetScenarioCoverage() float64 {
	if len(t.matrix.Scenarios) == 0 {
		return 0.0
	}
	
	covered := 0
	for _, scenario := range t.matrix.Scenarios {
		if scenario.Covered {
			covered++
		}
	}
	
	return float64(covered) / float64(len(t.matrix.Scenarios))
}

// Is100PercentCovered 检查是否100%覆盖
func (t *Tracker) Is100PercentCovered() bool {
	return t.GetScenarioCoverage() >= 1.0
}