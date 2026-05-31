package engine

import (
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
    "sync"
    "sync/atomic"
    "time"
)

// Telemetry aggregates execution metrics for the engine. All fields are
// safe for concurrent access. Counters use atomic operations for efficiency;
// more complex structures (e.g., per‑analyzer maps) use a mutex.
type Telemetry struct {
    // Total number of tasks scheduled.
    totalTasks int64
    // Number of tasks completed successfully.
    succeeded int64
    // Number of tasks that returned an error (excluding panics).
    failed int64
    // Number of tasks that panicked.
    panics int64
    // Cumulative execution duration of all tasks in nanoseconds.
    totalDuration int64
    // Per‑analyzer execution counts and durations for detailed reporting.
    perAnalyzer map[string]*analyzerMetrics
    mu         sync.Mutex // guards perAnalyzer map
    // Persistence configuration
    persistEnabled bool
    filePath       string
}

type analyzerMetrics struct {
    count    int64
    duration int64 // nanoseconds
}

func (t *Telemetry) EnablePersistence(dir string) {
    // Determine if persistence should be enabled based on env or flags.
    // For simplicity, enable if REPO_LYZER_DEBUG is set to true.
    if os.Getenv("REPO_LYZER_DEBUG") != "true" && os.Getenv("REPO_LYZER_DEBUG") != "1" {
        // Not enabled; keep in‑memory only.
        t.persistEnabled = false
        return
    }
    // Ensure directory exists.
    if err := os.MkdirAll(dir, 0o755); err != nil {
        fmt.Fprintf(os.Stderr, "[engine] telemetry persistence disabled: %v\n", err)
        t.persistEnabled = false
        return
    }
    t.filePath = filepath.Join(dir, "telemetry.json")
    // Verify we can create the file (will truncate if exists).
    f, err := os.OpenFile(t.filePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
    if err != nil {
        fmt.Fprintf(os.Stderr, "[engine] telemetry persistence disabled: %v\n", err)
        t.persistEnabled = false
        return
    }
    f.Close()
    t.persistEnabled = true
}

func (t *Telemetry) Flush() error {
    if !t.persistEnabled {
        // No persistence requested.
        return nil
    }
    snap := t.Snapshot()
    data, err := json.MarshalIndent(snap, "", "  ")
    if err != nil {
        return err
    }
    // Write atomically.
    tmpPath := t.filePath + ".tmp"
    if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
        return err
    }
    // Rename into place.
    if err := os.Rename(tmpPath, t.filePath); err != nil {
        return err
    }
    return nil
}

// RecordStart increments the total task counter. It should be called
// immediately before a task is dispatched to a worker.
func (t *Telemetry) RecordStart() {
    atomic.AddInt64(&t.totalTasks, 1)
}

// RecordSuccess records a successful execution with the elapsed duration.
func (t *Telemetry) RecordSuccess(name string, d time.Duration) {
    atomic.AddInt64(&t.succeeded, 1)
    atomic.AddInt64(&t.totalDuration, d.Nanoseconds())
    t.mu.Lock()
    m, ok := t.perAnalyzer[name]
    if !ok {
        m = &analyzerMetrics{}
        t.perAnalyzer[name] = m
    }
    m.count++
    m.duration += d.Nanoseconds()
    t.mu.Unlock()
}

// RecordFailure records a task that returned an error (non‑panic).
func (t *Telemetry) RecordFailure(name string, d time.Duration) {
    atomic.AddInt64(&t.failed, 1)
    atomic.AddInt64(&t.totalDuration, d.Nanoseconds())
    t.mu.Lock()
    m, ok := t.perAnalyzer[name]
    if !ok {
        m = &analyzerMetrics{}
        t.perAnalyzer[name] = m
    }
    m.count++
    m.duration += d.Nanoseconds()
    t.mu.Unlock()
}

// RecordPanic records a task that panicked. The panic information is
// stored as an error string for later inspection.
func (t *Telemetry) RecordPanic(name string, d time.Duration, panicInfo any) {
    atomic.AddInt64(&t.panics, 1)
    atomic.AddInt64(&t.totalDuration, d.Nanoseconds())
    t.mu.Lock()
    m, ok := t.perAnalyzer[name]
    if !ok {
        m = &analyzerMetrics{}
        t.perAnalyzer[name] = m
    }
    m.count++
    m.duration += d.Nanoseconds()
    t.mu.Unlock()
    // Panic info could be logged elsewhere; telemetry keeps count.
}

// Snapshot returns a copy of current metrics for reporting.
func (t *Telemetry) Snapshot() TelemetrySnapshot {
    snap := TelemetrySnapshot{}
    snap.TotalTasks = atomic.LoadInt64(&t.totalTasks)
    snap.Succeeded = atomic.LoadInt64(&t.succeeded)
    snap.Failed = atomic.LoadInt64(&t.failed)
    snap.Panics = atomic.LoadInt64(&t.panics)
    snap.TotalDuration = time.Duration(atomic.LoadInt64(&t.totalDuration))
    t.mu.Lock()
    snap.PerAnalyzer = make(map[string]AnalyzerSnapshot, len(t.perAnalyzer))
    for name, m := range t.perAnalyzer {
        snap.PerAnalyzer[name] = AnalyzerSnapshot{Count: m.count, Duration: time.Duration(m.duration)}
    }
    t.mu.Unlock()
    return snap
}

// TelemetrySnapshot is a read‑only view of telemetry data.
type TelemetrySnapshot struct {
    TotalTasks    int64
    Succeeded     int64
    Failed        int64
    Panics        int64
    TotalDuration time.Duration
    PerAnalyzer   map[string]AnalyzerSnapshot
}

type AnalyzerSnapshot struct {
    Count    int64
    Duration time.Duration
}
