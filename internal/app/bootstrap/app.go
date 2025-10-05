package bootstrap

import (
	"context"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/taoyao-code/iot-server/internal/api"
	"github.com/taoyao-code/iot-server/internal/api/middleware"
	"github.com/taoyao-code/iot-server/internal/app"
	cfgpkg "github.com/taoyao-code/iot-server/internal/config"
	"github.com/taoyao-code/iot-server/internal/gateway"
	"github.com/taoyao-code/iot-server/internal/metrics"
	"github.com/taoyao-code/iot-server/internal/protocol/ap3000"
	"github.com/taoyao-code/iot-server/internal/protocol/bkv"
	pgstorage "github.com/taoyao-code/iot-server/internal/storage/pg"
	"go.uber.org/zap"
)

// Run 统一启动流程
// P0修复: 重新编排启动顺序，确保依赖就绪后再启动TCP服务
func Run(cfg *cfgpkg.Config, log *zap.Logger) error {
	log.Info("starting IOT server", zap.String("version", "1.0.0"))

	// ========== 阶段1: 初始化基础组件 ==========
	reg, appm := app.NewMetrics()
	metricsHandler := metrics.Handler(reg)
	ready := app.NewReady()
	sess, policy := app.NewSessionAndPolicy(cfg.Session)
	log.Info("basic components initialized")

	// ========== 阶段2: 连接数据库（阻塞等待，失败直接返回）==========
	dbpool, err := app.ConnectDBAndMigrate(context.Background(), cfg.Database, "db/migrations", log)
	if err != nil {
		log.Error("database initialization failed", zap.Error(err))
		return err // P0修复: 数据库失败直接返回，不继续启动
	}
	defer dbpool.Close()
	ready.SetDBReady(true)
	log.Info("database ready", zap.String("dsn", maskDSN(cfg.Database.DSN)))

	// ========== 阶段3: 初始化业务处理器（确保DB已就绪）==========
	repo := &pgstorage.Repository{Pool: dbpool}

	var bkvReason *bkv.ReasonMap
	if cfg.Protocols.EnableBKV && cfg.Protocols.BKV.ReasonMapPath != "" {
		if rm, e := bkv.LoadReasonMap(cfg.Protocols.BKV.ReasonMapPath); e == nil {
			bkvReason = rm
			log.Info("bkv reason map loaded", zap.String("path", cfg.Protocols.BKV.ReasonMapPath))
		} else {
			log.Warn("load bkv reason map failed", zap.Error(e))
		}
	}

	pusher, pushURL := app.NewPusherIfEnabled(cfg.Thirdparty.Push.WebhookURL, cfg.Thirdparty.Push.Secret)
	handlerSet := &ap3000.Handlers{Repo: repo, Pusher: pusher, PushURL: pushURL, Metrics: appm}
	bkvHandlers := bkv.NewHandlers(repo, bkvReason)
	log.Info("protocol handlers initialized",
		zap.Bool("ap3000", cfg.Protocols.EnableAP3000),
		zap.Bool("bkv", cfg.Protocols.EnableBKV))

	// ========== 阶段4: 启动HTTP服务（非阻塞）==========
	readyFn := func() bool { return ready.Ready() }
	httpSrv := app.NewHTTPServer(cfg.HTTP, cfg.Metrics.Path, metricsHandler, readyFn)

	// Week2: 创建健康检查聚合器
	healthAgg := app.NewHealthAggregator(dbpool)
	log.Info("health aggregator initialized")

	// P0修复: 注册路由时传入认证配置
	// Week2: 同时注册健康检查路由
	httpSrv.Register(func(r *gin.Engine) {
		authCfg := middleware.AuthConfig{
			APIKeys: cfg.API.Auth.APIKeys,
			Enabled: cfg.API.Auth.Enabled,
		}
		api.RegisterReadOnlyRoutes(r, repo, sess, policy, authCfg, log)
		app.RegisterHealthRoutes(r, healthAgg) // Week2: 健康检查路由
	})

	go func() {
		if err := httpSrv.Start(); err != nil {
			log.Error("http server error", zap.Error(err))
		}
	}()
	log.Info("http server started", zap.String("addr", cfg.HTTP.Addr))

	// ========== Week2.2: 初始化Redis（如果启用）==========
	redisClient, err := app.NewRedisClient(cfg.Redis, log)
	if err != nil {
		log.Error("redis initialization failed", zap.Error(err))
		return err
	}
	if redisClient != nil {
		defer redisClient.Close()
		log.Info("redis initialized")

		// 添加Redis健康检查器
		app.AddRedisChecker(healthAgg, redisClient)
	}

	// ========== 阶段5: 启动下行队列Worker ==========
	var wcancel context.CancelFunc
	var outw interface {
		SetGetConn(func(string) (interface{}, bool))
	}

	if redisClient != nil {
		// Week2.2: 使用Redis队列
		redisQueue := app.NewRedisOutboundQueue(redisClient)
		redisWorker := app.NewRedisWorker(redisQueue, cfg.Gateway.ThrottleMs, cfg.Gateway.RetryMax, log)

		ctx, cancel := context.WithCancel(context.Background())
		wcancel = cancel

		go redisWorker.Start(ctx)
		redisWorker.SetGetConn(func(phyID string) (interface{}, bool) { return sess.GetConn(phyID) })
		log.Info("redis outbound worker started")
	} else {
		// 使用PostgreSQL队列（原有逻辑）
		var cancel context.CancelFunc
		cancel, outw = app.StartOutbound(dbpool, cfg.Gateway.ThrottleMs, cfg.Gateway.RetryMax, cfg.Gateway.DeadRetentionDays, appm, sess)
		wcancel = cancel
		outw.SetGetConn(func(phyID string) (interface{}, bool) { return sess.GetConn(phyID) })
		log.Info("postgresql outbound worker started")
	}
	defer wcancel()

	// ========== 阶段6: 最后启动TCP服务（此时所有依赖已就绪）==========
	tcpSrv := app.NewTCPServer(cfg.TCP, log) // Week2: 传递logger以支持限流日志
	tcpSrv.SetMetricsCallbacks(
		func() { appm.TCPAccepted.Inc() },
		func(n int) { appm.TCPBytesReceived.Add(float64(n)) },
	)
	tcpSrv.SetConnHandler(gateway.NewConnHandler(
		cfg.Protocols, sess, policy, appm,
		func() *ap3000.Handlers { return handlerSet }, // P0修复: Handler已初始化，非nil
		func() *bkv.Handlers { return bkvHandlers },   // P0修复: Handler已初始化，非nil
	))

	if err := tcpSrv.Start(); err != nil {
		log.Error("tcp server start failed", zap.Error(err))
		return err // P0修复: TCP启动失败直接返回
	}
	ready.SetTCPReady(true)
	log.Info("tcp server started", zap.String("addr", cfg.TCP.Addr))

	// Week2: TCP启动后，将TCP检查器添加到健康聚合器
	app.AddTCPChecker(healthAgg, tcpSrv)
	log.Info("all services ready, waiting for connections")

	// ========== 阶段7: 等待关闭信号 ==========
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Info("received shutdown signal, gracefully shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_ = httpSrv.Shutdown(ctx)
	log.Info("http server stopped")

	_ = tcpSrv.Shutdown(ctx)
	log.Info("tcp server stopped")

	log.Info("shutdown complete")
	return nil
}

// maskDSN 脱敏数据库连接字符串（隐藏密码）
func maskDSN(dsn string) string {
	// 简单实现：隐藏密码部分
	// postgres://user:password@host:port/db -> postgres://user:****@host:port/db
	if idx := strings.Index(dsn, "@"); idx > 0 {
		if pwdIdx := strings.LastIndex(dsn[:idx], ":"); pwdIdx > 0 {
			return dsn[:pwdIdx+1] + "****" + dsn[idx:]
		}
	}
	return dsn
}
