package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/ambientlabscomputing/mycelium_spine/internal/metrics"
	"github.com/ambientlabscomputing/mycelium_spine/internal/repository"
	"github.com/ambientlabscomputing/mycelium_spine/internal/types"
	"github.com/ambientlabscomputing/mycelium_spine/internal/utils"
	umsv1 "github.com/ambientlabscomputing/mycelium_spine/proto/ums/v1"
)

// subscribeServiceImpl implements SubscribeService
type subscribeServiceImpl struct {
	repo            repository.Repository
	deliveryService DeliveryService
	metrics         *metrics.Metrics
	logger          *slog.Logger
}

// NewSubscribeService creates a new subscribe service
func NewSubscribeService(repo repository.Repository, deliveryService DeliveryService, m *metrics.Metrics) SubscribeService {
	return &subscribeServiceImpl{
		repo:            repo,
		deliveryService: deliveryService,
		metrics:         m,
		logger:          utils.Logger.With("service", "subscribe"),
	}
}

// HandleSubscribe processes a SUBSCRIBE frame and adds mailboxes to session
func (s *subscribeServiceImpl) HandleSubscribe(ctx context.Context, session *types.Session, targets []*types.Target) (*umsv1.SubscribeOkFrame, error) {
	logger := s.logger.With("session_id", session.SessionID, "target_count", len(targets))
	logger.Info("processing SUBSCRIBE frame")

	// Resolve targets to mailbox IDs
	mailboxIDs, err := s.repo.ResolveTargets(ctx, targets)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve subscription targets: %w", err)
	}

	logger.Info("subscription targets resolved", "mailbox_count", len(mailboxIDs))

	// Add mailboxes to session subscriptions
	for _, mailboxID := range mailboxIDs {
		if !session.HasSubscription(mailboxID) {
			session.AddSubscription(mailboxID)
			logger.Debug("added subscription", "mailbox_id", mailboxID)
		}
	}

	// PHASE 2: Persist subscription changes to MongoDB
	if err := s.repo.UpdateSubscriptions(ctx, session.SessionID, session.Subscriptions); err != nil {
		logger.Error("failed to persist subscriptions", "error", err)
		return nil, fmt.Errorf("failed to persist subscriptions: %w", err)
	}

	// Start delivery loop for newly subscribed mailboxes
	// For V1, we'll rely on a single delivery loop per session that checks all subscriptions

	// Build SubscribeOkFrame
	subscribeOk := &umsv1.SubscribeOkFrame{
		MailboxIds: mailboxIDs,
	}

	logger.Info("subscription successful", "mailbox_count", len(mailboxIDs))
	return subscribeOk, nil
}
