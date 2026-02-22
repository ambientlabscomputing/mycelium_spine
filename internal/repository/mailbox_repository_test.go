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

// TestMailboxRepository runs all mailbox repository tests with a real MongoDB instance
func TestMailboxRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}

	ctx := context.Background()

	// Start MongoDB container
	mongoC, mongoURI, err := startMongoContainerForMailbox(ctx)
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
	db := client.Database("ums_mailbox_test")
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	repo := NewMongoRepository(db, logger)

	// Create indexes
	err = repo.CreateIndexes(ctx)
	require.NoError(t, err, "Failed to create indexes")

	// Run test suite
	t.Run("GetOrCreateMailbox", func(t *testing.T) {
		testGetOrCreateMailbox(t, ctx, repo)
	})

	t.Run("GetMailbox", func(t *testing.T) {
		testGetMailbox(t, ctx, repo)
	})

	t.Run("AppendEnvelope", func(t *testing.T) {
		testAppendEnvelope(t, ctx, repo)
	})

	t.Run("AppendEnvelopeSequenceAllocation", func(t *testing.T) {
		testAppendEnvelopeSequenceAllocation(t, ctx, repo)
	})

	t.Run("FetchEnvelopes", func(t *testing.T) {
		testFetchEnvelopes(t, ctx, repo)
	})

	t.Run("FetchEnvelopesPagination", func(t *testing.T) {
		testFetchEnvelopesPagination(t, ctx, repo)
	})

	t.Run("DeleteExpired", func(t *testing.T) {
		testDeleteExpired(t, ctx, repo)
	})

	t.Run("EnforceRetention", func(t *testing.T) {
		testEnforceRetention(t, ctx, repo)
	})

	t.Run("ConcurrentAppend", func(t *testing.T) {
		testConcurrentAppend(t, ctx, repo)
	})

	t.Run("MailboxIndexes", func(t *testing.T) {
		testMailboxIndexes(t, ctx, repo)
	})

	t.Run("NonAtomicSequenceIssue", func(t *testing.T) {
		testNonAtomicSequenceIssue(t, ctx, repo)
	})
}

func testGetOrCreateMailbox(t *testing.T, ctx context.Context, repo *MongoRepository) {
	retentionPolicy := types.RetentionPolicy{
		RetentionSeconds: 3600,
		MaxBytes:         1048576,
		MaxEnvelopes:     1000,
	}

	// First call should create mailbox
	mailbox1, err := repo.GetOrCreateMailbox(ctx, types.TargetTypeServer, "server-1", "org-1")
	assert.NoError(t, err)
	assert.NotNil(t, mailbox1)
	assert.Equal(t, types.TargetTypeServer, mailbox1.TargetType)
	assert.Equal(t, "server-1", mailbox1.TargetID)
	assert.Equal(t, "org-1", mailbox1.OrgID)
	assert.Equal(t, uint64(1), mailbox1.NextSeq)

	// Second call should return existing mailbox
	mailbox2, err := repo.GetOrCreateMailbox(ctx, types.TargetTypeServer, "server-1", "org-1")
	assert.NoError(t, err)
	assert.Equal(t, mailbox1.MailboxID, mailbox2.MailboxID)
	assert.Equal(t, mailbox1.NextSeq, mailbox2.NextSeq)

	// Different target type should create new mailbox
	mailbox3, err := repo.GetOrCreateMailbox(ctx, types.TargetTypeCluster, "server-1", "org-1")
	assert.NoError(t, err)
	assert.NotEqual(t, mailbox1.MailboxID, mailbox3.MailboxID)
	assert.Equal(t, types.TargetTypeCluster, mailbox3.TargetType)

	// Different org should create new mailbox
	mailbox4, err := repo.GetOrCreateMailbox(ctx, types.TargetTypeServer, "server-1", "org-2")
	assert.NoError(t, err)
	assert.NotEqual(t, mailbox1.MailboxID, mailbox4.MailboxID)
	assert.Equal(t, "org-2", mailbox4.OrgID)

	// Use retention policy explicitly
	_ = retentionPolicy
}

func testGetMailbox(t *testing.T, ctx context.Context, repo *MongoRepository) {
	// Create mailbox
	mailbox, err := repo.GetOrCreateMailbox(ctx, types.TargetTypeServer, "server-get-1", "org-1")
	require.NoError(t, err)

	// Retrieve by ID
	retrieved, err := repo.GetMailbox(ctx, mailbox.MailboxID)
	assert.NoError(t, err)
	assert.Equal(t, mailbox.MailboxID, retrieved.MailboxID)
	assert.Equal(t, mailbox.TargetType, retrieved.TargetType)
	assert.Equal(t, mailbox.TargetID, retrieved.TargetID)

	// Test not found
	_, err = repo.GetMailbox(ctx, "non-existent-mailbox")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "mailbox not found")
}

func testAppendEnvelope(t *testing.T, ctx context.Context, repo *MongoRepository) {
	// Create mailbox
	mailbox, err := repo.GetOrCreateMailbox(ctx, types.TargetTypeServer, "server-append-1", "org-1")
	require.NoError(t, err)

	// Append first envelope
	envelope1 := types.NewEnvelope(mailbox.MailboxID, types.QoSCommand, "test.command", []byte("payload1"), "org-1")
	seq1, err := repo.AppendEnvelope(ctx, mailbox.MailboxID, envelope1)
	assert.NoError(t, err)
	assert.Equal(t, uint64(1), seq1)
	assert.Equal(t, uint64(1), envelope1.Seq)

	// Append second envelope
	envelope2 := types.NewEnvelope(mailbox.MailboxID, types.QoSControl, "test.control", []byte("payload2"), "org-1")
	seq2, err := repo.AppendEnvelope(ctx, mailbox.MailboxID, envelope2)
	assert.NoError(t, err)
	assert.Equal(t, uint64(2), seq2)
	assert.Equal(t, uint64(2), envelope2.Seq)

	// Verify mailbox next_seq was incremented
	updatedMailbox, err := repo.GetMailbox(ctx, mailbox.MailboxID)
	assert.NoError(t, err)
	assert.Equal(t, uint64(3), updatedMailbox.NextSeq)
}

func testAppendEnvelopeSequenceAllocation(t *testing.T, ctx context.Context, repo *MongoRepository) {
	// Create mailbox
	mailbox, err := repo.GetOrCreateMailbox(ctx, types.TargetTypeServer, "server-seq-1", "org-1")
	require.NoError(t, err)

	// Append multiple envelopes and verify sequential allocation
	sequences := make([]uint64, 0)
	for i := 0; i < 10; i++ {
		envelope := types.NewEnvelope(mailbox.MailboxID, types.QoSCommand, "test.command", []byte(fmt.Sprintf("payload-%d", i)), "org-1")
		seq, err := repo.AppendEnvelope(ctx, mailbox.MailboxID, envelope)
		assert.NoError(t, err)
		sequences = append(sequences, seq)
	}

	// Verify sequences are sequential
	for i, seq := range sequences {
		assert.Equal(t, uint64(i+1), seq)
	}

	// Verify final next_seq
	updatedMailbox, err := repo.GetMailbox(ctx, mailbox.MailboxID)
	assert.NoError(t, err)
	assert.Equal(t, uint64(11), updatedMailbox.NextSeq)
}

func testFetchEnvelopes(t *testing.T, ctx context.Context, repo *MongoRepository) {
	// Create mailbox
	mailbox, err := repo.GetOrCreateMailbox(ctx, types.TargetTypeServer, "server-fetch-1", "org-1")
	require.NoError(t, err)

	// Append several envelopes
	for i := 0; i < 5; i++ {
		envelope := types.NewEnvelope(mailbox.MailboxID, types.QoSCommand, "test.command", []byte(fmt.Sprintf("payload-%d", i)), "org-1")
		_, err := repo.AppendEnvelope(ctx, mailbox.MailboxID, envelope)
		require.NoError(t, err)
	}

	// Fetch from beginning
	envelopes, err := repo.FetchEnvelopes(ctx, mailbox.MailboxID, 1, 10)
	assert.NoError(t, err)
	assert.Len(t, envelopes, 5)
	assert.Equal(t, uint64(1), envelopes[0].Seq)
	assert.Equal(t, uint64(5), envelopes[4].Seq)

	// Fetch from middle
	envelopes, err = repo.FetchEnvelopes(ctx, mailbox.MailboxID, 3, 10)
	assert.NoError(t, err)
	assert.Len(t, envelopes, 3)
	assert.Equal(t, uint64(3), envelopes[0].Seq)
	assert.Equal(t, uint64(5), envelopes[2].Seq)

	// Fetch beyond available
	envelopes, err = repo.FetchEnvelopes(ctx, mailbox.MailboxID, 10, 10)
	assert.NoError(t, err)
	assert.Empty(t, envelopes)
}

func testFetchEnvelopesPagination(t *testing.T, ctx context.Context, repo *MongoRepository) {
	// Create mailbox
	mailbox, err := repo.GetOrCreateMailbox(ctx, types.TargetTypeServer, "server-page-1", "org-1")
	require.NoError(t, err)

	// Append 50 envelopes
	for i := 0; i < 50; i++ {
		envelope := types.NewEnvelope(mailbox.MailboxID, types.QoSCommand, "test.command", []byte(fmt.Sprintf("payload-%d", i)), "org-1")
		_, err := repo.AppendEnvelope(ctx, mailbox.MailboxID, envelope)
		require.NoError(t, err)
	}

	// Fetch in pages of 10
	fromSeq := uint64(1)
	pageSize := 10
	totalFetched := 0

	for {
		envelopes, err := repo.FetchEnvelopes(ctx, mailbox.MailboxID, fromSeq, pageSize)
		assert.NoError(t, err)

		if len(envelopes) == 0 {
			break
		}

		// Verify sort order
		for i, env := range envelopes {
			assert.Equal(t, fromSeq+uint64(i), env.Seq)
		}

		totalFetched += len(envelopes)
		fromSeq = envelopes[len(envelopes)-1].Seq + 1

		if len(envelopes) < pageSize {
			break
		}
	}

	assert.Equal(t, 50, totalFetched)
}

func testDeleteExpired(t *testing.T, ctx context.Context, repo *MongoRepository) {
	// Create mailbox
	mailbox, err := repo.GetOrCreateMailbox(ctx, types.TargetTypeServer, "server-expire-1", "org-1")
	require.NoError(t, err)

	now := time.Now().UnixMilli()

	// Append expired envelopes
	for i := 0; i < 3; i++ {
		envelope := types.NewEnvelope(mailbox.MailboxID, types.QoSCommand, "test.command", []byte(fmt.Sprintf("expired-%d", i)), "org-1")
		envelope.ExpiresAtMs = now - 1000 // Expired 1 second ago
		_, err := repo.AppendEnvelope(ctx, mailbox.MailboxID, envelope)
		require.NoError(t, err)
	}

	// Append non-expired envelopes
	for i := 0; i < 2; i++ {
		envelope := types.NewEnvelope(mailbox.MailboxID, types.QoSCommand, "test.command", []byte(fmt.Sprintf("active-%d", i)), "org-1")
		envelope.ExpiresAtMs = now + 60000 // Expires in 1 minute
		_, err := repo.AppendEnvelope(ctx, mailbox.MailboxID, envelope)
		require.NoError(t, err)
	}

	// Append envelope without expiry
	envelope := types.NewEnvelope(mailbox.MailboxID, types.QoSCommand, "test.command", []byte("no-expiry"), "org-1")
	_, err = repo.AppendEnvelope(ctx, mailbox.MailboxID, envelope)
	require.NoError(t, err)

	// Delete expired
	deletedCount, err := repo.DeleteExpired(ctx, mailbox.MailboxID)
	assert.NoError(t, err)
	assert.Equal(t, int64(3), deletedCount)

	// Verify remaining envelopes
	envelopes, err := repo.FetchEnvelopes(ctx, mailbox.MailboxID, 1, 100)
	assert.NoError(t, err)
	assert.Len(t, envelopes, 3) // 2 active + 1 no-expiry
}

func testEnforceRetention(t *testing.T, ctx context.Context, repo *MongoRepository) {
	// Create mailbox
	mailbox, err := repo.GetOrCreateMailbox(ctx, types.TargetTypeServer, "server-retention-1", "org-1")
	require.NoError(t, err)

	// Append old envelopes (should be deleted by time-based retention)
	oldTime := time.Now().Add(-2 * time.Hour).UnixMilli()
	for i := 0; i < 5; i++ {
		envelope := types.NewEnvelope(mailbox.MailboxID, types.QoSCommand, "test.command", []byte(fmt.Sprintf("old-%d", i)), "org-1")
		envelope.CreatedAtMs = oldTime
		_, err := repo.AppendEnvelope(ctx, mailbox.MailboxID, envelope)
		require.NoError(t, err)
	}

	// Append recent envelopes
	for i := 0; i < 3; i++ {
		envelope := types.NewEnvelope(mailbox.MailboxID, types.QoSCommand, "test.command", []byte(fmt.Sprintf("recent-%d", i)), "org-1")
		_, err := repo.AppendEnvelope(ctx, mailbox.MailboxID, envelope)
		require.NoError(t, err)
	}

	// Enforce retention (1 hour retention)
	policy := types.RetentionPolicy{
		RetentionSeconds: 3600, // 1 hour
		MaxBytes:         1048576,
		MaxEnvelopes:     100,
	}
	err = repo.EnforceRetention(ctx, mailbox.MailboxID, policy)
	assert.NoError(t, err)

	// Verify old envelopes were deleted
	envelopes, err := repo.FetchEnvelopes(ctx, mailbox.MailboxID, 1, 100)
	assert.NoError(t, err)
	assert.Len(t, envelopes, 3) // Only recent envelopes remain
}

func testConcurrentAppend(t *testing.T, ctx context.Context, repo *MongoRepository) {
	// Create mailbox
	mailbox, err := repo.GetOrCreateMailbox(ctx, types.TargetTypeServer, "server-concurrent-1", "org-1")
	require.NoError(t, err)

	// Concurrently append envelopes
	numGoroutines := 10
	envelopesPerGoroutine := 10

	var wg sync.WaitGroup
	sequences := make(chan uint64, numGoroutines*envelopesPerGoroutine)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(routineID int) {
			defer wg.Done()
			for j := 0; j < envelopesPerGoroutine; j++ {
				envelope := types.NewEnvelope(
					mailbox.MailboxID,
					types.QoSCommand,
					"test.command",
					[]byte(fmt.Sprintf("routine-%d-env-%d", routineID, j)),
					"org-1",
				)
				seq, err := repo.AppendEnvelope(ctx, mailbox.MailboxID, envelope)
				if err != nil {
					t.Errorf("Failed to append envelope: %v", err)
					return
				}
				sequences <- seq
			}
		}(i)
	}

	wg.Wait()
	close(sequences)

	// Collect all sequences
	seqMap := make(map[uint64]bool)
	for seq := range sequences {
		if seqMap[seq] {
			t.Errorf("Duplicate sequence detected: %d", seq)
		}
		seqMap[seq] = true
	}

	// Verify we got all expected sequences
	expectedCount := numGoroutines * envelopesPerGoroutine
	assert.Len(t, seqMap, expectedCount)

	// Verify sequences are contiguous from 1 to expectedCount
	for i := 1; i <= expectedCount; i++ {
		assert.True(t, seqMap[uint64(i)], "Missing sequence %d", i)
	}

	// Verify final next_seq
	updatedMailbox, err := repo.GetMailbox(ctx, mailbox.MailboxID)
	assert.NoError(t, err)
	assert.Equal(t, uint64(expectedCount+1), updatedMailbox.NextSeq)
}

func testMailboxIndexes(t *testing.T, ctx context.Context, repo *MongoRepository) {
	// Test unique constraint on mailbox_id
	mailbox1, err := repo.GetOrCreateMailbox(ctx, types.TargetTypeServer, "server-idx-1", "org-1")
	require.NoError(t, err)

	// Manually try to insert duplicate mailbox_id (this should fail due to unique index)
	// Note: GetOrCreateMailbox handles this gracefully, so we're just verifying it works

	// Test unique constraint on (target_type, target_id, org_id)
	// Creating the same target should return existing mailbox
	mailbox2, err := repo.GetOrCreateMailbox(ctx, types.TargetTypeServer, "server-idx-1", "org-1")
	assert.NoError(t, err)
	assert.Equal(t, mailbox1.MailboxID, mailbox2.MailboxID)

	// Test envelope unique constraint on (mailbox_id, seq)
	envelope := types.NewEnvelope(mailbox1.MailboxID, types.QoSCommand, "test.command", []byte("payload"), "org-1")
	seq, err := repo.AppendEnvelope(ctx, mailbox1.MailboxID, envelope)
	require.NoError(t, err)

	// Manually try to insert duplicate (mailbox_id, seq) - would fail in real MongoDB
	// But AppendEnvelope auto-increments, so we can't easily test this without direct DB access
	_ = seq
}

func testNonAtomicSequenceIssue(t *testing.T, ctx context.Context, repo *MongoRepository) {
	t.Log("Testing documented non-atomic sequence allocation issue")
	t.Log("NOTE: This test documents the limitation that sequence allocation and envelope insertion are not atomic")
	t.Log("If envelope insertion fails after sequence allocation, a gap in the sequence will occur")
	t.Log("This is documented in mailbox_repository.go AppendEnvelope comments")

	// Create mailbox
	mailbox, err := repo.GetOrCreateMailbox(ctx, types.TargetTypeServer, "server-nonatomic-1", "org-1")
	require.NoError(t, err)

	// Append a few envelopes normally
	for i := 0; i < 3; i++ {
		envelope := types.NewEnvelope(mailbox.MailboxID, types.QoSCommand, "test.command", []byte(fmt.Sprintf("payload-%d", i)), "org-1")
		seq, err := repo.AppendEnvelope(ctx, mailbox.MailboxID, envelope)
		require.NoError(t, err)
		assert.Equal(t, uint64(i+1), seq)
	}

	// We cannot easily simulate the failure case without mocking, but we document the issue here
	// In a real failure scenario:
	// 1. FindOneAndUpdate increments next_seq (e.g., from 4 to 5)
	// 2. InsertOne fails (e.g., network error, disk full)
	// 3. Sequence 4 is now "consumed" but no envelope exists for it
	// 4. This creates a gap in the sequence, violating total order guarantee

	// TODO (Phase 4): Implement multi-document transactions to ensure atomicity
	// Use session.WithTransaction to wrap both operations

	t.Log("Integration test passed. Issue is documented and will be fixed in Phase 4 with transactions")
}

// startMongoContainerForMailbox starts a MongoDB testcontainer
func startMongoContainerForMailbox(ctx context.Context) (testcontainers.Container, string, error) {
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
