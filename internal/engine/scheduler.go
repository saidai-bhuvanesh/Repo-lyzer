package engine

import (
    "context"
    "fmt"
    "time"
)

// Scheduler controls rate‑limit aware execution. For now it provides a simple
// Acquire method that can be extended later. It tracks a simulated GitHub
// remaining‑quota counter.
type Scheduler struct {
    // remaining represents the number of API requests we think we can make.
    // In a real implementation this would be refreshed from GitHub headers.
    remaining int
    // mu protects remaining.
    mu       sync.Mutex
    // minInterval is the minimum pause between requests when quota is low.
    minInterval time.Duration
}

// NewScheduler creates a scheduler with a default large quota.
func NewScheduler() *Scheduler {
    return &Scheduler{remaining: 5000, minInterval: 10 * time.Millisecond}
}

// Acquire blocks until a token is available or the context is cancelled.
func (s *Scheduler) Acquire(ctx context.Context) error {
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        default:
        }
        s.mu.Lock()
        if s.remaining > 0 {
            s.remaining--
            s.mu.Unlock()
            return nil
        }
        s.mu.Unlock()
        // No quota left – wait a bit before retrying.
        select {
        case <-time.After(s.minInterval):
        case <-ctx.Done():
            return ctx.Err()
        }
    }
}

// Release adds back a token (e.g., after a successful request).
func (s *Scheduler) Release() {
    s.mu.Lock()
    s.remaining++
    s.mu.Unlock()
}

// SetRemaining allows external code (e.g., the GitHub client) to update the quota.
func (s *Scheduler) SetRemaining(n int) {
    s.mu.Lock()
    s.remaining = n
    s.mu.Unlock()
}

// Debug prints current remaining quota.
func (s *Scheduler) Debug() string {
    s.mu.Lock()
    defer s.mu.Unlock()
    return fmt.Sprintf("scheduler remaining quota: %d", s.remaining)
}
