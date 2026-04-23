package admin_server

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	admin "github.com/ambientlabscomputing/mycelium_spine/proto/admin"
)

type cursorsServiceImpl struct {
	admin.UnimplementedAdminCursorsServiceServer
	as *AdminServer
}

func (c *cursorsServiceImpl) List(ctx context.Context, req *admin.ListCursorsRequest) (*admin.ListCursorsResponse, error) {
	limit := int(req.Limit)
	offset := int(req.Offset)

	cursors, total, err := c.as.repo.ListCursors(ctx, req.ServerId, req.MailboxId, limit, offset)
	if err != nil {
		return nil, toGRPCError(err)
	}

	summaries := make([]*admin.CursorSummary, 0, len(cursors))
	for _, cur := range cursors {
		summaries = append(summaries, &admin.CursorSummary{
			ServerId:     cur.ServerID,
			MailboxId:    cur.MailboxID,
			LastAckedSeq: cur.LastAckedSeq,
			UpdatedAt:    cur.UpdatedAt,
		})
	}

	return &admin.ListCursorsResponse{
		Cursors: summaries,
		Pagination: &admin.Pagination{
			Total:  total,
			Limit:  int32(limit),
			Offset: int32(offset),
		},
	}, nil
}

func (c *cursorsServiceImpl) Get(ctx context.Context, req *admin.GetCursorRequest) (*admin.CursorDetail, error) {
	if req.ServerId == "" || req.MailboxId == "" {
		return nil, status.Error(codes.InvalidArgument, "server_id and mailbox_id are required")
	}

	cursors, _, err := c.as.repo.ListCursors(ctx, req.ServerId, req.MailboxId, 1, 0)
	if err != nil {
		return nil, toGRPCError(err)
	}
	if len(cursors) == 0 {
		return nil, status.Error(codes.NotFound, "cursor not found")
	}
	cur := cursors[0]

	// Fetch mailbox to compute inflight
	var nextSeq uint64
	mb, err := c.as.repo.GetMailbox(ctx, req.MailboxId)
	if err == nil {
		nextSeq = mb.NextSeq
	}

	var inflight uint64
	if nextSeq > cur.LastAckedSeq {
		inflight = nextSeq - cur.LastAckedSeq - 1
	}

	return &admin.CursorDetail{
		ServerId:       cur.ServerID,
		MailboxId:      cur.MailboxID,
		LastAckedSeq:   cur.LastAckedSeq,
		MailboxNextSeq: nextSeq,
		InflightCount:  inflight,
		UpdatedAt:      cur.UpdatedAt,
	}, nil
}

func (c *cursorsServiceImpl) Reset(ctx context.Context, req *admin.ResetCursorRequest) (*admin.Empty, error) {
	if req.ServerId == "" || req.MailboxId == "" {
		return nil, status.Error(codes.InvalidArgument, "server_id and mailbox_id are required")
	}

	if err := c.as.repo.ResetCursor(ctx, req.ServerId, req.MailboxId, req.Seq); err != nil {
		return nil, toGRPCError(err)
	}

	return &admin.Empty{}, nil
}

func (c *cursorsServiceImpl) DeleteServerCursors(ctx context.Context, req *admin.DeleteServerCursorsRequest) (*admin.DeleteServerCursorsResponse, error) {
	if req.ServerId == "" {
		return nil, status.Error(codes.InvalidArgument, "server_id is required")
	}

	// Count before delete for the response
	cursors, total, err := c.as.repo.ListCursors(ctx, req.ServerId, "", 0, 0)
	_ = cursors
	if err != nil {
		return nil, toGRPCError(err)
	}

	if err := c.as.repo.DeleteServerCursors(ctx, req.ServerId); err != nil {
		return nil, toGRPCError(err)
	}

	return &admin.DeleteServerCursorsResponse{DeletedCount: total}, nil
}
