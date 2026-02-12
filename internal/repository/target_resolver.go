package repository

import (
	"context"
	"fmt"

	"github.com/ambientlabscomputing/mycelium_spine/internal/types"
	"go.mongodb.org/mongo-driver/bson"
)

// MongoTargetResolver implements TargetResolver using MongoDB
type MongoTargetResolver struct {
	mailboxRepo *MongoMailboxRepository
}

// ResolveTargets maps targets to mailbox IDs
// For SERVER/ORG targets, it returns the single mailbox
// For CLUSTER targets, it resolves to all server mailboxes in that cluster plus cluster mailbox
func (r *MongoTargetResolver) ResolveTargets(ctx context.Context, targets []*types.Target) ([]string, error) {
	mailboxIDs := make([]string, 0)
	seen := make(map[string]bool)

	for _, target := range targets {
		var resolvedIDs []string
		var err error

		switch target.TargetType {
		case types.TargetTypeServer, types.TargetTypeOrg, types.TargetTypeService:
			// Single mailbox per target
			mailbox, err := r.mailboxRepo.GetOrCreateMailbox(ctx, target.TargetType, target.TargetID, target.OrgID)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve target %s:%s: %w", target.TargetType, target.TargetID, err)
			}
			resolvedIDs = append(resolvedIDs, mailbox.MailboxID)

		case types.TargetTypeCluster:
			// Cluster fanout: resolve to cluster mailbox + all server mailboxes in cluster
			resolvedIDs, err = r.resolveClusterTarget(ctx, target)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve cluster target %s: %w", target.TargetID, err)
			}

		case types.TargetTypeBroadcast:
			// Broadcast: resolve to all mailboxes in org (restricted, admin-only)
			resolvedIDs, err = r.resolveBroadcastTarget(ctx, target)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve broadcast target: %w", err)
			}

		default:
			return nil, fmt.Errorf("unsupported target type: %s", target.TargetType)
		}

		// Deduplicate mailbox IDs
		for _, id := range resolvedIDs {
			if !seen[id] {
				mailboxIDs = append(mailboxIDs, id)
				seen[id] = true
			}
		}
	}

	return mailboxIDs, nil
}

// resolveClusterTarget resolves a cluster target to mailbox IDs
// For V1, this creates the cluster-level mailbox
// Future: query cluster membership to get all server mailboxes
func (r *MongoTargetResolver) resolveClusterTarget(ctx context.Context, target *types.Target) ([]string, error) {
	// Get/create cluster-level mailbox
	mailbox, err := r.mailboxRepo.GetOrCreateMailbox(ctx, types.TargetTypeCluster, target.TargetID, target.OrgID)
	if err != nil {
		return nil, err
	}

	mailboxIDs := []string{mailbox.MailboxID}

	// TODO: If DeliverRole == "LEADER", resolve to leader's SERVER mailbox only
	// TODO: Otherwise, query cluster membership and append all server mailboxes

	return mailboxIDs, nil
}

// resolveBroadcastTarget resolves a broadcast target to all mailboxes in an org
// This is a restricted operation (admin-only)
func (r *MongoTargetResolver) resolveBroadcastTarget(ctx context.Context, target *types.Target) ([]string, error) {
	// Query all mailboxes in the org
	cursor, err := r.mailboxRepo.mailboxes.Find(ctx, bson.M{"org_id": target.OrgID})
	if err != nil {
		return nil, fmt.Errorf("failed to query mailboxes for broadcast: %w", err)
	}
	defer cursor.Close(ctx)

	var mailboxIDs []string
	for cursor.Next(ctx) {
		var mailbox types.Mailbox
		if err := cursor.Decode(&mailbox); err != nil {
			continue
		}
		mailboxIDs = append(mailboxIDs, mailbox.MailboxID)
	}

	return mailboxIDs, nil
}
