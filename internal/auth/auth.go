package auth

import (
	"context"
	"fmt"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// ClientIdentity represents an authenticated client identity extracted from mTLS certificate
type ClientIdentity struct {
	ClientID   string // From certificate CN
	Service    string // From certificate OU
	Subject    string // Full distinguished name
	RemoteAddr string // Peer address
}

// contextKey is a type for context keys
type contextKey string

const clientIdentityKey contextKey = "client_identity"

// ContextWithIdentity adds client identity to context
func ContextWithIdentity(ctx context.Context, identity *ClientIdentity) context.Context {
	return context.WithValue(ctx, clientIdentityKey, identity)
}

// IdentityFromContext retrieves client identity from context
func IdentityFromContext(ctx context.Context) *ClientIdentity {
	identity, ok := ctx.Value(clientIdentityKey).(*ClientIdentity)
	if !ok {
		return nil
	}
	return identity
}

// ExtractClientIdentityFromPeer extracts mTLS identity from gRPC peer
func ExtractClientIdentityFromPeer(p *peer.Peer) (*ClientIdentity, error) {
	if p == nil {
		return nil, fmt.Errorf("peer is nil")
	}

	// Extract TLS credentials
	tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok {
		return nil, fmt.Errorf("peer not authenticated with TLS")
	}

	certs := tlsInfo.State.PeerCertificates
	if len(certs) == 0 {
		return nil, fmt.Errorf("no client certificate presented")
	}

	cert := certs[0]
	if cert == nil {
		return nil, fmt.Errorf("invalid client certificate")
	}

	identity := &ClientIdentity{
		ClientID:   cert.Subject.CommonName,
		Subject:    cert.Subject.String(),
		RemoteAddr: p.Addr.String(),
	}

	if len(cert.Subject.OrganizationalUnit) > 0 {
		identity.Service = cert.Subject.OrganizationalUnit[0]
	}

	if identity.ClientID == "" {
		return nil, fmt.Errorf("client certificate missing CommonName")
	}

	return identity, nil
}

// ExtractClientIdentity extracts client identity from context or peer
func ExtractClientIdentity(ctx context.Context, strictAuth bool) (*ClientIdentity, error) {
	// Check context first
	if identity := IdentityFromContext(ctx); identity != nil {
		return identity, nil
	}

	// Extract from peer
	p, ok := peer.FromContext(ctx)
	if !ok {
		if strictAuth {
			return nil, status.Error(codes.Unauthenticated, "no peer information; mTLS may not be configured")
		}
		return nil, nil
	}

	identity, err := ExtractClientIdentityFromPeer(p)
	if err != nil {
		if strictAuth {
			return nil, status.Error(codes.Unauthenticated, err.Error())
		}
		return nil, nil
	}

	return identity, nil
}

// ValidateClientIDMatches ensures requested ID matches client certificate CN
func ValidateClientIDMatches(identity *ClientIdentity, requestedID string) error {
	if identity == nil {
		return status.Error(codes.Unauthenticated, "missing client identity")
	}

	if identity.ClientID != requestedID {
		return status.Errorf(codes.PermissionDenied, "client mTLS CN %q does not match requested ID %q", identity.ClientID, requestedID)
	}

	return nil
}

// LogClientIdentity logs client identity information
func LogClientIdentity(logger *slog.Logger, identity *ClientIdentity) {
	if identity == nil {
		logger.Warn("no client identity available")
		return
	}
	logger.Debug("client authenticated", "client_id", identity.ClientID, "service", identity.Service, "remote_addr", identity.RemoteAddr)
}
