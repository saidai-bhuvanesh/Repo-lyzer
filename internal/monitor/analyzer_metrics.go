package monitor

import (
    "sync/atomic"
    "time"
)

// Analyzer telemetry counters (atomic for thread‑safety)
var (
    analyzerExecutionDurationMs uint64 // cumulative execution duration in milliseconds
    analyzerFailureCount        uint64 // total number of failed analyzer runs
    analyzerTimeoutCount        uint64 // total number of analyzer timeouts
)

// AnalyzerExecutionDurationMsAdd adds the given duration (in milliseconds) to the cumulative metric.
func AnalyzerExecutionDurationMsAdd(d time.Duration) {
    atomic.AddUint64(&analyzerExecutionDurationMs, uint64(d.Milliseconds()))
}

// AnalyzerFailureCountInc increments the failure counter.
func AnalyzerFailureCountInc() {
    atomic.AddUint64(&analyzerFailureCount, 1)
}

// AnalyzerTimeoutCountInc increments the timeout counter.
func AnalyzerTimeoutCountInc() {
    atomic.AddUint64(&analyzerTimeoutCount, 1)
}

// AnalyzerMetricsSnapshot returns a snapshot of the current analyzer telemetry values.
func AnalyzerMetricsSnapshot() (durationMs uint64, failureCount uint64, timeoutCount uint64) {
    return atomic.LoadUint64(&analyzerExecutionDurationMs),
        atomic.LoadUint64(&analyzerFailureCount),
        atomic.LoadUint64(&analyzerTimeoutCount)
}
