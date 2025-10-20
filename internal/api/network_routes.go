package api

import (
	"github.com/gin-gonic/gin"
	pgstorage "github.com/taoyao-code/iot-server/internal/storage/pg"
	"go.uber.org/zap"
)

// RegisterNetworkRoutes 注册组网管理路由
// 重构: 使用独立Handler
func RegisterNetworkRoutes(r *gin.Engine, repo *pgstorage.Repository, logger *zap.Logger) {
	handler := NewNetworkHandler(repo, logger)

	network := r.Group("/api/gateway/:gateway_id/sockets")
	{
		network.GET("", handler.ListGatewaySockets)
		network.GET("/:socket_no", handler.GetGatewaySocket)
		network.DELETE("/:socket_no", handler.DeleteGatewaySocket)
	}

	logger.Info("network routes registered", zap.Int("endpoints", 3))
}
