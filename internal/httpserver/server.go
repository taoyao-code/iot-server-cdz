package httpserver

import (
	"context"
	"net/http"
	"net/http/pprof"

	"github.com/gin-gonic/gin"
	cfgpkg "github.com/taoyao-code/iot-server/internal/config"
)

// Server HTTP 服务封装
type Server struct {
	srv *http.Server
	r   *gin.Engine
}

// New 创建并配置 Gin + HTTP Server，注册健康检查与指标路由
func New(cfg cfgpkg.HTTPConfig, metricsPath string, metricsHandler http.Handler, readyFn func() bool) *Server {
	r := gin.New()
	r.Use(gin.Recovery())

	r.GET("/healthz", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	r.GET("/readyz", func(c *gin.Context) {
		if readyFn == nil || readyFn() {
			c.String(http.StatusOK, "ready")
			return
		}
		c.String(http.StatusServiceUnavailable, "not-ready")
	})
	if metricsPath == "" {
		metricsPath = "/metrics"
	}
	if metricsHandler != nil {
		r.GET(metricsPath, gin.WrapH(metricsHandler))
	}

	// pprof（可选）
	if cfg.Pprof.Enable {
		prefix := cfg.Pprof.Prefix
		if prefix == "" {
			prefix = "/debug/pprof"
		}
		r.GET(prefix, gin.WrapH(http.HandlerFunc(pprof.Index)))
		r.GET(prefix+"/cmdline", gin.WrapH(http.HandlerFunc(pprof.Cmdline)))
		r.GET(prefix+"/profile", gin.WrapH(http.HandlerFunc(pprof.Profile)))
		r.GET(prefix+"/symbol", gin.WrapH(http.HandlerFunc(pprof.Symbol)))
		r.GET(prefix+"/trace", gin.WrapH(http.HandlerFunc(pprof.Trace)))
		r.GET(prefix+"/heap", gin.WrapH(pprof.Handler("heap")))
		r.GET(prefix+"/goroutine", gin.WrapH(pprof.Handler("goroutine")))
		r.GET(prefix+"/threadcreate", gin.WrapH(pprof.Handler("threadcreate")))
		r.GET(prefix+"/block", gin.WrapH(pprof.Handler("block")))
		r.GET(prefix+"/allocs", gin.WrapH(pprof.Handler("allocs")))
	}

	srv := &http.Server{
		Addr:         cfg.Addr,
		Handler:      r,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	}
	return &Server{srv: srv, r: r}
}

// Start 启动 HTTP 服务（阻塞）
func (s *Server) Start() error {
	return s.srv.ListenAndServe()
}

// Shutdown 优雅关闭
func (s *Server) Shutdown(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}

// Register 允许外部注册自定义路由
func (s *Server) Register(fn func(*gin.Engine)) {
	if s == nil || s.r == nil || fn == nil {
		return
	}
	fn(s.r)
}
