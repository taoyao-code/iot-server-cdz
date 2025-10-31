package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const (
	ServerURL = "http://182.43.177.92:7055"
	APIKey    = "sk_test_thirdparty_key_for_testing_12345678"
	DeviceID  = "82241218000382"
)

type DeviceInfo struct {
	Online      bool   `json:"online"`
	Status      string `json:"status"`
	LastSeenAt  int64  `json:"last_seen_at"`
	ActiveOrder *struct {
		OrderNo string `json:"order_no"`
		PortNo  int    `json:"port_no"`
	} `json:"active_order"`
}

type StandardResponse struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type OrderInfo struct {
	OrderNo   string  `json:"order_no"`
	PortNo    int     `json:"port_no"`
	Status    string  `json:"status"`
	Amount    float64 `json:"amount"`
	StartTime *int64  `json:"start_time"`
	CreatedAt int64   `json:"created_at"`
}

var client = &http.Client{Timeout: 30 * time.Second}

func callAPI(method, path string, payload interface{}) ([]byte, error) {
	var body io.Reader
	if payload != nil {
		data, _ := json.Marshal(payload)
		body = bytes.NewReader(data)
	}
	req, _ := http.NewRequest(method, ServerURL+path, body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", APIKey)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func main() {
	port := 2
	if len(os.Args) > 1 {
		fmt.Sscanf(os.Args[1], "%d", &port)
	}

	fmt.Printf("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("  设备状态诊断 - 端口 %d\n", port)
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	// 检查设备状态
	body, _ := callAPI("GET", "/api/v1/third/devices/"+DeviceID, nil)
	var devResp StandardResponse
	json.Unmarshal(body, &devResp)
	var dev DeviceInfo
	json.Unmarshal(devResp.Data, &dev)

	fmt.Printf("设备状态: %s\n", dev.Status)
	fmt.Printf("在线: %v\n", dev.Online)
	if dev.ActiveOrder != nil {
		fmt.Printf("活跃订单: %s (端口%d)\n", dev.ActiveOrder.OrderNo, dev.ActiveOrder.PortNo)
	}
	fmt.Println()

	// 创建订单
	fmt.Printf("→ 创建端口%d订单...\n", port)
	req := map[string]int{
		"port_no":       port,
		"charge_mode":   1,
		"duration":      60,
		"amount":        500,
		"price_per_kwh": 150,
		"service_fee":   50,
	}
	body, _ = callAPI("POST", "/api/v1/third/devices/"+DeviceID+"/charge", req)
	var chResp StandardResponse
	json.Unmarshal(body, &chResp)

	if chResp.Code == 409 {
		fmt.Printf("⚠️  端口被占用\n")
		var conf struct {
			CurrentOrder string `json:"current_order"`
		}
		json.Unmarshal(chResp.Data, &conf)
		if conf.CurrentOrder != "" {
			fmt.Printf("→ 停止订单 %s...\n", conf.CurrentOrder)
			callAPI("POST", "/api/v1/third/devices/"+DeviceID+"/stop", map[string]int{"port_no": port})
			time.Sleep(2 * time.Second)
			body, _ = callAPI("POST", "/api/v1/third/devices/"+DeviceID+"/charge", req)
			json.Unmarshal(body, &chResp)
		}
	}

	var chargeData struct {
		OrderNo string `json:"order_no"`
	}
	json.Unmarshal(chResp.Data, &chargeData)
	fmt.Printf("✓ 订单: %s\n\n", chargeData.OrderNo)

	// 等待3秒后查询状态
	time.Sleep(3 * time.Second)

	body, _ = callAPI("GET", "/api/v1/third/orders/"+chargeData.OrderNo, nil)
	var ordResp StandardResponse
	json.Unmarshal(body, &ordResp)
	var order OrderInfo
	json.Unmarshal(ordResp.Data, &order)

	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("订单号: %s\n", order.OrderNo)
	fmt.Printf("端口: %d\n", order.PortNo)
	fmt.Printf("状态: %s\n", order.Status)
	if order.StartTime != nil {
		fmt.Printf("开始时间: %s\n", time.Unix(*order.StartTime, 0).Format("15:04:05"))
	}
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	if order.Status == "charging" {
		fmt.Printf("✓ 端口%d: 充电正常\n\n", port)
	} else if order.Status == "pending" {
		fmt.Printf("⚠️  端口%d: 设备未响应\n\n", port)
	}
}
