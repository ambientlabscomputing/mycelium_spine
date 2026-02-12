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

// MongoSessionRepository implements SessionRepository using MongoDB
type MongoSessionRepository struct {
	sessions *mongo.Collection
	cursors  *mongo.Collection
}

// CreateSession persists a new session
func (r *MongoSessionRepository) CreateSession(ctx context.Context, session *types.Session) error {
	_, err := r.sessions.InsertOne(ctx, session)
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	return nil
}

// GetSession retrieves a session by ID
func (r *MongoSessionRepository) GetSession(ctx context.Context, sessionID string) (*types.Session, error) {
	var session types.Session
	err := r.sessions.FindOne(ctx, bson.M{"session_id": sessionID}).Decode(&session)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("session not found: %s", sessionID)
		}
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	// Initialize runtime state
	session.InflightByQoS = map[types.QoS]int{
		types.QoSCommand:   0,
		types.QoSControl:   0,
		types.QoSTelemetry: 0,
	}

	return &session, nil
}

// GetSessionByServerID retrieves the active session for a server_id
func (r *MongoSessionRepository) GetSessionByServerID(ctx context.Context, serverID string) (*types.Session, error) {
	var session types.Session
	opts := options.FindOne().SetSort(bson.D{{Key: "session_epoch", Value: -1}})
	err := r.sessions.FindOne(ctx, bson.M{"server_id": serverID}, opts).Decode(&session)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil // No active session
		}
		return nil, fmt.Errorf("failed to get session by server_id: %w", err)
	}

	// Initialize runtime state
	session.InflightByQoS = map[types.QoS]int{
		types.QoSCommand:   0,
		types.QoSControl:   0,
		types.QoSTelemetry: 0,
	}

	return &session, nil
}

// UpdateResumeToken rotates the resume token for a session
func (r *MongoSessionRepository) UpdateResumeToken(ctx context.Context, sessionID string, newToken string) error {
	filter := bson.M{"session_id": sessionID}
	update := bson.M{"$set": bson.M{"resume_token": newToken}}

	result, err := r.sessions.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("failed to update resume token: %w", err)
	}

	if result.MatchedCount == 0 {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	return nil
}

// UpdateAckPosition advances the ack cursor for a mailbox
func (r *MongoSessionRepository) UpdateAckPosition(ctx context.Context, sessionID string, mailboxID string, seq uint64) error {
	filter := bson.M{
		"session_id": sessionID,
		"mailbox_id": mailboxID,
	}

	update := bson.M{
		"$set": bson.M{
			"last_acked_seq": seq,
			"updated_at":     time.Now().Format(time.RFC3339),
		},
		"$setOnInsert": bson.M{
			"session_id": sessionID,
			"mailbox_id": mailboxID,
		},
	}

	opts := options.Update().SetUpsert(true)
	_, err := r.cursors.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return fmt.Errorf("failed to update ack position: %w", err)
	}

	return nil
}

// GetAckPositions retrieves all ack positions for a session
func (r *MongoSessionRepository) GetAckPositions(ctx context.Context, sessionID string) (map[string]uint64, error) {
	cursor, err := r.cursors.Find(ctx, bson.M{"session_id": sessionID})
	if err != nil {
		return nil, fmt.Errorf("failed to get ack positions: %w", err)
	}
	defer cursor.Close(ctx)

	positions := make(map[string]uint64)
	for cursor.Next(ctx) {
		var mc types.MailboxCursor
		if err := cursor.Decode(&mc); err != nil {
			continue
		}
		positions[mc.MailboxID] = mc.LastAckedSeq
	}

	return positions, nil
}

// DeleteSession removes a session and its cursors
func (r *MongoSessionRepository) DeleteSession(ctx context.Context, sessionID string) error {
	// Delete session
	_, err := r.sessions.DeleteOne(ctx, bson.M{"session_id": sessionID})
	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	// Delete associated cursors
	_, err = r.cursors.DeleteMany(ctx, bson.M{"session_id": sessionID})
	if err != nil {
		return fmt.Errorf("failed to delete cursors: %w", err)
	}

	return nil
}

// UpdateHeartbeat updates the session's last heartbeat timestamp
func (r *MongoSessionRepository) UpdateHeartbeat(ctx context.Context, sessionID string) error {
	filter := bson.M{"session_id": sessionID}
	update := bson.M{"$set": bson.M{"last_heartbeat": time.Now().Format(time.RFC3339)}}

	result, err := r.sessions.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("failed to update heartbeat: %w", err)
	}

	if result.MatchedCount == 0 {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	return nil
}
