# UMS Security and mTLS Setup

## Overview

Underleaf Mycelium Spine (UMS) requires mutual TLS (mTLS) for:
- **Client-Server authentication**: Both client and server authenticate each other using certificates
- **Identity validation**: Each client identifies itself via the CN (CommonName) in the certificate
- **Tenant isolation**: The OU (Organization Unit) field identifies the service/tenant
- **Authorization**: Only authorized services can publish commands

## Certificate Structure

The mTLS certificates follow the  PKI structure:

```
CA (Root Certificate)
├── Certificate CN: "mycelium_spine" (optional, root)
├── Certificate OU: "" (not used at root level)

Server Certificate
├── CN: "mycelium_spine_server" (identifies the UMS server)
├── OU: "server" (server identity)
├── Signed by: CA
├── Valid for: TLS_CERT_PATH
├── Key at: TLS_KEY_PATH

Client Certificate (example: server_api service)
├── CN: "server_api_client" (identifies the client)
├── OU: "server_api" (service name - used for authorization)
├── Signed by: CA
├── Valid for: Client mTLS authentication

Client Certificate (example: UCRS service)
├── CN: "ucrs_client" (identifies the client)
├── OU: "ucrs" (service name)
├── Signed by: CA
├── Valid for: Command publishing
```

## Development Certificate Generation

**WARNING: For development only. Use your organization's PKI for production.**

### 1. Generate CA Private Key and Certificate

```bash
openssl genrsa -out ca.key 4096

openssl req -new -x509 -days 3650 -key ca.key -out ca.crt \
  -subj "/CN=Mycelium Spine CA/O=Ambient Labs/C=US"
```

### 2. Generate Server Certificate

```bash
# Generate server private key
openssl genrsa -out spine_server.key 4096

# Create CSR (Certificate Signing Request)
openssl req -new -key spine_server.key -out spine_server.csr \
  -subj "/CN=mycelium_spine_server/OU=server/O=Ambient Labs/C=US"

# Sign with CA
openssl x509 -req -days 365 \
  -in spine_server.csr \
  -CA ca.crt -CAkey ca.key -CAcreateserial \
  -out spine_server.crt \
  -extensions v3_req -extfile <(printf "subjectAltName=localhost,127.0.0.1")

rm spine_server.csr
```

### 3. Generate Client Certificates

**For server_api service:**

```bash
openssl genrsa -out server_api_client.key 4096

openssl req -new -key server_api_client.key -out server_api_client.csr \
  -subj "/CN=server_api_client/OU=server_api/O=Ambient Labs/C=US"

openssl x509 -req -days 365 \
  -in server_api_client.csr \
  -CA ca.crt -CAkey ca.key -CAcreateserial \
  -out server_api_client.crt

rm server_api_client.csr
```

**For UCRS service:**

```bash
openssl genrsa -out ucrs_client.key 4096

openssl req -new -key ucrs_client.key -out ucrs_client.csr \
  -subj "/CN=ucrs_client/OU=ucrs/O=Ambient Labs/C=US"

openssl x509 -req -days 365 \
  -in ucrs_client.csr \
  -CA ca.crt -CAkey ca.key -CAcreateserial \
  -out ucrs_client.crt

rm ucrs_client.csr
```

## Deployment in Kubernetes

Mount certificates as secrets:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: ums-tls
type: kubernetes.io/tls
data:
  tls.crt: base64-encoded-server-cert
  tls.key: base64-encoded-server-key
  ca.crt: base64-encoded-ca-cert
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: ums-config
data:
  config.yaml: |
    grpc:
      tls:
        cert_path: /etc/ums/tls/tls.crt
        key_path: /etc/ums/tls/tls.key
        ca_path: /etc/ums/tls/ca.crt
        client_auth: "require"
---
apiVersion: v1
kind: Pod
spec:
  volumes:
    - name: tls
      secret:
        secretName: ums-tls
    - name: config
      configMap:
        name: ums-config
  containers:
    - name: ums
      volumeMounts:
        - name: tls
          mountPath: /etc/ums/tls
          readOnly: true
        - name: config
          mountPath: /etc/ums
          readOnly: true
      env:
        - name: MONGODB_URI
          valueFrom:
            secretKeyRef:
              name: ums-secrets
              key: mongodb-uri
```

## Docker Deployment

Use environment variables to inject secrets:

```bash
docker run -d \
  -e TLS_CERT_PATH=/etc/ums/tls/server.crt \
  -e TLS_KEY_PATH=/etc/ums/tls/server.key \
  -e TLS_CA_PATH=/etc/ums/tls/ca.crt \
  -e MONGODB_URI="mongodb://user:pass@mongo:27017/ums" \
  -v /path/to/certs:/etc/ums/tls:ro \
  -v /path/to/config.production.yaml:/etc/ums/config.yaml:ro \
  ums:latest
```

## Certificate Rotation

Rotate certificates before expiration:

1. Generate new server certificate (same CN and OU)
2. Mount the new cert in the running container
3. Restart UMS gracefully (existing connections drain, new connections use new cert)

For client certificates, the issuing service (server_api, UCRS) must restart to use the new cert.

## Security Checklist

- [ ] TLS certificates are NOT committed to version control
- [ ] `client_auth: "require"` is set in production config
- [ ] Certificates have appropriate validity periods (server: 1 year, client: ~90 days)
- [ ] CA certificate is stored securely (not in container image)
- [ ] All clients present valid certificates signed by the same CA
- [ ] gRPC reflection is disabled in production
- [ ] MongoDB connection uses TLS with validated certs
- [ ] Regular certificate rotation is scheduled

## Troubleshooting

**Client certificate rejected:**
- Check certificate CN matches the client_id being used
- Verify certificate is signed by the correct CA
- Check certificate has not expired: `openssl x509 -noout -dates -in cert.crt`
- Verify OU field matches authorized services in config

**mTLS handshake fails:**
- Check server cert is valid: `openssl x509 -noout -dates -in server.crt`
- Verify CA cert matches the CA that signed server cert: `openssl verify -CAfile ca.crt spine_server.crt`
- Check TLS paths in config point to correct files
- Verify file permissions: certs should be readable by the UMS process

**Metrics not reflected:**
- Check peer extraction code if mTLS is not being detected
- Verify TLS is actually enabled on the server
- Check gRPC logs for credential-related errors
