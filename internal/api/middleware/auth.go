// Package middleware 提供HTTP中间件
// P0修复: API认证中间件
package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// AuthConfig API认证配置
type AuthConfig struct {
	APIKeys []string `json:"api_keys"`
	Enabled bool     `json:"enabled"`
}

// APIKeyAuth API Key认证中间件
// P0修复: 为HTTP API添加认证保护，防止未授权访问
//
// 使用方式:
//  1. Header: X-API-Key: sk_live_xxxx
//  2. Header: Authorization: Bearer sk_live_xxxx
//
// 审计日志: 记录所有认证请求和失败尝试
func APIKeyAuth(cfg AuthConfig, logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 如果未启用认证，直接放行（开发环境）
		if !cfg.Enabled {
			c.Next()
			return
		}

		// 从Header获取API Key
		apiKey := c.GetHeader("X-API-Key")
		if apiKey == "" {
			// 兼容Bearer Token格式
			auth := c.GetHeader("Authorization")
			if strings.HasPrefix(auth, "Bearer ") {
				apiKey = strings.TrimPrefix(auth, "Bearer ")
			}
		}

		// 验证API Key是否存在
		if apiKey == "" {
			logger.Warn("api auth: missing api key",
				zap.String("path", c.Request.URL.Path),
				zap.String("method", c.Request.Method),
				zap.String("remote_addr", c.ClientIP()),
				zap.String("user_agent", c.Request.UserAgent()),
			)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error":   "unauthorized",
				"message": "请在Header中提供 X-API-Key 或 Authorization: Bearer <token>",
			})
			return
		}

		// 检查API Key是否有效
		valid := false
		for _, k := range cfg.APIKeys {
			if k == apiKey {
				valid = true
				break
			}
		}

		if !valid {
			logger.Warn("api auth: invalid api key",
				zap.String("path", c.Request.URL.Path),
				zap.String("method", c.Request.Method),
				zap.String("remote_addr", c.ClientIP()),
				zap.String("api_key_prefix", maskAPIKey(apiKey)),
			)
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error":   "forbidden",
				"message": "无效的API Key",
			})
			return
		}

		// 记录审计日志（成功）
		logger.Info("api auth: authenticated",
			zap.String("path", c.Request.URL.Path),
			zap.String("method", c.Request.Method),
			zap.String("remote_addr", c.ClientIP()),
			zap.String("api_key_prefix", maskAPIKey(apiKey)),
		)

		// 设置上下文信息
		c.Set("authenticated", true)
		c.Set("api_key", apiKey)

		c.Next()
	}
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
		// TODO: 实现限流逻辑
		c.Next()
	}
}

// InternalAuth 内部测试控制台认证中间件
// 比APIKeyAuth更严格，用于保护内部测试接口
func InternalAuth(apiKeys []string, logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 从Header获取API Key
		apiKey := c.GetHeader("X-Internal-API-Key")
		if apiKey == "" {
			// 兼容通用API Key
			apiKey = c.GetHeader("X-API-Key")
		}
		if apiKey == "" {
			// 兼容Bearer Token格式
			auth := c.GetHeader("Authorization")
			if strings.HasPrefix(auth, "Bearer ") {
				apiKey = strings.TrimPrefix(auth, "Bearer ")
			}
		}

		// 验证API Key是否存在
		if apiKey == "" {
			logger.Warn("internal auth: missing api key",
				zap.String("path", c.Request.URL.Path),
				zap.String("method", c.Request.Method),
				zap.String("remote_addr", c.ClientIP()),
				zap.String("user_agent", c.Request.UserAgent()),
			)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error":   "unauthorized",
				"message": "内部接口需要认证：请在Header中提供 X-Internal-API-Key",
			})
			return
		}

		// 检查API Key是否有效
		valid := false
		for _, k := range apiKeys {
			if k == apiKey {
				valid = true
				break
			}
		}

		if !valid {
			logger.Warn("internal auth: invalid api key",
				zap.String("path", c.Request.URL.Path),
				zap.String("method", c.Request.Method),
				zap.String("remote_addr", c.ClientIP()),
				zap.String("api_key_prefix", maskAPIKey(apiKey)),
			)
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error":   "forbidden",
				"message": "无效的内部API Key",
			})
			return
		}

		// 记录审计日志（成功）
		logger.Info("internal auth: authenticated",
			zap.String("path", c.Request.URL.Path),
			zap.String("method", c.Request.Method),
			zap.String("remote_addr", c.ClientIP()),
			zap.String("api_key_prefix", maskAPIKey(apiKey)),
		)

		// 设置上下文信息
		c.Set("authenticated", true)
		c.Set("internal_access", true)
		c.Set("api_key", apiKey)

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
