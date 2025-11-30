package bootstrap

import (
	"context"
	"fmt"
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
	"github.com/taoyao-code/iot-server/internal/ordersession"
	"github.com/taoyao-code/iot-server/internal/protocol/ap3000"
	"github.com/taoyao-code/iot-server/internal/protocol/bkv"
	"github.com/taoyao-code/iot-server/internal/service"
	"github.com/taoyao-code/iot-server/internal/session"
	pgstorage "github.com/taoyao-code/iot-server/internal/storage/pg"
	"github.com/taoyao-code/iot-server/internal/thirdparty"
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

	// 生成服务器实例ID（用于Redis会话管理）
	serverID := app.GenerateServerID()
	log.Info("basic components initialized", zap.String("server_id", serverID))

	// ========== 阶段2: 初始化Redis（必选依赖）==========
	redisClient, err := app.NewRedisClient(cfg.Redis, log)
	if err != nil {
		log.Error("redis initialization failed", zap.Error(err))
		return fmt.Errorf("redis is required: %w", err)
	}
	defer redisClient.Close()
	log.Info("redis initialized successfully")

	// ========== 阶段3: 初始化会话管理器（需要Redis客户端）==========
	sess, policy := app.NewSessionAndPolicy(cfg.Session, redisClient, serverID, log)

	// ========== 阶段4: 连接数据库（阻塞等待，失败直接返回）==========
	dbpool, err := app.ConnectDBAndMigrate(context.Background(), cfg.Database, "db/migrations", log)
	if err != nil {
		log.Error("database initialization failed", zap.Error(err))
		return err
	}
	defer dbpool.Close()

	coreRepo, coreSQLDB, err := app.NewCoreRepo(cfg.Database, log)
	if err != nil {
		log.Error("gorm core repo initialization failed", zap.Error(err))
		return err
	}
	defer coreSQLDB.Close()

	ready.SetDBReady(true)
	log.Info("database ready", zap.String("dsn", maskDSN(cfg.Database.DSN)))

	// ========== 阶段5: 初始化业务处理器（确保DB已就绪）==========
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

	pusher, _ := app.NewPusherIfEnabled(cfg.Thirdparty.Push.WebhookURL, cfg.Thirdparty.Push.Secret)

	// 初始化事件队列和去重器
	var pusherTyped *thirdparty.Pusher
	if pusher != nil {
		pusherTyped, _ = pusher.(*thirdparty.Pusher)
	}
	eventQueue, deduper := app.NewEventQueue(cfg.Thirdparty.Push, redisClient, pusherTyped, log)

	redisQueue := app.NewRedisOutboundQueue(redisClient)

	// Week5: 创建Outbound适配器（用于BKV下行消息）
	outboundAdapter := app.NewOutboundAdapter(dbpool, repo, redisQueue)
	driverCommandSource := bkv.NewCommandSource(outboundAdapter, log)

	// 创建CardService（刷卡充电业务）
	pricingEngine := service.NewPricingEngine()
	cardService := service.NewCardService(repo, pricingEngine, log)

	// DriverCore: 协议驱动 -> 核心的事件收敛入口
	driverCore := app.NewDriverCore(coreRepo, eventQueue, log)
	orderTracker := ordersession.NewTracker(ordersession.WithObserver(ordersession.ObserverFunc(func(operation, status string) {
		if appm != nil {
			appm.ObserveSessionMapping(operation, status)
		}
	})))

	// v2.1: 注入Metrics支持充电上报监控
	bkvHandlers := bkv.NewHandlersWithServices(repo, coreRepo, bkvReason, cardService, outboundAdapter, eventQueue, deduper, driverCore, orderTracker)
	bkvHandlers.Metrics = appm // 注入指标采集器

	log.Info("protocol handlers initialized",
		zap.Bool("ap3000", cfg.Protocols.EnableAP3000),
		zap.Bool("bkv", cfg.Protocols.EnableBKV),
		zap.Bool("event_queue", eventQueue != nil),
		zap.Bool("outbound", outboundAdapter != nil),
		zap.Bool("deduper", deduper != nil))

	// ========== 阶段6: 启动HTTP服务（非阻塞）==========
	readyFn := func() bool { return ready.Ready() }
	httpSrv := app.NewHTTPServer(cfg.HTTP, cfg.Metrics.Path, metricsHandler, readyFn)

	// Week2: 创建健康检查聚合器
	healthAgg := app.NewHealthAggregator(dbpool)
	// Week2.2: 添加Redis健康检查器（如果Redis已启用）
	app.AddRedisChecker(healthAgg, redisClient)
	log.Info("health aggregator initialized")

	// 注册路由时传入认证配置
	// 同时注册健康检查路由
	httpSrv.Register(func(r *gin.Engine) {
		authCfg := middleware.AuthConfig{
			APIKeys: cfg.API.Auth.APIKeys,
			Enabled: cfg.API.Auth.Enabled,
		}
		api.RegisterReadOnlyRoutes(r, repo, sess, policy, authCfg, log)

		// 注册第三方API路由
		thirdpartyAuthCfg := middleware.AuthConfig{
			APIKeys: cfg.Thirdparty.Auth.APIKeys,
			Enabled: len(cfg.Thirdparty.Auth.APIKeys) > 0,
		}
		log.Info("third party api authentication config",
			zap.Int("api_keys_count", len(thirdpartyAuthCfg.APIKeys)),
			zap.Bool("enabled", thirdpartyAuthCfg.Enabled))
		api.RegisterThirdPartyRoutes(r, repo, coreRepo, sess, driverCommandSource, driverCore, orderTracker, eventQueue, appm, thirdpartyAuthCfg, log)

		r.Static("/static", "./web/static")
		r.GET("/test-console", func(c *gin.Context) {
			c.File("./web/static/index.html")
		})

		app.RegisterHealthRoutes(r, healthAgg) // Week2: 健康检查路由
	})

	go func() {
		if err := httpSrv.Start(); err != nil {
			log.Error("http server error", zap.Error(err))
		}
	}()
	log.Info("http server started", zap.String("addr", cfg.HTTP.Addr))

	// ========== 阶段7: 启动下行队列Worker（使用Redis队列）==========
	// Redis队列性能比PostgreSQL轮询快10倍
	redisWorker := app.NewRedisWorker(redisQueue, cfg.Gateway.ThrottleMs, cfg.Gateway.RetryMax, log)

	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()

	redisWorker.SetGetConn(func(phyID string) (interface{}, bool) { return sess.GetConn(phyID) })
	go redisWorker.Start(workerCtx)
	log.Info("redis outbound worker started",
		zap.Int("throttle_ms", cfg.Gateway.ThrottleMs),
		zap.Int("retry_max", cfg.Gateway.RetryMax))

	// ========== 阶段7.5: 启动事件队列/推送器(若启用)==========
	startEventPipeline(workerCtx, repo, eventQueue, cfg.Thirdparty.Push, log)

	// ========== 阶段7.7: P1-4启动端口状态同步器(检测端口状态不一致)==========
	// 修复：注入SessionManager用于实时在线判断
	portSyncer := app.NewPortStatusSyncer(repo, sess, driverCommandSource, appm, log)
	go portSyncer.Start(workerCtx)

	// ========== 阶段8: 最后启动TCP服务(此时所有依赖已就绪)==========
	tcpSrv := app.NewTCPServer(cfg.TCP, log) // Week2: 传递logger以支持限流日志
	tcpSrv.SetMetricsCallbacks(
		func() { appm.TCPAccepted.Inc() },
		func(n int) { appm.TCPBytesReceived.Add(float64(n)) },
	)
	tcpSrv.SetConnHandler(gateway.NewConnHandler(
		cfg.Protocols, sess, policy, appm,
		func() *ap3000.Handlers { return nil }, // AP3000 暂未实现
		func() *bkv.Handlers { return bkvHandlers },
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

	// ========== 阶段9: 等待关闭信号 ==========
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	workerCancel()

	log.Info("received shutdown signal, gracefully shutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_ = httpSrv.Shutdown(shutdownCtx)
	log.Info("http server stopped")

	_ = tcpSrv.Shutdown(shutdownCtx)
	log.Info("tcp server stopped")

	// 清理Redis会话数据
	if redisMgr, ok := sess.(*session.RedisManager); ok {
		if err := redisMgr.Cleanup(); err != nil {
			log.Warn("redis session cleanup failed", zap.Error(err))
		} else {
			log.Info("redis session cleaned up")
		}
	}

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

func startEventPipeline(ctx context.Context, repo *pgstorage.Repository, queue *thirdparty.EventQueue, pushCfg cfgpkg.ThirdpartyPushConfig, log *zap.Logger) {
	if queue == nil {
		return
	}
	app.StartEventQueueWorkers(ctx, queue, pushCfg.WorkerCount, log)
	eventPusher := app.NewEventPusher(repo, queue, log)
	go eventPusher.Start(ctx)
	log.Info("event pipeline started",
		zap.Int("push_workers", pushCfg.WorkerCount),
		zap.Duration("pusher_interval", 10*time.Second),
		zap.Int("pusher_batch", 50))
}
