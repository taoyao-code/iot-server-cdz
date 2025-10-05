package app

import (
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/taoyao-code/iot-server/internal/health"
	"github.com/taoyao-code/iot-server/internal/tcpserver"
)

// NewHealthAggregator 创建健康检查聚合器 (Week2)
func NewHealthAggregator(dbpool *pgxpool.Pool) *health.Aggregator {
	// 初始时只添加数据库检查器
	return health.NewAggregator(
		health.NewDatabaseChecker(dbpool),
	)
}

// RegisterHealthRoutes 注册健康检查HTTP路由 (Week2)
func RegisterHealthRoutes(r *gin.Engine, aggregator *health.Aggregator) {
	health.RegisterHTTPRoutes(r, aggregator)
}

// AddTCPChecker 添加TCP检查器到聚合器 (Week2)
func AddTCPChecker(aggregator *health.Aggregator, tcpServer *tcpserver.Server) {
	aggregator.AddChecker(health.NewTCPChecker(tcpServer))
}
