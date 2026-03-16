package grpc

import (
	"context"
	"io"
	"log/slog"
	"net"
	"os"
	"testing"
	"time"

	"github.com/ambientlabscomputing/mycelium_spine/internal/service"
	"github.com/ambientlabscomputing/mycelium_spine/internal/types"
	"github.com/ambientlabscomputing/mycelium_spine/internal/utils"
	umsv1 "github.com/ambientlabscomputing/mycelium_spine/proto/ums/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

const bufSize = 1024 * 1024

func init() {
	// Initialize logger for tests
	if utils.Logger == nil {
		utils.Logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelError, // Use ERROR level to reduce test output noise
		}))
	}
}

// MockAppService implements service.Service for testing
type MockAppService struct {
	mock.Mock
	sessionSvc   *MockSessionService
	deliverySvc  *MockDeliveryService
	publishSvc   *MockPublishService
	ackSvc       *MockAckService
	subscribeSvc *MockSubscribeService
}

func (m *MockAppService) Start(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockAppService) Stop(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockAppService) GetSessionService() service.SessionService {
	return m.sessionSvc
}

func (m *MockAppService) GetDeliveryService() service.DeliveryService {
	return m.deliverySvc
}

func (m *MockAppService) GetPublishService() service.PublishService {
	return m.publishSvc
}

func (m *MockAppService) GetAckService() service.AckService {
	return m.ackSvc
}

func (m *MockAppService) GetSubscribeService() service.SubscribeService {
	return m.subscribeSvc
}

// MockSessionService mocks SessionService
type MockSessionService struct {
	mock.Mock
}

func (m *MockSessionService) HandleHello(ctx context.Context, hello *umsv1.HelloFrame) (*umsv1.WelcomeFrame, *types.Session, error) {
	args := m.Called(ctx, hello)
	if args.Get(0) == nil {
		return nil, nil, args.Error(2)
	}
	return args.Get(0).(*umsv1.WelcomeFrame), args.Get(1).(*types.Session), args.Error(2)
}

func (m *MockSessionService) HandleResume(ctx context.Context, resumeToken string) (*umsv1.ResumeOkFrame, *types.Session, error) {
	args := m.Called(ctx, resumeToken)
	if args.Get(0) == nil {
		return nil, nil, args.Error(2)
	}
	return args.Get(0).(*umsv1.ResumeOkFrame), args.Get(1).(*types.Session), args.Error(2)
}

func (m *MockSessionService) HandleHeartbeat(ctx context.Context, sessionID string) error {
	args := m.Called(ctx, sessionID)
	return args.Error(0)
}

func (m *MockSessionService) EvictStale(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockSessionService) GetSession(ctx context.Context, sessionID string) (*types.Session, error) {
	args := m.Called(ctx, sessionID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*types.Session), args.Error(1)
}

func (m *MockSessionService) RegisterSession(ctx context.Context, session *types.Session) error {
	args := m.Called(ctx, session)
	return args.Error(0)
}

func (m *MockSessionService) UnregisterSession(ctx context.Context, sessionID string) error {
	args := m.Called(ctx, sessionID)
	return args.Error(0)
}

// MockDeliveryService mocks DeliveryService
type MockDeliveryService struct {
	mock.Mock
}

func (m *MockDeliveryService) StartDeliveryLoop(ctx context.Context, session *types.Session) error {
	args := m.Called(ctx, session)
	return args.Error(0)
}

func (m *MockDeliveryService) StopDeliveryLoop(ctx context.Context, sessionID string) error {
	args := m.Called(ctx, sessionID)
	return args.Error(0)
}

func (m *MockDeliveryService) DeliverToSession(ctx context.Context, session *types.Session, envelopes []*types.Envelope) error {
	args := m.Called(ctx, session, envelopes)
	return args.Error(0)
}

func (m *MockDeliveryService) HandleBackpressure(ctx context.Context, session *types.Session, hint *umsv1.FlowHintFrame) error {
	args := m.Called(ctx, session, hint)
	return args.Error(0)
}

func (m *MockDeliveryService) NotifyDeliveryLoops() {
	m.Called()
}

// MockPublishService mocks PublishService
type MockPublishService struct {
	mock.Mock
}

func (m *MockPublishService) Publish(ctx context.Context, envelope *types.Envelope, targets []*types.Target) (*service.PublishResult, error) {
	args := m.Called(ctx, envelope, targets)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*service.PublishResult), args.Error(1)
}

// MockAckService mocks AckService
type MockAckService struct {
	mock.Mock
}

func (m *MockAckService) HandleCumulativeAck(ctx context.Context, sessionID string, mailboxID string, seqAcked uint64) (uint64, error) {
	args := m.Called(ctx, sessionID, mailboxID, seqAcked)
	return args.Get(0).(uint64), args.Error(1)
}

func (m *MockAckService) HandleSelectiveAck(ctx context.Context, sessionID string, mailboxID string, seqs []uint64) error {
	args := m.Called(ctx, sessionID, mailboxID, seqs)
	return args.Error(0)
}

func (m *MockAckService) HandleNack(ctx context.Context, sessionID string, mailboxID string, seq uint64, reason string) error {
	args := m.Called(ctx, sessionID, mailboxID, seq, reason)
	return args.Error(0)
}

// MockSubscribeService mocks SubscribeService
type MockSubscribeService struct {
	mock.Mock
}

func (m *MockSubscribeService) HandleSubscribe(ctx context.Context, session *types.Session, targets []*types.Target) (*umsv1.SubscribeOkFrame, error) {
	args := m.Called(ctx, session, targets)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*umsv1.SubscribeOkFrame), args.Error(1)
}

// testServer holds the test gRPC server and bufconn listener
type testServer struct {
	lis      *bufconn.Listener
	server   *grpc.Server
	mockSvc  *MockAppService
	mockSess *MockSessionService
	mockDel  *MockDeliveryService
	mockAck  *MockAckService
	mockSub  *MockSubscribeService
	mockPub  *MockPublishService
}

// setupTestServer creates a test gRPC server with bufconn
func setupTestServer(t *testing.T) *testServer {
	lis := bufconn.Listen(bufSize)

	// Create mock services
	mockSess := &MockSessionService{}
	mockDel := &MockDeliveryService{}
	mockAck := &MockAckService{}
	mockSub := &MockSubscribeService{}
	mockPub := &MockPublishService{}

	mockSvc := &MockAppService{
		sessionSvc:   mockSess,
		deliverySvc:  mockDel,
		publishSvc:   mockPub,
		ackSvc:       mockAck,
		subscribeSvc: mockSub,
	}

	// Create handler
	handler := NewStreamHandler(mockSvc)

	// Create gRPC server
	server := grpc.NewServer()
	umsv1.RegisterSpineStreamServer(server, handler)

	// Start server in background
	go func() {
		if err := server.Serve(lis); err != nil {
			t.Logf("Server exited with error: %v", err)
		}
	}()

	return &testServer{
		lis:      lis,
		server:   server,
		mockSvc:  mockSvc,
		mockSess: mockSess,
		mockDel:  mockDel,
		mockAck:  mockAck,
		mockSub:  mockSub,
		mockPub:  mockPub,
	}
}

// createTestClient creates a gRPC client connected via bufconn
func (ts *testServer) createTestClient(ctx context.Context, t *testing.T) umsv1.SpineStreamClient {
	conn, err := grpc.DialContext(ctx, "bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return ts.lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		conn.Close()
	})

	return umsv1.NewSpineStreamClient(conn)
}

// shutdown stops the test server
func (ts *testServer) shutdown() {
	ts.server.GracefulStop()
	ts.lis.Close()
}

// TestStreamHandler_HelloFrame_NewSession tests the HELLO frame processing for a new session
func TestStreamHandler_HelloFrame_NewSession(t *testing.T) {
	ctx := context.Background()
	ts := setupTestServer(t)
	defer ts.shutdown()

	// Setup mock expectations
	welcomeFrame := &umsv1.WelcomeFrame{
		SessionId:    "test-session-123",
		SessionEpoch: 1,
		ResumeToken:  "resume-token-abc",
		ServerTimeMs: time.Now().UnixMilli(),
		Policy: &umsv1.SessionPolicy{
			MaxInflightTotal:         1000,
			MaxInflightCommand:       100,
			MaxInflightControl:       200,
			MaxInflightTelemetry:     700,
			MaxBatchBytes:            1024000,
			HeartbeatIntervalSeconds: 30,
		},
	}

	session := &types.Session{
		SessionID:    "test-session-123",
		ServerID:     "server-001",
		OrgID:        "org-001",
		SessionEpoch: 1,
		ResumeToken:  "resume-token-abc",
	}

	ts.mockSess.On("HandleHello", mock.Anything, mock.MatchedBy(func(hello *umsv1.HelloFrame) bool {
		return hello.ServerId == "server-001" && hello.ProtocolVersion == "1.0"
	})).Return(welcomeFrame, session, nil)

	ts.mockSess.On("RegisterSession", mock.Anything, mock.Anything).Return(nil)
	ts.mockDel.On("StartDeliveryLoop", mock.Anything, mock.Anything).Return(nil)
	ts.mockDel.On("StopDeliveryLoop", mock.Anything, "test-session-123").Return(nil)
	ts.mockSess.On("UnregisterSession", mock.Anything, "test-session-123").Return(nil)

	// Create client and stream
	client := ts.createTestClient(ctx, t)
	stream, err := client.Connect(ctx)
	require.NoError(t, err)

	// Send HELLO frame
	err = stream.Send(&umsv1.ClientFrame{
		Frame: &umsv1.ClientFrame_Hello{
			Hello: &umsv1.HelloFrame{
				ProtocolVersion: "1.0",
				ServerId:        "server-001",
				OrgId:           "org-001",
				ClientFeatures: map[string]bool{
					"supports_compression": true,
				},
			},
		},
	})
	require.NoError(t, err)

	// Receive WELCOME frame
	serverFrame, err := stream.Recv()
	require.NoError(t, err)
	require.NotNil(t, serverFrame.GetWelcome())

	welcome := serverFrame.GetWelcome()
	assert.Equal(t, "test-session-123", welcome.SessionId)
	assert.Equal(t, uint64(1), welcome.SessionEpoch)
	assert.Equal(t, "resume-token-abc", welcome.ResumeToken)
	assert.NotNil(t, welcome.Policy)

	// Close stream
	err = stream.CloseSend()
	require.NoError(t, err)

	// Wait for cleanup to complete
	time.Sleep(100 * time.Millisecond)

	// Verify mock calls
	ts.mockSess.AssertExpectations(t)
	ts.mockDel.AssertExpectations(t)
}

// TestStreamHandler_ResumeFrame_Success tests successful session resumption
func TestStreamHandler_ResumeFrame_Success(t *testing.T) {
	ctx := context.Background()
	ts := setupTestServer(t)
	defer ts.shutdown()

	// Setup mock expectations for resume
	resumeOkFrame := &umsv1.ResumeOkFrame{
		SessionId:    "test-session-123",
		SessionEpoch: 1,
		ResumeToken:  "new-resume-token",
		LastAckedSeq: map[string]uint64{
			"mailbox-1": 42,
			"mailbox-2": 15,
		},
		Policy: &umsv1.SessionPolicy{
			MaxInflightTotal:         1000,
			HeartbeatIntervalSeconds: 30,
		},
	}

	session := &types.Session{
		SessionID:    "test-session-123",
		ServerID:     "server-001",
		OrgID:        "org-001",
		SessionEpoch: 1,
		ResumeToken:  "new-resume-token",
	}

	ts.mockSess.On("HandleResume", mock.Anything, "old-resume-token").Return(resumeOkFrame, session, nil)
	ts.mockSess.On("RegisterSession", mock.Anything, mock.Anything).Return(nil)
	ts.mockDel.On("StartDeliveryLoop", mock.Anything, mock.Anything).Return(nil)
	ts.mockDel.On("StopDeliveryLoop", mock.Anything, "test-session-123").Return(nil)
	ts.mockSess.On("UnregisterSession", mock.Anything, "test-session-123").Return(nil)

	// Create client and stream
	client := ts.createTestClient(ctx, t)
	stream, err := client.Connect(ctx)
	require.NoError(t, err)

	// Send HELLO frame with resume token
	err = stream.Send(&umsv1.ClientFrame{
		Frame: &umsv1.ClientFrame_Hello{
			Hello: &umsv1.HelloFrame{
				ProtocolVersion: "1.0",
				ServerId:        "server-001",
				ResumeToken:     "old-resume-token",
			},
		},
	})
	require.NoError(t, err)

	// Receive RESUME_OK frame
	serverFrame, err := stream.Recv()
	require.NoError(t, err)
	require.NotNil(t, serverFrame.GetResumeOk())

	resumeOk := serverFrame.GetResumeOk()
	assert.Equal(t, "test-session-123", resumeOk.SessionId)
	assert.Equal(t, "new-resume-token", resumeOk.ResumeToken)
	assert.Equal(t, uint64(42), resumeOk.LastAckedSeq["mailbox-1"])

	// Close stream
	err = stream.CloseSend()
	require.NoError(t, err)

	// Wait for cleanup to complete
	time.Sleep(100 * time.Millisecond)

	ts.mockSess.AssertExpectations(t)
}

// TestStreamHandler_ResumeFrame_Denied tests failed session resumption
func TestStreamHandler_ResumeFrame_Denied(t *testing.T) {
	ctx := context.Background()
	ts := setupTestServer(t)
	defer ts.shutdown()

	// Setup mock expectations - resume fails, then hello succeeds
	ts.mockSess.On("HandleResume", mock.Anything, "invalid-token").Return(nil, nil, assert.AnError)

	welcomeFrame := &umsv1.WelcomeFrame{
		SessionId:    "new-session-456",
		SessionEpoch: 1,
		ResumeToken:  "new-token",
	}
	newSession := &types.Session{
		SessionID: "new-session-456",
		ServerID:  "server-001",
	}

	ts.mockSess.On("HandleHello", mock.Anything, mock.Anything).Return(welcomeFrame, newSession, nil)
	ts.mockSess.On("RegisterSession", mock.Anything, mock.Anything).Return(nil)
	ts.mockDel.On("StartDeliveryLoop", mock.Anything, mock.Anything).Return(nil)
	ts.mockDel.On("StopDeliveryLoop", mock.Anything, "new-session-456").Return(nil)
	ts.mockSess.On("UnregisterSession", mock.Anything, "new-session-456").Return(nil)

	// Create client and stream
	client := ts.createTestClient(ctx, t)
	stream, err := client.Connect(ctx)
	require.NoError(t, err)

	// Send HELLO frame with invalid resume token
	err = stream.Send(&umsv1.ClientFrame{
		Frame: &umsv1.ClientFrame_Hello{
			Hello: &umsv1.HelloFrame{
				ProtocolVersion: "1.0",
				ServerId:        "server-001",
				ResumeToken:     "invalid-token",
			},
		},
	})
	require.NoError(t, err)

	// Receive RESUME_DENIED frame
	serverFrame, err := stream.Recv()
	require.NoError(t, err)
	require.NotNil(t, serverFrame.GetResumeDenied())

	// Then receive WELCOME for new session
	serverFrame, err = stream.Recv()
	require.NoError(t, err)
	require.NotNil(t, serverFrame.GetWelcome())
	assert.Equal(t, "new-session-456", serverFrame.GetWelcome().SessionId)

	// Close stream
	err = stream.CloseSend()
	require.NoError(t, err)

	// Wait for cleanup to complete
	time.Sleep(100 * time.Millisecond)

	ts.mockSess.AssertExpectations(t)
}

// TestStreamHandler_SubscribeFrame tests SUBSCRIBE frame processing
func TestStreamHandler_SubscribeFrame(t *testing.T) {
	ctx := context.Background()
	ts := setupTestServer(t)
	defer ts.shutdown()

	// Setup mock expectations
	_ = setupMockSession(t, ts)

	subscribeOkFrame := &umsv1.SubscribeOkFrame{
		MailboxIds: []string{"mailbox-1", "mailbox-2", "mailbox-3"},
	}

	ts.mockSub.On("HandleSubscribe", mock.Anything, mock.Anything, mock.MatchedBy(func(targets []*types.Target) bool {
		return len(targets) == 2 && targets[0].TargetID == "server-001"
	})).Return(subscribeOkFrame, nil)

	// Create client and stream
	client := ts.createTestClient(ctx, t)
	stream, err := client.Connect(ctx)
	require.NoError(t, err)

	// Send HELLO
	sendHelloAndExpectWelcome(t, stream)

	// Send SUBSCRIBE frame
	err = stream.Send(&umsv1.ClientFrame{
		Frame: &umsv1.ClientFrame_Subscribe{
			Subscribe: &umsv1.SubscribeFrame{
				Targets: []*umsv1.Target{
					{
						TargetType: umsv1.TargetType_TARGET_TYPE_SERVER,
						TargetId:   "server-001",
						OrgId:      "org-001",
					},
					{
						TargetType: umsv1.TargetType_TARGET_TYPE_CLUSTER,
						TargetId:   "cluster-001",
						OrgId:      "org-001",
					},
				},
			},
		},
	})
	require.NoError(t, err)

	// Receive SUBSCRIBE_OK frame
	serverFrame, err := stream.Recv()
	require.NoError(t, err)
	require.NotNil(t, serverFrame.GetSubscribeOk())

	subscribeOk := serverFrame.GetSubscribeOk()
	assert.Len(t, subscribeOk.MailboxIds, 3)
	assert.Contains(t, subscribeOk.MailboxIds, "mailbox-1")

	// Close stream
	err = stream.CloseSend()
	require.NoError(t, err)

	ts.mockSub.AssertExpectations(t)
}

// TestStreamHandler_AckFrame_Cumulative tests cumulative ACK processing
func TestStreamHandler_AckFrame_Cumulative(t *testing.T) {
	ctx := context.Background()
	ts := setupTestServer(t)
	defer ts.shutdown()

	session := setupMockSession(t, ts)

	ts.mockAck.On("HandleCumulativeAck", mock.Anything, session.ServerID, "mailbox-1", uint64(42)).Return(uint64(0), nil)

	// Create client and stream
	client := ts.createTestClient(ctx, t)
	stream, err := client.Connect(ctx)
	require.NoError(t, err)

	// Send HELLO
	sendHelloAndExpectWelcome(t, stream)

	// Send ACK frame
	err = stream.Send(&umsv1.ClientFrame{
		Frame: &umsv1.ClientFrame_Ack{
			Ack: &umsv1.AckFrame{
				MailboxId: "mailbox-1",
				SeqAcked:  42,
			},
		},
	})
	require.NoError(t, err)

	// Give time for processing
	time.Sleep(50 * time.Millisecond)

	// Close stream
	err = stream.CloseSend()
	require.NoError(t, err)

	ts.mockAck.AssertExpectations(t)
}

// TestStreamHandler_AckSetFrame_Selective tests selective ACK processing
func TestStreamHandler_AckSetFrame_Selective(t *testing.T) {
	ctx := context.Background()
	ts := setupTestServer(t)
	defer ts.shutdown()

	session := setupMockSession(t, ts)

	ts.mockAck.On("HandleSelectiveAck", mock.Anything, session.SessionID, "mailbox-1",
		[]uint64{10, 15, 20, 25}).Return(nil)

	// Create client and stream
	client := ts.createTestClient(ctx, t)
	stream, err := client.Connect(ctx)
	require.NoError(t, err)

	// Send HELLO
	sendHelloAndExpectWelcome(t, stream)

	// Send ACK_SET frame
	err = stream.Send(&umsv1.ClientFrame{
		Frame: &umsv1.ClientFrame_AckSet{
			AckSet: &umsv1.AckSetFrame{
				MailboxId: "mailbox-1",
				Seqs:      []uint64{10, 15, 20, 25},
			},
		},
	})
	require.NoError(t, err)

	// Give time for processing
	time.Sleep(50 * time.Millisecond)

	// Close stream
	err = stream.CloseSend()
	require.NoError(t, err)

	ts.mockAck.AssertExpectations(t)
}

// TestStreamHandler_NackFrame tests NACK processing
func TestStreamHandler_NackFrame(t *testing.T) {
	ctx := context.Background()
	ts := setupTestServer(t)
	defer ts.shutdown()

	session := setupMockSession(t, ts)

	ts.mockAck.On("HandleNack", mock.Anything, session.SessionID, "mailbox-1", uint64(100), "malformed_payload").Return(nil)

	// Create client and stream
	client := ts.createTestClient(ctx, t)
	stream, err := client.Connect(ctx)
	require.NoError(t, err)

	// Send HELLO
	sendHelloAndExpectWelcome(t, stream)

	// Send NACK frame
	err = stream.Send(&umsv1.ClientFrame{
		Frame: &umsv1.ClientFrame_Nack{
			Nack: &umsv1.NackFrame{
				MailboxId: "mailbox-1",
				Seq:       100,
				Reason:    "malformed_payload",
			},
		},
	})
	require.NoError(t, err)

	// Give time for processing
	time.Sleep(50 * time.Millisecond)

	// Close stream
	err = stream.CloseSend()
	require.NoError(t, err)

	ts.mockAck.AssertExpectations(t)
}

// TestStreamHandler_PingPongFrame tests heartbeat processing
func TestStreamHandler_PingPongFrame(t *testing.T) {
	ctx := context.Background()
	ts := setupTestServer(t)
	defer ts.shutdown()

	session := setupMockSession(t, ts)

	ts.mockSess.On("HandleHeartbeat", mock.Anything, session.SessionID).Return(nil)

	// Create client and stream
	client := ts.createTestClient(ctx, t)
	stream, err := client.Connect(ctx)
	require.NoError(t, err)

	// Send HELLO
	sendHelloAndExpectWelcome(t, stream)

	// Send PING frame
	clientTime := time.Now().UnixMilli()
	err = stream.Send(&umsv1.ClientFrame{
		Frame: &umsv1.ClientFrame_Ping{
			Ping: &umsv1.PingFrame{
				ClientTimeMs: clientTime,
			},
		},
	})
	require.NoError(t, err)

	// Receive PONG frame
	serverFrame, err := stream.Recv()
	require.NoError(t, err)
	require.NotNil(t, serverFrame.GetPong())

	pong := serverFrame.GetPong()
	assert.Equal(t, clientTime, pong.ClientTimeMs)
	assert.GreaterOrEqual(t, pong.ServerTimeMs, int64(0))

	// Close stream
	err = stream.CloseSend()
	require.NoError(t, err)

	// Wait for cleanup to complete
	time.Sleep(100 * time.Millisecond)

	ts.mockSess.AssertExpectations(t)
}

// TestStreamHandler_FlowHintFrame tests backpressure handling
func TestStreamHandler_FlowHintFrame(t *testing.T) {
	ctx := context.Background()
	ts := setupTestServer(t)
	defer ts.shutdown()

	_ = setupMockSession(t, ts)

	ts.mockDel.On("HandleBackpressure", mock.Anything, mock.Anything, mock.MatchedBy(func(hint *umsv1.FlowHintFrame) bool {
		return hint.Hint == "overloaded"
	})).Return(nil)

	// Create client and stream
	client := ts.createTestClient(ctx, t)
	stream, err := client.Connect(ctx)
	require.NoError(t, err)

	// Send HELLO
	sendHelloAndExpectWelcome(t, stream)

	// Send FLOW_HINT frame
	err = stream.Send(&umsv1.ClientFrame{
		Frame: &umsv1.ClientFrame_FlowHint{
			FlowHint: &umsv1.FlowHintFrame{
				Hint: "overloaded",
			},
		},
	})
	require.NoError(t, err)

	// Give time for processing
	time.Sleep(50 * time.Millisecond)

	// Close stream
	err = stream.CloseSend()
	require.NoError(t, err)

	// Wait for cleanup to complete
	time.Sleep(100 * time.Millisecond)

	ts.mockDel.AssertExpectations(t)
}

// TestStreamHandler_ErrorScenario_NoHello tests protocol error when first frame is not HELLO
func TestStreamHandler_ErrorScenario_NoHello(t *testing.T) {
	ctx := context.Background()
	ts := setupTestServer(t)
	defer ts.shutdown()

	// Create client and stream
	client := ts.createTestClient(ctx, t)
	stream, err := client.Connect(ctx)
	require.NoError(t, err)

	// Send SUBSCRIBE frame as first frame (should fail)
	err = stream.Send(&umsv1.ClientFrame{
		Frame: &umsv1.ClientFrame_Subscribe{
			Subscribe: &umsv1.SubscribeFrame{
				Targets: []*umsv1.Target{},
			},
		},
	})
	require.NoError(t, err)

	// Receive ERROR frame
	serverFrame, err := stream.Recv()
	require.NoError(t, err)
	require.NotNil(t, serverFrame.GetError())

	errorFrame := serverFrame.GetError()
	assert.Equal(t, "PROTOCOL_ERROR", errorFrame.Code)
	assert.Contains(t, errorFrame.Message, "HELLO")
	assert.False(t, errorFrame.Retryable)
}

// TestStreamHandler_BidirectionalStreaming tests full bidirectional communication
func TestStreamHandler_BidirectionalStreaming(t *testing.T) {
	ctx := context.Background()
	ts := setupTestServer(t)
	defer ts.shutdown()

	session := setupMockSession(t, ts)

	// Setup expectations for multiple operations
	ts.mockAck.On("HandleCumulativeAck", mock.Anything, session.ServerID, "mailbox-1", uint64(10)).Return(uint64(0), nil)
	ts.mockAck.On("HandleCumulativeAck", mock.Anything, session.ServerID, "mailbox-1", uint64(20)).Return(uint64(10), nil)
	ts.mockSess.On("HandleHeartbeat", mock.Anything, session.SessionID).Return(nil).Times(2)

	// Create client and stream
	client := ts.createTestClient(ctx, t)
	stream, err := client.Connect(ctx)
	require.NoError(t, err)

	// Send HELLO
	sendHelloAndExpectWelcome(t, stream)

	// Send multiple frames and verify responses
	frames := []struct {
		send       *umsv1.ClientFrame
		expectPong bool
	}{
		{
			send: &umsv1.ClientFrame{
				Frame: &umsv1.ClientFrame_Ack{
					Ack: &umsv1.AckFrame{MailboxId: "mailbox-1", SeqAcked: 10},
				},
			},
			expectPong: false,
		},
		{
			send: &umsv1.ClientFrame{
				Frame: &umsv1.ClientFrame_Ping{
					Ping: &umsv1.PingFrame{ClientTimeMs: time.Now().UnixMilli()},
				},
			},
			expectPong: true,
		},
		{
			send: &umsv1.ClientFrame{
				Frame: &umsv1.ClientFrame_Ack{
					Ack: &umsv1.AckFrame{MailboxId: "mailbox-1", SeqAcked: 20},
				},
			},
			expectPong: false,
		},
		{
			send: &umsv1.ClientFrame{
				Frame: &umsv1.ClientFrame_Ping{
					Ping: &umsv1.PingFrame{ClientTimeMs: time.Now().UnixMilli()},
				},
			},
			expectPong: true,
		},
	}

	for _, frame := range frames {
		err = stream.Send(frame.send)
		require.NoError(t, err)

		if frame.expectPong {
			serverFrame, err := stream.Recv()
			require.NoError(t, err)
			require.NotNil(t, serverFrame.GetPong())
		}
	}

	// Give time for async ACK processing
	time.Sleep(100 * time.Millisecond)

	// Close stream
	err = stream.CloseSend()
	require.NoError(t, err)

	// Wait for cleanup to complete
	time.Sleep(100 * time.Millisecond)

	ts.mockAck.AssertExpectations(t)
	ts.mockSess.AssertExpectations(t)
}

// TestStreamHandler_ClientDisconnect tests client disconnect handling
func TestStreamHandler_ClientDisconnect(t *testing.T) {
	ctx := context.Background()
	ts := setupTestServer(t)
	defer ts.shutdown()

	session := setupMockSession(t, ts)

	// Create client and stream
	client := ts.createTestClient(ctx, t)
	stream, err := client.Connect(ctx)
	require.NoError(t, err)

	// Send HELLO
	sendHelloAndExpectWelcome(t, stream)

	// Close stream immediately
	err = stream.CloseSend()
	require.NoError(t, err)

	// Try to receive (should get EOF or error)
	_, err = stream.Recv()
	assert.True(t, err == io.EOF || err != nil)

	// Verify cleanup happened
	time.Sleep(100 * time.Millisecond)
	ts.mockDel.AssertCalled(t, "StopDeliveryLoop", mock.Anything, session.SessionID)
	ts.mockSess.AssertCalled(t, "UnregisterSession", mock.Anything, session.SessionID)
}

// Helper functions

func setupMockSession(t *testing.T, ts *testServer) *types.Session {
	welcomeFrame := &umsv1.WelcomeFrame{
		SessionId:    "test-session-123",
		SessionEpoch: 1,
		ResumeToken:  "resume-token",
	}

	session := &types.Session{
		SessionID:    "test-session-123",
		ServerID:     "server-001",
		OrgID:        "org-001",
		SessionEpoch: 1,
		InflightByQoS: map[types.QoS]int{
			types.QoSCommand:   0,
			types.QoSControl:   0,
			types.QoSTelemetry: 0,
		},
	}

	ts.mockSess.On("HandleHello", mock.Anything, mock.Anything).Return(welcomeFrame, session, nil)
	ts.mockSess.On("RegisterSession", mock.Anything, mock.Anything).Return(nil)
	ts.mockDel.On("StartDeliveryLoop", mock.Anything, mock.Anything).Return(nil)
	ts.mockDel.On("StopDeliveryLoop", mock.Anything, session.SessionID).Return(nil)
	ts.mockDel.On("NotifyDeliveryLoops").Return().Maybe()
	ts.mockSess.On("UnregisterSession", mock.Anything, session.SessionID).Return(nil)

	return session
}

func sendHelloAndExpectWelcome(t *testing.T, stream umsv1.SpineStream_ConnectClient) {
	err := stream.Send(&umsv1.ClientFrame{
		Frame: &umsv1.ClientFrame_Hello{
			Hello: &umsv1.HelloFrame{
				ProtocolVersion: "1.0",
				ServerId:        "server-001",
				OrgId:           "org-001",
			},
		},
	})
	require.NoError(t, err)

	// Receive WELCOME
	serverFrame, err := stream.Recv()
	require.NoError(t, err)
	require.NotNil(t, serverFrame.GetWelcome())
}
