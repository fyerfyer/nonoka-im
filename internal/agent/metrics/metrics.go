// Package metrics holds the Prometheus collectors for the agent worker.
// Names follow the nonoka_<subsystem>_<name> style used by the gateway.
package metrics

import "github.com/prometheus/client_golang/prometheus"

type Metrics struct {
	Registry *prometheus.Registry

	RunsTotal              *prometheus.CounterVec
	RunDuration            prometheus.Histogram
	SweepsTotal            prometheus.Counter
	SweepDuration          prometheus.Histogram
	StaleRunsRequeuedTotal prometheus.Counter
	StaleRunsFailedTotal   prometheus.Counter
	QueueDepth             prometheus.Gauge
	ApprovalsTotal         *prometheus.CounterVec
	DispatchTotal          *prometheus.CounterVec
}

func New() *Metrics {
	m := &Metrics{
		Registry: prometheus.NewRegistry(),
		RunsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "nonoka", Subsystem: "agent_worker", Name: "runs_total",
			Help: "Total number of queued agent runs executed.",
		}, []string{"result"}),
		RunDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "nonoka", Subsystem: "agent_worker", Name: "run_duration_seconds",
			Help: "Duration of executing a queued agent run.", Buckets: prometheus.ExponentialBuckets(0.1, 2, 12),
		}),
		SweepsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "nonoka", Subsystem: "agent_worker", Name: "sweeps_total",
			Help: "Total number of sweeper passes.",
		}),
		SweepDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "nonoka", Subsystem: "agent_worker", Name: "sweep_duration_seconds",
			Help: "Duration of a sweep pass.", Buckets: prometheus.ExponentialBuckets(0.001, 2, 12),
		}),
		StaleRunsRequeuedTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "nonoka", Subsystem: "agent_worker", Name: "stale_runs_requeued_total",
			Help: "Total number of stale runs requeued by the sweeper.",
		}),
		StaleRunsFailedTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "nonoka", Subsystem: "agent_worker", Name: "stale_runs_failed_total",
			Help: "Total number of stale runs marked failed by the sweeper.",
		}),
		QueueDepth: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "nonoka", Subsystem: "agent_worker", Name: "queue_depth",
			Help: "Number of queued runs returned by the last replay poll.",
		}),
		ApprovalsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "nonoka", Subsystem: "agent_worker", Name: "approvals_total",
			Help: "Approval lifecycle transitions observed by the worker.",
		}, []string{"status"}),
		DispatchTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "nonoka", Subsystem: "agent_worker", Name: "dispatch_total",
			Help: "Dispatched agent-trigger events.",
		}, []string{"result"}),
	}
	m.Registry.MustRegister(m.RunsTotal, m.RunDuration, m.SweepsTotal, m.SweepDuration,
		m.StaleRunsRequeuedTotal, m.StaleRunsFailedTotal, m.QueueDepth, m.ApprovalsTotal, m.DispatchTotal)
	return m
}
