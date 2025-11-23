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

// RegisterTestConsoleRoutes 注册内部测试控制台路由
// 仅供内部测试/运维人员使用，需要严格的访问控制
func RegisterTestConsoleRoutes(
	r *gin.Engine,
	repo *pgstorage.Repository,
	coreRepo storage.CoreRepo,
	sess session.SessionManager,
	commandSource driverapi.CommandSource,
	eventQueue *thirdparty.EventQueue,
	metrics *metrics.AppMetrics,
	authCfg middleware.AuthConfig,
	logger *zap.Logger,
	enableTestConsole bool,
) {
	// 如果测试控制台未启用，直接返回
	if !enableTestConsole {
		logger.Info("test console disabled, skipping route registration")
		return
	}

	// 创建处理器
	handler := NewTestConsoleHandler(repo, coreRepo, sess, commandSource, eventQueue, metrics, logger)

	// 内部测试控制台路由组
	// 使用更严格的认证策略
	internal := r.Group("/internal/test")

	// 启用内部认证中间件
	if authCfg.Enabled {
		internal.Use(middleware.InternalAuth(authCfg.APIKeys, logger))
		logger.Info("test console authentication enabled")
	} else {
		logger.Warn("test console authentication disabled - only for development!")
	}

	// 设备查询API
	internal.GET("/devices", handler.ListTestDevices)          // 列出可测试设备
	internal.GET("/devices/:device_id", handler.GetTestDevice) // 获取设备详情

	// 测试场景API
	internal.GET("/scenarios", handler.ListTestScenarios) // 列出测试场景

	// 测试控制API
	internal.POST("/devices/:device_id/charge", handler.StartTestCharge) // 启动测试充电
	internal.POST("/devices/:device_id/stop", handler.StopTestCharge)    // 停止测试充电

	// 订单查询API
	internal.GET("/orders", handler.ListTestOrders)         // 列出测试订单
	internal.GET("/orders/:order_no", handler.GetTestOrder) // 获取订单详情

	// 时间线查询API
	internal.GET("/sessions/:test_session_id", handler.GetTestSession) // 获取测试会话时间线

	// 模拟第三方调用API
	internal.POST("/simulate", handler.SimulateThirdPartyCall) // 模拟第三方调用

	logger.Info("test console routes registered", zap.Int("endpoints", 10))
}
