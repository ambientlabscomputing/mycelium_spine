# Mycelium Spine SDK

Go SDK for interacting with the Mycelium Spine Universal Messaging Service (UMS).

## Features

- **Session Management**: Connect, authenticate, and manage bidirectional streaming sessions
- **Message Publishing**: Publish envelopes to target mailboxes with QoS guarantees
- **Message Delivery**: Receive and process delivered messages
- **Acknowledgments**: Send cumulative or selective acknowledgments
- **Subscriptions**: Subscribe to mailboxes by target (server, cluster, org, service, broadcast)
- **Heartbeat Management**: Automatic ping/pong for connection health
- **Session Resumption**: Resume interrupted sessions with state recovery

## Installation

```bash
go get github.com/ambientlabscomputing/mycelium_spine/sdk
```

## Quick Start

### Connect and Subscribe

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
    client, err := sdk.NewClient("localhost:9090", sdk.ClientConfig{
        ServerID:          "my-server-001",
        OrgID:            "org-123",
        ProtocolVersion:  "1.0",
    })
    if err != nil {
        log.Fatalf("Failed to create client: %v", err)
    }
    defer client.Close()
    
    // Connect and establish session
    ctx := context.Background()
    if err := client.Connect(ctx); err != nil {
        log.Fatalf("Failed to connect: %v", err)
    }
    
    // Subscribe to server target
    targets := []*umsv1.Target{
        {
            TargetType: umsv1.TargetType_TARGET_TYPE_SERVER,
            TargetId:   "my-server-001",
            OrgId:      "org-123",
        },
    }
    
    if err := client.Subscribe(targets); err != nil {
        log.Fatalf("Failed to subscribe: %v", err)
    }
    
    // Handle delivered messages
    for delivery := range client.Deliveries() {
        for _, envelope := range delivery.Envelopes {
            log.Printf("Received: type=%s seq=%d", envelope.Type, envelope.Seq)
            
            // Process message...
            
            // Acknowledge
            if err := client.Ack(envelope.MailboxId, envelope.Seq); err != nil {
                log.Printf("Failed to ack: %v", err)
            }
        }
    }
}
```

### Publish Messages

```go
package main

import (
    "context"
    "log"
    
    "github.com/ambientlabscomputing/mycelium_spine/sdk"
    umsv1 "github.com/ambientlabscomputing/mycelium_spine/proto/ums/v1"
)

func main() {
    // Create publisher client
    publisher, err := sdk.NewPublisher("localhost:9090")
    if err != nil {
        log.Fatalf("Failed to create publisher: %v", err)
    }
    defer publisher.Close()
    
    ctx := context.Background()
    
    // Publish command envelope
    envelope := &umsv1.Envelope{
        Type:    "command.deploy",
        Qos:     umsv1.QoS_QOS_COMMAND,
        Payload: []byte(`{"service":"api","version":"v2.1.0"}`),
        OrgId:   "org-123",
    }
    
    targets := []*umsv1.Target{
        {
            TargetType: umsv1.TargetType_TARGET_TYPE_SERVER,
            TargetId:   "prod-server-01",
            OrgId:      "org-123",
        },
    }
    
    resp, err := publisher.Publish(ctx, envelope, targets)
    if err != nil {
        log.Fatalf("Failed to publish: %v", err)
    }
    
    log.Printf("Published to mailboxes: %v", resp.MailboxSeqs)
}
```

## Architecture

The SDK provides two main client types:

1. **Client** (`sdk.Client`): Bidirectional streaming client for receiving messages
   - Maintains persistent connection with automatic reconnect
   - Handles session lifecycle (connect, resume, heartbeat)
   - Processes incoming deliveries
   - Sends acknowledgments

2. **Publisher** (`sdk.Publisher`): Unary RPC client for publishing messages
   - Used by control planes (UCRS, Server API) to push envelopes
   - Supports batch publishing to multiple targets
   - Returns assigned sequence numbers per mailbox

## API Reference

### Client Configuration

```go
type ClientConfig struct {
    ServerID           string            // Stable Underleaf server identity
    OrgID             string            // Organization ID
    ProtocolVersion   string            // Protocol version (default: "1.0")
    DeviceFingerprint string            // TPM/SW attestation (optional)
    ResumeToken       string            // For session resumption (optional)
    ClientFeatures    map[string]bool   // Feature flags (optional)
    TLSConfig         *tls.Config       // TLS configuration (optional)
}
```

### Client Methods

- `Connect(ctx) error` - Establish bidirectional stream and send Hello
- `Subscribe(targets) error` - Subscribe to mailboxes
- `Ack(mailboxID, seq) error` - Cumulative acknowledgment
- `AckSet(mailboxID, seqs) error` - Selective acknowledgment
- `Nack(mailboxID, seq, reason) error` - Negative acknowledgment
- `Ping() error` - Send heartbeat ping
- `FlowHint(hint) error` - Send backpressure hint
- `Deliveries() <-chan *DeliverFrame` - Channel of delivered messages
- `Errors() <-chan error` - Channel of error events
- `Close() error` - Gracefully close connection

### Publisher Methods

- `Publish(ctx, envelope, targets) (*PublishResponse, error)` - Publish envelope to targets
- `Close() error` - Close publisher connection

## Error Handling

The SDK provides structured error types for common failure scenarios:

- `ErrNotConnected` - Client not connected
- `ErrSessionClosed` - Session was closed by server
- `ErrInvalidFrame` - Received invalid frame
- `ErrResumeD Denied` - Session resume rejected

## Testing

Run SDK tests:

```bash
go test ./...
```

## License

Copyright © 2026 Ambient Labs Computing
