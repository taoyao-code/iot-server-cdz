package api

import (
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"github.com/taoyao-code/iot-server/internal/api/middleware"
	"github.com/taoyao-code/iot-server/internal/session"
	pgstorage "github.com/taoyao-code/iot-server/internal/storage/pg"
	"go.uber.org/zap"

	_ "github.com/taoyao-code/iot-server/api/swagger" // swagger docs
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

	// Swagger 文档(无需认证)
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

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
	api.GET("/devices/:device_id/ports", handler.ListDevicePorts)
	api.GET("/devices/:device_id/params", handler.GetDeviceParams)

	// 会话管理
	api.GET("/sessions/:device_id", handler.GetSessionStatus)

	// 订单管理
	api.GET("/orders/:order_id", handler.GetOrder)
	api.GET("/devices/:device_id/orders", handler.ListDeviceOrders)

	logger.Info("readonly routes registered", zap.Int("endpoints", 6))
}
