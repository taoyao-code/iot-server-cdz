package thirdparty

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Event 与 openapi components.schemas.ThirdpartyEvent 对应
type Event struct {
	Event       string         `json:"event"`
	DevicePhyID string         `json:"devicePhyId"`
	Timestamp   int64          `json:"timestamp"`
	Nonce       string         `json:"nonce"`
	Data        map[string]any `json:"data"`
}

type Pusher struct {
	Client  *http.Client
	APIKey  string
	Secret  string
	Retries int
	Backoff []time.Duration
	// 去重占位：可注入外部去重存储（如 redis）。当前仅作为接口预留
	Deduper func(key string, ttl time.Duration) bool
}

func NewPusher(client *http.Client, apiKey, secret string) *Pusher {
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	return &Pusher{
		Client:  client,
		APIKey:  apiKey,
		Secret:  secret,
		Retries: 5,
		Backoff: []time.Duration{100 * time.Millisecond, 200 * time.Millisecond, 500 * time.Millisecond, time.Second, 2 * time.Second},
	}
}

// buildCanonical 构建 canonical string: method\npath\ntimestamp\nnonce\nbodySha256Hex
func buildCanonical(method, path string, ts int64, nonce, bodyHex string) string {
	return fmt.Sprintf("%s\n%s\n%d\n%s\n%s", strings.ToUpper(method), path, ts, nonce, bodyHex)
}

// hashHex 计算 sha256(body) 的 hex 小写
func hashHex(body []byte) string {
	h := sha256.Sum256(body)
	return hex.EncodeToString(h[:])
}

// SendJSON 发送 JSON 事件，自动添加签名头
func (p *Pusher) SendJSON(ctx context.Context, endpoint string, payload any) (int, []byte, error) {
	if p == nil || p.Client == nil {
		return 0, nil, errors.New("nil pusher")
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return 0, nil, err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return 0, nil, err
	}
	path := u.Path
	ts := time.Now().Unix()
	nonce := fmt.Sprintf("%08x", rand.Uint32())
	bodyHex := hashHex(body)
	canonical := buildCanonical(http.MethodPost, path, ts, nonce, bodyHex)
	sig := SignHMAC(p.Secret, canonical)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(body)))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", p.APIKey)
	req.Header.Set("X-Signature", sig)
	req.Header.Set("X-Timestamp", fmt.Sprintf("%d", ts))
	req.Header.Set("X-Nonce", nonce)

	// 简单重试（5xx/网络错误）
	var respBody []byte
	var code int
	var lastErr error
	for attempt := 0; attempt <= p.Retries; attempt++ {
		resp, err := p.Client.Do(req)
		if err != nil {
			lastErr = err
		} else {
			code = resp.StatusCode
			rb, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			respBody = rb
			if code >= 200 && code < 300 {
				return code, respBody, nil
			}
			// 非2xx：仅对5xx重试
			if code < 500 {
				return code, respBody, nil
			}
		}
		if attempt == p.Retries {
			break
		}
		backoff := p.Backoff[min(attempt, len(p.Backoff)-1)]
		select {
		case <-ctx.Done():
			return 0, nil, ctx.Err()
		case <-time.After(backoff):
		}
	}
	if lastErr != nil {
		return 0, nil, lastErr
	}
	return code, respBody, fmt.Errorf("http %d", code)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
