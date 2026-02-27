package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/ambientlabscomputing/mycelium_spine/internal/metrics"
	"github.com/ambientlabscomputing/mycelium_spine/internal/repository"
	"github.com/ambientlabscomputing/mycelium_spine/internal/types"
	"github.com/ambientlabscomputing/mycelium_spine/internal/utils"
)

// publishServiceImpl implements PublishService
type publishServiceImpl struct {
	repo            repository.Repository
	deliveryService DeliveryService
	metrics         *metrics.Metrics
	logger          *slog.Logger
}

// NewPublishService creates a new publish service
func NewPublishService(repo repository.Repository, deliveryService DeliveryService, m *metrics.Metrics) PublishService {
	return &publishServiceImpl{
		repo:            repo,
		deliveryService: deliveryService,
		metrics:         m,
		logger:          utils.Logger.With("service", "publish"),
	}
}

// Publish resolves targets, appends envelopes to mailboxes, and triggers delivery
func (s *publishServiceImpl) Publish(ctx context.Context, envelope *types.Envelope, targets []*types.Target) (*PublishResult, error) {
	logger := s.logger.With("envelope_id", envelope.EnvelopeID, "type", envelope.Type, "qos", envelope.QoS, "trace_id", envelope.TraceID)
	logger.Info("publishing envelope", "target_count", len(targets))

	// Resolve targets to mailbox IDs
	mailboxIDs, err := s.repo.ResolveTargets(ctx, targets)
	if err != nil {
		return &PublishResult{
			Success: false,
			Error:   fmt.Sprintf("failed to resolve targets: %v", err),
		}, err
	}

	logger.Info("targets resolved", "mailbox_count", len(mailboxIDs))

	// Append envelope to each mailbox
	mailboxSeqs := make(map[string]uint64)
	for _, mailboxID := range mailboxIDs {
		// Clone envelope for each mailbox (different seq per mailbox)
		envCopy := *envelope
		envCopy.MailboxID = mailboxID

		seq, err := s.repo.AppendEnvelope(ctx, mailboxID, &envCopy)
		if err != nil {
			logger.Error("failed to append envelope to mailbox",
				"mailbox_id", mailboxID,
				"error", err)
			continue
		}

		mailboxSeqs[mailboxID] = seq
		logger.Debug("envelope appended", "mailbox_id", mailboxID, "seq", seq)

		// TODO: Update MailboxBacklog metric when repository provides sequential state
		// Backlog tracking requires both NextSeq and LastAckedSeq from repository
	}

	// PHASE 2: Trigger immediate delivery for active sessions subscribed to these mailboxes
	s.deliveryService.NotifyDeliveryLoops()

	logger.Info("envelope published successfully", "mailboxes_written", len(mailboxSeqs))
	return &PublishResult{
		Success:     true,
		MailboxSeqs: mailboxSeqs,
	}, nil
}
