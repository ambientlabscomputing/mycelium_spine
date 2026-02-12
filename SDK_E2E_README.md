# Mycelium Spine SDK & E2E Test Suite

Complete SDK, CLI tool, and end-to-end test suite for Mycelium Spine Universal Messaging Service (UMS).

## 📦 What's New

This repository now includes:

1. **Go SDK** (`/sdk`) - Client library for interacting with Mycelium Spine
2. **CLI Tool** (`/cmd/mspinectl`) - Command-line interface for UMS operations
3. **E2E Test Suite** (`/e2e`) - Comprehensive end-to-end tests with Docker orchestration

## 🚀 Quick Start

### Prerequisites

- Go 1.24+
- Docker & Docker Compose
- Make

### 1. Build Everything

```bash
# Generate proto code and build service
make build

# Build SDK
make sdk-build

# Build CLI
make cli-build
```

### 2. Run E2E Tests

```bash
# Build and run complete E2E test suite
make e2e-test
```

This will:
- Build Mycelium Spine service container
- Build test client containers with mspinectl
- Start MongoDB and all services
- Run 7 comprehensive test scenarios
- Report results

## 📚 SDK Documentation

### Installation

```bash
go get github.com/ambientlabscomputing/mycelium_spine/sdk
```

### Streaming Client Example

```go
package main

import (
    "context"
    "log"
    
    "github.com/ambientlabscomputing/mycelium_spine/sdk"
    umsv1 "github.com/ambientlabscomputing/mycelium_spine/proto/ums/v1"
)

func main() {
    // Create client
    client, _ := sdk.NewClient("localhost:9090", sdk.ClientConfig{
        ServerID: "my-server-01",
        OrgID:    "org-123",
    })
    defer client.Close()
    
    // Connect
    client.Connect(context.Background())
    
    // Subscribe to mailbox
    targets := []*umsv1.Target{{
        TargetType: umsv1.TargetType_TARGET_TYPE_SERVER,
        TargetId:   "my-server-01",
        OrgId:      "org-123",
    }}
    client.Subscribe(targets)
    
    // Receive messages
    for delivery := range client.Deliveries() {
        for _, envelope := range delivery.Envelopes {
            log.Printf("Received: %s", envelope.Type)
            client.Ack(envelope.MailboxId, envelope.Seq)
        }
    }
}
```

### Publisher Example

```go
package main

import (
    "context"
    
    "github.com/ambientlabscomputing/mycelium_spine/sdk"
    umsv1 "github.com/ambientlabscomputing/mycelium_spine/proto/ums/v1"
)

func main() {
    // Create publisher
    publisher, _ := sdk.NewPublisher("localhost:9090")
    defer publisher.Close()
    
    // Publish command
    envelope := &umsv1.Envelope{
        Type:    "command.deploy",
        Qos:     umsv1.QoS_QOS_COMMAND,
        Payload: []byte(`{"version":"v2.0"}`),
        OrgId:   "org-123",
    }
    
    targets := []*umsv1.Target{{
        TargetType: umsv1.TargetType_TARGET_TYPE_SERVER,
        TargetId:   "prod-server-01",
        OrgId:      "org-123",
    }}
    
    resp, _ := publisher.Publish(context.Background(), envelope, targets)
    log.Printf("Published to: %v", resp.MailboxSeqs)
}
```

## 🔧 CLI Usage

### Installation

```bash
# Build and install
make cli-install
```

### Commands

#### Subscribe to Messages

```bash
# Subscribe to server mailbox
mspinectl subscribe \
  --server localhost:9090 \
  --server-id my-server-01 \
  --org-id org-123 \
  --target-type server \
  --target-id my-server-01 \
  --auto-ack

# Subscribe to cluster
mspinectl subscribe \
  --target-type cluster \
  --target-id cluster-west \
  --count 10
```

#### Publish Messages

```bash
# Publish command
mspinectl publish \
  --server localhost:9090 \
  --server-id control-plane \
  --org-id org-123 \
  --type command.deploy \
  --qos command \
  --target-type server \
  --target-id prod-01 \
  --data '{"version":"v2.1.0"}'

# Publish from file
mspinectl publish \
  --type command.config \
  --qos command \
  --target-type cluster \
  --target-id cluster-east \
  --file ./config.json
```

#### Acknowledge Messages

```bash
# Send acknowledgment
mspinectl ack \
  --server localhost:9090 \
  --server-id my-server-01 \
  --org-id org-123 \
  --mailbox-id MAILBOX_ID \
  --seq 42
```

### Environment Variables

Configure defaults with environment variables:

```bash
export MSPINE_SERVER=localhost:9090
export MSPINE_SERVER_ID=my-server-01
export MSPINE_ORG_ID=org-123

# Now you can omit --server, --server-id, --org-id flags
mspinectl subscribe --target-type server --target-id my-server-01
```

## 🧪 E2E Test Suite

The E2E test suite validates complete message flows through the system.

### Architecture

```
┌─────────────┐       ┌──────────────────┐       ┌─────────────┐
│ Publisher   │──────>│  Mycelium Spine  │<──────│ Test Client │
│ (Control)   │       │       UMS        │       │  (Server A) │
└─────────────┘       │                  │       └─────────────┘
                      │   MongoDB        │
┌─────────────┐       └──────────────────┘       ┌─────────────┐
│ Test Client │<──────────────┘                  │ Test Client │
│ (Server B)  │                                   │  (Cluster)  │
└─────────────┘                                   └─────────────┘
```

### Running Tests

```bash
# Full test suite
make e2e-test

# Start environment for manual testing
make e2e-up

# View logs
make e2e-logs

# Stop environment
make e2e-down

# Clean everything
make e2e-clean
```

### Test Scenarios

1. **Basic Publish & Subscribe** - Single publisher to single subscriber
2. **Multiple Subscribers** - Fanout delivery to multiple clients
3. **QoS Levels** - Command, Control, and Telemetry message handling
4. **Cluster Targets** - Cluster-based routing
5. **Acknowledgments** - Manual and automatic ack flows
6. **Concurrent Publishers** - Multiple publishers sending simultaneously
7. **Payload Types** - JSON validation and integrity

### Manual Testing

```bash
# Start environment
make e2e-up

# In terminal 1: Subscribe
docker-compose -f docker-compose.e2e.yml exec test-client-1 \
  mspinectl subscribe \
    --target-type server \
    --target-id test-server-01 \
    --auto-ack

# In terminal 2: Publish
docker-compose -f docker-compose.e2e.yml exec publisher-client \
  mspinectl publish \
    --type command.test \
    --qos command \
    --target-type server \
    --target-id test-server-01 \
    --data '{"hello":"world"}'

# Cleanup
make e2e-down
```

## 📊 Test Coverage

Current test coverage:

- **Types Package**: 100% (envelopes, mailboxes, sessions, targets)
- **Workers Package**: 46.7% (worker pools, concurrent processing)
- **E2E Tests**: 7 scenarios covering real-world usage

Generate coverage report:

```bash
make coverage
open coverage.html
```

## 🏗️ Project Structure

```
mycelium_spine/
├── sdk/                    # Go SDK for clients
│   ├── client.go          # Streaming client
│   ├── publisher.go       # Publishing client
│   └── README.md          # SDK documentation
├── cmd/
│   ├── mspinectl/         # CLI tool
│   │   ├── cmd/           # Cobra commands
│   │   └── main.go
│   └── serve/             # UMS service
├── e2e/                   # E2E test suite
│   ├── test.sh           # Test orchestration script
│   └── README.md         # E2E documentation
├── internal/              # Service implementation
│   ├── types/            # Domain types (100% tested)
│   ├── workers/          # Worker pools (46.7% tested)
│   ├── service/          # Business logic
│   ├── repository/       # Data access
│   └── grpc/             # gRPC handlers
├── proto/                # Proto definitions
├── Dockerfile            # Service container
├── Dockerfile.testclient # Test client container
├── docker-compose.e2e.yml # E2E orchestration
└── Makefile              # Build automation
```

## 🔌 Integration Examples

### Server API Publishing

```go
// In server_api: publish deployment commands
publisher, _ := sdk.NewPublisher("mycelium-spine:9090")

envelope := &umsv1.Envelope{
    Type:    "command.deploy",
    Qos:     umsv1.QoS_QOS_COMMAND,
    Payload: []byte(deployPayload),
    OrgId:   serverOrgID,
}

targets := []*umsv1.Target{{
    TargetType: umsv1.TargetType_TARGET_TYPE_SERVER,
    TargetId:   targetServerID,
    OrgId:      serverOrgID,
}}

publisher.Publish(ctx, envelope, targets)
```

### Underleaf Client Receiving

```go
// In underleaf_client: receive commands
client, _ := sdk.NewClient("mycelium-spine:9090", sdk.ClientConfig{
    ServerID: myServerID,
    OrgID:    myOrgID,
})

client.Connect(ctx)

targets := []*umsv1.Target{{
    TargetType: umsv1.TargetType_TARGET_TYPE_SERVER,
    TargetId:   myServerID,
    OrgId:      myOrgID,
}}

client.Subscribe(targets)

for delivery := range client.Deliveries() {
    for _, envelope := range delivery.Envelopes {
        // Process command
        handleCommand(envelope)
        client.Ack(envelope.MailboxId, envelope.Seq)
    }
}
```

## 🐛 Debugging

### View Service Logs

```bash
# E2E environment
make e2e-logs

# Specific service
docker-compose -f docker-compose.e2e.yml logs -f mycelium-spine
```

### Inspect MongoDB

```bash
# Connect to MongoDB
docker-compose -f docker-compose.e2e.yml exec mongodb mongosh \
  -u admin -p password123 mycelium_spine

# View mailboxes
db.mailboxes.find().pretty()

# View envelopes
db.envelopes.find().sort({created_at_ms: -1}).limit(10).pretty()

# View cursors
db.cursors.find().pretty()
```

### Debug Client Connection

```bash
# Enable verbose output
mspinectl subscribe --verbose \
  --target-type server \
  --target-id my-server-01
```

## 📝 Make Targets

### Development

- `make build` - Build Mycelium Spine service
- `make test` - Run unit tests
- `make coverage` - Generate coverage report
- `make run` - Run service locally
- `make proto` - Generate proto code

### SDK/CLI

- `make sdk-build` - Build SDK
- `make cli-build` - Build mspinectl
- `make cli-install` - Install mspinectl globally

### E2E Testing

- `make e2e-build` - Build E2E containers
- `make e2e-test` - Run E2E test suite
- `make e2e-up` - Start E2E environment
- `make e2e-down` - Stop E2E environment
- `make e2e-logs` - View E2E logs
- `make e2e-clean` - Clean E2E volumes/images

## 🤝 Contributing

When adding features:

1. Update SDK if API changes
2. Add CLI commands if user-facing
3. Add E2E tests for new scenarios
4. Update documentation
5. Run full test suite: `make test && make e2e-test`

## 📄 License

Copyright © 2026 Ambient Labs Computing

## 🎯 Next Steps

- [ ] Add TLS support to SDK/CLI
- [ ] Implement session resumption in SDK
- [ ] Add metrics endpoint querying to CLI
- [ ] Create load testing scenarios
- [ ] Add CI/CD pipeline for automated testing
- [ ] Build language bindings (Python, TypeScript)

## 📞 Support

For issues or questions:
- File an issue on GitHub
- See `/sdk/README.md` for SDK docs
- See `/e2e/README.md` for E2E test docs
