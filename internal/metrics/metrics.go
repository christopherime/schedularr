// Package metrics provides Prometheus metrics instrumentation for Schedularr.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Define Prometheus metrics
var (
	// SchedulesGeneratedTotal counts the total number of schedules generated
	SchedulesGeneratedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "schedularr_schedules_generated_total",
		Help: "Total number of schedules generated",
	}, []string{"channel_id", "block_name"})

	// ScheduleGenerationDurationSeconds measures the duration of schedule generation
	ScheduleGenerationDurationSeconds = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "schedularr_schedule_generation_duration_seconds",
		Help:    "Duration of schedule generation in seconds",
		Buckets: prometheus.DefBuckets,
	})

	// ScheduleErrorsTotal counts the total number of errors during schedule generation
	ScheduleErrorsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "schedularr_schedule_errors_total",
		Help: "Total number of errors during schedule generation",
	}, []string{"error_type"})

	// ProgramsScheduledTotal counts the total number of programs scheduled
	ProgramsScheduledTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "schedularr_programs_scheduled_total",
		Help: "Total number of programs scheduled",
	}, []string{"channel_id", "block_name", "program_type"})

	// SeriesStateUpdatesTotal counts the total number of series state updates
	SeriesStateUpdatesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "schedularr_series_state_updates_total",
		Help: "Total number of series state updates (e.g., episode progress)",
	}, []string{"show_title"})

	// ConflictsResolvedTotal counts the total number of scheduling conflicts resolved
	ConflictsResolvedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "schedularr_conflicts_resolved_total",
		Help: "Total number of scheduling conflicts resolved by priority",
	})

	// ScheduleFallbacksTotal counts fallback scenarios during schedule generation
	// Labels: channel_id, block_name, reason (history_exhausted, fallback_filter_error, fallback_no_content, filler_fetch_error, filler_empty)
	ScheduleFallbacksTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "schedularr_schedule_fallbacks_total",
		Help: "Total number of fallback scenarios during schedule generation",
	}, []string{"channel_id", "block_name", "reason"})

	// TunarrAPICallsTotal counts the total number of Tunarr API calls
	TunarrAPICallsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "schedularr_tunarr_api_calls_total",
		Help: "Total number of Tunarr API calls",
	}, []string{"endpoint", "method"})

	// TunarrAPICallDurationSeconds measures the duration of Tunarr API calls
	TunarrAPICallDurationSeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "schedularr_tunarr_api_call_duration_seconds",
		Help:    "Duration of Tunarr API calls in seconds",
		Buckets: prometheus.DefBuckets,
	}, []string{"endpoint", "method"})

	// TunarrAPIErrorsTotal counts the total number of errors during Tunarr API calls
	TunarrAPIErrorsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "schedularr_tunarr_api_errors_total",
		Help: "Total number of errors during Tunarr API calls",
	}, []string{"endpoint", "method", "error_type"})
)

// RegisterMetrics registers all custom metrics with the default Prometheus registry.
// This function is typically called once at application startup.
func RegisterMetrics() {
	// All metrics defined with promauto are automatically registered with the default registry.
	// This function serves as a conceptual placeholder if explicit registration were needed
	// or if there were other setup required for metrics.
}
