package types

import (
	"time"

	"github.com/google/uuid"
)

// QoS defines quality of service class for envelopes
type QoS string

const (
	QoSUnspecified QoS = "UNSPECIFIED"
	QoSCommand     QoS = "COMMAND"   // At-least-once, durable, never drop unless expired
	QoSControl     QoS = "CONTROL"   // At-least-once, durable, can be collapsed
	QoSTelemetry   QoS = "TELEMETRY" // Best-effort, drop-first under backpressure
)

// Envelope is a single message unit delivered over UMS
type Envelope struct {
	EnvelopeID  string `json:"envelope_id" bson:"envelope_id"`
	MailboxID   string `json:"mailbox_id" bson:"mailbox_id"`
	Seq         uint64 `json:"seq" bson:"seq"` // Monotonic per mailbox
	QoS         QoS    `json:"qos" bson:"qos"`
	Type        string `json:"type" bson:"type"` // e.g., "command.run", "deploy.apply"
	CreatedAtMs int64  `json:"created_at_ms" bson:"created_at_ms"`
	ExpiresAtMs int64  `json:"expires_at_ms,omitempty" bson:"expires_at_ms,omitempty"`
	TraceID     string `json:"trace_id,omitempty" bson:"trace_id,omitempty"`
	Payload     []byte `json:"payload" bson:"payload"` // JSON/protobuf/CBOR
	DedupeKey   string `json:"dedupe_key,omitempty" bson:"dedupe_key,omitempty"`
	Priority    int32  `json:"priority,omitempty" bson:"priority,omitempty"`
	RequiresAck bool   `json:"requires_ack" bson:"requires_ack"`
	OrgID       string `json:"org_id" bson:"org_id"` // Multi-tenancy
}

// NewEnvelope creates a new envelope with generated ID and timestamp
func NewEnvelope(mailboxID string, qos QoS, envelopeType string, payload []byte, orgID string) *Envelope {
	now := time.Now().UnixMilli()
	requiresAck := qos == QoSCommand || qos == QoSControl
	return &Envelope{
		EnvelopeID:  uuid.New().String(),
		MailboxID:   mailboxID,
		QoS:         qos,
		Type:        envelopeType,
		CreatedAtMs: now,
		Payload:     payload,
		RequiresAck: requiresAck,
		OrgID:       orgID,
	}
}

// WithSeq sets the sequence number (assigned by mailbox repository)
func (e *Envelope) WithSeq(seq uint64) *Envelope {
	e.Seq = seq
	return e
}

// WithTraceID sets the trace ID for request correlation
func (e *Envelope) WithTraceID(traceID string) *Envelope {
	e.TraceID = traceID
	return e
}

// WithDedupeKey sets the dedupe key for idempotency
func (e *Envelope) WithDedupeKey(key string) *Envelope {
	e.DedupeKey = key
	return e
}

// WithExpiry sets the expiration time (used for COMMAND envelopes)
func (e *Envelope) WithExpiry(expiresAtMs int64) *Envelope {
	e.ExpiresAtMs = expiresAtMs
	return e
}

// WithPriority sets the priority within QoS class
func (e *Envelope) WithPriority(priority int32) *Envelope {
	e.Priority = priority
	return e
}

// IsExpired checks if the envelope has expired
func (e *Envelope) IsExpired() bool {
	if e.ExpiresAtMs == 0 {
		return false
	}
	return time.Now().UnixMilli() >= e.ExpiresAtMs
}
