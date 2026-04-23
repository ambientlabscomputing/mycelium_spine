package service

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/ambientlabscomputing/mycelium_spine/internal/metrics"
	"github.com/ambientlabscomputing/mycelium_spine/internal/repository"
	"github.com/ambientlabscomputing/mycelium_spine/internal/types"
	"github.com/ambientlabscomputing/mycelium_spine/internal/utils"
	umsv1 "github.com/ambientlabscomputing/mycelium_spine/proto/ums/v1"
)

// sessionServiceImpl implements SessionService
type sessionServiceImpl struct {
	repo     repository.Repository
	settings *utils.Settings
	appSvc   *AppService
	metrics  *metrics.Metrics
	sessions sync.Map // serverID → *types.Session (in-memory registry)
	logger   *slog.Logger
}

// NewSessionService creates a new session service
func NewSessionService(repo repository.Repository, settings *utils.Settings, appSvc *AppService, m *metrics.Metrics) SessionService {
	return &sessionServiceImpl{
		repo:     repo,
		settings: settings,
		appSvc:   appSvc,
		metrics:  m,
		logger:   utils.Logger.With("service", "session"),
	}
}

// HandleHello processes a HelloFrame and creates a new session
func (s *sessionServiceImpl) HandleHello(ctx context.Context, hello *umsv1.HelloFrame) (*umsv1.WelcomeFrame, *types.Session, error) {
	logger := s.logger.With("server_id", hello.ServerId, "org_id", hello.OrgId)
	logger.Info("processing HELLO frame")

	// Check if there's an existing persisted session for this server_id.
	// We also reconcile any live in-memory session below because an older
	// delivery loop can remain active until the transport fully tears down.
	existingSession, err := s.repo.GetSessionByServerID(ctx, hello.ServerId)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to check existing session: %w", err)
	}

	var epoch uint64 = 1
	if existingSession != nil {
		epoch = existingSession.SessionEpoch + 1
	}
	if liveSession := s.getLiveSessionByServerID(hello.ServerId); liveSession != nil && liveSession.SessionEpoch >= epoch {
		epoch = liveSession.SessionEpoch + 1
	}

	if s.settings.Session.MaxSessionsPerServerID == 1 {
		if err := s.kickConflictingSessions(ctx, hello.ServerId, "", logger); err != nil {
			return nil, nil, err
		}
	}

	// Convert client features from proto map to Go map
	clientFeatures := make(map[string]bool)
	for k, v := range hello.ClientFeatures {
		clientFeatures[k] = v
	}

	// Create new session (stream will be set by handler after sending WELCOME)
	session := types.NewSession(
		hello.ServerId,
		hello.OrgId,
		epoch,
		hello.DeviceFingerprint,
		clientFeatures,
		nil, // stream will be set by handler
	)

	// Persist session
	if err := s.repo.CreateSession(ctx, session); err != nil {
		return nil, nil, fmt.Errorf("failed to create session: %w", err)
	}

	// Register in memory
	s.sessions.Store(hello.ServerId, session)

	// Record metric: new session created
	s.metrics.SessionsActive.Inc()

	// Build WelcomeFrame
	welcome := &umsv1.WelcomeFrame{
		SessionId:    session.SessionID,
		SessionEpoch: session.SessionEpoch,
		ResumeToken:  session.ResumeToken,
		ServerTimeMs: time.Now().UnixMilli(),
		Policy: &umsv1.SessionPolicy{
			MaxInflightTotal:         int32(s.settings.FlowControl.MaxInflightTotal),
			MaxInflightCommand:       int32(s.settings.FlowControl.MaxInflightCommand),
			MaxInflightControl:       int32(s.settings.FlowControl.MaxInflightControl),
			MaxInflightTelemetry:     int32(s.settings.FlowControl.MaxInflightTelemetry),
			MaxBatchBytes:            int32(s.settings.FlowControl.MaxBatchBytes),
			HeartbeatIntervalSeconds: int32(s.settings.Session.HeartbeatIntervalSeconds),
		},
	}

	logger.Info("session created", "session_id", session.SessionID, "epoch", epoch)
	return welcome, session, nil
}

// HandleResume processes a session resumption request
func (s *sessionServiceImpl) HandleResume(ctx context.Context, resumeToken string) (*umsv1.ResumeOkFrame, *types.Session, error) {
	logger := s.logger.With("resume_token", resumeToken)
	logger.Info("processing session resume")

	// Find session by resume token
	session, err := s.repo.GetSessionByResumeToken(ctx, resumeToken)
	if err != nil {
		logger.Warn("session resume failed", "error", err)
		return nil, nil, fmt.Errorf("session resume failed: invalid or expired resume token")
	}

	if session == nil {
		logger.Warn("session not found for resume token")
		return nil, nil, fmt.Errorf("session not found")
	}

	logger = logger.With("session_id", session.SessionID, "server_id", session.ServerID)
	if s.settings.Session.MaxSessionsPerServerID == 1 {
		if err := s.kickConflictingSessions(ctx, session.ServerID, session.SessionID, logger); err != nil {
			return nil, nil, err
		}
	}

	// Advance session epoch for this resume
	oldEpoch := session.SessionEpoch
	session.SessionEpoch++
	logger.Info("session resumed", "old_epoch", oldEpoch, "new_epoch", session.SessionEpoch)

	// Generate new resume token
	session.RotateResumeToken()

	// Record metric: successful reconnection
	s.metrics.ReconnectsTotal.Inc()

	// Persist the updated session
	if err := s.repo.UpdateResumeToken(ctx, session.SessionID, session.ResumeToken); err != nil {
		logger.Error("failed to rotate resume token", "error", err)
		return nil, nil, fmt.Errorf("failed to update session: %w", err)
	}

	// Fetch ack positions for replay (keyed by server_id so they survive session rotation)
	ackPositions, err := s.repo.GetAckPositions(ctx, session.ServerID)
	if err != nil {
		logger.Error("failed to get ack positions", "error", err)
		return nil, nil, fmt.Errorf("failed to get ack positions: %w", err)
	}

	// Build ResumeOkFrame
	resumeOk := &umsv1.ResumeOkFrame{
		SessionId:    session.SessionID,
		SessionEpoch: session.SessionEpoch,
		ResumeToken:  session.ResumeToken,
		LastAckedSeq: ackPositions,
		Policy:       nil, // TODO: Fetch SessionPolicy from config or database
	}

	logger.Info("session resume successful", "ack_positions_count", len(ackPositions))
	return resumeOk, session, nil
}

func (s *sessionServiceImpl) getLiveSessionByServerID(serverID string) *types.Session {
	if live, ok := s.sessions.Load(serverID); ok {
		return live.(*types.Session)
	}
	return nil
}

func (s *sessionServiceImpl) kickConflictingSessions(ctx context.Context, serverID, keepSessionID string, logger *slog.Logger) error {
	kicked := make(map[string]struct{})
	kickIfNeeded := func(session *types.Session, source string) error {
		if session == nil || session.SessionID == keepSessionID {
			return nil
		}
		if _, alreadyKicked := kicked[session.SessionID]; alreadyKicked {
			return nil
		}
		if err := s.kickSession(ctx, session, logger, source); err != nil {
			return err
		}
		kicked[session.SessionID] = struct{}{}
		return nil
	}

	if liveSession := s.getLiveSessionByServerID(serverID); liveSession != nil && liveSession.SessionID != keepSessionID {
		if err := kickIfNeeded(liveSession, "live"); err != nil {
			return err
		}
	}

	persistedSession, err := s.repo.GetSessionByServerID(ctx, serverID)
	if err != nil {
		return fmt.Errorf("failed to check latest session for server %s: %w", serverID, err)
	}
	if persistedSession != nil && persistedSession.SessionID != keepSessionID {
		if err := kickIfNeeded(persistedSession, "persisted"); err != nil {
			return err
		}
	}

	return nil
}

func (s *sessionServiceImpl) kickSession(ctx context.Context, session *types.Session, logger *slog.Logger, source string) error {
	if session == nil {
		return nil
	}

	logger.Info("kicking conflicting session", "old_session_id", session.SessionID, "source", source)
	if s.appSvc != nil && s.appSvc.GetDeliveryService() != nil {
		if err := s.appSvc.GetDeliveryService().StopDeliveryLoop(ctx, session.SessionID); err != nil {
			return fmt.Errorf("failed to stop delivery loop for session %s: %w", session.SessionID, err)
		}
	}
	if err := s.UnregisterSession(ctx, session.SessionID); err != nil {
		return fmt.Errorf("failed to unregister session %s: %w", session.SessionID, err)
	}
	if err := s.repo.DeleteSession(ctx, session.SessionID); err != nil {
		return fmt.Errorf("failed to delete session %s: %w", session.SessionID, err)
	}
	return nil
}

// HandleHeartbeat updates the last heartbeat timestamp
func (s *sessionServiceImpl) HandleHeartbeat(ctx context.Context, sessionID string) error {
	return s.repo.UpdateHeartbeat(ctx, sessionID)
}

// EvictStale removes sessions that have exceeded heartbeat timeout
func (s *sessionServiceImpl) EvictStale(ctx context.Context) error {
	// TODO: Query sessions where last_heartbeat is older than timeout
	// For each stale session: UnregisterSession + DeleteSession
	return nil
}

// GetSession retrieves a session by ID (from in-memory or DB)
func (s *sessionServiceImpl) GetSession(ctx context.Context, sessionID string) (*types.Session, error) {
	// Check in-memory first
	var found *types.Session
	s.sessions.Range(func(key, value interface{}) bool {
		session := value.(*types.Session)
		if session.SessionID == sessionID {
			found = session
			return false // stop iteration
		}
		return true // continue iteration
	})

	if found != nil {
		return found, nil
	}

	// Fallback to DB
	return s.repo.GetSession(ctx, sessionID)
}

// RegisterSession adds a session to the in-memory registry
func (s *sessionServiceImpl) RegisterSession(ctx context.Context, session *types.Session) error {
	s.sessions.Store(session.ServerID, session)
	return nil
}

// UnregisterSession removes a session from the in-memory registry
func (s *sessionServiceImpl) UnregisterSession(ctx context.Context, sessionID string) error {
	// Find and delete from sync.Map
	s.sessions.Range(func(key, value interface{}) bool {
		session := value.(*types.Session)
		if session.SessionID == sessionID {
			s.sessions.Delete(key)
			// Record metric: session ended
			s.metrics.SessionsActive.Dec()
			return false
		}
		return true
	})

	return nil
}

// ListActiveSessions returns all currently connected sessions from the in-memory registry.
func (s *sessionServiceImpl) ListActiveSessions(_ context.Context) ([]*types.Session, error) {
	var sessions []*types.Session
	s.sessions.Range(func(_, value interface{}) bool {
		sessions = append(sessions, value.(*types.Session))
		return true
	})
	return sessions, nil
}
