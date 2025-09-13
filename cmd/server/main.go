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
	"github.com/taoyao-code/iot-server/internal/protocol/adapter"
	"github.com/taoyao-code/iot-server/internal/protocol/ap3000"
	"github.com/taoyao-code/iot-server/internal/protocol/bkv"
	"github.com/taoyao-code/iot-server/internal/session"
	pgstorage "github.com/taoyao-code/iot-server/internal/storage/pg"
	"github.com/taoyao-code/iot-server/internal/tcpserver"
	"github.com/taoyao-code/iot-server/internal/thirdparty"

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
		return ready.Ready()
	}
	httpSrv := httpserver.New(cfg.HTTP, cfg.Metrics.Path, metricsHandler, readyFn)

	// 6) TCP 网关
	tcpSrv := tcpserver.New(cfg.TCP)
	tcpSrv.SetMetricsCallbacks(func() { appm.TCPAccepted.Inc() }, func(n int) { appm.TCPBytesReceived.Add(float64(n)) })

	// repo 声明（在 DB 成功后赋值）
	var repo *pgstorage.Repository

	// 预加载 BKV 原因映射（可选）
	var bkvReason *bkv.ReasonMap
	if cfg.Protocols.EnableBKV && cfg.Protocols.BKV.ReasonMapPath != "" {
		if rm, e := bkv.LoadReasonMap(cfg.Protocols.BKV.ReasonMapPath); e == nil {
			bkvReason = rm
		} else {
			log.Warn("load bkv reason map failed", zap.Error(e))
		}
	}

	// 使用连接级处理器 + 多协议复用器（首帧初判 -> 固定协议处理）
	var handlerSet *ap3000.Handlers // repo 初始化后再赋值（handler 内部已判空）
	tcpSrv.SetConnHandler(func(cc *tcpserver.ConnContext) {
		var adapters []adapter.Adapter
		var apAdapter *ap3000.Adapter
		if cfg.Protocols.EnableAP3000 {
			apAdapter = ap3000.NewAdapter()
			adapters = append(adapters, apAdapter)
		}
		var bkvAdapter *bkv.Adapter
		if cfg.Protocols.EnableBKV {
			bkvAdapter = bkv.NewAdapter()
			adapters = append(adapters, bkvAdapter)
		}

		var boundPhy string
		bindIfNeeded := func(phy string) {
			if boundPhy != phy {
				boundPhy = phy
				sess.Bind(phy, cc)
			}
		}

		if apAdapter != nil {
			apAdapter.Register(0x20, func(f *ap3000.Frame) error {
				bindIfNeeded(f.PhyID)
				sess.OnHeartbeat(f.PhyID, time.Now())
				appm.HeartbeatTotal.Inc()
				appm.OnlineGauge.Set(float64(sess.OnlineCount(time.Now())))
				appm.AP3000RouteTotal.WithLabelValues(fmt.Sprintf("%02X", f.Cmd)).Inc()
				return handlerSet.HandleRegister(context.Background(), f)
			})
			apAdapter.Register(0x21, func(f *ap3000.Frame) error {
				bindIfNeeded(f.PhyID)
				sess.OnHeartbeat(f.PhyID, time.Now())
				appm.HeartbeatTotal.Inc()
				appm.OnlineGauge.Set(float64(sess.OnlineCount(time.Now())))
				appm.AP3000RouteTotal.WithLabelValues(fmt.Sprintf("%02X", f.Cmd)).Inc()
				return handlerSet.HandleHeartbeat(context.Background(), f)
			})
			apAdapter.Register(0x22, func(f *ap3000.Frame) error {
				appm.AP3000RouteTotal.WithLabelValues(fmt.Sprintf("%02X", f.Cmd)).Inc()
				return handlerSet.HandleGeneric(context.Background(), f)
			})
			apAdapter.Register(0x12, func(f *ap3000.Frame) error {
				appm.AP3000RouteTotal.WithLabelValues(fmt.Sprintf("%02X", f.Cmd)).Inc()
				return handlerSet.HandleGeneric(context.Background(), f)
			})
			apAdapter.Register(0x82, func(f *ap3000.Frame) error {
				appm.AP3000RouteTotal.WithLabelValues(fmt.Sprintf("%02X", f.Cmd)).Inc()
				return handlerSet.Handle82Ack(context.Background(), f)
			})
			apAdapter.Register(0x03, func(f *ap3000.Frame) error {
				appm.AP3000RouteTotal.WithLabelValues(fmt.Sprintf("%02X", f.Cmd)).Inc()
				return handlerSet.Handle03(context.Background(), f)
			})
			apAdapter.Register(0x06, func(f *ap3000.Frame) error {
				appm.AP3000RouteTotal.WithLabelValues(fmt.Sprintf("%02X", f.Cmd)).Inc()
				return handlerSet.Handle06(context.Background(), f)
			})
		}

		if bkvAdapter != nil {
			bh := &bkv.Handlers{Repo: repo, Reason: bkvReason}
			bkvAdapter.Register(0x10, func(f *bkv.Frame) error {
				appm.BKVRouteTotal.WithLabelValues(fmt.Sprintf("%02X", f.Cmd)).Inc()
				return bh.HandleHeartbeat(context.Background(), f)
			})
			bkvAdapter.Register(0x11, func(f *bkv.Frame) error {
				appm.BKVRouteTotal.WithLabelValues(fmt.Sprintf("%02X", f.Cmd)).Inc()
				return bh.HandleStatus(context.Background(), f)
			})
			bkvAdapter.Register(0x30, func(f *bkv.Frame) error {
				appm.BKVRouteTotal.WithLabelValues(fmt.Sprintf("%02X", f.Cmd)).Inc()
				return bh.HandleSettle(context.Background(), f)
			})
			bkvAdapter.Register(0x82, func(f *bkv.Frame) error {
				appm.BKVRouteTotal.WithLabelValues(fmt.Sprintf("%02X", f.Cmd)).Inc()
				return bh.HandleAck(context.Background(), f)
			})
			// 占位：控制与参数
			bkvAdapter.Register(0x90, func(f *bkv.Frame) error {
				appm.BKVRouteTotal.WithLabelValues(fmt.Sprintf("%02X", f.Cmd)).Inc()
				return bh.HandleControl(context.Background(), f)
			})
			bkvAdapter.Register(0x83, func(f *bkv.Frame) error {
				appm.BKVRouteTotal.WithLabelValues(fmt.Sprintf("%02X", f.Cmd)).Inc()
				return bh.HandleParam(context.Background(), f)
			})
			bkvAdapter.Register(0x84, func(f *bkv.Frame) error {
				appm.BKVRouteTotal.WithLabelValues(fmt.Sprintf("%02X", f.Cmd)).Inc()
				return bh.HandleParam(context.Background(), f)
			})
			bkvAdapter.Register(0x85, func(f *bkv.Frame) error {
				appm.BKVRouteTotal.WithLabelValues(fmt.Sprintf("%02X", f.Cmd)).Inc()
				return bh.HandleParam(context.Background(), f)
			})
		}

		mux := tcpserver.NewMux(adapters...)
		mux.BindToConn(cc)

		go func() {
			<-cc.Done()
			if boundPhy != "" {
				sess.UnbindByPhy(boundPhy)
			}
		}()
	})

	go func() {
		if err := httpSrv.Start(); err != nil {
			log.Error("http server error", zap.Error(err))
		}
	}()
	if err := tcpSrv.Start(); err != nil {
		log.Fatal("tcp server start error", zap.Error(err))
	}
	ready.SetTCPReady(true)

	dbpool, err := pgstorage.NewPool(context.Background(), cfg.Database.DSN, cfg.Database.MaxOpenConns, cfg.Database.MaxIdleConns, cfg.Database.ConnMaxLifetime)
	if err != nil {
		log.Error("db connect error", zap.Error(err))
	} else {
		ready.SetDBReady(true)
		repo = &pgstorage.Repository{Pool: dbpool}
		var pusher interface {
			SendJSON(ctx context.Context, endpoint string, payload any) (int, []byte, error)
		}
		var pushURL string
		if cfg.Thirdparty.Push.WebhookURL != "" && cfg.Thirdparty.Push.Secret != "" {
			pusher = thirdparty.NewPusher(nil, "", cfg.Thirdparty.Push.Secret)
			pushURL = cfg.Thirdparty.Push.WebhookURL
		}
		handlerSet = &ap3000.Handlers{Repo: repo, Pusher: pusher, PushURL: pushURL, Metrics: appm}
		defer dbpool.Close()

		if cfg.Database.AutoMigrate {
			if err = (migrate.Runner{Dir: "db/migrations"}).Up(context.Background(), dbpool); err != nil {
				log.Error("db migrate error", zap.Error(err))
			} else {
				log.Info("db migrations applied")
			}
		}

		wctx, wcancel := context.WithCancel(context.Background())
		defer wcancel()
		outw := outbound.New(dbpool)
		outw.Throttle = time.Duration(cfg.Gateway.ThrottleMs) * time.Millisecond
		if cfg.Gateway.RetryMax > 0 {
			outw.MaxRetries = cfg.Gateway.RetryMax
		}
		outw.DeadRetentionDays = cfg.Gateway.DeadRetentionDays
		outw.Metrics = appm
		outw.SetGetConn(func(phyID string) (interface{}, bool) {
			c, ok := sess.GetConn(phyID)
			return c, ok
		})
		go outw.Run(wctx)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(ctx)
	_ = tcpSrv.Shutdown(ctx)
}
