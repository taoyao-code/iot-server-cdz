package main

import (
	"github.com/taoyao-code/iot-server/internal/app/bootstrap"
	cfgpkg "github.com/taoyao-code/iot-server/internal/config"
	"github.com/taoyao-code/iot-server/internal/logging"
)

// @title IoT充电桩服务器 API
// @version 1.0.0
// @description IoT充电桩设备管理和第三方集成API
// @termsOfService http://swagger.io/terms/

// @contact.name API Support
// @contact.email support@example.com

// @license.name Apache 2.0
// @license.url http://www.apache.org/licenses/LICENSE-2.0.html

// @host 182.43.177.92:7055
// @BasePath /

// @securityDefinitions.apikey ApiKeyAuth
// @in header
// @name X-Api-Key

// @securityDefinitions.apikey SignatureAuth
// @in header
// @name X-Signature

func main() {
	cfg, err := cfgpkg.Load("")
	if err != nil {
		panic(err)
	}
	logger, err := logging.InitLogger(cfg.Logging)
	if err != nil {
		panic(err)
	}
	defer func() { _ = logger.Sync() }()
	_ = bootstrap.Run(cfg, logger)
}
