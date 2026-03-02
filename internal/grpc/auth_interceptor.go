package grpc

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"log/slog"

	"github.com/ambientlabscomputing/mycelium_spine/internal/auth"
	"github.com/ambientlabscomputing/mycelium_spine/internal/utils"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// Header auth metadata keys (must be lowercase for gRPC)
const (
	headerClientCert = "x-client-certificate"
	headerClientSig  = "x-client-signature"
	headerTimestamp  = "x-request-timestamp"
)

// UnaryHeaderAuthInterceptor creates an interceptor for unary RPCs that validates mTLS-over-headers
func UnaryHeaderAuthInterceptor(caPool *x509.CertPool, settings *utils.Settings, logger *slog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if !settings.Auth.HeaderAuth.Enabled {
			return handler(ctx, req)
		}

		md, ok := metadata.FromIncomingContext(ctx)
		if !ok || len(md[headerClientCert]) == 0 {
			if settings.Auth.RequireMTLS {
				logger.Warn("unary auth: missing metadata or client cert")
				return nil, status.Error(codes.Unauthenticated, "client certificate required via metadata")
			}
			return handler(ctx, req)
		}

		identity, err := validateHeaderAuth(md, caPool, settings, info.FullMethod, req)
		if err != nil {
			logger.Warn("unary auth failed", "error", err, "method", info.FullMethod)
			if settings.Auth.RequireMTLS {
				return nil, status.Error(codes.Unauthenticated, err.Error())
			}
			return handler(ctx, req)
		}

		// Inject identity into context
		ctx = auth.ContextWithIdentity(ctx, identity)
		return handler(ctx, req)
	}
}

// StreamHeaderAuthInterceptor creates an interceptor for stream RPCs that validates mTLS-over-headers
func StreamHeaderAuthInterceptor(caPool *x509.CertPool, settings *utils.Settings, logger *slog.Logger) grpc.StreamServerInterceptor {
	return func(srv interface{}, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if !settings.Auth.HeaderAuth.Enabled {
			return handler(srv, stream)
		}

		ctx := stream.Context()
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok || len(md[headerClientCert]) == 0 {
			if settings.Auth.RequireMTLS {
				logger.Warn("stream auth: missing metadata or client cert")
				return status.Error(codes.Unauthenticated, "client certificate required via metadata")
			}
			return handler(srv, stream)
		}

		// For streams, we don't have a specific request body at stream open to sign,
		// so we sign just the method name and timestamp.
		identity, err := validateHeaderAuth(md, caPool, settings, info.FullMethod, nil)
		if err != nil {
			logger.Warn("stream auth failed", "error", err, "method", info.FullMethod)
			if settings.Auth.RequireMTLS {
				return status.Error(codes.Unauthenticated, err.Error())
			}
			return handler(srv, stream)
		}

		// Wrap the stream to override its Context
		wrappedStream := &wrappedServerStream{
			ServerStream: stream,
			ctx:          auth.ContextWithIdentity(ctx, identity),
		}

		return handler(srv, wrappedStream)
	}
}

// validateHeaderAuth performs the actual validation of header auth metadata
func validateHeaderAuth(md metadata.MD, caPool *x509.CertPool, settings *utils.Settings, fullMethod string, req interface{}) (*auth.ClientIdentity, error) {
	// Extract and validate timestamp
	timestamps := md[headerTimestamp]
	if len(timestamps) == 0 {
		return nil, fmt.Errorf("missing request timestamp")
	}
	timestampStr := timestamps[0]

	_, err := auth.ValidateTimestamp(timestampStr, settings.Auth.HeaderAuth.TimestampSkewSeconds)
	if err != nil {
		return nil, fmt.Errorf("invalid timestamp: %w", err)
	}

	// Extract and decode certificate
	certBase64 := md[headerClientCert][0]
	certPEM, err := base64.StdEncoding.DecodeString(certBase64)
	if err != nil {
		return nil, fmt.Errorf("invalid certificate encoding")
	}

	// Extract and decode signature
	sigs := md[headerClientSig]
	if len(sigs) == 0 {
		return nil, fmt.Errorf("missing request signature")
	}
	sigDER, err := base64.StdEncoding.DecodeString(sigs[0])
	if err != nil {
		return nil, fmt.Errorf("invalid signature encoding")
	}

	// Parse and verify certificate against CA
	cert, err := auth.ParseAndVerifyCertificate(certPEM, caPool)
	if err != nil {
		return nil, fmt.Errorf("invalid certificate: %w", err)
	}

	// Serialize request body for unary requests (if available)
	var bodyBytes []byte
	if req != nil {
		message, ok := req.(proto.Message)
		if !ok {
			return nil, fmt.Errorf("request is not a proto.Message")
		}
		bodyBytes, err = proto.Marshal(message)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request: %w", err)
		}
	}

	// Verify signature
	payload := auth.BuildGRPCSignaturePayload(fullMethod, timestampStr, bodyBytes)
	if err := auth.VerifyRequestSignature(cert, payload, sigDER); err != nil {
		return nil, fmt.Errorf("invalid signature: %w", err)
	}

	// Create identity from cert
	identity := &auth.ClientIdentity{
		ClientID: cert.Subject.CommonName,
		Subject:  cert.Subject.String(),
	}

	if len(cert.Subject.OrganizationalUnit) > 0 {
		identity.Service = cert.Subject.OrganizationalUnit[0]
	}

	if identity.ClientID == "" {
		return nil, fmt.Errorf("client certificate missing CommonName")
	}

	// Note: We leave RemoteAddr empty or populated from a different source later since headers
	// don't reliably provide the real peer addr directly without looking at trusting proxies.
	// We can trust the x-forwarded-for header if needed, but not strictly required for auth auth yet.

	return identity, nil
}

// wrappedServerStream wraps a grpc.ServerStream to override its context
type wrappedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

// Context returns the overridden context
func (w *wrappedServerStream) Context() context.Context {
	return w.ctx
}
