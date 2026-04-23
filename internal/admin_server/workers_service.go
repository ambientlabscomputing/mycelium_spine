package admin_server

import (
	"context"

	admin "github.com/ambientlabscomputing/mycelium_spine/proto/admin"
)

type workersServiceImpl struct {
	admin.UnimplementedAdminWorkersServiceServer
	as *AdminServer
}

func (w *workersServiceImpl) GetStats(ctx context.Context, _ *admin.Empty) (*admin.WorkerStatsResponse, error) {
	metrics := w.as.svc.GetPoolManager().GetMetrics()
	pools := make(map[string]*admin.PoolStats, len(metrics))
	for name, m := range metrics {
		pools[name] = &admin.PoolStats{
			ActiveWorkers:  int32(m.ActiveWorkers),
			QueueDepth:     int32(m.QueueDepth),
			QueueCapacity:  int32(m.QueueCapacity),
			TasksProcessed: m.TasksProcessed,
			TaskErrors:     m.TaskErrors,
		}
	}
	return &admin.WorkerStatsResponse{Pools: pools}, nil
}
