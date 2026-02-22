package metrics

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Server exposes Prometheus metrics via HTTP
type Server struct {
	server *http.Server
	port   int
	logger *slog.Logger
}

// NewMetricsServer creates a new metrics HTTP server
func NewMetricsServer(port int, logger *slog.Logger) *Server {
	// Setup HTTP handler for metrics
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	addr := fmt.Sprintf(":%d", port)

	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	return &Server{
		server: server,
		port:   port,
		logger: logger,
	}
}

// Start starts the metrics server
func (s *Server) Start() error {
	s.logger.Info("metrics server starting", "port", s.port, "address", s.server.Addr)

	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("metrics server error", "error", err)
		}
	}()

	return nil
}

// Stop stops the metrics server gracefully
func (s *Server) Stop(ctx context.Context) error {
	if s.server != nil {
		return s.server.Shutdown(ctx)
	}
	return nil
}
