package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/ambientlabscomputing/mycelium_spine/internal/types"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// MongoMailboxRepository implements MailboxRepository using MongoDB
type MongoMailboxRepository struct {
	mailboxes mongo.Collection
	envelopes *mongo.Collection
}

// GetOrCreateMailbox finds an existing mailbox or creates it
func (r *MongoMailboxRepository) GetOrCreateMailbox(ctx context.Context, targetType types.TargetType, targetID string, orgID string) (*types.Mailbox, error) {
	// Try to find existing mailbox
	filter := bson.M{
		"target_type": targetType,
		"target_id":   targetID,
		"org_id":      orgID,
	}

	var mailbox types.Mailbox
	err := r.mailboxes.FindOne(ctx, filter).Decode(&mailbox)
	if err == nil {
		return &mailbox, nil
	}

	if err != mongo.ErrNoDocuments {
		return nil, fmt.Errorf("failed to query mailbox: %w", err)
	}

	// Mailbox doesn't exist, create it with default retention from settings
	retentionPolicy := types.RetentionPolicy{
		RetentionSeconds: 86400,     // 24 hours default
		MaxBytes:         104857600, // 100MB default
		MaxEnvelopes:     100000,    // 100k default
	}

	newMailbox := types.NewMailbox(targetType, targetID, orgID, retentionPolicy)

	_, err = r.mailboxes.InsertOne(ctx, newMailbox)
	if err != nil {
		// Check if another thread just created it (race condition)
		err2 := r.mailboxes.FindOne(ctx, filter).Decode(&mailbox)
		if err2 == nil {
			return &mailbox, nil
		}
		return nil, fmt.Errorf("failed to create mailbox: %w", err)
	}

	return newMailbox, nil
}

// GetMailbox retrieves a mailbox by ID
func (r *MongoMailboxRepository) GetMailbox(ctx context.Context, mailboxID string) (*types.Mailbox, error) {
	var mailbox types.Mailbox
	err := r.mailboxes.FindOne(ctx, bson.M{"mailbox_id": mailboxID}).Decode(&mailbox)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("mailbox not found: %s", mailboxID)
		}
		return nil, fmt.Errorf("failed to get mailbox: %w", err)
	}
	return &mailbox, nil
}

// AppendEnvelope atomically assigns a sequence number and inserts an envelope.
// IMPORTANT: This implementation is NOT fully atomic. The seq is allocated via FindOneAndUpdate,
// then the envelope is inserted separately. If InsertOne fails, the seq is consumed creating
// a gap in the sequence, violating spec §12 (total order per mailbox).
// TODO (Phase 4): Wrap in a MongoDB multi-document transaction to ensure atomicity.
// For now, document the limitation and assume failures are transient.
func (r *MongoMailboxRepository) AppendEnvelope(ctx context.Context, mailboxID string, envelope *types.Envelope) (uint64, error) {
	// Use MongoDB findOneAndUpdate with $inc to atomically get next sequence
	filter := bson.M{"mailbox_id": mailboxID}
	update := bson.M{
		"$inc": bson.M{"next_seq": 1},
		"$set": bson.M{"updated_at": time.Now().Format(time.RFC3339)},
	}
	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)

	var mailbox types.Mailbox
	err := r.mailboxes.FindOneAndUpdate(ctx, filter, update, opts).Decode(&mailbox)
	if err != nil {
		return 0, fmt.Errorf("failed to allocate sequence: %w", err)
	}

	seq := mailbox.NextSeq - 1 // We just incremented, so the assigned seq is NextSeq - 1
	envelope.Seq = seq
	envelope.MailboxID = mailboxID

	// Insert the envelope
	_, err = r.envelopes.InsertOne(ctx, envelope)
	if err != nil {
		return 0, fmt.Errorf("failed to insert envelope: %w", err)
	}

	return seq, nil
}

// FetchEnvelopes retrieves envelopes from a mailbox starting at fromSeq
func (r *MongoMailboxRepository) FetchEnvelopes(ctx context.Context, mailboxID string, fromSeq uint64, limit int) ([]*types.Envelope, error) {
	filter := bson.M{
		"mailbox_id": mailboxID,
		"seq":        bson.M{"$gte": fromSeq},
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "seq", Value: 1}}).
		SetLimit(int64(limit))

	cursor, err := r.envelopes.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch envelopes: %w", err)
	}
	defer cursor.Close(ctx)

	var envelopes []*types.Envelope
	if err := cursor.All(ctx, &envelopes); err != nil {
		return nil, fmt.Errorf("failed to decode envelopes: %w", err)
	}

	return envelopes, nil
}

// DeleteExpired removes expired envelopes from a mailbox
func (r *MongoMailboxRepository) DeleteExpired(ctx context.Context, mailboxID string) (int64, error) {
	now := time.Now().UnixMilli()
	filter := bson.M{
		"mailbox_id":    mailboxID,
		"expires_at_ms": bson.M{"$gt": 0, "$lte": now},
	}

	result, err := r.envelopes.DeleteMany(ctx, filter)
	if err != nil {
		return 0, fmt.Errorf("failed to delete expired envelopes: %w", err)
	}

	return result.DeletedCount, nil
}

// EnforceRetention ensures mailbox stays within retention policy limits
func (r *MongoMailboxRepository) EnforceRetention(ctx context.Context, mailboxID string, policy types.RetentionPolicy) error {
	// Delete by age
	cutoffTime := time.Now().Add(-time.Duration(policy.RetentionSeconds) * time.Second).UnixMilli()
	_, err := r.envelopes.DeleteMany(ctx, bson.M{
		"mailbox_id":    mailboxID,
		"created_at_ms": bson.M{"$lt": cutoffTime},
	})
	if err != nil {
		return fmt.Errorf("failed to enforce time-based retention: %w", err)
	}

	// Count envelopes and delete oldest if over limit
	count, err := r.envelopes.CountDocuments(ctx, bson.M{"mailbox_id": mailboxID})
	if err != nil {
		return fmt.Errorf("failed to count envelopes: %w", err)
	}

	if count > int64(policy.MaxEnvelopes) {
		// Find oldest envelopes and delete them
		excess := count - int64(policy.MaxEnvelopes)
		opts := options.Find().SetSort(bson.D{{Key: "seq", Value: 1}}).SetLimit(excess)
		cursor, err := r.envelopes.Find(ctx, bson.M{"mailbox_id": mailboxID}, opts)
		if err != nil {
			return fmt.Errorf("failed to find excess envelopes: %w", err)
		}
		defer cursor.Close(ctx)

		var toDelete []uint64
		for cursor.Next(ctx) {
			var env types.Envelope
			if err := cursor.Decode(&env); err != nil {
				continue
			}
			toDelete = append(toDelete, env.Seq)
		}

		if len(toDelete) > 0 {
			_, err = r.envelopes.DeleteMany(ctx, bson.M{
				"mailbox_id": mailboxID,
				"seq":        bson.M{"$in": toDelete},
			})
			if err != nil {
				return fmt.Errorf("failed to delete excess envelopes: %w", err)
			}
		}
	}

	return nil
}
