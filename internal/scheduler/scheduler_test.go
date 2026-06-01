package scheduler

import (
    "context"
    "errors"
    "testing"
    "time"

    "github.com/agnivo988/Repo-lyzer/internal/config"
    "github.com/agnivo988/Repo-lyzer/internal/monitor"
)

// Helper to reset metrics by capturing current values; subsequent checks use delta.
func captureMetrics() (uint64, uint64, uint64) {
    t, c, d := monitor.Snapshot()
    return t, c, d
}

func TestWorkerTimeoutHandling(t *testing.T) {
    s, err := NewScheduler()
    if err != nil {
        t.Fatalf("failed to create scheduler: %v", err)
    }
    // Use a very short timeout for the test.
    s.workerExecutionTimeout = 10 * time.Millisecond

    // Capture initial metric values.
    startTimeout, startCancelled, startDuration := captureMetrics()

    // Create a context that is already timed out.
    ctx, cancel := context.WithTimeout(context.Background(), 0)
    cancel()

    job := config.ScheduledJob{ID: "timeout-test"}
    err = s.runJobWithTimeout(ctx, job)
    if !errors.Is(err, context.DeadlineExceeded) {
        t.Fatalf("expected DeadlineExceeded error, got %v", err)
    }

    // Verify metrics have incremented.
    timeoutCount, cancelledCount, totalDuration := monitor.Snapshot()
    if timeoutCount != startTimeout+1 {
        t.Errorf("expected timeout count to increase, got %d (start %d)", timeoutCount, startTimeout)
    }
    if cancelledCount != startCancelled {
        t.Errorf("cancelled count should not change on timeout, got %d (start %d)", cancelledCount, startCancelled)
    }
    expectedDur := s.workerExecutionTimeout.Milliseconds()
    if totalDuration != startDuration+uint64(expectedDur) {
        t.Errorf("expected cumulative timeout duration to increase by %d ms, got %d (start %d)", expectedDur, totalDuration, startDuration)
    }
}

func TestSemaphoreReleaseAfterTimeout(t *testing.T) {
    s, err := NewScheduler()
    if err != nil {
        t.Fatalf("failed to create scheduler: %v", err)
    }
    s.workerExecutionTimeout = 10 * time.Millisecond

    // Acquire all worker slots.
    for i := 0; i < s.maxWorkers; i++ {
        if err := s.workerSem.Acquire(context.Background(), 1); err != nil {
            t.Fatalf("failed to acquire semaphore slot %d: %v", i, err)
        }
    }

    // Context already timed out.
    ctx, cancel := context.WithTimeout(context.Background(), 0)
    cancel()
    job := config.ScheduledJob{ID: "semaphore-test"}
    _ = s.runJobWithTimeout(ctx, job) // ignore error; focus on semaphore state.

    // After the call, one slot should be released, allowing acquisition.
    if err := s.workerSem.Acquire(context.Background(), 1); err != nil {
        t.Fatalf("semaphore slot was not released after timeout: %v", err)
    }
    // Release the slot we just acquired to avoid leaking for subsequent tests.
    s.workerSem.Release(1)
}

func TestSuccessfulWorkerExecution(t *testing.T) {
    s, err := NewScheduler()
    if err != nil {
        t.Fatalf("failed to create scheduler: %v", err)
    }
    // Set a generous timeout so it will not trigger.
    s.workerExecutionTimeout = 5 * time.Second

    startTimeout, startCancelled, _ := captureMetrics()

    // Use a background context that will not timeout.
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    // Create a job that is expected to fail quickly (missing repo) but should not hit timeout.
    job := config.ScheduledJob{ID: "quick-fail", Owner: "nonexistent", Repo: "repo", Enabled: true}
    // Directly call the timeout wrapper.
    err = s.runJobWithTimeout(ctx, job)
    // The error is expected because the job cannot be processed, but it should not be a timeout.
    if errors.Is(err, context.DeadlineExceeded) {
        t.Fatalf("unexpected timeout for quick job: %v", err)
    }

    // Verify that timeout metrics did not change.
    timeoutCount, cancelledCount, _ := monitor.Snapshot()
    if timeoutCount != startTimeout {
        t.Errorf("timeout count should remain unchanged, got %d (start %d)", timeoutCount, startTimeout)
    }
    if cancelledCount != startCancelled {
        t.Errorf("cancelled count should remain unchanged, got %d (start %d)", cancelledCount, startCancelled)
    }
}
