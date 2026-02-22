package e2e

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"testing"
	"time"

	myceliumgrpc "github.com/ambientlabscomputing/mycelium_spine/internal/grpc"
	"github.com/ambientlabscomputing/mycelium_spine/internal/repository"
	"github.com/ambientlabscomputing/mycelium_spine/internal/service"
	"github.com/ambientlabscomputing/mycelium_spine/internal/utils"
	"github.com/ambientlabscomputing/mycelium_spine/internal/workers"
	umsv1 "github.com/ambientlabscomputing/mycelium_spine/proto/ums/v1"
	"github.com/ambientlabscomputing/mycelium_spine/sdk"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// TestEnv encapsulates the test environment
type TestEnv struct {
	MongoContainer testcontainers.Container
	MongoURI       string
	MongoClient    *mongo.Client
	ServerAddr     string
	ServerShutdown func()
	Logger         *slog.Logger
	T              *testing.T
	Ctx            context.Context
}

// NewTestEnv creates a new test environment with MongoDB and UMS server
func NewTestEnv(t *testing.T) *TestEnv {
	ctx := context.Background()

	// Start MongoDB container
	mongoC, mongoURI := startMongoContainer(t, ctx)

	// Connect to MongoDB
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	require.NoError(t, err, "Failed to connect to MongoDB")

	// Wait for MongoDB to be ready
	err = client.Ping(ctx, nil)
	require.NoError(t, err, "Failed to ping MongoDB")

	// Create test database
	dbName := fmt.Sprintf("ums_e2e_test_%d", time.Now().UnixNano())
	db := client.Database(dbName)

	// Setup logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Start UMS server
	serverAddr, shutdown := startUMSServer(t, ctx, db, logger)

	// Wait for server to be ready
	time.Sleep(500 * time.Millisecond)

	return &TestEnv{
		MongoContainer: mongoC,
		MongoURI:       mongoURI,
		MongoClient:    client,
		ServerAddr:     serverAddr,
		ServerShutdown: shutdown,
		Logger:         logger,
		T:              t,
		Ctx:            ctx,
	}
}

// Cleanup tears down the test environment
func (e *TestEnv) Cleanup() {
	if e.ServerShutdown != nil {
		e.ServerShutdown()
	}
	if e.MongoClient != nil {
		e.MongoClient.Disconnect(e.Ctx)
	}
	if e.MongoContainer != nil {
		e.MongoContainer.Terminate(e.Ctx)
	}
}

// startMongoContainer starts a MongoDB container for testing
func startMongoContainer(t *testing.T, ctx context.Context) (testcontainers.Container, string) {
	req := testcontainers.ContainerRequest{
		Image:        "mongo:7",
		ExposedPorts: []string{"27017/tcp"},
		WaitingFor:   wait.ForLog("Waiting for connections"),
		Env: map[string]string{
			"MONGO_INITDB_ROOT_USERNAME": "admin",
			"MONGO_INITDB_ROOT_PASSWORD": "password123",
		},
	}

	mongoC, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err, "Failed to start MongoDB container")

	host, err := mongoC.Host(ctx)
	require.NoError(t, err, "Failed to get container host")

	port, err := mongoC.MappedPort(ctx, "27017")
	require.NoError(t, err, "Failed to get container port")

	mongoURI := fmt.Sprintf("mongodb://admin:password123@%s:%s", host, port.Port())
	return mongoC, mongoURI
}

// startUMSServer starts a UMS server for testing
func startUMSServer(t *testing.T, ctx context.Context, db *mongo.Database, logger *slog.Logger) (string, func()) {
	// Get a random available port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "Failed to get available port")
	addr := listener.Addr().String()

	// Initialize repository
	repo := repository.NewMongoRepository(db, logger)
	err = repo.CreateIndexes(ctx)
	require.NoError(t, err, "Failed to create indexes")

	// Create a simplified settings object
	settings := &utils.Settings{}
	settings.GRPC.Port = 0
	settings.Session.HeartbeatIntervalSeconds = 15
	settings.Session.HeartbeatTimeoutMultiplier = 3
	settings.Session.ResumeTokenTTLSeconds = 3600
	settings.Session.MaxSessionsPerServerID = 1
	settings.Workers.SessionPoolSize = 5
	settings.Workers.DeliveryPoolSize = 5
	settings.Workers.AckPoolSize = 5
	settings.Workers.MailboxPoolSize = 5

	// Create worker pool manager
	poolMgr := workers.NewPoolManager(settings, logger)

	// Create app service
	appService := service.NewAppService(repo, poolMgr, settings)
	err = appService.Start(ctx)
	require.NoError(t, err, "Failed to start app service")

	// Create gRPC server without TLS for testing
	grpcServer := grpc.NewServer(
		grpc.Creds(insecure.NewCredentials()),
	)

	// Create handlers
	streamHandler := myceliumgrpc.NewStreamHandler(appService)
	publishHandler := myceliumgrpc.NewPublishHandler(appService)

	// Register services
	umsv1.RegisterSpineStreamServer(grpcServer, streamHandler)
	umsv1.RegisterSpinePublishServer(grpcServer, publishHandler)

	// Start server in background
	errCh := make(chan error, 1)
	go func() {
		if err := grpcServer.Serve(listener); err != nil {
			errCh <- err
		}
	}()

	// Check for immediate startup errors
	select {
	case err := <-errCh:
		t.Fatalf("Server failed to start: %v", err)
	case <-time.After(200 * time.Millisecond):
		// Server started successfully
	}

	shutdown := func() {
		grpcServer.GracefulStop()
		appService.Stop(context.Background())
	}

	return addr, shutdown
}

// CreateClient creates a new SDK client for testing
func (e *TestEnv) CreateClient(serverID, orgID string) *sdk.Client {
	client, err := sdk.NewClient(e.ServerAddr, sdk.ClientConfig{
		ServerID:          serverID,
		OrgID:             orgID,
		ProtocolVersion:   "1.0",
		HeartbeatInterval: 5 * time.Second,
	})
	require.NoError(e.T, err, "Failed to create client")
	return client
}

// CreateClientWithResume creates a client with resume token
func (e *TestEnv) CreateClientWithResume(serverID, orgID, resumeToken string) *sdk.Client {
	client, err := sdk.NewClient(e.ServerAddr, sdk.ClientConfig{
		ServerID:          serverID,
		OrgID:             orgID,
		ProtocolVersion:   "1.0",
		ResumeToken:       resumeToken,
		HeartbeatInterval: 5 * time.Second,
	})
	require.NoError(e.T, err, "Failed to create client")
	return client
}

// CreatePublisher creates a new SDK publisher for testing
func (e *TestEnv) CreatePublisher() *sdk.Publisher {
	pub, err := sdk.NewPublisher(e.ServerAddr)
	require.NoError(e.T, err, "Failed to create publisher")
	return pub
}

// WaitForDelivery waits for a delivery or times out
func WaitForDelivery(t *testing.T, deliveries <-chan *umsv1.DeliverFrame, timeout time.Duration) *umsv1.DeliverFrame {
	select {
	case delivery := <-deliveries:
		return delivery
	case <-time.After(timeout):
		t.Fatal("Timeout waiting for delivery")
		return nil
	}
}

// ExpectNoDelivery verifies no delivery is received within timeout
func ExpectNoDelivery(t *testing.T, deliveries <-chan *umsv1.DeliverFrame, timeout time.Duration) {
	select {
	case delivery := <-deliveries:
		t.Fatalf("Unexpected delivery received: %+v", delivery)
	case <-time.After(timeout):
		// Expected - no delivery
	}
}

// CreateServerTarget creates a SERVER type target
func CreateServerTarget(serverID string) *umsv1.Target {
	return &umsv1.Target{
		TargetType: umsv1.TargetType_TARGET_TYPE_SERVER,
		TargetId:   serverID,
	}
}

// CreateClusterTarget creates a CLUSTER type target
func CreateClusterTarget(clusterID string) *umsv1.Target {
	return &umsv1.Target{
		TargetType: umsv1.TargetType_TARGET_TYPE_CLUSTER,
		TargetId:   clusterID,
	}
}

// CreateBroadcastTarget creates a BROADCAST type target
func CreateBroadcastTarget() *umsv1.Target {
	return &umsv1.Target{
		TargetType: umsv1.TargetType_TARGET_TYPE_ORG,
	}
}
