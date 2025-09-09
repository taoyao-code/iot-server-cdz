package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	cfgpkg "github.com/taoyao-code/iot-server/internal/config"
	"github.com/taoyao-code/iot-server/internal/httpserver"
	"github.com/taoyao-code/iot-server/internal/logging"
	"github.com/taoyao-code/iot-server/internal/metrics"
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

	// 4) HTTP 服务
	httpSrv := httpserver.New(cfg.HTTP, cfg.Metrics.Path, metricsHandler, func() bool { return true })

	// 5) TCP 网关
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

	// 信号处理，优雅关闭
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(ctx)
	_ = tcpSrv.Shutdown(ctx)
}
