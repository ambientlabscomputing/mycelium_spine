package service

import (
	"context"
	"errors"
	"testing"

	"github.com/ambientlabscomputing/mycelium_spine/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleSubscribe_SingleTarget_Success(t *testing.T) {
	mailboxID := "mailbox-123"
	session := types.NewSession("test-server-1", "org-123", 1, "fingerprint", nil, nil)

	mockRepo := &MockRepository{
		ResolveTargetsFunc: func(ctx context.Context, targets []*types.Target) ([]string, error) {
			assert.Len(t, targets, 1)
			return []string{mailboxID}, nil
		},
		UpdateSubscriptionsFunc: func(ctx context.Context, sessionID string, mailboxIDs []string) error {
			assert.Equal(t, session.SessionID, sessionID)
			assert.Contains(t, mailboxIDs, mailboxID)
			return nil
		},
	}

	mockDelivery := &MockDeliveryService{}
	// Use shared testMetrics
	svc := NewSubscribeService(mockRepo, mockDelivery, testMetrics)

	targets := []*types.Target{
		types.NewTarget(types.TargetTypeServer, "server-1", "org-123"),
	}

	ctx := context.Background()
	subscribeOk, err := svc.HandleSubscribe(ctx, session, targets)

	require.NoError(t, err)
	require.NotNil(t, subscribeOk)
	assert.Len(t, subscribeOk.MailboxIds, 1)
	assert.Equal(t, mailboxID, subscribeOk.MailboxIds[0])

	// Verify session has the subscription
	assert.True(t, session.HasSubscription(mailboxID))
}

func TestHandleSubscribe_MultipleTargets_Success(t *testing.T) {
	mailbox1 := "mailbox-1"
	mailbox2 := "mailbox-2"
	mailbox3 := "mailbox-3"
	session := types.NewSession("test-server-1", "org-123", 1, "fingerprint", nil, nil)

	mockRepo := &MockRepository{
		ResolveTargetsFunc: func(ctx context.Context, targets []*types.Target) ([]string, error) {
			assert.Len(t, targets, 3)
			return []string{mailbox1, mailbox2, mailbox3}, nil
		},
		UpdateSubscriptionsFunc: func(ctx context.Context, sessionID string, mailboxIDs []string) error {
			assert.Equal(t, session.SessionID, sessionID)
			assert.Len(t, mailboxIDs, 3)
			return nil
		},
	}

	mockDelivery := &MockDeliveryService{}
	// Use shared testMetrics
	svc := NewSubscribeService(mockRepo, mockDelivery, testMetrics)

	targets := []*types.Target{
		types.NewTarget(types.TargetTypeServer, "server-1", "org-123"),
		types.NewTarget(types.TargetTypeCluster, "cluster-1", "org-123"),
		types.NewTarget(types.TargetTypeService, "service-1", "org-123"),
	}

	ctx := context.Background()
	subscribeOk, err := svc.HandleSubscribe(ctx, session, targets)

	require.NoError(t, err)
	require.NotNil(t, subscribeOk)
	assert.Len(t, subscribeOk.MailboxIds, 3)

	// Verify all subscriptions are added to session
	assert.True(t, session.HasSubscription(mailbox1))
	assert.True(t, session.HasSubscription(mailbox2))
	assert.True(t, session.HasSubscription(mailbox3))
}

func TestHandleSubscribe_ResolveTargetsError(t *testing.T) {
	session := types.NewSession("test-server-1", "org-123", 1, "fingerprint", nil, nil)

	mockRepo := &MockRepository{
		ResolveTargetsFunc: func(ctx context.Context, targets []*types.Target) ([]string, error) {
			return nil, errors.New("target resolution failed")
		},
	}

	mockDelivery := &MockDeliveryService{}
	// Use shared testMetrics
	svc := NewSubscribeService(mockRepo, mockDelivery, testMetrics)

	targets := []*types.Target{
		types.NewTarget(types.TargetTypeServer, "server-1", "org-123"),
	}

	ctx := context.Background()
	_, err := svc.HandleSubscribe(ctx, session, targets)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to resolve subscription targets")
}

func TestHandleSubscribe_UpdateSubscriptionsError(t *testing.T) {
	mailboxID := "mailbox-123"
	session := types.NewSession("test-server-1", "org-123", 1, "fingerprint", nil, nil)

	mockRepo := &MockRepository{
		ResolveTargetsFunc: func(ctx context.Context, targets []*types.Target) ([]string, error) {
			return []string{mailboxID}, nil
		},
		UpdateSubscriptionsFunc: func(ctx context.Context, sessionID string, mailboxIDs []string) error {
			return errors.New("database error")
		},
	}

	mockDelivery := &MockDeliveryService{}
	// Use shared testMetrics
	svc := NewSubscribeService(mockRepo, mockDelivery, testMetrics)

	targets := []*types.Target{
		types.NewTarget(types.TargetTypeServer, "server-1", "org-123"),
	}

	ctx := context.Background()
	_, err := svc.HandleSubscribe(ctx, session, targets)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to persist subscriptions")
}

func TestHandleSubscribe_DuplicateSubscription(t *testing.T) {
	mailboxID := "mailbox-123"
	session := types.NewSession("test-server-1", "org-123", 1, "fingerprint", nil, nil)

	// Pre-add the subscription
	session.AddSubscription(mailboxID)

	mockRepo := &MockRepository{
		ResolveTargetsFunc: func(ctx context.Context, targets []*types.Target) ([]string, error) {
			return []string{mailboxID}, nil
		},
		UpdateSubscriptionsFunc: func(ctx context.Context, sessionID string, mailboxIDs []string) error {
			// Should still be called with the mailbox ID
			assert.Contains(t, mailboxIDs, mailboxID)
			return nil
		},
	}

	mockDelivery := &MockDeliveryService{}
	// Use shared testMetrics
	svc := NewSubscribeService(mockRepo, mockDelivery, testMetrics)

	targets := []*types.Target{
		types.NewTarget(types.TargetTypeServer, "server-1", "org-123"),
	}

	ctx := context.Background()
	subscribeOk, err := svc.HandleSubscribe(ctx, session, targets)

	require.NoError(t, err)
	require.NotNil(t, subscribeOk)

	// Should not add duplicate
	count := 0
	for _, id := range session.Subscriptions {
		if id == mailboxID {
			count++
		}
	}
	assert.Equal(t, 1, count, "mailbox should only appear once in subscriptions")
}

func TestHandleSubscribe_EmptyTargets(t *testing.T) {
	session := types.NewSession("test-server-1", "org-123", 1, "fingerprint", nil, nil)

	mockRepo := &MockRepository{
		ResolveTargetsFunc: func(ctx context.Context, targets []*types.Target) ([]string, error) {
			assert.Len(t, targets, 0)
			return []string{}, nil
		},
		UpdateSubscriptionsFunc: func(ctx context.Context, sessionID string, mailboxIDs []string) error {
			return nil
		},
	}

	mockDelivery := &MockDeliveryService{}
	// Use shared testMetrics
	svc := NewSubscribeService(mockRepo, mockDelivery, testMetrics)

	targets := []*types.Target{}

	ctx := context.Background()
	subscribeOk, err := svc.HandleSubscribe(ctx, session, targets)

	require.NoError(t, err)
	require.NotNil(t, subscribeOk)
	assert.Empty(t, subscribeOk.MailboxIds)
}

func TestHandleSubscribe_NilSession(t *testing.T) {
	mockRepo := &MockRepository{
		ResolveTargetsFunc: func(ctx context.Context, targets []*types.Target) ([]string, error) {
			return []string{"mailbox-1"}, nil
		},
	}

	mockDelivery := &MockDeliveryService{}
	// Use shared testMetrics
	svc := NewSubscribeService(mockRepo, mockDelivery, testMetrics)

	targets := []*types.Target{
		types.NewTarget(types.TargetTypeServer, "server-1", "org-123"),
	}

	ctx := context.Background()

	// This should panic or cause nil pointer dereference
	defer func() {
		if r := recover(); r != nil {
			t.Log("Caught expected panic:", r)
		}
	}()

	_, _ = svc.HandleSubscribe(ctx, nil, targets)
}

func TestHandleSubscribe_DifferentTargetTypes(t *testing.T) {
	tests := []struct {
		name       string
		targetType types.TargetType
		targetID   string
		mailboxID  string
	}{
		{
			name:       "server target",
			targetType: types.TargetTypeServer,
			targetID:   "server-1",
			mailboxID:  "mailbox-server-1",
		},
		{
			name:       "cluster target",
			targetType: types.TargetTypeCluster,
			targetID:   "cluster-1",
			mailboxID:  "mailbox-cluster-1",
		},
		{
			name:       "org target",
			targetType: types.TargetTypeOrg,
			targetID:   "org-1",
			mailboxID:  "mailbox-org-1",
		},
		{
			name:       "service target",
			targetType: types.TargetTypeService,
			targetID:   "service-1",
			mailboxID:  "mailbox-service-1",
		},
		{
			name:       "broadcast target",
			targetType: types.TargetTypeBroadcast,
			targetID:   "broadcast-1",
			mailboxID:  "mailbox-broadcast-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session := types.NewSession("test-server-1", "org-123", 1, "fingerprint", nil, nil)

			mockRepo := &MockRepository{
				ResolveTargetsFunc: func(ctx context.Context, targets []*types.Target) ([]string, error) {
					assert.Len(t, targets, 1)
					assert.Equal(t, tt.targetType, targets[0].TargetType)
					assert.Equal(t, tt.targetID, targets[0].TargetID)
					return []string{tt.mailboxID}, nil
				},
				UpdateSubscriptionsFunc: func(ctx context.Context, sessionID string, mailboxIDs []string) error {
					assert.Contains(t, mailboxIDs, tt.mailboxID)
					return nil
				},
			}

			mockDelivery := &MockDeliveryService{}
			// Use shared testMetrics
			svc := NewSubscribeService(mockRepo, mockDelivery, testMetrics)

			targets := []*types.Target{
				types.NewTarget(tt.targetType, tt.targetID, "org-123"),
			}

			ctx := context.Background()
			subscribeOk, err := svc.HandleSubscribe(ctx, session, targets)

			require.NoError(t, err)
			require.NotNil(t, subscribeOk)
			assert.Len(t, subscribeOk.MailboxIds, 1)
			assert.Equal(t, tt.mailboxID, subscribeOk.MailboxIds[0])
			assert.True(t, session.HasSubscription(tt.mailboxID))
		})
	}
}

func TestHandleSubscribe_MultipleSubscriptionsOverTime(t *testing.T) {
	session := types.NewSession("test-server-1", "org-123", 1, "fingerprint", nil, nil)

	mockRepo := &MockRepository{
		ResolveTargetsFunc: func(ctx context.Context, targets []*types.Target) ([]string, error) {
			mailboxIDs := make([]string, len(targets))
			for i, target := range targets {
				mailboxIDs[i] = "mailbox-" + target.TargetID
			}
			return mailboxIDs, nil
		},
		UpdateSubscriptionsFunc: func(ctx context.Context, sessionID string, mailboxIDs []string) error {
			return nil
		},
	}

	mockDelivery := &MockDeliveryService{}
	// Use shared testMetrics
	svc := NewSubscribeService(mockRepo, mockDelivery, testMetrics)

	ctx := context.Background()

	// First subscription
	targets1 := []*types.Target{
		types.NewTarget(types.TargetTypeServer, "server-1", "org-123"),
	}
	_, err := svc.HandleSubscribe(ctx, session, targets1)
	require.NoError(t, err)
	assert.Len(t, session.Subscriptions, 1)

	// Second subscription
	targets2 := []*types.Target{
		types.NewTarget(types.TargetTypeCluster, "cluster-1", "org-123"),
	}
	_, err = svc.HandleSubscribe(ctx, session, targets2)
	require.NoError(t, err)
	assert.Len(t, session.Subscriptions, 2)

	// Third subscription
	targets3 := []*types.Target{
		types.NewTarget(types.TargetTypeService, "service-1", "org-123"),
	}
	_, err = svc.HandleSubscribe(ctx, session, targets3)
	require.NoError(t, err)
	assert.Len(t, session.Subscriptions, 3)

	// Verify all subscriptions are present
	assert.True(t, session.HasSubscription("mailbox-server-1"))
	assert.True(t, session.HasSubscription("mailbox-cluster-1"))
	assert.True(t, session.HasSubscription("mailbox-service-1"))
}

func TestHandleSubscribe_NilTargets(t *testing.T) {
	session := types.NewSession("test-server-1", "org-123", 1, "fingerprint", nil, nil)

	callCount := 0
	mockRepo := &MockRepository{
		ResolveTargetsFunc: func(ctx context.Context, targets []*types.Target) ([]string, error) {
			callCount++
			// nil slice should be treated as empty
			if targets == nil {
				return []string{}, nil
			}
			return []string{}, nil
		},
		UpdateSubscriptionsFunc: func(ctx context.Context, sessionID string, mailboxIDs []string) error {
			return nil
		},
	}

	mockDelivery := &MockDeliveryService{}
	// Use shared testMetrics
	svc := NewSubscribeService(mockRepo, mockDelivery, testMetrics)

	ctx := context.Background()
	subscribeOk, err := svc.HandleSubscribe(ctx, session, nil)

	require.NoError(t, err)
	require.NotNil(t, subscribeOk)
	assert.Empty(t, subscribeOk.MailboxIds)
	assert.Equal(t, 1, callCount)
}
