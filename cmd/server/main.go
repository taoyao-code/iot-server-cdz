package main

import (
	"github.com/taoyao-code/iot-server/internal/app/bootstrap"
	cfgpkg "github.com/taoyao-code/iot-server/internal/config"
	"github.com/taoyao-code/iot-server/internal/logging"
)

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
