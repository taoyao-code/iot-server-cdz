package app

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/taoyao-code/iot-server/internal/metrics"
)

// NewMetrics 初始化注册表与应用指标
func NewMetrics() (*prometheus.Registry, *metrics.AppMetrics) {
	reg := metrics.NewRegistry()
	appm := metrics.NewAppMetrics(reg)
	return reg, appm
}
