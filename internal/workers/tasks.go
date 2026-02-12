package workers

import (
	"context"
	"fmt"
)

// TaskType constants for identifying task categories
const (
	TaskTypeSession  = "session"
	TaskTypeDelivery = "delivery"
	TaskTypeAck      = "ack"
	TaskTypeMailbox  = "mailbox"
)

// SessionTask handles HELLO processing, session creation, and resume validation
type SessionTask struct {
	taskType string
	handler  func(ctx context.Context) error
}

// NewSessionTask creates a new session task
func NewSessionTask(handler func(ctx context.Context) error) *SessionTask {
	return &SessionTask{
		taskType: TaskTypeSession,
		handler:  handler,
	}
}

func (t *SessionTask) Execute(ctx context.Context) error {
	if t.handler == nil {
		return fmt.Errorf("session task has no handler")
	}
	return t.handler(ctx)
}

func (t *SessionTask) Type() string {
	return t.taskType
}

// DeliveryTask pushes envelopes from mailbox to active session streams
type DeliveryTask struct {
	taskType string
	handler  func(ctx context.Context) error
}

// NewDeliveryTask creates a new delivery task
func NewDeliveryTask(handler func(ctx context.Context) error) *DeliveryTask {
	return &DeliveryTask{
		taskType: TaskTypeDelivery,
		handler:  handler,
	}
}

func (t *DeliveryTask) Execute(ctx context.Context) error {
	if t.handler == nil {
		return fmt.Errorf("delivery task has no handler")
	}
	return t.handler(ctx)
}

func (t *DeliveryTask) Type() string {
	return t.taskType
}

// AckTask processes ACK/ACK_SET frames and advances mailbox cursors
type AckTask struct {
	taskType string
	handler  func(ctx context.Context) error
}

// NewAckTask creates a new ack task
func NewAckTask(handler func(ctx context.Context) error) *AckTask {
	return &AckTask{
		taskType: TaskTypeAck,
		handler:  handler,
	}
}

func (t *AckTask) Execute(ctx context.Context) error {
	if t.handler == nil {
		return fmt.Errorf("ack task has no handler")
	}
	return t.handler(ctx)
}

func (t *AckTask) Type() string {
	return t.taskType
}

// MailboxTask handles envelope appends, retention cleanup, and NACK/dead-lettering
type MailboxTask struct {
	taskType string
	handler  func(ctx context.Context) error
}

// NewMailboxTask creates a new mailbox task
func NewMailboxTask(handler func(ctx context.Context) error) *MailboxTask {
	return &MailboxTask{
		taskType: TaskTypeMailbox,
		handler:  handler,
	}
}

func (t *MailboxTask) Execute(ctx context.Context) error {
	if t.handler == nil {
		return fmt.Errorf("mailbox task has no handler")
	}
	return t.handler(ctx)
}

func (t *MailboxTask) Type() string {
	return t.taskType
}
