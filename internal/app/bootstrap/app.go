package bootstrap

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/taoyao-code/iot-server/internal/api"
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
func Run(cfg *cfgpkg.Config, log *zap.Logger) error {
	reg, appm := app.NewMetrics()
	metricsHandler := metrics.Handler(reg)
	ready := app.NewReady()
	sess, policy := app.NewSessionAndPolicy(cfg.Session)

	readyFn := func() bool { return ready.Ready() }
	httpSrv := app.NewHTTPServer(cfg.HTTP, cfg.Metrics.Path, metricsHandler, readyFn)
	tcpSrv := app.NewTCPServer(cfg.TCP)
	tcpSrv.SetMetricsCallbacks(func() { appm.TCPAccepted.Inc() }, func(n int) { appm.TCPBytesReceived.Add(float64(n)) })

	var repo *pgstorage.Repository
	var bkvReason *bkv.ReasonMap
	if cfg.Protocols.EnableBKV && cfg.Protocols.BKV.ReasonMapPath != "" {
		if rm, e := bkv.LoadReasonMap(cfg.Protocols.BKV.ReasonMapPath); e == nil {
			bkvReason = rm
		} else {
			log.Warn("load bkv reason map failed", zap.Error(e))
		}
	}

	var handlerSet *ap3000.Handlers
	var bkvHandlers *bkv.Handlers
	tcpSrv.SetConnHandler(gateway.NewConnHandler(cfg.Protocols, sess, policy, appm, func() *ap3000.Handlers { return handlerSet }, func() *bkv.Handlers { return bkvHandlers }))

	go func() {
		if err := httpSrv.Start(); err != nil {
			log.Error("http server error", zap.Error(err))
		}
	}()
	if err := tcpSrv.Start(); err != nil {
		log.Fatal("tcp server start error", zap.Error(err))
	}
	ready.SetTCPReady(true)

	dbpool, err := app.ConnectDBAndMigrate(context.Background(), cfg.Database, "db/migrations", log)
	if err != nil {
		log.Error("db connect error", zap.Error(err))
	} else {
		ready.SetDBReady(true)
		repo = &pgstorage.Repository{Pool: dbpool}
		pusher, pushURL := app.NewPusherIfEnabled(cfg.Thirdparty.Push.WebhookURL, cfg.Thirdparty.Push.Secret)
		handlerSet = &ap3000.Handlers{Repo: repo, Pusher: pusher, PushURL: pushURL, Metrics: appm}
		bkvHandlers = &bkv.Handlers{Repo: repo, Reason: bkvReason}
		defer dbpool.Close()

		httpSrv.Register(func(r *gin.Engine) { api.RegisterReadOnlyRoutes(r, repo, sess, policy) })

		wcancel, outw := app.StartOutbound(dbpool, cfg.Gateway.ThrottleMs, cfg.Gateway.RetryMax, cfg.Gateway.DeadRetentionDays, appm, sess)
		defer wcancel()
		outw.SetGetConn(func(phyID string) (interface{}, bool) { return sess.GetConn(phyID) })
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(ctx)
	_ = tcpSrv.Shutdown(ctx)
	return nil
}
