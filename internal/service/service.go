package service

import (
	"context"
	"time"

	"github.com/ambientlabscomputing/mycelium_spine/internal/metrics"
	"github.com/ambientlabscomputing/mycelium_spine/internal/repository"
	"github.com/ambientlabscomputing/mycelium_spine/internal/types"
	"github.com/ambientlabscomputing/mycelium_spine/internal/utils"
	"github.com/ambientlabscomputing/mycelium_spine/internal/workers"
	umsv1 "github.com/ambientlabscomputing/mycelium_spine/proto/ums/v1"
)

// Service is the main interface for all UMS business logic
type Service interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	GetSessionService() SessionService
	GetDeliveryService() DeliveryService
	GetPublishService() PublishService
	GetAckService() AckService
	GetSubscribeService() SubscribeService
}

// SessionService handles session lifecycle and authentication
type SessionService interface {
	HandleHello(ctx context.Context, hello *umsv1.HelloFrame) (*umsv1.WelcomeFrame, *types.Session, error)
	HandleResume(ctx context.Context, resumeToken string) (*umsv1.ResumeOkFrame, *types.Session, error)
	HandleHeartbeat(ctx context.Context, sessionID string) error
	EvictStale(ctx context.Context) error
	GetSession(ctx context.Context, sessionID string) (*types.Session, error)
	RegisterSession(ctx context.Context, session *types.Session) error
	UnregisterSession(ctx context.Context, sessionID string) error
}

// DeliveryService manages envelope delivery to active sessions
type DeliveryService interface {
	StartDeliveryLoop(ctx context.Context, session *types.Session) error
	StopDeliveryLoop(ctx context.Context, sessionID string) error
	DeliverToSession(ctx context.Context, session *types.Session, envelopes []*types.Envelope) error
	HandleBackpressure(ctx context.Context, session *types.Session, hint *umsv1.FlowHintFrame) error
	NotifyDeliveryLoops() // Signal all delivery loops that new envelopes are available
}

// PublishService handles envelope publishing from external services
type PublishService interface {
	Publish(ctx context.Context, envelope *types.Envelope, targets []*types.Target) (*PublishResult, error)
}

// PublishResult contains the result of a publish operation
type PublishResult struct {
	Success     bool
	MailboxSeqs map[string]uint64 // mailbox_id → assigned seq
	Error       string
}

// AckService processes acknowledgements from clients
type AckService interface {
	// HandleCumulativeAck records that the client has processed all envelopes up to seqAcked.
	// serverID is the stable agent identity (not session_id) so progress survives reconnects.
	HandleCumulativeAck(ctx context.Context, serverID string, mailboxID string, seqAcked uint64) error
	HandleSelectiveAck(ctx context.Context, sessionID string, mailboxID string, seqs []uint64) error
	HandleNack(ctx context.Context, sessionID string, mailboxID string, seq uint64, reason string) error
}

// SubscribeService manages mailbox subscriptions
type SubscribeService interface {
	HandleSubscribe(ctx context.Context, session *types.Session, targets []*types.Target) (*umsv1.SubscribeOkFrame, error)
}

// AppService is the main implementation of Service
type AppService struct {
	repo             repository.Repository
	poolManager      *workers.PoolManager
	settings         *utils.Settings
	metrics          *metrics.Metrics
	metricsServer    *metrics.Server
	sessionService   SessionService
	deliveryService  DeliveryService
	publishService   PublishService
	ackService       AckService
	subscribeService SubscribeService
}

// NewAppService creates a new application service
func NewAppService(repo repository.Repository, poolManager *workers.PoolManager, settings *utils.Settings) *AppService {
	svc := &AppService{
		repo:        repo,
		poolManager: poolManager,
		settings:    settings,
		metrics:     metrics.NewMetrics(),
	}

	// Initialize sub-services
	svc.sessionService = NewSessionService(repo, settings, svc, svc.metrics)
	svc.deliveryService = NewDeliveryService(repo, poolManager, settings, svc.metrics)
	svc.publishService = NewPublishService(repo, svc.deliveryService, svc.metrics)
	svc.ackService = NewAckService(repo, svc.metrics)
	svc.subscribeService = NewSubscribeService(repo, svc.deliveryService, svc.metrics)

	return svc
}

// Start starts the application service and embedded servers
func (s *AppService) Start(ctx context.Context) error {
	// Start worker pools
	s.poolManager.Start(ctx)

	// Start metrics server if enabled\n\tif s.settings.Metrics.Enabled > 0 {\n\t\tmetricsServer := metrics.NewMetricsServer(int(s.settings.Metrics.Port), utils.Logger)\n\t\tif err := metricsServer.Start(); err != nil {\n\t\t\treturn err\n\t\t}\n\t\ts.metricsServer = metricsServer\n\t}

	// TODO: Start background tasks (heartbeat checker, retention janitor)

	return nil
}

// Stop stops the application service and embedded servers
func (s *AppService) Stop(ctx context.Context) error {
	// Stop worker pools
	s.poolManager.Stop()

	// Stop metrics server if running
	if s.metricsServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		s.metricsServer.Stop(ctx)
		cancel()
	}

	return nil
}

// GetSessionService returns the session service
func (s *AppService) GetSessionService() SessionService {
	return s.sessionService
}

// GetDeliveryService returns the delivery service
func (s *AppService) GetDeliveryService() DeliveryService {
	return s.deliveryService
}

// GetPublishService returns the publish service
func (s *AppService) GetPublishService() PublishService {
	return s.publishService
}

// GetAckService returns the ack service
func (s *AppService) GetAckService() AckService {
	return s.ackService
}

// GetSubscribeService returns the subscribe service
func (s *AppService) GetSubscribeService() SubscribeService {
	return s.subscribeService
}

// Legacy compatibility
func NewService() Service {
	// This is just a placeholder for backwards compatibility
	// Real initialization happens in NewAppService
	return nil
}
