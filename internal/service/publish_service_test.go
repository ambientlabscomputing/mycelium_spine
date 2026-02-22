package service

import (
	"context"
	"errors"
	"testing"

	"github.com/ambientlabscomputing/mycelium_spine/internal/types"
	umsv1 "github.com/ambientlabscomputing/mycelium_spine/proto/ums/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockDeliveryService implements DeliveryService for testing
type MockDeliveryService struct {
	NotifyDeliveryLoopsCalled bool
}

func (m *MockDeliveryService) StartDeliveryLoop(ctx context.Context, session *types.Session) error {
	return nil
}

func (m *MockDeliveryService) StopDeliveryLoop(ctx context.Context, sessionID string) error {
	return nil
}

func (m *MockDeliveryService) DeliverToSession(ctx context.Context, session *types.Session, envelopes []*types.Envelope) error {
	return nil
}

func (m *MockDeliveryService) HandleBackpressure(ctx context.Context, session *types.Session, hint *umsv1.FlowHintFrame) error {
	return nil
}

func (m *MockDeliveryService) NotifyDeliveryLoops() {
	m.NotifyDeliveryLoopsCalled = true
}

func TestPublish_SingleTarget_Success(t *testing.T) {
	mailboxID := "mailbox-123"
	targetSeq := uint64(42)

	mockRepo := &MockRepository{
		ResolveTargetsFunc: func(ctx context.Context, targets []*types.Target) ([]string, error) {
			assert.Len(t, targets, 1)
			return []string{mailboxID}, nil
		},
		AppendEnvelopeFunc: func(ctx context.Context, mid string, envelope *types.Envelope) (uint64, error) {
			assert.Equal(t, mailboxID, mid)
			assert.NotNil(t, envelope)
			return targetSeq, nil
		},
	}

	mockDelivery := &MockDeliveryService{}
	// Use shared testMetrics
	svc := NewPublishService(mockRepo, mockDelivery, testMetrics)

	envelope := types.NewEnvelope("", types.QoSCommand, "test.command", []byte("payload"), "org-123")
	targets := []*types.Target{
		types.NewTarget(types.TargetTypeServer, "server-1", "org-123"),
	}

	ctx := context.Background()
	result, err := svc.Publish(ctx, envelope, targets)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)
	assert.Equal(t, targetSeq, result.MailboxSeqs[mailboxID])
	assert.True(t, mockDelivery.NotifyDeliveryLoopsCalled)
}

func TestPublish_MultipleTargets_Success(t *testing.T) {
	mailbox1 := "mailbox-1"
	mailbox2 := "mailbox-2"
	mailbox3 := "mailbox-3"

	mockRepo := &MockRepository{
		ResolveTargetsFunc: func(ctx context.Context, targets []*types.Target) ([]string, error) {
			assert.Len(t, targets, 3)
			return []string{mailbox1, mailbox2, mailbox3}, nil
		},
		AppendEnvelopeFunc: func(ctx context.Context, mid string, envelope *types.Envelope) (uint64, error) {
			switch mid {
			case mailbox1:
				return uint64(10), nil
			case mailbox2:
				return uint64(20), nil
			case mailbox3:
				return uint64(30), nil
			}
			return 0, errors.New("unexpected mailbox")
		},
	}

	mockDelivery := &MockDeliveryService{}
	// Use shared testMetrics
	svc := NewPublishService(mockRepo, mockDelivery, testMetrics)

	envelope := types.NewEnvelope("", types.QoSCommand, "test.command", []byte("payload"), "org-123")
	targets := []*types.Target{
		types.NewTarget(types.TargetTypeServer, "server-1", "org-123"),
		types.NewTarget(types.TargetTypeServer, "server-2", "org-123"),
		types.NewTarget(types.TargetTypeCluster, "cluster-1", "org-123"),
	}

	ctx := context.Background()
	result, err := svc.Publish(ctx, envelope, targets)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)
	assert.Len(t, result.MailboxSeqs, 3)
	assert.Equal(t, uint64(10), result.MailboxSeqs[mailbox1])
	assert.Equal(t, uint64(20), result.MailboxSeqs[mailbox2])
	assert.Equal(t, uint64(30), result.MailboxSeqs[mailbox3])
	assert.True(t, mockDelivery.NotifyDeliveryLoopsCalled)
}

func TestPublish_ResolveTargetsError(t *testing.T) {
	mockRepo := &MockRepository{
		ResolveTargetsFunc: func(ctx context.Context, targets []*types.Target) ([]string, error) {
			return nil, errors.New("failed to resolve targets")
		},
	}

	mockDelivery := &MockDeliveryService{}
	// Use shared testMetrics
	svc := NewPublishService(mockRepo, mockDelivery, testMetrics)

	envelope := types.NewEnvelope("", types.QoSCommand, "test.command", []byte("payload"), "org-123")
	targets := []*types.Target{
		types.NewTarget(types.TargetTypeServer, "server-1", "org-123"),
	}

	ctx := context.Background()
	result, err := svc.Publish(ctx, envelope, targets)

	require.Error(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "failed to resolve targets")
}

func TestPublish_AppendEnvelopeError_ContinuesOthers(t *testing.T) {
	mailbox1 := "mailbox-1"
	mailbox2 := "mailbox-2"
	mailbox3 := "mailbox-3"

	mockRepo := &MockRepository{
		ResolveTargetsFunc: func(ctx context.Context, targets []*types.Target) ([]string, error) {
			return []string{mailbox1, mailbox2, mailbox3}, nil
		},
		AppendEnvelopeFunc: func(ctx context.Context, mid string, envelope *types.Envelope) (uint64, error) {
			switch mid {
			case mailbox1:
				return uint64(10), nil
			case mailbox2:
				return 0, errors.New("database error") // This one fails
			case mailbox3:
				return uint64(30), nil
			}
			return 0, errors.New("unexpected mailbox")
		},
	}

	mockDelivery := &MockDeliveryService{}
	// Use shared testMetrics
	svc := NewPublishService(mockRepo, mockDelivery, testMetrics)

	envelope := types.NewEnvelope("", types.QoSCommand, "test.command", []byte("payload"), "org-123")
	targets := []*types.Target{
		types.NewTarget(types.TargetTypeServer, "server-1", "org-123"),
		types.NewTarget(types.TargetTypeServer, "server-2", "org-123"),
		types.NewTarget(types.TargetTypeCluster, "cluster-1", "org-123"),
	}

	ctx := context.Background()
	result, err := svc.Publish(ctx, envelope, targets)

	// Should succeed even if one mailbox fails
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)

	// Only successful mailboxes should be in result
	assert.Len(t, result.MailboxSeqs, 2)
	assert.Equal(t, uint64(10), result.MailboxSeqs[mailbox1])
	assert.Equal(t, uint64(30), result.MailboxSeqs[mailbox3])
	_, hasMailbox2 := result.MailboxSeqs[mailbox2]
	assert.False(t, hasMailbox2)
}

func TestPublish_EmptyTargets(t *testing.T) {
	mockRepo := &MockRepository{
		ResolveTargetsFunc: func(ctx context.Context, targets []*types.Target) ([]string, error) {
			assert.Len(t, targets, 0)
			return []string{}, nil
		},
	}

	mockDelivery := &MockDeliveryService{}
	// Use shared testMetrics
	svc := NewPublishService(mockRepo, mockDelivery, testMetrics)

	envelope := types.NewEnvelope("", types.QoSCommand, "test.command", []byte("payload"), "org-123")
	targets := []*types.Target{}

	ctx := context.Background()
	result, err := svc.Publish(ctx, envelope, targets)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)
	assert.Empty(t, result.MailboxSeqs)
	assert.True(t, mockDelivery.NotifyDeliveryLoopsCalled)
}

func TestPublish_NilEnvelope(t *testing.T) {
	mockRepo := &MockRepository{
		ResolveTargetsFunc: func(ctx context.Context, targets []*types.Target) ([]string, error) {
			return []string{"mailbox-1"}, nil
		},
		AppendEnvelopeFunc: func(ctx context.Context, mid string, envelope *types.Envelope) (uint64, error) {
			// This will panic if envelope is nil, which is expected
			return uint64(1), nil
		},
	}

	mockDelivery := &MockDeliveryService{}
	// Use shared testMetrics
	svc := NewPublishService(mockRepo, mockDelivery, testMetrics)

	targets := []*types.Target{
		types.NewTarget(types.TargetTypeServer, "server-1", "org-123"),
	}

	ctx := context.Background()
	// This should panic or cause an error
	defer func() {
		if r := recover(); r != nil {
			// Expected panic when dereferencing nil envelope
			t.Log("Caught expected panic:", r)
		}
	}()

	_, _ = svc.Publish(ctx, nil, targets)
}

func TestPublish_DifferentQoSLevels(t *testing.T) {
	tests := []struct {
		name     string
		qos      types.QoS
		wantType string
	}{
		{
			name:     "command QoS",
			qos:      types.QoSCommand,
			wantType: "test.command",
		},
		{
			name:     "control QoS",
			qos:      types.QoSControl,
			wantType: "test.control",
		},
		{
			name:     "telemetry QoS",
			qos:      types.QoSTelemetry,
			wantType: "test.telemetry",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := &MockRepository{
				ResolveTargetsFunc: func(ctx context.Context, targets []*types.Target) ([]string, error) {
					return []string{"mailbox-1"}, nil
				},
				AppendEnvelopeFunc: func(ctx context.Context, mid string, envelope *types.Envelope) (uint64, error) {
					assert.Equal(t, tt.qos, envelope.QoS)
					assert.Equal(t, tt.wantType, envelope.Type)
					return uint64(1), nil
				},
			}

			mockDelivery := &MockDeliveryService{}
			// Use shared testMetrics
			svc := NewPublishService(mockRepo, mockDelivery, testMetrics)

			envelope := types.NewEnvelope("", tt.qos, tt.wantType, []byte("payload"), "org-123")
			targets := []*types.Target{
				types.NewTarget(types.TargetTypeServer, "server-1", "org-123"),
			}

			ctx := context.Background()
			result, err := svc.Publish(ctx, envelope, targets)

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.True(t, result.Success)
		})
	}
}

func TestPublish_DifferentTargetTypes(t *testing.T) {
	tests := []struct {
		name       string
		targetType types.TargetType
		targetID   string
	}{
		{
			name:       "server target",
			targetType: types.TargetTypeServer,
			targetID:   "server-1",
		},
		{
			name:       "cluster target",
			targetType: types.TargetTypeCluster,
			targetID:   "cluster-1",
		},
		{
			name:       "org target",
			targetType: types.TargetTypeOrg,
			targetID:   "org-1",
		},
		{
			name:       "service target",
			targetType: types.TargetTypeService,
			targetID:   "service-1",
		},
		{
			name:       "broadcast target",
			targetType: types.TargetTypeBroadcast,
			targetID:   "broadcast-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := &MockRepository{
				ResolveTargetsFunc: func(ctx context.Context, targets []*types.Target) ([]string, error) {
					assert.Len(t, targets, 1)
					assert.Equal(t, tt.targetType, targets[0].TargetType)
					assert.Equal(t, tt.targetID, targets[0].TargetID)
					return []string{"mailbox-1"}, nil
				},
				AppendEnvelopeFunc: func(ctx context.Context, mid string, envelope *types.Envelope) (uint64, error) {
					return uint64(1), nil
				},
			}

			mockDelivery := &MockDeliveryService{}
			// Use shared testMetrics
			svc := NewPublishService(mockRepo, mockDelivery, testMetrics)

			envelope := types.NewEnvelope("", types.QoSCommand, "test.command", []byte("payload"), "org-123")
			targets := []*types.Target{
				types.NewTarget(tt.targetType, tt.targetID, "org-123"),
			}

			ctx := context.Background()
			result, err := svc.Publish(ctx, envelope, targets)

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.True(t, result.Success)
		})
	}
}
