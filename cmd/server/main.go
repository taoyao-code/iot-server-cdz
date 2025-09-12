package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
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
		// 占位阈值：在线数量>=0，即只要 DB/TCP Ready 则返回 ready；
		// 后续可从配置读取阈值
		return ready.Ready()
	}
	httpSrv := httpserver.New(cfg.HTTP, cfg.Metrics.Path, metricsHandler, readyFn)

	// 6) TCP 网关
	tcpSrv := tcpserver.New(cfg.TCP)
	tcpSrv.SetMetricsCallbacks(func() { appm.TCPAccepted.Inc() }, func(n int) { appm.TCPBytesReceived.Add(float64(n)) })

	// repo 声明（在 DB 成功后赋值）
	var repo *pgstorage.Repository

	// 使用连接级处理器 + 多协议复用器（首帧初判 -> 固定协议处理）
	var handlerSet *ap3000.Handlers // repo 初始化后再赋值（handler 内部已判空）
	tcpSrv.SetConnHandler(func(cc *tcpserver.ConnContext) {
		// 构造启用的适配器列表
		var adapters []adapter.Adapter

		// 每个连接独立的协议适配器（持有各自的解码缓冲）
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

		// 绑定维持：在收到 0x20/0x21 心跳/注册时，绑定 phy -> 连接（BKV 暂用占位）
		var boundPhy string
		bindIfNeeded := func(phy string) {
			if boundPhy != phy {
				boundPhy = phy
				sess.Bind(phy, cc)
			}
		}

		// 仅当启用了 AP3000 时注册路由处理器与指标
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

		// BKV：最小处理器注册（占位指令 0x10 心跳，0x11 状态）
		if bkvAdapter != nil {
			bh := &bkv.Handlers{Repo: repo}
			bkvAdapter.Register(0x10, func(f *bkv.Frame) error { return bh.HandleHeartbeat(context.Background(), f) })
			bkvAdapter.Register(0x11, func(f *bkv.Frame) error { return bh.HandleStatus(context.Background(), f) })
		}

		// 复用器绑定到连接
		mux := tcpserver.NewMux(adapters...)
		mux.BindToConn(cc)

		// 连接关闭时解除绑定
		go func() {
			<-cc.Done()
			if boundPhy != "" {
				sess.UnbindByPhy(boundPhy)
			}
		}()
	})

	// 并行启动 HTTP（提前注册 API）
	apiToken := os.Getenv("IOT_HTTP_TOKEN")
	httpSrv.Register(func(r *gin.Engine) {
		api := r.Group("/api", func(c *gin.Context) {
			if apiToken != "" && c.GetHeader("X-Api-Token") != apiToken {
				c.AbortWithStatus(http.StatusUnauthorized)
				return
			}
		})
		api.GET("/devices", func(c *gin.Context) {
			limit := 100
			offset := 0
			if v := c.Query("limit"); v != "" {
				if n, err := strconv.Atoi(v); err == nil {
					limit = n
				}
			}
			if v := c.Query("offset"); v != "" {
				if n, err := strconv.Atoi(v); err == nil {
					offset = n
				}
			}
			if repo != nil {
				items, err := repo.ListDevices(c.Request.Context(), limit, offset)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, items)
				return
			}
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "repo not ready"})
		})
		api.GET("/devices/:phyID/ports", func(c *gin.Context) {
			phy := c.Param("phyID")
			if repo != nil {
				items, err := repo.ListPortsByPhyID(c.Request.Context(), phy)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, items)
				return
			}
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "repo not ready"})
		})
		api.POST("/commands", func(c *gin.Context) {
			var req struct {
				PhyID         string  `json:"phyID"`
				PortNo        *int    `json:"portNo"`
				Cmd           int     `json:"cmd"`
				Payload       []byte  `json:"payload"`
				Priority      int     `json:"priority"`
				CorrelationID *string `json:"correlationID"`
				TimeoutSec    int     `json:"timeoutSec"`
			}
			if err := c.BindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			if repo == nil {
				c.JSON(http.StatusServiceUnavailable, gin.H{"error": "repo not ready"})
				return
			}
			// resolve device id
			devID, err := repo.EnsureDevice(c.Request.Context(), req.PhyID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			phy := req.PhyID
			id, err := repo.EnqueueOutbox(c.Request.Context(), devID, &phy, req.PortNo, req.Cmd, req.Payload, req.Priority, req.CorrelationID, nil, req.TimeoutSec)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"id": id, "correlationID": req.CorrelationID})
		})
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
		repo = &pgstorage.Repository{Pool: dbpool}
		// 可选第三方推送器注入（依据配置 thirdparty.push.webhook_url/secret）
		var pusher interface {
			SendJSON(ctx context.Context, endpoint string, payload any) (int, []byte, error)
		}
		var pushURL string
		if cfg.Thirdparty.Push.WebhookURL != "" && cfg.Thirdparty.Push.Secret != "" {
			pusher = thirdparty.NewPusher(nil, "", cfg.Thirdparty.Push.Secret)
			pushURL = cfg.Thirdparty.Push.WebhookURL
		}
		handlerSet = &ap3000.Handlers{Repo: repo, Pusher: pusher, PushURL: pushURL}
		defer dbpool.Close()

		// 8) 自动迁移（可选）
		if cfg.Database.AutoMigrate {
			if err = (migrate.Runner{Dir: "db/migrations"}).Up(context.Background(), dbpool); err != nil {
				log.Error("db migrate error", zap.Error(err))
			} else {
				log.Info("db migrations applied")
			}
		}

		// 9) 启动下行 worker（接入配置）
		wctx, wcancel := context.WithCancel(context.Background())
		defer wcancel()
		outw := outbound.New(dbpool)
		outw.Throttle = time.Duration(cfg.Gateway.ThrottleMs) * time.Millisecond
		if cfg.Gateway.RetryMax > 0 {
			outw.MaxRetries = cfg.Gateway.RetryMax
		}
		outw.SetGetConn(func(phyID string) (interface{}, bool) {
			c, ok := sess.GetConn(phyID)
			return c, ok
		})
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
