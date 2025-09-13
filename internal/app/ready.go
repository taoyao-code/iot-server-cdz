package app

import "github.com/taoyao-code/iot-server/internal/health"

func NewReady() *health.Readiness { return health.New() }
