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

	// ListMailboxes returns mailboxes with optional filtering and pagination.
	ListMailboxes(ctx context.Context, orgID string, targetType string, limit, offset int) ([]*types.Mailbox, int64, error)

	// ClearMailbox deletes all envelopes for a mailbox and resets next_seq to 1.
	ClearMailbox(ctx context.Context, mailboxID string) (int64, error)

	// CountMailboxes returns the total number of mailboxes.
	CountMailboxes(ctx context.Context) (int64, error)
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

	// UpdateAckPosition advances the ack cursor for a mailbox, keyed by server_id (stable across sessions).
	UpdateAckPosition(ctx context.Context, serverID string, mailboxID string, seq uint64) error

	// GetAckPositions retrieves all ack positions for a server, keyed by server_id.
	GetAckPositions(ctx context.Context, serverID string) (map[string]uint64, error)

	// DeleteSession removes a session document only. Ack cursors are NOT deleted because they
	// are keyed by server_id and must survive session rotation to prevent re-delivery.
	DeleteSession(ctx context.Context, sessionID string) error

	// DeleteServerCursors removes all ack cursors for a server. Call only when permanently
	// decommissioning a server (not on normal reconnect or session rotation).
	DeleteServerCursors(ctx context.Context, serverID string) error

	// UpdateHeartbeat updates the session's last heartbeat timestamp
	UpdateHeartbeat(ctx context.Context, sessionID string) error

	// UpdateSubscriptions persists the session's subscription list to MongoDB
	UpdateSubscriptions(ctx context.Context, sessionID string, mailboxIDs []string) error

	// ListCursors returns all ACK cursors, optionally filtered by server_id or mailbox_id.
	ListCursors(ctx context.Context, serverID string, mailboxID string, limit, offset int) ([]*types.MailboxCursor, int64, error)

	// ResetCursor sets last_acked_seq to the specified value for a (server_id, mailbox_id) cursor.
	ResetCursor(ctx context.Context, serverID string, mailboxID string, seq uint64) error
}

// TargetResolver resolves targets to mailbox IDs
type TargetResolver interface {
	// ResolveTargets maps targets to mailbox IDs
	ResolveTargets(ctx context.Context, targets []*types.Target) ([]string, error)
}
