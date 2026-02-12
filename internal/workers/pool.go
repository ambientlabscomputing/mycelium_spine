package workers

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
)

// Task is the interface that all worker tasks must implement
type Task interface {
	Execute(ctx context.Context) error
	Type() string
}

// WorkerPool manages a pool of worker goroutines that process tasks from a queue
type WorkerPool struct {
	name          string
	size          int
	taskChan      chan Task
	wg            sync.WaitGroup
	ctx           context.Context
	cancel        context.CancelFunc
	tasksExecuted atomic.Int64
	taskErrors    atomic.Int64
	logger        *slog.Logger
}

// NewWorkerPool creates a new worker pool with the specified size and queue capacity
func NewWorkerPool(name string, size int, queueSize int, logger *slog.Logger) *WorkerPool {
	ctx, cancel := context.WithCancel(context.Background())
	return &WorkerPool{
		name:     name,
		size:     size,
		taskChan: make(chan Task, queueSize),
		ctx:      ctx,
		cancel:   cancel,
		logger:   logger.With("pool", name),
	}
}

// Start launches the worker goroutines in the pool
func (p *WorkerPool) Start(ctx context.Context) {
	p.logger.Info("starting worker pool", "size", p.size, "queue_size", cap(p.taskChan))
	for i := 0; i < p.size; i++ {
		p.wg.Add(1)
		go p.worker(i)
	}
}

// worker is the main loop for each worker goroutine
func (p *WorkerPool) worker(id int) {
	defer p.wg.Done()
	workerLogger := p.logger.With("worker_id", id)
	workerLogger.Debug("worker started")

	for {
		select {
		case <-p.ctx.Done():
			workerLogger.Debug("worker shutting down")
			return
		case task, ok := <-p.taskChan:
			if !ok {
				workerLogger.Debug("task channel closed, worker exiting")
				return
			}
			// Execute task
			taskLogger := workerLogger.With("task_type", task.Type())
			taskLogger.Debug("executing task")
			if err := task.Execute(p.ctx); err != nil {
				p.taskErrors.Add(1)
				taskLogger.Error("task execution failed", "error", err)
			} else {
				p.tasksExecuted.Add(1)
				taskLogger.Debug("task completed successfully")
			}
		}
	}
}

// Submit adds a task to the worker pool's queue
// Returns an error if the queue is full (non-blocking submission with backpressure)
func (p *WorkerPool) Submit(task Task) error {
	select {
	case p.taskChan <- task:
		return nil
	default:
		// Queue is full, apply backpressure
		p.logger.Warn("task queue full, rejecting task",
			"task_type", task.Type(),
			"queue_depth", len(p.taskChan),
			"queue_capacity", cap(p.taskChan))
		return ErrQueueFull
	}
}

// Stop gracefully shuts down the worker pool
// It waits for all in-flight tasks to complete but does not process queued tasks
func (p *WorkerPool) Stop() {
	p.logger.Info("stopping worker pool")
	// Signal all workers to stop
	p.cancel()
	// Close the task channel (workers will drain in-flight tasks)
	close(p.taskChan)
	// Wait for all workers to finish
	p.wg.Wait()
	p.logger.Info("worker pool stopped",
		"tasks_executed", p.tasksExecuted.Load(),
		"task_errors", p.taskErrors.Load())
}

// ActiveWorkers returns the configured number of workers in the pool
func (p *WorkerPool) ActiveWorkers() int {
	return p.size
}

// QueueDepth returns the current number of tasks waiting in the queue
func (p *WorkerPool) QueueDepth() int {
	return len(p.taskChan)
}

// QueueCapacity returns the maximum capacity of the task queue
func (p *WorkerPool) QueueCapacity() int {
	return cap(p.taskChan)
}

// TasksProcessed returns the total number of successfully executed tasks
func (p *WorkerPool) TasksProcessed() int64 {
	return p.tasksExecuted.Load()
}

// TaskErrors returns the total number of failed task executions
func (p *WorkerPool) TaskErrors() int64 {
	return p.taskErrors.Load()
}
