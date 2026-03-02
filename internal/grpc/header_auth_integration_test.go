package grpc

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"log/slog"
	"math/big"
	"net"
	"os"
	"testing"
	"time"

	"github.com/ambientlabscomputing/mycelium_spine/internal/auth"
	"github.com/ambientlabscomputing/mycelium_spine/internal/service"
	"github.com/ambientlabscomputing/mycelium_spine/internal/utils"
	umsv1 "github.com/ambientlabscomputing/mycelium_spine/proto/ums/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
)

// ---------- Test CA / certificate helpers ----------

// testPKI holds a CA and a client cert + key generated in‑memory for testing.
type testPKI struct {
	caPool     *x509.CertPool
	caCert     *x509.Certificate
	caKey      *ecdsa.PrivateKey
	clientCert *x509.Certificate
	clientKey  *ecdsa.PrivateKey
	clientPEM  []byte // PEM‑encoded client certificate
}

// newTestPKI creates a fresh CA and client certificate in‑memory.
func newTestPKI(t *testing.T) *testPKI {
	t.Helper()

	// --- 1. CA key‑pair ---
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName:   "Test CA",
			Organization: []string{"Ambient Labs Testing"},
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	caCertDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	require.NoError(t, err)
	caCert, err := x509.ParseCertificate(caCertDER)
	require.NoError(t, err)

	caPool := x509.NewCertPool()
	caPool.AddCert(caCert)

	// --- 2. Client key‑pair ---
	clientKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	clientTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject: pkix.Name{
			CommonName:         "test-publisher",
			Organization:       []string{"Ambient Labs"},
			OrganizationalUnit: []string{"server_api"},
		},
		NotBefore:   time.Now().Add(-1 * time.Hour),
		NotAfter:    time.Now().Add(24 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}

	clientCertDER, err := x509.CreateCertificate(rand.Reader, clientTemplate, caCert, &clientKey.PublicKey, caKey)
	require.NoError(t, err)
	clientCert, err := x509.ParseCertificate(clientCertDER)
	require.NoError(t, err)

	clientPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: clientCertDER})

	return &testPKI{
		caPool:     caPool,
		caCert:     caCert,
		caKey:      caKey,
		clientCert: clientCert,
		clientKey:  clientKey,
		clientPEM:  clientPEM,
	}
}

// ---------- Signature helpers (mirrors SDK logic) ----------

type ecdsaSig struct {
	R, S *big.Int
}

func signPayloadForTest(key *ecdsa.PrivateKey, payload []byte) ([]byte, error) {
	hash := sha256.Sum256(payload)
	r, s, err := ecdsa.Sign(rand.Reader, key, hash[:])
	if err != nil {
		return nil, err
	}
	return asn1.Marshal(ecdsaSig{R: r, S: s})
}

func buildGRPCPayload(method, timestamp string, body []byte) []byte {
	p := fmt.Sprintf("GRPC\n%s\n%s\n", method, timestamp)
	if len(body) > 0 {
		return append([]byte(p), body...)
	}
	return []byte(p)
}

// ---------- Test server with header auth interceptor ----------

type authTestServer struct {
	lis         *bufconn.Listener
	server      *grpc.Server
	mockPubSvc  *MockPublishService
	mockService *MockAppService
}

func setupHeaderAuthTestServer(t *testing.T, pki *testPKI, requireMTLS bool) *authTestServer {
	lis := bufconn.Listen(bufSize)

	settings := &utils.Settings{}
	settings.Auth.HeaderAuth.Enabled = true
	settings.Auth.HeaderAuth.TimestampSkewSeconds = 300
	settings.Auth.RequireMTLS = requireMTLS

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	mockPubSvc := &MockPublishService{}
	mockService := &MockAppService{publishSvc: mockPubSvc}
	handler := NewPublishHandler(mockService)

	server := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			UnaryHeaderAuthInterceptor(pki.caPool, settings, logger),
		),
	)
	umsv1.RegisterSpinePublishServer(server, handler)

	go func() {
		if err := server.Serve(lis); err != nil {
			t.Logf("authTestServer exited: %v", err)
		}
	}()

	return &authTestServer{lis: lis, server: server, mockPubSvc: mockPubSvc, mockService: mockService}
}

func (ts *authTestServer) shutdown() {
	ts.server.GracefulStop()
	ts.lis.Close()
}

func (ts *authTestServer) dial(ctx context.Context, t *testing.T, opts ...grpc.DialOption) umsv1.SpinePublishClient {
	t.Helper()
	base := []grpc.DialOption{
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return ts.lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}
	base = append(base, opts...)
	conn, err := grpc.NewClient("passthrough:///bufnet", base...)
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })
	return umsv1.NewSpinePublishClient(conn)
}

// ---------- Interceptor that injects header‑auth metadata (client side) ----------

// headerAuthUnaryInterceptor returns a grpc.UnaryClientInterceptor that mirrors
// the SDK's UnaryHeaderAuthInterceptor but using in‑memory keys.
func headerAuthUnaryInterceptor(pki *testPKI) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		timestamp := time.Now().UTC().Format(time.RFC3339)

		var bodyBytes []byte
		if req != nil {
			if msg, ok := req.(proto.Message); ok {
				var err error
				bodyBytes, err = proto.Marshal(msg)
				if err != nil {
					return fmt.Errorf("marshal request: %w", err)
				}
			}
		}

		payload := buildGRPCPayload(method, timestamp, bodyBytes)
		sigDER, err := signPayloadForTest(pki.clientKey, payload)
		if err != nil {
			return err
		}

		ctx = metadata.AppendToOutgoingContext(ctx,
			"x-client-certificate", base64.StdEncoding.EncodeToString(pki.clientPEM),
			"x-request-timestamp", timestamp,
			"x-client-signature", base64.StdEncoding.EncodeToString(sigDER),
		)

		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// ---------- Tests ----------

func TestHeaderAuth_PublishWithValidCert(t *testing.T) {
	pki := newTestPKI(t)
	ts := setupHeaderAuthTestServer(t, pki, true /* requireMTLS */)
	defer ts.shutdown()

	publishResult := &service.PublishResult{
		Success:     true,
		MailboxSeqs: map[string]uint64{"mailbox-server-001": 1},
	}

	// The mock should receive a context carrying the identity injected by the interceptor.
	ts.mockPubSvc.On("Publish", mock.MatchedBy(func(ctx context.Context) bool {
		id := auth.IdentityFromContext(ctx)
		return id != nil && id.ClientID == "test-publisher" && id.Service == "server_api"
	}), mock.Anything, mock.Anything).Return(publishResult, nil)

	ctx := context.Background()
	client := ts.dial(ctx, t, grpc.WithUnaryInterceptor(headerAuthUnaryInterceptor(pki)))

	req := &umsv1.PublishRequest{
		Envelope: &umsv1.Envelope{
			EnvelopeId:  "auth-test-1",
			Qos:         umsv1.QoS_QOS_COMMAND,
			Type:        "test.header_auth",
			CreatedAtMs: time.Now().UnixMilli(),
			ExpiresAtMs: time.Now().Add(10 * time.Minute).UnixMilli(),
			Payload:     []byte(`{"ok":true}`),
			OrgId:       "org-test",
		},
		Targets: []*umsv1.Target{
			{TargetType: umsv1.TargetType_TARGET_TYPE_SERVER, TargetId: "server-001", OrgId: "org-test"},
		},
	}

	resp, err := client.Publish(ctx, req)
	require.NoError(t, err)
	assert.True(t, resp.Success, "publish should succeed")
	ts.mockPubSvc.AssertExpectations(t)
}

func TestHeaderAuth_RejectMissingHeaders(t *testing.T) {
	pki := newTestPKI(t)
	ts := setupHeaderAuthTestServer(t, pki, true /* requireMTLS */)
	defer ts.shutdown()

	ctx := context.Background()
	// Dial WITHOUT the auth interceptor — no headers will be sent.
	client := ts.dial(ctx, t)

	req := &umsv1.PublishRequest{
		Envelope: &umsv1.Envelope{
			EnvelopeId: "reject-test-1",
			Qos:        umsv1.QoS_QOS_COMMAND,
			Type:       "test.reject",
			Payload:    []byte(`{}`),
			OrgId:      "org-test",
		},
		Targets: []*umsv1.Target{
			{TargetType: umsv1.TargetType_TARGET_TYPE_SERVER, TargetId: "s1", OrgId: "org-test"},
		},
	}

	_, err := client.Publish(ctx, req)
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unauthenticated, st.Code())
}

func TestHeaderAuth_RejectBadSignature(t *testing.T) {
	pki := newTestPKI(t)
	ts := setupHeaderAuthTestServer(t, pki, true)
	defer ts.shutdown()

	// Generate a *different* key not signed by the CA
	rogueKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	// Build an interceptor that signs with the rogue key but sends the real cert
	badInterceptor := func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		timestamp := time.Now().UTC().Format(time.RFC3339)
		var bodyBytes []byte
		if req != nil {
			if msg, ok := req.(proto.Message); ok {
				bodyBytes, _ = proto.Marshal(msg)
			}
		}
		payload := buildGRPCPayload(method, timestamp, bodyBytes)
		sigDER, _ := signPayloadForTest(rogueKey, payload) // wrong key!

		ctx = metadata.AppendToOutgoingContext(ctx,
			"x-client-certificate", base64.StdEncoding.EncodeToString(pki.clientPEM),
			"x-request-timestamp", timestamp,
			"x-client-signature", base64.StdEncoding.EncodeToString(sigDER),
		)
		return invoker(ctx, method, req, reply, cc, opts...)
	}

	ctx := context.Background()
	client := ts.dial(ctx, t, grpc.WithUnaryInterceptor(badInterceptor))

	req := &umsv1.PublishRequest{
		Envelope: &umsv1.Envelope{
			EnvelopeId: "bad-sig-1",
			Qos:        umsv1.QoS_QOS_COMMAND,
			Type:       "test.badsig",
			Payload:    []byte(`{}`),
			OrgId:      "org-test",
		},
		Targets: []*umsv1.Target{
			{TargetType: umsv1.TargetType_TARGET_TYPE_SERVER, TargetId: "s1", OrgId: "org-test"},
		},
	}

	_, err = client.Publish(ctx, req)
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unauthenticated, st.Code())
}

func TestHeaderAuth_RejectUntrustedCert(t *testing.T) {
	pki := newTestPKI(t)
	ts := setupHeaderAuthTestServer(t, pki, true)
	defer ts.shutdown()

	// Create a *separate* CA and client cert — untrusted by the server.
	roguePKI := newTestPKI(t)

	ctx := context.Background()
	client := ts.dial(ctx, t, grpc.WithUnaryInterceptor(headerAuthUnaryInterceptor(roguePKI)))

	req := &umsv1.PublishRequest{
		Envelope: &umsv1.Envelope{
			EnvelopeId: "untrusted-cert-1",
			Qos:        umsv1.QoS_QOS_COMMAND,
			Type:       "test.untrusted",
			Payload:    []byte(`{}`),
			OrgId:      "org-test",
		},
		Targets: []*umsv1.Target{
			{TargetType: umsv1.TargetType_TARGET_TYPE_SERVER, TargetId: "s1", OrgId: "org-test"},
		},
	}

	_, err := client.Publish(ctx, req)
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unauthenticated, st.Code())
}

func TestHeaderAuth_PassthroughWhenNotRequired(t *testing.T) {
	pki := newTestPKI(t)
	// requireMTLS = false: requests without headers should pass through.
	ts := setupHeaderAuthTestServer(t, pki, false)
	defer ts.shutdown()

	publishResult := &service.PublishResult{
		Success:     true,
		MailboxSeqs: map[string]uint64{"mb-1": 1},
	}
	ts.mockPubSvc.On("Publish", mock.Anything, mock.Anything, mock.Anything).Return(publishResult, nil)

	ctx := context.Background()
	// No auth interceptor on client — headers will be absent.
	client := ts.dial(ctx, t)

	req := &umsv1.PublishRequest{
		Envelope: &umsv1.Envelope{
			EnvelopeId: "passthrough-1",
			Qos:        umsv1.QoS_QOS_COMMAND,
			Type:       "test.passthrough",
			Payload:    []byte(`{}`),
			OrgId:      "org-test",
		},
		Targets: []*umsv1.Target{
			{TargetType: umsv1.TargetType_TARGET_TYPE_SERVER, TargetId: "s1", OrgId: "org-test"},
		},
	}

	resp, err := client.Publish(ctx, req)
	require.NoError(t, err)
	assert.True(t, resp.Success, "should pass through when require_mtls is false")
}

func TestHeaderAuth_RejectExpiredTimestamp(t *testing.T) {
	pki := newTestPKI(t)
	ts := setupHeaderAuthTestServer(t, pki, true)
	defer ts.shutdown()

	// Interceptor that sends a timestamp 10 minutes in the past
	staleInterceptor := func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		timestamp := time.Now().Add(-10 * time.Minute).UTC().Format(time.RFC3339)
		var bodyBytes []byte
		if req != nil {
			if msg, ok := req.(proto.Message); ok {
				bodyBytes, _ = proto.Marshal(msg)
			}
		}
		payload := buildGRPCPayload(method, timestamp, bodyBytes)
		sigDER, _ := signPayloadForTest(pki.clientKey, payload)

		ctx = metadata.AppendToOutgoingContext(ctx,
			"x-client-certificate", base64.StdEncoding.EncodeToString(pki.clientPEM),
			"x-request-timestamp", timestamp,
			"x-client-signature", base64.StdEncoding.EncodeToString(sigDER),
		)
		return invoker(ctx, method, req, reply, cc, opts...)
	}

	ctx := context.Background()
	client := ts.dial(ctx, t, grpc.WithUnaryInterceptor(staleInterceptor))

	req := &umsv1.PublishRequest{
		Envelope: &umsv1.Envelope{
			EnvelopeId: "stale-ts-1",
			Qos:        umsv1.QoS_QOS_COMMAND,
			Type:       "test.staletime",
			Payload:    []byte(`{}`),
			OrgId:      "org-test",
		},
		Targets: []*umsv1.Target{
			{TargetType: umsv1.TargetType_TARGET_TYPE_SERVER, TargetId: "s1", OrgId: "org-test"},
		},
	}

	_, err := client.Publish(ctx, req)
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unauthenticated, st.Code())
}
