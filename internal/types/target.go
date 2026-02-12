package types

// Target identifies a routing destination for envelopes
type Target struct {
	TargetType  TargetType `json:"target_type" bson:"target_type"`
	TargetID    string     `json:"target_id" bson:"target_id"`
	OrgID       string     `json:"org_id" bson:"org_id"`
	DeliverRole string     `json:"deliver_role,omitempty" bson:"deliver_role,omitempty"` // Optional: "LEADER" for cluster fanout
}

// NewTarget creates a new target
func NewTarget(targetType TargetType, targetID string, orgID string) *Target {
	return &Target{
		TargetType: targetType,
		TargetID:   targetID,
		OrgID:      orgID,
	}
}

// WithDeliverRole sets the deliver role for cluster-based delivery
func (t *Target) WithDeliverRole(role string) *Target {
	t.DeliverRole = role
	return t
}

// ToMailboxKey generates a unique key for this target's mailbox
func (t *Target) ToMailboxKey() string {
	return string(t.TargetType) + ":" + t.TargetID + ":" + t.OrgID
}
