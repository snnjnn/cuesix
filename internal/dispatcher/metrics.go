package dispatcher

import "github.com/prometheus/client_golang/prometheus"

const (
	metricsNamespace = "sixpack"
	metricsSubsystem = "dispatcher"
)

var (
	dispatcherEnqueued = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: metricsNamespace,
		Subsystem: metricsSubsystem,
		Name:      "enqueued_total",
		Help:      "Total number of compile requests enqueued.",
	})
	dispatcherDropped = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: metricsNamespace,
		Subsystem: metricsSubsystem,
		Name:      "dropped_total",
		Help:      "Total number of compile requests dropped due to a full queue.",
	})
	dispatcherDequeued = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: metricsNamespace,
		Subsystem: metricsSubsystem,
		Name:      "dequeued_total",
		Help:      "Total number of compile requests dequeued.",
	})
	dispatcherSkipped = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: metricsNamespace,
		Subsystem: metricsSubsystem,
		Name:      "skipped_total",
		Help:      "Total number of runs skipped due to no changes.",
	})
	dispatcherErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: metricsNamespace,
		Subsystem: metricsSubsystem,
		Name:      "errors_total",
		Help:      "Total number of pipeline errors by stage.",
	}, []string{"stage"})
	dispatcherDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: metricsNamespace,
		Subsystem: metricsSubsystem,
		Name:      "stage_duration_seconds",
		Help:      "Time spent in each pipeline stage.",
		Buckets:   prometheus.DefBuckets,
	}, []string{"stage"})
)

func init() {
	prometheus.MustRegister(
		dispatcherEnqueued,
		dispatcherDropped,
		dispatcherDequeued,
		dispatcherSkipped,
		dispatcherErrors,
		dispatcherDuration,
	)
}
