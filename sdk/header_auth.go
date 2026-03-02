package sdk

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
)

// Header auth metadata keys (must be lowercase for gRPC)
const (
	headerClientCert = "x-client-certificate"
	headerClientSig  = "x-client-signature"
	headerTimestamp  = "x-request-timestamp"
)

// HeaderAuthCredentials stores the credentials needed for mTLS-over-headers
type HeaderAuthCredentials struct {
	PrivateKey *ecdsa.PrivateKey
	CertPEM    []byte
}

// LoadHeaderAuthCredentials loads certificate and private key from files
func LoadHeaderAuthCredentials(certPath, keyPath string) (*HeaderAuthCredentials, error) {
	// Load cert
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read certificate %q: %w", certPath, err)
	}

	// Load key
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read private key %q: %w", keyPath, err)
	}

	block, _ := pem.Decode(keyPEM)
	if block == nil {
		return nil, fmt.Errorf("failed to decode private key PEM")
	}

	keyInterface, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		// Try PKCS1
		keyInterface, err = x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			// Try EC private key
			keyInterface, err = x509.ParseECPrivateKey(block.Bytes)
			if err != nil {
				return nil, fmt.Errorf("failed to parse private key: %w", err)
			}
		}
	}

	privateKey, ok := keyInterface.(*ecdsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("private key is not ECDSA")
	}

	return &HeaderAuthCredentials{
		PrivateKey: privateKey,
		CertPEM:    certPEM,
	}, nil
}

// ECDSASignature represents an ECDSA signature in ASN.1 DER format
type ECDSASignature struct {
	R, S *big.Int
}

func signPayload(privateKey *ecdsa.PrivateKey, payload []byte) ([]byte, error) {
	hash := sha256.Sum256(payload)
	r, s, err := ecdsa.Sign(nil, privateKey, hash[:])
	if err != nil {
		return nil, fmt.Errorf("failed to sign payload: %w", err)
	}

	return asn1.Marshal(ECDSASignature{R: r, S: s})
}

// buildSignaturePayload creates the data that should be signed for a gRPC request
func buildSignaturePayload(fullMethod, timestamp string, body []byte) []byte {
	var buf bytes.Buffer
	buf.WriteString("GRPC\n")
	buf.WriteString(fullMethod)
	buf.WriteString("\n")
	buf.WriteString(timestamp)
	buf.WriteString("\n")
	if len(body) > 0 {
		buf.Write(body)
	}
	return buf.Bytes()
}

// UnaryHeaderAuthInterceptor creates a client interceptor for unary RPCs
func UnaryHeaderAuthInterceptor(creds *HeaderAuthCredentials) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		timestamp := time.Now().UTC().Format(time.RFC3339)

		var bodyBytes []byte
		if req != nil {
			if message, ok := req.(proto.Message); ok {
				var err error
				bodyBytes, err = proto.Marshal(message)
				if err != nil {
					return fmt.Errorf("failed to marshal request: %w", err)
				}
			}
		}

		payload := buildSignaturePayload(method, timestamp, bodyBytes)
		sigDER, err := signPayload(creds.PrivateKey, payload)
		if err != nil {
			return err
		}

		ctx = metadata.AppendToOutgoingContext(ctx,
			headerClientCert, base64.StdEncoding.EncodeToString(creds.CertPEM),
			headerTimestamp, timestamp,
			headerClientSig, base64.StdEncoding.EncodeToString(sigDER),
		)

		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// StreamHeaderAuthInterceptor creates a client interceptor for stream RPCs
func StreamHeaderAuthInterceptor(creds *HeaderAuthCredentials) grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		timestamp := time.Now().UTC().Format(time.RFC3339)

		// For streams, we sign only the method name and timestamp
		payload := buildSignaturePayload(method, timestamp, nil)
		sigDER, err := signPayload(creds.PrivateKey, payload)
		if err != nil {
			return nil, err
		}

		ctx = metadata.AppendToOutgoingContext(ctx,
			headerClientCert, base64.StdEncoding.EncodeToString(creds.CertPEM),
			headerTimestamp, timestamp,
			headerClientSig, base64.StdEncoding.EncodeToString(sigDER),
		)

		return streamer(ctx, desc, cc, method, opts...)
	}
}
