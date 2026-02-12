package types

import (
	"testing"
	"time"
)

func TestNewEnvelope(t *testing.T) {
	mailboxID := "mailbox-123"
	qos := QoSCommand
	envelopeType := "command.deploy"
	payload := []byte(`{"action":"deploy"}`)
	orgID := "org-456"

	envelope := NewEnvelope(mailboxID, qos, envelopeType, payload, orgID)

	if envelope == nil {
		t.Fatal("expected non-nil envelope")
	}

	if envelope.EnvelopeID == "" {
		t.Error("expected non-empty envelope ID")
	}

	if envelope.MailboxID != mailboxID {
		t.Errorf("expected mailbox ID %s, got %s", mailboxID, envelope.MailboxID)
	}

	if envelope.QoS != qos {
		t.Errorf("expected QoS %s, got %s", qos, envelope.QoS)
	}

	if envelope.Type != envelopeType {
		t.Errorf("expected type %s, got %s", envelopeType, envelope.Type)
	}

	if string(envelope.Payload) != string(payload) {
		t.Errorf("expected payload %s, got %s", payload, envelope.Payload)
	}

	if envelope.OrgID != orgID {
		t.Errorf("expected org ID %s, got %s", orgID, envelope.OrgID)
	}

	if envelope.CreatedAtMs == 0 {
		t.Error("expected non-zero created timestamp")
	}

	if !envelope.RequiresAck {
		t.Error("expected RequiresAck to be true for COMMAND QoS")
	}
}

func TestEnvelopeRequiresAck(t *testing.T) {
	tests := []struct {
		qos         QoS
		requiresAck bool
	}{
		{QoSCommand, true},
		{QoSControl, true},
		{QoSTelemetry, false},
		{QoSUnspecified, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.qos), func(t *testing.T) {
			envelope := NewEnvelope("mailbox-1", tt.qos, "test", []byte("data"), "org-1")

			if envelope.RequiresAck != tt.requiresAck {
				t.Errorf("QoS %s: expected RequiresAck=%v, got %v",
					tt.qos, tt.requiresAck, envelope.RequiresAck)
			}
		})
	}
}

func TestEnvelopeWithSeq(t *testing.T) {
	envelope := NewEnvelope("mailbox-1", QoSCommand, "test", []byte("data"), "org-1")
	result := envelope.WithSeq(42)

	if result != envelope {
		t.Error("expected fluent interface to return same envelope")
	}

	if envelope.Seq != 42 {
		t.Errorf("expected seq 42, got %d", envelope.Seq)
	}
}

func TestEnvelopeWithTraceID(t *testing.T) {
	envelope := NewEnvelope("mailbox-1", QoSCommand, "test", []byte("data"), "org-1")
	traceID := "trace-abc-123"

	result := envelope.WithTraceID(traceID)

	if result != envelope {
		t.Error("expected fluent interface to return same envelope")
	}

	if envelope.TraceID != traceID {
		t.Errorf("expected trace ID %s, got %s", traceID, envelope.TraceID)
	}
}

func TestEnvelopeWithDedupeKey(t *testing.T) {
	envelope := NewEnvelope("mailbox-1", QoSCommand, "test", []byte("data"), "org-1")
	dedupeKey := "dedupe-xyz-789"

	result := envelope.WithDedupeKey(dedupeKey)

	if result != envelope {
		t.Error("expected fluent interface to return same envelope")
	}

	if envelope.DedupeKey != dedupeKey {
		t.Errorf("expected dedupe key %s, got %s", dedupeKey, envelope.DedupeKey)
	}
}

func TestEnvelopeWithExpiry(t *testing.T) {
	envelope := NewEnvelope("mailbox-1", QoSCommand, "test", []byte("data"), "org-1")
	expiresAt := time.Now().Add(1 * time.Hour).UnixMilli()

	result := envelope.WithExpiry(expiresAt)

	if result != envelope {
		t.Error("expected fluent interface to return same envelope")
	}

	if envelope.ExpiresAtMs != expiresAt {
		t.Errorf("expected expiry %d, got %d", expiresAt, envelope.ExpiresAtMs)
	}
}

func TestEnvelopeWithPriority(t *testing.T) {
	envelope := NewEnvelope("mailbox-1", QoSCommand, "test", []byte("data"), "org-1")
	priority := int32(5)

	result := envelope.WithPriority(priority)

	if result != envelope {
		t.Error("expected fluent interface to return same envelope")
	}

	if envelope.Priority != priority {
		t.Errorf("expected priority %d, got %d", priority, envelope.Priority)
	}
}

func TestEnvelopeIsExpired(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt int64
		expected  bool
	}{
		{
			name:      "no expiry set",
			expiresAt: 0,
			expected:  false,
		},
		{
			name:      "expired in past",
			expiresAt: time.Now().Add(-1 * time.Hour).UnixMilli(),
			expected:  true,
		},
		{
			name:      "expires in future",
			expiresAt: time.Now().Add(1 * time.Hour).UnixMilli(),
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			envelope := NewEnvelope("mailbox-1", QoSCommand, "test", []byte("data"), "org-1")
			envelope.ExpiresAtMs = tt.expiresAt

			result := envelope.IsExpired()
			if result != tt.expected {
				t.Errorf("expected IsExpired=%v, got %v", tt.expected, result)
			}
		})
	}
}

func TestEnvelopeFluentInterface(t *testing.T) {
	envelope := NewEnvelope("mailbox-1", QoSCommand, "test.command", []byte("data"), "org-1")

	result := envelope.
		WithSeq(100).
		WithTraceID("trace-123").
		WithDedupeKey("dedupe-456").
		WithExpiry(time.Now().Add(5 * time.Minute).UnixMilli()).
		WithPriority(10)

	if result != envelope {
		t.Error("expected fluent interface to return same envelope")
	}

	if envelope.Seq != 100 {
		t.Errorf("expected seq 100, got %d", envelope.Seq)
	}

	if envelope.TraceID != "trace-123" {
		t.Errorf("expected trace ID trace-123, got %s", envelope.TraceID)
	}

	if envelope.DedupeKey != "dedupe-456" {
		t.Errorf("expected dedupe key dedupe-456, got %s", envelope.DedupeKey)
	}

	if envelope.ExpiresAtMs == 0 {
		t.Error("expected non-zero expiry")
	}

	if envelope.Priority != 10 {
		t.Errorf("expected priority 10, got %d", envelope.Priority)
	}
}
