package thirdparty

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// 推送指标

	// PushTotal 推送总数
	PushTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "thirdparty_push_total",
			Help: "Total number of event pushes to third party",
		},
		[]string{"event_type", "result"}, // result: success/failed/retry
	)

	// PushDuration 推送延迟
	PushDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "thirdparty_push_duration_seconds",
			Help:    "Duration of event push to third party in seconds",
			Buckets: prometheus.DefBuckets, // 默认bucket
		},
		[]string{"event_type"},
	)

	// PushRetryTotal 重试次数
	PushRetryTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "thirdparty_push_retry_total",
			Help: "Total number of event push retries",
		},
		[]string{"event_type"},
	)

	// QueueSize 队列长度
	QueueSize = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "thirdparty_queue_size",
			Help: "Current size of event queue",
		},
		[]string{"queue_type"}, // queue_type: main/dlq
	)

	// DedupHitTotal 去重命中次数
	DedupHitTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "thirdparty_dedup_hit_total",
			Help: "Total number of duplicate events detected",
		},
		[]string{"event_type"},
	)

	// EnqueueTotal 入队总数
	EnqueueTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "thirdparty_enqueue_total",
			Help: "Total number of events enqueued",
		},
		[]string{"event_type", "result"}, // result: success/failed
	)

	// DequeueTotal 出队总数
	DequeueTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "thirdparty_dequeue_total",
			Help: "Total number of events dequeued",
		},
		[]string{"event_type"},
	)

	// DLQMoveTotal 移入死信队列总数
	DLQMoveTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "thirdparty_dlq_move_total",
			Help: "Total number of events moved to DLQ",
		},
		[]string{"event_type", "reason"},
	)

	// API指标

	// APIRequestsTotal API请求总数
	APIRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "thirdparty_api_requests_total",
			Help: "Total number of third party API requests",
		},
		[]string{"endpoint", "method", "status"},
	)

	// APIDuration API响应延迟
	APIDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "thirdparty_api_duration_seconds",
			Help:    "Duration of third party API requests in seconds",
			Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10}, // 自定义bucket
		},
		[]string{"endpoint"},
	)

	// APIErrorTotal API错误总数
	APIErrorTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "thirdparty_api_error_total",
			Help: "Total number of third party API errors",
		},
		[]string{"endpoint", "error_type"},
	)

	// APIRateLimitHitTotal API限流命中次数
	APIRateLimitHitTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "thirdparty_api_rate_limit_hit_total",
			Help: "Total number of third party API rate limit hits",
		},
		[]string{"endpoint", "api_key"},
	)

	// APIAuthFailureTotal API认证失败次数
	APIAuthFailureTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "thirdparty_api_auth_failure_total",
			Help: "Total number of third party API authentication failures",
		},
		[]string{"endpoint", "failure_type"}, // failure_type: invalid_key/invalid_signature/expired
	)
)

// RecordPushSuccess 记录推送成功
func RecordPushSuccess(eventType EventType) {
	PushTotal.WithLabelValues(string(eventType), "success").Inc()
}

// RecordPushFailure 记录推送失败
func RecordPushFailure(eventType EventType) {
	PushTotal.WithLabelValues(string(eventType), "failed").Inc()
}

// RecordPushRetry 记录推送重试
func RecordPushRetry(eventType EventType) {
	PushTotal.WithLabelValues(string(eventType), "retry").Inc()
	PushRetryTotal.WithLabelValues(string(eventType)).Inc()
}

// RecordEnqueueSuccess 记录入队成功
func RecordEnqueueSuccess(eventType EventType) {
	EnqueueTotal.WithLabelValues(string(eventType), "success").Inc()
}

// RecordEnqueueFailure 记录入队失败
func RecordEnqueueFailure(eventType EventType) {
	EnqueueTotal.WithLabelValues(string(eventType), "failed").Inc()
}

// RecordDequeue 记录出队
func RecordDequeue(eventType EventType) {
	DequeueTotal.WithLabelValues(string(eventType)).Inc()
}

// RecordDLQMove 记录移入死信队列
func RecordDLQMove(eventType EventType, reason string) {
	DLQMoveTotal.WithLabelValues(string(eventType), reason).Inc()
}

// RecordDedupHit 记录去重命中
func RecordDedupHit(eventType EventType) {
	DedupHitTotal.WithLabelValues(string(eventType)).Inc()
}

// UpdateQueueSize 更新队列长度
func UpdateQueueSize(queueType string, size int64) {
	QueueSize.WithLabelValues(queueType).Set(float64(size))
}

// RecordAPIRequest 记录API请求
func RecordAPIRequest(endpoint, method, status string) {
	APIRequestsTotal.WithLabelValues(endpoint, method, status).Inc()
}

// RecordAPIError 记录API错误
func RecordAPIError(endpoint, errorType string) {
	APIErrorTotal.WithLabelValues(endpoint, errorType).Inc()
}

// RecordAPIRateLimitHit 记录API限流命中
func RecordAPIRateLimitHit(endpoint, apiKey string) {
	APIRateLimitHitTotal.WithLabelValues(endpoint, apiKey).Inc()
}

// RecordAPIAuthFailure 记录API认证失败
func RecordAPIAuthFailure(endpoint, failureType string) {
	APIAuthFailureTotal.WithLabelValues(endpoint, failureType).Inc()
}
