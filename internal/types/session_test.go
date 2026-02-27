package types

import (
	"testing"
)

func TestNewSession(t *testing.T) {
	serverID := "server-abc-123"
	orgID := "org-xyz-789"
	epoch := uint64(5)
	deviceFingerprint := "device-fp-001"
	clientFeatures := map[string]bool{
		"compression": true,
		"batching":    false,
	}

	session := NewSession(serverID, orgID, epoch, deviceFingerprint, clientFeatures, nil)

	if session == nil {
		t.Fatal("expected non-nil session")
	}

	if session.SessionID == "" {
		t.Error("expected non-empty session ID")
	}

	if session.ServerID != serverID {
		t.Errorf("expected server ID %s, got %s", serverID, session.ServerID)
	}

	if session.OrgID != orgID {
		t.Errorf("expected org ID %s, got %s", orgID, session.OrgID)
	}

	if session.SessionEpoch != epoch {
		t.Errorf("expected epoch %d, got %d", epoch, session.SessionEpoch)
	}

	if session.ResumeToken == "" {
		t.Error("expected non-empty resume token")
	}

	if session.DeviceFingerprint != deviceFingerprint {
		t.Errorf("expected device fingerprint %s, got %s", deviceFingerprint, session.DeviceFingerprint)
	}

	if len(session.ClientFeatures) != 2 {
		t.Errorf("expected 2 client features, got %d", len(session.ClientFeatures))
	}

	if session.ConnectedAt == "" {
		t.Error("expected non-empty connected timestamp")
	}

	if session.LastHeartbeat == "" {
		t.Error("expected non-empty heartbeat timestamp")
	}

	if len(session.Subscriptions) != 0 {
		t.Errorf("expected empty subscriptions, got %d", len(session.Subscriptions))
	}

	if session.InflightByQoS == nil {
		t.Fatal("expected non-nil inflight map")
	}

	if len(session.InflightByQoS) != 3 {
		t.Errorf("expected 3 QoS levels initialized, got %d", len(session.InflightByQoS))
	}

	for _, qos := range []QoS{QoSCommand, QoSControl, QoSTelemetry} {
		if session.InflightByQoS[qos] != 0 {
			t.Errorf("expected inflight count 0 for %s, got %d", qos, session.InflightByQoS[qos])
		}
	}
}

func TestSessionUpdateHeartbeat(t *testing.T) {
	session := NewSession("server-1", "org-1", 1, "device-1", nil, nil)

	session.UpdateHeartbeat()

	if session.LastHeartbeat == "" {
		t.Error("expected non-empty heartbeat timestamp")
	}
}

func TestSessionRotateResumeToken(t *testing.T) {
	session := NewSession("server-1", "org-1", 1, "device-1", nil, nil)

	initialToken := session.ResumeToken

	session.RotateResumeToken()

	if session.ResumeToken == initialToken {
		t.Error("expected resume token to change")
	}

	if session.ResumeToken == "" {
		t.Error("expected non-empty resume token after rotation")
	}
}

func TestSessionAddSubscription(t *testing.T) {
	session := NewSession("server-1", "org-1", 1, "device-1", nil, nil)

	mailboxID1 := "mailbox-abc"
	mailboxID2 := "mailbox-xyz"

	session.AddSubscription(mailboxID1)

	if len(session.Subscriptions) != 1 {
		t.Errorf("expected 1 subscription, got %d", len(session.Subscriptions))
	}

	if session.Subscriptions[0] != mailboxID1 {
		t.Errorf("expected subscription %s, got %s", mailboxID1, session.Subscriptions[0])
	}

	session.AddSubscription(mailboxID2)

	if len(session.Subscriptions) != 2 {
		t.Errorf("expected 2 subscriptions, got %d", len(session.Subscriptions))
	}
}

func TestSessionHasSubscription(t *testing.T) {
	session := NewSession("server-1", "org-1", 1, "device-1", nil, nil)

	mailboxID1 := "mailbox-abc"
	mailboxID2 := "mailbox-xyz"
	mailboxID3 := "mailbox-not-subscribed"

	session.AddSubscription(mailboxID1)
	session.AddSubscription(mailboxID2)

	if !session.HasSubscription(mailboxID1) {
		t.Errorf("expected session to have subscription %s", mailboxID1)
	}

	if !session.HasSubscription(mailboxID2) {
		t.Errorf("expected session to have subscription %s", mailboxID2)
	}

	if session.HasSubscription(mailboxID3) {
		t.Errorf("expected session to not have subscription %s", mailboxID3)
	}
}

func TestSessionIncrementInflight(t *testing.T) {
	session := NewSession("server-1", "org-1", 1, "device-1", nil, nil)

	session.IncrementInflight(QoSCommand)

	if session.GetInflight(QoSCommand) != 1 {
		t.Errorf("expected inflight count 1 for COMMAND, got %d", session.GetInflight(QoSCommand))
	}

	session.IncrementInflight(QoSCommand)
	session.IncrementInflight(QoSCommand)

	if session.GetInflight(QoSCommand) != 3 {
		t.Errorf("expected inflight count 3 for COMMAND, got %d", session.GetInflight(QoSCommand))
	}

	if session.GetInflight(QoSControl) != 0 {
		t.Errorf("expected inflight count 0 for CONTROL, got %d", session.GetInflight(QoSControl))
	}
}

func TestSessionDecrementInflight(t *testing.T) {
	session := NewSession("server-1", "org-1", 1, "device-1", nil, nil)

	session.IncrementInflight(QoSControl)
	session.IncrementInflight(QoSControl)
	session.IncrementInflight(QoSControl)

	if session.GetInflight(QoSControl) != 3 {
		t.Errorf("expected inflight count 3, got %d", session.GetInflight(QoSControl))
	}

	session.DecrementInflight(QoSControl, 2)

	if session.GetInflight(QoSControl) != 1 {
		t.Errorf("expected inflight count 1, got %d", session.GetInflight(QoSControl))
	}

	session.DecrementInflight(QoSControl, 10)

	if session.GetInflight(QoSControl) != 0 {
		t.Errorf("expected inflight count 0 (floored), got %d", session.GetInflight(QoSControl))
	}
}

func TestSessionGetTotalInflight(t *testing.T) {
	session := NewSession("server-1", "org-1", 1, "device-1", nil, nil)

	session.IncrementInflight(QoSCommand)
	session.IncrementInflight(QoSCommand)
	session.IncrementInflight(QoSControl)
	session.IncrementInflight(QoSControl)
	session.IncrementInflight(QoSControl)
	session.IncrementInflight(QoSTelemetry)

	total := session.GetTotalInflight()
	expected := 6

	if total != expected {
		t.Errorf("expected total inflight %d, got %d", expected, total)
	}
}

func TestSessionInflightConcurrency(t *testing.T) {
	session := NewSession("server-1", "org-1", 1, "device-1", nil, nil)

	done := make(chan bool)
	iterations := 100

	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < iterations; j++ {
				session.IncrementInflight(QoSCommand)
			}
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	expected := 10 * iterations
	actual := session.GetInflight(QoSCommand)

	if actual != expected {
		t.Errorf("expected inflight count %d, got %d (race condition?)", expected, actual)
	}
}
