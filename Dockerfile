# Underleaf Mycelium Spine (UMS) - Multi-stage Dockerfile

# ============================================================================
# Stage 1: Builder
# ============================================================================
FROM golang:1.25-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make protobuf-dev

# Install protoc plugins
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@latest && \
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Set working directory
WORKDIR /build

# Copy go.mod and go.sum first for layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Generate proto code
RUN make proto

# Build the application
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -installsuffix cgo \
    -ldflags="-w -s" \
    -o spine \
    cmd/serve/main.go

# ============================================================================
# Stage 2: Runtime
# ============================================================================
FROM alpine:latest

# Install ca-certificates for TLS
RUN apk --no-cache add ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1000 spine && \
    adduser -D -u 1000 -G spine spine

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/spine .

# Copy config example (actual config should be mounted)
COPY --from=builder /build/config.yaml .
COPY --from=builder /build/config.yaml.example .
COPY --from=builder /build/config.e2e.yaml .

# Create directories for certs and logs
RUN mkdir -p /app/certs /app/logs && \
    chown -R spine:spine /app

# Switch to non-root user
USER spine

# Expose gRPC port (default 9090)
EXPOSE 9090

# Health check (TODO: implement health endpoint)
# HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
#   CMD grpc_health_probe -addr=:9090 || exit 1

# Set environment variables
ENV CONFIG_PATH=/app/config.yaml

# Run the application
CMD ["./spine"]
