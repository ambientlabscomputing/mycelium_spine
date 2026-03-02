package sdk

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	umsv1 "github.com/ambientlabscomputing/mycelium_spine/proto/ums/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// Errors
var (
	ErrNotConnected  = errors.New("client not connected")
	ErrSessionClosed = errors.New("session closed")
	ErrInvalidConfig = errors.New("invalid client configuration")
)

// ClientConfig configures a Client.
type ClientConfig struct {
	ServerID          string
	OrgID             string
	ProtocolVersion   string
	TLSConfig         *tls.Config
	HeaderAuthConfig  *HeaderAuthCredentials // Used for mTLS over HTTP headers (e.g. Cloudflare tunnel)
	ResumeToken       string
	HeartbeatInterval time.Duration

	// AutoReconnect enables automatic reconnection with exponential backoff.
	AutoReconnect bool
	// InitialReconnectDelay is the starting backoff delay (default 1s).
	InitialReconnectDelay time.Duration
	// MaxReconnectDelay caps the backoff delay (default 30s).
	MaxReconnectDelay time.Duration
	// OnReconnect is called after a successful reconnection, if set.
	OnReconnect func()
}

// Client is a bidirectional streaming client for Mycelium Spine.
// When ClientConfig.AutoReconnect is true the client transparently re-establishes
// the gRPC stream on disconnection and re-issues any registered subscriptions.
type Client struct {
	addr   string
	config ClientConfig

	// gRPC handles — replaced atomically on each reconnect (protected by connMu).
	connMu sync.Mutex
	conn   *grpc.ClientConn
	client umsv1.SpineStreamClient
	stream umsv1.SpineStream_ConnectClient

	// Session state (protected by mu).
	mu          sync.RWMutex
	sessionID   string
	connected   bool
	resumeToken string

	// Last subscribed targets — restored automatically after reconnect.
	targetsMu   sync.Mutex
	lastTargets []*umsv1.Target

	// Outbound / inbound channels (long-lived, never closed until Close()).
	deliveries chan *umsv1.DeliverFrame
	errors     chan error
	sendCh     chan *umsv1.ClientFrame

	// Client lifetime (cancelled by Close()).
	ctx    context.Context
	cancel context.CancelFunc
	// wg tracks the supervisor goroutine (started in Connect, exits when ctx done).
	wg sync.WaitGroup

	// Per-connection cancel — cancels receivePump/sendPump/heartbeatPump.
	connCtx    context.Context
	connCancel context.CancelFunc
	// connWg tracks the three pump goroutines for the active connection.
	connWg sync.WaitGroup
}

// NewClient creates a new streaming client.
func NewClient(addr string, config ClientConfig) (*Client, error) {
	if config.ServerID == "" || config.OrgID == "" {
		return nil, ErrInvalidConfig
	}
	if config.ProtocolVersion == "" {
		config.ProtocolVersion = "1.0"
	}
	if config.HeartbeatInterval == 0 {
		config.HeartbeatInterval = 30 * time.Second
	}
	if config.InitialReconnectDelay == 0 {
		config.InitialReconnectDelay = time.Second
	}
	if config.MaxReconnectDelay == 0 {
		config.MaxReconnectDelay = 30 * time.Second
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Client{
		addr:       addr,
		config:     config,
		deliveries: make(chan *umsv1.DeliverFrame, 100),
		errors:     make(chan error, 10),
		sendCh:     make(chan *umsv1.ClientFrame, 100),
		ctx:        ctx,
		cancel:     cancel,
	}, nil
}

// Connect establishes the streaming connection and starts background goroutines.
// If AutoReconnect is enabled a supervisor goroutine is started that retries the
// connection with exponential backoff whenever it is lost.
func (c *Client) Connect(ctx context.Context) error {
	if err := c.doConnect(ctx); err != nil {
		return err
	}
	c.startPumps()

	if c.config.AutoReconnect {
		c.wg.Add(1)
		go c.supervisor()
	}
	return nil
}

// doConnect dials gRPC, performs the Hello/Welcome handshake, and updates the
// connection fields.  It does NOT start the pump goroutines.
func (c *Client) doConnect(ctx context.Context) error {
	var grpcOpts []grpc.DialOption
	if c.config.TLSConfig != nil {
		grpcOpts = append(grpcOpts, grpc.WithTransportCredentials(credentials.NewTLS(c.config.TLSConfig)))
	} else {
		grpcOpts = append(grpcOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	if c.config.HeaderAuthConfig != nil {
		grpcOpts = append(grpcOpts, grpc.WithStreamInterceptor(StreamHeaderAuthInterceptor(c.config.HeaderAuthConfig)))
	}

	conn, err := grpc.NewClient(c.addr, grpcOpts...)
	if err != nil {
		return fmt.Errorf("failed to dial: %w", err)
	}

	grpcClient := umsv1.NewSpineStreamClient(conn)
	stream, err := grpcClient.Connect(c.ctx)
	if err != nil {
		conn.Close()
		return fmt.Errorf("failed to create stream: %w", err)
	}

	// Determine resume token to use (prefer session-local token, fall back to config).
	c.mu.RLock()
	resumeToken := c.resumeToken
	c.mu.RUnlock()
	if resumeToken == "" {
		resumeToken = c.config.ResumeToken
	}

	hello := &umsv1.ClientFrame{
		Frame: &umsv1.ClientFrame_Hello{
			Hello: &umsv1.HelloFrame{
				ProtocolVersion: c.config.ProtocolVersion,
				ServerId:        c.config.ServerID,
				OrgId:           c.config.OrgID,
				ResumeToken:     resumeToken,
			},
		},
	}
	if err := stream.Send(hello); err != nil {
		conn.Close()
		return fmt.Errorf("failed to send hello: %w", err)
	}

	frame, err := stream.Recv()
	if err != nil {
		conn.Close()
		return fmt.Errorf("failed to receive welcome: %w", err)
	}

	switch f := frame.Frame.(type) {
	case *umsv1.ServerFrame_Welcome:
		c.mu.Lock()
		c.sessionID = f.Welcome.SessionId
		c.resumeToken = f.Welcome.ResumeToken
		c.connected = true
		c.mu.Unlock()
	case *umsv1.ServerFrame_ResumeOk:
		c.mu.Lock()
		c.sessionID = f.ResumeOk.SessionId
		c.connected = true
		c.mu.Unlock()
	case *umsv1.ServerFrame_Error:
		conn.Close()
		return fmt.Errorf("server error: %s", f.Error.Message)
	default:
		conn.Close()
		return fmt.Errorf("unexpected frame type: %T", f)
	}

	c.connMu.Lock()
	if c.conn != nil {
		c.conn.Close() // close previous connection if any
	}
	c.conn = conn
	c.client = grpcClient
	c.stream = stream
	c.connMu.Unlock()

	return nil
}

// startPumps creates a new per-connection context and launches the three pump goroutines.
func (c *Client) startPumps() {
	connCtx, connCancel := context.WithCancel(c.ctx)
	c.connCtx = connCtx
	c.connCancel = connCancel

	c.connWg.Add(3)
	go c.receivePump(connCtx)
	go c.sendPump(connCtx)
	go c.heartbeatPump(connCtx)
}

// stopPumps cancels the per-connection context and waits for pump goroutines to exit.
func (c *Client) stopPumps() {
	if c.connCancel != nil {
		c.connCancel()
	}
	c.connWg.Wait()
}

// supervisor runs when AutoReconnect is enabled. It waits for the pump goroutines
// to exit (indicating a disconnection) and then retries the connection with
// exponential backoff until it succeeds or the client is closed.
func (c *Client) supervisor() {
	defer c.wg.Done()

	for {
		// Wait for the current set of pumps to stop.
		c.connWg.Wait()

		// If the client is being closed, exit.
		if c.ctx.Err() != nil {
			return
		}

		// Mark as disconnected.
		c.mu.Lock()
		c.connected = false
		c.mu.Unlock()

		// Exponential backoff reconnection loop.
		delay := c.config.InitialReconnectDelay
		for {
			if c.ctx.Err() != nil {
				return
			}

			select {
			case <-time.After(delay):
			case <-c.ctx.Done():
				return
			}

			if err := c.doConnect(c.ctx); err != nil {
				// Drain errors channel if full to avoid blocking.
				select {
				case c.errors <- fmt.Errorf("reconnect attempt failed: %w", err):
				default:
				}
				delay = min(delay*2, c.config.MaxReconnectDelay)
				continue
			}

			// Re-subscribe with last known targets.
			c.targetsMu.Lock()
			targets := c.lastTargets
			c.targetsMu.Unlock()

			if len(targets) > 0 {
				_ = c.sendSubscribe(targets) // best-effort; pumps will retry if needed
			}

			c.startPumps()

			if c.config.OnReconnect != nil {
				c.config.OnReconnect()
			}
			break
		}
	}
}

// min returns the smaller of two durations.
func min(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

// receivePump receives frames from the server until the per-connection context is cancelled.
func (c *Client) receivePump(ctx context.Context) {
	defer c.connWg.Done()

	for {
		c.connMu.Lock()
		stream := c.stream
		c.connMu.Unlock()

		frame, err := stream.Recv()
		if err != nil {
			if err == io.EOF || ctx.Err() != nil || c.ctx.Err() != nil {
				return
			}
			select {
			case c.errors <- fmt.Errorf("recv error: %w", err):
			default:
			}
			return
		}

		switch f := frame.Frame.(type) {
		case *umsv1.ServerFrame_Deliver:
			select {
			case c.deliveries <- f.Deliver:
			case <-ctx.Done():
				return
			}
		case *umsv1.ServerFrame_Pong:
			// Heartbeat response received — no action needed.
		case *umsv1.ServerFrame_Error:
			select {
			case c.errors <- fmt.Errorf("server error: %s", f.Error.Message):
			default:
			}
		}
	}
}

// sendPump drains sendCh and writes frames to the server stream.
func (c *Client) sendPump(ctx context.Context) {
	defer c.connWg.Done()

	for {
		select {
		case frame := <-c.sendCh:
			c.connMu.Lock()
			stream := c.stream
			c.connMu.Unlock()

			if err := stream.Send(frame); err != nil {
				select {
				case c.errors <- fmt.Errorf("send error: %w", err):
				default:
				}
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

// heartbeatPump sends periodic pings.
func (c *Client) heartbeatPump(ctx context.Context) {
	defer c.connWg.Done()

	ticker := time.NewTicker(c.config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := c.Ping(); err != nil {
				// Non-fatal; supervisor will handle reconnection if the stream is dead.
				select {
				case c.errors <- fmt.Errorf("ping error: %w", err):
				default:
				}
			}
		case <-ctx.Done():
			return
		}
	}
}

// Subscribe subscribes to the specified targets. The targets are remembered so
// they can be automatically re-issued after a reconnect when AutoReconnect is enabled.
func (c *Client) Subscribe(targets []*umsv1.Target) error {
	c.mu.RLock()
	connected := c.connected
	c.mu.RUnlock()

	if !connected {
		return ErrNotConnected
	}

	// Persist targets for reconnect.
	c.targetsMu.Lock()
	c.lastTargets = targets
	c.targetsMu.Unlock()

	return c.sendSubscribe(targets)
}

// sendSubscribe sends a SubscribeFrame directly without updating lastTargets.
func (c *Client) sendSubscribe(targets []*umsv1.Target) error {
	frame := &umsv1.ClientFrame{
		Frame: &umsv1.ClientFrame_Subscribe{
			Subscribe: &umsv1.SubscribeFrame{
				Targets: targets,
			},
		},
	}

	select {
	case c.sendCh <- frame:
		return nil
	case <-c.ctx.Done():
		return ErrSessionClosed
	}
}

// Ack sends a cumulative acknowledgment
func (c *Client) Ack(mailboxID string, seq uint64) error {
	c.mu.RLock()
	connected := c.connected
	c.mu.RUnlock()

	if !connected {
		return ErrNotConnected
	}

	frame := &umsv1.ClientFrame{
		Frame: &umsv1.ClientFrame_Ack{
			Ack: &umsv1.AckFrame{
				MailboxId: mailboxID,
				SeqAcked:  seq,
			},
		},
	}

	select {
	case c.sendCh <- frame:
		return nil
	case <-c.ctx.Done():
		return ErrSessionClosed
	}
}

// AckSet acknowledges multiple sequence numbers
func (c *Client) AckSet(mailboxID string, seqs []uint64) error {
	c.mu.RLock()
	connected := c.connected
	c.mu.RUnlock()

	if !connected {
		return ErrNotConnected
	}

	frame := &umsv1.ClientFrame{
		Frame: &umsv1.ClientFrame_AckSet{
			AckSet: &umsv1.AckSetFrame{
				MailboxId: mailboxID,
				Seqs:      seqs,
			},
		},
	}

	select {
	case c.sendCh <- frame:
		return nil
	case <-c.ctx.Done():
		return ErrSessionClosed
	}
}

// Nack sends a negative acknowledgment
func (c *Client) Nack(mailboxID string, seq uint64, reason string) error {
	c.mu.RLock()
	connected := c.connected
	c.mu.RUnlock()

	if !connected {
		return ErrNotConnected
	}

	frame := &umsv1.ClientFrame{
		Frame: &umsv1.ClientFrame_Nack{
			Nack: &umsv1.NackFrame{
				MailboxId: mailboxID,
				Seq:       seq,
				Reason:    reason,
			},
		},
	}

	select {
	case c.sendCh <- frame:
		return nil
	case <-c.ctx.Done():
		return ErrSessionClosed
	}
}

// Ping sends a heartbeat ping
func (c *Client) Ping() error {
	c.mu.RLock()
	connected := c.connected
	c.mu.RUnlock()

	if !connected {
		return ErrNotConnected
	}

	frame := &umsv1.ClientFrame{
		Frame: &umsv1.ClientFrame_Ping{
			Ping: &umsv1.PingFrame{
				ClientTimeMs: currentTimeMs(),
			},
		},
	}

	select {
	case c.sendCh <- frame:
		return nil
	case <-c.ctx.Done():
		return ErrSessionClosed
	}
}

// FlowHint sends a flow control hint
func (c *Client) FlowHint(ready bool) error {
	c.mu.RLock()
	connected := c.connected
	c.mu.RUnlock()

	if !connected {
		return ErrNotConnected
	}

	hint := "ready"
	if !ready {
		hint = "overloaded"
	}

	frame := &umsv1.ClientFrame{
		Frame: &umsv1.ClientFrame_FlowHint{
			FlowHint: &umsv1.FlowHintFrame{
				Hint: hint,
			},
		},
	}

	select {
	case c.sendCh <- frame:
		return nil
	case <-c.ctx.Done():
		return ErrSessionClosed
	}
}

// SessionID returns the current session ID
func (c *Client) SessionID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.sessionID
}

// ResumeToken returns the current resume token
func (c *Client) ResumeToken() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.resumeToken
}

// Deliveries returns the channel for receiving delivered envelopes
func (c *Client) Deliveries() <-chan *umsv1.DeliverFrame {
	return c.deliveries
}

// Errors returns the channel for receiving errors
func (c *Client) Errors() <-chan error {
	return c.errors
}

// Close closes the client connection and stops all background goroutines.
func (c *Client) Close() error {
	// Cancel the client lifetime context — stops the supervisor and all pumps.
	c.cancel()

	// Wait for pump goroutines (connWg) then the supervisor (wg).
	c.connWg.Wait()
	c.wg.Wait()

	close(c.deliveries)
	close(c.errors)
	close(c.sendCh)

	c.connMu.Lock()
	conn := c.conn
	c.connMu.Unlock()

	if conn != nil {
		return conn.Close()
	}
	return nil
}
