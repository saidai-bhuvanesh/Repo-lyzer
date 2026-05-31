package engine

import (
    "context"
    "time"
)

// Analyzer defines the contract that each analysis component must satisfy.
// Name returns a unique identifier used for logging, tracing and dependency resolution.
// Execute runs the analyzer logic within the provided context and returns a Result.
// Dependencies returns the names of other analyzers that must complete before this one runs.
type Analyzer interface {
    Name() string
    Execute(ctx context.Context) (Result, error)
    Dependencies() []string
}

// Result captures the outcome of a single analyzer execution.
type Result struct {
    Name     string                 // Analyzer name
    Score    float64                // Normalised score (0‑1)
    Category string                 // Optional category (e.g., "health", "risk")
    Metadata map[string]any         // Additional data for reporting
    Duration time.Duration          // Execution time
    Err      error                  // Non‑nil on failure or timeout
}

// Task represents a unit of work for the Engine. It wraps an Analyzer and
// includes execution metadata such as timeout. The Engine constructs these
// tasks from the provided Analyzer slice.
type Task struct {
    Analyzer Analyzer
    Timeout  time.Duration
    Retries  int
}

// NewTask creates a Task from an Analyzer with a default timeout and retry count.
func NewTask(a Analyzer, timeout time.Duration, retries int) *Task {
    return &Task{Analyzer: a, Timeout: timeout, Retries: retries}
}
