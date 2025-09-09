package httpserver

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	cfgpkg "github.com/taoyao-code/iot-server/internal/config"
)

// Server HTTP 服务封装
type Server struct {
	srv *http.Server
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

	srv := &http.Server{
		Addr:         cfg.Addr,
		Handler:      r,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	}
	return &Server{srv: srv}
}

// Start 启动 HTTP 服务（阻塞）
func (s *Server) Start() error {
	return s.srv.ListenAndServe()
}

// Shutdown 优雅关闭
func (s *Server) Shutdown(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}
