package admin_server

import (
	"context"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	admin "github.com/ambientlabscomputing/mycelium_spine/proto/admin"
)

type sessionsServiceImpl struct {
	admin.UnimplementedAdminSessionsServiceServer
	as *AdminServer
}

func (s *sessionsServiceImpl) List(ctx context.Context, req *admin.ListSessionsRequest) (*admin.ListSessionsResponse, error) {
	sessions, err := s.as.svc.GetSessionService().ListActiveSessions(ctx)
	if err != nil {
		return nil, toGRPCError(err)
	}

	var summaries []*admin.SessionSummary
	for _, sess := range sessions {
		if req.OrgId != "" && sess.OrgID != req.OrgId {
			continue
		}
		summaries = append(summaries, &admin.SessionSummary{
			ServerId:          sess.ServerID,
			SessionId:         sess.SessionID,
			OrgId:             sess.OrgID,
			SessionEpoch:      sess.SessionEpoch,
			SubscriptionCount: int32(len(sess.Subscriptions)),
			ConnectedAt:       sess.ConnectedAt,
			LastHeartbeat:     sess.LastHeartbeat,
		})
	}

	limit := req.Limit
	offset := req.Offset
	total := int64(len(summaries))

	// Apply pagination
	if offset > 0 && int(offset) < len(summaries) {
		summaries = summaries[offset:]
	} else if offset > 0 {
		summaries = nil
	}
	if limit > 0 && int(limit) < len(summaries) {
		summaries = summaries[:limit]
	}

	return &admin.ListSessionsResponse{
		Sessions: summaries,
		Pagination: &admin.Pagination{
			Total:  total,
			Limit:  limit,
			Offset: offset,
		},
	}, nil
}

func (s *sessionsServiceImpl) Get(ctx context.Context, req *admin.GetSessionRequest) (*admin.SessionDetail, error) {
	sessions, err := s.as.svc.GetSessionService().ListActiveSessions(ctx)
	if err != nil {
		return nil, toGRPCError(err)
	}

	for _, sess := range sessions {
		if (req.SessionId != "" && sess.SessionID == req.SessionId) ||
			(req.ServerId != "" && sess.ServerID == req.ServerId) {
			features := make(map[string]bool, len(sess.ClientFeatures))
			for k, v := range sess.ClientFeatures {
				features[k] = v
			}
			return &admin.SessionDetail{
				ServerId:          sess.ServerID,
				SessionId:         sess.SessionID,
				OrgId:             sess.OrgID,
				SessionEpoch:      sess.SessionEpoch,
				Subscriptions:     sess.Subscriptions,
				ConnectedAt:       sess.ConnectedAt,
				LastHeartbeat:     sess.LastHeartbeat,
				DeviceFingerprint: sess.DeviceFingerprint,
				ClientFeatures:    features,
			}, nil
		}
	}

	return nil, status.Error(codes.NotFound, fmt.Sprintf("session not found"))
}

func (s *sessionsServiceImpl) Disconnect(ctx context.Context, req *admin.DisconnectSessionRequest) (*admin.Empty, error) {
	if req.SessionId == "" && req.ServerId == "" {
		return nil, status.Error(codes.InvalidArgument, "session_id or server_id must be provided")
	}

	sessions, err := s.as.svc.GetSessionService().ListActiveSessions(ctx)
	if err != nil {
		return nil, toGRPCError(err)
	}

	for _, sess := range sessions {
		if (req.SessionId != "" && sess.SessionID == req.SessionId) ||
			(req.ServerId != "" && sess.ServerID == req.ServerId) {
			if err := s.as.svc.GetSessionService().UnregisterSession(ctx, sess.SessionID); err != nil {
				return nil, toGRPCError(err)
			}
			if err := s.as.svc.GetDeliveryService().StopDeliveryLoop(ctx, sess.SessionID); err != nil {
				// non-fatal — loop may already be gone
				s.as.logger.Warn("failed to stop delivery loop", "session_id", sess.SessionID, "error", err)
			}
			return &admin.Empty{}, nil
		}
	}

	return nil, status.Error(codes.NotFound, "session not found")
}

func (s *sessionsServiceImpl) EvictStale(ctx context.Context, _ *admin.Empty) (*admin.EvictStaleResponse, error) {
	if err := s.as.svc.GetSessionService().EvictStale(ctx); err != nil {
		return nil, toGRPCError(err)
	}
	// EvictStale doesn't return a count — return 0 as a best-effort response.
	return &admin.EvictStaleResponse{EvictedCount: 0}, nil
}
