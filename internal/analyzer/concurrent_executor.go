// Package analyzer provides concurrent execution utilities for analyzer jobs.
package analyzer

import (
    "context"
    "runtime"
    "sync"
    "time"

    "golang.org/x/sync/semaphore"
    "github.com/agnivo988/Repo-lyzer/internal/monitor"
)

// ConcurrentExecutor runs a slice of analyzer jobs with bounded concurrency.
// Each job receives a child context that can be timed out.
// Results are returned in the same order as the input slice.
type ConcurrentExecutor struct {
    maxWorkers int
    sem        *semaphore.Weighted
    wg         sync.WaitGroup
}

// NewConcurrentExecutor creates an executor with the given max workers.
// If maxWorkers <= 0 it defaults to runtime.NumCPU().
func NewConcurrentExecutor(maxWorkers int) *ConcurrentExecutor {
    if maxWorkers <= 0 {
        maxWorkers = runtime.NumCPU()
    }
    return &ConcurrentExecutor{
        maxWorkers: maxWorkers,
        sem:        semaphore.NewWeighted(int64(maxWorkers)),
    }
}

// Run executes each job function concurrently respecting the concurrency limit.
// The caller provides a base context; each job gets a child context with a timeout of 2 minutes.
// It returns a slice of errors aligned with the input order.
func (e *ConcurrentExecutor) Run(ctx context.Context, jobs []func(context.Context) error) ([]error, error) {
    results := make([]error, len(jobs))
    // Acquire workers and launch goroutine per job.
    for i, job := range jobs {
        // Acquire semaphore respecting cancellation.
        if err := e.sem.Acquire(ctx, 1); err != nil {
            // Context cancelled before acquiring; stop launching further jobs.
            return results, err
        }
        e.wg.Add(1)
        go func(idx int, fn func(context.Context) error) {
            defer e.sem.Release(1)
            defer e.wg.Done()
            // Per‑job timeout.
            jobCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
            start := time.Now()
            err := fn(jobCtx)
            durationMs := time.Since(start).Milliseconds()
            // Record telemetry.
            monitor.AnalyzerExecutionDurationMs.Add(durationMs)
            if err != nil {
                if err == context.DeadlineExceeded {
                    monitor.AnalyzerTimeoutCount.Inc()
                } else {
                    monitor.AnalyzerFailureCount.Inc()
                }
            }
            results[idx] = err
            cancel()
        }(i, job)
    }
    // Wait for all launched jobs.
    e.wg.Wait()
    return results, nil
}

// Ensure the monitor package exposes the counters.
var _ = monitor.AnalyzerExecutionDurationMs
