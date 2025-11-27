package app

import (
	"testing"

	"github.com/taoyao-code/iot-server/internal/coremodel"
)

// TestParseBusiness_BKVHexFormat 测试BKV协议的4位十六进制格式解析
// 这是修复parseBusiness函数base转换bug的回归测试
func TestParseBusiness_BKVHexFormat(t *testing.T) {
	tests := []struct {
		name           string
		input          coremodel.BusinessNo
		expectedString string
		expectedValue  int32
		description    string
	}{
		{
			name:           "BKV standard format 0041",
			input:          "0041",
			expectedString: "0041",
			expectedValue:  65,
			description:    "BKV协议标准格式：fmt.Sprintf(\"%04X\", 65) = \"0041\"",
		},
		{
			name:           "BKV standard format FFFF",
			input:          "FFFF",
			expectedString: "FFFF",
			expectedValue:  65535,
			description:    "最大业务号",
		},
		{
			name:           "BKV standard format 00A3",
			input:          "00A3",
			expectedString: "00A3",
			expectedValue:  163,
			description:    "包含字母的十六进制",
		},
		{
			name:           "lowercase hex format",
			input:          "00a3",
			expectedString: "00A3",
			expectedValue:  163,
			description:    "小写字母也应识别为十六进制",
		},
		{
			name:           "0x prefixed format",
			input:          "0x0041",
			expectedString: "0041",
			expectedValue:  65,
			description:    "带0x前缀的格式应正常解析",
		},
		{
			name:           "0X prefixed format",
			input:          "0X00A3",
			expectedString: "00A3",
			expectedValue:  163,
			description:    "带0X前缀的格式应正常解析",
		},
		{
			name:           "zero value normalized to 1",
			input:          "0000",
			expectedString: "0001",
			expectedValue:  1,
			description:    "零值应归一化为1",
		},
		{
			name:           "short decimal format",
			input:          "41",
			expectedString: "0029",
			expectedValue:  41,
			description:    "非4位格式应按十进制解析（保持现有行为）",
		},
		{
			name:           "mixed case",
			input:          "FfAa",
			expectedString: "FFAA",
			expectedValue:  65450,
			description:    "大小写混合应正确识别",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotString, gotValuePtr := parseBusiness(nil, tt.input)

			if gotString != tt.expectedString {
				t.Errorf("parseBusiness(%q) string = %q, want %q\nDescription: %s",
					tt.input, gotString, tt.expectedString, tt.description)
			}

			if gotValuePtr == nil {
				t.Fatalf("parseBusiness(%q) returned nil value pointer", tt.input)
			}

			if *gotValuePtr != tt.expectedValue {
				t.Errorf("parseBusiness(%q) value = %d, want %d\nDescription: %s",
					tt.input, *gotValuePtr, tt.expectedValue, tt.description)
			}

			t.Logf("✓ %s: input=%q → string=%q, value=%d",
				tt.description, tt.input, gotString, *gotValuePtr)
		})
	}
}

// TestParseBusiness_EdgeCases 测试边界情况和错误处理
func TestParseBusiness_EdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		eventBiz    *coremodel.BusinessNo
		payloadBiz  coremodel.BusinessNo
		expectedStr string
		expectedNil bool
		description string
	}{
		{
			name:        "empty payload, no event",
			eventBiz:    nil,
			payloadBiz:  "",
			expectedStr: "",
			expectedNil: true,
			description: "空输入应返回空字符串和nil指针",
		},
		{
			name:        "empty payload, use event fallback",
			eventBiz:    ptrBusinessNo("0041"),
			payloadBiz:  "",
			expectedStr: "0041",
			expectedNil: false,
			description: "payload为空时应使用event中的业务号",
		},
		{
			name:        "whitespace trimming",
			eventBiz:    nil,
			payloadBiz:  "  0041  ",
			expectedStr: "0041",
			expectedNil: false,
			description: "应正确去除前后空白",
		},
		{
			name:        "invalid format returns original",
			eventBiz:    nil,
			payloadBiz:  "ZZZZ",
			expectedStr: "ZZZZ",
			expectedNil: true,
			description: "非法格式应返回原字符串和nil指针",
		},
		{
			name:        "3-digit should not be hex format",
			eventBiz:    nil,
			payloadBiz:  "041",
			expectedStr: "0029",
			expectedNil: false,
			description: "3位字符串不应被识别为BKV格式，按十进制解析",
		},
		{
			name:        "5-digit should not be hex format",
			eventBiz:    nil,
			payloadBiz:  "00041",
			expectedStr: "0029",
			expectedNil: false,
			description: "5位字符串不应被识别为BKV格式，按十进制解析",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotStr, gotPtr := parseBusiness(tt.eventBiz, tt.payloadBiz)

			if gotStr != tt.expectedStr {
				t.Errorf("parseBusiness() string = %q, want %q\nDescription: %s",
					gotStr, tt.expectedStr, tt.description)
			}

			if (gotPtr == nil) != tt.expectedNil {
				t.Errorf("parseBusiness() nil pointer = %v, want %v\nDescription: %s",
					gotPtr == nil, tt.expectedNil, tt.description)
			}

			t.Logf("✓ %s", tt.description)
		})
	}
}

// TestIsBKVHexFormat 测试BKV十六进制格式检测函数
func TestIsBKVHexFormat(t *testing.T) {
	tests := []struct {
		input       string
		expected    bool
		description string
	}{
		{"0041", true, "前导零，应识别为十六进制"},
		{"FFFF", true, "包含F字母"},
		{"00a3", true, "前导零且包含a"},
		{"FfAa", true, "包含A和F"},
		{"0000", false, "特殊情况：全零不识别为前导零"},
		{"1234", false, "纯数字无前导零，保持十进制"},
		{"0123", true, "前导零，应识别为十六进制"},
		{"00AB", true, "包含AB字母"},
		{"", false, "空字符串"},
		{"041", false, "3位"},
		{"00041", false, "5位"},
		{"ZZZZ", false, "非法字符"},
		{"00G3", false, "包含G"},
		{"0x41", false, "包含x"},
		{"12 4", false, "包含空格"},
		{"9999", false, "纯数字无前导零"},
		{"0999", true, "前导零"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := isBKVHexFormat(tt.input)
			if got != tt.expected {
				t.Errorf("isBKVHexFormat(%q) = %v, want %v (%s)",
					tt.input, got, tt.expected, tt.description)
			} else {
				t.Logf("✓ %s: %q → %v", tt.description, tt.input, got)
			}
		})
	}
}

// TestParseBusiness_DecimalCompatibility 测试纯数字十进制兼容性
// 确保"1234"等纯数字字符串保持十进制解析，不会被误判为十六进制
func TestParseBusiness_DecimalCompatibility(t *testing.T) {
	tests := []struct {
		input          coremodel.BusinessNo
		expectedString string
		expectedValue  int32
		description    string
	}{
		{
			input:          "1234",
			expectedString: "04D2",
			expectedValue:  1234,
			description:    "纯数字无前导零应按十进制解析",
		},
		{
			input:          "9999",
			expectedString: "270F",
			expectedValue:  9999,
			description:    "大数字无前导零应按十进制解析",
		},
		{
			input:          "5678",
			expectedString: "162E",
			expectedValue:  5678,
			description:    "中等数字无前导零应按十进制解析",
		},
		{
			input:          "0123",
			expectedString: "0123",
			expectedValue:  291,
			description:    "有前导零的4位数应按十六进制解析（BKV格式）",
		},
		{
			input:          "0041",
			expectedString: "0041",
			expectedValue:  65,
			description:    "BKV协议标准格式（前导零）应按十六进制解析",
		},
	}

	for _, tt := range tests {
		t.Run(string(tt.input), func(t *testing.T) {
			gotString, gotPtr := parseBusiness(nil, tt.input)

			if gotString != tt.expectedString {
				t.Errorf("%s\ninput=%q, got string=%q, want=%q",
					tt.description, tt.input, gotString, tt.expectedString)
			}

			if gotPtr == nil {
				t.Fatal("Value pointer should not be nil")
			}

			if *gotPtr != tt.expectedValue {
				t.Errorf("%s\ninput=%q, got value=%d, want=%d",
					tt.description, tt.input, *gotPtr, tt.expectedValue)
			}

			t.Logf("✓ %s: %q → string=%q, value=%d",
				tt.description, tt.input, gotString, *gotPtr)
		})
	}
}

// TestParseBusiness_ProductionScenario 模拟生产环境的完整场景
// 这是对线上bug的直接回归测试
func TestParseBusiness_ProductionScenario(t *testing.T) {
	t.Log("=== 模拟生产环境bug场景 ===")
	t.Log("设备: 82241218000382")
	t.Log("ACK业务号: 0x0041 (65)")
	t.Log("充电结束业务号: 0x0041 (65)")

	// BKV协议层格式化的业务号（来自fmt.Sprintf("%04X", 65)）
	deviceBusinessNo := coremodel.BusinessNo("0041")

	// 旧代码会错误解析为十进制41
	// 新代码应正确识别为十六进制65
	gotStr, gotPtr := parseBusiness(nil, deviceBusinessNo)

	if gotStr != "0041" {
		t.Errorf("❌ Bug still exists! parseBusiness(\"0041\") = %q, expected \"0041\"", gotStr)
		t.Errorf("   This means \"0041\" was parsed as decimal 41 instead of hex 65")
	} else {
		t.Log("✓ Bug fixed! parseBusiness(\"0041\") = \"0041\"")
	}

	if gotPtr == nil {
		t.Fatal("Value pointer should not be nil")
	}

	if *gotPtr != 65 {
		t.Errorf("❌ Value parsing error! Got %d, expected 65", *gotPtr)
	} else {
		t.Log("✓ Value correctly parsed as 65")
	}

	t.Log("=== SessionEnded event will now show correct business_no ===")
	t.Logf("Expected log: business_no=\"0041\" (was incorrectly \"0029\" before fix)")
}

func ptrBusinessNo(s string) *coremodel.BusinessNo {
	b := coremodel.BusinessNo(s)
	return &b
}
