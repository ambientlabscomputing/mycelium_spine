# Running Underleaf Mycelium Spine (UMS) Locally

## Quick Start

### Prerequisites
- Go 1.24+ installed
- MongoDB running on localhost:27017 (credentials: admin/eventbus123)
- TLS certificates in `certs/` directory

### Start the Service

```bash
# From the mycelium_spine directory
export CONFIG_PATH="${PWD}/config.yaml"
go run cmd/serve/main.go
```

Or use the Makefile (if in correct shell):
```bash
make run
```

### Run in Background

```bash
export CONFIG_PATH="${PWD}/config.yaml"
nohup go run cmd/serve/main.go > logs/spine.log 2>&1 &
echo $! > /tmp/ums.pid
```

### Stop the Service

```bash
# If you saved the PID
kill $(cat /tmp/ums.pid)

# Or find and kill the process
pkill -f "cmd/serve/main.go"
```

### Check Service Health

```bash
# Check if process is running
pgrep -f "cmd/serve/main.go"

# Check if gRPC server is listening
lsof -i :9090 -P -n | grep LISTEN

# View logs
tail -f /tmp/ums.log
# or
tail -f logs/spine.log

# Check MongoDB collections
docker exec server-api-mongodb mongosh -u admin -p eventbus123 \
  --authenticationDatabase admin mycelium_spine_dev \
  --eval "db.getCollectionNames()"
```

### Test with grpcurl (if installed)

```bash
# List available services
grpcurl -insecure localhost:9090 list

# List methods for SpineStream service
grpcurl -insecure localhost:9090 list ums.v1.SpineStream

# Describe a service
grpcurl -insecure localhost:9090 describe ums.v1.SpineStream
```

## Configuration

Service configuration is in `config.yaml`. Key settings:

- **gRPC Port**: 9090
- **MongoDB**: localhost:27017 (admin/eventbus123)
- **Database**: mycelium_spine_dev
- **TLS**: Enabled with self-signed certificates (client_auth: none)
- **Log Level**: debug
- **Worker Pools**:
  - Session: 4 workers
  - Delivery: 8 workers
  - ACK: 4 workers
  - Mailbox: 4 workers

## Current Status

✅ **Service Running**: PID can be found with `pgrep -f "cmd/serve/main.go"`  
✅ **gRPC Server**: Listening on port 9090 with TLS  
✅ **MongoDB**: Connected to mycelium_spine_dev database  
✅ **Collections**: sessions, mailboxes, envelopes, cursors  
✅ **Worker Pools**: All 4 pools running (session, delivery, ack, mailbox)  

## Troubleshooting

### MongoDB Connection Issues
- Ensure MongoDB container is running: `docker ps | grep mongo`
- Verify credentials in config.yaml match container env vars
- Check MongoDB logs: `docker logs server-api-mongodb`

### TLS Certificate Issues
- Regenerate self-signed certs:
  ```bash
  openssl req -x509 -newkey rsa:4096 -keyout certs/spine.key \
    -out certs/spine.crt -days 365 -nodes \
    -subj "/CN=localhost/O=Underleaf/C=US"
  ```

### Port Already in Use
- Check what's using port 9090: `lsof -i :9090`
- Change port in config.yaml if needed

### View Real-time Logs
```bash
tail -f /tmp/ums.log | grep -E "(ERROR|WARN|INFO|ready)"
```
