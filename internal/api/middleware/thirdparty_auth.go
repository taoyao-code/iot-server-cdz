package middleware

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/taoyao-code/iot-server/internal/thirdparty"
	"go.uber.org/zap"
)

// ThirdPartyAuth 第三方API Key认证中间件
func ThirdPartyAuth(apiKeys []string, logger *zap.Logger) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(apiKeys))
	for _, key := range apiKeys {
		if strings.TrimSpace(key) != "" {
			allowed[key] = struct{}{}
		}
	}

	return func(c *gin.Context) {
		apiKey := extractAPIKey(c, "X-Api-Key")
		if apiKey == "" {
			recordThirdpartyFailure(c, logger, "missing_key", "missing api key")
			return
		}
		if _, ok := allowed[apiKey]; !ok {
			recordThirdpartyFailure(c, logger, "invalid_key", "invalid api key")
			return
		}
		c.Set("api_key", apiKey)
		c.Next()
	}
}

func recordThirdpartyFailure(c *gin.Context, logger *zap.Logger, reason, message string) {
	logger.Warn("third party auth failed",
		zap.String("path", c.Request.URL.Path),
		zap.String("method", c.Request.Method),
	)
	thirdparty.RecordAPIAuthFailure(c.Request.URL.Path, reason)
	c.JSON(http.StatusUnauthorized, gin.H{
		"code":    http.StatusUnauthorized,
		"message": message,
	})
	c.Abort()
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
