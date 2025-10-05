package health

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// RegisterHTTPRoutes 注册健康检查HTTP路由
func RegisterHTTPRoutes(r *gin.Engine, aggregator *Aggregator) {
	// 1. Readiness探针（K8s使用）
	// GET /health/ready
	r.GET("/health/ready", func(c *gin.Context) {
		ctx := c.Request.Context()

		if !aggregator.Ready(ctx) {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "unhealthy",
				"ready":  false,
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
			"ready":  true,
		})
	})

	// 2. Liveness探针（K8s使用）
	// GET /health/live
	r.GET("/health/live", func(c *gin.Context) {
		if !aggregator.Alive() {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"alive": false,
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"alive": true,
		})
	})

	// 3. 详细健康检查
	// GET /health
	r.GET("/health", func(c *gin.Context) {
		ctx := c.Request.Context()

		results := aggregator.CheckAll(ctx)
		overall := aggregator.OverallStatus(ctx)

		// 根据状态设置HTTP状态码
		code := http.StatusOK
		if overall == StatusUnhealthy {
			code = http.StatusServiceUnavailable
		}
		// Degraded状态仍返回200，表示可以服务

		c.JSON(code, gin.H{
			"status":    overall,
			"timestamp": time.Now(),
			"checks":    results,
		})
	})
}
