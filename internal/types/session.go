package types

import (
	"sync"
	"time"

	umsv1 "github.com/ambientlabscomputing/mycelium_spine/proto/ums/v1"
	"github.com/google/uuid"
)

// Session represents an active client connection
type Session struct {
	SessionID         string          `json:"session_id" bson:"session_id"`
	ServerID          string          `json:"server_id" bson:"server_id"` // Stable Underleaf identity
	OrgID             string          `json:"org_id" bson:"org_id"`
	SessionEpoch      uint64          `json:"session_epoch" bson:"session_epoch"` // Monotonic per server_id
	ResumeToken       string          `json:"resume_token" bson:"resume_token"`
	Subscriptions     []string        `json:"subscriptions" bson:"subscriptions"` // Mailbox IDs
	ConnectedAt       int64           `json:"connected_at" bson:"connected_at"`
	LastHeartbeat     int64           `json:"last_heartbeat" bson:"last_heartbeat"`
	DeviceFingerprint string          `json:"device_fingerprint,omitempty" bson:"device_fingerprint,omitempty"`
	ClientFeatures    map[string]bool `json:"client_features,omitempty" bson:"client_features,omitempty"`

	// Runtime state (not persisted)
	Stream        umsv1.SpineStream_ConnectServer `json:"-" bson:"-"` // Active gRPC stream
	InflightByQoS map[QoS]int                     `json:"-" bson:"-"` // Current inflight counts
	mu            sync.RWMutex                    `json:"-" bson:"-"` // Protects inflight counters
}

// NewSession creates a new session
func NewSession(serverID string, orgID string, epoch uint64, deviceFingerprint string, clientFeatures map[string]bool, stream umsv1.SpineStream_ConnectServer) *Session {
	now := time.Now().UnixMilli()
	sessionID := uuid.New().String()
	resumeToken := uuid.New().String()

	return &Session{
		SessionID:         sessionID,
		ServerID:          serverID,
		OrgID:             orgID,
		SessionEpoch:      epoch,
		ResumeToken:       resumeToken,
		Subscriptions:     []string{},
		ConnectedAt:       now,
		LastHeartbeat:     now,
		DeviceFingerprint: deviceFingerprint,
		ClientFeatures:    clientFeatures,
		Stream:            stream,
		InflightByQoS: map[QoS]int{
			QoSCommand:   0,
			QoSControl:   0,
			QoSTelemetry: 0,
		},
	}
}

// UpdateHeartbeat updates the last heartbeat timestamp
func (s *Session) UpdateHeartbeat() {
	s.LastHeartbeat = time.Now().UnixMilli()
}

// RotateResumeToken generates a new resume token
func (s *Session) RotateResumeToken() {
	s.ResumeToken = uuid.New().String()
}

// AddSubscription adds a mailbox to the session's subscriptions
func (s *Session) AddSubscription(mailboxID string) {
	s.Subscriptions = append(s.Subscriptions, mailboxID)
}

// HasSubscription checks if the session is subscribed to a mailbox
func (s *Session) HasSubscription(mailboxID string) bool {
	for _, id := range s.Subscriptions {
		if id == mailboxID {
			return true
		}
	}
	return false
}

// IncrementInflight increments the inflight counter for a QoS
func (s *Session) IncrementInflight(qos QoS) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.InflightByQoS[qos]++
}

// DecrementInflight decrements the inflight counter for a QoS
func (s *Session) DecrementInflight(qos QoS, count int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.InflightByQoS[qos] -= count
	if s.InflightByQoS[qos] < 0 {
		s.InflightByQoS[qos] = 0
	}
}

// GetInflight returns the current inflight count for a QoS
func (s *Session) GetInflight(qos QoS) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.InflightByQoS[qos]
}

// GetTotalInflight returns the total inflight count across all QoS
func (s *Session) GetTotalInflight() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	total := 0
	for _, count := range s.InflightByQoS {
		total += count
	}
	return total
}
