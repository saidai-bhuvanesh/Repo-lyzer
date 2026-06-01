package monitor

import (
    "sync/atomic"
    "time"
)

// Scheduler timeout telemetry counters (atomic for thread‑safety)
var (
    schedulerTimeoutCount      uint64 // total number of timed‑out worker executions
    schedulerCancelledJobs    uint64 // total number of jobs cancelled (e.g., via context)
    schedulerTimeoutDurationMs uint64 // cumulative timeout duration in milliseconds
)

// IncrementTimeout increments the timeout count.
func IncrementTimeout() {
    atomic.AddUint64(&schedulerTimeoutCount, 1)
}

// IncrementCancelled increments the cancelled jobs count.
func IncrementCancelled() {
    atomic.AddUint64(&schedulerCancelledJobs, 1)
}

// AddTimeoutDuration adds a duration (in ms) to the cumulative timeout duration metric.
func AddTimeoutDuration(d time.Duration) {
    atomic.AddUint64(&schedulerTimeoutDurationMs, uint64(d.Milliseconds()))
}

// Snapshot returns current metric values for monitoring/reporting.
func Snapshot() (timeoutCount uint64, cancelledCount uint64, totalDurationMs uint64) {
    return atomic.LoadUint64(&schedulerTimeoutCount), atomic.LoadUint64(&schedulerCancelledJobs), atomic.LoadUint64(&schedulerTimeoutDurationMs)
}
