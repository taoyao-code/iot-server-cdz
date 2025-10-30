package bkv

import (
	"encoding/binary"
	"encoding/hex"
	"strings"
	"testing"
)

// TestProtocolDocumentExamples 测试协议文档中的所有示例帧
// 严格按照 "设备对接指引-组网设备2024(1).txt" V1.7 中的示例
func TestProtocolDocumentExamples(t *testing.T) {
	tests := []struct {
		section   string // 协议章节
		name      string // 测试名称
		direction string // uplink/downlink
		rawHex    string // 原始hex
		expected  struct {
			header    uint16 // 帧头
			length    uint16 // 包长
			cmd       uint16 // 命令
			seqNo     uint32 // 流水号
			direction uint8  // 方向
			gatewayID string // 网关ID
		}
	}{
		// 2.1.1 心跳上报
		{
			section:   "2.1.1",
			name:      "心跳上报",
			direction: "uplink",
			rawHex:    "fcfe002e0000000000000182200520004869383938363034363331313230373033313934313763562e31723436001fcafcee",
			expected: struct {
				header    uint16
				length    uint16
				cmd       uint16
				seqNo     uint32
				direction uint8
				gatewayID string
			}{
				header:    0xFCFE,
				length:    0x002E,
				cmd:       0x0000,
				seqNo:     0x00000000,
				direction: 0x01,
				gatewayID: "82200520004869",
			},
		},
		// 2.1.1 心跳回复
		{
			section:   "2.1.1",
			name:      "心跳回复",
			direction: "downlink",
			rawHex:    "fcff0018000000000000008220052000486920200730164545a7fcee",
			expected: struct {
				header    uint16
				length    uint16
				cmd       uint16
				seqNo     uint32
				direction uint8
				gatewayID string
			}{
				header:    0xFCFF,
				length:    0x0018,
				cmd:       0x0000,
				seqNo:     0x00000000,
				direction: 0x00,
				gatewayID: "82200520004869",
			},
		},
		// 2.2.3 插座状态上报 (BKV 0x1017)
		{
			section:   "2.2.3",
			name:      "插座状态上报",
			direction: "uplink",
			rawHex:    "fcfe0091100000000000018223121400270004010110170a010200000000000000000901038223121400270065019403014a0104013effff030107250301961e28015b030108000301098004010a000004019508e304010b000004010c000104010d000004010e000028015b030108010301098004010a000004019508e304010b000004010c000104010d000004010e000030fcee",
			expected: struct {
				header    uint16
				length    uint16
				cmd       uint16
				seqNo     uint32
				direction uint8
				gatewayID string
			}{
				header:    0xFCFE,
				length:    0x0091,
				cmd:       0x1000,
				seqNo:     0x00000000,
				direction: 0x01,
				gatewayID: "82231214002700",
			},
		},
		// 2.2.4 平台查询插座状态
		{
			section:   "2.2.4",
			name:      "平台查询插座状态",
			direction: "downlink",
			rawHex:    "fcff00150015001c91ee008600445945300500011D0181fcee",
			expected: struct {
				header    uint16
				length    uint16
				cmd       uint16
				seqNo     uint32
				direction uint8
				gatewayID string
			}{
				header:    0xFCFF,
				length:    0x0015,
				cmd:       0x0015,
				seqNo:     0x001C91EE,
				direction: 0x00,
				gatewayID: "86004459453005",
			},
		},
		// 2.2.4 设备-插座状态回复
		{
			section:   "2.2.4",
			name:      "设备-插座状态回复",
			direction: "uplink",
			rawHex:    "fcfe00350015001c91ee018600445945300500211c01513629150080000008ef00000001000000000180000008ef000000010000000077fcee",
			expected: struct {
				header    uint16
				length    uint16
				cmd       uint16
				seqNo     uint32
				direction uint8
				gatewayID string
			}{
				header:    0xFCFE,
				length:    0x0035,
				cmd:       0x0015,
				seqNo:     0x001C91EE,
				direction: 0x01,
				gatewayID: "86004459453005",
			},
		},
		// 2.2.5 下发网络节点列表---刷新列表
		{
			section:   "2.2.5",
			name:      "下发网络节点列表---刷新列表",
			direction: "downlink",
			rawHex:    "fcff00310005001c94f90086004459453005001d08040145003070024702450030700743033500307012470425910240232075fcee",
			expected: struct {
				header    uint16
				length    uint16
				cmd       uint16
				seqNo     uint32
				direction uint8
				gatewayID string
			}{
				header:    0xFCFF,
				length:    0x0031,
				cmd:       0x0005,
				seqNo:     0x001C94F9,
				direction: 0x00,
				gatewayID: "86004459453005",
			},
		},
		// 2.2.8 控制设备（按时充电）
		{
			section:   "2.2.8",
			name:      "控制设备（按时充电）",
			direction: "downlink",
			rawHex:    "fcff001c0015001c9a5100860044594530050008070200010100f00000c8fcee",
			expected: struct {
				header    uint16
				length    uint16
				cmd       uint16
				seqNo     uint32
				direction uint8
				gatewayID string
			}{
				header:    0xFCFF,
				length:    0x001C,
				cmd:       0x0015,
				seqNo:     0x001C9A51,
				direction: 0x00,
				gatewayID: "86004459453005",
			},
		},
		// 2.2.8 控制设备回复
		{
			section:   "2.2.8",
			name:      "控制设备回复",
			direction: "uplink",
			rawHex:    "fcfe00190015001c9c2b0186004459453005000507010200006826fcee",
			expected: struct {
				header    uint16
				length    uint16
				cmd       uint16
				seqNo     uint32
				direction uint8
				gatewayID string
			}{
				header:    0xFCFE,
				length:    0x0019,
				cmd:       0x0015,
				seqNo:     0x001C9C2B,
				direction: 0x01,
				gatewayID: "86004459453005",
			},
		},
		// 2.2.9 充电结束上报（按时/按电量）
		{
			section:   "2.2.9",
			name:      "充电结束上报（按时/按电量）",
			direction: "uplink",
			rawHex:    "fcfe00250015000000000186004459453005001102025036302000980068000000010050002d41fcee",
			expected: struct {
				header    uint16
				length    uint16
				cmd       uint16
				seqNo     uint32
				direction uint8
				gatewayID string
			}{
				header:    0xFCFE,
				length:    0x0025,
				cmd:       0x0015,
				seqNo:     0x00000000,
				direction: 0x01,
				gatewayID: "86004459453005",
			},
		},
		// 2.2.1 按功率下发充电命令
		{
			section:   "2.2.1",
			name:      "按功率下发充电命令",
			direction: "downlink",
			rawHex:    "fcff0038000500282bda008600445945300500241701000100640507d00019003c0fa00032003c17700064003c1f400096003c4e2001f4007829fcee",
			expected: struct {
				header    uint16
				length    uint16
				cmd       uint16
				seqNo     uint32
				direction uint8
				gatewayID string
			}{
				header:    0xFCFF,
				length:    0x0038,
				cmd:       0x0005,
				seqNo:     0x00282BDA,
				direction: 0x00,
				gatewayID: "86004459453005",
			},
		},
		// 2.2.2 按功率充电结束上报
		{
			section:   "2.2.2",
			name:      "按功率充电结束上报",
			direction: "uplink",
			rawHex:    "fcfe003c00150000000001860044594530050028180151362d2000980017000000020001002407e406080e150702000f0000050024000000000000000037fcee",
			expected: struct {
				header    uint16
				length    uint16
				cmd       uint16
				seqNo     uint32
				direction uint8
				gatewayID string
			}{
				header:    0xFCFE,
				length:    0x003C,
				cmd:       0x0015,
				seqNo:     0x00000000,
				direction: 0x01,
				gatewayID: "86004459453005",
			},
		},
		// 2.2.2 按电费+服务费下发充电命令
		{
			section:   "2.2.2",
			name:      "按电费+服务费下发充电命令",
			direction: "downlink",
			rawHex:    "fcff00631000215445a5008221022500052004010110070a010200000000215445a50901038221022500052003014a01030108000301130103011204030147010301f40204018800640301800103018901080183173b003200325ffcee",
			expected: struct {
				header    uint16
				length    uint16
				cmd       uint16
				seqNo     uint32
				direction uint8
				gatewayID string
			}{
				header:    0xFCFF,
				length:    0x0063,
				cmd:       0x1000,
				seqNo:     0x215445A5,
				direction: 0x00,
				gatewayID: "82210225000520",
			},
		},
		// 2.2.2 充电结束上报
		{
			section:   "2.2.2",
			name:      "充电结束上报",
			direction: "uplink",
			rawHex:    "fcfe007d100000000000018221022500052004010110040a01020000000000000000090103822102250005200301072a03014a01030108000301099804010a003304010b000004010c000004010d000004010e000109012e2024082310172903012f08030112040401850000040186000003018901080184000100000000dbfcee",
			expected: struct {
				header    uint16
				length    uint16
				cmd       uint16
				seqNo     uint32
				direction uint8
				gatewayID string
			}{
				header:    0xFCFE,
				length:    0x007D,
				cmd:       0x1000,
				seqNo:     0x00000000,
				direction: 0x01,
				gatewayID: "82210225000520",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.section+" - "+tt.name, func(t *testing.T) {
			// 清理hex字符串
			hexClean := strings.ReplaceAll(tt.rawHex, " ", "")
			hexClean = strings.ReplaceAll(hexClean, "\n", "")
			hexClean = strings.ToLower(hexClean)

			data, err := hex.DecodeString(hexClean)
			if err != nil {
				t.Fatalf("十六进制解码失败: %v", err)
			}

			if len(data) < 10 {
				t.Fatalf("帧长度太短: %d字节", len(data))
			}

			// 验证帧头
			header := binary.BigEndian.Uint16(data[0:2])
			if header != tt.expected.header {
				t.Errorf("帧头错误: got 0x%04X, want 0x%04X", header, tt.expected.header)
			}

			// 验证方向（魔术字）
			if tt.direction == "uplink" {
				if data[0] != 0xFC || data[1] != 0xFE {
					t.Errorf("上行帧头错误: %02X%02X (期望: FCFE)", data[0], data[1])
				}
			} else {
				if data[0] != 0xFC || data[1] != 0xFF {
					t.Errorf("下行帧头错误: %02X%02X (期望: FCFF)", data[0], data[1])
				}
			}

			// 验证包长
			length := binary.BigEndian.Uint16(data[2:4])
			if length != tt.expected.length {
				t.Errorf("包长错误: got 0x%04X, want 0x%04X", length, tt.expected.length)
			}

			// 验证命令码
			cmd := binary.BigEndian.Uint16(data[4:6])
			if cmd != tt.expected.cmd {
				t.Errorf("命令码错误: got 0x%04X, want 0x%04X", cmd, tt.expected.cmd)
			}

			// 验证流水号
			seqNo := binary.BigEndian.Uint32(data[6:10])
			if seqNo != tt.expected.seqNo {
				t.Errorf("流水号错误: got 0x%08X, want 0x%08X", seqNo, tt.expected.seqNo)
			}

			// 验证方向字节
			direction := data[10]
			if direction != tt.expected.direction {
				t.Errorf("方向字节错误: got 0x%02X, want 0x%02X", direction, tt.expected.direction)
			}

			// 验证网关ID (7字节)
			if len(data) >= 18 {
				gatewayID := strings.ToUpper(hex.EncodeToString(data[11:18]))
				expectedGW := strings.ToUpper(tt.expected.gatewayID)
				if gatewayID != expectedGW {
					t.Errorf("网关ID错误: got %s, want %s", gatewayID, expectedGW)
				}
			}

			// 验证帧尾
			if len(data) >= 2 {
				if data[len(data)-2] != 0xFC || data[len(data)-1] != 0xEE {
					t.Errorf("帧尾错误: %02X%02X (期望: FCEE)",
						data[len(data)-2], data[len(data)-1])
				}
			}

			t.Logf("✅ 协议文档示例验证通过 - 数据长度: %d字节", len(data))
		})
	}
}

// TestBKVFrameParsing 测试BKV协议帧的解析
func TestBKVFrameParsing(t *testing.T) {
	tests := []struct {
		name       string
		rawHex     string
		wantCmd    uint16
		wantBKV    bool
		bkvCmd     uint16
		frameSeq   uint64
		gatewayID  string
		skip       bool // 跳过有问题的测试用例
		skipReason string
	}{
		{
			name:      "BKV状态上报 0x1017",
			rawHex:    "fcfe0091100000000000018223121400270004010110170a010200000000000000000901038223121400270065019403014a0104013effff030107250301961e28015b030108000301098004010a000004019508e304010b000004010c000104010d000004010e000028015b030108010301098004010a000004019508e304010b000004010c000104010d000004010e000030fcee",
			wantCmd:   0x1000,
			wantBKV:   true,
			bkvCmd:    0x1017,
			frameSeq:  0,
			gatewayID: "82231214002700",
		},
		{
			name:       "BKV控制命令 0x1007",
			rawHex:     "fcff00631000215445a5008221022500052004010110070a010200000000215445a50901038221022500052003014a01030108000301130103011204030147010301f40204018800640301800103018901080183173b003200325ffcee",
			wantCmd:    0x1000,
			wantBKV:    true,
			bkvCmd:     0x1007,
			frameSeq:   0x215445A5,
			gatewayID:  "82210225000520",
			skip:       true,
			skipReason: "协议文档示例长度不匹配: 实际93字节但包长字段为99(0x0063)",
		},
		{
			name:      "BKV充电结束 0x1004",
			rawHex:    "fcfe007d100000000000018221022500052004010110040a01020000000000000000090103822102250005200301072a03014a01030108000301099804010a003304010b000004010c000004010d000004010e000109012e2024082310172903012f08030112040401850000040186000003018901080184000100000000dbfcee",
			wantCmd:   0x1000,
			wantBKV:   true,
			bkvCmd:    0x1004,
			frameSeq:  0,
			gatewayID: "82210225000520",
		},
		{
			name:      "普通心跳帧",
			rawHex:    "fcfe002e0000000000000182200520004869383938363034363331313230373033313934313763562e31723436001fcafcee",
			wantCmd:   0x0000,
			wantBKV:   false,
			gatewayID: "82200520004869",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skip {
				t.Skipf("跳过: %s", tt.skipReason)
				return
			}

			hexClean := strings.ReplaceAll(strings.ToLower(tt.rawHex), " ", "")
			data, err := hex.DecodeString(hexClean)
			if err != nil {
				t.Fatalf("解码失败: %v", err)
			}

			frame, err := Parse(data)
			if err != nil {
				t.Fatalf("解析帧失败: %v", err)
			}

			if frame.Cmd != tt.wantCmd {
				t.Errorf("命令码错误: got 0x%04X, want 0x%04X", frame.Cmd, tt.wantCmd)
			}

			if frame.GatewayID != tt.gatewayID {
				t.Errorf("网关ID错误: got %s, want %s", frame.GatewayID, tt.gatewayID)
			}

			// 检查是否是BKV帧
			isBKV := frame.IsBKVFrame()
			if isBKV != tt.wantBKV {
				t.Errorf("BKV帧判断错误: got %v, want %v", isBKV, tt.wantBKV)
			}

			if tt.wantBKV {
				bkvPayload, err := frame.GetBKVPayload()
				if err != nil {
					t.Fatalf("获取BKV payload失败: %v", err)
				}

				if bkvPayload.Cmd != tt.bkvCmd {
					t.Errorf("BKV命令码错误: got 0x%04X, want 0x%04X", bkvPayload.Cmd, tt.bkvCmd)
				}

				if bkvPayload.FrameSeq != tt.frameSeq {
					t.Errorf("BKV流水号错误: got 0x%016X, want 0x%016X", bkvPayload.FrameSeq, tt.frameSeq)
				}

				if bkvPayload.GatewayID != tt.gatewayID {
					t.Errorf("BKV网关ID错误: got %s, want %s", bkvPayload.GatewayID, tt.gatewayID)
				}

				t.Logf("✅ BKV帧解析成功 - Cmd: 0x%04X, FrameSeq: 0x%016X, GW: %s",
					bkvPayload.Cmd, bkvPayload.FrameSeq, bkvPayload.GatewayID)
			}
		})
	}
}

// TestProtocolFieldExtraction 测试协议字段提取的准确性
func TestProtocolFieldExtraction(t *testing.T) {
	// 2.2.8 控制设备命令详细字段验证
	rawHex := "fcff001c0015001c9a5100860044594530050008070200010100f00000c8fcee"
	hexClean := strings.ReplaceAll(strings.ToLower(rawHex), " ", "")
	data, err := hex.DecodeString(hexClean)
	if err != nil {
		t.Fatalf("解码失败: %v", err)
	}

	// 协议文档规定的字段位置
	// fcff: 帧头 (0-1)
	// 001c: 包长 (2-3)
	// 0015: 命令 (4-5)
	// 001c9a51: 流水号 (6-9)
	// 00: 方向 (10)
	// 86004459453005: 网关ID (11-17)
	// 0008: 帧长 (18-19)
	// 07: 子命令 (20)
	// 02: 插座号 (21)
	// 00: 插孔号 (22)
	// 01: 开关 (23)
	// 01: 类型 (24)
	// 00f0: 时长 (25-26)
	// 0000: 电量 (27-28)
	// c8: 校验和 (29)
	// fcee: 帧尾 (30-31)

	if len(data) != 32 {
		t.Fatalf("数据长度错误: got %d, want 32", len(data))
	}

	// 验证关键字段
	tests := []struct {
		name     string
		offset   int
		length   int
		expected []byte
	}{
		{"帧头", 0, 2, []byte{0xFC, 0xFF}},
		{"包长", 2, 2, []byte{0x00, 0x1C}},
		{"命令", 4, 2, []byte{0x00, 0x15}},
		{"流水号", 6, 4, []byte{0x00, 0x1C, 0x9A, 0x51}},
		{"方向", 10, 1, []byte{0x00}},
		{"网关ID", 11, 7, []byte{0x86, 0x00, 0x44, 0x59, 0x45, 0x30, 0x05}},
		{"子命令", 20, 1, []byte{0x07}},
		{"插座号", 21, 1, []byte{0x02}},
		{"插孔号", 22, 1, []byte{0x00}},
		{"开关", 23, 1, []byte{0x01}},
		{"类型", 24, 1, []byte{0x01}},
		{"时长", 25, 2, []byte{0x00, 0xF0}}, // 240分钟
		{"电量", 27, 2, []byte{0x00, 0x00}},
		{"帧尾", 30, 2, []byte{0xFC, 0xEE}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := data[tt.offset : tt.offset+tt.length]
			if !bytesEqual(actual, tt.expected) {
				t.Errorf("%s字段错误:\n  got:  %s\n  want: %s",
					tt.name,
					hex.EncodeToString(actual),
					hex.EncodeToString(tt.expected))
			}
		})
	}
}

// bytesEqual 比较两个字节数组是否相等
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
