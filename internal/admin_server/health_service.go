package admin_server

import (
	"context"
	"time"

	admin "github.com/ambientlabscomputing/mycelium_spine/proto/admin"
)

type healthServiceImpl struct {
	admin.UnimplementedAdminHealthServiceServer
	as *AdminServer
}

func (h *healthServiceImpl) Check(ctx context.Context, _ *admin.Empty) (*admin.HealthCheckResponse, error) {
	sessions, err := h.as.svc.GetSessionService().ListActiveSessions(ctx)
	if err != nil {
		return nil, toGRPCError(err)
	}

	mailboxCount, err := h.as.repo.CountMailboxes(ctx)
	if err != nil {
		return nil, toGRPCError(err)
	}

	uptime := time.Since(h.as.startedAt).Round(time.Second).String()
	return &admin.HealthCheckResponse{
		Status:         "ok",
		ActiveSessions: int32(len(sessions)),
		TotalMailboxes: mailboxCount,
		Uptime:         uptime,
		Version:        h.as.version,
	}, nil
}

func (h *healthServiceImpl) Detail(ctx context.Context, _ *admin.Empty) (*admin.HealthDetailResponse, error) {
	sessions, err := h.as.svc.GetSessionService().ListActiveSessions(ctx)
	if err != nil {
		return nil, toGRPCError(err)
	}

	mailboxCount, err := h.as.repo.CountMailboxes(ctx)
	if err != nil {
		return nil, toGRPCError(err)
	}

	uptime := time.Since(h.as.startedAt).Round(time.Second).String()

	// Worker pool stats
	metrics := h.as.svc.GetPoolManager().GetMetrics()
	workerPools := make(map[string]*admin.WorkerPoolStats, len(metrics))
	for name, m := range metrics {
		workerPools[name] = &admin.WorkerPoolStats{
			ActiveWorkers:  int32(m.ActiveWorkers),
			QueueDepth:     int32(m.QueueDepth),
			QueueCapacity:  int32(m.QueueCapacity),
			TasksProcessed: m.TasksProcessed,
			TaskErrors:     m.TaskErrors,
		}
	}

	return &admin.HealthDetailResponse{
		Status:         "ok",
		ActiveSessions: int32(len(sessions)),
		TotalMailboxes: mailboxCount,
		Uptime:         uptime,
		Version:        h.as.version,
		WorkerPools:    workerPools,
	}, nil
}
