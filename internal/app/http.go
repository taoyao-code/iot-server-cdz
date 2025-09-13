package app

import (
	"net/http"

	cfgpkg "github.com/taoyao-code/iot-server/internal/config"
	"github.com/taoyao-code/iot-server/internal/httpserver"
)

// NewHTTPServer 根据配置创建 HTTP 服务器
func NewHTTPServer(cfg cfgpkg.HTTPConfig, metricsPath string, metricsHandler http.Handler, readyFn func() bool) *httpserver.Server {
	return httpserver.New(cfg, metricsPath, metricsHandler, readyFn)
}
