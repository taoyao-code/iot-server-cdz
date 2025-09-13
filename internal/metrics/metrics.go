package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// NewRegistry 创建自定义 Prometheus Registry，并注册常用采集器
func NewRegistry() *prometheus.Registry {
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	return reg
}

// Handler 返回 Prometheus 指标 HTTP 处理器
func Handler(reg *prometheus.Registry) http.Handler {
	return promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg})
}

// AppMetrics 自定义业务指标
type AppMetrics struct {
	TCPAccepted      prometheus.Counter
	TCPBytesReceived prometheus.Counter
	AP3000ParseTotal *prometheus.CounterVec // labels: result=ok|error
	AP3000RouteTotal *prometheus.CounterVec // labels: cmd
	BKVRouteTotal    *prometheus.CounterVec // labels: cmd
	OnlineGauge      prometheus.Gauge       // 当前在线设备数
	HeartbeatTotal   prometheus.Counter     // 心跳计数
	// 新增：协议与出站可靠性
	AP3000Ack82Total         *prometheus.CounterVec // labels: result=ok|err
	OutboundResendTotal      prometheus.Counter     // 出站重试计数
	OutboundTimeoutTotal     prometheus.Counter     // 出站ACK超时计数
	OutboundDeadCleanupTotal prometheus.Counter     // dead 清理删除的记录数（累加）
	OutboundQueueSize        *prometheus.GaugeVec   // labels: status=0|1|2|3
	// 新增：会话离线事件
	SessionOfflineTotal *prometheus.CounterVec // labels: reason=heartbeat|ack|tcp
}

// NewAppMetrics 注册并返回业务指标
func NewAppMetrics(reg *prometheus.Registry) *AppMetrics {
	m := &AppMetrics{
		TCPAccepted: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "tcp_accept_total",
			Help: "Total accepted TCP connections.",
		}),
		TCPBytesReceived: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "tcp_bytes_received_total",
			Help: "Total bytes received over TCP.",
		}),
		AP3000ParseTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "ap3000_parse_total",
			Help: "AP3000 frame parse attempts.",
		}, []string{"result"}),
		AP3000RouteTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "ap3000_route_total",
			Help: "AP3000 routed frames by command.",
		}, []string{"cmd"}),
		BKVRouteTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "bkv_route_total",
			Help: "BKV routed frames by command.",
		}, []string{"cmd"}),
		OnlineGauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "session_online_count",
			Help: "Current number of online devices.",
		}),
		HeartbeatTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "session_heartbeat_total",
			Help: "Total heartbeats observed.",
		}),
		AP3000Ack82Total: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "ap3000_82_ack_total",
			Help: "Count of 0x82 ACK results by outcome.",
		}, []string{"result"}),
		OutboundResendTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "outbound_resend_total",
			Help: "Total number of outbound resend attempts.",
		}),
		OutboundTimeoutTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "outbound_timeout_total",
			Help: "Total number of outbound ACK timeouts handled.",
		}),
		OutboundDeadCleanupTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "outbound_dead_cleanup_total",
			Help: "Total number of deleted dead outbound records.",
		}),
		OutboundQueueSize: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "outbound_queue_size",
			Help: "Current outbound_queue size by status.",
		}, []string{"status"}),
		SessionOfflineTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "session_offline_total",
			Help: "Count of offline decisions by reason.",
		}, []string{"reason"}),
	}
	reg.MustRegister(
		m.TCPAccepted, m.TCPBytesReceived,
		m.AP3000ParseTotal, m.AP3000RouteTotal, m.BKVRouteTotal,
		m.OnlineGauge, m.HeartbeatTotal,
		m.AP3000Ack82Total, m.OutboundResendTotal, m.OutboundTimeoutTotal, m.OutboundDeadCleanupTotal,
		m.OutboundQueueSize, m.SessionOfflineTotal,
	)
	return m
}
