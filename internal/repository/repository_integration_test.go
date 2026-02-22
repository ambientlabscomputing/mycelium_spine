package repository

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/ambientlabscomputing/mycelium_spine/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// TestRepositoryIntegration runs comprehensive integration tests across all repository components
func TestRepositoryIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}

	ctx := context.Background()

	// Start MongoDB container
	mongoC, mongoURI, err := startMongoContainerForIntegration(ctx)
	require.NoError(t, err, "Failed to start MongoDB container")
	defer func() {
		if err := mongoC.Terminate(ctx); err != nil {
			t.Logf("Failed to terminate container: %v", err)
		}
	}()

	// Connect to MongoDB
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	require.NoError(t, err, "Failed to connect to MongoDB")
	defer client.Disconnect(ctx)

	// Create database and repository
	db := client.Database("ums_integration_test")
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	repo := NewMongoRepository(db, logger)

	// Create indexes
	err = repo.CreateIndexes(ctx)
	require.NoError(t, err, "Failed to create indexes")

	// Run integration test suite
	t.Run("SessionWithSubscriptionsAndAckPositions", func(t *testing.T) {
		testSessionWithSubscriptionsAndAckPositions(t, ctx, repo)
	})

	t.Run("MultipleMailboxesWithEnvelopes", func(t *testing.T) {
		testMultipleMailboxesWithEnvelopes(t, ctx, repo)
	})

	t.Run("SessionResumeScenario", func(t *testing.T) {
		testSessionResumeScenario(t, ctx, repo)
	})

	t.Run("LargeDatasetPagination", func(t *testing.T) {
		testLargeDatasetPagination(t, ctx, repo)
	})

	t.Run("ConcurrentSessionsAndEnvelopes", func(t *testing.T) {
		testConcurrentSessionsAndEnvelopes(t, ctx, repo)
	})

	t.Run("TargetResolution", func(t *testing.T) {
		testTargetResolution(t, ctx, repo)
	})

	t.Run("TargetResolutionCluster", func(t *testing.T) {
		testTargetResolutionCluster(t, ctx, repo)
	})

	t.Run("TargetResolutionBroadcast", func(t *testing.T) {
		testTargetResolutionBroadcast(t, ctx, repo)
	})

	t.Run("EnvelopeDeliveryFlow", func(t *testing.T) {
		testEnvelopeDeliveryFlow(t, ctx, repo)
	})

	t.Run("RetentionAndCleanup", func(t *testing.T) {
		testRetentionAndCleanup(t, ctx, repo)
	})

	t.Run("MultiTenancy", func(t *testing.T) {
		testMultiTenancy(t, ctx, repo)
	})
}

// testSessionWithSubscriptionsAndAckPositions tests the complete session lifecycle
// with subscriptions and ack position tracking
func testSessionWithSubscriptionsAndAckPositions(t *testing.T, ctx context.Context, repo *MongoRepository) {
	// Create session
	sessionID := "session-full-1"
	session := &types.Session{
		SessionID:     sessionID,
		ServerID:      "server-full-1",
		OrgID:         "org-1",
		SessionEpoch:  1,
		ResumeToken:   "token-full-1",
		Subscriptions: []string{},
		ConnectedAt:   time.Now().UnixMilli(),
		LastHeartbeat: time.Now().UnixMilli(),
	}
	err := repo.CreateSession(ctx, session)
	require.NoError(t, err)

	// Create multiple mailboxes
	mailboxIDs := make([]string, 3)
	for i := 0; i < 3; i++ {
		mailbox, err := repo.GetOrCreateMailbox(ctx, types.TargetTypeServer, fmt.Sprintf("server-%d", i), "org-1")
		require.NoError(t, err)
		mailboxIDs[i] = mailbox.MailboxID

		// Append some envelopes to each mailbox
		for j := 0; j < 5; j++ {
			envelope := types.NewEnvelope(mailbox.MailboxID, types.QoSCommand, "test.command", []byte(fmt.Sprintf("payload-%d-%d", i, j)), "org-1")
			_, err := repo.AppendEnvelope(ctx, mailbox.MailboxID, envelope)
			require.NoError(t, err)
		}
	}

	// Subscribe to all mailboxes
	err = repo.UpdateSubscriptions(ctx, sessionID, mailboxIDs)
	assert.NoError(t, err)

	// Verify subscriptions
	retrieved, err := repo.GetSession(ctx, sessionID)
	assert.NoError(t, err)
	assert.Equal(t, mailboxIDs, retrieved.Subscriptions)

	// Update ack positions for each mailbox
	for i, mailboxID := range mailboxIDs {
		err = repo.UpdateAckPosition(ctx, sessionID, mailboxID, uint64(i+3))
		assert.NoError(t, err)
	}

	// Retrieve and verify ack positions
	positions, err := repo.GetAckPositions(ctx, sessionID)
	assert.NoError(t, err)
	assert.Len(t, positions, 3)
	for i, mailboxID := range mailboxIDs {
		assert.Equal(t, uint64(i+3), positions[mailboxID])
	}

	// Fetch unacked envelopes from each mailbox
	for i, mailboxID := range mailboxIDs {
		fromSeq := positions[mailboxID] + 1
		envelopes, err := repo.FetchEnvelopes(ctx, mailboxID, fromSeq, 10)
		assert.NoError(t, err)
		expectedUnacked := 5 - int(positions[mailboxID])
		if expectedUnacked < 0 {
			expectedUnacked = 0
		}
		assert.Len(t, envelopes, expectedUnacked)
		t.Logf("Mailbox %d: %d unacked envelopes starting from seq %d", i, len(envelopes), fromSeq)
	}

	// Clean up
	err = repo.DeleteSession(ctx, sessionID)
	assert.NoError(t, err)
}

// testMultipleMailboxesWithEnvelopes tests operations across multiple mailboxes
func testMultipleMailboxesWithEnvelopes(t *testing.T, ctx context.Context, repo *MongoRepository) {
	numMailboxes := 10
	envelopesPerMailbox := 20

	mailboxIDs := make([]string, numMailboxes)

	// Create mailboxes and populate with envelopes
	for i := 0; i < numMailboxes; i++ {
		mailbox, err := repo.GetOrCreateMailbox(ctx, types.TargetTypeServer, fmt.Sprintf("server-multi-%d", i), "org-1")
		require.NoError(t, err)
		mailboxIDs[i] = mailbox.MailboxID

		for j := 0; j < envelopesPerMailbox; j++ {
			envelope := types.NewEnvelope(mailbox.MailboxID, types.QoSCommand, "test.command", []byte(fmt.Sprintf("data-%d-%d", i, j)), "org-1")
			_, err := repo.AppendEnvelope(ctx, mailbox.MailboxID, envelope)
			require.NoError(t, err)
		}
	}

	// Verify each mailbox
	for i, mailboxID := range mailboxIDs {
		mailbox, err := repo.GetMailbox(ctx, mailboxID)
		assert.NoError(t, err)
		assert.Equal(t, uint64(envelopesPerMailbox+1), mailbox.NextSeq)

		// Fetch all envelopes
		envelopes, err := repo.FetchEnvelopes(ctx, mailboxID, 1, 100)
		assert.NoError(t, err)
		assert.Len(t, envelopes, envelopesPerMailbox)

		// Verify sequence order
		for j, env := range envelopes {
			assert.Equal(t, uint64(j+1), env.Seq)
		}

		t.Logf("Mailbox %d verified: %d envelopes", i, len(envelopes))
	}
}

// testSessionResumeScenario tests session resumption with token rotation
func testSessionResumeScenario(t *testing.T, ctx context.Context, repo *MongoRepository) {
	serverID := "server-resume-1"
	orgID := "org-1"

	// Create initial session (epoch 1)
	session1 := &types.Session{
		SessionID:     "session-resume-1",
		ServerID:      serverID,
		OrgID:         orgID,
		SessionEpoch:  1,
		ResumeToken:   "token-1",
		Subscriptions: []string{},
		ConnectedAt:   time.Now().UnixMilli(),
		LastHeartbeat: time.Now().UnixMilli(),
	}
	err := repo.CreateSession(ctx, session1)
	require.NoError(t, err)

	// Subscribe to a mailbox
	mailbox, err := repo.GetOrCreateMailbox(ctx, types.TargetTypeServer, "target-resume-1", orgID)
	require.NoError(t, err)
	err = repo.UpdateSubscriptions(ctx, session1.SessionID, []string{mailbox.MailboxID})
	require.NoError(t, err)

	// Update ack position
	err = repo.UpdateAckPosition(ctx, session1.SessionID, mailbox.MailboxID, 5)
	require.NoError(t, err)

	// Rotate token (simulating token rotation during session)
	err = repo.UpdateResumeToken(ctx, session1.SessionID, "token-2")
	assert.NoError(t, err)

	// Retrieve session by new token
	retrieved, err := repo.GetSessionByResumeToken(ctx, "token-2")
	assert.NoError(t, err)
	assert.Equal(t, session1.SessionID, retrieved.SessionID)

	// Simulate disconnection and new connection (epoch 2)
	session2 := &types.Session{
		SessionID:     "session-resume-2",
		ServerID:      serverID,
		OrgID:         orgID,
		SessionEpoch:  2,
		ResumeToken:   "token-3",
		Subscriptions: []string{mailbox.MailboxID},
		ConnectedAt:   time.Now().UnixMilli(),
		LastHeartbeat: time.Now().UnixMilli(),
	}
	err = repo.CreateSession(ctx, session2)
	require.NoError(t, err)

	// Retrieve by server_id (should get highest epoch)
	active, err := repo.GetSessionByServerID(ctx, serverID)
	assert.NoError(t, err)
	assert.Equal(t, uint64(2), active.SessionEpoch)
	assert.Equal(t, session2.SessionID, active.SessionID)

	// Old session's ack position should still be accessible
	positions, err := repo.GetAckPositions(ctx, session1.SessionID)
	assert.NoError(t, err)
	assert.Equal(t, uint64(5), positions[mailbox.MailboxID])

	// Clean up
	err = repo.DeleteSession(ctx, session1.SessionID)
	assert.NoError(t, err)
	err = repo.DeleteSession(ctx, session2.SessionID)
	assert.NoError(t, err)
}

// testLargeDatasetPagination tests pagination with large datasets
func testLargeDatasetPagination(t *testing.T, ctx context.Context, repo *MongoRepository) {
	// Create mailbox
	mailbox, err := repo.GetOrCreateMailbox(ctx, types.TargetTypeServer, "server-large-1", "org-1")
	require.NoError(t, err)

	// Append 1000 envelopes
	numEnvelopes := 1000
	for i := 0; i < numEnvelopes; i++ {
		envelope := types.NewEnvelope(mailbox.MailboxID, types.QoSCommand, "test.command", []byte(fmt.Sprintf("payload-%d", i)), "org-1")
		_, err := repo.AppendEnvelope(ctx, mailbox.MailboxID, envelope)
		require.NoError(t, err)
	}

	t.Logf("Created %d envelopes", numEnvelopes)

	// Paginate through all envelopes
	pageSize := 50
	fromSeq := uint64(1)
	totalFetched := 0
	pageCount := 0

	for {
		envelopes, err := repo.FetchEnvelopes(ctx, mailbox.MailboxID, fromSeq, pageSize)
		assert.NoError(t, err)

		if len(envelopes) == 0 {
			break
		}

		pageCount++
		totalFetched += len(envelopes)

		// Verify sequence continuity within page
		for i, env := range envelopes {
			expectedSeq := fromSeq + uint64(i)
			assert.Equal(t, expectedSeq, env.Seq, "Page %d, index %d", pageCount, i)
		}

		// Move to next page
		lastSeq := envelopes[len(envelopes)-1].Seq
		fromSeq = lastSeq + 1

		if len(envelopes) < pageSize {
			break
		}
	}

	assert.Equal(t, numEnvelopes, totalFetched)
	t.Logf("Fetched %d envelopes in %d pages", totalFetched, pageCount)
}

// testConcurrentSessionsAndEnvelopes tests concurrent operations
func testConcurrentSessionsAndEnvelopes(t *testing.T, ctx context.Context, repo *MongoRepository) {
	numSessions := 5
	numMailboxes := 3
	envelopesPerSession := 20

	var wg sync.WaitGroup

	// Concurrently create sessions and append envelopes
	for i := 0; i < numSessions; i++ {
		wg.Add(1)
		go func(sessionNum int) {
			defer wg.Done()

			sessionID := fmt.Sprintf("session-concurrent-%d", sessionNum)
			session := &types.Session{
				SessionID:     sessionID,
				ServerID:      fmt.Sprintf("server-concurrent-%d", sessionNum),
				OrgID:         "org-1",
				SessionEpoch:  1,
				ResumeToken:   fmt.Sprintf("token-concurrent-%d", sessionNum),
				Subscriptions: []string{},
				ConnectedAt:   time.Now().UnixMilli(),
				LastHeartbeat: time.Now().UnixMilli(),
			}
			err := repo.CreateSession(ctx, session)
			if err != nil {
				t.Errorf("Failed to create session: %v", err)
				return
			}

			// Each session appends to multiple mailboxes
			for j := 0; j < numMailboxes; j++ {
				mailbox, err := repo.GetOrCreateMailbox(ctx, types.TargetTypeServer, fmt.Sprintf("target-concurrent-%d", j), "org-1")
				if err != nil {
					t.Errorf("Failed to get/create mailbox: %v", err)
					return
				}

				for k := 0; k < envelopesPerSession; k++ {
					envelope := types.NewEnvelope(
						mailbox.MailboxID,
						types.QoSCommand,
						"test.command",
						[]byte(fmt.Sprintf("session-%d-mailbox-%d-env-%d", sessionNum, j, k)),
						"org-1",
					)
					_, err := repo.AppendEnvelope(ctx, mailbox.MailboxID, envelope)
					if err != nil {
						t.Errorf("Failed to append envelope: %v", err)
						return
					}
				}
			}
		}(i)
	}

	wg.Wait()

	// Verify all mailboxes have correct number of envelopes
	for j := 0; j < numMailboxes; j++ {
		mailbox, err := repo.GetOrCreateMailbox(ctx, types.TargetTypeServer, fmt.Sprintf("target-concurrent-%d", j), "org-1")
		require.NoError(t, err)

		envelopes, err := repo.FetchEnvelopes(ctx, mailbox.MailboxID, 1, 1000)
		assert.NoError(t, err)

		expectedCount := numSessions * envelopesPerSession
		assert.Len(t, envelopes, expectedCount)

		t.Logf("Mailbox %d: %d envelopes (expected %d)", j, len(envelopes), expectedCount)
	}
}

// testTargetResolution tests target resolution for different target types
func testTargetResolution(t *testing.T, ctx context.Context, repo *MongoRepository) {
	orgID := "org-target-1"

	// Test SERVER target
	serverTarget := types.NewTarget(types.TargetTypeServer, "server-1", orgID)
	mailboxIDs, err := repo.ResolveTargets(ctx, []*types.Target{serverTarget})
	assert.NoError(t, err)
	assert.Len(t, mailboxIDs, 1)

	// Test ORG target
	orgTarget := types.NewTarget(types.TargetTypeOrg, "org-target-1", orgID)
	mailboxIDs, err = repo.ResolveTargets(ctx, []*types.Target{orgTarget})
	assert.NoError(t, err)
	assert.Len(t, mailboxIDs, 1)

	// Test SERVICE target
	serviceTarget := types.NewTarget(types.TargetTypeService, "service-1", orgID)
	mailboxIDs, err = repo.ResolveTargets(ctx, []*types.Target{serviceTarget})
	assert.NoError(t, err)
	assert.Len(t, mailboxIDs, 1)

	// Test multiple targets (deduplication)
	targets := []*types.Target{
		types.NewTarget(types.TargetTypeServer, "server-1", orgID),
		types.NewTarget(types.TargetTypeServer, "server-2", orgID),
		types.NewTarget(types.TargetTypeServer, "server-1", orgID), // Duplicate
	}
	mailboxIDs, err = repo.ResolveTargets(ctx, targets)
	assert.NoError(t, err)
	assert.Len(t, mailboxIDs, 2) // Deduplicated
}

// testTargetResolutionCluster tests cluster target resolution
func testTargetResolutionCluster(t *testing.T, ctx context.Context, repo *MongoRepository) {
	orgID := "org-cluster-1"

	// Test CLUSTER target
	clusterTarget := types.NewTarget(types.TargetTypeCluster, "cluster-1", orgID)
	mailboxIDs, err := repo.ResolveTargets(ctx, []*types.Target{clusterTarget})
	assert.NoError(t, err)
	assert.Len(t, mailboxIDs, 1) // Only cluster mailbox for now (Phase 1)

	t.Log("Cluster target resolution returns cluster mailbox")
	t.Log("TODO: Phase 2 will expand to include all server mailboxes in cluster")
}

// testTargetResolutionBroadcast tests broadcast target resolution
func testTargetResolutionBroadcast(t *testing.T, ctx context.Context, repo *MongoRepository) {
	orgID := "org-broadcast-1"

	// Create several mailboxes in the org
	for i := 0; i < 5; i++ {
		_, err := repo.GetOrCreateMailbox(ctx, types.TargetTypeServer, fmt.Sprintf("server-broadcast-%d", i), orgID)
		require.NoError(t, err)
	}

	// Test BROADCAST target
	broadcastTarget := types.NewTarget(types.TargetTypeBroadcast, "", orgID)
	mailboxIDs, err := repo.ResolveTargets(ctx, []*types.Target{broadcastTarget})
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, len(mailboxIDs), 5) // At least the 5 we created

	t.Logf("Broadcast target resolved to %d mailboxes in org %s", len(mailboxIDs), orgID)
}

// testEnvelopeDeliveryFlow tests a complete envelope delivery flow
func testEnvelopeDeliveryFlow(t *testing.T, ctx context.Context, repo *MongoRepository) {
	orgID := "org-delivery-1"
	serverID := "server-delivery-1"

	// Create session
	sessionID := "session-delivery-1"
	session := &types.Session{
		SessionID:     sessionID,
		ServerID:      serverID,
		OrgID:         orgID,
		SessionEpoch:  1,
		ResumeToken:   "token-delivery-1",
		Subscriptions: []string{},
		ConnectedAt:   time.Now().UnixMilli(),
		LastHeartbeat: time.Now().UnixMilli(),
	}
	err := repo.CreateSession(ctx, session)
	require.NoError(t, err)

	// Create mailbox for this server
	mailbox, err := repo.GetOrCreateMailbox(ctx, types.TargetTypeServer, serverID, orgID)
	require.NoError(t, err)

	// Subscribe to mailbox
	err = repo.UpdateSubscriptions(ctx, sessionID, []string{mailbox.MailboxID})
	require.NoError(t, err)

	// Sender appends envelopes
	numEnvelopes := 10
	for i := 0; i < numEnvelopes; i++ {
		envelope := types.NewEnvelope(mailbox.MailboxID, types.QoSCommand, "command.execute", []byte(fmt.Sprintf("command-%d", i)), orgID)
		envelope.RequiresAck = true
		_, err := repo.AppendEnvelope(ctx, mailbox.MailboxID, envelope)
		require.NoError(t, err)
	}

	// Receiver fetches envelopes (initial fetch from seq 1)
	envelopes, err := repo.FetchEnvelopes(ctx, mailbox.MailboxID, 1, 5)
	assert.NoError(t, err)
	assert.Len(t, envelopes, 5)

	// Receiver acknowledges first batch
	err = repo.UpdateAckPosition(ctx, sessionID, mailbox.MailboxID, 5)
	assert.NoError(t, err)

	// Fetch next batch (from seq 6)
	envelopes, err = repo.FetchEnvelopes(ctx, mailbox.MailboxID, 6, 5)
	assert.NoError(t, err)
	assert.Len(t, envelopes, 5)

	// Acknowledge second batch
	err = repo.UpdateAckPosition(ctx, sessionID, mailbox.MailboxID, 10)
	assert.NoError(t, err)

	// Verify all envelopes are acknowledged
	positions, err := repo.GetAckPositions(ctx, sessionID)
	assert.NoError(t, err)
	assert.Equal(t, uint64(10), positions[mailbox.MailboxID])

	// Fetch from current position (should be empty)
	envelopes, err = repo.FetchEnvelopes(ctx, mailbox.MailboxID, 11, 5)
	assert.NoError(t, err)
	assert.Empty(t, envelopes)

	t.Log("Complete delivery flow verified")
}

// testRetentionAndCleanup tests retention policy enforcement
func testRetentionAndCleanup(t *testing.T, ctx context.Context, repo *MongoRepository) {
	// Create mailbox
	mailbox, err := repo.GetOrCreateMailbox(ctx, types.TargetTypeServer, "server-cleanup-1", "org-1")
	require.NoError(t, err)

	// Append 100 envelopes with varying ages
	oldTime := time.Now().Add(-2 * time.Hour).UnixMilli()
	recentTime := time.Now().UnixMilli()

	for i := 0; i < 100; i++ {
		envelope := types.NewEnvelope(mailbox.MailboxID, types.QoSCommand, "test.command", []byte(fmt.Sprintf("payload-%d", i)), "org-1")
		if i < 30 {
			envelope.CreatedAtMs = oldTime
		} else {
			envelope.CreatedAtMs = recentTime
		}
		_, err := repo.AppendEnvelope(ctx, mailbox.MailboxID, envelope)
		require.NoError(t, err)
	}

	// Enforce retention (1 hour)
	policy := types.RetentionPolicy{
		RetentionSeconds: 3600,
		MaxBytes:         1048576,
		MaxEnvelopes:     1000,
	}
	err = repo.EnforceRetention(ctx, mailbox.MailboxID, policy)
	assert.NoError(t, err)

	// Verify old envelopes were deleted
	envelopes, err := repo.FetchEnvelopes(ctx, mailbox.MailboxID, 1, 200)
	assert.NoError(t, err)
	assert.LessOrEqual(t, len(envelopes), 70) // Should have deleted ~30 old envelopes

	t.Logf("After retention: %d envelopes remain", len(envelopes))
}

// testMultiTenancy tests multi-tenancy isolation
func testMultiTenancy(t *testing.T, ctx context.Context, repo *MongoRepository) {
	// Create mailboxes for different orgs with same target
	targetID := "server-shared"

	mailbox1, err := repo.GetOrCreateMailbox(ctx, types.TargetTypeServer, targetID, "org-tenant-1")
	require.NoError(t, err)

	mailbox2, err := repo.GetOrCreateMailbox(ctx, types.TargetTypeServer, targetID, "org-tenant-2")
	require.NoError(t, err)

	// Mailboxes should be different
	assert.NotEqual(t, mailbox1.MailboxID, mailbox2.MailboxID)
	assert.Equal(t, "org-tenant-1", mailbox1.OrgID)
	assert.Equal(t, "org-tenant-2", mailbox2.OrgID)

	// Append envelopes to each
	for i := 0; i < 5; i++ {
		env1 := types.NewEnvelope(mailbox1.MailboxID, types.QoSCommand, "test.command", []byte(fmt.Sprintf("org1-%d", i)), "org-tenant-1")
		_, err := repo.AppendEnvelope(ctx, mailbox1.MailboxID, env1)
		require.NoError(t, err)

		env2 := types.NewEnvelope(mailbox2.MailboxID, types.QoSCommand, "test.command", []byte(fmt.Sprintf("org2-%d", i)), "org-tenant-2")
		_, err = repo.AppendEnvelope(ctx, mailbox2.MailboxID, env2)
		require.NoError(t, err)
	}

	// Verify isolation
	envelopes1, err := repo.FetchEnvelopes(ctx, mailbox1.MailboxID, 1, 100)
	assert.NoError(t, err)
	assert.Len(t, envelopes1, 5)
	for _, env := range envelopes1 {
		assert.Equal(t, "org-tenant-1", env.OrgID)
	}

	envelopes2, err := repo.FetchEnvelopes(ctx, mailbox2.MailboxID, 1, 100)
	assert.NoError(t, err)
	assert.Len(t, envelopes2, 5)
	for _, env := range envelopes2 {
		assert.Equal(t, "org-tenant-2", env.OrgID)
	}

	t.Log("Multi-tenancy isolation verified")
}

// startMongoContainerForIntegration starts a MongoDB testcontainer
func startMongoContainerForIntegration(ctx context.Context) (testcontainers.Container, string, error) {
	req := testcontainers.ContainerRequest{
		Image:        "mongo:7.0",
		ExposedPorts: []string{"27017/tcp"},
		WaitingFor:   wait.ForLog("Waiting for connections"),
	}

	mongoC, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, "", err
	}

	host, err := mongoC.Host(ctx)
	if err != nil {
		return nil, "", err
	}

	port, err := mongoC.MappedPort(ctx, "27017")
	if err != nil {
		return nil, "", err
	}

	mongoURI := fmt.Sprintf("mongodb://%s:%s", host, port.Port())
	return mongoC, mongoURI, nil
}
