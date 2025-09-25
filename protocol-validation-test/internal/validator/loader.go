package validator

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// TestSuite 测试套件
type TestSuite struct {
	Name        string                 `yaml:"name" json:"name"`
	Description string                 `yaml:"description" json:"description"`
	Category    string                 `yaml:"category" json:"category"`
	Version     string                 `yaml:"version" json:"version"`
	Author      string                 `yaml:"author" json:"author"`
	CreatedAt   string                 `yaml:"created_at" json:"created_at"`
	Scenarios   []TestCase             `yaml:"scenarios" json:"scenarios"`
	Metadata    map[string]interface{} `yaml:"metadata,omitempty" json:"metadata,omitempty"`
}

// Loader 测试用例加载器
type Loader struct {
	testDataDir string
}

// NewLoader 创建测试用例加载器
func NewLoader(testDataDir string) *Loader {
	return &Loader{
		testDataDir: testDataDir,
	}
}

// LoadAllTestSuites 加载所有测试套件
func (l *Loader) LoadAllTestSuites() ([]*TestSuite, error) {
	var suites []*TestSuite

	// 扫描测试数据目录
	err := filepath.WalkDir(l.testDataDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// 只处理YAML文件
		if d.IsDir() || !strings.HasSuffix(strings.ToLower(path), ".yaml") {
			return nil
		}

		// 跳过schemas目录
		if strings.Contains(path, "schemas") {
			return nil
		}

		suite, err := l.LoadTestSuite(path)
		if err != nil {
			return fmt.Errorf("failed to load test suite %s: %w", path, err)
		}

		if suite != nil {
			suites = append(suites, suite)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk test data directory: %w", err)
	}

	return suites, nil
}

// LoadTestSuite 加载单个测试套件
func (l *Loader) LoadTestSuite(filePath string) (*TestSuite, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", filePath, err)
	}

	var suite TestSuite
	if err := yaml.Unmarshal(data, &suite); err != nil {
		return nil, fmt.Errorf("failed to unmarshal YAML from %s: %w", filePath, err)
	}

	// 为每个测试用例设置文件路径信息
	for i := range suite.Scenarios {
		if suite.Scenarios[i].Metadata == nil {
			suite.Scenarios[i].Metadata = make(map[string]interface{})
		}
		suite.Scenarios[i].Metadata["source_file"] = filePath
		suite.Scenarios[i].Category = suite.Category
	}

	return &suite, nil
}

// LoadTestSuitesByCategory 按分类加载测试套件
func (l *Loader) LoadTestSuitesByCategory(category string) ([]*TestSuite, error) {
	categoryDir := filepath.Join(l.testDataDir, "scenarios", category)
	
	if _, err := os.Stat(categoryDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("category directory not found: %s", categoryDir)
	}

	var suites []*TestSuite

	err := filepath.WalkDir(categoryDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() || !strings.HasSuffix(strings.ToLower(path), ".yaml") {
			return nil
		}

		suite, err := l.LoadTestSuite(path)
		if err != nil {
			return fmt.Errorf("failed to load test suite %s: %w", path, err)
		}

		if suite != nil && suite.Category == category {
			suites = append(suites, suite)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk category directory %s: %w", categoryDir, err)
	}

	return suites, nil
}

// LoadTestCase 加载单个测试用例
func (l *Loader) LoadTestCase(suiteFile, testCaseID string) (*TestCase, error) {
	suite, err := l.LoadTestSuite(suiteFile)
	if err != nil {
		return nil, err
	}

	for _, testCase := range suite.Scenarios {
		if testCase.ID == testCaseID {
			return &testCase, nil
		}
	}

	return nil, fmt.Errorf("test case %s not found in suite %s", testCaseID, suiteFile)
}

// GetAllTestCases 获取所有测试用例
func (l *Loader) GetAllTestCases() ([]*TestCase, error) {
	suites, err := l.LoadAllTestSuites()
	if err != nil {
		return nil, err
	}

	var testCases []*TestCase
	for _, suite := range suites {
		for i := range suite.Scenarios {
			testCases = append(testCases, &suite.Scenarios[i])
		}
	}

	return testCases, nil
}

// GetTestCasesByCategory 按分类获取测试用例
func (l *Loader) GetTestCasesByCategory(category string) ([]*TestCase, error) {
	suites, err := l.LoadTestSuitesByCategory(category)
	if err != nil {
		return nil, err
	}

	var testCases []*TestCase
	for _, suite := range suites {
		for i := range suite.Scenarios {
			testCases = append(testCases, &suite.Scenarios[i])
		}
	}

	return testCases, nil
}

// ValidateTestSuite 验证测试套件格式
func (l *Loader) ValidateTestSuite(suite *TestSuite) error {
	if suite.Name == "" {
		return fmt.Errorf("test suite name is required")
	}

	if suite.Category == "" {
		return fmt.Errorf("test suite category is required")
	}

	validCategories := []string{"basic", "advanced", "validation"}
	categoryValid := false
	for _, validCategory := range validCategories {
		if suite.Category == validCategory {
			categoryValid = true
			break
		}
	}
	if !categoryValid {
		return fmt.Errorf("invalid category: %s", suite.Category)
	}

	if len(suite.Scenarios) == 0 {
		return fmt.Errorf("test suite must contain at least one scenario")
	}

	// 验证每个测试用例
	for i, testCase := range suite.Scenarios {
		if err := l.ValidateTestCase(&testCase); err != nil {
			return fmt.Errorf("invalid test case at index %d: %w", i, err)
		}
	}

	return nil
}

// ValidateTestCase 验证测试用例格式
func (l *Loader) ValidateTestCase(testCase *TestCase) error {
	if testCase.ID == "" {
		return fmt.Errorf("test case ID is required")
	}

	if testCase.Name == "" {
		return fmt.Errorf("test case name is required")
	}

	if testCase.Input.HexFrame == "" {
		return fmt.Errorf("test case hex frame is required")
	}

	// 验证十六进制字符串格式
	if _, err := hexToBytes(testCase.Input.HexFrame); err != nil {
		return fmt.Errorf("invalid hex frame format: %w", err)
	}

	validTypes := []string{"uplink", "downlink", "bidirectional"}
	typeValid := false
	for _, validType := range validTypes {
		if testCase.Type == validType {
			typeValid = true
			break
		}
	}
	if !typeValid {
		return fmt.Errorf("invalid test case type: %s", testCase.Type)
	}

	return nil
}

// GetTestSuiteStatistics 获取测试套件统计信息
func (l *Loader) GetTestSuiteStatistics() (*TestSuiteStatistics, error) {
	suites, err := l.LoadAllTestSuites()
	if err != nil {
		return nil, err
	}

	stats := &TestSuiteStatistics{
		TotalSuites:    len(suites),
		TotalTestCases: 0,
		Categories:     make(map[string]int),
		Types:          make(map[string]int),
	}

	for _, suite := range suites {
		stats.Categories[suite.Category]++
		
		for _, testCase := range suite.Scenarios {
			stats.TotalTestCases++
			stats.Types[testCase.Type]++
		}
	}

	return stats, nil
}

// TestSuiteStatistics 测试套件统计信息
type TestSuiteStatistics struct {
	TotalSuites    int            `json:"total_suites"`
	TotalTestCases int            `json:"total_test_cases"`
	Categories     map[string]int `json:"categories"`
	Types          map[string]int `json:"types"`
}

// FindTestCase 查找测试用例
func (l *Loader) FindTestCase(id string) (*TestCase, error) {
	testCases, err := l.GetAllTestCases()
	if err != nil {
		return nil, err
	}

	for _, testCase := range testCases {
		if testCase.ID == id {
			return testCase, nil
		}
	}

	return nil, fmt.Errorf("test case not found: %s", id)
}

// GetTestCasesByScenario 根据场景获取测试用例
func (l *Loader) GetTestCasesByScenario(scenario string) ([]*TestCase, error) {
	testCases, err := l.GetAllTestCases()
	if err != nil {
		return nil, err
	}

	var matchedCases []*TestCase
	for _, testCase := range testCases {
		if testCase.Input.Scenario == scenario {
			matchedCases = append(matchedCases, testCase)
		}
	}

	return matchedCases, nil
}