package bkv

import (
	"testing"
)

// Week 6: 组网管理协议测试

func TestEncodeNetworkRefreshCommand(t *testing.T) {
	data := EncodeNetworkRefreshCommand()

	// 刷新命令无参数
	if len(data) != 0 {
		t.Errorf("Expected empty data, got %d bytes", len(data))
	}
}

func TestParseNetworkRefreshResponse(t *testing.T) {
	// 构造测试数据：2个插座
	data := []byte{
		0x02, // 插座数量 = 2

		// 插座1
		0x01,                               // 编号 = 1
		0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF, // MAC
		0x12, 0x34, 0x56, 0x78, // UID
		0x06, // 信道 = 6
		0xD0, // 信号强度 = -48 (0xD0 = -48)
		0x01, // 状态 = 在线

		// 插座2
		0x02,                               // 编号 = 2
		0x11, 0x22, 0x33, 0x44, 0x55, 0x66, // MAC
		0xAB, 0xCD, 0xEF, 0x01, // UID
		0x0B, // 信道 = 11
		0xC8, // 信号强度 = -56
		0x00, // 状态 = 离线
	}

	resp, err := ParseNetworkRefreshResponse(data)
	if err != nil {
		t.Fatalf("ParseNetworkRefreshResponse failed: %v", err)
	}

	if resp.SocketCount != 2 {
		t.Errorf("Expected socket count 2, got %d", resp.SocketCount)
	}

	if len(resp.Sockets) != 2 {
		t.Fatalf("Expected 2 sockets, got %d", len(resp.Sockets))
	}

	// 验证插座1
	s1 := resp.Sockets[0]
	if s1.SocketNo != 1 {
		t.Errorf("Socket1: expected no=1, got %d", s1.SocketNo)
	}
	if s1.SocketMAC != "AA:BB:CC:DD:EE:FF" {
		t.Errorf("Socket1: expected MAC=AA:BB:CC:DD:EE:FF, got %s", s1.SocketMAC)
	}
	if s1.SocketUID != "12345678" {
		t.Errorf("Socket1: expected UID=12345678, got %s", s1.SocketUID)
	}
	if s1.Channel != 6 {
		t.Errorf("Socket1: expected channel=6, got %d", s1.Channel)
	}
	if s1.SignalStrength != -48 {
		t.Errorf("Socket1: expected signal=-48, got %d", s1.SignalStrength)
	}
	if s1.Status != 1 {
		t.Errorf("Socket1: expected status=1, got %d", s1.Status)
	}

	// 验证插座2
	s2 := resp.Sockets[1]
	if s2.SocketNo != 2 {
		t.Errorf("Socket2: expected no=2, got %d", s2.SocketNo)
	}
	if s2.Status != 0 {
		t.Errorf("Socket2: expected status=0, got %d", s2.Status)
	}
}

func TestEncodeNetworkAddNodeCommand(t *testing.T) {
	cmd := &NetworkAddNodeCommand{
		SocketNo:  10,
		SocketMAC: "AA:BB:CC:DD:EE:FF",
		Channel:   6,
	}

	data := EncodeNetworkAddNodeCommand(cmd)

	if len(data) != 8 {
		t.Fatalf("Expected 8 bytes, got %d", len(data))
	}

	if data[0] != 10 {
		t.Errorf("Expected socket_no=10, got %d", data[0])
	}

	// 验证MAC地址
	expectedMAC := []byte{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF}
	for i := 0; i < 6; i++ {
		if data[1+i] != expectedMAC[i] {
			t.Errorf("MAC byte %d: expected 0x%02X, got 0x%02X", i, expectedMAC[i], data[1+i])
		}
	}

	if data[7] != 6 {
		t.Errorf("Expected channel=6, got %d", data[7])
	}
}

func TestParseNetworkAddNodeResponse(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected *NetworkAddNodeResponse
	}{
		{
			name: "Success",
			data: []byte{0x0A, 0x00}, // socket_no=10, result=0
			expected: &NetworkAddNodeResponse{
				SocketNo: 10,
				Result:   0,
				Reason:   "",
			},
		},
		{
			name: "Failed with reason",
			data: []byte{0x0A, 0x02, 'c', 'o', 'n', 'f', 'l', 'i', 'c', 't'}, // result=2, reason="conflict"
			expected: &NetworkAddNodeResponse{
				SocketNo: 10,
				Result:   2,
				Reason:   "conflict",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := ParseNetworkAddNodeResponse(tt.data)
			if err != nil {
				t.Fatalf("ParseNetworkAddNodeResponse failed: %v", err)
			}

			if resp.SocketNo != tt.expected.SocketNo {
				t.Errorf("Expected socket_no=%d, got %d", tt.expected.SocketNo, resp.SocketNo)
			}

			if resp.Result != tt.expected.Result {
				t.Errorf("Expected result=%d, got %d", tt.expected.Result, resp.Result)
			}

			if resp.Reason != tt.expected.Reason {
				t.Errorf("Expected reason=%s, got %s", tt.expected.Reason, resp.Reason)
			}
		})
	}
}

func TestEncodeNetworkDeleteNodeCommand(t *testing.T) {
	cmd := &NetworkDeleteNodeCommand{
		SocketNo: 15,
	}

	data := EncodeNetworkDeleteNodeCommand(cmd)

	if len(data) != 1 {
		t.Fatalf("Expected 1 byte, got %d", len(data))
	}

	if data[0] != 15 {
		t.Errorf("Expected socket_no=15, got %d", data[0])
	}
}

func TestParseNetworkDeleteNodeResponse(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected *NetworkDeleteNodeResponse
	}{
		{
			name: "Success",
			data: []byte{0x0F, 0x00}, // socket_no=15, result=0
			expected: &NetworkDeleteNodeResponse{
				SocketNo: 15,
				Result:   0,
				Reason:   "",
			},
		},
		{
			name: "Not found",
			data: []byte{0x0F, 0x02, 'n', 'o', 't', ' ', 'f', 'o', 'u', 'n', 'd'}, // result=2
			expected: &NetworkDeleteNodeResponse{
				SocketNo: 15,
				Result:   2,
				Reason:   "not found",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := ParseNetworkDeleteNodeResponse(tt.data)
			if err != nil {
				t.Fatalf("ParseNetworkDeleteNodeResponse failed: %v", err)
			}

			if resp.SocketNo != tt.expected.SocketNo {
				t.Errorf("Expected socket_no=%d, got %d", tt.expected.SocketNo, resp.SocketNo)
			}

			if resp.Result != tt.expected.Result {
				t.Errorf("Expected result=%d, got %d", tt.expected.Result, resp.Result)
			}

			if resp.Reason != tt.expected.Reason {
				t.Errorf("Expected reason=%s, got %s", tt.expected.Reason, resp.Reason)
			}
		})
	}
}

// TestNetworkManagement_E2E 端到端集成测试
func TestNetworkManagement_E2E(t *testing.T) {
	t.Run("RefreshSocketsList", func(t *testing.T) {
		// 1. 编码刷新命令
		cmdData := EncodeNetworkRefreshCommand()
		if len(cmdData) != 0 {
			t.Errorf("Refresh command should be empty")
		}

		// 2. 模拟设备响应（3个插座）
		respData := []byte{
			0x03, // 3个插座
			// 插座1
			0x01, 0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF,
			0x11, 0x22, 0x33, 0x44, 0x06, 0xD0, 0x01,
			// 插座2
			0x02, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66,
			0xAA, 0xBB, 0xCC, 0xDD, 0x0B, 0xC8, 0x01,
			// 插座3
			0x03, 0xFF, 0xEE, 0xDD, 0xCC, 0xBB, 0xAA,
			0x99, 0x88, 0x77, 0x66, 0x01, 0xE0, 0x00,
		}

		// 3. 解析响应
		resp, err := ParseNetworkRefreshResponse(respData)
		if err != nil {
			t.Fatalf("Parse response failed: %v", err)
		}

		if len(resp.Sockets) != 3 {
			t.Errorf("Expected 3 sockets, got %d", len(resp.Sockets))
		}

		t.Logf("✅ E2E Test passed: Refresh returned %d sockets", len(resp.Sockets))
	})

	t.Run("AddAndDeleteSocket", func(t *testing.T) {
		// 1. 添加插座
		addCmd := &NetworkAddNodeCommand{
			SocketNo:  20,
			SocketMAC: "AA:BB:CC:DD:EE:FF",
			Channel:   6,
		}
		_ = EncodeNetworkAddNodeCommand(addCmd) // 编码命令（实际应用中会发送到设备）

		// 2. 模拟成功响应
		addRespData := []byte{0x14, 0x00} // socket_no=20, result=0
		addResp, err := ParseNetworkAddNodeResponse(addRespData)
		if err != nil {
			t.Fatalf("Parse add response failed: %v", err)
		}

		if addResp.Result != 0 {
			t.Errorf("Add socket failed: %s", addResp.Reason)
		}

		// 3. 删除插座
		delCmd := &NetworkDeleteNodeCommand{SocketNo: 20}
		_ = EncodeNetworkDeleteNodeCommand(delCmd) // 编码命令（实际应用中会发送到设备）

		// 4. 模拟成功响应
		delRespData := []byte{0x14, 0x00} // socket_no=20, result=0
		delResp, err := ParseNetworkDeleteNodeResponse(delRespData)
		if err != nil {
			t.Fatalf("Parse delete response failed: %v", err)
		}

		if delResp.Result != 0 {
			t.Errorf("Delete socket failed: %s", delResp.Reason)
		}

		t.Logf("✅ E2E Test passed: Add and delete socket successfully")
	})
}
