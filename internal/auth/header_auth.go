package auth

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"time"
)

// HeaderAuth handles mTLS authentication through HTTP headers/gRPC metadata
// Based on the server_api implementation for Cloudflare tunnel environments

// ECDSASignature represents an ECDSA signature in ASN.1 DER format
type ECDSASignature struct {
	R, S *big.Int
}

// LoadCAPool creates a cert pool from a CA certificate file
func LoadCAPool(caPath string) (*x509.CertPool, error) {
	caCertBytes, err := os.ReadFile(caPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA certificate %q: %w", caPath, err)
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caCertBytes) {
		return nil, fmt.Errorf("failed to parse CA certificate %q", caPath)
	}

	return pool, nil
}

// ParseAndVerifyCertificate decodes, parses, and validates a client certificate
func ParseAndVerifyCertificate(certPEM []byte, caPool *x509.CertPool) (*x509.Certificate, error) {
	if len(certPEM) == 0 {
		return nil, fmt.Errorf("certificate PEM is empty")
	}

	// Decode PEM
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return nil, fmt.Errorf("failed to decode certificate PEM")
	}

	// Parse certificate
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %w", err)
	}

	// Verify against CA pool
	verifyOptions := x509.VerifyOptions{
		Roots:     caPool,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}

	if _, err := cert.Verify(verifyOptions); err != nil {
		return nil, fmt.Errorf("certificate verification failed: %w", err)
	}

	return cert, nil
}

// VerifyRequestSignature verifies that the given payload was signed by the cert's private key
func VerifyRequestSignature(cert *x509.Certificate, payload, signatureDER []byte) error {
	if cert == nil {
		return fmt.Errorf("certificate is nil")
	}

	publicKey, ok := cert.PublicKey.(*ecdsa.PublicKey)
	if !ok {
		return fmt.Errorf("certificate public key is not ECDSA")
	}

	// Parse ASN.1 DER encoded signature
	var sig ECDSASignature
	_, err := asn1.Unmarshal(signatureDER, &sig)
	if err != nil {
		return fmt.Errorf("failed to unmarshal signature: %w", err)
	}

	// Hash the payload
	hash := sha256.Sum256(payload)

	// Verify signature
	if !ecdsa.Verify(publicKey, hash[:], sig.R, sig.S) {
		return fmt.Errorf("signature verification failed")
	}

	return nil
}

// BuildGRPCSignaturePayload creates the data that should be signed for a gRPC request
// Format: GRPC\nFULL_METHOD\nTIMESTAMP\nBODY (body is optional)
func BuildGRPCSignaturePayload(fullMethod, timestamp string, body []byte) []byte {
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

// ValidateTimestamp ensures the provided timestamp string is within the allowed skew
// Returns the parsed timestamp and any error
func ValidateTimestamp(timestampStr string, maxSkewSeconds int) (time.Time, error) {
	if timestampStr == "" {
		return time.Time{}, fmt.Errorf("timestamp is empty")
	}

	ts, err := time.Parse(time.RFC3339, timestampStr)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid timestamp format: %w", err)
	}

	skew := time.Duration(maxSkewSeconds) * time.Second
	// Default to 5 minutes if not specified
	if skew == 0 {
		skew = 5 * time.Minute
	}

	timeDiff := time.Since(ts)
	if timeDiff > skew || timeDiff < -1*time.Minute {
		return ts, fmt.Errorf("timestamp out of acceptable range (diff: %v)", timeDiff)
	}

	return ts, nil
}
