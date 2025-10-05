package bkv

import (
	"testing"
)

// Week 7: OTA升级协议测试

func TestEncodeOTACommand(t *testing.T) {
	tests := []struct {
		name string
		cmd  *OTACommand
	}{
		{
			name: "DTU Upgrade",
			cmd: &OTACommand{
				TargetType: 0x01,
				SocketNo:   0x00,
				FTPServer:  "192.168.1.100",
				FTPPort:    21,
				FileName:   "firmware.bin",
			},
		},
		{
			name: "Socket Upgrade",
			cmd: &OTACommand{
				TargetType: 0x02,
				SocketNo:   0x05,
				FTPServer:  "10.0.0.1",
				FTPPort:    2121,
				FileName:   "socket_v2.bin",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := EncodeOTACommand(tt.cmd)

			if len(data) != 20 {
				t.Errorf("Expected 20 bytes, got %d", len(data))
			}

			// 验证目标类型
			if data[0] != tt.cmd.TargetType {
				t.Errorf("Expected target_type=%d, got %d", tt.cmd.TargetType, data[0])
			}

			// 验证插座编号
			if data[1] != tt.cmd.SocketNo {
				t.Errorf("Expected socket_no=%d, got %d", tt.cmd.SocketNo, data[1])
			}

			// 验证FTP端口
			port := uint16(data[6])<<8 | uint16(data[7])
			if port != tt.cmd.FTPPort {
				t.Errorf("Expected port=%d, got %d", tt.cmd.FTPPort, port)
			}

			t.Logf("✅ OTA command encoded: %d bytes", len(data))
		})
	}
}

func TestParseOTAResponse(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected *OTAResponse
	}{
		{
			name: "Success",
			data: []byte{0x01, 0x00, 0x00}, // target=DTU, socket=0, result=0
			expected: &OTAResponse{
				TargetType: 0x01,
				SocketNo:   0x00,
				Result:     0,
				Reason:     "",
			},
		},
		{
			name: "FTP Connection Failed",
			data: []byte{0x02, 0x05, 0x02, 'F', 'T', 'P', ' ', 'f', 'a', 'i', 'l', 'e', 'd'},
			expected: &OTAResponse{
				TargetType: 0x02,
				SocketNo:   0x05,
				Result:     0x02,
				Reason:     "FTP failed",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := ParseOTAResponse(tt.data)
			if err != nil {
				t.Fatalf("ParseOTAResponse failed: %v", err)
			}

			if resp.TargetType != tt.expected.TargetType {
				t.Errorf("Expected target_type=%d, got %d", tt.expected.TargetType, resp.TargetType)
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

func TestParseOTAProgress(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected *OTAProgress
	}{
		{
			name: "Downloading",
			data: []byte{0x01, 0x00, 50, 0x00}, // target=DTU, socket=0, progress=50%, status=downloading
			expected: &OTAProgress{
				TargetType: 0x01,
				SocketNo:   0x00,
				Progress:   50,
				Status:     0x00,
				ErrorMsg:   "",
			},
		},
		{
			name: "Installing",
			data: []byte{0x02, 0x03, 80, 0x01}, // target=socket, socket=3, progress=80%, status=installing
			expected: &OTAProgress{
				TargetType: 0x02,
				SocketNo:   0x03,
				Progress:   80,
				Status:     0x01,
				ErrorMsg:   "",
			},
		},
		{
			name: "Complete",
			data: []byte{0x01, 0x00, 100, 0x02}, // target=DTU, progress=100%, status=complete
			expected: &OTAProgress{
				TargetType: 0x01,
				SocketNo:   0x00,
				Progress:   100,
				Status:     0x02,
				ErrorMsg:   "",
			},
		},
		{
			name: "Failed",
			data: []byte{0x02, 0x05, 0, 0x03, 'C', 'h', 'e', 'c', 'k', 's', 'u', 'm', ' ', 'e', 'r', 'r', 'o', 'r'},
			expected: &OTAProgress{
				TargetType: 0x02,
				SocketNo:   0x05,
				Progress:   0,
				Status:     0x03,
				ErrorMsg:   "Checksum error",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			progress, err := ParseOTAProgress(tt.data)
			if err != nil {
				t.Fatalf("ParseOTAProgress failed: %v", err)
			}

			if progress.TargetType != tt.expected.TargetType {
				t.Errorf("Expected target_type=%d, got %d", tt.expected.TargetType, progress.TargetType)
			}

			if progress.SocketNo != tt.expected.SocketNo {
				t.Errorf("Expected socket_no=%d, got %d", tt.expected.SocketNo, progress.SocketNo)
			}

			if progress.Progress != tt.expected.Progress {
				t.Errorf("Expected progress=%d, got %d", tt.expected.Progress, progress.Progress)
			}

			if progress.Status != tt.expected.Status {
				t.Errorf("Expected status=%d, got %d", tt.expected.Status, progress.Status)
			}

			if progress.ErrorMsg != tt.expected.ErrorMsg {
				t.Errorf("Expected error_msg=%s, got %s", tt.expected.ErrorMsg, progress.ErrorMsg)
			}
		})
	}
}

// TestOTA_E2E 端到端集成测试
func TestOTA_E2E(t *testing.T) {
	t.Run("DTU Upgrade Flow", func(t *testing.T) {
		// 1. 编码OTA命令
		cmd := &OTACommand{
			TargetType: 0x01,
			SocketNo:   0x00,
			FTPServer:  "192.168.1.100",
			FTPPort:    21,
			FileName:   "firmware.bin",
		}
		cmdData := EncodeOTACommand(cmd)

		t.Logf("Sent OTA command: %d bytes", len(cmdData))

		// 2. 模拟设备响应（开始升级）
		respData := []byte{0x01, 0x00, 0x00} // Success
		resp, err := ParseOTAResponse(respData)
		if err != nil {
			t.Fatalf("Parse response failed: %v", err)
		}

		if resp.Result != 0 {
			t.Errorf("OTA should start successfully, got result=%d", resp.Result)
		}

		// 3. 模拟设备上报进度
		progressData := []byte{0x01, 0x00, 30, 0x00} // 30% downloading
		progress, err := ParseOTAProgress(progressData)
		if err != nil {
			t.Fatalf("Parse progress failed: %v", err)
		}

		if progress.Progress != 30 || progress.Status != 0 {
			t.Errorf("Expected 30%% downloading, got %d%% status=%d", progress.Progress, progress.Status)
		}

		// 4. 模拟升级完成
		completeData := []byte{0x01, 0x00, 100, 0x02} // 100% complete
		complete, err := ParseOTAProgress(completeData)
		if err != nil {
			t.Fatalf("Parse complete failed: %v", err)
		}

		if complete.Progress != 100 || complete.Status != 2 {
			t.Errorf("Expected 100%% complete, got %d%% status=%d", complete.Progress, complete.Status)
		}

		t.Logf("✅ E2E Test passed: DTU upgrade completed successfully")
	})

	t.Run("Socket Upgrade Failure", func(t *testing.T) {
		// 1. 编码插座升级命令
		cmd := &OTACommand{
			TargetType: 0x02,
			SocketNo:   0x05,
			FTPServer:  "10.0.0.1",
			FTPPort:    2121,
			FileName:   "socket_v2.bin",
		}
		_ = EncodeOTACommand(cmd)

		// 2. 模拟设备响应（FTP连接失败）
		respData := []byte{0x02, 0x05, 0x02, 'F', 'T', 'P', ' ', 'e', 'r', 'r', 'o', 'r'}
		resp, err := ParseOTAResponse(respData)
		if err != nil {
			t.Fatalf("Parse response failed: %v", err)
		}

		if resp.Result != 2 {
			t.Errorf("Expected FTP error (2), got result=%d", resp.Result)
		}

		if resp.Reason != "FTP error" {
			t.Errorf("Expected reason='FTP error', got '%s'", resp.Reason)
		}

		t.Logf("✅ E2E Test passed: Socket upgrade failure handled correctly")
	})
}

func TestOTAHelperFunctions(t *testing.T) {
	t.Run("GetOTAResultDescription", func(t *testing.T) {
		tests := []struct {
			result   uint8
			expected string
		}{
			{0, "成功开始升级"},
			{1, "升级失败"},
			{2, "FTP连接失败"},
			{3, "文件不存在"},
			{99, "未知结果(99)"},
		}

		for _, tt := range tests {
			desc := GetOTAResultDescription(tt.result)
			if desc != tt.expected {
				t.Errorf("Result %d: expected '%s', got '%s'", tt.result, tt.expected, desc)
			}
		}
	})

	t.Run("GetOTAStatusDescription", func(t *testing.T) {
		tests := []struct {
			status   uint8
			expected string
		}{
			{0, "下载中"},
			{1, "安装中"},
			{2, "完成"},
			{3, "失败"},
			{99, "未知状态(99)"},
		}

		for _, tt := range tests {
			desc := GetOTAStatusDescription(tt.status)
			if desc != tt.expected {
				t.Errorf("Status %d: expected '%s', got '%s'", tt.status, tt.expected, desc)
			}
		}
	})
}
