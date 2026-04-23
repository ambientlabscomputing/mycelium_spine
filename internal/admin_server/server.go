// Package admin_server implements the gRPC admin socket server for mycelium_spine.
package admin_server

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"time"

	"google.golang.org/grpc"

	"github.com/ambientlabscomputing/mycelium_spine/internal/repository"
	"github.com/ambientlabscomputing/mycelium_spine/internal/service"
	"github.com/ambientlabscomputing/mycelium_spine/internal/utils"
	admin "github.com/ambientlabscomputing/mycelium_spine/proto/admin"
)

// AdminServer serves the admin gRPC socket.
type AdminServer struct {
	svc        *service.AppService
	repo       repository.Repository
	grpcServer *grpc.Server
	listener   net.Listener
	socketPath string
	logger     *slog.Logger
	startedAt  time.Time
	version    string
}

// NewAdminServer creates an AdminServer.
func NewAdminServer(svc *service.AppService, repo repository.Repository, socketPath string, version string) *AdminServer {
	return &AdminServer{
		svc:        svc,
		repo:       repo,
		socketPath: socketPath,
		version:    version,
		logger:     utils.Logger.With("component", "admin_server"),
	}
}

// Start starts the admin gRPC socket server.
func (as *AdminServer) Start(ctx context.Context) error {
	as.startedAt = time.Now()

	dir := filepath.Dir(as.socketPath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("admin_server: create socket dir: %w", err)
		}
	}
	// Remove stale socket file if present.
	if err := os.RemoveAll(as.socketPath); err != nil {
		return fmt.Errorf("admin_server: remove stale socket: %w", err)
	}

	ln, err := net.Listen("unix", as.socketPath)
	if err != nil {
		return fmt.Errorf("admin_server: listen %s: %w", as.socketPath, err)
	}
	if err := os.Chmod(as.socketPath, 0600); err != nil {
		ln.Close()
		return fmt.Errorf("admin_server: chmod socket: %w", err)
	}
	as.listener = ln

	as.grpcServer = grpc.NewServer()
	as.registerServices()

	go func() {
		as.logger.Info("admin gRPC socket listening", "socket", as.socketPath)
		if err := as.grpcServer.Serve(ln); err != nil {
			as.logger.Error("admin gRPC server error", "error", err)
		}
	}()

	return nil
}

// Stop gracefully shuts down the admin server.
func (as *AdminServer) Stop(ctx context.Context) error {
	if as.grpcServer == nil {
		return nil
	}
	as.logger.Info("stopping admin socket server")
	done := make(chan struct{})
	go func() {
		as.grpcServer.GracefulStop()
		close(done)
	}()
	if as.listener != nil {
		as.listener.Close()
	}
	_ = os.RemoveAll(as.socketPath)
	select {
	case <-ctx.Done():
		as.grpcServer.Stop()
		return ctx.Err()
	case <-done:
		return nil
	}
}

func (as *AdminServer) registerServices() {
	admin.RegisterAdminHealthServiceServer(as.grpcServer, &healthServiceImpl{as: as})
	admin.RegisterAdminSessionsServiceServer(as.grpcServer, &sessionsServiceImpl{as: as})
	admin.RegisterAdminMailboxesServiceServer(as.grpcServer, &mailboxesServiceImpl{as: as})
	admin.RegisterAdminCursorsServiceServer(as.grpcServer, &cursorsServiceImpl{as: as})
	admin.RegisterAdminWorkersServiceServer(as.grpcServer, &workersServiceImpl{as: as})
}
