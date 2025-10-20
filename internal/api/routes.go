package api

import (
	"github.com/gin-gonic/gin"
	"github.com/taoyao-code/iot-server/internal/api/middleware"
	"github.com/taoyao-code/iot-server/internal/session"
	pgstorage "github.com/taoyao-code/iot-server/internal/storage/pg"
	"go.uber.org/zap"
)

// RegisterReadOnlyRoutes 注册只读查询路由
// 重构: 使用独立Handler替代内联匿名函数
func RegisterReadOnlyRoutes(
	r *gin.Engine,
	repo *pgstorage.Repository,
	sess session.SessionManager,
	policy session.WeightedPolicy,
	authCfg middleware.AuthConfig,
	logger *zap.Logger,
) {
	if r == nil || repo == nil || sess == nil {
		return
	}

	// 创建Handler
	handler := NewReadOnlyHandler(repo, sess, policy, logger)

	// 快速就绪检查(无需认证)
	r.GET("/ready", handler.Ready)

	// API路由组(需要认证)
	api := r.Group("/api")
	if authCfg.Enabled {
		api.Use(middleware.APIKeyAuth(authCfg, logger))
		logger.Info("api authentication enabled", zap.Int("api_keys_count", len(authCfg.APIKeys)))
	} else {
		logger.Warn("api authentication disabled - only for development!")
	}

	// 设备管理
	api.GET("/devices", handler.ListDevices)
	api.GET("/devices/:phyId/ports", handler.ListDevicePorts)
	api.GET("/devices/:phyId/params", handler.GetDeviceParams)

	// 会话管理
	api.GET("/sessions/:phyId", handler.GetSessionStatus)

	// 订单管理
	api.GET("/orders/:id", handler.GetOrder)
	api.GET("/devices/:phyId/orders", handler.ListDeviceOrders)

	logger.Info("readonly routes registered", zap.Int("endpoints", 6))
}
