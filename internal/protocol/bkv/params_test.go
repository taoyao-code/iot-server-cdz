package bkv

import (
	"testing"
)

// Week 9: 参数管理协议测试

func TestParamReadRequest(t *testing.T) {
	tests := []struct {
		name     string
		paramIDs []uint16
	}{
		{
			name:     "Single Param",
			paramIDs: []uint16{0x0001},
		},
		{
			name:     "Multiple Params",
			paramIDs: []uint16{0x0001, 0x0002, 0x0003, 0x0010, 0x0020},
		},
		{
			name: "Max Params",
			paramIDs: func() []uint16 {
				ids := make([]uint16, 20)
				for i := 0; i < 20; i++ {
					ids[i] = uint16(i + 1)
				}
				return ids
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &ParamReadRequest{ParamIDs: tt.paramIDs}
			data := EncodeParamReadRequest(req)

			expectedCount := len(tt.paramIDs)
			if expectedCount > 20 {
				expectedCount = 20
			}

			if data[0] != uint8(expectedCount) {
				t.Errorf("Expected count=%d, got %d", expectedCount, data[0])
			}

			t.Logf("✅ Encoded %d param IDs: %d bytes", expectedCount, len(data))
		})
	}
}

func TestParamReadResponse(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{
			name: "Single Param",
			data: []byte{
				0x01,       // count = 1
				0x00, 0x01, // param_id = 1
				0x04,                   // length = 4
				0x00, 0x00, 0x00, 0x0A, // value = 10
			},
		},
		{
			name: "Multiple Params",
			data: []byte{
				0x02,       // count = 2
				0x00, 0x01, // param_id = 1
				0x02,       // length = 2
				0x00, 0x64, // value = 100
				0x00, 0x02, // param_id = 2
				0x04,                   // length = 4
				0x00, 0x00, 0x03, 0xE8, // value = 1000
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := ParseParamReadResponse(tt.data)
			if err != nil {
				t.Fatalf("Parse failed: %v", err)
			}

			if len(resp.Params) != int(tt.data[0]) {
				t.Errorf("Expected %d params, got %d", tt.data[0], len(resp.Params))
			}

			t.Logf("✅ Parsed %d params", len(resp.Params))
			for i, param := range resp.Params {
				t.Logf("   Param %d: ID=0x%04X, Length=%d, Value=%v",
					i+1, param.ParamID, param.Length, param.Value)
			}
		})
	}
}

func TestParamWriteRequest(t *testing.T) {
	tests := []struct {
		name   string
		params []ParamValue
	}{
		{
			name: "Single Param",
			params: []ParamValue{
				{ParamID: 0x0001, Value: []byte{0x00, 0x64}},
			},
		},
		{
			name: "Multiple Params",
			params: []ParamValue{
				{ParamID: 0x0001, Value: []byte{0x00, 0x64}},
				{ParamID: 0x0002, Value: []byte{0x00, 0x00, 0x03, 0xE8}},
				{ParamID: 0x0010, Value: []byte{0x01}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &ParamWriteRequest{Params: tt.params}
			data := EncodeParamWriteRequest(req)

			if data[0] != uint8(len(tt.params)) {
				t.Errorf("Expected count=%d, got %d", len(tt.params), data[0])
			}

			t.Logf("✅ Encoded %d params: %d bytes", len(tt.params), len(data))
		})
	}
}

func TestParamWriteResponse(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{
			name: "All Success",
			data: []byte{
				0x02,       // count = 2
				0x00, 0x01, // param_id = 1
				0x00,       // result = 0 (成功)
				0x00, 0x02, // param_id = 2
				0x00, // result = 0 (成功)
			},
		},
		{
			name: "Mixed Results",
			data: []byte{
				0x03,       // count = 3
				0x00, 0x01, // param_id = 1
				0x00,       // result = 0 (成功)
				0x00, 0x02, // param_id = 2
				0x02,       // result = 2 (参数不存在)
				0x00, 0x03, // param_id = 3
				0x03, // result = 3 (值无效)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := ParseParamWriteResponse(tt.data)
			if err != nil {
				t.Fatalf("Parse failed: %v", err)
			}

			if len(resp.Results) != int(tt.data[0]) {
				t.Errorf("Expected %d results, got %d", tt.data[0], len(resp.Results))
			}

			t.Logf("✅ Parsed %d results", len(resp.Results))
			for _, result := range resp.Results {
				t.Logf("   Param 0x%04X: %s",
					result.ParamID, GetParamWriteResultDescription(result.Result))
			}
		})
	}
}

func TestParamSync(t *testing.T) {
	t.Run("Encode Request", func(t *testing.T) {
		req := &ParamSyncRequest{SyncType: 0}
		data := EncodeParamSyncRequest(req)

		if len(data) != 1 {
			t.Errorf("Expected 1 byte, got %d", len(data))
		}

		if data[0] != 0 {
			t.Errorf("Expected sync_type=0, got %d", data[0])
		}

		t.Logf("✅ Encoded sync request: type=%d", req.SyncType)
	})

	t.Run("Parse Response", func(t *testing.T) {
		data := []byte{
			0x01, // result = 1 (同步中)
			0x32, // progress = 50%
		}

		resp, err := ParseParamSyncResponse(data)
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}

		if resp.Result != 1 {
			t.Errorf("Expected result=1, got %d", resp.Result)
		}

		if resp.Progress != 50 {
			t.Errorf("Expected progress=50, got %d", resp.Progress)
		}

		t.Logf("✅ Parsed sync response: %s, %d%%",
			GetParamSyncResultDescription(resp.Result), resp.Progress)
	})
}

func TestParamReset(t *testing.T) {
	t.Run("Factory Reset", func(t *testing.T) {
		req := &ParamResetRequest{ResetType: 0}
		data := EncodeParamResetRequest(req)

		if len(data) != 1 {
			t.Errorf("Expected 1 byte, got %d", len(data))
		}

		if data[0] != 0 {
			t.Errorf("Expected reset_type=0, got %d", data[0])
		}

		t.Logf("✅ Encoded factory reset request")
	})

	t.Run("Specific Params Reset", func(t *testing.T) {
		req := &ParamResetRequest{
			ResetType: 1,
			ParamIDs:  []uint16{0x0001, 0x0002, 0x0010},
		}
		data := EncodeParamResetRequest(req)

		if data[0] != 1 {
			t.Errorf("Expected reset_type=1, got %d", data[0])
		}

		if data[1] != 3 {
			t.Errorf("Expected count=3, got %d", data[1])
		}

		t.Logf("✅ Encoded specific params reset: %d params", len(req.ParamIDs))
	})

	t.Run("Parse Response", func(t *testing.T) {
		data := []byte{
			0x00,                    // result = 0 (成功)
			'R', 'e', 's', 'e', 't', // message = "Reset"
		}

		resp, err := ParseParamResetResponse(data)
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}

		if resp.Result != 0 {
			t.Errorf("Expected result=0, got %d", resp.Result)
		}

		if resp.Message != "Reset" {
			t.Errorf("Expected message='Reset', got '%s'", resp.Message)
		}

		t.Logf("✅ Parsed reset response: result=%d, message=%s", resp.Result, resp.Message)
	})
}

// TestParamManagement_E2E 端到端集成测试
func TestParamManagement_E2E(t *testing.T) {
	t.Run("Complete Param Lifecycle", func(t *testing.T) {
		// 1. 批量读取参数
		readReq := &ParamReadRequest{
			ParamIDs: []uint16{0x0001, 0x0002, 0x0010},
		}
		readData := EncodeParamReadRequest(readReq)
		t.Logf("Step 1: Read params request: %d bytes", len(readData))

		// 2. 模拟设备响应
		readRespData := []byte{
			0x03,       // count = 3
			0x00, 0x01, // param_id = 1
			0x02,       // length = 2
			0x00, 0x64, // value = 100
			0x00, 0x02, // param_id = 2
			0x04,                   // length = 4
			0x00, 0x00, 0x03, 0xE8, // value = 1000
			0x00, 0x10, // param_id = 16
			0x01, // length = 1
			0x01, // value = 1
		}

		readResp, err := ParseParamReadResponse(readRespData)
		if err != nil {
			t.Fatalf("Parse read response failed: %v", err)
		}
		t.Logf("Step 2: Received %d params", len(readResp.Params))

		// 3. 修改参数值
		writeReq := &ParamWriteRequest{
			Params: []ParamValue{
				{ParamID: 0x0001, Value: []byte{0x00, 0xC8}},             // 200
				{ParamID: 0x0002, Value: []byte{0x00, 0x00, 0x07, 0xD0}}, // 2000
			},
		}
		writeData := EncodeParamWriteRequest(writeReq)
		t.Logf("Step 3: Write params request: %d bytes", len(writeData))

		// 4. 模拟写入响应
		writeRespData := []byte{
			0x02,       // count = 2
			0x00, 0x01, // param_id = 1
			0x00,       // result = 0 (成功)
			0x00, 0x02, // param_id = 2
			0x00, // result = 0 (成功)
		}

		writeResp, err := ParseParamWriteResponse(writeRespData)
		if err != nil {
			t.Fatalf("Parse write response failed: %v", err)
		}

		successCount := 0
		for _, result := range writeResp.Results {
			if result.Result == 0 {
				successCount++
			}
		}
		t.Logf("Step 4: Write result: %d/%d success", successCount, len(writeResp.Results))

		if successCount != len(writeResp.Results) {
			t.Errorf("Expected all writes to succeed, got %d/%d", successCount, len(writeResp.Results))
		}

		t.Logf("✅ E2E Test passed: Complete param lifecycle")
	})
}
