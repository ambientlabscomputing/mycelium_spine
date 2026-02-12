# Underleaf Mycelium Spine (UMS)

**Underleaf Mycelium Spine (UMS)** is the cloud↔edge control fabric protocol that replaces Event Bus v4. It provides long-lived, client-initiated connectivity with server push capabilities over gRPC bidirectional streaming, enabling low-latency command/control and targeted messaging for distributed edge systems.

## Features

- **gRPC Bidirectional Streaming** - NAT-safe, agent-initiated connections with server push
- **Durable Mailboxes** - Per-target (SERVER/CLUSTER/ORG) message queues with retention policies
- **QoS Classes** - Three-tier quality of service (COMMAND, CONTROL, TELEMETRY)
- **At-Least-Once Delivery** - Cumulative ACK semantics with session resumption
- **Flow Control** - Configurable inflight limits per QoS with backpressure handling
- **Cluster Fanout** - Publish to all servers in a cluster without polling
- **Worker Pool Architecture** - Typed, independently-scalable worker pools for parallelism
- **Multi-Tenancy** - Organization-level isolation enforced at database layer

## Quick Start

```bash
# 1. Install dependencies
make deps install-tools

# 2. Generate proto code
make proto

# 3. Configure
cp config.yaml.example config.yaml

# 4. Start with Docker Compose (includes MongoDB)
make docker-run

# Or run locally
make run
```

## Documentation

- [Full README](README_FULL.md) - Complete documentation
- [Protocol Specification](guiding_docs/SPEC.md) - RFC-style UMS protocol spec
- [Code Conventions](../CODE_CONVENTIONS.md) - Underleaf coding standards

## Project Structure

```
cmd/serve/           # Main entry point
proto/ums/v1/        # Protobuf definitions
internal/
  ├── grpc/          # gRPC handlers
  ├── service/       # Business logic
  ├── repository/    # MongoDB data access
  ├── types/         # Domain types
  ├── workers/       # Worker pool framework
  └── utils/         # Settings, logging
```

## Status

**V1 Complete** - Core functionality implemented:
- ✅ Proto definitions (SpineStream, SpinePublish)
- ✅ Session management (HELLO, resume)  
- ✅ Mailbox persistence (MongoDB)
- ✅ Delivery with flow control
- ✅ Worker pool architecture
- ✅ Docker deployment

**Next Phase**: Session resumption refinement, notification channels, Prometheus metrics

## License

Proprietary - Ambient Labs Computing
