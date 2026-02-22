package repository

import (
	"context"
	"fmt"
	"log/slog"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// MongoRepository is the main MongoDB implementation of Repository
type MongoRepository struct {
	*MongoMailboxRepository
	*MongoSessionRepository
	*MongoTargetResolver

	db     *mongo.Database
	logger *slog.Logger
}

// NewMongoRepository creates a new MongoDB repository with all sub-repositories
func NewMongoRepository(db *mongo.Database, logger *slog.Logger) *MongoRepository {
	mailboxRepo := &MongoMailboxRepository{
		mailboxes: *db.Collection("mailboxes"),
		envelopes: db.Collection("envelopes"),
	}

	sessionRepo := &MongoSessionRepository{
		sessions: db.Collection("sessions"),
		cursors:  db.Collection("cursors"),
	}

	targetResolver := &MongoTargetResolver{
		mailboxRepo: mailboxRepo,
	}

	return &MongoRepository{
		MongoMailboxRepository: mailboxRepo,
		MongoSessionRepository: sessionRepo,
		MongoTargetResolver:    targetResolver,
		db:                     db,
		logger:                 logger,
	}
}

// CreateIndexes creates all required MongoDB indexes
func (r *MongoRepository) CreateIndexes(ctx context.Context) error {
	r.logger.Info("creating MongoDB indexes")

	// Mailboxes indexes
	mailboxIndexes := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "mailbox_id", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys: bson.D{
				{Key: "target_type", Value: 1},
				{Key: "target_id", Value: 1},
				{Key: "org_id", Value: 1},
			},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys: bson.D{{Key: "org_id", Value: 1}},
		},
	}

	mailboxesCollection := r.db.Collection("mailboxes")
	_, err := mailboxesCollection.Indexes().CreateMany(ctx, mailboxIndexes)
	if err != nil {
		return fmt.Errorf("failed to create mailboxes indexes: %w", err)
	}

	// Envelopes indexes
	envelopeIndexes := []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "mailbox_id", Value: 1},
				{Key: "seq", Value: 1},
			},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys:    bson.D{{Key: "envelope_id", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys: bson.D{{Key: "mailbox_id", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "org_id", Value: 1}},
		},
		// NOTE: TTL index disabled - expires_at_ms is int64 but MongoDB TTL indexes require Date type.
		// Retention is enforced by background janitor goroutine (see service.go Start/EvictExpired)
		// TODO: Convert expires_at_ms to time.Time or implement proper TTL index
		// {
		// 	Keys: bson.D{{Key: "expires_at_ms", Value: 1}},
		// 	Options: options.Index().SetExpireAfterSeconds(0).
		// 		SetPartialFilterExpression(bson.D{{Key: "expires_at_ms", Value: bson.D{{Key: "$gt", Value: 0}}}}),
		// },
	}

	envelopesCollection := r.db.Collection("envelopes")
	_, err = envelopesCollection.Indexes().CreateMany(ctx, envelopeIndexes)
	if err != nil {
		return fmt.Errorf("failed to create envelopes indexes: %w", err)
	}

	// Sessions indexes
	sessionIndexes := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "session_id", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys: bson.D{{Key: "server_id", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "resume_token", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "org_id", Value: 1}},
		},
	}

	sessionsCollection := r.db.Collection("sessions")
	_, err = sessionsCollection.Indexes().CreateMany(ctx, sessionIndexes)
	if err != nil {
		return fmt.Errorf("failed to create sessions indexes: %w", err)
	}

	// Cursors indexes
	cursorIndexes := []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "session_id", Value: 1},
				{Key: "mailbox_id", Value: 1},
			},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys: bson.D{{Key: "session_id", Value: 1}},
		},
	}

	cursorsCollection := r.db.Collection("cursors")
	_, err = cursorsCollection.Indexes().CreateMany(ctx, cursorIndexes)
	if err != nil {
		return fmt.Errorf("failed to create cursors indexes: %w", err)
	}

	r.logger.Info("MongoDB indexes created successfully")
	return nil
}
