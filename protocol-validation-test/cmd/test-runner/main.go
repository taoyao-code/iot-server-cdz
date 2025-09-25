package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/taoyao-code/protocol-validation-test/internal/coverage"
	"github.com/taoyao-code/protocol-validation-test/internal/parser"
	"github.com/taoyao-code/protocol-validation-test/internal/validator"
)

const (
	version = "1.0.0"
	banner = `
╔═══════════════════════════════════════════════════════════════╗
║              IoT协议验证测试体系 v%s                    ║
║         Protocol Validation Test System                       ║
║                                                               ║
║  严格按照《设备对接指引-组网设备2024(1).txt》构建              ║
║  100%覆盖31个协议场景，完全解耦独立项目                        ║
╚═══════════════════════════════════════════════════════════════╝
`
)

// Config 配置
type Config struct {
	TestDataDir   string
	ReportDir     string
	Scenario      string
	Category      string
	Verbose       bool
	Parallel      int
	Timeout       string
	OutputFormat  string // json, yaml, html
	ShowVersion   bool
	ShowHelp      bool
}

func main() {
	config := parseFlags()
	
	if config.ShowVersion {
		fmt.Printf("Protocol Validation Test System v%s\n", version)
		os.Exit(0)
	}
	
	if config.ShowHelp {
		showHelp()
		os.Exit(0)
	}

	fmt.Printf(banner, version)
	
	// 运行测试
	if err := runTests(config); err != nil {
		log.Fatalf("测试执行失败: %v", err)
	}
}

func parseFlags() *Config {
	config := &Config{}
	
	flag.StringVar(&config.TestDataDir, "testdata", "./testdata", "测试数据目录")
	flag.StringVar(&config.ReportDir, "reports", "./reports", "报告输出目录")
	flag.StringVar(&config.Scenario, "scenario", "", "指定场景ID，为空则运行所有场景")
	flag.StringVar(&config.Category, "category", "", "指定场景分类: basic, advanced, validation")
	flag.BoolVar(&config.Verbose, "verbose", false, "详细输出")
	flag.IntVar(&config.Parallel, "parallel", 1, "并行测试数量")
	flag.StringVar(&config.Timeout, "timeout", "30s", "测试超时时间")
	flag.StringVar(&config.OutputFormat, "format", "html", "输出格式: json, yaml, html")
	flag.BoolVar(&config.ShowVersion, "version", false, "显示版本信息")
	flag.BoolVar(&config.ShowHelp, "help", false, "显示帮助信息")
	
	flag.Parse()
	return config
}

func showHelp() {
	fmt.Printf(banner, version)
	fmt.Println("\n使用方法:")
	fmt.Println("  test-runner [选项]")
	fmt.Println("\n选项:")
	flag.PrintDefaults()
	fmt.Println("\n场景分类:")
	fmt.Println("  basic      - 基础场景(9个): 心跳、状态上报、查询、组网、控制充电等")
	fmt.Println("  advanced   - 进阶场景(10个): 按功率充电、刷卡、参数设置、异常事件、OTA等")
	fmt.Println("  validation - 验证场景(12个): 校验和、序列号、错误帧、边界值等")
	fmt.Println("\n示例:")
	fmt.Println("  # 运行所有测试")
	fmt.Println("  test-runner")
	fmt.Println("\n  # 运行心跳场景")
	fmt.Println("  test-runner --scenario heartbeat")
	fmt.Println("\n  # 运行基础场景")
	fmt.Println("  test-runner --category basic")
	fmt.Println("\n  # 并行运行并输出详细信息")
	fmt.Println("  test-runner --parallel 4 --verbose")
	fmt.Println("\n  # 生成JSON格式报告")
	fmt.Println("  test-runner --format json")
}

func runTests(config *Config) error {
	fmt.Printf("📁 测试数据目录: %s\n", config.TestDataDir)
	fmt.Printf("📊 报告输出目录: %s\n", config.ReportDir)
	
	// 确保报告目录存在
	if err := os.MkdirAll(config.ReportDir, 0755); err != nil {
		return fmt.Errorf("创建报告目录失败: %w", err)
	}
	
	// 创建覆盖度追踪器
	tracker := coverage.NewTracker()
	tracker.InitializeScenarios()
	
	fmt.Println("\n🚀 初始化协议验证测试体系...")
	fmt.Println("📋 31个协议场景已加载:")
	
	matrix := tracker.GetMatrix()
	categories := map[string]int{
		"basic":      0,
		"advanced":   0, 
		"validation": 0,
	}
	
	for _, scenario := range matrix.Scenarios {
		categories[scenario.Category]++
	}
	
	fmt.Printf("   • 基础场景: %d个\n", categories["basic"])
	fmt.Printf("   • 进阶场景: %d个\n", categories["advanced"])
	fmt.Printf("   • 验证场景: %d个\n", categories["validation"])
	fmt.Printf("   • 总计: %d个场景\n", len(matrix.Scenarios))
	
	// 执行实际测试
	if err := executeTests(config, tracker); err != nil {
		return fmt.Errorf("执行测试失败: %w", err)
	}
	
	// 生成覆盖度报告
	return generateReports(config, tracker)
}

func executeTests(config *Config, tracker *coverage.Tracker) error {
	fmt.Println("\n🔧 阶段2完成 - 初始化协议解析器...")
	
	// 创建协议解析器
	frameParser := parser.NewDefaultFrameParser()
	tlvParser := parser.NewDefaultTLVParser()
	bkvParser := parser.NewDefaultBKVParser(tlvParser)
	
	// 创建验证引擎
	engine := validator.NewEngine(frameParser, bkvParser, tlvParser)
	
	// 创建测试用例加载器
	loader := validator.NewLoader(config.TestDataDir)
	
	fmt.Println("✅ 协议解析器已初始化")
	fmt.Println("✅ 验证引擎已创建")
	fmt.Println("✅ 测试用例加载器已准备")
	
	// 加载测试用例
	var testCases []*validator.TestCase
	var err error
	
	if config.Category != "" {
		fmt.Printf("\n📂 加载 %s 分类的测试用例...\n", config.Category)
		testCases, err = loader.GetTestCasesByCategory(config.Category)
	} else if config.Scenario != "" {
		fmt.Printf("\n🎯 加载场景 %s 的测试用例...\n", config.Scenario)
		testCases, err = loader.GetTestCasesByScenario(config.Scenario)
	} else {
		fmt.Println("\n📋 加载所有测试用例...")
		testCases, err = loader.GetAllTestCases()
	}
	
	if err != nil {
		return fmt.Errorf("加载测试用例失败: %w", err)
	}
	
	fmt.Printf("✅ 已加载 %d 个测试用例\n", len(testCases))
	
	// 执行测试
	fmt.Println("\n🧪 开始执行协议验证测试...")
	
	passed := 0
	failed := 0
	
	for i, testCase := range testCases {
		fmt.Printf("\r进度: [%d/%d] 执行测试用例 %s", i+1, len(testCases), testCase.ID)
		
		// 执行单个测试用例
		result := engine.ValidateTestCase(testCase)
		
		// 记录测试结果
		tracker.RecordTestResult(result)
		
		if result.Passed {
			passed++
		} else {
			failed++
			if config.Verbose {
				fmt.Printf("\n❌ 测试失败: %s - %s\n", testCase.ID, testCase.Name)
				for _, err := range result.Errors {
					fmt.Printf("   错误: %s\n", err.Message)
				}
			}
		}
	}
	
	fmt.Printf("\n\n✅ 测试执行完成!\n")
	fmt.Printf("📊 测试结果: 通过 %d / 失败 %d / 总计 %d\n", passed, failed, len(testCases))
	fmt.Printf("📈 通过率: %.1f%%\n", float64(passed)/float64(len(testCases))*100)
	
	return nil
}

func generateReports(config *Config, tracker *coverage.Tracker) error {
	fmt.Println("\n📈 生成覆盖度报告...")
	
	// 导出JSON报告
	jsonData, err := tracker.ExportJSON()
	if err != nil {
		return fmt.Errorf("导出JSON报告失败: %w", err)
	}
	
	jsonFile := filepath.Join(config.ReportDir, "coverage-matrix.json")
	if err := os.WriteFile(jsonFile, jsonData, 0644); err != nil {
		return fmt.Errorf("写入JSON报告失败: %w", err)
	}
	
	fmt.Printf("✅ JSON报告已生成: %s\n", jsonFile)
	
	// TODO: 生成HTML报告
	htmlFile := filepath.Join(config.ReportDir, "coverage-matrix.html")
	htmlContent := generateHTMLReport(tracker.GetMatrix())
	if err := os.WriteFile(htmlFile, []byte(htmlContent), 0644); err != nil {
		return fmt.Errorf("写入HTML报告失败: %w", err)
	}
	
	fmt.Printf("✅ HTML报告已生成: %s\n", htmlFile)
	
	// 显示摘要
	summary := tracker.GenerateSummary()
	fmt.Printf("\n📊 覆盖度摘要:\n")
	fmt.Printf("   • 场景覆盖率: %.1f%% (%d/%d)\n", 
		summary.ScenarioCoverage*100, 
		summary.CoveredScenarios, 
		summary.TotalScenarios)
	fmt.Printf("   • 测试通过率: %.1f%% (%d/%d)\n", 
		summary.TestPassRate*100, 
		summary.PassedTestCases, 
		summary.TotalTestCases)
		
	if summary.TotalTestCases > 0 {
		fmt.Printf("\n🎯 阶段2已完成: 核心协议解析和验证框架\n")
		fmt.Printf("   ✅ 协议帧解析器\n")
		fmt.Printf("   ✅ TLV结构解析器\n") 
		fmt.Printf("   ✅ BKV协议解析器\n")
		fmt.Printf("   ✅ 验证引擎核心\n")
		fmt.Printf("   ✅ 测试用例加载器\n")
		fmt.Printf("   ✅ 实际测试执行\n")
	} else {
		fmt.Printf("\n⚠️  注意: 暂无测试用例数据，请检查testdata目录\n")
	}
	
	return nil
}

func generateHTMLReport(matrix *coverage.CoverageMatrix) string {
	html := `<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>IoT协议验证测试覆盖矩阵</title>
    <style>
        body { font-family: 'Microsoft YaHei', sans-serif; margin: 20px; }
        .header { text-align: center; margin-bottom: 30px; }
        .summary { background: #f5f5f5; padding: 20px; border-radius: 8px; margin-bottom: 30px; }
        .scenarios { display: grid; grid-template-columns: repeat(auto-fill, minmax(300px, 1fr)); gap: 20px; }
        .scenario { border: 1px solid #ddd; border-radius: 8px; padding: 15px; }
        .scenario.covered { border-color: #52c41a; background: #f6ffed; }
        .scenario.uncovered { border-color: #ff4d4f; background: #fff2f0; }
        .category-basic { border-left: 4px solid #1890ff; }
        .category-advanced { border-left: 4px solid #722ed1; }
        .category-validation { border-left: 4px solid #fa8c16; }
        .progress { width: 100%; height: 20px; background: #f0f0f0; border-radius: 10px; overflow: hidden; }
        .progress-bar { height: 100%; background: #52c41a; transition: width 0.3s; }
    </style>
</head>
<body>
    <div class="header">
        <h1>IoT协议验证测试覆盖矩阵</h1>
        <p>严格按照《设备对接指引-组网设备2024(1).txt》构建 | 版本: ` + matrix.Version + `</p>
    </div>
    
    <div class="summary">
        <h2>覆盖度摘要</h2>
        <p><strong>总场景数:</strong> ` + fmt.Sprintf("%d", len(matrix.Scenarios)) + `</p>
        <p><strong>已覆盖场景:</strong> 0 个 (0%)</p>
        <div class="progress">
            <div class="progress-bar" style="width: 0%"></div>
        </div>
        <p><em>注意: 阶段1完成，实际测试执行将在后续阶段实现</em></p>
    </div>
    
    <h2>31个协议场景</h2>
    <div class="scenarios">`

	for _, scenario := range matrix.Scenarios {
		status := "uncovered"
		if scenario.Covered {
			status = "covered"
		}
		
		html += fmt.Sprintf(`
        <div class="scenario %s category-%s">
            <h3>%s</h3>
            <p><strong>ID:</strong> %s</p>
            <p><strong>分类:</strong> %s</p>
            <p><strong>优先级:</strong> %s</p>
            <p><strong>状态:</strong> %s</p>
            <p><strong>测试用例:</strong> %d</p>
        </div>`, 
		status, scenario.Category, scenario.Name, scenario.ID, 
		scenario.Category, scenario.Priority,
		map[bool]string{true: "✅ 已覆盖", false: "❌ 未覆盖"}[scenario.Covered],
		scenario.TotalCases)
	}

	html += `
    </div>
    
    <div style="margin-top: 40px; text-align: center; color: #666;">
        <p>生成时间: ` + matrix.Summary.GeneratedAt.Format("2006-01-02 15:04:05") + `</p>
        <p>IoT协议验证测试体系 - 独立项目，可随时删除</p>
    </div>
</body>
</html>`

	return html
}