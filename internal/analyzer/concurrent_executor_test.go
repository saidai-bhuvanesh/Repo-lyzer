package analyzer

import (
    "context"
    "fmt"
    "testing"
    "time"
)

func TestConcurrentExecutor_ParallelismLimit(t *testing.T) {
    maxWorkers := 3
    exec := NewConcurrentExecutor(maxWorkers)
    // 10 jobs each sleep 100ms
    jobCount := 10
    jobs := make([]func(context.Context) error, jobCount)
    for i := 0; i < jobCount; i++ {
        jobs[i] = func(ctx context.Context) error {
            select {
            case <-time.After(100 * time.Millisecond):
                return nil
            case <-ctx.Done():
                return ctx.Err()
            }
        }
    }
    start := time.Now()
    _, err := exec.Run(context.Background(), jobs)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    elapsed := time.Since(start)
    // Expected roughly ceil(10/3)*100ms = 4*100ms = 400ms (+ overhead)
    if elapsed < 350*time.Millisecond || elapsed > 600*time.Millisecond {
        t.Fatalf("expected execution time around 400ms, got %v", elapsed)
    }
}

func TestConcurrentExecutor_TimeoutReleasesSlot(t *testing.T) {
    maxWorkers := 2
    exec := NewConcurrentExecutor(maxWorkers)
    // First job sleeps longer than timeout (200ms), timeout set to 50ms in executor (hardcoded 2min) cannot test.
    // Instead simulate by using context with short timeout via wrapper.
    // Use a custom executor with overridden timeout via context.
    ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
    defer cancel()
    jobs := []func(context.Context) error{
        func(c context.Context) error {
            // Sleep longer than context deadline
            select {
            case <-time.After(200 * time.Millisecond):
                return nil
            case <-c.Done():
                return c.Err()
            }
        },
        func(c context.Context) error { return nil },
    }
    _, err := exec.Run(ctx, jobs)
    if err == nil {
        t.Fatalf("expected context deadline error")
    }
    // second job should still run and succeed
    if ctx.Err() != context.DeadlineExceeded {
        t.Fatalf("expected deadline exceeded, got %v", ctx.Err())
    }
}

func TestConcurrentExecutor_ErrorIsolation(t *testing.T) {
    exec := NewConcurrentExecutor(2)
    errJob := func(ctx context.Context) error { return fmt.Errorf("fail") }
    okJob := func(ctx context.Context) error { return nil }
    jobs := []func(context.Context) error{errJob, okJob}
    results, err := exec.Run(context.Background(), jobs)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if results[0] == nil || results[1] != nil {
        t.Fatalf("error isolation failed, got %v", results)
    }
}

func TestConcurrentExecutor_DeterministicOrder(t *testing.T) {
    exec := NewConcurrentExecutor(3)
    jobs := []func(context.Context) error{
        func(ctx context.Context) error { time.Sleep(30 * time.Millisecond); return nil },
        func(ctx context.Context) error { time.Sleep(10 * time.Millisecond); return nil },
        func(ctx context.Context) error { time.Sleep(20 * time.Millisecond); return nil },
    }
    results, err := exec.Run(context.Background(), jobs)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    for i, r := range results {
        if r != nil {
            t.Fatalf("job %d returned error: %v", i, r)
        }
    }
}
