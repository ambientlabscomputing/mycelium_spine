package grpc

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"time"

	"github.com/ambientlabscomputing/mycelium_spine/internal/service"
	"github.com/ambientlabscomputing/mycelium_spine/internal/utils"
	umsv1 "github.com/ambientlabscomputing/mycelium_spine/proto/ums/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
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

// NewServer creates a new gRPC server.
// An optional getCert function may be passed (e.g. bootstrap.CertManager.GetCertificate)
// to enable zero-downtime TLS cert hot-swap. When omitted, the cert is loaded
// from disk once at startup via settings.GRPC.TLS.CertPath / KeyPath.
func NewServer(appService service.Service, settings *utils.Settings, getCert ...func(*tls.ClientHelloInfo) (*tls.Certificate, error)) (*Server, error) {
	logger := utils.Logger.With("component", "grpc_server")

	// Load TLS credentials
	var tlsConfig *tls.Config
	if settings.GRPC.TLS.Enabled {
		var certGetter func(*tls.ClientHelloInfo) (*tls.Certificate, error)
		if len(getCert) > 0 {
			certGetter = getCert[0]
		}
		var err error
		tlsConfig, err = loadTLSConfig(settings, certGetter)
		if err != nil {
			return nil, fmt.Errorf("failed to load TLS config: %w", err)
		}
	}

	// Prepare CA pool for header auth if enabled
	var caPool *x509.CertPool
	if settings.Auth.HeaderAuth.Enabled {
		caPath := settings.GRPC.TLS.CAPath
		if caPath == "" {
			return nil, fmt.Errorf("header auth enabled but ca_path is not set")
		}

		caCertBytes, err := os.ReadFile(caPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA cert for header auth: %w", err)
		}
		caPool = x509.NewCertPool()
		if !caPool.AppendCertsFromPEM(caCertBytes) {
			return nil, fmt.Errorf("failed to parse CA cert for header auth")
		}
	}

	// Create gRPC server with TLS and interceptors
	var serverOpts []grpc.ServerOption
	if tlsConfig != nil {
		serverOpts = append(serverOpts, grpc.Creds(credentials.NewTLS(tlsConfig)))
	}
	serverOpts = append(serverOpts,
		grpc.ChainUnaryInterceptor(
			UnaryHeaderAuthInterceptor(caPool, settings, logger),
			unaryLoggingInterceptor(logger),
			unaryRecoveryInterceptor(logger),
		),
		grpc.ChainStreamInterceptor(
			StreamHeaderAuthInterceptor(caPool, settings, logger),
			streamLoggingInterceptor(logger),
			streamRecoveryInterceptor(logger),
		),
	)
	grpcServer := grpc.NewServer(append(serverOpts,
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             20 * time.Second,
			PermitWithoutStream: true,
		}),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			Time:    30 * time.Second,
			Timeout: 10 * time.Second,
		}),
	)...)

	// Create handlers
	streamHandler := NewStreamHandler(appService)
	publishHandler := NewPublishHandler(appService)

	// Register services
	umsv1.RegisterSpineStreamServer(grpcServer, streamHandler)
	umsv1.RegisterSpinePublishServer(grpcServer, publishHandler)

	// Enable reflection only in debug mode (grpcurl, development)
	// SECURITY: Reflection exposes the full gRPC API surface; disable in production
	if settings.LogLevel == "debug" {
		logger.Info("gRPC reflection enabled (debug mode)")
		reflection.Register(grpcServer)
	} else {
		logger.Info("gRPC reflection disabled (production mode)")
	}

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

// loadTLSConfig builds a tls.Config from settings.
// If getCert is non-nil it is set as GetCertificate, enabling hot-swap; otherwise
// the cert is loaded once from disk via CertPath/KeyPath.
func loadTLSConfig(settings *utils.Settings, getCert func(*tls.ClientHelloInfo) (*tls.Certificate, error)) (*tls.Config, error) {
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	if getCert != nil {
		// Dynamic cert provider — supports zero-downtime renewal.
		tlsConfig.GetCertificate = getCert
	} else {
		// Static: load cert from disk once at startup.
		cert, err := tls.LoadX509KeyPair(settings.GRPC.TLS.CertPath, settings.GRPC.TLS.KeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load server certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
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
		// Extract trace ID if this is a PublishRequest
		logAttrs := []any{"method", info.FullMethod}
		if pubReq, ok := req.(*umsv1.PublishRequest); ok && pubReq.Envelope != nil && pubReq.Envelope.TraceId != "" {
			logAttrs = append(logAttrs, "trace_id", pubReq.Envelope.TraceId)
		}
		logger.Debug("unary RPC call", logAttrs...)
		resp, err := handler(ctx, req)
		if err != nil {
			logger.Error("unary RPC error", append(logAttrs, "error", err)...)
		}
		return resp, err
	}
}

func streamLoggingInterceptor(logger *slog.Logger) grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		logger.Debug("stream RPC call", "method", info.FullMethod)
		err := handler(srv, ss)
		if err != nil {
			// context.Canceled and io.EOF are normal stream termination — not errors.
			if errors.Is(err, context.Canceled) || errors.Is(err, io.EOF) {
				logger.Debug("stream closed", "method", info.FullMethod)
			} else {
				logger.Error("stream RPC error", "method", info.FullMethod, "error", err)
			}
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
