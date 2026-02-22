package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all Prometheus metrics for UMS
type Metrics struct {
	// Sessions
	SessionsActive  prometheus.Gauge
	ReconnectsTotal prometheus.Counter

	// Delivery latency (histogram)
	DeliveryLatency prometheus.Histogram

	// Inflight envelopes by QoS
	InflightCommand   prometheus.Gauge
	InflightControl   prometheus.Gauge
	InflightTelemetry prometheus.Gauge

	// Mailbox metrics
	MailboxBacklog *prometheus.GaugeVec // mailbox_id label
	AckLag         *prometheus.GaugeVec // mailbox_id label

	// Lost envelopes
	TelemetryDropped prometheus.Counter
	CommandsExpired  prometheus.Counter

	// gRPC
	RPCsProcessed  *prometheus.CounterVec   // method label
	RPCErrorsTotal *prometheus.CounterVec   // method, code labels
	RPCDuration    *prometheus.HistogramVec // method label (latency)

	// Storage
	MongoDBOperations *prometheus.CounterVec   // operation, status labels
	MongoDBLatency    *prometheus.HistogramVec // operation label
}

// NewMetrics creates and registers all metrics
func NewMetrics() *Metrics {
	return &Metrics{
		// Sessions
		SessionsActive: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "ums_sessions_active",
			Help: "Number of active client sessions",
		}),

		ReconnectsTotal: promauto.NewCounter(prometheus.CounterOpts{
			Name: "ums_reconnects_total",
			Help: "Total number of client reconnections",
		}),

		// Delivery latency (from publish to delivery, in seconds)
		DeliveryLatency: promauto.NewHistogram(prometheus.HistogramOpts{
			Name:    "ums_delivery_latency_seconds",
			Help:    "Envelope delivery latency from publish to client delivery",
			Buckets: []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1.0, 5.0},
		}),

		// Inflight envelopes
		InflightCommand: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "ums_inflight_command",
			Help: "Current number of inflight COMMAND envelopes",
		}),

		InflightControl: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "ums_inflight_control",
			Help: "Current number of inflight CONTROL envelopes",
		}),

		InflightTelemetry: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "ums_inflight_telemetry",
			Help: "Current number of inflight TELEMETRY envelopes",
		}),

		// Mailbox metrics
		MailboxBacklog: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "ums_mailbox_backlog",
			Help: "Number of unacked envelopes per mailbox",
		}, []string{"mailbox_id"}),

		AckLag: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "ums_ack_lag_sequences",
			Help: "Number of sequences between last appended and last acked per mailbox",
		}, []string{"mailbox_id"}),

		// Lost envelopes
		TelemetryDropped: promauto.NewCounter(prometheus.CounterOpts{
			Name: "ums_telemetry_dropped_total",
			Help: "Total number of TELEMETRY envelopes dropped due to backpressure",
		}),

		CommandsExpired: promauto.NewCounter(prometheus.CounterOpts{
			Name: "ums_commands_expired_total",
			Help: "Total number of COMMAND envelopes expired before delivery",
		}),

		// gRPC
		RPCsProcessed: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "ums_grpc_rpcs_processed_total",
			Help: "Total number of gRPC RPCs processed",
		}, []string{"method"}),

		RPCErrorsTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "ums_grpc_errors_total",
			Help: "Total number of gRPC errors by method and code",
		}, []string{"method", "code"}),

		RPCDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "ums_grpc_duration_seconds",
			Help:    "gRPC RPC duration in seconds",
			Buckets: prometheus.ExponentialBuckets(0.001, 10, 5),
		}, []string{"method"}),

		// MongoDB operations
		MongoDBOperations: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "ums_mongodb_operations_total",
			Help: "Total MongoDB operations by type and status",
		}, []string{"operation", "status"}),

		MongoDBLatency: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "ums_mongodb_latency_seconds",
			Help:    "MongoDB operation latency in seconds",
			Buckets: []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1.0},
		}, []string{"operation"}),
	}
}

// Snapshot returns current metric values as strings for logging/debugging
func (m *Metrics) Snapshot() map[string]interface{} {
	return map[string]interface{}{
		"sessions_active":    m.SessionsActive,
		"reconnects_total":   "N/A", // Can't get counter value directly
		"delivery_latency":   "histogram",
		"inflight_command":   m.InflightCommand,
		"inflight_control":   m.InflightControl,
		"inflight_telemetry": m.InflightTelemetry,
	}
}
