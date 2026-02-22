package grpc

import (
	"context"
	"log/slog"
	"net"
	"os"
	"testing"

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

func init() {
	// Initialize logger for tests
	if utils.Logger == nil {
		utils.Logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelError, // Use ERROR level to reduce test output noise
		}))
	}
}

// testPublishServer holds the test gRPC server for publish handler
type testPublishServer struct {
	lis         *bufconn.Listener
	server      *grpc.Server
	mockPubSvc  *MockPublishService
	mockService *MockAppService
}

// setupPublishTestServer creates a test gRPC server with bufconn for publish handler
func setupPublishTestServer(t *testing.T) *testPublishServer {
	lis := bufconn.Listen(bufSize)

	// Create mock services
	mockPubSvc := &MockPublishService{}

	mockService := &MockAppService{
		publishSvc: mockPubSvc,
	}

	// Create handler
	handler := NewPublishHandler(mockService)

	// Create gRPC server
	server := grpc.NewServer()
	umsv1.RegisterSpinePublishServer(server, handler)

	// Start server in background
	go func() {
		if err := server.Serve(lis); err != nil {
			t.Logf("Server exited with error: %v", err)
		}
	}()

	return &testPublishServer{
		lis:         lis,
		server:      server,
		mockPubSvc:  mockPubSvc,
		mockService: mockService,
	}
}

// createPublishTestClient creates a gRPC client connected via bufconn
func (ts *testPublishServer) createPublishTestClient(ctx context.Context, t *testing.T) umsv1.SpinePublishClient {
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

	return umsv1.NewSpinePublishClient(conn)
}

// shutdown stops the test server
func (ts *testPublishServer) shutdown() {
	ts.server.GracefulStop()
	ts.lis.Close()
}

// TestPublishHandler_Publish_Success tests successful envelope publishing
func TestPublishHandler_Publish_Success(t *testing.T) {
	ctx := context.Background()
	ts := setupPublishTestServer(t)
	defer ts.shutdown()

	// Setup mock expectations
	publishResult := &service.PublishResult{
		Success: true,
		MailboxSeqs: map[string]uint64{
			"mailbox-server-001": 123,
			"mailbox-server-002": 456,
		},
	}

	ts.mockPubSvc.On("Publish", mock.Anything, mock.MatchedBy(func(env *types.Envelope) bool {
		return env.EnvelopeID == "envelope-123" && env.Type == "command.run"
	}), mock.MatchedBy(func(targets []*types.Target) bool {
		return len(targets) == 2 && targets[0].TargetType == types.TargetTypeServer
	})).Return(publishResult, nil)

	// Create client
	client := ts.createPublishTestClient(ctx, t)

	// Send publish request
	req := &umsv1.PublishRequest{
		Envelope: &umsv1.Envelope{
			EnvelopeId:  "envelope-123",
			MailboxId:   "",
			Seq:         0,
			Qos:         umsv1.QoS_QOS_COMMAND,
			Type:        "command.run",
			CreatedAtMs: 1234567890,
			ExpiresAtMs: 1234567900,
			TraceId:     "trace-abc",
			Payload:     []byte(`{"action":"restart"}`),
			DedupeKey:   "dedupe-123",
			Priority:    10,
			RequiresAck: true,
			OrgId:       "org-001",
		},
		Targets: []*umsv1.Target{
			{
				TargetType: umsv1.TargetType_TARGET_TYPE_SERVER,
				TargetId:   "server-001",
				OrgId:      "org-001",
			},
			{
				TargetType: umsv1.TargetType_TARGET_TYPE_SERVER,
				TargetId:   "server-002",
				OrgId:      "org-001",
			},
		},
	}

	resp, err := client.Publish(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.True(t, resp.Success)
	assert.Empty(t, resp.Error)
	assert.Len(t, resp.MailboxSeqs, 2)
	assert.Equal(t, uint64(123), resp.MailboxSeqs["mailbox-server-001"])
	assert.Equal(t, uint64(456), resp.MailboxSeqs["mailbox-server-002"])

	ts.mockPubSvc.AssertExpectations(t)
}

// TestPublishHandler_Publish_SingleTarget tests publishing to a single target
func TestPublishHandler_Publish_SingleTarget(t *testing.T) {
	ctx := context.Background()
	ts := setupPublishTestServer(t)
	defer ts.shutdown()

	// Setup mock expectations
	publishResult := &service.PublishResult{
		Success: true,
		MailboxSeqs: map[string]uint64{
			"mailbox-cluster-prod": 789,
		},
	}

	ts.mockPubSvc.On("Publish", mock.Anything, mock.Anything, mock.MatchedBy(func(targets []*types.Target) bool {
		return len(targets) == 1 && targets[0].TargetType == types.TargetTypeCluster
	})).Return(publishResult, nil)

	// Create client
	client := ts.createPublishTestClient(ctx, t)

	// Send publish request
	req := &umsv1.PublishRequest{
		Envelope: &umsv1.Envelope{
			EnvelopeId:  "envelope-456",
			Qos:         umsv1.QoS_QOS_CONTROL,
			Type:        "config.update",
			CreatedAtMs: 1234567890,
			Payload:     []byte(`{"config":"value"}`),
			RequiresAck: false,
			OrgId:       "org-001",
		},
		Targets: []*umsv1.Target{
			{
				TargetType: umsv1.TargetType_TARGET_TYPE_CLUSTER,
				TargetId:   "prod",
				OrgId:      "org-001",
			},
		},
	}

	resp, err := client.Publish(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.True(t, resp.Success)
	assert.Len(t, resp.MailboxSeqs, 1)
	assert.Equal(t, uint64(789), resp.MailboxSeqs["mailbox-cluster-prod"])

	ts.mockPubSvc.AssertExpectations(t)
}

// TestPublishHandler_Publish_MultipleTargets tests publishing to multiple target types
func TestPublishHandler_Publish_MultipleTargets(t *testing.T) {
	ctx := context.Background()
	ts := setupPublishTestServer(t)
	defer ts.shutdown()

	// Setup mock expectations
	publishResult := &service.PublishResult{
		Success: true,
		MailboxSeqs: map[string]uint64{
			"mailbox-server-001":   100,
			"mailbox-cluster-prod": 200,
			"mailbox-org-acme":     300,
		},
	}

	ts.mockPubSvc.On("Publish", mock.Anything, mock.Anything, mock.MatchedBy(func(targets []*types.Target) bool {
		if len(targets) != 3 {
			return false
		}
		// Verify we have all three target types
		hasServer := false
		hasCluster := false
		hasOrg := false
		for _, t := range targets {
			switch t.TargetType {
			case types.TargetTypeServer:
				hasServer = true
			case types.TargetTypeCluster:
				hasCluster = true
			case types.TargetTypeOrg:
				hasOrg = true
			}
		}
		return hasServer && hasCluster && hasOrg
	})).Return(publishResult, nil)

	// Create client
	client := ts.createPublishTestClient(ctx, t)

	// Send publish request with multiple target types
	req := &umsv1.PublishRequest{
		Envelope: &umsv1.Envelope{
			EnvelopeId:  "envelope-789",
			Qos:         umsv1.QoS_QOS_COMMAND,
			Type:        "deploy.execute",
			CreatedAtMs: 1234567890,
			Payload:     []byte(`{"version":"v2.0"}`),
			RequiresAck: true,
			OrgId:       "org-001",
		},
		Targets: []*umsv1.Target{
			{
				TargetType: umsv1.TargetType_TARGET_TYPE_SERVER,
				TargetId:   "server-001",
				OrgId:      "org-001",
			},
			{
				TargetType: umsv1.TargetType_TARGET_TYPE_CLUSTER,
				TargetId:   "prod",
				OrgId:      "org-001",
			},
			{
				TargetType: umsv1.TargetType_TARGET_TYPE_ORG,
				TargetId:   "acme",
				OrgId:      "org-001",
			},
		},
	}

	resp, err := client.Publish(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.True(t, resp.Success)
	assert.Len(t, resp.MailboxSeqs, 3)
	assert.Equal(t, uint64(100), resp.MailboxSeqs["mailbox-server-001"])
	assert.Equal(t, uint64(200), resp.MailboxSeqs["mailbox-cluster-prod"])
	assert.Equal(t, uint64(300), resp.MailboxSeqs["mailbox-org-acme"])

	ts.mockPubSvc.AssertExpectations(t)
}

// TestPublishHandler_Publish_TelemetryQoS tests publishing telemetry messages
func TestPublishHandler_Publish_TelemetryQoS(t *testing.T) {
	ctx := context.Background()
	ts := setupPublishTestServer(t)
	defer ts.shutdown()

	// Setup mock expectations
	publishResult := &service.PublishResult{
		Success: true,
		MailboxSeqs: map[string]uint64{
			"mailbox-server-monitor": 555,
		},
	}

	ts.mockPubSvc.On("Publish", mock.Anything, mock.MatchedBy(func(env *types.Envelope) bool {
		return env.QoS == types.QoSTelemetry && env.Type == "metrics.cpu"
	}), mock.Anything).Return(publishResult, nil)

	// Create client
	client := ts.createPublishTestClient(ctx, t)

	// Send telemetry publish request
	req := &umsv1.PublishRequest{
		Envelope: &umsv1.Envelope{
			EnvelopeId:  "envelope-telemetry",
			Qos:         umsv1.QoS_QOS_TELEMETRY,
			Type:        "metrics.cpu",
			CreatedAtMs: 1234567890,
			Payload:     []byte(`{"cpu":45.2,"memory":78.5}`),
			RequiresAck: false,
			OrgId:       "org-001",
		},
		Targets: []*umsv1.Target{
			{
				TargetType: umsv1.TargetType_TARGET_TYPE_SERVER,
				TargetId:   "monitor",
				OrgId:      "org-001",
			},
		},
	}

	resp, err := client.Publish(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.True(t, resp.Success)
	assert.Len(t, resp.MailboxSeqs, 1)

	ts.mockPubSvc.AssertExpectations(t)
}

// TestPublishHandler_Publish_Error tests error handling during publish
func TestPublishHandler_Publish_Error(t *testing.T) {
	ctx := context.Background()
	ts := setupPublishTestServer(t)
	defer ts.shutdown()

	// Setup mock expectations - service returns error
	ts.mockPubSvc.On("Publish", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, assert.AnError)

	// Create client
	client := ts.createPublishTestClient(ctx, t)

	// Send publish request
	req := &umsv1.PublishRequest{
		Envelope: &umsv1.Envelope{
			EnvelopeId:  "envelope-error",
			Qos:         umsv1.QoS_QOS_COMMAND,
			Type:        "command.fail",
			CreatedAtMs: 1234567890,
			Payload:     []byte(`{}`),
			RequiresAck: true,
			OrgId:       "org-001",
		},
		Targets: []*umsv1.Target{
			{
				TargetType: umsv1.TargetType_TARGET_TYPE_SERVER,
				TargetId:   "server-001",
				OrgId:      "org-001",
			},
		},
	}

	resp, err := client.Publish(ctx, req)

	// The handler returns success=false in the response, not a gRPC error
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.False(t, resp.Success)
	assert.NotEmpty(t, resp.Error)
	assert.Empty(t, resp.MailboxSeqs)

	ts.mockPubSvc.AssertExpectations(t)
}

// TestPublishHandler_Publish_EmptyTargets tests publishing with no targets (edge case)
func TestPublishHandler_Publish_EmptyTargets(t *testing.T) {
	ctx := context.Background()
	ts := setupPublishTestServer(t)
	defer ts.shutdown()

	// Setup mock expectations - empty targets
	publishResult := &service.PublishResult{
		Success:     true,
		MailboxSeqs: map[string]uint64{},
	}

	ts.mockPubSvc.On("Publish", mock.Anything, mock.Anything, mock.MatchedBy(func(targets []*types.Target) bool {
		return len(targets) == 0
	})).Return(publishResult, nil)

	// Create client
	client := ts.createPublishTestClient(ctx, t)

	// Send publish request with no targets
	req := &umsv1.PublishRequest{
		Envelope: &umsv1.Envelope{
			EnvelopeId:  "envelope-notargets",
			Qos:         umsv1.QoS_QOS_CONTROL,
			Type:        "test.empty",
			CreatedAtMs: 1234567890,
			Payload:     []byte(`{}`),
			RequiresAck: false,
			OrgId:       "org-001",
		},
		Targets: []*umsv1.Target{},
	}

	resp, err := client.Publish(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.True(t, resp.Success)
	assert.Empty(t, resp.MailboxSeqs)

	ts.mockPubSvc.AssertExpectations(t)
}

// TestPublishHandler_Publish_WithDeliverRole tests publishing with deliver role
func TestPublishHandler_Publish_WithDeliverRole(t *testing.T) {
	ctx := context.Background()
	ts := setupPublishTestServer(t)
	defer ts.shutdown()

	// Setup mock expectations
	publishResult := &service.PublishResult{
		Success: true,
		MailboxSeqs: map[string]uint64{
			"mailbox-cluster-leader": 999,
		},
	}

	ts.mockPubSvc.On("Publish", mock.Anything, mock.Anything, mock.MatchedBy(func(targets []*types.Target) bool {
		return len(targets) == 1 && targets[0].DeliverRole == "LEADER"
	})).Return(publishResult, nil)

	// Create client
	client := ts.createPublishTestClient(ctx, t)

	// Send publish request with deliver role
	req := &umsv1.PublishRequest{
		Envelope: &umsv1.Envelope{
			EnvelopeId:  "envelope-leader",
			Qos:         umsv1.QoS_QOS_COMMAND,
			Type:        "cluster.command",
			CreatedAtMs: 1234567890,
			Payload:     []byte(`{"action":"elect"}`),
			RequiresAck: true,
			OrgId:       "org-001",
		},
		Targets: []*umsv1.Target{
			{
				TargetType:  umsv1.TargetType_TARGET_TYPE_CLUSTER,
				TargetId:    "prod-cluster",
				OrgId:       "org-001",
				DeliverRole: "LEADER",
			},
		},
	}

	resp, err := client.Publish(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.True(t, resp.Success)
	assert.Len(t, resp.MailboxSeqs, 1)

	ts.mockPubSvc.AssertExpectations(t)
}

// TestPublishHandler_Publish_AllQoSTypes tests publishing with different QoS levels
func TestPublishHandler_Publish_AllQoSTypes(t *testing.T) {
	tests := []struct {
		name      string
		qos       umsv1.QoS
		expectQoS types.QoS
	}{
		{
			name:      "Command_QoS",
			qos:       umsv1.QoS_QOS_COMMAND,
			expectQoS: types.QoSCommand,
		},
		{
			name:      "Control_QoS",
			qos:       umsv1.QoS_QOS_CONTROL,
			expectQoS: types.QoSControl,
		},
		{
			name:      "Telemetry_QoS",
			qos:       umsv1.QoS_QOS_TELEMETRY,
			expectQoS: types.QoSTelemetry,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			ts := setupPublishTestServer(t)
			defer ts.shutdown()

			// Setup mock expectations
			publishResult := &service.PublishResult{
				Success: true,
				MailboxSeqs: map[string]uint64{
					"mailbox-test": 1,
				},
			}

			ts.mockPubSvc.On("Publish", mock.Anything, mock.MatchedBy(func(env *types.Envelope) bool {
				return env.QoS == tt.expectQoS
			}), mock.Anything).Return(publishResult, nil)

			// Create client
			client := ts.createPublishTestClient(ctx, t)

			// Send publish request
			req := &umsv1.PublishRequest{
				Envelope: &umsv1.Envelope{
					EnvelopeId:  "envelope-qos-test",
					Qos:         tt.qos,
					Type:        "test.qos",
					CreatedAtMs: 1234567890,
					Payload:     []byte(`{}`),
					RequiresAck: tt.qos != umsv1.QoS_QOS_TELEMETRY,
					OrgId:       "org-001",
				},
				Targets: []*umsv1.Target{
					{
						TargetType: umsv1.TargetType_TARGET_TYPE_SERVER,
						TargetId:   "test",
						OrgId:      "org-001",
					},
				},
			}

			resp, err := client.Publish(ctx, req)
			require.NoError(t, err)
			require.NotNil(t, resp)

			assert.True(t, resp.Success)

			ts.mockPubSvc.AssertExpectations(t)
		})
	}
}

// TestPublishHandler_Publish_LargePayload tests publishing with large payload
func TestPublishHandler_Publish_LargePayload(t *testing.T) {
	ctx := context.Background()
	ts := setupPublishTestServer(t)
	defer ts.shutdown()

	// Create a large payload (1MB)
	largePayload := make([]byte, 1024*1024)
	for i := range largePayload {
		largePayload[i] = byte(i % 256)
	}

	// Setup mock expectations
	publishResult := &service.PublishResult{
		Success: true,
		MailboxSeqs: map[string]uint64{
			"mailbox-server-001": 1,
		},
	}

	ts.mockPubSvc.On("Publish", mock.Anything, mock.MatchedBy(func(env *types.Envelope) bool {
		return len(env.Payload) == 1024*1024
	}), mock.Anything).Return(publishResult, nil)

	// Create client
	client := ts.createPublishTestClient(ctx, t)

	// Send publish request
	req := &umsv1.PublishRequest{
		Envelope: &umsv1.Envelope{
			EnvelopeId:  "envelope-large",
			Qos:         umsv1.QoS_QOS_COMMAND,
			Type:        "data.transfer",
			CreatedAtMs: 1234567890,
			Payload:     largePayload,
			RequiresAck: true,
			OrgId:       "org-001",
		},
		Targets: []*umsv1.Target{
			{
				TargetType: umsv1.TargetType_TARGET_TYPE_SERVER,
				TargetId:   "server-001",
				OrgId:      "org-001",
			},
		},
	}

	resp, err := client.Publish(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.True(t, resp.Success)

	ts.mockPubSvc.AssertExpectations(t)
}

// TestPublishHandler_Publish_EnvelopeFields tests all envelope fields are properly converted
func TestPublishHandler_Publish_EnvelopeFields(t *testing.T) {
	ctx := context.Background()
	ts := setupPublishTestServer(t)
	defer ts.shutdown()

	// Setup mock expectations with detailed field matching
	publishResult := &service.PublishResult{
		Success: true,
		MailboxSeqs: map[string]uint64{
			"mailbox-server-test": 42,
		},
	}

	ts.mockPubSvc.On("Publish", mock.Anything, mock.MatchedBy(func(env *types.Envelope) bool {
		return env.EnvelopeID == "env-123" &&
			env.Type == "test.type" &&
			env.CreatedAtMs == 1000000 &&
			env.ExpiresAtMs == 2000000 &&
			env.TraceID == "trace-xyz" &&
			env.DedupeKey == "dedupe-abc" &&
			env.Priority == 99 &&
			env.RequiresAck == true &&
			env.OrgID == "org-test"
	}), mock.Anything).Return(publishResult, nil)

	// Create client
	client := ts.createPublishTestClient(ctx, t)

	// Send publish request with all fields
	req := &umsv1.PublishRequest{
		Envelope: &umsv1.Envelope{
			EnvelopeId:  "env-123",
			Qos:         umsv1.QoS_QOS_COMMAND,
			Type:        "test.type",
			CreatedAtMs: 1000000,
			ExpiresAtMs: 2000000,
			TraceId:     "trace-xyz",
			Payload:     []byte(`{"test":"data"}`),
			DedupeKey:   "dedupe-abc",
			Priority:    99,
			RequiresAck: true,
			OrgId:       "org-test",
		},
		Targets: []*umsv1.Target{
			{
				TargetType: umsv1.TargetType_TARGET_TYPE_SERVER,
				TargetId:   "test",
				OrgId:      "org-test",
			},
		},
	}

	resp, err := client.Publish(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.True(t, resp.Success)

	ts.mockPubSvc.AssertExpectations(t)
}
