package workers

import "errors"

var (
	// ErrQueueFull is returned when a task cannot be submitted because the queue is full
	ErrQueueFull = errors.New("worker pool queue is full")

	// ErrPoolStopped is returned when attempting to submit to a stopped pool
	ErrPoolStopped = errors.New("worker pool is stopped")
)
