package service

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"

	"github.com/ambientlabscomputing/mycelium_spine/internal/metrics"
	"github.com/ambientlabscomputing/mycelium_spine/internal/types"
	"github.com/ambientlabscomputing/mycelium_spine/internal/utils"
	umsv1 "github.com/ambientlabscomputing/mycelium_spine/proto/ums/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testMetrics *metrics.Metrics

func init() {
	// Initialize logger for tests
	utils.Logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	// Initialize shared metrics instance
	if testMetrics == nil {
		testMetrics = metrics.NewMetrics()
	}
}

// MockRepository implements repository.Repository for testing
type MockRepository struct {
	// Session methods
	CreateSessionFunc           func(ctx context.Context, session *types.Session) error
	GetSessionFunc              func(ctx context.Context, sessionID string) (*types.Session, error)
	GetSessionByServerIDFunc    func(ctx context.Context, serverID string) (*types.Session, error)
	GetSessionByResumeTokenFunc func(ctx context.Context, resumeToken string) (*types.Session, error)
	UpdateResumeTokenFunc       func(ctx context.Context, sessionID string, newToken string) error
	UpdateAckPositionFunc       func(ctx context.Context, sessionID string, mailboxID string, seq uint64) error
	GetAckPositionsFunc         func(ctx context.Context, sessionID string) (map[string]uint64, error)
	DeleteSessionFunc           func(ctx context.Context, sessionID string) error
	UpdateHeartbeatFunc         func(ctx context.Context, sessionID string) error
	UpdateSubscriptionsFunc     func(ctx context.Context, sessionID string, mailboxIDs []string) error

	// Mailbox methods
	GetOrCreateMailboxFunc func(ctx context.Context, targetType types.TargetType, targetID string, orgID string) (*types.Mailbox, error)
	GetMailboxFunc         func(ctx context.Context, mailboxID string) (*types.Mailbox, error)
	AppendEnvelopeFunc     func(ctx context.Context, mailboxID string, envelope *types.Envelope) (uint64, error)
	FetchEnvelopesFunc     func(ctx context.Context, mailboxID string, fromSeq uint64, limit int) ([]*types.Envelope, error)
	DeleteExpiredFunc      func(ctx context.Context, mailboxID string) (int64, error)
	EnforceRetentionFunc   func(ctx context.Context, mailboxID string, policy types.RetentionPolicy) error

	// Target resolver
	ResolveTargetsFunc func(ctx context.Context, targets []*types.Target) ([]string, error)
}

// Session methods
func (m *MockRepository) CreateSession(ctx context.Context, session *types.Session) error {
	if m.CreateSessionFunc != nil {
		return m.CreateSessionFunc(ctx, session)
	}
	return nil
}

func (m *MockRepository) GetSession(ctx context.Context, sessionID string) (*types.Session, error) {
	if m.GetSessionFunc != nil {
		return m.GetSessionFunc(ctx, sessionID)
	}
	return nil, nil
}

func (m *MockRepository) GetSessionByServerID(ctx context.Context, serverID string) (*types.Session, error) {
	if m.GetSessionByServerIDFunc != nil {
		return m.GetSessionByServerIDFunc(ctx, serverID)
	}
	return nil, nil
}

func (m *MockRepository) GetSessionByResumeToken(ctx context.Context, resumeToken string) (*types.Session, error) {
	if m.GetSessionByResumeTokenFunc != nil {
		return m.GetSessionByResumeTokenFunc(ctx, resumeToken)
	}
	return nil, nil
}

func (m *MockRepository) UpdateResumeToken(ctx context.Context, sessionID string, newToken string) error {
	if m.UpdateResumeTokenFunc != nil {
		return m.UpdateResumeTokenFunc(ctx, sessionID, newToken)
	}
	return nil
}

func (m *MockRepository) UpdateAckPosition(ctx context.Context, sessionID string, mailboxID string, seq uint64) error {
	if m.UpdateAckPositionFunc != nil {
		return m.UpdateAckPositionFunc(ctx, sessionID, mailboxID, seq)
	}
	return nil
}

func (m *MockRepository) GetAckPositions(ctx context.Context, sessionID string) (map[string]uint64, error) {
	if m.GetAckPositionsFunc != nil {
		return m.GetAckPositionsFunc(ctx, sessionID)
	}
	return make(map[string]uint64), nil
}

func (m *MockRepository) DeleteSession(ctx context.Context, sessionID string) error {
	if m.DeleteSessionFunc != nil {
		return m.DeleteSessionFunc(ctx, sessionID)
	}
	return nil
}

func (m *MockRepository) UpdateHeartbeat(ctx context.Context, sessionID string) error {
	if m.UpdateHeartbeatFunc != nil {
		return m.UpdateHeartbeatFunc(ctx, sessionID)
	}
	return nil
}

func (m *MockRepository) UpdateSubscriptions(ctx context.Context, sessionID string, mailboxIDs []string) error {
	if m.UpdateSubscriptionsFunc != nil {
		return m.UpdateSubscriptionsFunc(ctx, sessionID, mailboxIDs)
	}
	return nil
}

// Mailbox methods
func (m *MockRepository) GetOrCreateMailbox(ctx context.Context, targetType types.TargetType, targetID string, orgID string) (*types.Mailbox, error) {
	if m.GetOrCreateMailboxFunc != nil {
		return m.GetOrCreateMailboxFunc(ctx, targetType, targetID, orgID)
	}
	return nil, nil
}

func (m *MockRepository) GetMailbox(ctx context.Context, mailboxID string) (*types.Mailbox, error) {
	if m.GetMailboxFunc != nil {
		return m.GetMailboxFunc(ctx, mailboxID)
	}
	return nil, nil
}

func (m *MockRepository) AppendEnvelope(ctx context.Context, mailboxID string, envelope *types.Envelope) (uint64, error) {
	if m.AppendEnvelopeFunc != nil {
		return m.AppendEnvelopeFunc(ctx, mailboxID, envelope)
	}
	return 0, nil
}

func (m *MockRepository) FetchEnvelopes(ctx context.Context, mailboxID string, fromSeq uint64, limit int) ([]*types.Envelope, error) {
	if m.FetchEnvelopesFunc != nil {
		return m.FetchEnvelopesFunc(ctx, mailboxID, fromSeq, limit)
	}
	return nil, nil
}

func (m *MockRepository) DeleteExpired(ctx context.Context, mailboxID string) (int64, error) {
	if m.DeleteExpiredFunc != nil {
		return m.DeleteExpiredFunc(ctx, mailboxID)
	}
	return 0, nil
}

func (m *MockRepository) EnforceRetention(ctx context.Context, mailboxID string, policy types.RetentionPolicy) error {
	if m.EnforceRetentionFunc != nil {
		return m.EnforceRetentionFunc(ctx, mailboxID, policy)
	}
	return nil
}

// Target resolver
func (m *MockRepository) ResolveTargets(ctx context.Context, targets []*types.Target) ([]string, error) {
	if m.ResolveTargetsFunc != nil {
		return m.ResolveTargetsFunc(ctx, targets)
	}
	return nil, nil
}

// Helper to create default settings for tests
func createTestSettings() *utils.Settings {
	return &utils.Settings{
		Session: struct {
			HeartbeatIntervalSeconds   int `yaml:"heartbeat_interval_seconds"`
			HeartbeatTimeoutMultiplier int `yaml:"heartbeat_timeout_multiplier"`
			ResumeTokenTTLSeconds      int `yaml:"resume_token_ttl_seconds"`
			MaxSessionsPerServerID     int `yaml:"max_sessions_per_server_id"`
		}{
			HeartbeatIntervalSeconds:   30,
			HeartbeatTimeoutMultiplier: 3,
			ResumeTokenTTLSeconds:      300,
			MaxSessionsPerServerID:     1,
		},
		FlowControl: struct {
			MaxInflightTotal     int `yaml:"max_inflight_total"`
			MaxInflightCommand   int `yaml:"max_inflight_command"`
			MaxInflightControl   int `yaml:"max_inflight_control"`
			MaxInflightTelemetry int `yaml:"max_inflight_telemetry"`
			MaxBatchBytes        int `yaml:"max_batch_bytes"`
		}{
			MaxInflightTotal:     1000,
			MaxInflightCommand:   100,
			MaxInflightControl:   200,
			MaxInflightTelemetry: 300,
			MaxBatchBytes:        1024 * 1024,
		},
	}
}

func TestHandleHello_NewSession(t *testing.T) {
	mockRepo := &MockRepository{
		GetSessionByServerIDFunc: func(ctx context.Context, serverID string) (*types.Session, error) {
			return nil, nil // No existing session
		},
		CreateSessionFunc: func(ctx context.Context, session *types.Session) error {
			return nil
		},
	}

	settings := createTestSettings()
	appSvc := &AppService{}
	// Use shared testMetrics
	svc := NewSessionService(mockRepo, settings, appSvc, testMetrics)

	hello := &umsv1.HelloFrame{
		ServerId:          "test-server-1",
		OrgId:             "org-123",
		DeviceFingerprint: "fingerprint-abc",
		ClientFeatures: map[string]bool{
			"compression": true,
		},
	}

	ctx := context.Background()
	welcome, session, err := svc.HandleHello(ctx, hello)

	require.NoError(t, err)
	require.NotNil(t, welcome)
	require.NotNil(t, session)

	assert.Equal(t, hello.ServerId, session.ServerID)
	assert.Equal(t, hello.OrgId, session.OrgID)
	assert.Equal(t, uint64(1), session.SessionEpoch)
	assert.NotEmpty(t, session.SessionID)
	assert.NotEmpty(t, session.ResumeToken)

	assert.Equal(t, session.SessionID, welcome.SessionId)
	assert.Equal(t, session.SessionEpoch, welcome.SessionEpoch)
	assert.Equal(t, session.ResumeToken, welcome.ResumeToken)
	assert.NotNil(t, welcome.Policy)
	assert.Equal(t, int32(settings.FlowControl.MaxInflightTotal), welcome.Policy.MaxInflightTotal)
}

func TestHandleHello_ExistingSession_KickedOut(t *testing.T) {
	existingSession := types.NewSession("test-server-1", "org-123", 1, "old-fingerprint", nil, nil)

	mockRepo := &MockRepository{
		GetSessionByServerIDFunc: func(ctx context.Context, serverID string) (*types.Session, error) {
			return existingSession, nil
		},
		CreateSessionFunc: func(ctx context.Context, session *types.Session) error {
			return nil
		},
		DeleteSessionFunc: func(ctx context.Context, sessionID string) error {
			assert.Equal(t, existingSession.SessionID, sessionID)
			return nil
		},
	}

	settings := createTestSettings()
	settings.Session.MaxSessionsPerServerID = 1
	appSvc := &AppService{}
	// Use shared testMetrics
	svc := NewSessionService(mockRepo, settings, appSvc, testMetrics)

	hello := &umsv1.HelloFrame{
		ServerId:          "test-server-1",
		OrgId:             "org-123",
		DeviceFingerprint: "new-fingerprint",
		ClientFeatures:    map[string]bool{},
	}

	ctx := context.Background()
	welcome, session, err := svc.HandleHello(ctx, hello)

	require.NoError(t, err)
	require.NotNil(t, welcome)
	require.NotNil(t, session)

	// New session should have incremented epoch
	assert.Equal(t, uint64(2), session.SessionEpoch)
}

func TestHandleHello_CreateSessionError(t *testing.T) {
	mockRepo := &MockRepository{
		GetSessionByServerIDFunc: func(ctx context.Context, serverID string) (*types.Session, error) {
			return nil, nil
		},
		CreateSessionFunc: func(ctx context.Context, session *types.Session) error {
			return errors.New("database error")
		},
	}

	settings := createTestSettings()
	appSvc := &AppService{}
	// Use shared testMetrics
	svc := NewSessionService(mockRepo, settings, appSvc, testMetrics)

	hello := &umsv1.HelloFrame{
		ServerId: "test-server-1",
		OrgId:    "org-123",
	}

	ctx := context.Background()
	_, _, err := svc.HandleHello(ctx, hello)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create session")
}

func TestHandleResume_Success(t *testing.T) {
	existingSession := types.NewSession("test-server-1", "org-123", 3, "fingerprint", nil, nil)
	oldResumeToken := existingSession.ResumeToken
	oldEpoch := existingSession.SessionEpoch

	ackPositions := map[string]uint64{
		"mailbox-1": 10,
		"mailbox-2": 25,
	}

	mockRepo := &MockRepository{
		GetSessionByResumeTokenFunc: func(ctx context.Context, resumeToken string) (*types.Session, error) {
			if resumeToken == oldResumeToken {
				return existingSession, nil
			}
			return nil, errors.New("invalid token")
		},
		UpdateResumeTokenFunc: func(ctx context.Context, sessionID string, newToken string) error {
			assert.Equal(t, existingSession.SessionID, sessionID)
			assert.NotEqual(t, oldResumeToken, newToken)
			return nil
		},
		GetAckPositionsFunc: func(ctx context.Context, sessionID string) (map[string]uint64, error) {
			return ackPositions, nil
		},
	}

	settings := createTestSettings()
	appSvc := &AppService{}
	// Use shared testMetrics
	svc := NewSessionService(mockRepo, settings, appSvc, testMetrics)

	ctx := context.Background()
	resumeOk, session, err := svc.HandleResume(ctx, oldResumeToken)

	require.NoError(t, err)
	require.NotNil(t, resumeOk)
	require.NotNil(t, session)

	// Session epoch should be incremented
	assert.Equal(t, oldEpoch+1, session.SessionEpoch)
	assert.Equal(t, oldEpoch+1, resumeOk.SessionEpoch)

	// Resume token should be rotated
	assert.NotEqual(t, oldResumeToken, session.ResumeToken)
	assert.Equal(t, session.ResumeToken, resumeOk.ResumeToken)

	// Ack positions should be returned
	assert.Equal(t, ackPositions, resumeOk.LastAckedSeq)
}

func TestHandleResume_InvalidToken(t *testing.T) {
	mockRepo := &MockRepository{
		GetSessionByResumeTokenFunc: func(ctx context.Context, resumeToken string) (*types.Session, error) {
			return nil, errors.New("invalid token")
		},
	}

	settings := createTestSettings()
	appSvc := &AppService{}
	// Use shared testMetrics
	svc := NewSessionService(mockRepo, settings, appSvc, testMetrics)

	ctx := context.Background()
	_, _, err := svc.HandleResume(ctx, "invalid-token")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "session resume failed")
}

func TestHandleResume_UpdateTokenError(t *testing.T) {
	existingSession := types.NewSession("test-server-1", "org-123", 1, "fingerprint", nil, nil)

	mockRepo := &MockRepository{
		GetSessionByResumeTokenFunc: func(ctx context.Context, resumeToken string) (*types.Session, error) {
			return existingSession, nil
		},
		UpdateResumeTokenFunc: func(ctx context.Context, sessionID string, newToken string) error {
			return errors.New("database error")
		},
	}

	settings := createTestSettings()
	appSvc := &AppService{}
	// Use shared testMetrics
	svc := NewSessionService(mockRepo, settings, appSvc, testMetrics)

	ctx := context.Background()
	_, _, err := svc.HandleResume(ctx, existingSession.ResumeToken)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to update session")
}

func TestHandleHeartbeat_Success(t *testing.T) {
	sessionID := "test-session-1"
	heartbeatCalled := false

	mockRepo := &MockRepository{
		UpdateHeartbeatFunc: func(ctx context.Context, sid string) error {
			assert.Equal(t, sessionID, sid)
			heartbeatCalled = true
			return nil
		},
	}

	settings := createTestSettings()
	appSvc := &AppService{}
	// Use shared testMetrics
	svc := NewSessionService(mockRepo, settings, appSvc, testMetrics)

	ctx := context.Background()
	err := svc.HandleHeartbeat(ctx, sessionID)

	require.NoError(t, err)
	assert.True(t, heartbeatCalled)
}

func TestHandleHeartbeat_Error(t *testing.T) {
	mockRepo := &MockRepository{
		UpdateHeartbeatFunc: func(ctx context.Context, sessionID string) error {
			return errors.New("database error")
		},
	}

	settings := createTestSettings()
	appSvc := &AppService{}
	// Use shared testMetrics
	svc := NewSessionService(mockRepo, settings, appSvc, testMetrics)

	ctx := context.Background()
	err := svc.HandleHeartbeat(ctx, "test-session-1")

	require.Error(t, err)
}

func TestGetSession_InMemory(t *testing.T) {
	session := types.NewSession("test-server-1", "org-123", 1, "fingerprint", nil, nil)

	mockRepo := &MockRepository{}

	settings := createTestSettings()
	appSvc := &AppService{}
	// Use shared testMetrics
	svc := NewSessionService(mockRepo, settings, appSvc, testMetrics)

	// Register session
	ctx := context.Background()
	err := svc.RegisterSession(ctx, session)
	require.NoError(t, err)

	// Retrieve session
	retrieved, err := svc.GetSession(ctx, session.SessionID)
	require.NoError(t, err)
	require.NotNil(t, retrieved)
	assert.Equal(t, session.SessionID, retrieved.SessionID)
}

func TestGetSession_FromRepository(t *testing.T) {
	session := types.NewSession("test-server-1", "org-123", 1, "fingerprint", nil, nil)

	mockRepo := &MockRepository{
		GetSessionFunc: func(ctx context.Context, sessionID string) (*types.Session, error) {
			if sessionID == session.SessionID {
				return session, nil
			}
			return nil, errors.New("not found")
		},
	}

	settings := createTestSettings()
	appSvc := &AppService{}
	// Use shared testMetrics
	svc := NewSessionService(mockRepo, settings, appSvc, testMetrics)

	ctx := context.Background()
	retrieved, err := svc.GetSession(ctx, session.SessionID)

	require.NoError(t, err)
	require.NotNil(t, retrieved)
	assert.Equal(t, session.SessionID, retrieved.SessionID)
}

func TestRegisterSession(t *testing.T) {
	session := types.NewSession("test-server-1", "org-123", 1, "fingerprint", nil, nil)

	mockRepo := &MockRepository{}
	settings := createTestSettings()
	appSvc := &AppService{}
	// Use shared testMetrics
	svc := NewSessionService(mockRepo, settings, appSvc, testMetrics)

	ctx := context.Background()
	err := svc.RegisterSession(ctx, session)
	require.NoError(t, err)

	// Verify session can be retrieved
	retrieved, err := svc.GetSession(ctx, session.SessionID)
	require.NoError(t, err)
	require.NotNil(t, retrieved)
	assert.Equal(t, session.ServerID, retrieved.ServerID)
}

func TestUnregisterSession(t *testing.T) {
	session := types.NewSession("test-server-1", "org-123", 1, "fingerprint", nil, nil)

	mockRepo := &MockRepository{}
	settings := createTestSettings()
	appSvc := &AppService{}
	// Use shared testMetrics
	svc := NewSessionService(mockRepo, settings, appSvc, testMetrics)

	ctx := context.Background()

	// Register session
	err := svc.RegisterSession(ctx, session)
	require.NoError(t, err)

	// Unregister session
	err = svc.UnregisterSession(ctx, session.SessionID)
	require.NoError(t, err)

	// Verify session is no longer in memory (will try DB)
	mockRepo.GetSessionFunc = func(ctx context.Context, sessionID string) (*types.Session, error) {
		return nil, errors.New("not found")
	}
	retrieved, err := svc.GetSession(ctx, session.SessionID)
	require.Error(t, err)
	require.Nil(t, retrieved)
}

func TestEvictStale(t *testing.T) {
	mockRepo := &MockRepository{}
	settings := createTestSettings()
	appSvc := &AppService{}
	// Use shared testMetrics
	svc := NewSessionService(mockRepo, settings, appSvc, testMetrics)

	ctx := context.Background()
	// TODO: This is a placeholder implementation that returns nil
	err := svc.EvictStale(ctx)
	require.NoError(t, err)
}
