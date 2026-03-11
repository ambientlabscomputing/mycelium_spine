# Spine Certificate Bootstrap Plan

## Background

`mycelium_spine` (the Nginx/mTLS reverse proxy) currently requires its TLS certificate and key to be pre-staged on disk by an operator before startup (`certs/spine.crt`, `certs/spine.key`). The CA cert (`certs/ca.crt`) must also be present for client verification. This process is documented in `MTLS_GUIDE.md` and was a manual, error-prone operation — the same class of problem fixed for MMA by Phase 3 of the cert architecture cleanup.

The goal of this plan is to give Spine the same self-bootstrapping capability: on startup, if its cert is absent or expired, Spine fetches a CA-signed cert dynamically from server_api using its M2M service account token.

---

## Architecture

### Why Spine is Different from MMA

| Dimension           | MMA                                              | Spine                                         |
|---------------------|--------------------------------------------------|-----------------------------------------------|
| Lifetime            | Ephemeral; started/stopped by the agent          | Long-lived service; restarts infrequently     |
| Cert storage        | In-memory `*tls.Config` only                    | Disk (cert survives restarts, avoids CSR on every boot) |
| Identity            | `CN=<server_id>` (per-server)                   | `CN=spine.<env>` (per-deployment)             |
| Credential source   | Agent platform config → kernel IssueLocalCertificate | M2M service account token (Auth0 client credentials) |

### CA Authority

server_api is the sole CA authority. Its public endpoint:

```
GET /api/v1/servers/ca/certificate
```

returns the PEM-encoded CA cert (no auth required). Spine should always fetch the CA cert from this endpoint on startup to ensure it has the current CA even after a CA rotation.

---

## Bootstrap Flow

```
Spine startup
    │
    ├─ GET /api/v1/servers/ca/certificate  →  save to certs/ca.crt
    │
    ├─ certs/spine.crt present and valid (expiry > 7 days)?
    │   └─ YES → load cert from disk, start Nginx with mTLS config
    │
    └─ NO (missing or expiring)
        │
        ├─ POST /oauth/token (Auth0 client_credentials)
        │       client_id, client_secret from config.yaml [hyphae.m2m]
        │   → M2M access token
        │
        ├─ Generate ECDSA P-256 key  (certs/spine.key)
        │
        ├─ Generate CSR  CN=spine.<env>  O=Underleaf
        │
        ├─ POST /api/v1/servers/services/csr  (Bearer: M2M token)
        │       body: { "csr": "<PEM>", "service": "spine" }
        │   → signed cert PEM
        │
        ├─ Save cert to certs/spine.crt
        │
        └─ Start Nginx with mTLS config
```

---

## server_api Changes Required

### New endpoint: `POST /api/v1/servers/services/csr`

Accepts CSR from a service account (M2M token), signs it with the platform CA, returns the signed cert.

```go
// Request
type ServiceCSRRequest struct {
    CSR     string `json:"csr"`      // PEM-encoded PKCS#10 CSR
    Service string `json:"service"`  // e.g. "spine", "hyphae"
}

// Response
type ServiceCSRResponse struct {
    CertificatePEM string `json:"certificate_pem"`
}
```

**Authorization**: Verify the M2M token via Auth0 JWKS. The token's `azp` (authorized party) claim must match the configured Spine client ID. Use an `allowed_services` list in server_api config to control which service accounts can call this endpoint.

**Cert constraints**:
- Subject: `O=Underleaf, CN=<service>.<deployment_env>`
- Extended Key Usage: `serverAuth` + `clientAuth`
- Validity: 90 days (unlike agent certs which are 30 days; Spine is long-lived)
- Spine should trigger renewal when ≤ 7 days remain (i.e., rotate at day 83)

---

## Spine Implementation

### New file: `internal/bootstrap/cert_bootstrap.go`

```go
package bootstrap

// EnsureCert checks whether the Spine cert is present and valid.
// If missing or expiring within renewThreshold, it fetches a fresh
// cert from server_api using the M2M token and writes it to disk.
func EnsureCert(ctx context.Context, cfg *Config) error
```

**Config fields needed** (all sourced from `config.yaml`):
```yaml
server_api:
  base_url: https://api.underleafdev.com
  ca_cert_path: certs/ca.crt          # written by bootstrap itself

spine:
  cert_path: certs/spine.crt
  key_path:  certs/spine.key
  cn:        spine.dev                 # CN for the cert

hyphae:
  m2m:
    client_id:     <Auth0 client ID>
    client_secret: <Auth0 client secret>
    token_url:     https://underleafdev.auth0.com/oauth/token
    audience:      https://api.underleafdev.com
```

### Integration point: `cmd/serve/main.go`

```go
func main() {
    cfg := config.Load()

    // Bootstrap: fetch CA cert + ensure Spine's own cert is valid.
    // Blocks startup until certs are ready (with retry/backoff).
    if err := bootstrap.EnsureCert(ctx, cfg); err != nil {
        slog.Error("spine cert bootstrap failed", "error", err)
        os.Exit(1)
    }

    // Start Nginx with the bootstrapped cert config
    startNginx(cfg)
}
```

Use the same retry pattern as MMA bootstrap: 5 attempts, 2s initial backoff, exponential doubling, context-aware.

---

## Certificate Rotation

Spine is long-lived so proactive rotation is required. Two mechanisms:

1. **Startup check**: On every Spine restart, `EnsureCert` checks expiry. If ≤ 7 days remain, it renews before starting Nginx.

2. **Background renewal goroutine**: After startup, a goroutine checks the cert expiry every 24h. When ≤ 7 days remain, it performs the CSR flow and signals Nginx to reload via `nginx -s reload` (zero-downtime cert swap).

```go
go bootstrap.RunRenewalLoop(ctx, cfg, renewThreshold)
```

---

## Security Considerations

- **Private key never leaves Spine**: The ECDSA key is generated locally; only the CSR is sent to server_api. server_api never sees the private key.
- **M2M secret at rest**: Store `client_secret` in an environment variable or a secrets manager (e.g. Vault), not directly in `config.yaml`. The config struct should read from `SPINE_M2M_CLIENT_SECRET` env var with `config.yaml` as fallback.
- **CA cert pinning**: The CA cert is fetched once on startup and written to disk. Subsequent TLS calls (including the CSR POST) verify against this pinned CA cert. This avoids TOFU (trust-on-first-use) issues if the server_api endpoint is behind the load balancer.
- **CSR validation server-side**: server_api must verify the CSR signature before signing. The CN must match an allowed pattern (`spine.*`, `hyphae.*`) — never allow arbitrary CNs from service accounts.

---

## File Structure After Bootstrap

```
mycelium_spine/
  certs/
    ca.crt          ← fetched from GET /api/v1/servers/ca/certificate on startup
    spine.crt       ← fetched via CSR flow, rotated proactively
    spine.key       ← generated locally, never transmitted
  internal/
    bootstrap/
      cert_bootstrap.go   ← new
      renewal.go          ← new
```

The `certs/` directory should remain in `.gitignore` (already is). The `ca.crt` in the repo root is a development-time reference only and should not be relied on at runtime.

---

## Migration from Manual Setup

Once this bootstrap is implemented:

1. Remove the `certs/spine.crt` and `certs/spine.key` pre-staging step from `MTLS_GUIDE.md` (replace with "Spine bootstraps its own cert on startup").
2. Remove any `scp certs/spine.*` steps from deployment scripts.
3. The `certs/ca.crt` in the repo becomes a development-only reference; production deployments always fetch it from server_api.
4. Update the `MTLS_GUIDE.md` to document the new M2M config fields required in `config.yaml`.

---

## Implementation Order

1. **server_api**: Add `POST /api/v1/servers/services/csr` endpoint (new route, M2M auth middleware, signing logic)
2. **Spine**: Add `bootstrap.EnsureCert` + integration in `cmd/serve/main.go`
3. **Spine**: Add `bootstrap.RunRenewalLoop` goroutine
4. **Cleanup**: Remove manual cert staging from deployment docs and scripts
