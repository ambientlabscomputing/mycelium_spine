# Mycelium Spine E2E Tests

End-to-end test suite for Mycelium Spine Universal Messaging Service (UMS).

## Overview

The E2E test suite validates the complete message flow through Mycelium Spine, including:

- Publishing messages from control planes
- Subscribing to mailboxes from multiple clients
- Message delivery with different QoS levels
- Acknowledgment flows
- Concurrent operations
- Cluster and broadcast targets

## Architecture

```
┌─────────────────┐
│  Publisher      │
│  (Control       │──┐
│   Plane)        │  │
└─────────────────┘  │
                     │    ┌──────────────────┐
┌─────────────────┐  │    │  Mycelium Spine  │
│  Test Client 1  │──┼───>│       UMS        │
│  (Server A)     │  │    │                  │
└─────────────────┘  │    │   MongoDB        │
                     │    └──────────────────┘
┌─────────────────┐  │
│  Test Client 2  │──┤
│  (Server B)     │  │
└─────────────────┘  │
                     │
┌─────────────────┐  │
│  Test Client 3  │──┘
│  (Cluster Node) │
└─────────────────┘
```

## Prerequisites

- Docker and Docker Compose
- Bash shell
- Make (optional, for convenience commands)

## Running Tests

### Quick Start

```bash
# Run all E2E tests
./e2e/test.sh
```

### Using Make

```bash
# Build and run E2E tests
make e2e-test

# Build Docker images only
make e2e-build

# Start services without running tests
make e2e-up

# Stop and cleanup
make e2e-down

# View logs
make e2e-logs
```

## Test Scenarios

### Test 1: Basic Publish and Subscribe

- Publisher sends message to target
- Single subscriber receives and acknowledges
- Validates end-to-end message flow

### Test 2: Multiple Subscribers Same Target

- Multiple clients subscribe to same server target
- Publisher sends one message
- All subscribers receive the message
- Tests fanout delivery

### Test 3: QoS Levels

- Publishes messages with COMMAND, CONTROL, and TELEMETRY QoS
- Validates different QoS handling
- Verifies acknowledgment requirements

### Test 4: Cluster Target

- Publishes to cluster target type
- Cluster node subscriber receives message
- Tests routing to cluster mailboxes

### Test 5: Acknowledgment Flow

- Subscriber receives without auto-ack
- Manual acknowledgment sent
- Validates ack processing

### Test 6: Concurrent Publishers

- Multiple publishers send simultaneously
- Single subscriber receives all messages
- Tests concurrent write handling

### Test 7: Payload Types

- Sends simple and complex JSON payloads
- Validates payload integrity
- Tests JSON handling

## Manual Testing

You can also run manual tests by executing commands in the containers:

```bash
# Start environment
docker-compose -f docker-compose.e2e.yml up -d

# Subscribe in one terminal
docker-compose -f docker-compose.e2e.yml exec test-client-1 mspinectl subscribe \
  --target-type server \
  --target-id test-server-01 \
  --auto-ack

# Publish in another terminal
docker-compose -f docker-compose.e2e.yml exec publisher-client mspinectl publish \
  --type command.test \
  --qos command \
  --target-type server \
  --target-id test-server-01 \
  --data '{"hello":"world"}'

# Cleanup
docker-compose -f docker-compose.e2e.yml down -v
```

## Container Details

### mycelium-spine

The main UMS service container:
- Runs the Mycelium Spine server
- Connects to MongoDB for persistence
- Exposes gRPC on port 9090

### test-client-1, test-client-2, test-client-3

Client containers with mspinectl CLI:
- Different server IDs for multi-client scenarios
- Pre-configured with environment variables
- Run as long-lived containers for exec commands

### publisher-client

Dedicated publisher for control plane operations:
- Used to simulate server_api/UCRS publishing
- Configured with control-plane server ID

### mongodb

Persistence layer:
- Stores mailboxes, envelopes, and cursors
- Exposed on port 27017 for debugging

## Environment Variables

Containers use these environment variables:

- `MSPINE_SERVER` - Mycelium Spine server address (default: mycelium-spine:9090)
- `MSPINE_SERVER_ID` - Server ID for authentication
- `MSPINE_ORG_ID` - Organization ID (default: test-org)
- `MONGODB_URI` - MongoDB connection string
- `GRPC_PORT` - gRPC server port (default: 9090)

## Debugging

### View container logs

```bash
# All services
docker-compose -f docker-compose.e2e.yml logs -f

# Specific service
docker-compose -f docker-compose.e2e.yml logs -f mycelium-spine
```

### Execute commands in container

```bash
# Open shell in container
docker-compose -f docker-compose.e2e.yml exec test-client-1 /bin/bash

# Run mspinectl commands
docker-compose -f docker-compose.e2e.yml exec test-client-1 mspinectl --help
```

### Check MongoDB data

```bash
# Connect to MongoDB
docker-compose -f docker-compose.e2e.yml exec mongodb mongosh \
  -u admin -p password123 mycelium_spine

# List mailboxes
db.mailboxes.find().pretty()

# List envelopes
db.envelopes.find().pretty()

# List cursors
db.cursors.find().pretty()
```

## Troubleshooting

### Services don't start

Check health status:
```bash
docker-compose -f docker-compose.e2e.yml ps
```

### Messages not delivered

1. Check Mycelium Spine logs for errors
2. Verify MongoDB connection
3. Ensure subscribers are connected before publishing
4. Check network connectivity between containers

### Tests fail intermittently

- Increase sleep delays in test.sh
- Check resource constraints on Docker
- Review container logs for timing issues

## CI/CD Integration

The E2E test suite can be integrated into CI/CD pipelines:

```yaml
# GitHub Actions example
- name: Run E2E Tests
  run: |
    cd mycelium_spine
    chmod +x e2e/test.sh
    ./e2e/test.sh
```

## Test Output

The test script provides colored output:
- 🔵 BLUE: Informational messages
- ✅ GREEN: Passed tests
- ❌ RED: Failed tests
- 🟡 YELLOW: Test names

Example output:
```
========================================
  Mycelium Spine E2E Test Suite
========================================

[INFO] Starting services...
[PASS] Mycelium Spine is healthy

[TEST] Test 1: Basic Publish and Subscribe
[INFO] Starting subscriber on test-client-1...
[INFO] Publishing message from publisher-client...
[PASS] Message successfully delivered

========================================
  Test Summary
========================================
Tests Run:    7
Tests Passed: 7
Tests Failed: 0
========================================
[PASS] All tests passed!
```

## Contributing

When adding new test scenarios:

1. Add test function to `e2e/test.sh`
2. Call it from `main()`
3. Use log functions for output
4. Clean up temporary files
5. Update this README with test description

## License

Copyright © 2026 Ambient Labs Computing
