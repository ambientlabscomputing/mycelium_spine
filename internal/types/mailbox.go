package types

import (
	"time"

	"github.com/google/uuid"
)

// TargetType defines mailbox target types
type TargetType string

const (
	TargetTypeUnspecified TargetType = "UNSPECIFIED"
	TargetTypeServer      TargetType = "SERVER"
	TargetTypeCluster     TargetType = "CLUSTER"
	TargetTypeOrg         TargetType = "ORG"
	TargetTypeService     TargetType = "SERVICE"
	TargetTypeBroadcast   TargetType = "BROADCAST"
)

// RetentionPolicy defines how long envelopes are kept in a mailbox
type RetentionPolicy struct {
	RetentionSeconds int `json:"retention_seconds" bson:"retention_seconds"`
	MaxBytes         int `json:"max_bytes" bson:"max_bytes"`
	MaxEnvelopes     int `json:"max_envelopes" bson:"max_envelopes"`
}

// Mailbox is a durable ordered log of envelopes for a target
type Mailbox struct {
	MailboxID       string          `json:"mailbox_id" bson:"mailbox_id"`
	TargetType      TargetType      `json:"target_type" bson:"target_type"`
	TargetID        string          `json:"target_id" bson:"target_id"`
	OrgID           string          `json:"org_id" bson:"org_id"`     // Multi-tenancy
	NextSeq         uint64          `json:"next_seq" bson:"next_seq"` // Atomic counter
	RetentionPolicy RetentionPolicy `json:"retention_policy" bson:"retention_policy"`
	CreatedAt       string          `json:"created_at" bson:"created_at"`
	UpdatedAt       string          `json:"updated_at" bson:"updated_at"`
}

// NewMailbox creates a new mailbox with default retention policy
func NewMailbox(targetType TargetType, targetID string, orgID string, retentionPolicy RetentionPolicy) *Mailbox {
	now := time.Now().Format(time.RFC3339)
	mailboxID := uuid.New().String()

	return &Mailbox{
		MailboxID:       mailboxID,
		TargetType:      targetType,
		TargetID:        targetID,
		OrgID:           orgID,
		NextSeq:         1, // Start at sequence 1
		RetentionPolicy: retentionPolicy,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}

// MailboxCursor tracks the last acknowledged sequence per session
type MailboxCursor struct {
	SessionID    string `json:"session_id" bson:"session_id"`
	MailboxID    string `json:"mailbox_id" bson:"mailbox_id"`
	LastAckedSeq uint64 `json:"last_acked_seq" bson:"last_acked_seq"`
	UpdatedAt    string `json:"updated_at" bson:"updated_at"`
}

// NewMailboxCursor creates a new cursor for a mailbox
func NewMailboxCursor(sessionID string, mailboxID string) *MailboxCursor {
	return &MailboxCursor{
		SessionID:    sessionID,
		MailboxID:    mailboxID,
		LastAckedSeq: 0, // Start at 0, first delivery will be seq 1
		UpdatedAt:    time.Now().Format(time.RFC3339),
	}
}

// Advance moves the cursor forward (cumulative ack)
func (c *MailboxCursor) Advance(seq uint64) {
	if seq > c.LastAckedSeq {
		c.LastAckedSeq = seq
		c.UpdatedAt = time.Now().Format(time.RFC3339)
	}
}
