package reports

import (
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// CoverageMatrix 覆盖度矩阵
type CoverageMatrix struct {
	GeneratedAt time.Time              `json:"generated_at"`
	TotalScenarios int                 `json:"total_scenarios"`
	CoveredScenarios int               `json:"covered_scenarios"`
	CoveragePercent float64            `json:"coverage_percent"`
	Categories map[string]*CategoryCoverage `json:"categories"`
	Scenarios []ScenarioCoverage      `json:"scenarios"`
	TestCases map[string]*TestCaseCoverage `json:"test_cases"`
	Summary   *CoverageSummary         `json:"summary"`
}

// CategoryCoverage 分类覆盖度
type CategoryCoverage struct {
	Name           string  `json:"name"`
	TotalScenarios int     `json:"total_scenarios"`
	CoveredScenarios int   `json:"covered_scenarios"`
	CoveragePercent float64 `json:"coverage_percent"`
	Scenarios      []string `json:"scenarios"`
}

// ScenarioCoverage 场景覆盖度
type ScenarioCoverage struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Category      string    `json:"category"`
	Description   string    `json:"description"`
	Priority      string    `json:"priority"`
	Covered       bool      `json:"covered"`
	TestCases     []string  `json:"test_cases"`
	LastTested    time.Time `json:"last_tested,omitempty"`
	PassRate      float64   `json:"pass_rate"`
	FailureReasons []string `json:"failure_reasons,omitempty"`
}

// TestCaseCoverage 测试用例覆盖度
type TestCaseCoverage struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Category     string    `json:"category"`
	Scenarios    []string  `json:"scenarios"`
	Passed       bool      `json:"passed"`
	ExecutedAt   time.Time `json:"executed_at"`
	Duration     string    `json:"duration"`
	ErrorMessage string    `json:"error_message,omitempty"`
}

// CoverageSummary 覆盖度摘要
type CoverageSummary struct {
	BasicScenarios      *CategorySummary `json:"basic_scenarios"`
	AdvancedScenarios   *CategorySummary `json:"advanced_scenarios"`
	ValidationScenarios *CategorySummary `json:"validation_scenarios"`
	TopFailureReasons   []FailureReason  `json:"top_failure_reasons"`
	RecentlyAdded       []string         `json:"recently_added"`
	RecommendedActions  []string         `json:"recommended_actions"`
}

// CategorySummary 分类摘要
type CategorySummary struct {
	Covered   int     `json:"covered"`
	Total     int     `json:"total"`
	Percent   float64 `json:"percent"`
	Status    string  `json:"status"` // "complete", "partial", "minimal"
}

// FailureReason 失败原因
type FailureReason struct {
	Reason string `json:"reason"`
	Count  int    `json:"count"`
}

// CoverageReporter 覆盖度报告生成器
type CoverageReporter struct {
	outputDir string
}

// NewCoverageReporter 创建覆盖度报告生成器
func NewCoverageReporter(outputDir string) *CoverageReporter {
	return &CoverageReporter{
		outputDir: outputDir,
	}
}

// Generate 生成覆盖度矩阵
func (r *CoverageReporter) Generate(scenarios []ScenarioCoverage, testCases map[string]*TestCaseCoverage) (*CoverageMatrix, error) {
	matrix := &CoverageMatrix{
		GeneratedAt:  time.Now(),
		Categories:   make(map[string]*CategoryCoverage),
		Scenarios:    scenarios,
		TestCases:    testCases,
	}

	// 按分类统计覆盖度
	categoryMap := make(map[string][]ScenarioCoverage)
	for _, scenario := range scenarios {
		categoryMap[scenario.Category] = append(categoryMap[scenario.Category], scenario)
		if scenario.Covered {
			matrix.CoveredScenarios++
		}
	}
	matrix.TotalScenarios = len(scenarios)

	// 计算各分类覆盖度
	for category, categoryScenarios := range categoryMap {
		covered := 0
		scenarioIDs := make([]string, len(categoryScenarios))
		for i, scenario := range categoryScenarios {
			scenarioIDs[i] = scenario.ID
			if scenario.Covered {
				covered++
			}
		}

		matrix.Categories[category] = &CategoryCoverage{
			Name:             category,
			TotalScenarios:   len(categoryScenarios),
			CoveredScenarios: covered,
			CoveragePercent:  float64(covered) / float64(len(categoryScenarios)) * 100,
			Scenarios:        scenarioIDs,
		}
	}

	// 计算总体覆盖度
	if matrix.TotalScenarios > 0 {
		matrix.CoveragePercent = float64(matrix.CoveredScenarios) / float64(matrix.TotalScenarios) * 100
	}

	// 生成摘要
	matrix.Summary = r.generateSummary(matrix)

	return matrix, nil
}

// SaveJSON 保存JSON格式报告
func (r *CoverageReporter) SaveJSON(matrix *CoverageMatrix, filename string) error {
	if err := os.MkdirAll(r.outputDir, 0755); err != nil {
		return err
	}

	filePath := filepath.Join(r.outputDir, filename)
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(matrix)
}

// SaveHTML 保存HTML格式报告
func (r *CoverageReporter) SaveHTML(matrix *CoverageMatrix, filename string) error {
	if err := os.MkdirAll(r.outputDir, 0755); err != nil {
		return err
	}

	tmpl := template.Must(template.New("coverage").Parse(htmlTemplate))

	filePath := filepath.Join(r.outputDir, filename)
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	return tmpl.Execute(file, matrix)
}

// generateSummary 生成覆盖度摘要
func (r *CoverageReporter) generateSummary(matrix *CoverageMatrix) *CoverageSummary {
	summary := &CoverageSummary{
		TopFailureReasons:  make([]FailureReason, 0),
		RecentlyAdded:      make([]string, 0),
		RecommendedActions: make([]string, 0),
	}

	// 分类摘要
	if basicCat := matrix.Categories["basic"]; basicCat != nil {
		summary.BasicScenarios = &CategorySummary{
			Covered: basicCat.CoveredScenarios,
			Total:   basicCat.TotalScenarios,
			Percent: basicCat.CoveragePercent,
			Status:  getStatusFromPercent(basicCat.CoveragePercent),
		}
	}

	if advancedCat := matrix.Categories["advanced"]; advancedCat != nil {
		summary.AdvancedScenarios = &CategorySummary{
			Covered: advancedCat.CoveredScenarios,
			Total:   advancedCat.TotalScenarios,
			Percent: advancedCat.CoveragePercent,
			Status:  getStatusFromPercent(advancedCat.CoveragePercent),
		}
	}

	if validationCat := matrix.Categories["validation"]; validationCat != nil {
		summary.ValidationScenarios = &CategorySummary{
			Covered: validationCat.CoveredScenarios,
			Total:   validationCat.TotalScenarios,
			Percent: validationCat.CoveragePercent,
			Status:  getStatusFromPercent(validationCat.CoveragePercent),
		}
	}

	// 统计失败原因
	reasonCount := make(map[string]int)
	for _, scenario := range matrix.Scenarios {
		for _, reason := range scenario.FailureReasons {
			reasonCount[reason]++
		}
	}

	// 排序失败原因
	for reason, count := range reasonCount {
		summary.TopFailureReasons = append(summary.TopFailureReasons, FailureReason{
			Reason: reason,
			Count:  count,
		})
	}
	sort.Slice(summary.TopFailureReasons, func(i, j int) bool {
		return summary.TopFailureReasons[i].Count > summary.TopFailureReasons[j].Count
	})

	// 限制显示数量
	if len(summary.TopFailureReasons) > 5 {
		summary.TopFailureReasons = summary.TopFailureReasons[:5]
	}

	// 生成建议
	summary.RecommendedActions = r.generateRecommendations(matrix)

	return summary
}

// getStatusFromPercent 根据百分比获取状态
func getStatusFromPercent(percent float64) string {
	if percent >= 90 {
		return "complete"
	} else if percent >= 50 {
		return "partial"
	}
	return "minimal"
}

// generateRecommendations 生成建议
func (r *CoverageReporter) generateRecommendations(matrix *CoverageMatrix) []string {
	recommendations := make([]string, 0)

	if matrix.CoveragePercent < 80 {
		recommendations = append(recommendations, "增加更多测试用例以提高整体覆盖度")
	}

	for category, coverage := range matrix.Categories {
		if coverage.CoveragePercent < 70 {
			recommendations = append(recommendations, fmt.Sprintf("补充%s分类的测试场景", category))
		}
	}

	uncoveredCount := matrix.TotalScenarios - matrix.CoveredScenarios
	if uncoveredCount > 0 {
		recommendations = append(recommendations, fmt.Sprintf("还有%d个场景未覆盖，需要添加相应测试用例", uncoveredCount))
	}

	// 分析失败原因
	if matrix.Summary != nil && len(matrix.Summary.TopFailureReasons) > 0 {
		topReason := matrix.Summary.TopFailureReasons[0].Reason
		recommendations = append(recommendations, fmt.Sprintf("重点关注主要失败原因：%s", topReason))
	}

	if len(recommendations) == 0 {
		recommendations = append(recommendations, "覆盖度良好，继续保持测试质量")
	}

	return recommendations
}

// HTML模板
const htmlTemplate = `
<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>IoT协议验证测试覆盖度报告</title>
    <style>
        body { font-family: 'Segoe UI', Arial, sans-serif; margin: 0; padding: 20px; background: #f5f7fa; }
        .container { max-width: 1200px; margin: 0 auto; }
        .header { background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); color: white; padding: 30px; border-radius: 10px; margin-bottom: 30px; }
        .header h1 { margin: 0; font-size: 2.5em; }
        .header .subtitle { margin-top: 10px; opacity: 0.9; }
        .stats-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(250px, 1fr)); gap: 20px; margin-bottom: 30px; }
        .stat-card { background: white; padding: 25px; border-radius: 10px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); }
        .stat-value { font-size: 2.5em; font-weight: bold; color: #333; }
        .stat-label { color: #666; margin-top: 5px; }
        .coverage-bar { width: 100%; height: 8px; background: #e0e0e0; border-radius: 4px; margin-top: 10px; }
        .coverage-fill { height: 100%; border-radius: 4px; transition: width 0.3s ease; }
        .complete { background: #4caf50; }
        .partial { background: #ff9800; }
        .minimal { background: #f44336; }
        .section { background: white; margin-bottom: 30px; border-radius: 10px; overflow: hidden; box-shadow: 0 2px 10px rgba(0,0,0,0.1); }
        .section-header { background: #f8f9fa; padding: 20px; border-bottom: 1px solid #e9ecef; }
        .section-content { padding: 20px; }
        .scenario-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(300px, 1fr)); gap: 15px; }
        .scenario-card { padding: 15px; border: 1px solid #e0e0e0; border-radius: 8px; }
        .scenario-covered { border-left: 4px solid #4caf50; }
        .scenario-uncovered { border-left: 4px solid #f44336; }
        .badge { padding: 4px 8px; border-radius: 4px; font-size: 0.8em; font-weight: bold; }
        .badge-success { background: #d4edda; color: #155724; }
        .badge-danger { background: #f8d7da; color: #721c24; }
        .recommendations { background: #e7f3ff; border-left: 4px solid #2196f3; padding: 15px; border-radius: 4px; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>IoT协议验证测试覆盖度报告</h1>
            <div class="subtitle">生成时间: {{.GeneratedAt.Format "2006-01-02 15:04:05"}}</div>
        </div>

        <div class="stats-grid">
            <div class="stat-card">
                <div class="stat-value">{{.CoveredScenarios}}/{{.TotalScenarios}}</div>
                <div class="stat-label">场景覆盖</div>
                <div class="coverage-bar">
                    <div class="coverage-fill {{if ge .CoveragePercent 90}}complete{{else if ge .CoveragePercent 50}}partial{{else}}minimal{{end}}" 
                         style="width: {{.CoveragePercent}}%"></div>
                </div>
            </div>
            
            {{range $category, $coverage := .Categories}}
            <div class="stat-card">
                <div class="stat-value">{{$coverage.CoveredScenarios}}/{{$coverage.TotalScenarios}}</div>
                <div class="stat-label">{{$coverage.Name}}</div>
                <div class="coverage-bar">
                    <div class="coverage-fill {{if ge $coverage.CoveragePercent 90}}complete{{else if ge $coverage.CoveragePercent 50}}partial{{else}}minimal{{end}}" 
                         style="width: {{$coverage.CoveragePercent}}%"></div>
                </div>
            </div>
            {{end}}
        </div>

        <div class="section">
            <div class="section-header">
                <h2>场景覆盖详情</h2>
            </div>
            <div class="section-content">
                <div class="scenario-grid">
                    {{range .Scenarios}}
                    <div class="scenario-card {{if .Covered}}scenario-covered{{else}}scenario-uncovered{{end}}">
                        <h4>{{.Name}}</h4>
                        <p>{{.Description}}</p>
                        <div>
                            <span class="badge {{if .Covered}}badge-success{{else}}badge-danger{{end}}">
                                {{if .Covered}}已覆盖{{else}}未覆盖{{end}}
                            </span>
                            <small>分类: {{.Category}}</small>
                        </div>
                        {{if .TestCases}}
                        <div style="margin-top: 10px;">
                            <small>测试用例: {{range .TestCases}}{{.}} {{end}}</small>
                        </div>
                        {{end}}
                    </div>
                    {{end}}
                </div>
            </div>
        </div>

        {{if .Summary.RecommendedActions}}
        <div class="section">
            <div class="section-header">
                <h2>建议行动</h2>
            </div>
            <div class="section-content">
                <div class="recommendations">
                    <ul>
                        {{range .Summary.RecommendedActions}}
                        <li>{{.}}</li>
                        {{end}}
                    </ul>
                </div>
            </div>
        </div>
        {{end}}
    </div>
</body>
</html>
`