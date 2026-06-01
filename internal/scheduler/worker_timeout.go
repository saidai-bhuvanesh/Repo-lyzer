package scheduler

import (
    "context"
    "errors"
    "log"

    "github.com/agnivo988/Repo-lyzer/internal/config"
    "github.com/agnivo988/Repo-lyzer/internal/monitor"
)

// runJobWithTimeout executes a scheduled job with a bounded timeout.
// It guarantees semaphore release and WaitGroup cleanup even if the job times out or panics.
func (s *Scheduler) runJobWithTimeout(ctx context.Context, job config.ScheduledJob) error {
    // Acquire a worker slot (semaphore). Use a background context to avoid being blocked by the timeout.
    if err := s.workerSem.Acquire(context.Background(), 1); err != nil {
        return err
    }
    // Ensure the semaphore is released regardless of outcome.
    defer s.workerSem.Release(1)

    // Channel to capture executeJob result.
    resultCh := make(chan error, 1)
    go func() {
        // Recover from any panic to avoid crashing the scheduler.
        defer func() {
            if r := recover(); r != nil {
                log.Printf("panic in job %s: %v", job.ID, r)
                resultCh <- errors.New("panic in executeJob")
            }
        }()
        resultCh <- s.executeJob(job)
    }()

    select {
    case <-ctx.Done():
        // Timeout or cancellation occurred.
        if errors.Is(ctx.Err(), context.DeadlineExceeded) {
            log.Printf("scheduler worker timeout: job=%s timeout=%v", job.ID, s.workerExecutionTimeout)
            monitor.IncrementTimeout()
            monitor.AddTimeoutDuration(s.workerExecutionTimeout)
        } else {
            // Context cancelled (e.g., shutdown)
            monitor.IncrementCancelled()
        }
        return ctx.Err()
    case err := <-resultCh:
        return err
    }
}
