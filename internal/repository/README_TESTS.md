# UMS MongoDB Repository Integration Tests

This directory contains comprehensive integration tests for the UMS MongoDB repository layer using testcontainers-go for real MongoDB testing.

## Test Files

### 1. session_repository_test.go
Tests for session CRUD operations and cursor management:
- **CreateSession** - Create new sessions
- **GetSession** - Retrieve sessions by ID
- **GetSessionByServerID** - Find session by server_id (highest epoch)
- **GetSessionByResumeToken** - Retrieve session by resume token
- **UpdateResumeToken** - Token rotation
- **UpdateHeartbeat** - Heartbeat timestamp updates
- **UpdateSubscriptions** - Manage session subscriptions
- **DeleteSession** - Session and cursor cleanup
- **UpdateAckPosition** - Update mailbox acknowledgment positions
- **GetAckPositions** - Retrieve all ack positions for a session
- **SessionIndexes** - Verify MongoDB indexes and unique constraints

### 2. mailbox_repository_test.go
Tests for mailbox and envelope operations:
- **GetOrCreateMailbox** - Mailbox creation and retrieval (idempotent)
- **GetMailbox** - Retrieve mailbox by ID
- **AppendEnvelope** - Atomic sequence allocation and envelope insertion
- **AppendEnvelopeSequenceAllocation** - Sequential sequence number allocation
- **FetchEnvelopes** - Query envelopes with pagination and filtering
- **FetchEnvelopesPagination** - Large dataset pagination
- **DeleteExpired** - Remove expired envelopes
- **EnforceRetention** - Retention policy enforcement (time and count based)
- **ConcurrentAppend** - Concurrent envelope appends (race condition testing)
- **MailboxIndexes** - Verify MongoDB indexes and unique constraints
- **NonAtomicSequenceIssue** - Document known limitation (non-atomic sequence allocation)

### 3. repository_integration_test.go
Complex cross-entity integration scenarios:
- **SessionWithSubscriptionsAndAckPositions** - Complete session lifecycle with subscriptions
- **MultipleMailboxesWithEnvelopes** - Multi-mailbox envelope management
- **SessionResumeScenario** - Session resumption with token rotation
- **LargeDatasetPagination** - Pagination with 1000+ envelopes
- **ConcurrentSessionsAndEnvelopes** - Concurrent multi-session operations
- **TargetResolution** - Resolve targets to mailbox IDs (SERVER, ORG, SERVICE)
- **TargetResolutionCluster** - Cluster target resolution
- **TargetResolutionBroadcast** - Broadcast target resolution
- **EnvelopeDeliveryFlow** - End-to-end envelope delivery workflow
- **RetentionAndCleanup** - Retention policy enforcement across mailboxes
- **MultiTenancy** - Verify tenant isolation (org_id separation)

## Prerequisites

### Docker
The tests use testcontainers-go which requires Docker to be running:
```bash
# Check if Docker is running
docker info
```

### Go Dependencies
All dependencies are managed via go.mod:
```bash
go mod download
```

## Running Tests

### Run All Integration Tests
```bash
cd mycelium_spine/internal/repository
go test -v -timeout 10m
```

### Run Specific Test Suite
```bash
# Session repository tests only
go test -v -run TestSessionRepository

# Mailbox repository tests only
go test -v -run TestMailboxRepository

# Integration tests only
go test -v -run TestRepositoryIntegration
```

### Run Specific Test Case
```bash
# Run a specific sub-test
go test -v -run TestSessionRepository/CreateSession
go test -v -run TestMailboxRepository/ConcurrentAppend
go test -v -run TestRepositoryIntegration/EnvelopeDeliveryFlow
```

### Skip Integration Tests (Short Mode)
Use `-short` flag to skip integration tests that require Docker:
```bash
go test -short
```

### Run with Race Detector
```bash
go test -race -timeout 15m
```

## Test Coverage

### What is Tested

#### MongoDB Features
- ✅ Unique indexes and constraints
- ✅ Compound indexes (multi-field)
- ✅ Query operations (find, findOne, findOneAndUpdate)
- ✅ Atomic updates ($inc, $set, $setOnInsert)
- ✅ Upsert operations
- ✅ Sort and pagination
- ✅ Delete operations (single and batch)

#### Repository Operations
- ✅ CRUD operations for sessions and mailboxes
- ✅ Sequence number allocation (atomic counter)
- ✅ Cursor tracking (ack positions)
- ✅ Target resolution (different target types)
- ✅ Retention policy enforcement
- ✅ Concurrent operations (race conditions)
- ✅ Large dataset handling (1000+ documents)
- ✅ Multi-tenancy isolation

#### Known Limitations
- ⚠️ **Non-atomic sequence allocation**: Documented in `testNonAtomicSequenceIssue`
  - Sequence allocation and envelope insertion are separate operations
  - If insertion fails after sequence allocation, a gap occurs
  - Violates total order guarantee (spec §12)
  - TODO: Fix in Phase 4 with MongoDB transactions

### What is Not Tested
- ❌ Network failures and reconnection
- ❌ MongoDB replica set failover
- ❌ Very large payloads (>16MB BSON limit)
- ❌ Index performance under heavy load
- ❌ TTL index behavior (currently disabled, see mongo_repository.go)

## Test Infrastructure

### TestContainers Setup
Each test suite:
1. Starts a MongoDB 7.0 container
2. Connects to the container
3. Creates a fresh database
4. Creates all indexes
5. Runs tests
6. Tears down the container

### Isolation
- Each test suite uses a separate database
- Tests within a suite share the database but use unique IDs
- No test cleanup between sub-tests (intentional for debugging)

### Timeouts
- Individual tests: Fast (< 1 second)
- Full suite: ~30 seconds (includes container startup)
- Use `-timeout` flag to adjust if needed

## Troubleshooting

### Docker Not Running
```
Error: Cannot connect to the Docker daemon
```
**Solution**: Start Docker Desktop or Docker daemon

### Port Conflicts
```
Error: Bind for 0.0.0.0:27017 failed: port is already allocated
```
**Solution**: Stop local MongoDB or change testcontainer port mapping

### Container Cleanup Issues
```bash
# List dangling containers
docker ps -a | grep mongo

# Clean up
docker rm -f $(docker ps -aq -f "ancestor=mongo:7.0")
```

### Slow Tests
If tests are slow:
1. Check Docker resources (CPU/Memory in Docker Desktop)
2. Use `-short` to skip integration tests during development
3. Run specific test suites instead of all tests

## Alternative: Local MongoDB

If testcontainers-go is not available, you can run tests against a local MongoDB:

### Setup
```bash
# Start MongoDB with docker-compose
cd mycelium_spine
docker-compose up -d mongodb

# Or use local MongoDB
brew install mongodb-community
brew services start mongodb-community
```

### Modify Tests
In each test file, replace `startMongoContainer` with a fixed URI:
```go
mongoURI := "mongodb://localhost:27017"
// Skip container creation and cleanup
```

## CI/CD Integration

### GitHub Actions Example
```yaml
- name: Run Repository Tests
  run: |
    cd mycelium_spine/internal/repository
    go test -v -timeout 10m -coverprofile=coverage.out
    
- name: Upload Coverage
  uses: codecov/codecov-action@v3
  with:
    files: ./coverage.out
```

### Docker-in-Docker
For CI environments that support Docker-in-Docker:
```yaml
services:
  docker:
    image: docker:dind
    privileged: true
```

## Contributing

When adding new repository methods:
1. Add tests to the appropriate test file
2. Follow existing naming conventions (`test<MethodName>`)
3. Test both success and error cases
4. Test edge cases (empty results, duplicates, etc.)
5. Run tests with `-race` flag
6. Update this README if needed

## Performance Notes

### Test Execution Times (Approximate)
- Session tests: ~8-10 seconds
- Mailbox tests: ~10-12 seconds (includes concurrent tests)
- Integration tests: ~12-15 seconds (includes large datasets)
- Total: ~30-40 seconds (with container startup)

### Resource Usage
- MongoDB container: ~100MB memory
- Test process: ~50MB memory
- Disk: Minimal (in-memory container)

## Further Reading

- [testcontainers-go Documentation](https://golang.testcontainers.org/)
- [MongoDB Go Driver](https://www.mongodb.com/docs/drivers/go/current/)
- [UMS Specification](../../docs/SPEC.md)
- [Repository Design](../../docs/ARCHITECTURE.md)
