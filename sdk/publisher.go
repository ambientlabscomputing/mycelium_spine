package sdk

import (
	"context"
	"crypto/tls"
	"fmt"

	umsv1 "github.com/ambientlabscomputing/mycelium_spine/proto/ums/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// Publisher is a client for publishing envelopes via SpinePublish service
type Publisher struct {
	conn   *grpc.ClientConn
	client umsv1.SpinePublishClient
}

// PublisherOption configures a Publisher
type PublisherOption func(*publisherOptions)

type publisherOptions struct {
	tlsConfig *tls.Config
}

// WithTLS configures TLS for the publisher
func WithTLS(config *tls.Config) PublisherOption {
	return func(o *publisherOptions) {
		o.tlsConfig = config
	}
}

// NewPublisher creates a new publisher client
func NewPublisher(addr string, opts ...PublisherOption) (*Publisher, error) {
	options := &publisherOptions{}
	for _, opt := range opts {
		opt(options)
	}

	var grpcOpts []grpc.DialOption
	if options.tlsConfig != nil {
		grpcOpts = append(grpcOpts, grpc.WithTransportCredentials(credentials.NewTLS(options.tlsConfig)))
	} else {
		grpcOpts = append(grpcOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	conn, err := grpc.NewClient(addr, grpcOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to dial: %w", err)
	}

	return &Publisher{
		conn:   conn,
		client: umsv1.NewSpinePublishClient(conn),
	}, nil
}

// Close closes the publisher connection
func (p *Publisher) Close() error {
	if p.conn != nil {
		return p.conn.Close()
	}
	return nil
}

// Publish publishes an envelope to the specified targets
func (p *Publisher) Publish(ctx context.Context, envelope *umsv1.Envelope, targets []*umsv1.Target) (*umsv1.PublishResponse, error) {
	// Fill in envelope defaults
	if envelope.EnvelopeId == "" {
		envelope.EnvelopeId = generateID()
	}
	if envelope.CreatedAtMs == 0 {
		envelope.CreatedAtMs = currentTimeMs()
	}

	req := &umsv1.PublishRequest{
		Envelope: envelope,
		Targets:  targets,
	}

	return p.client.Publish(ctx, req)
}

// PublishCommand is a helper for publishing COMMAND QoS messages
func (p *Publisher) PublishCommand(ctx context.Context, msgType string, payload []byte, orgID string, targets []*umsv1.Target) (*umsv1.PublishResponse, error) {
	envelope := &umsv1.Envelope{
		Type:        msgType,
		Qos:         umsv1.QoS_QOS_COMMAND,
		Payload:     payload,
		OrgId:       orgID,
		RequiresAck: true,
	}
	return p.Publish(ctx, envelope, targets)
}

// PublishControl is a helper for publishing CONTROL QoS messages
func (p *Publisher) PublishControl(ctx context.Context, msgType string, payload []byte, orgID string, targets []*umsv1.Target) (*umsv1.PublishResponse, error) {
	envelope := &umsv1.Envelope{
		Type:        msgType,
		Qos:         umsv1.QoS_QOS_CONTROL,
		Payload:     payload,
		OrgId:       orgID,
		RequiresAck: true,
	}
	return p.Publish(ctx, envelope, targets)
}

// PublishTelemetry is a helper for publishing TELEMETRY QoS messages
func (p *Publisher) PublishTelemetry(ctx context.Context, msgType string, payload []byte, orgID string, targets []*umsv1.Target) (*umsv1.PublishResponse, error) {
	envelope := &umsv1.Envelope{
		Type:        msgType,
		Qos:         umsv1.QoS_QOS_TELEMETRY,
		Payload:     payload,
		OrgId:       orgID,
		RequiresAck: false,
	}
	return p.Publish(ctx, envelope, targets)
}
