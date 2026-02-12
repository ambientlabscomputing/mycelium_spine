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
	ErrNotConnected   = errors.New("client not connected")
	ErrSessionClosed  = errors.New("session closed")
	ErrInvalidConfig  = errors.New("invalid client configuration")
)

// ClientConfig configures a Client
type ClientConfig struct {
	ServerID          string
	OrgID             string
	ProtocolVersion   string
	TLSConfig         *tls.Config
	ResumeToken       string
	HeartbeatInterval time.Duration
}

// Client is a bidirectional streaming client for Mycelium Spine
type Client struct {
	addr   string
	config ClientConfig

	conn   *grpc.ClientConn
	client umsv1.SpineStreamClient
	stream umsv1.SpineStream_ConnectClient

	mu           sync.RWMutex
	sessionID    string
	connected    bool
	resumeToken  string
	
	deliveries chan *umsv1.DeliverFrame
	errors     chan error
	sendCh     chan *umsv1.ClientFrame
	
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewClient creates a new streaming client
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

// Connect establishes the streaming connection
func (c *Client) Connect(ctx context.Context) error {
	var grpcOpts []grpc.DialOption
	if c.config.TLSConfig != nil {
		grpcOpts = append(grpcOpts, grpc.WithTransportCredentials(credentials.NewTLS(c.config.TLSConfig)))
	} else {
		grpcOpts = append(grpcOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	conn, err := grpc.NewClient(c.addr, grpcOpts...)
	if err != nil {
		return fmt.Errorf("failed to dial: %w", err)
	}

	c.conn = conn
	c.client = umsv1.NewSpineStreamClient(conn)

	stream, err := c.client.Connect(c.ctx)
	if err != nil {
		return fmt.Errorf("failed to create stream: %w", err)
	}

	c.stream = stream

	// Send Hello with optional resume token
	helloFrame := &umsv1.ClientFrame{
		Frame: &umsv1.ClientFrame_Hello{
			Hello: &umsv1.HelloFrame{
				ProtocolVersion: c.config.ProtocolVersion,
				ServerId:        c.config.ServerID,
				OrgId:           c.config.OrgID,
				ResumeToken:     c.config.ResumeToken,
			},
		},
	}

	if err := stream.Send(helloFrame); err != nil {
		return fmt.Errorf("failed to send hello: %w", err)
	}

	// Wait for Welcome or ResumeOk
	frame, err := stream.Recv()
	if err != nil {
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
		return fmt.Errorf("server error: %s", f.Error.Message)
	default:
		return fmt.Errorf("unexpected frame type: %T", f)
	}

	// Start pumps
	c.wg.Add(3)
	go c.receivePump()
	go c.sendPump()
	go c.heartbeatPump()

	return nil
}

// receivePump receives frames from the server
func (c *Client) receivePump() {
	defer c.wg.Done()

	for {
		frame, err := c.stream.Recv()
		if err != nil {
			if err == io.EOF || c.ctx.Err() != nil {
				return
			}
			c.errors <- fmt.Errorf("recv error: %w", err)
			return
		}

		switch f := frame.Frame.(type) {
		case *umsv1.ServerFrame_Deliver:
			select {
			case c.deliveries <- f.Deliver:
			case <-c.ctx.Done():
				return
			}
		case *umsv1.ServerFrame_Pong:
			// Heartbeat response received
		case *umsv1.ServerFrame_Error:
			c.errors <- fmt.Errorf("server error: %s", f.Error.Message)
		}
	}
}

// sendPump sends frames to the server
func (c *Client) sendPump() {
	defer c.wg.Done()

	for {
		select {
		case frame := <-c.sendCh:
			if err := c.stream.Send(frame); err != nil {
				c.errors <- fmt.Errorf("send error: %w", err)
				return
			}
		case <-c.ctx.Done():
			return
		}
	}
}

// heartbeatPump sends periodic pings
func (c *Client) heartbeatPump() {
	defer c.wg.Done()

	ticker := time.NewTicker(c.config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := c.Ping(); err != nil {
				c.errors <- fmt.Errorf("ping error: %w", err)
			}
		case <-c.ctx.Done():
			return
		}
	}
}

// Subscribe subscribes to the specified targets
func (c *Client) Subscribe(targets []*umsv1.Target) error {
	c.mu.RLock()
	connected := c.connected
	c.mu.RUnlock()

	if !connected {
		return ErrNotConnected
	}

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

// Close closes the client connection
func (c *Client) Close() error {
	c.cancel()
	c.wg.Wait()

	close(c.deliveries)
	close(c.errors)
	close(c.sendCh)

	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}
