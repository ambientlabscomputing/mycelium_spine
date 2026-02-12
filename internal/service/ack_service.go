package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/ambientlabscomputing/mycelium_spine/internal/repository"
	"github.com/ambientlabscomputing/mycelium_spine/internal/utils"
)

// ackServiceImpl implements AckService
type ackServiceImpl struct {
	repo   repository.Repository
	logger *slog.Logger
}

// NewAckService creates a new ack service
func NewAckService(repo repository.Repository) AckService {
	return &ackServiceImpl{
		repo:   repo,
		logger: utils.Logger.With("service", "ack"),
	}
}

// HandleCumulativeAck processes a cumulative ACK (all seq <= seqAcked)
func (s *ackServiceImpl) HandleCumulativeAck(ctx context.Context, sessionID string, mailboxID string, seqAcked uint64) error {
	logger := s.logger.With("session_id", sessionID, "mailbox_id", mailboxID, "seq_acked", seqAcked)
	logger.Debug("processing cumulative ACK")

	// Update ack position in repository
	if err := s.repo.UpdateAckPosition(ctx, sessionID, mailboxID, seqAcked); err != nil {
		logger.Error("failed to update ack position", "error", err)
		return fmt.Errorf("failed to update ack position: %w", err)
	}

	logger.Debug("cumulative ACK processed successfully")
	return nil
}

// HandleSelectiveAck processes a selective ACK (specific seqs)
func (s *ackServiceImpl) HandleSelectiveAck(ctx context.Context, sessionID string, mailboxID string, seqs []uint64) error {
	logger := s.logger.With("session_id", sessionID, "mailbox_id", mailboxID, "seq_count", len(seqs))
	logger.Debug("processing selective ACK")

	// For selective ACK, we need to track individual seqs
	// For V1, we'll convert to cumulative ACK by taking max(seqs)
	var maxSeq uint64
	for _, seq := range seqs {
		if seq > maxSeq {
			maxSeq = seq
		}
	}

	if err := s.repo.UpdateAckPosition(ctx, sessionID, mailboxID, maxSeq); err != nil {
		logger.Error("failed to update ack position", "error", err)
		return fmt.Errorf("failed to update ack position: %w", err)
	}

	logger.Debug("selective ACK processed successfully", "max_seq", maxSeq)
	return nil
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
