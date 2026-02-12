package types

import (
	"testing"
)

func TestNewMailbox(t *testing.T) {
	targetType := TargetTypeServer
	targetID := "server-123"
	orgID := "org-456"
	retentionPolicy := RetentionPolicy{
		RetentionSeconds: 86400,
		MaxBytes:         1048576,
		MaxEnvelopes:     1000,
	}

	mailbox := NewMailbox(targetType, targetID, orgID, retentionPolicy)

	if mailbox == nil {
		t.Fatal("expected non-nil mailbox")
	}

	if mailbox.MailboxID == "" {
		t.Error("expected non-empty mailbox ID")
	}

	if mailbox.TargetType != targetType {
		t.Errorf("expected target type %s, got %s", targetType, mailbox.TargetType)
	}

	if mailbox.TargetID != targetID {
		t.Errorf("expected target ID %s, got %s", targetID, mailbox.TargetID)
	}

	if mailbox.OrgID != orgID {
		t.Errorf("expected org ID %s, got %s", orgID, mailbox.OrgID)
	}

	if mailbox.NextSeq != 1 {
		t.Errorf("expected next seq 1, got %d", mailbox.NextSeq)
	}

	if mailbox.CreatedAt == "" {
		t.Error("expected non-empty created timestamp")
	}

	if mailbox.UpdatedAt == "" {
		t.Error("expected non-empty updated timestamp")
	}

	if mailbox.RetentionPolicy.RetentionSeconds != retentionPolicy.RetentionSeconds {
		t.Errorf("expected retention seconds %d, got %d", retentionPolicy.RetentionSeconds, mailbox.RetentionPolicy.RetentionSeconds)
	}
}

func TestMailboxIDGeneration(t *testing.T) {
	retentionPolicy := RetentionPolicy{
		RetentionSeconds: 86400,
		MaxBytes:         1048576,
		MaxEnvelopes:     1000,
	}

	mailbox1 := NewMailbox(TargetTypeServer, "server-1", "org-1", retentionPolicy)
	mailbox2 := NewMailbox(TargetTypeServer, "server-1", "org-1", retentionPolicy)

	if mailbox1.MailboxID == mailbox2.MailboxID {
		t.Error("expected different mailbox IDs for different instances")
	}
}

func TestNewMailboxCursor(t *testing.T) {
	sessionID := "session-abc-123"
	mailboxID := "mailbox-xyz-789"

	cursor := NewMailboxCursor(sessionID, mailboxID)

	if cursor == nil {
		t.Fatal("expected non-nil cursor")
	}

	if cursor.SessionID != sessionID {
		t.Errorf("expected session ID %s, got %s", sessionID, cursor.SessionID)
	}

	if cursor.MailboxID != mailboxID {
		t.Errorf("expected mailbox ID %s, got %s", mailboxID, cursor.MailboxID)
	}

	if cursor.LastAckedSeq != 0 {
		t.Errorf("expected initial last acked seq 0, got %d", cursor.LastAckedSeq)
	}

	if cursor.UpdatedAt == "" {
		t.Error("expected non-empty updated timestamp")
	}
}

func TestMailboxCursorAdvance(t *testing.T) {
	cursor := NewMailboxCursor("session-1", "mailbox-1")

	cursor.Advance(5)

	if cursor.LastAckedSeq != 5 {
		t.Errorf("expected last acked seq 5, got %d", cursor.LastAckedSeq)
	}

	if cursor.UpdatedAt == "" {
		t.Error("expected non-empty updated timestamp")
	}

	// Advancing to earlier sequence should not change cursor
	cursor.Advance(3)

	if cursor.LastAckedSeq != 5 {
		t.Errorf("expected last acked seq to remain 5, got %d", cursor.LastAckedSeq)
	}
}

func TestMailboxRetentionPolicy(t *testing.T) {
	retentionPolicy := RetentionPolicy{
		RetentionSeconds: 3600,
		MaxBytes:         524288,
		MaxEnvelopes:     500,
	}

	mailbox := NewMailbox(TargetTypeCluster, "cluster-1", "org-1", retentionPolicy)

	if mailbox.RetentionPolicy.RetentionSeconds != 3600 {
		t.Errorf("expected retention seconds 3600, got %d", mailbox.RetentionPolicy.RetentionSeconds)
	}

	if mailbox.RetentionPolicy.MaxEnvelopes != 500 {
		t.Errorf("expected max envelopes 500, got %d", mailbox.RetentionPolicy.MaxEnvelopes)
	}

	if mailbox.RetentionPolicy.MaxBytes != 524288 {
		t.Errorf("expected max bytes 524288, got %d", mailbox.RetentionPolicy.MaxBytes)
	}
}

func TestMailboxTargetTypes(t *testing.T) {
	retentionPolicy := RetentionPolicy{
		RetentionSeconds: 86400,
		MaxBytes:         1048576,
		MaxEnvelopes:     1000,
	}

	targetTypes := []TargetType{
		TargetTypeServer,
		TargetTypeCluster,
		TargetTypeOrg,
		TargetTypeService,
		TargetTypeBroadcast,
	}

	for _, targetType := range targetTypes {
		t.Run(string(targetType), func(t *testing.T) {
			mailbox := NewMailbox(targetType, "target-1", "org-1", retentionPolicy)

			if mailbox.TargetType != targetType {
				t.Errorf("expected target type %s, got %s", targetType, mailbox.TargetType)
			}
		})
	}
}
