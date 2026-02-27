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

	// Check if there's an existing session for this server_id
	existingSession, err := s.repo.GetSessionByServerID(ctx, hello.ServerId)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to check existing session: %w", err)
	}

	var epoch uint64 = 1
	if existingSession != nil {
		// Enforce max_sessions_per_server_id policy (kick old session)
		if s.settings.Session.MaxSessionsPerServerID == 1 {
			logger.Info("kicking existing session", "old_session_id", existingSession.SessionID)
			s.UnregisterSession(ctx, existingSession.SessionID)
			s.repo.DeleteSession(ctx, existingSession.SessionID)
		}
		epoch = existingSession.SessionEpoch + 1
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

	// Fetch ack positions for replay
	ackPositions, err := s.repo.GetAckPositions(ctx, session.SessionID)
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
