package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds Prometheus metrics for Gateway and MsgWorker.
// It is designed to be passed as an optional dependency (nil-safe) so that
// existing tests and callers are not broken.
type Metrics struct {
	registry *prometheus.Registry

	// Gateway connection metrics.
	ActiveConnections prometheus.Gauge
	ConnectionsTotal  prometheus.Counter

	// Gateway publish metrics.
	MessagesPublished prometheus.Counter
	PublishLatency    prometheus.Histogram

	// Gateway push metrics.
	MessagesPushed prometheus.Counter
	PushLatency    prometheus.Histogram

	// MsgWorker consumption metrics.
	MessagesConsumed  prometheus.Counter
	MessagesProcessed prometheus.Counter
	MessagesDuplicate prometheus.Counter
	ProcessingLatency prometheus.Histogram

	// MsgWorker MongoDB metrics.
	MongoOpsTotal *prometheus.CounterVec

	// MsgWorker push metrics.
	PushAttemptsTotal prometheus.Counter
	PushFailedTotal   prometheus.Counter
	PushRetryTotal    prometheus.Counter
	GrpcPushLatency   prometheus.Histogram
}

// NewMetrics creates a new Metrics instance with a dedicated registry.
func NewMetrics() *Metrics {
	registry := prometheus.NewRegistry()

	m := &Metrics{
		registry: registry,
		ActiveConnections: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "nonoka",
			Subsystem: "gateway",
			Name:      "active_connections",
			Help:      "Number of active WebSocket connections.",
		}),
		ConnectionsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "nonoka",
			Subsystem: "gateway",
			Name:      "connections_total",
			Help:      "Total number of WebSocket connections accepted.",
		}),
		MessagesPublished: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "nonoka",
			Subsystem: "gateway",
			Name:      "messages_published_total",
			Help:      "Total number of messages published by clients.",
		}),
		PublishLatency: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "nonoka",
			Subsystem: "gateway",
			Name:      "publish_latency_seconds",
			Help:      "Latency of client publish to Kafka ACK.",
			Buckets:   prometheus.ExponentialBuckets(0.001, 2, 16),
		}),
		MessagesPushed: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "nonoka",
			Subsystem: "gateway",
			Name:      "messages_pushed_total",
			Help:      "Total number of messages pushed to online users.",
		}),
		PushLatency: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "nonoka",
			Subsystem: "gateway",
			Name:      "push_latency_seconds",
			Help:      "Latency of pushing a message to online users.",
			Buckets:   prometheus.ExponentialBuckets(0.0001, 2, 16),
		}),
		MessagesConsumed: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "nonoka",
			Subsystem: "msgworker",
			Name:      "messages_consumed_total",
			Help:      "Total number of messages consumed from Kafka.",
		}),
		MessagesProcessed: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "nonoka",
			Subsystem: "msgworker",
			Name:      "messages_processed_total",
			Help:      "Total number of messages successfully processed.",
		}),
		MessagesDuplicate: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "nonoka",
			Subsystem: "msgworker",
			Name:      "messages_duplicate_total",
			Help:      "Total number of duplicate messages ignored.",
		}),
		ProcessingLatency: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "nonoka",
			Subsystem: "msgworker",
			Name:      "processing_latency_seconds",
			Help:      "Latency of processing a Kafka message end-to-end.",
			Buckets:   prometheus.ExponentialBuckets(0.001, 2, 16),
		}),
		MongoOpsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "nonoka",
			Subsystem: "msgworker",
			Name:      "mongo_ops_total",
			Help:      "Total number of MongoDB operations by collection and operation.",
		}, []string{"collection", "op"}),
		PushAttemptsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "nonoka",
			Subsystem: "msgworker",
			Name:      "push_attempts_total",
			Help:      "Total number of push attempts to online users.",
		}),
		PushFailedTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "nonoka",
			Subsystem: "msgworker",
			Name:      "push_failed_total",
			Help:      "Total number of failed push attempts.",
		}),
		PushRetryTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "nonoka",
			Subsystem: "msgworker",
			Name:      "push_retry_total",
			Help:      "Total number of push retries scheduled.",
		}),
		GrpcPushLatency: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "nonoka",
			Subsystem: "msgworker",
			Name:      "grpc_push_latency_seconds",
			Help:      "Latency of gRPC push from msgworker to gateway.",
			Buckets:   prometheus.ExponentialBuckets(0.0001, 2, 16),
		}),
	}

	registry.MustRegister(
		m.ActiveConnections,
		m.ConnectionsTotal,
		m.MessagesPublished,
		m.PublishLatency,
		m.MessagesPushed,
		m.PushLatency,
		m.MessagesConsumed,
		m.MessagesProcessed,
		m.MessagesDuplicate,
		m.ProcessingLatency,
		m.MongoOpsTotal,
		m.PushAttemptsTotal,
		m.PushFailedTotal,
		m.PushRetryTotal,
		m.GrpcPushLatency,
	)

	return m
}

// Handler returns an HTTP handler that exposes Prometheus metrics.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

// IncConnectionsTotal increments the total connections counter if metrics is non-nil.
func (m *Metrics) IncConnectionsTotal() {
	if m != nil && m.ConnectionsTotal != nil {
		m.ConnectionsTotal.Inc()
	}
}

// SetActiveConnections sets the active connections gauge if metrics is non-nil.
func (m *Metrics) SetActiveConnections(v float64) {
	if m != nil && m.ActiveConnections != nil {
		m.ActiveConnections.Set(v)
	}
}

// AddActiveConnections adds delta to the active connections gauge if metrics is non-nil.
func (m *Metrics) AddActiveConnections(v float64) {
	if m != nil && m.ActiveConnections != nil {
		m.ActiveConnections.Add(v)
	}
}

// ObservePublishLatency observes publish latency if metrics is non-nil.
func (m *Metrics) ObservePublishLatency(v float64) {
	if m != nil && m.PublishLatency != nil {
		m.PublishLatency.Observe(v)
	}
}

// IncMessagesPublished increments the published messages counter if metrics is non-nil.
func (m *Metrics) IncMessagesPublished() {
	if m != nil && m.MessagesPublished != nil {
		m.MessagesPublished.Inc()
	}
}

// ObservePushLatency observes push latency if metrics is non-nil.
func (m *Metrics) ObservePushLatency(v float64) {
	if m != nil && m.PushLatency != nil {
		m.PushLatency.Observe(v)
	}
}

// IncMessagesPushed increments the pushed messages counter if metrics is non-nil.
func (m *Metrics) IncMessagesPushed() {
	if m != nil && m.MessagesPushed != nil {
		m.MessagesPushed.Inc()
	}
}

// AddMessagesPushed adds delta to the pushed messages counter if metrics is non-nil.
func (m *Metrics) AddMessagesPushed(v float64) {
	if m != nil && m.MessagesPushed != nil {
		m.MessagesPushed.Add(v)
	}
}

// IncMessagesConsumed increments the consumed messages counter if metrics is non-nil.
func (m *Metrics) IncMessagesConsumed() {
	if m != nil && m.MessagesConsumed != nil {
		m.MessagesConsumed.Inc()
	}
}

// IncMessagesProcessed increments the processed messages counter if metrics is non-nil.
func (m *Metrics) IncMessagesProcessed() {
	if m != nil && m.MessagesProcessed != nil {
		m.MessagesProcessed.Inc()
	}
}

// IncMessagesDuplicate increments the duplicate messages counter if metrics is non-nil.
func (m *Metrics) IncMessagesDuplicate() {
	if m != nil && m.MessagesDuplicate != nil {
		m.MessagesDuplicate.Inc()
	}
}

// ObserveProcessingLatency observes message processing latency if metrics is non-nil.
func (m *Metrics) ObserveProcessingLatency(v float64) {
	if m != nil && m.ProcessingLatency != nil {
		m.ProcessingLatency.Observe(v)
	}
}

// IncMongoOps increments the MongoDB operations counter if metrics is non-nil.
func (m *Metrics) IncMongoOps(collection, op string) {
	if m != nil && m.MongoOpsTotal != nil {
		m.MongoOpsTotal.WithLabelValues(collection, op).Inc()
	}
}

// IncPushAttempts increments the push attempts counter if metrics is non-nil.
func (m *Metrics) IncPushAttempts() {
	if m != nil && m.PushAttemptsTotal != nil {
		m.PushAttemptsTotal.Inc()
	}
}

// IncPushFailed increments the push failed counter if metrics is non-nil.
func (m *Metrics) IncPushFailed() {
	if m != nil && m.PushFailedTotal != nil {
		m.PushFailedTotal.Inc()
	}
}

// IncPushRetry increments the push retry counter if metrics is non-nil.
func (m *Metrics) IncPushRetry() {
	if m != nil && m.PushRetryTotal != nil {
		m.PushRetryTotal.Inc()
	}
}

// ObserveGrpcPushLatency observes gRPC push latency if metrics is non-nil.
func (m *Metrics) ObserveGrpcPushLatency(v float64) {
	if m != nil && m.GrpcPushLatency != nil {
		m.GrpcPushLatency.Observe(v)
	}
}
