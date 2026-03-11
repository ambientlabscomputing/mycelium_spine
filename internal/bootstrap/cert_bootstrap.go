// Package bootstrap handles TLS certificate provisioning for Spine on startup.
// When settings.Bootstrap.CertCN is configured, Spine fetches its own TLS cert
// from server_api using an M2M token, writing the cert + key to disk so the gRPC
// server can load them normally. A CertManager enables zero-downtime cert renewal.
package bootstrap

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ambientlabscomputing/mycelium_spine/internal/utils"
)

const DefaultRenewBeforeDays = 7

// CertManager holds the active TLS certificate and supports atomic hot-swap
// via the GetCertificate callback, which is wired into the gRPC TLS config.
type CertManager struct {
	mu   sync.RWMutex
	cert *tls.Certificate
}

// NewCertManager wraps an initial certificate for use with a gRPC server.
func NewCertManager(cert tls.Certificate) *CertManager {
	return &CertManager{cert: &cert}
}

// GetCertificate satisfies the tls.Config.GetCertificate signature.
// The gRPC server calls this for every new TLS handshake, so cert renewals
// performed by RunRenewalLoop take effect without a server restart.
func (m *CertManager) GetCertificate(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cert, nil
}

// Update atomically replaces the live certificate.
func (m *CertManager) Update(cert tls.Certificate) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cert = &cert
}

// EnsureCACert fetches the CA certificate from server_api's public endpoint and
// writes it to the path specified by settings.GRPC.TLS.CAPath. This is safe to
// call on every startup because server_api's CA cert is publicly served.
func EnsureCACert(ctx context.Context, settings *utils.Settings) error {
	caPath := settings.GRPC.TLS.CAPath
	if caPath == "" {
		caPath = "certs/ca.crt"
	}

	baseURL := strings.TrimRight(settings.ServerAPI.BaseURL, "/")
	caCertURL := baseURL + "/ca/certificate"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, caCertURL, nil)
	if err != nil {
		return fmt.Errorf("build CA cert request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetch CA cert from %s: %w", caCertURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("CA cert fetch returned %d: %s", resp.StatusCode, string(body))
	}

	caCertPEM, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read CA cert response: %w", err)
	}

	if err := os.MkdirAll("certs", 0o700); err != nil {
		return fmt.Errorf("create certs dir: %w", err)
	}
	if err := os.WriteFile(caPath, caCertPEM, 0o644); err != nil {
		return fmt.Errorf("write CA cert to %s: %w", caPath, err)
	}

	return nil
}

// EnsureCert returns a valid spine TLS certificate. If the cert on disk is absent
// or will expire within settings.Bootstrap.RenewBeforeDays days, it fetches a
// fresh cert from server_api via the service CSR endpoint and writes it to disk.
func EnsureCert(ctx context.Context, settings *utils.Settings) (tls.Certificate, error) {
	certPath := settings.GRPC.TLS.CertPath
	keyPath := settings.GRPC.TLS.KeyPath

	renewBefore := time.Duration(settings.Bootstrap.RenewBeforeDays) * 24 * time.Hour
	if renewBefore == 0 {
		renewBefore = DefaultRenewBeforeDays * 24 * time.Hour
	}

	// Check if the on-disk cert is present and not expiring soon.
	if _, err := os.Stat(certPath); err == nil {
		if cert, err := tls.LoadX509KeyPair(certPath, keyPath); err == nil {
			if x509Cert, err := x509.ParseCertificate(cert.Certificate[0]); err == nil {
				if time.Until(x509Cert.NotAfter) > renewBefore {
					return cert, nil
				}
			}
		}
	}

	// Cert is absent or expiring — fetch a new one via M2M CSR flow.
	certCN := settings.Bootstrap.CertCN

	token, err := fetchM2MToken(ctx, settings)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("fetch M2M token: %w", err)
	}

	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("generate ECDSA key: %w", err)
	}

	csrPEM, err := generateCSR(privKey, certCN)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("generate CSR: %w", err)
	}

	certPEM, err := signServiceCSR(ctx, settings, token, "spine", csrPEM)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("sign CSR with server_api: %w", err)
	}

	keyBytes, err := x509.MarshalPKCS8PrivateKey(privKey)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("marshal private key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyBytes})

	if err := os.MkdirAll("certs", 0o700); err != nil {
		return tls.Certificate{}, fmt.Errorf("create certs dir: %w", err)
	}
	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		return tls.Certificate{}, fmt.Errorf("write cert to %s: %w", certPath, err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		return tls.Certificate{}, fmt.Errorf("write key to %s: %w", keyPath, err)
	}

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("parse newly issued cert: %w", err)
	}
	return cert, nil
}

// m2mTokenResponse is the Auth0 token endpoint response.
type m2mTokenResponse struct {
	AccessToken string `json:"access_token"`
}

func fetchM2MToken(ctx context.Context, settings *utils.Settings) (string, error) {
	m2m := settings.Bootstrap.M2M
	form := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {m2m.ClientID},
		"client_secret": {string(m2m.ClientSecret)},
		"audience":      {m2m.Audience},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m2m.TokenURL,
		strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("token request to %s: %w", m2m.TokenURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp m2mTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("decode token response: %w", err)
	}
	return tokenResp.AccessToken, nil
}

func generateCSR(key *ecdsa.PrivateKey, cn string) ([]byte, error) {
	template := &x509.CertificateRequest{
		Subject: pkix.Name{
			Organization: []string{"Underleaf"},
			CommonName:   cn,
		},
	}
	csrBytes, err := x509.CreateCertificateRequest(rand.Reader, template, key)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrBytes}), nil
}

type serviceCSRRequestBody struct {
	CSR     string `json:"csr"`
	Service string `json:"service"`
}

type serviceCSRResponse struct {
	CertificatePEM string `json:"certificate_pem"`
}

func signServiceCSR(ctx context.Context, settings *utils.Settings, token, service string, csrPEM []byte) ([]byte, error) {
	reqBody, err := json.Marshal(serviceCSRRequestBody{
		CSR:     string(csrPEM),
		Service: service,
	})
	if err != nil {
		return nil, err
	}

	baseURL := strings.TrimRight(settings.ServerAPI.BaseURL, "/")
	csrURL := baseURL + "/services/csr"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, csrURL,
		strings.NewReader(string(reqBody)))
	if err != nil {
		return nil, fmt.Errorf("build CSR request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("CSR request to %s: %w", csrURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("CSR signing returned %d: %s", resp.StatusCode, string(body))
	}

	var csrResp serviceCSRResponse
	if err := json.NewDecoder(resp.Body).Decode(&csrResp); err != nil {
		return nil, fmt.Errorf("decode CSR response: %w", err)
	}
	return []byte(csrResp.CertificatePEM), nil
}
