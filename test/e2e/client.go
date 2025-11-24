package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// APIClient E2E测试API客户端
type APIClient struct {
	config     *Config
	httpClient *http.Client
}

// NewAPIClient 创建API客户端
func NewAPIClient(cfg *Config) *APIClient {
	return &APIClient{
		config: cfg,
		httpClient: &http.Client{
			Timeout: cfg.RequestTimeout,
		},
	}
}

// APIError API错误
type APIError struct {
	StatusCode int
	Code       int
	Message    string
	RequestID  string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("API Error [%d]: %s (code=%d, request_id=%s)",
		e.StatusCode, e.Message, e.Code, e.RequestID)
}

// IsConflict 判断是否为冲突错误（409）
func (e *APIError) IsConflict() bool {
	return e.StatusCode == http.StatusConflict
}

// IsNotFound 判断是否为未找到错误（404）
func (e *APIError) IsNotFound() bool {
	return e.StatusCode == http.StatusNotFound
}

// doRequest 执行HTTP请求
func (c *APIClient) doRequest(ctx context.Context, method, path string, body interface{}, result interface{}) error {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	url := c.config.ServerURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", c.config.APIKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}

	// 解析标准响应
	var stdResp StandardResponse
	if err := json.Unmarshal(respBody, &stdResp); err != nil {
		return fmt.Errorf("unmarshal response: %w (body: %s)", err, string(respBody))
	}

	// 检查业务错误
	if stdResp.Code != 0 {
		return &APIError{
			StatusCode: resp.StatusCode,
			Code:       stdResp.Code,
			Message:    stdResp.Message,
			RequestID:  stdResp.RequestID,
		}
	}

	// 解析业务数据
	if result != nil && stdResp.Data != nil {
		dataBytes, err := json.Marshal(stdResp.Data)
		if err != nil {
			return fmt.Errorf("marshal data: %w", err)
		}
		if err := json.Unmarshal(dataBytes, result); err != nil {
			return fmt.Errorf("unmarshal data: %w", err)
		}
	}

	return nil
}

// GetDevice 获取设备信息
func (c *APIClient) GetDevice(ctx context.Context, deviceID string) (*DeviceInfo, error) {
	var device DeviceInfo
	path := fmt.Sprintf("/api/v1/third/devices/%s", deviceID)
	if err := c.doRequest(ctx, http.MethodGet, path, nil, &device); err != nil {
		return nil, err
	}
	return &device, nil
}

// StartCharge 启动充电
func (c *APIClient) StartCharge(ctx context.Context, deviceID string, req *StartChargeRequest) (*ChargeResponse, error) {
	var resp ChargeResponse
	path := fmt.Sprintf("/api/v1/third/devices/%s/charge", deviceID)
	if err := c.doRequest(ctx, http.MethodPost, path, req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// StopCharge 停止充电
func (c *APIClient) StopCharge(ctx context.Context, deviceID string, portNo int, socketUID string) error {
	path := fmt.Sprintf("/api/v1/third/devices/%s/stop", deviceID)
	req := &StopChargeRequest{PortNo: portNo, SocketUID: socketUID}
	return c.doRequest(ctx, http.MethodPost, path, req, nil)
}

// GetOrder 获取订单信息
func (c *APIClient) GetOrder(ctx context.Context, orderNo string) (*OrderInfo, error) {
	var order OrderInfo
	path := fmt.Sprintf("/api/v1/third/orders/%s", orderNo)
	if err := c.doRequest(ctx, http.MethodGet, path, nil, &order); err != nil {
		return nil, err
	}
	return &order, nil
}

// WaitForOrderStatus 等待订单达到指定状态
func (c *APIClient) WaitForOrderStatus(ctx context.Context, orderNo string, expectedStatus OrderStatus, timeout time.Duration) (*OrderInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// 超时，返回最后一次查询结果
			order, _ := c.GetOrder(context.Background(), orderNo)
			if order != nil {
				return nil, fmt.Errorf("timeout waiting for status %s, current status: %s", expectedStatus, order.Status)
			}
			return nil, fmt.Errorf("timeout waiting for status %s", expectedStatus)

		case <-ticker.C:
			order, err := c.GetOrder(ctx, orderNo)
			if err != nil {
				continue // 忽略查询错误，继续等待
			}
			if order.Status == expectedStatus {
				return order, nil
			}
			// 如果订单已经失败或取消，提前返回
			if order.Status == OrderStatusFailed || order.Status == OrderStatusCancelled {
				return order, fmt.Errorf("order status is %s, expected %s", order.Status, expectedStatus)
			}
		}
	}
}

// WaitForDeviceOnline 等待设备上线
func (c *APIClient) WaitForDeviceOnline(ctx context.Context, deviceID string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for device online")

		case <-ticker.C:
			device, err := c.GetDevice(ctx, deviceID)
			if err != nil {
				continue
			}
			if device.Online {
				return nil
			}
		}
	}
}

// RetryOnConflict 在端口冲突时重试（先停止，再重试）
func (c *APIClient) RetryOnConflict(ctx context.Context, deviceID string, req *StartChargeRequest) (*ChargeResponse, error) {
	resp, err := c.StartCharge(ctx, deviceID, req)
	if err == nil {
		return resp, nil
	}

	// 检查是否为冲突错误
	apiErr, ok := err.(*APIError)
	if !ok || !apiErr.IsConflict() {
		return nil, err
	}

	// 停止当前端口的充电
	if stopErr := c.StopCharge(ctx, deviceID, req.PortNo, req.SocketUID); stopErr != nil {
		return nil, fmt.Errorf("stop charge failed: %w (original error: %v)", stopErr, err)
	}

	// 等待一小段时间
	time.Sleep(2 * time.Second)

	// 重试启动充电
	return c.StartCharge(ctx, deviceID, req)
}
