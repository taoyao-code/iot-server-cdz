package thirdparty

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// SignHMAC 生成 HMAC-SHA256 签名（hex）
func SignHMAC(secret string, canonical string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(canonical))
	return hex.EncodeToString(mac.Sum(nil))
}
