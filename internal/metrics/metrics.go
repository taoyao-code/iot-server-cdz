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
	// 新增：充电上报监控（2025-10-31）
	ChargeReportTotal        *prometheus.CounterVec // labels: device_id, port_no, status=charging|idle|abnormal
	ChargeReportPowerGauge   *prometheus.GaugeVec   // labels: device_id, port_no (瞬时功率W)
	ChargeReportCurrentGauge *prometheus.GaugeVec   // labels: device_id, port_no (瞬时电流A)
	ChargeReportEnergyTotal  *prometheus.CounterVec // labels: device_id, port_no (累计电量Wh)

	// P1-4: 端口状态同步监控
	PortStatusMismatchTotal      *prometheus.CounterVec // labels: reason=port_missing|status_mismatch|device_offline
	PortStatusAutoFixedTotal     prometheus.Counter     // 自动修复成功次数
	PortStatusQueryResponseTotal *prometheus.CounterVec // labels: device_id, status=空闲|充电中|故障

	// 关键监控指标（健康评估新增）
	OutboundACKTimeoutTotal       *prometheus.CounterVec // labels: device_id, cmd - ACK超时计数
	OrderStateInconsistencyTotal  *prometheus.CounterVec // labels: type=duplicate|invalid_transition - 订单状态不一致
	SessionZombieConnectionsGauge prometheus.Gauge       // 僵尸连接数（无会话但连接未关闭）
	ProtocolChecksumErrorTotal    *prometheus.CounterVec // labels: protocol=bkv|ap3000 - 校验和错误

	// 一致性监控指标 (Consistency Lifecycle Spec - Line 15)
	ConsistencyEventsTotal        *prometheus.CounterVec // labels: source, scenario, severity=info|warn|error
	ConsistencyAutoFixTotal       *prometheus.CounterVec // labels: source, scenario, action
	ConsistencyFallbackStopTotal  prometheus.Counter     // Fallback stop 无订单场景计数
	ConsistencyLonelyPortFixTotal prometheus.Counter     // 孤立端口修复计数
	ConsistencyOrderTimeoutTotal  *prometheus.CounterVec // labels: order_status=cancelling|stopping|interrupted
	ConsistencyQueueFailureTotal  *prometheus.CounterVec // labels: operation=enqueue|dequeue, reason
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
		ChargeReportTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "charge_report_total",
			Help: "Total charge status reports from devices.",
		}, []string{"device_id", "port_no", "status"}),
		ChargeReportPowerGauge: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "charge_report_power_watts",
			Help: "Current charging power in watts.",
		}, []string{"device_id", "port_no"}),
		ChargeReportCurrentGauge: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "charge_report_current_amperes",
			Help: "Current charging current in amperes.",
		}, []string{"device_id", "port_no"}),
		ChargeReportEnergyTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "charge_report_energy_wh_total",
			Help: "Total energy consumed in watt-hours.",
		}, []string{"device_id", "port_no"}),
		PortStatusMismatchTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "port_status_mismatch_total",
			Help: "P1-4: Port status inconsistencies detected by reason.",
		}, []string{"reason"}),
		PortStatusAutoFixedTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "port_status_auto_fixed_total",
			Help: "P1-4: Port status inconsistencies auto-fixed count.",
		}),
		PortStatusQueryResponseTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "port_status_query_response_total",
			Help: "P1-4: Port status query responses (0x1D) by device and status.",
		}, []string{"device_id", "status"}),
		OutboundACKTimeoutTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "outbound_ack_timeout_total",
			Help: "Total ACK timeouts by device and command.",
		}, []string{"device_id", "cmd"}),
		OrderStateInconsistencyTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "order_state_inconsistency_total",
			Help: "Order state inconsistencies detected by type.",
		}, []string{"type"}),
		SessionZombieConnectionsGauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "session_zombie_connections",
			Help: "Number of zombie connections (no session but connection not closed).",
		}),
		ProtocolChecksumErrorTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "protocol_checksum_error_total",
			Help: "Protocol checksum errors by protocol type.",
		}, []string{"protocol"}),

		// 一致性监控指标
		ConsistencyEventsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "consistency_events_total",
			Help: "Total consistency events by source, scenario and severity.",
		}, []string{"source", "scenario", "severity"}),
		ConsistencyAutoFixTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "consistency_auto_fix_total",
			Help: "Total auto-fix actions by source, scenario and action.",
		}, []string{"source", "scenario", "action"}),
		ConsistencyFallbackStopTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "consistency_fallback_stop_total",
			Help: "Total fallback stop operations (port charging without order).",
		}),
		ConsistencyLonelyPortFixTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "consistency_lonely_port_fix_total",
			Help: "Total lonely charging port fixes.",
		}),
		ConsistencyOrderTimeoutTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "consistency_order_timeout_total",
			Help: "Total order timeout auto-finalizations by order status.",
		}, []string{"order_status"}),
		ConsistencyQueueFailureTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "consistency_queue_failure_total",
			Help: "Total queue operation failures by operation and reason.",
		}, []string{"operation", "reason"}),
	}
	reg.MustRegister(
		m.TCPAccepted, m.TCPBytesReceived,
		m.AP3000ParseTotal, m.AP3000RouteTotal, m.BKVRouteTotal,
		m.OnlineGauge, m.HeartbeatTotal,
		m.AP3000Ack82Total, m.OutboundResendTotal, m.OutboundTimeoutTotal, m.OutboundDeadCleanupTotal,
		m.OutboundQueueSize, m.SessionOfflineTotal,
		m.ChargeReportTotal, m.ChargeReportPowerGauge, m.ChargeReportCurrentGauge, m.ChargeReportEnergyTotal,
		m.PortStatusMismatchTotal, m.PortStatusAutoFixedTotal, m.PortStatusQueryResponseTotal,
		m.OutboundACKTimeoutTotal, m.OrderStateInconsistencyTotal, m.SessionZombieConnectionsGauge, m.ProtocolChecksumErrorTotal,
		// 一致性监控指标
		m.ConsistencyEventsTotal, m.ConsistencyAutoFixTotal,
		m.ConsistencyFallbackStopTotal, m.ConsistencyLonelyPortFixTotal,
		m.ConsistencyOrderTimeoutTotal, m.ConsistencyQueueFailureTotal,
	)
	return m
}

// GetChargeReportTotal 实现 bkv.MetricsAPI 接口（2025-10-31新增）
func (m *AppMetrics) GetChargeReportTotal() *prometheus.CounterVec {
	return m.ChargeReportTotal
}

// GetChargeReportPowerGauge 实现 bkv.MetricsAPI 接口
func (m *AppMetrics) GetChargeReportPowerGauge() *prometheus.GaugeVec {
	return m.ChargeReportPowerGauge
}

// GetChargeReportCurrentGauge 实现 bkv.MetricsAPI 接口
func (m *AppMetrics) GetChargeReportCurrentGauge() *prometheus.GaugeVec {
	return m.ChargeReportCurrentGauge
}

// GetChargeReportEnergyTotal 实现 bkv.MetricsAPI 接口
func (m *AppMetrics) GetChargeReportEnergyTotal() *prometheus.CounterVec {
	return m.ChargeReportEnergyTotal
}

// GetPortStatusQueryResponseTotal 实现 bkv.MetricsAPI 接口（P1-4端口状态查询响应）
func (m *AppMetrics) GetPortStatusQueryResponseTotal() *prometheus.CounterVec {
	return m.PortStatusQueryResponseTotal
}
