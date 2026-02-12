package types

import (
	"testing"
)

func TestNewTarget(t *testing.T) {
	targetType := TargetTypeServer
	targetID := "server-abc-123"
	orgID := "org-xyz-789"

	target := NewTarget(targetType, targetID, orgID)

	if target == nil {
		t.Fatal("expected non-nil target")
	}

	if target.TargetType != targetType {
		t.Errorf("expected target type %s, got %s", targetType, target.TargetType)
	}

	if target.TargetID != targetID {
		t.Errorf("expected target ID %s, got %s", targetID, target.TargetID)
	}

	if target.OrgID != orgID {
		t.Errorf("expected org ID %s, got %s", orgID, target.OrgID)
	}

	if target.DeliverRole != "" {
		t.Errorf("expected empty deliver role by default, got %s", target.DeliverRole)
	}
}

func TestTargetWithDeliverRole(t *testing.T) {
	target := NewTarget(TargetTypeCluster, "cluster-1", "org-1")

	result := target.WithDeliverRole("LEADER")

	if result != target {
		t.Error("expected fluent interface to return same target")
	}

	if target.DeliverRole != "LEADER" {
		t.Errorf("expected deliver role 'LEADER', got %s", target.DeliverRole)
	}
}

func TestTargetToMailboxKey(t *testing.T) {
	tests := []struct {
		name        string
		targetType  TargetType
		targetID    string
		orgID       string
		expectedKey string
	}{
		{
			name:        "server target",
			targetType:  TargetTypeServer,
			targetID:    "server-123",
			orgID:       "org-456",
			expectedKey: "SERVER:server-123:org-456",
		},
		{
			name:        "cluster target",
			targetType:  TargetTypeCluster,
			targetID:    "cluster-abc",
			orgID:       "org-xyz",
			expectedKey: "CLUSTER:cluster-abc:org-xyz",
		},
		{
			name:        "org target",
			targetType:  TargetTypeOrg,
			targetID:    "org-global",
			orgID:       "org-global",
			expectedKey: "ORG:org-global:org-global",
		},
		{
			name:        "service target",
			targetType:  TargetTypeService,
			targetID:    "auth-service",
			orgID:       "org-789",
			expectedKey: "SERVICE:auth-service:org-789",
		},
		{
			name:        "broadcast target",
			targetType:  TargetTypeBroadcast,
			targetID:    "all",
			orgID:       "org-001",
			expectedKey: "BROADCAST:all:org-001",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := NewTarget(tt.targetType, tt.targetID, tt.orgID)

			key := target.ToMailboxKey()

			if key != tt.expectedKey {
				t.Errorf("expected mailbox key %s, got %s", tt.expectedKey, key)
			}
		})
	}
}

func TestTargetToMailboxKeyUniqueness(t *testing.T) {
	target1 := NewTarget(TargetTypeServer, "server-1", "org-a")
	target2 := NewTarget(TargetTypeServer, "server-1", "org-b")

	key1 := target1.ToMailboxKey()
	key2 := target2.ToMailboxKey()

	if key1 == key2 {
		t.Error("expected different mailbox keys for different orgs")
	}

	target3 := NewTarget(TargetTypeServer, "server-1", "org-a")
	key3 := target3.ToMailboxKey()

	if key1 != key3 {
		t.Error("expected same mailbox key for identical targets")
	}
}

func TestTargetTypes(t *testing.T) {
	expectedTypes := []TargetType{
		TargetTypeServer,
		TargetTypeCluster,
		TargetTypeOrg,
		TargetTypeService,
		TargetTypeBroadcast,
	}

	for _, targetType := range expectedTypes {
		t.Run(string(targetType), func(t *testing.T) {
			target := NewTarget(targetType, "target-id", "org-id")

			if target.TargetType != targetType {
				t.Errorf("expected target type %s, got %s", targetType, target.TargetType)
			}
		})
	}
}
