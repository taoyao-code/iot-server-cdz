package api

import (
	"github.com/gin-gonic/gin"
	"github.com/taoyao-code/iot-server/internal/api/middleware"
	"github.com/taoyao-code/iot-server/internal/driverapi"
	"github.com/taoyao-code/iot-server/internal/metrics"
	"github.com/taoyao-code/iot-server/internal/session"
	"github.com/taoyao-code/iot-server/internal/storage"
	pgstorage "github.com/taoyao-code/iot-server/internal/storage/pg"
	"github.com/taoyao-code/iot-server/internal/thirdparty"
	"go.uber.org/zap"
)

// RegisterThirdPartyRoutes 注册第三方API路由
func RegisterThirdPartyRoutes(
	r *gin.Engine,
	repo *pgstorage.Repository,
	coreRepo storage.CoreRepo,
	sess session.SessionManager,
	commandSource driverapi.CommandSource,
	driverCore DriverCoreInterface,
	eventQueue *thirdparty.EventQueue,
	metrics *metrics.AppMetrics,
	authCfg middleware.AuthConfig,
	logger *zap.Logger,
) {
	// 创建处理器
	handler := NewThirdPartyHandler(repo, coreRepo, sess, commandSource, driverCore, eventQueue, metrics, logger)

	// 第三方API路由组
	// 使用第三方认证中间件（与内部API认证分开）
	api := r.Group("/api/v1/third")
	if authCfg.Enabled {
		api.Use(middleware.ThirdPartyAuth(authCfg.APIKeys, logger))
	}

	// 设备控制API
	api.GET("/devices", handler.ListDevices)                    // 查询设备列表
	api.GET("/devices/:device_id", handler.GetDevice)           // 查询设备状态
	api.POST("/devices/:device_id/charge", handler.StartCharge) // 启动充电
	api.POST("/devices/:device_id/stop", handler.StopCharge)    // 停止充电

	// 状态定义API
	api.GET("/status/definitions", handler.GetStatusDefinitions) // 获取状态定义

	// 组网管理API
	api.POST("/devices/:device_id/network/configure", handler.ConfigureNetwork) // 配置组网
}
