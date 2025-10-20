package api

import (
	"github.com/gin-gonic/gin"
	pgstorage "github.com/taoyao-code/iot-server/internal/storage/pg"
	"go.uber.org/zap"
)

// RegisterOTARoutes 注册OTA升级路由
// 重构: 使用独立Handler
func RegisterOTARoutes(r *gin.Engine, repo *pgstorage.Repository, logger *zap.Logger) {
	handler := NewOTAHandler(repo, logger)

	ota := r.Group("/api/devices/:device_id/ota")
	{
		ota.POST("", handler.CreateOTATask)
		ota.GET("/:task_id", handler.GetOTATask)
		ota.GET("", handler.ListOTATasks)
	}

	logger.Info("ota routes registered", zap.Int("endpoints", 3))
}
