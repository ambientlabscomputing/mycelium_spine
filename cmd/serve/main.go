package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/ambientlabscomputing/mycelium_spine/internal/bootstrap"
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

	// 5b. Bootstrap TLS cert if configured.
	// When settings.Bootstrap.CertCN is set and TLS is enabled, Spine fetches its
	// own cert from server_api on startup (and renews it proactively in the background).
	// When CertCN is empty, Spine uses pre-staged disk certs (local dev / manual setup).
	var certMgr *bootstrap.CertManager
	if settings.GRPC.TLS.Enabled && settings.Bootstrap.CertCN != "" {
		logger.Info("cert bootstrap enabled", "cert_cn", settings.Bootstrap.CertCN)
		certMgr = runBootstrap(ctx, settings, logger)
	} else if settings.Bootstrap.CertCN != "" && !settings.GRPC.TLS.Enabled {
		logger.Warn("bootstrap.cert_cn is set but grpc.tls.enabled is false — bootstrap skipped")
	}

	// 6. Initialize gRPC server
	var grpcServer *grpc.Server
	if certMgr != nil {
		grpcServer, err = grpc.NewServer(appService, settings, certMgr.GetCertificate)
	} else {
		grpcServer, err = grpc.NewServer(appService, settings)
	}
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
		"tls_enabled", settings.GRPC.TLS.Enabled,
		"mtls_mode", settings.GRPC.TLS.ClientAuth)

	// 8a. Start cert renewal loop if bootstrap is active.
	if certMgr != nil {
		go bootstrap.RunRenewalLoop(serverCtx, settings, certMgr, logger)
		logger.Info("cert renewal loop started", "check_interval", "24h")
	}

	// 8b. Wait for exit signal or error
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

// runBootstrap fetches the CA cert and ensures the spine TLS cert is valid,
// retrying with exponential backoff (5 attempts, 2s initial). Exits the process
// if all attempts fail — the cert is required for the gRPC server to start.
func runBootstrap(ctx context.Context, settings *utils.Settings, logger *slog.Logger) *bootstrap.CertManager {
	const maxAttempts = 5

	// Step 1: Fetch CA cert from server_api (public endpoint, no auth).
	backoff := 2 * time.Second
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := bootstrap.EnsureCACert(ctx, settings); err == nil {
			break
		} else {
			logger.Error("CA cert fetch failed", "attempt", attempt, "max", maxAttempts, "backoff", backoff, "error", err)
			if attempt == maxAttempts {
				logger.Error("CA cert bootstrap failed after all attempts")
				os.Exit(1)
			}
			select {
			case <-ctx.Done():
				logger.Error("context cancelled during CA cert bootstrap")
				os.Exit(1)
			case <-time.After(backoff):
			}
			backoff *= 2
		}
	}
	logger.Info("CA cert fetched from server_api")

	// Step 2: Ensure our own TLS cert (CSR flow if absent or expiring).
	backoff = 2 * time.Second
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		cert, err := bootstrap.EnsureCert(ctx, settings)
		if err == nil {
			logger.Info("TLS cert bootstrapped", "cert_cn", settings.Bootstrap.CertCN)
			return bootstrap.NewCertManager(cert)
		}
		logger.Error("spine cert bootstrap failed", "attempt", attempt, "max", maxAttempts, "backoff", backoff, "error", err)
		if attempt == maxAttempts {
			logger.Error("spine cert bootstrap failed after all attempts")
			os.Exit(1)
		}
		select {
		case <-ctx.Done():
			logger.Error("context cancelled during cert bootstrap")
			os.Exit(1)
		case <-time.After(backoff):
		}
		backoff *= 2
	}
	// Unreachable — os.Exit above.
	return nil
}

// buildMongoClient creates a MongoDB client with connection pooling and auth.
//
// Two auth patterns are supported — credentials are NEVER stored as literal
// text inside the URI string:
//
//  1. Templated URI (preferred for complex topologies):
//     Set uri to a template containing <username> and <password> placeholders.
//     At startup the placeholders are substituted with the config field values
//     (properly URL-encoded), and the fully-resolved URI is passed to the
//     driver. authSource, authMechanism, replicaSet, and any other query params
//     in the URI template are preserved exactly.
//     Example:
//     uri: "mongodb://<username>:<password>@host.docker.internal:27017/mycelium_spine?authSource=admin"
//     user: "spine_user"
//     password: "s3cr3t"
//
//  2. Plain URI + config credentials (simple single-node setups):
//     Leave the URI without placeholders and let auth_source / auth_mechanism
//     config fields drive the Credential struct passed to SetAuth().
//     Example:
//     uri: "mongodb://host.docker.internal:27017"
//     user: "spine_user"
//     password: "s3cr3t"
//     auth_source: "admin"
func buildMongoClient(ctx context.Context, settings *utils.Settings, logger *slog.Logger) (*mongo.Client, error) {
	// Resolve the connection URI. If the template contains <username> or
	// <password> placeholders, substitute them with URL-encoded config values so
	// that special characters in passwords are handled correctly and raw
	// credentials never appear in the config URI string.
	connURI := settings.Mongo.URI
	useTemplate := strings.Contains(connURI, "<username>") || strings.Contains(connURI, "<password>")
	if useTemplate {
		if settings.Mongo.User == "" {
			return nil, fmt.Errorf("mongo URI contains credential placeholders but mongo.user is not set in config")
		}
		connURI = strings.ReplaceAll(connURI, "<username>", url.PathEscape(settings.Mongo.User))
		connURI = strings.ReplaceAll(connURI, "<password>", url.PathEscape(string(settings.Mongo.Password)))
		logger.Debug("mongodb URI template resolved", "user", settings.Mongo.User)
	}

	clientOptions := options.Client().ApplyURI(connURI)

	// When using a templated URI, ApplyURI() already has the full credential
	// (user:pass + authSource from query params). Do not call SetAuth() — it
	// would overwrite the parsed credential and drop query-string auth options.
	//
	// For a plain URI, call SetAuth() explicitly so auth_source and
	// auth_mechanism from config are applied to the connection.
	if !useTemplate && settings.Mongo.User != "" {
		credential := options.Credential{
			Username:      settings.Mongo.User,
			Password:      string(settings.Mongo.Password),
			AuthSource:    settings.Mongo.AuthSource,
			AuthMechanism: settings.Mongo.AuthMechanism,
		}
		clientOptions.SetAuth(credential)
		logger.Debug("mongodb auth configured via SetAuth",
			"user", settings.Mongo.User,
			"auth_source", settings.Mongo.AuthSource,
			"auth_mechanism", settings.Mongo.AuthMechanism,
		)
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
