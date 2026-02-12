package service

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/ambientlabscomputing/mycelium_spine/internal/repository"
	"github.com/ambientlabscomputing/mycelium_spine/internal/types"
	"github.com/ambientlabscomputing/mycelium_spine/internal/utils"
	"github.com/ambientlabscomputing/mycelium_spine/internal/workers"
	umsv1 "github.com/ambientlabscomputing/mycelium_spine/proto/ums/v1"
)

// deliveryServiceImpl implements DeliveryService
type deliveryServiceImpl struct {
	repo          repository.Repository
	poolManager   *workers.PoolManager
	settings      *utils.Settings
	deliveryLoops sync.Map // sessionID → context.CancelFunc
	logger        *slog.Logger
}

// NewDeliveryService creates a new delivery service
func NewDeliveryService(repo repository.Repository, poolManager *workers.PoolManager, settings *utils.Settings) DeliveryService {
	return &deliveryServiceImpl{
		repo:        repo,
		poolManager: poolManager,
		settings:    settings,
		logger:      utils.Logger.With("service", "delivery"),
	}
}

// StartDeliveryLoop starts a background delivery loop for a session
func (s *deliveryServiceImpl) StartDeliveryLoop(ctx context.Context, session *types.Session) error {
	logger := s.logger.With("session_id", session.SessionID)
	logger.Info("starting delivery loop")

	// Create a cancellable context for this delivery loop
	loopCtx, cancel := context.WithCancel(ctx)
	s.deliveryLoops.Store(session.SessionID, cancel)

	// Start delivery loop in background
	go s.deliveryLoop(loopCtx, session)

	return nil
}

// StopDeliveryLoop stops the delivery loop for a session
func (s *deliveryServiceImpl) StopDeliveryLoop(ctx context.Context, sessionID string) error {
	s.logger.Info("stopping delivery loop", "session_id", sessionID)

	// Cancel the delivery loop context
	if cancel, ok := s.deliveryLoops.LoadAndDelete(sessionID); ok {
		cancelFunc := cancel.(context.CancelFunc)
		cancelFunc()
	}

	return nil
}

// deliveryLoop is the background goroutine that polls mailboxes and pushes envelopes
func (s *deliveryServiceImpl) deliveryLoop(ctx context.Context, session *types.Session) {
	logger := s.logger.With("session_id", session.SessionID)
	logger.Info("delivery loop started")

	defer func() {
		logger.Info("delivery loop stopped")
	}()

	// For V1, use a simple polling approach
	// TODO: Replace with notification channel pattern (wait for signals from Publish)

	for {
		select {
		case <-ctx.Done():
			return

		default:
			// Poll each subscribed mailbox
			for _, mailboxID := range session.Subscriptions {
				s.deliverFromMailbox(ctx, session, mailboxID)
			}

			// Sleep to avoid tight loop (TODO: use notification channel instead)
			// time.Sleep(100 * time.Millisecond)
		}
	}
}

// deliverFromMailbox fetches and delivers envelopes from a single mailbox
func (s *deliveryServiceImpl) deliverFromMailbox(ctx context.Context, session *types.Session, mailboxID string) {
	logger := s.logger.With("session_id", session.SessionID, "mailbox_id", mailboxID)

	// Get current ack position
	ackPositions, err := s.repo.GetAckPositions(ctx, session.SessionID)
	if err != nil {
		logger.Error("failed to get ack positions", "error", err)
		return
	}

	lastAckedSeq := ackPositions[mailboxID]

	// Check flow control limits
	totalInflight := session.GetTotalInflight()
	if totalInflight >= s.settings.FlowControl.MaxInflightTotal {
		logger.Debug("flow control limit reached, skipping delivery",
			"inflight", totalInflight,
			"limit", s.settings.FlowControl.MaxInflightTotal)
		return
	}

	// Fetch envelopes from mailbox (starting at lastAckedSeq + 1)
	limit := s.settings.FlowControl.MaxInflightTotal - totalInflight
	if limit > 100 {
		limit = 100 // Batch limit
	}

	envelopes, err := s.repo.FetchEnvelopes(ctx, mailboxID, lastAckedSeq+1, limit)
	if err != nil {
		logger.Error("failed to fetch envelopes", "error", err)
		return
	}

	if len(envelopes) == 0 {
		return // No new envelopes
	}

	logger.Debug("fetched envelopes", "count", len(envelopes))

	// Deliver envelopes to session
	if err := s.DeliverToSession(ctx, session, envelopes); err != nil {
		logger.Error("failed to deliver envelopes", "error", err)
	}
}

// DeliverToSession pushes envelopes through the gRPC stream
func (s *deliveryServiceImpl) DeliverToSession(ctx context.Context, session *types.Session, envelopes []*types.Envelope) error {
	logger := s.logger.With("session_id", session.SessionID, "envelope_count", len(envelopes))
	logger.Debug("delivering envelopes to session")

	if session.Stream == nil {
		return fmt.Errorf("session has no active stream")
	}

	// Group envelopes by QoS and apply flow control
	batchByQoS := make(map[types.QoS][]*types.Envelope)
	for _, env := range envelopes {
		// Check QoS-specific inflight limits
		inflight := session.GetInflight(env.QoS)
		var limit int
		switch env.QoS {
		case types.QoSCommand:
			limit = s.settings.FlowControl.MaxInflightCommand
		case types.QoSControl:
			limit = s.settings.FlowControl.MaxInflightControl
		case types.QoSTelemetry:
			limit = s.settings.FlowControl.MaxInflightTelemetry
		}

		if inflight >= limit {
			// Skip this envelope (will be delivered next loop iteration)
			continue
		}

		batchByQoS[env.QoS] = append(batchByQoS[env.QoS], env)
	}

	// Convert to proto and send
	protoEnvelopes := make([]*umsv1.Envelope, 0)
	for _, envs := range batchByQoS {
		for _, env := range envs {
			protoEnv := &umsv1.Envelope{
				EnvelopeId:  env.EnvelopeID,
				MailboxId:   env.MailboxID,
				Seq:         env.Seq,
				Qos:         qosToProto(env.QoS),
				Type:        env.Type,
				CreatedAtMs: env.CreatedAtMs,
				ExpiresAtMs: env.ExpiresAtMs,
				TraceId:     env.TraceID,
				Payload:     env.Payload,
				DedupeKey:   env.DedupeKey,
				Priority:    env.Priority,
				RequiresAck: env.RequiresAck,
				OrgId:       env.OrgID,
			}
			protoEnvelopes = append(protoEnvelopes, protoEnv)

			// Increment inflight counter
			session.IncrementInflight(env.QoS)
		}
	}

	// Send DELIVER frame
	deliverFrame := &umsv1.ServerFrame{
		Frame: &umsv1.ServerFrame_Deliver{
			Deliver: &umsv1.DeliverFrame{
				BatchId:   "batch-" + session.SessionID,
				Envelopes: protoEnvelopes,
			},
		},
	}

	if err := session.Stream.SendMsg(deliverFrame); err != nil {
		logger.Error("failed to send DELIVER frame", "error", err)
		return fmt.Errorf("failed to send DELIVER frame: %w", err)
	}

	logger.Info("envelopes delivered successfully", "count", len(protoEnvelopes))
	return nil
}

// HandleBackpressure adjusts delivery behavior based on client hints
func (s *deliveryServiceImpl) HandleBackpressure(ctx context.Context, session *types.Session, hint *umsv1.FlowHintFrame) error {
	logger := s.logger.With("session_id", session.SessionID, "hint", hint.Hint)
	logger.Info("processing flow hint")

	// TODO: Implement backpressure handling
	// - "overloaded": reduce all delivery rates
	// - "reduce_telemetry": drop TELEMETRY envelopes
	// - "pause_control": skip CONTROL delivery temporarily

	return nil
}

// qosToProto converts types.QoS to proto QoS
func qosToProto(qos types.QoS) umsv1.QoS {
	switch qos {
	case types.QoSCommand:
		return umsv1.QoS_QOS_COMMAND
	case types.QoSControl:
		return umsv1.QoS_QOS_CONTROL
	case types.QoSTelemetry:
		return umsv1.QoS_QOS_TELEMETRY
	default:
		return umsv1.QoS_QOS_UNSPECIFIED
	}
}
