package admin_server

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	admin "github.com/ambientlabscomputing/mycelium_spine/proto/admin"
)

type mailboxesServiceImpl struct {
	admin.UnimplementedAdminMailboxesServiceServer
	as *AdminServer
}

func (m *mailboxesServiceImpl) List(ctx context.Context, req *admin.ListMailboxesRequest) (*admin.ListMailboxesResponse, error) {
	limit := int(req.Limit)
	offset := int(req.Offset)

	mailboxes, total, err := m.as.repo.ListMailboxes(ctx, req.OrgId, req.TargetType, limit, offset)
	if err != nil {
		return nil, toGRPCError(err)
	}

	summaries := make([]*admin.MailboxSummary, 0, len(mailboxes))
	for _, mb := range mailboxes {
		summaries = append(summaries, &admin.MailboxSummary{
			MailboxId:  mb.MailboxID,
			TargetType: string(mb.TargetType),
			TargetId:   mb.TargetID,
			OrgId:      mb.OrgID,
			NextSeq:    mb.NextSeq,
			CreatedAt:  mb.CreatedAt,
			UpdatedAt:  mb.UpdatedAt,
		})
	}

	return &admin.ListMailboxesResponse{
		Mailboxes: summaries,
		Pagination: &admin.Pagination{
			Total:  total,
			Limit:  int32(limit),
			Offset: int32(offset),
		},
	}, nil
}

func (m *mailboxesServiceImpl) Get(ctx context.Context, req *admin.GetMailboxRequest) (*admin.MailboxDetail, error) {
	if req.MailboxId == "" {
		return nil, status.Error(codes.InvalidArgument, "mailbox_id is required")
	}

	mb, err := m.as.repo.GetMailbox(ctx, req.MailboxId)
	if err != nil {
		return nil, toGRPCError(err)
	}

	return &admin.MailboxDetail{
		MailboxId:        mb.MailboxID,
		TargetType:       string(mb.TargetType),
		TargetId:         mb.TargetID,
		OrgId:            mb.OrgID,
		NextSeq:          mb.NextSeq,
		CreatedAt:        mb.CreatedAt,
		UpdatedAt:        mb.UpdatedAt,
		RetentionSeconds: int32(mb.RetentionPolicy.RetentionSeconds),
		MaxEnvelopes:     int32(mb.RetentionPolicy.MaxEnvelopes),
		MaxBytes:         int32(mb.RetentionPolicy.MaxBytes),
	}, nil
}

func (m *mailboxesServiceImpl) ListEnvelopes(ctx context.Context, req *admin.ListEnvelopesRequest) (*admin.ListEnvelopesResponse, error) {
	if req.MailboxId == "" {
		return nil, status.Error(codes.InvalidArgument, "mailbox_id is required")
	}

	limit := int(req.Limit)
	if limit <= 0 {
		limit = 20
	}

	envelopes, err := m.as.repo.FetchEnvelopes(ctx, req.MailboxId, req.FromSeq, limit)
	if err != nil {
		return nil, toGRPCError(err)
	}

	summaries := make([]*admin.EnvelopeSummary, 0, len(envelopes))
	for _, e := range envelopes {
		summaries = append(summaries, &admin.EnvelopeSummary{
			MailboxId:   e.MailboxID,
			Seq:         e.Seq,
			Type:        e.Type,
			Qos:         string(e.QoS),
			OrgId:       e.OrgID,
			RequiresAck: e.RequiresAck,
			ExpiresAtMs: e.ExpiresAtMs,
		})
	}

	return &admin.ListEnvelopesResponse{Envelopes: summaries}, nil
}

func (m *mailboxesServiceImpl) PurgeExpired(ctx context.Context, req *admin.PurgeExpiredRequest) (*admin.PurgeExpiredResponse, error) {
	if req.MailboxId == "" {
		return nil, status.Error(codes.InvalidArgument, "mailbox_id is required")
	}

	count, err := m.as.repo.DeleteExpired(ctx, req.MailboxId)
	if err != nil {
		return nil, toGRPCError(err)
	}

	return &admin.PurgeExpiredResponse{DeletedCount: count}, nil
}

func (m *mailboxesServiceImpl) ClearOutbox(ctx context.Context, req *admin.ClearOutboxRequest) (*admin.ClearOutboxResponse, error) {
	if req.MailboxId == "" {
		return nil, status.Error(codes.InvalidArgument, "mailbox_id is required")
	}

	count, err := m.as.repo.ClearMailbox(ctx, req.MailboxId)
	if err != nil {
		return nil, toGRPCError(err)
	}

	return &admin.ClearOutboxResponse{DeletedCount: count}, nil
}
