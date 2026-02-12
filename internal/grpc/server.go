package grpc

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"net"
	"os"

	"github.com/ambientlabscomputing/mycelium_spine/internal/service"
	"github.com/ambientlabscomputing/mycelium_spine/internal/utils"
	umsv1 "github.com/ambientlabscomputing/mycelium_spine/proto/ums/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/reflection"
)

// Server wraps the gRPC server and handlers
type Server struct {
	appService     service.Service
	settings       *utils.Settings
	grpcServer     *grpc.Server
	streamHandler  *StreamHandler
	publishHandler *PublishHandler
	logger         *slog.Logger
}

// NewServer creates a new gRPC server
func NewServer(appService service.Service, settings *utils.Settings) (*Server, error) {
	logger := utils.Logger.With("component", "grpc_server")

	// Load TLS credentials
	tlsConfig, err := loadTLSConfig(settings)
	if err != nil {
		return nil, fmt.Errorf("failed to load TLS config: %w", err)
	}

	// Create gRPC server with TLS and interceptors
	grpcServer := grpc.NewServer(
		grpc.Creds(credentials.NewTLS(tlsConfig)),
		grpc.ChainUnaryInterceptor(
			unaryLoggingInterceptor(logger),
			unaryRecoveryInterceptor(logger),
		),
		grpc.ChainStreamInterceptor(
			streamLoggingInterceptor(logger),
			streamRecoveryInterceptor(logger),
		),
	)

	// Create handlers
	streamHandler := NewStreamHandler(appService)
	publishHandler := NewPublishHandler(appService)

	// Register services
	umsv1.RegisterSpineStreamServer(grpcServer, streamHandler)
	umsv1.RegisterSpinePublishServer(grpcServer, publishHandler)

	// Enable reflection for debugging (grpcurl, etc.)
	reflection.Register(grpcServer)

	return &Server{
		appService:     appService,
		settings:       settings,
		grpcServer:     grpcServer,
		streamHandler:  streamHandler,
		publishHandler: publishHandler,
		logger:         logger,
	}, nil
}

// Serve starts the gRPC server
func (s *Server) Serve(ctx context.Context) error {
	addr := fmt.Sprintf(":%d", s.settings.GRPC.Port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	s.logger.Info("gRPC server starting", "address", addr)

	// Start serving in a goroutine
	errChan := make(chan error, 1)
	go func() {
		if err := s.grpcServer.Serve(listener); err != nil {
			errChan <- err
		}
	}()

	// Wait for context cancellation or error
	select {
	case <-ctx.Done():
		s.logger.Info("shutting down gRPC server")
		s.grpcServer.GracefulStop()
		return nil
	case err := <-errChan:
		return err
	}
}

// loadTLSConfig loads TLS configuration from settings
func loadTLSConfig(settings *utils.Settings) (*tls.Config, error) {
	// Load server certificate and key
	cert, err := tls.LoadX509KeyPair(settings.GRPC.TLS.CertPath, settings.GRPC.TLS.KeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load server certificate: %w", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	// Configure client authentication (mTLS)
	switch settings.GRPC.TLS.ClientAuth {
	case "require_and_verify":
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
		// Load CA certificate for client verification
		caCert, err := os.ReadFile(settings.GRPC.TLS.CAPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA certificate: %w", err)
		}
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}
		tlsConfig.ClientCAs = caCertPool

	case "require":
		tlsConfig.ClientAuth = tls.RequireAnyClientCert

	case "request":
		tlsConfig.ClientAuth = tls.RequestClientCert

	case "none":
		tlsConfig.ClientAuth = tls.NoClientCert

	default:
		return nil, fmt.Errorf("unknown client_auth mode: %s", settings.GRPC.TLS.ClientAuth)
	}

	return tlsConfig, nil
}

// Interceptors

func unaryLoggingInterceptor(logger *slog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		logger.Debug("unary RPC call", "method", info.FullMethod)
		resp, err := handler(ctx, req)
		if err != nil {
			logger.Error("unary RPC error", "method", info.FullMethod, "error", err)
		}
		return resp, err
	}
}

func streamLoggingInterceptor(logger *slog.Logger) grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		logger.Debug("stream RPC call", "method", info.FullMethod)
		err := handler(srv, ss)
		if err != nil {
			logger.Error("stream RPC error", "method", info.FullMethod, "error", err)
		}
		return err
	}
}

func unaryRecoveryInterceptor(logger *slog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("panic in unary RPC", "method", info.FullMethod, "panic", r)
				err = fmt.Errorf("internal server error")
			}
		}()
		return handler(ctx, req)
	}
}

func streamRecoveryInterceptor(logger *slog.Logger) grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) (err error) {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("panic in stream RPC", "method", info.FullMethod, "panic", r)
				err = fmt.Errorf("internal server error")
			}
		}()
		return handler(srv, ss)
	}
}
