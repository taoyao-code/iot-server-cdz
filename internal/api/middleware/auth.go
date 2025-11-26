package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// AuthConfig API认证配置
type AuthConfig struct {
	APIKeys  []string
	Enabled  bool
	Header   string
	Internal bool
}

// APIKeyAuth 提供公共 API Key 认证
func APIKeyAuth(cfg AuthConfig, logger *zap.Logger) gin.HandlerFunc {
	return buildAuthMiddleware(cfg, logger)
}

// InternalAuth 用于内部测试接口
func InternalAuth(apiKeys []string, logger *zap.Logger) gin.HandlerFunc {
	return buildAuthMiddleware(AuthConfig{
		APIKeys:  apiKeys,
		Enabled:  true,
		Header:   "X-Internal-API-Key",
		Internal: true,
	}, logger)
}

func buildAuthMiddleware(cfg AuthConfig, logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !cfg.Enabled {
			c.Next()
			return
		}
		apiKey := extractAPIKey(c, cfg.Header)
		if apiKey == "" {
			handleAuthFailure(logger, c, http.StatusUnauthorized, cfg.Internal)
			return
		}
		if !isValidKey(apiKey, cfg.APIKeys) {
			handleAuthFailure(logger, c, http.StatusForbidden, cfg.Internal)
			return
		}
		logger.Info("api auth: authenticated",
			zap.String("path", c.Request.URL.Path),
			zap.String("method", c.Request.Method),
			zap.String("remote_addr", c.ClientIP()),
			zap.String("api_key_prefix", maskAPIKey(apiKey)),
		)
		c.Set("authenticated", true)
		c.Set("api_key", apiKey)
		if cfg.Internal {
			c.Set("internal_access", true)
		}
		c.Next()
	}
}

func extractAPIKey(c *gin.Context, customHeader string) string {
	headers := []string{customHeader, "X-API-Key"}
	for _, h := range headers {
		if strings.TrimSpace(h) == "" {
			continue
		}
		if key := c.GetHeader(h); key != "" {
			return key
		}
	}
	auth := c.GetHeader("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return ""
}

func isValidKey(apiKey string, keys []string) bool {
	for _, k := range keys {
		if k == apiKey {
			return true
		}
	}
	return false
}

func handleAuthFailure(logger *zap.Logger, c *gin.Context, status int, internal bool) {
	msg := "请在Header中提供 X-API-Key 或 Authorization: Bearer <token>"
	if internal {
		msg = "内部接口需要认证：请在Header中提供 X-Internal-API-Key"
	}
	logger.Warn("api auth: failed",
		zap.String("path", c.Request.URL.Path),
		zap.String("method", c.Request.Method),
		zap.String("remote_addr", c.ClientIP()),
	)
	c.AbortWithStatusJSON(status, gin.H{
		"error":   http.StatusText(status),
		"message": msg,
	})
}

// maskAPIKey 脱敏API Key（仅显示前4位和后4位）
func maskAPIKey(key string) string {
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + "****" + key[len(key)-4:]
}

// RateLimitConfig 限流配置（未来扩展）
type RateLimitConfig struct {
	Enabled        bool
	RequestsPerMin int
	BurstSize      int
}

// RateLimit 限流中间件（占位，未来实现）
func RateLimit(cfg RateLimitConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
	}
}

// CORS CORS中间件（如果需要）
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-API-Key, X-Internal-API-Key, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
