package middleware

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/taoyao-code/iot-server/internal/thirdparty"
	"go.uber.org/zap"
)

// ThirdPartyAuth 第三方API Key认证中间件
func ThirdPartyAuth(apiKeys []string, logger *zap.Logger) gin.HandlerFunc {
	// 构建API Key映射表
	keyMap := make(map[string]bool)
	for _, key := range apiKeys {
		if key != "" {
			keyMap[key] = true
		}
	}

	return func(c *gin.Context) {
		// 获取API Key from Header（支持两种格式以保持兼容性）
		apiKey := c.GetHeader("X-Api-Key")
		if apiKey == "" {
			// 兼容标准格式（大写）
			apiKey = c.GetHeader("X-API-Key")
		}

		if apiKey == "" {
			logger.Warn("third party auth failed: missing api key",
				zap.String("path", c.Request.URL.Path),
				zap.String("method", c.Request.Method))

			thirdparty.RecordAPIAuthFailure(c.Request.URL.Path, "missing_key")

			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    401,
				"message": "missing api key",
			})
			c.Abort()
			return
		}

		// 验证API Key
		if !keyMap[apiKey] {
			logger.Warn("third party auth failed: invalid api key",
				zap.String("path", c.Request.URL.Path),
				zap.String("api_key_prefix", maskAPIKey(apiKey)))

			thirdparty.RecordAPIAuthFailure(c.Request.URL.Path, "invalid_key")

			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    401,
				"message": "invalid api key",
			})
			c.Abort()
			return
		}

		// TODO: 可选的HMAC签名验证
		// signature := c.GetHeader("X-Signature")
		// if signature != "" {
		//     验证HMAC签名
		// }

		// 认证成功，继续处理
		c.Set("api_key", apiKey)
		c.Next()
	}
}

// RequestTracing 请求追踪中间件（添加request_id）
func RequestTracing() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 生成或获取request_id
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			requestID = generateRequestID()
		}

		c.Set("request_id", requestID)
		c.Header("X-Request-ID", requestID)

		c.Next()
	}
}

// generateRequestID 生成请求ID
func generateRequestID() string {
	// 简单实现：使用纳秒时间戳，避免引入新依赖
	return fmt.Sprintf("req-%d", time.Now().UnixNano())
}
