package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ambientlabscomputing/mycelium_spine/internal/grpc"
	"github.com/ambientlabscomputing/mycelium_spine/internal/repository"
	"github.com/ambientlabscomputing/mycelium_spine/internal/service"
	"github.com/ambientlabscomputing/mycelium_spine/internal/utils"
	"github.com/ambientlabscomputing/mycelium_spine/internal/workers"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var ctx = context.Background()

func main() {
	// 1. Load settings and initialize logger
	settings := utils.LoadSettings()
	logger, ctx := utils.InitLoggerWithContext(ctx, settings)
	logger.Info("Underleaf Mycelium Spine (UMS) starting",
		"version", "1.0.0",
		"grpc_port", settings.GRPC.Port)

	// 2. Connect to MongoDB
	mongoClient, err := buildMongoClient(ctx, settings, logger)
	if err != nil {
		logger.Error("failed to connect to MongoDB", "error", err)
		panic(err)
	}
	logger.Info("MongoDB connected", "database", settings.Mongo.Database)

	// 3. Initialize repository layer
	db := mongoClient.Database(settings.Mongo.Database)
	repo := repository.NewMongoRepository(db, logger)

	// Create indexes
	if err := repo.CreateIndexes(ctx); err != nil {
		logger.Error("failed to create indexes", "error", err)
		panic(err)
	}
	logger.Info("MongoDB indexes created")

	// 4. Initialize worker pool manager
	poolManager := workers.NewPoolManager(settings, logger)
	logger.Info("worker pool manager created",
		"session_workers", settings.Workers.SessionPoolSize,
		"delivery_workers", settings.Workers.DeliveryPoolSize,
		"ack_workers", settings.Workers.AckPoolSize,
		"mailbox_workers", settings.Workers.MailboxPoolSize)

	// 5. Initialize service layer
	appService := service.NewAppService(repo, poolManager, settings)
	if err := appService.Start(ctx); err != nil {
		logger.Error("failed to start application service", "error", err)
		panic(err)
	}
	logger.Info("application service started")

	// 6. Initialize gRPC server
	grpcServer, err := grpc.NewServer(appService, settings)
	if err != nil {
		logger.Error("failed to create gRPC server", "error", err)
		panic(err)
	}
	logger.Info("gRPC server initialized")

	// 7. Start gRPC server in background
	serverCtx, serverCancel := context.WithCancel(ctx)
	defer serverCancel()
	errChan := make(chan error, 1)
	go func() {
		if err := grpcServer.Serve(serverCtx); err != nil {
			errChan <- err
		}
	}()

	logger.Info("UMS ready to accept connections",
		"grpc_port", settings.GRPC.Port,
		"tls_enabled", true,
		"mtls_mode", settings.GRPC.TLS.ClientAuth)

	// 8. Wait for exit signal or error
	exitChannel := make(chan os.Signal, 1)
	signal.Notify(exitChannel, os.Interrupt, syscall.SIGTERM)
	select {
	case sig := <-exitChannel:
		logger.Info("received shutdown signal", "signal", sig)
	case err := <-errChan:
		logger.Error("gRPC server error", "error", err)
	}

	// 9. Graceful shutdown
	logger.Info("shutting down UMS")
	// Cancel server context to stop accepting new connections
	serverCancel()

	// Stop services with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()
	if err := appService.Stop(shutdownCtx); err != nil {
		logger.Error("error during service shutdown", "error", err)
	}

	// Disconnect MongoDB
	if err := mongoClient.Disconnect(shutdownCtx); err != nil {
		logger.Error("error disconnecting MongoDB", "error", err)
	}
	logger.Info("UMS shutdown complete")
}

// buildMongoClient creates a MongoDB client with connection pooling and auth
func buildMongoClient(ctx context.Context, settings *utils.Settings, logger *slog.Logger) (*mongo.Client, error) {
	clientOptions := options.Client().ApplyURI(settings.Mongo.URI)

	// Add authentication if credentials provided
	if settings.Mongo.User != "" {
		credential := options.Credential{
			Username: settings.Mongo.User,
			Password: string(settings.Mongo.Password),
		}
		clientOptions.SetAuth(credential)
	}

	// Configure connection pooling and timeouts
	clientOptions.SetMaxPoolSize(100)
	clientOptions.SetMinPoolSize(10)
	clientOptions.SetConnectTimeout(10 * time.Second)
	clientOptions.SetServerSelectionTimeout(5 * time.Second)

	// Connect to MongoDB
	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	// Ping to verify connection
	if err := client.Ping(ctx, nil); err != nil {
		return nil, fmt.Errorf("failed to ping MongoDB: %w", err)
	}

	return client, nil
}
