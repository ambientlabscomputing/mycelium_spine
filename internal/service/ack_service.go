package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/ambientlabscomputing/mycelium_spine/internal/metrics"
	"github.com/ambientlabscomputing/mycelium_spine/internal/repository"
	"github.com/ambientlabscomputing/mycelium_spine/internal/utils"
)

// ackServiceImpl implements AckService
type ackServiceImpl struct {
	repo    repository.Repository
	metrics *metrics.Metrics
	logger  *slog.Logger
}

// NewAckService creates a new ack service
func NewAckService(repo repository.Repository, m *metrics.Metrics) AckService {
	return &ackServiceImpl{
		repo:    repo,
		metrics: m,
		logger:  utils.Logger.With("service", "ack"),
	}
}

// HandleCumulativeAck processes a cumulative ACK (all seq <= seqAcked).
// serverID is the stable agent identity keyed by server_id (not session_id) so progress
// persists across reconnects and session rotation.
func (s *ackServiceImpl) HandleCumulativeAck(ctx context.Context, serverID string, mailboxID string, seqAcked uint64) error {
	logger := s.logger.With("server_id", serverID, "mailbox_id", mailboxID, "seq_acked", seqAcked)
	logger.Debug("processing cumulative ACK")

	// Update ack position in repository (keyed by server_id)
	if err := s.repo.UpdateAckPosition(ctx, serverID, mailboxID, seqAcked); err != nil {
		logger.Error("failed to update ack position", "error", err)
		return fmt.Errorf("failed to update ack position: %w", err)
	}

	// TODO: Update AckLag metric when repository provides last acked sequence
	// For now, metrics are updated from delivery service when envelopes are delivered

	logger.Debug("cumulative ACK processed successfully")
	return nil
}

// HandleSelectiveAck processes a selective ACK (specific seqs out-of-order)
func (s *ackServiceImpl) HandleSelectiveAck(ctx context.Context, sessionID string, mailboxID string, seqs []uint64) error {
	logger := s.logger.With("session_id", sessionID, "mailbox_id", mailboxID, "seq_count", len(seqs))
	logger.Debug("processing selective ACK")

	// FIXME: Selective ACK requires per-seq gap tracking, not cumulative.
	// Taking max(seqs) silently marks unprocessed gaps as acked, causing data loss.
	// For now, return error to reject selective ACKs until proper tracking is implemented.
	// TODO: Implement bitset-based selective ACK tracking in repository

	logger.Warn("selective ACK not yet fully supported; use cumulative ACK instead")
	return fmt.Errorf("selective ACK requires gap tracking; not yet implemented in V1. use cumulative ACK")
}

// HandleNack processes a NACK (client rejects an envelope)
func (s *ackServiceImpl) HandleNack(ctx context.Context, sessionID string, mailboxID string, seq uint64, reason string) error {
	logger := s.logger.With(
		"session_id", sessionID,
		"mailbox_id", mailboxID,
		"seq", seq,
		"reason", reason,
	)
	logger.Warn("processing NACK")

	// TODO: Implement dead-lettering or quarantine policy
	// For V1, just log the NACK

	logger.Info("NACK recorded (no policy applied in V1)")
	return nil
}
