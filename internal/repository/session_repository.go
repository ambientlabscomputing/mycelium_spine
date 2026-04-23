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

// GetSessionByResumeToken retrieves a session using its resume token
func (r *MongoSessionRepository) GetSessionByResumeToken(ctx context.Context, resumeToken string) (*types.Session, error) {
	var session types.Session
	err := r.sessions.FindOne(ctx, bson.M{"resume_token": resumeToken}).Decode(&session)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("no session found for resume token")
		}
		return nil, fmt.Errorf("failed to get session by resume token: %w", err)
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

// UpdateAckPosition advances the ack cursor for a mailbox, keyed by server_id.
// Using server_id (not session_id) ensures ack progress survives session rotation and reconnects.
func (r *MongoSessionRepository) UpdateAckPosition(ctx context.Context, serverID string, mailboxID string, seq uint64) error {
	filter := bson.M{
		"server_id":  serverID,
		"mailbox_id": mailboxID,
	}

	update := bson.M{
		"$max": bson.M{
			"last_acked_seq": seq,
		},
		"$set": bson.M{
			"updated_at": time.Now().Format(time.RFC3339),
		},
		"$setOnInsert": bson.M{
			"server_id":  serverID,
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

// GetAckPositions retrieves all ack positions for a server (keyed by server_id).
func (r *MongoSessionRepository) GetAckPositions(ctx context.Context, serverID string) (map[string]uint64, error) {
	cursor, err := r.cursors.Find(ctx, bson.M{"server_id": serverID})
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

// DeleteSession removes a session document only.
// Ack cursors are intentionally NOT deleted because they are keyed by server_id and must
// survive session rotation to prevent re-delivery of already-acknowledged commands.
// Use DeleteServerCursors to explicitly remove cursors when decommissioning a server.
func (r *MongoSessionRepository) DeleteSession(ctx context.Context, sessionID string) error {
	// Delete session document only
	_, err := r.sessions.DeleteOne(ctx, bson.M{"session_id": sessionID})
	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	return nil
}

// DeleteServerCursors removes all ack cursors for a server.
// Only call this when permanently decommissioning a server, never during normal reconnect.
func (r *MongoSessionRepository) DeleteServerCursors(ctx context.Context, serverID string) error {
	_, err := r.cursors.DeleteMany(ctx, bson.M{"server_id": serverID})
	if err != nil {
		return fmt.Errorf("failed to delete server cursors: %w", err)
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

// UpdateSubscriptions persists the session's subscription list to MongoDB
func (r *MongoSessionRepository) UpdateSubscriptions(ctx context.Context, sessionID string, mailboxIDs []string) error {
	filter := bson.M{"session_id": sessionID}
	update := bson.M{
		"$set": bson.M{
			"subscriptions": mailboxIDs,
			"updated_at":    time.Now().Format(time.RFC3339),
		},
	}

	result, err := r.sessions.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("failed to update subscriptions: %w", err)
	}

	if result.MatchedCount == 0 {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	return nil
}

// ListCursors returns ACK cursors with optional filtering by server_id or mailbox_id.
func (r *MongoSessionRepository) ListCursors(ctx context.Context, serverID string, mailboxID string, limit, offset int) ([]*types.MailboxCursor, int64, error) {
	filter := bson.M{}
	if serverID != "" {
		filter["server_id"] = serverID
	}
	if mailboxID != "" {
		filter["mailbox_id"] = mailboxID
	}

	if limit <= 0 {
		limit = 50
	}

	total, err := r.cursors.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count cursors: %w", err)
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "server_id", Value: 1}, {Key: "mailbox_id", Value: 1}}).
		SetLimit(int64(limit)).
		SetSkip(int64(offset))

	cur, err := r.cursors.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list cursors: %w", err)
	}
	defer cur.Close(ctx)

	var results []*types.MailboxCursor
	if err := cur.All(ctx, &results); err != nil {
		return nil, 0, fmt.Errorf("failed to decode cursors: %w", err)
	}

	return results, total, nil
}

// ResetCursor sets last_acked_seq to the specified value for a (server_id, mailbox_id) cursor.
func (r *MongoSessionRepository) ResetCursor(ctx context.Context, serverID string, mailboxID string, seq uint64) error {
	filter := bson.M{
		"server_id":  serverID,
		"mailbox_id": mailboxID,
	}
	update := bson.M{
		"$set": bson.M{
			"last_acked_seq": seq,
			"updated_at":     time.Now().Format(time.RFC3339),
		},
	}

	result, err := r.cursors.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("failed to reset cursor: %w", err)
	}

	if result.MatchedCount == 0 {
		return fmt.Errorf("cursor not found for server_id=%s mailbox_id=%s", serverID, mailboxID)
	}

	return nil
}
