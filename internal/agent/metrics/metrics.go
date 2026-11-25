// Package metrics holds the Prometheus collectors for the agent worker.
// Names follow the nonoka_<subsystem>_<name> style used by the gateway.
package metrics

import (
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
)

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
	ToolCallsTotal         *prometheus.CounterVec
	ToolDuration           *prometheus.HistogramVec
	ModelTokensTotal       *prometheus.CounterVec
	EstimatedCostUSD       *prometheus.CounterVec
	ApprovalWait           prometheus.Histogram
	StreamEventsTotal      *prometheus.CounterVec
	QueueQueued            prometheus.Gauge
	QueueOldestAge         prometheus.Gauge
	QueueWait              prometheus.Histogram
	RunFailuresTotal       *prometheus.CounterVec
	MCPServerUp            *prometheus.GaugeVec
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
		ToolCallsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "nonoka", Subsystem: "agent", Name: "tool_calls_total",
			Help: "Agent tool calls by tool and terminal status.",
		}, []string{"tool", "status"}),
		ToolDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "nonoka", Subsystem: "agent", Name: "tool_duration_seconds",
			Help: "Agent tool execution duration.", Buckets: prometheus.ExponentialBuckets(0.001, 2, 14),
		}, []string{"tool", "status"}),
		ModelTokensTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "nonoka", Subsystem: "agent", Name: "tokens_total",
			Help: "Model tokens consumed by agent and model.",
		}, []string{"agent_id", "model", "direction"}),
		EstimatedCostUSD: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "nonoka", Subsystem: "agent", Name: "estimated_cost_usd_total",
			Help: "Estimated model cost by agent and model.",
		}, []string{"agent_id", "model"}),
		ApprovalWait: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "nonoka", Subsystem: "agent", Name: "approval_wait_seconds",
			Help: "Time from approval creation to decision or expiry.", Buckets: prometheus.ExponentialBuckets(1, 2, 12),
		}),
		StreamEventsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "nonoka", Subsystem: "agent", Name: "stream_events_total",
			Help: "Transient agent run stream events.",
		}, []string{"type"}),
		QueueQueued: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "nonoka", Subsystem: "agent", Name: "queue_queued",
			Help: "Queued agent runs waiting for a worker.",
		}),
		QueueOldestAge: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "nonoka", Subsystem: "agent", Name: "queue_oldest_age_seconds",
			Help: "Age of the oldest queued agent run.",
		}),
		QueueWait: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "nonoka", Subsystem: "agent", Name: "queue_wait_seconds",
			Help: "Queue wait from enqueue to claim.", Buckets: prometheus.ExponentialBuckets(0.01, 2, 14),
		}),
		RunFailuresTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "nonoka", Subsystem: "agent", Name: "run_failures_total",
			Help: "Terminal agent failures by class and code.",
		}, []string{"class", "code"}),
		MCPServerUp: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "nonoka", Subsystem: "agent", Name: "mcp_server_up",
			Help: "MCP server health (1 healthy, 0 unhealthy/needs_auth).",
		}, []string{"server"}),
	}
	m.Registry.MustRegister(m.RunsTotal, m.RunDuration, m.SweepsTotal, m.SweepDuration,
		m.StaleRunsRequeuedTotal, m.StaleRunsFailedTotal, m.QueueDepth, m.ApprovalsTotal, m.DispatchTotal,
		m.ToolCallsTotal, m.ToolDuration, m.ModelTokensTotal, m.EstimatedCostUSD, m.ApprovalWait,
		m.StreamEventsTotal, m.QueueQueued, m.QueueOldestAge, m.QueueWait, m.RunFailuresTotal,
		m.MCPServerUp)
	return m
}

// ObserveTool records one tool call. Labels are bounded tool names/statuses.
func (m *Metrics) ObserveTool(tool, status string, duration float64) {
	if m == nil {
		return
	}
	m.ToolCallsTotal.WithLabelValues(tool, status).Inc()
	m.ToolDuration.WithLabelValues(tool, status).Observe(duration)
}

// ObserveUsage records model tokens and estimated cost.
func (m *Metrics) ObserveUsage(agentID int64, model string, input, output int, cost float64) {
	if m == nil {
		return
	}
	agent := strconv.FormatInt(agentID, 10)
	m.ModelTokensTotal.WithLabelValues(agent, model, "input").Add(float64(input))
	m.ModelTokensTotal.WithLabelValues(agent, model, "output").Add(float64(output))
	m.EstimatedCostUSD.WithLabelValues(agent, model).Add(cost)
}

// ObserveApprovalWait records approval decision latency.
func (m *Metrics) ObserveApprovalWait(seconds float64) {
	if m == nil {
		return
	}
	m.ApprovalWait.Observe(seconds)
}

// ObserveStreamEvent records a transient run event type.
func (m *Metrics) ObserveStreamEvent(eventType string) {
	if m == nil {
		return
	}
	m.StreamEventsTotal.WithLabelValues(eventType).Inc()
}

// SetQueueStats updates queue depth and oldest age gauges.
func (m *Metrics) SetQueueStats(queued int64, oldestSeconds float64) {
	if m == nil {
		return
	}
	m.QueueQueued.Set(float64(queued))
	m.QueueOldestAge.Set(oldestSeconds)
}

// ObserveQueueWait records wait time from enqueue to claim.
func (m *Metrics) ObserveQueueWait(seconds float64) {
	if m == nil {
		return
	}
	m.QueueWait.Observe(seconds)
}

// SetMCPHealth records one MCP server's health gauge.
func (m *Metrics) SetMCPHealth(server, status string) {
	if m == nil {
		return
	}
	value := 0.0
	if status == "healthy" {
		value = 1
	}
	m.MCPServerUp.WithLabelValues(server).Set(value)
}

// ObserveRunFailure records a terminal failure class/code.
func (m *Metrics) ObserveRunFailure(class, code string) {
	if m == nil {
		return
	}
	m.RunFailuresTotal.WithLabelValues(class, code).Inc()
}
