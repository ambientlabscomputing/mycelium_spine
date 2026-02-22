# Mycelium Spine E2E Tests

Comprehensive end-to-end tests for the Universal Messaging Service (UMS).

## Overview

This directory contains Go-based e2e tests that validate the complete UMS system including:

- Session lifecycle (connect, resume, disconnect)
- Publish/subscribe flows
- Multi-client scenarios
- Reconnection and replay logic
- Delivery latency and performance

## Test Files

### 1. `e2e_session_lifecycle_test.go`
Tests complete session lifecycle:
- Client connects with HELLO, receives WELCOME
- Client reconnects with RESUME, receives RESUME_OK
- Session state persists across reconnects
- Heartbeat keeps session alive
- Session cleanup on disconnect
- New session kicks old session with same server_id

### 2. `e2e_publish_subscribe_test.go`  
Tests pub/sub flow:
- Basic publish and subscribe
- Envelopes delivered in correct order
- Different QoS levels (COMMAND, CONTROL, TELEMETRY)
- Multiple subscriptions per client
- ACK and AckSet operations
- Flow control hints
- Large batch delivery
- NACK handling

### 3. `e2e_multi_client_test.go`
Tests multiple clients:
- Simultaneous connections
- Different/overlapping subscriptions
- Targeted delivery to specific clients
- Independent ACKing
- Org isolation (multi-tenancy)
- Concurrent publishing
- Broadcast messages
- High concurrency scenarios

### 4. `e2e_reconnection_test.go`
Tests reconnection scenarios:
- Basic resume flow
- Unacked envelopes replayed after reconnect
- Partial ACK replay (only unacked messages)
- Resume token rotation
- Subscriptions restored after reconnect
- ACK positions preserved
- Multiple disconnect/reconnect cycles
- Rapid reconnects

### 5. `e2e_delivery_latency_test.go`
Tests delivery performance:
- Basic message latency measurement
- Average latency over multiple messages
- Immediate delivery notification
- Large batch performance
- Concurrent publishers
- Latency with reconnection
- P99 latency
- Maximum throughput

## Running Tests

### Prerequisites

- Docker (for MongoDB testcontainers)
- Go 1.25.5 or later

### Run All Tests

```bash
# From the e2e directory
go test -v ./...

# Or from the mycelium_spine root
go test -v ./e2e/...
```

### Run Specific Test File

```bash
go test -v -run TestSessionLifecycle ./...
go test -v -run TestPubSub ./...
go test -v -run TestMultiClient ./...
go test -v -run TestReconnection ./...
go test -v -run TestDeliveryLatency ./...
```

### Run Specific Test

```bash
go test -v -run TestSessionLifecycle_HelloAndWelcome
go test -v -run TestPubSub_BasicPublishAndSubscribe
```

### Skip Long-Running Tests

```bash
go test -v -short ./...
```

## Test Infrastructure

### TestEnv
The `TestEnv` struct provides a complete test environment:
- MongoDB container (via testcontainers)
- Running UMS server (without TLS for testing)
- Helper methods for creating clients and publishers

### Key Helper Functions

- `NewTestEnv(t)` - Creates a complete test environment
- `env.CreateClient(serverID, orgID)` - Creates an SDK client
- `env.CreateClientWithResume(serverID, orgID, resumeToken)` - Creates client with resume token
- `env.CreatePublisher()` - Creates an SDK publisher
- `WaitForDelivery(t, deliveries, timeout)` - Waits for message delivery
- `ExpectNoDelivery(t, deliveries, timeout)` - Verifies no delivery received
- `CreateServerTarget(serverID)` - Creates a SERVER target
- `CreateClusterTarget(clusterID)` - Creates a CLUSTER target
- `CreateBroadcastTarget()` - Creates a BROADCAST (ORG) target

### Cleanup

Tests automatically clean up resources (MongoDB containers, gRPC server) using `defer env.Cleanup()`.

## Architecture

```
┌─────────────────────┐
│   E2E Test          │
│                     │
├─────────────────────┤
│   TestEnv           │
│   - MongoDB         │
│   - UMS Server      │
└─────────────────────┘
         │
         │ Uses SDK
         ▼
┌─────────────────────┐
│   SDK Client        │
│   - Connect         │
│   - Subscribe       │
│   - Ack/Nack        │
└─────────────────────┘
         │
         │ gRPC (insecure)
         ▼
┌─────────────────────┐
│   UMS Server        │
│   - Stream Handler  │
│   - Publish Handler │
│   - Services        │
│   - Repository      │
└─────────────────────┘
         │
         ▼
┌─────────────────────┐
│   MongoDB           │
│   (testcontainer)   │
└─────────────────────┘
```

## Notes

- Tests use **insecure** gRPC connections (no TLS) for simplicity
- Each test gets a fresh MongoDB database with unique name
- Tests are designed to be independent and can run in any order
- MongoDB containers are automatically started and stopped
- Tests use real SDK clients, not mocks
- All network timeouts are configured for local testing

## Troubleshooting

### Tests Hang
- Check Docker is running and accessible
- Verify ports are not blocked by firewall
- Check MongoDB container logs

### Connection Refused
- Server may not have started in time, increase startup wait time in `startUMSServer`
- Check server logs in test output

### Flaky Tests
- Some tests may be timing-sensitive
- Increase timeouts if running on slow machine
- Tests with `t.Skip("Skipping...")` can be enabled for specific scenarios

## Future Improvements

- Add performance benchmarks
- Test TLS/mTLS connections
- Add chaos testing (network failures, crashes)
- Test with multiple UMS instances (clustering)
- Add tests for retention and cleanup
- Add tests for metrics collection
