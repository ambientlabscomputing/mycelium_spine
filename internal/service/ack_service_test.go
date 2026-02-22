package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleCumulativeAck_Success(t *testing.T) {
	sessionID := "test-session-1"
	mailboxID := "mailbox-123"
	seqAcked := uint64(100)
	ackCalled := false

	mockRepo := &MockRepository{
		UpdateAckPositionFunc: func(ctx context.Context, sid string, mid string, seq uint64) error {
			assert.Equal(t, sessionID, sid)
			assert.Equal(t, mailboxID, mid)
			assert.Equal(t, seqAcked, seq)
			ackCalled = true
			return nil
		},
	}

	svc := NewAckService(mockRepo, testMetrics)

	ctx := context.Background()
	err := svc.HandleCumulativeAck(ctx, sessionID, mailboxID, seqAcked)

	require.NoError(t, err)
	assert.True(t, ackCalled)
}

func TestHandleCumulativeAck_RepositoryError(t *testing.T) {
	mockRepo := &MockRepository{
		UpdateAckPositionFunc: func(ctx context.Context, sid string, mid string, seq uint64) error {
			return errors.New("database error")
		},
	}

	// Use shared testMetrics
	svc := NewAckService(mockRepo, testMetrics)

	ctx := context.Background()
	err := svc.HandleCumulativeAck(ctx, "session-1", "mailbox-1", 50)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to update ack position")
}

func TestHandleCumulativeAck_TableDriven(t *testing.T) {
	tests := []struct {
		name       string
		sessionID  string
		mailboxID  string
		seqAcked   uint64
		repoError  error
		wantError  bool
		errorMatch string
	}{
		{
			name:      "successful ack",
			sessionID: "session-1",
			mailboxID: "mailbox-1",
			seqAcked:  50,
			repoError: nil,
			wantError: false,
		},
		{
			name:       "repository error",
			sessionID:  "session-2",
			mailboxID:  "mailbox-2",
			seqAcked:   100,
			repoError:  errors.New("connection timeout"),
			wantError:  true,
			errorMatch: "failed to update ack position",
		},
		{
			name:      "zero sequence",
			sessionID: "session-3",
			mailboxID: "mailbox-3",
			seqAcked:  0,
			repoError: nil,
			wantError: false,
		},
		{
			name:      "large sequence number",
			sessionID: "session-4",
			mailboxID: "mailbox-4",
			seqAcked:  uint64(1<<63 - 1),
			repoError: nil,
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := &MockRepository{
				UpdateAckPositionFunc: func(ctx context.Context, sid string, mid string, seq uint64) error {
					assert.Equal(t, tt.sessionID, sid)
					assert.Equal(t, tt.mailboxID, mid)
					assert.Equal(t, tt.seqAcked, seq)
					return tt.repoError
				},
			}

			// Use shared testMetrics
			svc := NewAckService(mockRepo, testMetrics)

			ctx := context.Background()
			err := svc.HandleCumulativeAck(ctx, tt.sessionID, tt.mailboxID, tt.seqAcked)

			if tt.wantError {
				require.Error(t, err)
				if tt.errorMatch != "" {
					assert.Contains(t, err.Error(), tt.errorMatch)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestHandleSelectiveAck_NotImplemented(t *testing.T) {
	mockRepo := &MockRepository{}
	// Use shared testMetrics
	svc := NewAckService(mockRepo, testMetrics)

	ctx := context.Background()
	seqs := []uint64{10, 20, 30, 50}
	err := svc.HandleSelectiveAck(ctx, "session-1", "mailbox-1", seqs)

	// Selective ACK is not yet implemented in V1
	require.Error(t, err)
	assert.Contains(t, err.Error(), "selective ACK requires gap tracking")
}

func TestHandleSelectiveAck_EmptySeqs(t *testing.T) {
	mockRepo := &MockRepository{}
	// Use shared testMetrics
	svc := NewAckService(mockRepo, testMetrics)

	ctx := context.Background()
	err := svc.HandleSelectiveAck(ctx, "session-1", "mailbox-1", []uint64{})

	// Should still return error since selective ACK is not implemented
	require.Error(t, err)
}

func TestHandleSelectiveAck_NilSeqs(t *testing.T) {
	mockRepo := &MockRepository{}
	// Use shared testMetrics
	svc := NewAckService(mockRepo, testMetrics)

	ctx := context.Background()
	err := svc.HandleSelectiveAck(ctx, "session-1", "mailbox-1", nil)

	// Should still return error since selective ACK is not implemented
	require.Error(t, err)
}

func TestHandleNack_Success(t *testing.T) {
	mockRepo := &MockRepository{}
	// Use shared testMetrics
	svc := NewAckService(mockRepo, testMetrics)

	ctx := context.Background()
	err := svc.HandleNack(ctx, "session-1", "mailbox-1", 42, "invalid payload")

	// NACK is accepted but not yet acted upon in V1
	require.NoError(t, err)
}

func TestHandleNack_TableDriven(t *testing.T) {
	tests := []struct {
		name      string
		sessionID string
		mailboxID string
		seq       uint64
		reason    string
	}{
		{
			name:      "nack with reason",
			sessionID: "session-1",
			mailboxID: "mailbox-1",
			seq:       10,
			reason:    "corrupted data",
		},
		{
			name:      "nack with empty reason",
			sessionID: "session-2",
			mailboxID: "mailbox-2",
			seq:       50,
			reason:    "",
		},
		{
			name:      "nack at sequence zero",
			sessionID: "session-3",
			mailboxID: "mailbox-3",
			seq:       0,
			reason:    "protocol error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := &MockRepository{}
			// Use shared testMetrics
			svc := NewAckService(mockRepo, testMetrics)

			ctx := context.Background()
			err := svc.HandleNack(ctx, tt.sessionID, tt.mailboxID, tt.seq, tt.reason)

			// NACK is logged but no error should occur
			require.NoError(t, err)
		})
	}
}

func TestHandleNack_EmptySessionID(t *testing.T) {
	mockRepo := &MockRepository{}
	// Use shared testMetrics
	svc := NewAckService(mockRepo, testMetrics)

	ctx := context.Background()
	err := svc.HandleNack(ctx, "", "mailbox-1", 10, "test reason")

	// Even with empty session ID, no error should occur (just logged)
	require.NoError(t, err)
}

func TestHandleNack_EmptyMailboxID(t *testing.T) {
	mockRepo := &MockRepository{}
	// Use shared testMetrics
	svc := NewAckService(mockRepo, testMetrics)

	ctx := context.Background()
	err := svc.HandleNack(ctx, "session-1", "", 10, "test reason")

	// Even with empty mailbox ID, no error should occur (just logged)
	require.NoError(t, err)
}
