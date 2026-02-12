package workers

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Mock task for testing
type mockTask struct {
	taskType  string
	delay     time.Duration
	shouldErr bool
	executed  atomic.Bool
}

func (m *mockTask) Execute(ctx context.Context) error {
	if m.delay > 0 {
		time.Sleep(m.delay)
	}
	m.executed.Store(true)
	if m.shouldErr {
		return errors.New("mock task error")
	}
	return nil
}

func (m *mockTask) Type() string {
	return m.taskType
}

func TestNewWorkerPool(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	pool := NewWorkerPool("test-pool", 4, 100, logger)

	if pool == nil {
		t.Fatal("expected non-nil pool")
	}

	if pool.name != "test-pool" {
		t.Errorf("expected name 'test-pool', got %s", pool.name)
	}

	if pool.size != 4 {
		t.Errorf("expected size 4, got %d", pool.size)
	}

	if cap(pool.taskChan) != 100 {
		t.Errorf("expected queue capacity 100, got %d", cap(pool.taskChan))
	}
}

func TestWorkerPoolSubmit(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	pool := NewWorkerPool("test-pool", 2, 10, logger)
	ctx := context.Background()
	pool.Start(ctx)
	defer pool.Stop()

	task := &mockTask{taskType: "test", delay: 10 * time.Millisecond}
	err := pool.Submit(task)
	if err != nil {
		t.Errorf("unexpected error submitting task: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	if !task.executed.Load() {
		t.Error("task was not executed")
	}

	if pool.TasksProcessed() != 1 {
		t.Errorf("expected 1 task processed, got %d", pool.TasksProcessed())
	}
}

func TestWorkerPoolBackpressure(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	pool := NewWorkerPool("test-pool", 1, 2, logger)
	ctx := context.Background()
	pool.Start(ctx)
	defer pool.Stop()

	// With 1 worker and queue size 2:
	// - Task 1: Picked up by worker
	// - Task 2: Goes into queue (queue depth: 1)
	// - Task 3: Goes into queue (queue depth: 2, queue full)
	// - Task 4: Should fail with ErrQueueFull
	slowTask1 := &mockTask{taskType: "slow1", delay: 300 * time.Millisecond}
	slowTask2 := &mockTask{taskType: "slow2", delay: 300 * time.Millisecond}
	slowTask3 := &mockTask{taskType: "slow3", delay: 300 * time.Millisecond}
	fastTask := &mockTask{taskType: "fast", delay: 10 * time.Millisecond}

	if err := pool.Submit(slowTask1); err != nil {
		t.Fatalf("failed to submit slow task 1: %v", err)
	}
	
	// Give worker time to pick up first task
	time.Sleep(50 * time.Millisecond)
	
	// Now submit 2 more to fill the queue
	if err := pool.Submit(slowTask2); err != nil {
		t.Fatalf("failed to submit slow task 2: %v", err)
	}
	if err := pool.Submit(slowTask3); err != nil {
		t.Fatalf("failed to submit slow task 3: %v", err)
	}

	// Queue is now full, this should fail
	err := pool.Submit(fastTask)
	if err != ErrQueueFull {
		t.Errorf("expected ErrQueueFull, got %v", err)
	}
}

func TestWorkerPoolTaskError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	pool := NewWorkerPool("test-pool", 2, 10, logger)
	ctx := context.Background()
	pool.Start(ctx)
	defer pool.Stop()

	errorTask := &mockTask{taskType: "error-task", shouldErr: true}
	err := pool.Submit(errorTask)
	if err != nil {
		t.Errorf("unexpected error submitting task: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	if !errorTask.executed.Load() {
		t.Error("error task was not executed")
	}

	if pool.TaskErrors() != 1 {
		t.Errorf("expected 1 task error, got %d", pool.TaskErrors())
	}
}

func TestWorkerPoolConcurrency(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	pool := NewWorkerPool("test-pool", 4, 100, logger)
	ctx := context.Background()
	pool.Start(ctx)
	defer pool.Stop()

	numTasks := 50
	var wg sync.WaitGroup

	for i := 0; i < numTasks; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			task := &mockTask{taskType: "concurrent", delay: 5 * time.Millisecond}
			_ = pool.Submit(task)
		}()
	}

	wg.Wait()
	time.Sleep(500 * time.Millisecond)

	processed := pool.TasksProcessed()
	if processed < int64(numTasks)/2 {
		t.Errorf("expected at least %d tasks processed, got %d", numTasks/2, processed)
	}
}

func TestWorkerPoolMetrics(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	pool := NewWorkerPool("test-pool", 3, 20, logger)

	if pool.ActiveWorkers() != 3 {
		t.Errorf("expected 3 active workers, got %d", pool.ActiveWorkers())
	}

	if pool.QueueCapacity() != 20 {
		t.Errorf("expected queue capacity 20, got %d", pool.QueueCapacity())
	}

	ctx := context.Background()
	pool.Start(ctx)
	defer pool.Stop()

	for i := 0; i < 5; i++ {
		task := &mockTask{taskType: "metric-test", delay: 10 * time.Millisecond}
		if err := pool.Submit(task); err != nil {
			t.Errorf("failed to submit task: %v", err)
		}
	}

	time.Sleep(200 * time.Millisecond)

	depth := pool.QueueDepth()
	if depth < 0 || depth > 5 {
		t.Errorf("unexpected queue depth: %d", depth)
	}

	processed := pool.TasksProcessed()
	if processed != 5 {
		t.Errorf("expected 5 tasks processed, got %d", processed)
	}

	if pool.TaskErrors() != 0 {
		t.Errorf("expected 0 task errors, got %d", pool.TaskErrors())
	}
}