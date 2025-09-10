package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	cfgpkg "github.com/taoyao-code/iot-server/internal/config"
	"github.com/taoyao-code/iot-server/internal/health"
	"github.com/taoyao-code/iot-server/internal/httpserver"
	"github.com/taoyao-code/iot-server/internal/logging"
	"github.com/taoyao-code/iot-server/internal/metrics"
	"github.com/taoyao-code/iot-server/internal/migrate"
	"github.com/taoyao-code/iot-server/internal/outbound"
	"github.com/taoyao-code/iot-server/internal/protocol/ap3000"
	"github.com/taoyao-code/iot-server/internal/session"
	pgstorage "github.com/taoyao-code/iot-server/internal/storage/pg"
	"github.com/taoyao-code/iot-server/internal/tcpserver"

	"go.uber.org/zap"
)

func main() {
	// 1) 加载配置
	cfg, err := cfgpkg.Load("")
	if err != nil {
		panic(err)
	}

	// 2) 初始化日志
	logger, err := logging.InitLogger(cfg.Logging)
	if err != nil {
		panic(err)
	}
	defer func() { _ = logger.Sync() }()
	zap.ReplaceGlobals(logger)
	log := zap.L()

	// 3) 指标注册与处理器
	reg := metrics.NewRegistry()
	metricsHandler := metrics.Handler(reg)
	appm := metrics.NewAppMetrics(reg)

	// 4) 就绪聚合
	ready := health.New()

	// 会话管理
	sess := session.New(6 * time.Minute)

	// 5) HTTP 服务（ready 由闭包计算）
	readyFn := func() bool {
		// 占位阈值：在线数量>=0，即只要 DB/TCP Ready 则返回 ready；
		// 后续可从配置读取阈值
		return ready.Ready()
	}
	httpSrv := httpserver.New(cfg.HTTP, cfg.Metrics.Path, metricsHandler, readyFn)

	// 6) TCP 网关
	tcpSrv := tcpserver.New(cfg.TCP)
	tcpSrv.SetMetricsCallbacks(func() { appm.TCPAccepted.Inc() }, func(n int) { appm.TCPBytesReceived.Add(float64(n)) })

	// 接线：读取到的原始数据，尝试按 AP3000 解析并路由（占位实现）
	router := ap3000.NewTable()
	// 占位：注册常用指令
	var handlerSet *ap3000.Handlers // repo 初始化后再赋值
	router.Register(0x20, func(f *ap3000.Frame) error {
		sess.OnHeartbeat(f.PhyID, time.Now())
		appm.HeartbeatTotal.Inc()
		appm.OnlineGauge.Set(float64(sess.OnlineCount(time.Now())))
		return handlerSet.HandleRegister(context.Background(), f)
	})
	router.Register(0x21, func(f *ap3000.Frame) error {
		sess.OnHeartbeat(f.PhyID, time.Now())
		appm.HeartbeatTotal.Inc()
		appm.OnlineGauge.Set(float64(sess.OnlineCount(time.Now())))
		return handlerSet.HandleHeartbeat(context.Background(), f)
	})
	router.Register(0x22, func(f *ap3000.Frame) error { return handlerSet.HandleGeneric(context.Background(), f) })
	router.Register(0x12, func(f *ap3000.Frame) error { return handlerSet.HandleGeneric(context.Background(), f) })
	router.Register(0x82, func(f *ap3000.Frame) error { return handlerSet.HandleGeneric(context.Background(), f) })
	router.Register(0x03, func(f *ap3000.Frame) error { return handlerSet.HandleGeneric(context.Background(), f) })
	router.Register(0x06, func(f *ap3000.Frame) error { return handlerSet.HandleGeneric(context.Background(), f) })

	tcpSrv.SetHandler(func(raw []byte) {
		if fr, err := ap3000.Parse(raw); err == nil {
			appm.AP3000ParseTotal.WithLabelValues("ok").Inc()
			appm.AP3000RouteTotal.WithLabelValues(fmt.Sprintf("%02X", fr.Cmd)).Inc()
			_ = router.Route(fr)
		} else {
			appm.AP3000ParseTotal.WithLabelValues("error").Inc()
		}
	})

	// 并行启动
	go func() {
		if err := httpSrv.Start(); err != nil {
			log.Error("http server error", zap.Error(err))
		}
	}()
	if err := tcpSrv.Start(); err != nil {
		log.Fatal("tcp server start error", zap.Error(err))
	}
	ready.SetTCPReady(true)

	// 7) 数据库连接
	dbpool, err := pgstorage.NewPool(context.Background(), cfg.Database.DSN, cfg.Database.MaxOpenConns, cfg.Database.MaxIdleConns, cfg.Database.ConnMaxLifetime)
	if err != nil {
		log.Error("db connect error", zap.Error(err))
	} else {
		ready.SetDBReady(true)
		repo := &pgstorage.Repository{Pool: dbpool}
		handlerSet = &ap3000.Handlers{Repo: repo}
		defer dbpool.Close()

		// 8) 自动迁移（可选）
		if cfg.Database.AutoMigrate {
			if err = (migrate.Runner{Dir: "db/migrations"}).Up(context.Background(), dbpool); err != nil {
				log.Error("db migrate error", zap.Error(err))
			} else {
				log.Info("db migrations applied")
			}
		}

		// 9) 启动下行 worker（占位）
		wctx, wcancel := context.WithCancel(context.Background())
		defer wcancel()
		outw := outbound.New(dbpool)
		go outw.Run(wctx)
	}

	// 信号处理，优雅关闭
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(ctx)
	_ = tcpSrv.Shutdown(ctx)
}
