package engine

import (
    "context"
    "fmt"
    "runtime"
    "sync"
    "time"
    "os"
    "path/filepath"
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

// Engine is the public entry point for the concurrent execution layer.
// It owns a worker pool, a scheduler (rate‑limit aware), and a telemetry collector.
type Engine struct {
    pool      *WorkerPool
    scheduler *Scheduler
    telemetry *Telemetry
    // internal wait group to coordinate shutdown
    wg sync.WaitGroup
}

// Option configures the Engine during creation.
type Option func(*Engine) error

// WithWorkers allows overriding the default worker count.
func WithWorkers(n int) Option {
    return func(e *Engine) error {
        if n <= 0 {
            return fmt.Errorf("worker count must be > 0")
        }
        e.pool = NewWorkerPool(n)
        return nil
    }
}

// WithScheduler injects a custom Scheduler (mostly for testing).
func WithScheduler(s *Scheduler) Option {
    return func(e *Engine) error { e.scheduler = s; return nil }
}

// WithTelemetry injects a telemetry collector.
func WithTelemetry(t *Telemetry) Option {
    return func(e *Engine) error { e.telemetry = t; return nil }
}

// NewEngine creates an Engine with sensible defaults.
func NewEngine(opts ...Option) (*Engine, error) {
    defaultWorkers := runtime.NumCPU()
    if defaultWorkers > 8 {
        defaultWorkers = 8
    }
    e := &Engine{
        pool:      NewWorkerPool(defaultWorkers),
        scheduler: NewScheduler(),
        telemetry: NewTelemetry(),
    }
    // Enable telemetry persistence based on environment flags.
    // Directory .repo-lyzer will be created if persistence is active.
    e.telemetry.EnablePersistence(filepath.Join(".", ".repo-lyzer"))
    for _, opt := range opts {
        if err := opt(e); err != nil {
            return nil, err
        }
    }
    return e, nil
}

// Run receives a slice of Analyzers, builds a DAG, and executes them concurrently.
// It returns a slice of Results preserving the order of input analyzers.
func (e *Engine) Run(ctx context.Context, analyzers []Analyzer) ([]Result, error) {
    // Build task map
    tasks := make(map[string]*Task)
    for _, a := range analyzers {
        tasks[a.Name()] = &Task{analyzer: a, timeout: 2 * time.Minute}
    }
    // Resolve dependencies into a DAG and schedule ready tasks.
    orchestrator := NewOrchestrator(e.pool, e.scheduler, e.telemetry)
    results, err := orchestrator.Execute(ctx, tasks)
    return results, err
}

// Shutdown gracefully stops the engine, its worker pool, and flushes telemetry.
func (e *Engine) Shutdown(ctx context.Context) error {
    // Stop the worker pool.
    if err := e.pool.Shutdown(ctx); err != nil {
        return err
    }
    // Flush telemetry metrics if persistence is enabled.
    if err := e.telemetry.Flush(); err != nil {
        // Log warning but do not fail shutdown.
        fmt.Fprintf(os.Stderr, "[engine] telemetry flush error: %v\n", err)
    }
    return nil
}
