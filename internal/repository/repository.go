package repository

import (
	"context"

	"github.com/ambientlabscomputing/mycelium_spine/internal/types"
)

// Repository is the main interface for data access operations
type Repository interface {
	MailboxRepository
	SessionRepository
	TargetResolver
}

// MailboxRepository handles mailbox and envelope persistence
type MailboxRepository interface {
	// GetOrCreateMailbox finds an existing mailbox or creates it
	GetOrCreateMailbox(ctx context.Context, targetType types.TargetType, targetID string, orgID string) (*types.Mailbox, error)

	// GetMailbox retrieves a mailbox by ID
	GetMailbox(ctx context.Context, mailboxID string) (*types.Mailbox, error)

	// AppendEnvelope atomically assigns a sequence number and inserts an envelope
	AppendEnvelope(ctx context.Context, mailboxID string, envelope *types.Envelope) (uint64, error)

	// FetchEnvelopes retrieves envelopes from a mailbox starting at fromSeq
	FetchEnvelopes(ctx context.Context, mailboxID string, fromSeq uint64, limit int) ([]*types.Envelope, error)

	// DeleteExpired removes expired envelopes from a mailbox
	DeleteExpired(ctx context.Context, mailboxID string) (int64, error)

	// EnforceRetention ensures mailbox stays within retention policy limits
	EnforceRetention(ctx context.Context, mailboxID string, policy types.RetentionPolicy) error
}

// SessionRepository handles session persistence and cursor tracking
type SessionRepository interface {
	// CreateSession persists a new session
	CreateSession(ctx context.Context, session *types.Session) error

	// GetSession retrieves a session by ID
	GetSession(ctx context.Context, sessionID string) (*types.Session, error)

	// GetSessionByServerID retrieves the active session for a server_id
	GetSessionByServerID(ctx context.Context, serverID string) (*types.Session, error)

	// GetSessionByResumeToken retrieves a session by its resume token (for session resumption)
	GetSessionByResumeToken(ctx context.Context, resumeToken string) (*types.Session, error)

	// UpdateResumeToken rotates the resume token for a session
	UpdateResumeToken(ctx context.Context, sessionID string, newToken string) error

	// UpdateAckPosition advances the ack cursor for a mailbox
	UpdateAckPosition(ctx context.Context, sessionID string, mailboxID string, seq uint64) error

	// GetAckPositions retrieves all ack positions for a session
	GetAckPositions(ctx context.Context, sessionID string) (map[string]uint64, error)

	// DeleteSession removes a session and its cursors
	DeleteSession(ctx context.Context, sessionID string) error

	// UpdateHeartbeat updates the session's last heartbeat timestamp
	UpdateHeartbeat(ctx context.Context, sessionID string) error

	// UpdateSubscriptions persists the session's subscription list to MongoDB
	UpdateSubscriptions(ctx context.Context, sessionID string, mailboxIDs []string) error
}

// TargetResolver resolves targets to mailbox IDs
type TargetResolver interface {
	// ResolveTargets maps targets to mailbox IDs
	ResolveTargets(ctx context.Context, targets []*types.Target) ([]string, error)
}
