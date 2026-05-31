package engine

import (
    "context"
    "fmt"
    "sync"
    "time"
)

// WorkerPool implements a bounded pool of goroutines that execute Tasks.
// The pool size is fixed at creation time. Tasks are submitted via Submit.
// The pool recovers from panics in workers and records the error.
type WorkerPool struct {
    workers   int
    taskCh    chan *Task
    wg        sync.WaitGroup
    shutdown  chan struct{}
    // Optional error handler can be set to capture panic errors.
    onError   func(taskName string, err error)
}

// NewWorkerPool creates a worker pool with the given size.
func NewWorkerPool(workers int) *WorkerPool {
    p := &WorkerPool{
        workers:  workers,
        taskCh:   make(chan *Task),
        shutdown: make(chan struct{}),
    }
    p.start()
    return p
}

// start launches the worker goroutines.
func (p *WorkerPool) start() {
    p.wg.Add(p.workers)
    for i := 0; i < p.workers; i++ {
        go func(id int) {
            defer p.wg.Done()
            for {
                select {
                case task := <-p.taskCh:
                    // Recover panics inside task execution.
                    func() {
                        defer func() {
                            if r := recover(); r != nil {
                                err := fmt.Errorf("panic in task %s: %v", task.Name, r)
                                if p.onError != nil {
                                    p.onError(task.Name, err)
                                }
                            }
                        }()
                        // Execute task function.
                        _ = task.Fn(context.Background()) // ignore error here; orchestrator handles result.
                    }()
                case <-p.shutdown:
                    return
                }
            }
        }(i)
    }
}

// Submit queues a task for execution.
func (p *WorkerPool) Submit(task *Task) error {
    select {
    case p.taskCh <- task:
        return nil
    case <-p.shutdown:
        return fmt.Errorf("worker pool is shutting down")
    }
}

// Shutdown gracefully stops all workers after completing queued tasks.
func (p *WorkerPool) Shutdown(ctx context.Context) error {
    close(p.shutdown)
    done := make(chan struct{})
    go func() {
        p.wg.Wait()
        close(done)
    }()
    select {
    case <-done:
        return nil
    case <-ctx.Done():
        return ctx.Err()
    }
}
