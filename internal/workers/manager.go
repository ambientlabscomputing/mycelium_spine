package workers

import (
	"context"
	"log/slog"

	"github.com/ambientlabscomputing/mycelium_spine/internal/utils"
)

// PoolManager manages all worker pools in the UMS system
type PoolManager struct {
	sessionPool  *WorkerPool
	deliveryPool *WorkerPool
	ackPool      *WorkerPool
	mailboxPool  *WorkerPool
	logger       *slog.Logger
}

// NewPoolManager creates a new pool manager with pools configured from settings
func NewPoolManager(settings *utils.Settings, logger *slog.Logger) *PoolManager {
	return &PoolManager{
		sessionPool: NewWorkerPool(
			"session",
			settings.Workers.SessionPoolSize,
			settings.Workers.TaskQueueSize,
			logger,
		),
		deliveryPool: NewWorkerPool(
			"delivery",
			settings.Workers.DeliveryPoolSize,
			settings.Workers.TaskQueueSize,
			logger,
		),
		ackPool: NewWorkerPool(
			"ack",
			settings.Workers.AckPoolSize,
			settings.Workers.TaskQueueSize,
			logger,
		),
		mailboxPool: NewWorkerPool(
			"mailbox",
			settings.Workers.MailboxPoolSize,
			settings.Workers.TaskQueueSize,
			logger,
		),
		logger: logger,
	}
}

// Start starts all worker pools
func (m *PoolManager) Start(ctx context.Context) {
	m.logger.Info("starting pool manager")
	m.sessionPool.Start(ctx)
	m.deliveryPool.Start(ctx)
	m.ackPool.Start(ctx)
	m.mailboxPool.Start(ctx)
	m.logger.Info("all worker pools started")
}

// Stop gracefully stops all worker pools
func (m *PoolManager) Stop() {
	m.logger.Info("stopping pool manager")
	m.sessionPool.Stop()
	m.deliveryPool.Stop()
	m.ackPool.Stop()
	m.mailboxPool.Stop()
	m.logger.Info("all worker pools stopped")
}

// SubmitSessionTask submits a task to the session worker pool
func (m *PoolManager) SubmitSessionTask(task Task) error {
	return m.sessionPool.Submit(task)
}

// SubmitDeliveryTask submits a task to the delivery worker pool
func (m *PoolManager) SubmitDeliveryTask(task Task) error {
	return m.deliveryPool.Submit(task)
}

// SubmitAckTask submits a task to the ack worker pool
func (m *PoolManager) SubmitAckTask(task Task) error {
	return m.ackPool.Submit(task)
}

// SubmitMailboxTask submits a task to the mailbox worker pool
func (m *PoolManager) SubmitMailboxTask(task Task) error {
	return m.mailboxPool.Submit(task)
}

// GetMetrics returns metrics for all worker pools
func (m *PoolManager) GetMetrics() map[string]PoolMetrics {
	return map[string]PoolMetrics{
		"session": {
			ActiveWorkers:  m.sessionPool.ActiveWorkers(),
			QueueDepth:     m.sessionPool.QueueDepth(),
			QueueCapacity:  m.sessionPool.QueueCapacity(),
			TasksProcessed: m.sessionPool.TasksProcessed(),
			TaskErrors:     m.sessionPool.TaskErrors(),
		},
		"delivery": {
			ActiveWorkers:  m.deliveryPool.ActiveWorkers(),
			QueueDepth:     m.deliveryPool.QueueDepth(),
			QueueCapacity:  m.deliveryPool.QueueCapacity(),
			TasksProcessed: m.deliveryPool.TasksProcessed(),
			TaskErrors:     m.deliveryPool.TaskErrors(),
		},
		"ack": {
			ActiveWorkers:  m.ackPool.ActiveWorkers(),
			QueueDepth:     m.ackPool.QueueDepth(),
			QueueCapacity:  m.ackPool.QueueCapacity(),
			TasksProcessed: m.ackPool.TasksProcessed(),
			TaskErrors:     m.ackPool.TaskErrors(),
		},
		"mailbox": {
			ActiveWorkers:  m.mailboxPool.ActiveWorkers(),
			QueueDepth:     m.mailboxPool.QueueDepth(),
			QueueCapacity:  m.mailboxPool.QueueCapacity(),
			TasksProcessed: m.mailboxPool.TasksProcessed(),
			TaskErrors:     m.mailboxPool.TaskErrors(),
		},
	}
}

// PoolMetrics contains metrics for a single worker pool
type PoolMetrics struct {
	ActiveWorkers  int
	QueueDepth     int
	QueueCapacity  int
	TasksProcessed int64
	TaskErrors     int64
}
