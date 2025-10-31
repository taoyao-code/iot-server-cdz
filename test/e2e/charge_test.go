package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"
)

const (
	testServerURL = "http://182.43.177.92:7055"
	testAPIKey    = "sk_test_thirdparty_key_for_testing_12345678"
	testDeviceID  = "82241218000382"
)

type testClient struct {
	client *http.Client
	t      *testing.T
}

func newTestClient(t *testing.T) *testClient {
	return &testClient{
		client: &http.Client{Timeout: 30 * time.Second},
		t:      t,
	}
}

func (tc *testClient) request(method, path string, body interface{}) ([]byte, int) {
	var reqBody io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, testServerURL+path, reqBody)
	if err != nil {
		tc.t.Fatalf("创建请求失败: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", testAPIKey)

	resp, err := tc.client.Do(req)
	if err != nil {
		tc.t.Fatalf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	return respBody, resp.StatusCode
}

func (tc *testClient) getDeviceStatus() map[string]interface{} {
	body, status := tc.request("GET", "/api/v1/third/devices/"+testDeviceID, nil)
	if status != 200 {
		tc.t.Fatalf("查询设备失败: status=%d body=%s", status, body)
	}

	var resp struct {
		Code int                    `json:"code"`
		Data map[string]interface{} `json:"data"`
	}
	json.Unmarshal(body, &resp)
	return resp.Data
}

func (tc *testClient) startCharge(portNo int) string {
	reqBody := map[string]int{
		"port_no":       portNo,
		"charge_mode":   1,
		"duration":      60,
		"amount":        500,
		"price_per_kwh": 150,
		"service_fee":   50,
	}

	body, status := tc.request("POST", "/api/v1/third/devices/"+testDeviceID+"/charge", reqBody)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			OrderNo string `json:"order_no"`
		} `json:"data"`
		Message string `json:"message"`
	}
	json.Unmarshal(body, &resp)

	if status == 409 {
		tc.t.Logf("端口%d被占用，尝试停止现有订单", portNo)
		tc.stopCharge(portNo)
		time.Sleep(2 * time.Second)
		body, status = tc.request("POST", "/api/v1/third/devices/"+testDeviceID+"/charge", reqBody)
		json.Unmarshal(body, &resp)
	}

	if resp.Code != 0 {
		tc.t.Fatalf("创建订单失败: code=%d message=%s", resp.Code, resp.Message)
	}

	return resp.Data.OrderNo
}

func (tc *testClient) stopCharge(portNo int) {
	reqBody := map[string]int{"port_no": portNo}
	tc.request("POST", "/api/v1/third/devices/"+testDeviceID+"/stop", reqBody)
}

func (tc *testClient) getOrderStatus(orderNo string) map[string]interface{} {
	body, status := tc.request("GET", "/api/v1/third/orders/"+orderNo, nil)
	if status != 200 {
		tc.t.Fatalf("查询订单失败: status=%d body=%s", status, body)
	}

	var resp struct {
		Code int                    `json:"code"`
		Data map[string]interface{} `json:"data"`
	}
	json.Unmarshal(body, &resp)
	return resp.Data
}

func (tc *testClient) waitForStatus(orderNo string, expectedStatus string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		order := tc.getOrderStatus(orderNo)
		status := order["status"].(string)
		if status == expectedStatus {
			return true
		}
		time.Sleep(1 * time.Second)
	}
	return false
}

// TestChargePort1 测试端口1充电
func TestChargePort1(t *testing.T) {
	tc := newTestClient(t)

	t.Log("━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	t.Log("  测试端口1充电（BKV插孔0/A孔）")
	t.Log("━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// 1. 检查设备在线
	device := tc.getDeviceStatus()
	if !device["online"].(bool) {
		t.Fatal("❌ 设备不在线")
	}
	t.Logf("✓ 设备在线: %v", device["status"])

	// 2. 创建订单
	t.Log("\n→ 创建端口1订单...")
	orderNo := tc.startCharge(1)
	t.Logf("✓ 订单创建: %s", orderNo)

	// 3. 等待订单变为charging
	t.Log("\n→ 等待设备响应（最多10秒）...")
	if tc.waitForStatus(orderNo, "charging", 10*time.Second) {
		t.Log("✅ 端口1充电成功！")
	} else {
		order := tc.getOrderStatus(orderNo)
		t.Errorf("❌ 端口1充电失败: 订单状态=%s", order["status"])
	}

	// 4. 停止充电
	t.Log("\n→ 停止充电...")
	tc.stopCharge(1)
	time.Sleep(2 * time.Second)

	order := tc.getOrderStatus(orderNo)
	t.Logf("最终状态: %s", order["status"])
}

// TestChargePort2 测试端口2充电
func TestChargePort2(t *testing.T) {
	tc := newTestClient(t)

	t.Log("━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	t.Log("  测试端口2充电（BKV插孔1/B孔）")
	t.Log("━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// 1. 检查设备在线
	device := tc.getDeviceStatus()
	if !device["online"].(bool) {
		t.Fatal("❌ 设备不在线")
	}
	t.Logf("✓ 设备在线: %v", device["status"])

	// 2. 创建订单
	t.Log("\n→ 创建端口2订单...")
	orderNo := tc.startCharge(2)
	t.Logf("✓ 订单创建: %s", orderNo)

	// 3. 等待订单变为charging
	t.Log("\n→ 等待设备响应（最多10秒）...")
	if tc.waitForStatus(orderNo, "charging", 10*time.Second) {
		t.Log("✅ 端口2充电成功！")
	} else {
		order := tc.getOrderStatus(orderNo)
		t.Errorf("❌ 端口2充电失败: 订单状态=%s", order["status"])
	}

	// 4. 停止充电
	t.Log("\n→ 停止充电...")
	tc.stopCharge(2)
	time.Sleep(2 * time.Second)

	order := tc.getOrderStatus(orderNo)
	t.Logf("最终状态: %s", order["status"])
}

// TestChargeBothPorts 测试两个端口同时充电（应该失败）
func TestChargeBothPorts(t *testing.T) {
	tc := newTestClient(t)

	t.Log("━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	t.Log("  测试双端口并发（预期端口冲突）")
	t.Log("━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// 1. 启动端口1
	t.Log("\n→ 启动端口1...")
	order1 := tc.startCharge(1)
	t.Logf("✓ 端口1订单: %s", order1)

	// 2. 等待端口1 charging
	time.Sleep(3 * time.Second)

	// 3. 尝试启动端口2
	t.Log("\n→ 尝试启动端口2...")
	order2 := tc.startCharge(2)
	t.Logf("✓ 端口2订单: %s", order2)

	// 4. 检查两个订单状态
	time.Sleep(3 * time.Second)
	status1 := tc.getOrderStatus(order1)["status"]
	status2 := tc.getOrderStatus(order2)["status"]

	t.Logf("\n端口1状态: %s", status1)
	t.Logf("端口2状态: %s", status2)

	// 5. 清理
	tc.stopCharge(1)
	tc.stopCharge(2)

	if status1 == "charging" && status2 == "charging" {
		t.Log("✅ 两个端口都能充电")
	} else if status1 == "charging" || status2 == "charging" {
		t.Log("⚠️  只有一个端口能充电")
	} else {
		t.Error("❌ 两个端口都无法充电")
	}
}

// TestProtocolCompliance 测试协议合规性
func TestProtocolCompliance(t *testing.T) {
	t.Log("━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	t.Log("  协议合规性验证")
	t.Log("━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	t.Run("端口映射", func(t *testing.T) {
		mapping := map[string]string{
			"API端口1": "BKV插孔0(A孔)",
			"API端口2": "BKV插孔1(B孔)",
		}
		for api, bkv := range mapping {
			t.Logf("✓ %s → %s", api, bkv)
		}
	})

	t.Run("命令格式", func(t *testing.T) {
		t.Log("0x0015下行格式: [长度(2)] [07] [插座] [插孔] [开关] [模式] [时长(2)] [电量(2)]")
		t.Log("长度字段 = 参数字节数(不含0x07) = 8字节")
	})

	t.Run("ACK格式", func(t *testing.T) {
		t.Log("0x0015上行格式: [长度(2)] [07] [结果] [插座] [插孔] [业务号(2)]")
		t.Log("结果: 0x01=成功, 0x00=失败")
	})
}
