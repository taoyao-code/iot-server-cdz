package main

import (
	"context"
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

	// 4) 就绪聚合
	ready := health.New()

	// 5) HTTP 服务
	httpSrv := httpserver.New(cfg.HTTP, cfg.Metrics.Path, metricsHandler, ready.Ready)

	// 6) TCP 网关
	tcpSrv := tcpserver.New(cfg.TCP)

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
		defer dbpool.Close()

		// 8) 自动迁移（可选）
		if cfg.Database.AutoMigrate {
			if err = (migrate.Runner{Dir: "db/migrations"}).Up(context.Background(), dbpool); err != nil {
				log.Error("db migrate error", zap.Error(err))
			} else {
				log.Info("db migrations applied")
			}
		}
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
