package api

import (
	"github.com/gin-gonic/gin"
	"github.com/taoyao-code/iot-server/internal/api/middleware"
	"github.com/taoyao-code/iot-server/internal/session"
	pgstorage "github.com/taoyao-code/iot-server/internal/storage/pg"
	redisstorage "github.com/taoyao-code/iot-server/internal/storage/redis"
	"github.com/taoyao-code/iot-server/internal/thirdparty"
	"go.uber.org/zap"
)

// RegisterThirdPartyRoutes 注册第三方API路由
func RegisterThirdPartyRoutes(
	r *gin.Engine,
	repo *pgstorage.Repository,
	sess session.SessionManager,
	outboundQ *redisstorage.OutboundQueue,
	eventQueue *thirdparty.EventQueue,
	authCfg middleware.AuthConfig,
	logger *zap.Logger,
) {
	// 创建处理器
	handler := NewThirdPartyHandler(repo, sess, outboundQ, eventQueue, logger)

	// 第三方API路由组
	// 使用第三方认证中间件（与内部API认证分开）
	api := r.Group("/api/v1/third")
	if authCfg.Enabled {
		api.Use(middleware.ThirdPartyAuth(authCfg.APIKeys, logger))
	}

	// 设备控制API
	api.POST("/devices/:device_id/charge", handler.StartCharge) // 启动充电
	api.POST("/devices/:device_id/stop", handler.StopCharge)    // 停止充电
	api.GET("/devices/:device_id", handler.GetDevice)           // 查询设备状态

	// 订单查询API
	api.GET("/orders/:order_id", handler.GetOrder)              // 查询订单详情
	api.GET("/orders", handler.ListOrders)                      // 订单列表（分页）
	api.POST("/orders/:order_id/cancel", handler.CancelOrder)   // P0修复: 取消订单
	api.GET("/orders/:order_id/events", handler.GetOrderEvents) // P1-7完善: 查询订单事件

	// 参数和OTA API
	api.POST("/devices/:device_id/params", handler.SetParams) // 设置参数
	api.POST("/devices/:device_id/ota", handler.TriggerOTA)   // 触发OTA升级

	// 组网管理API
	api.POST("/devices/:device_id/network/configure", handler.ConfigureNetwork) // 配置组网

	logger.Info("third party routes registered", zap.Int("endpoints", 10)) // P1-7: 9→10
}
