package bootstrap

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"time"

	"github.com/ambientlabscomputing/mycelium_spine/internal/utils"
)

// RunRenewalLoop runs as a background goroutine and proactively renews the spine
// TLS cert before it expires. The check interval is 24 hours; renewal is triggered
// when fewer than settings.Bootstrap.RenewBeforeDays days remain.
//
// After a successful renewal, the CertManager is updated in-place so new TLS
// handshakes use the fresh cert without a server restart.
func RunRenewalLoop(ctx context.Context, settings *utils.Settings, mgr *CertManager, logger *slog.Logger) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := checkAndRenew(ctx, settings, mgr, logger); err != nil {
				logger.Error("cert renewal failed — will retry in 24h", "error", err)
			}
		}
	}
}

func checkAndRenew(ctx context.Context, settings *utils.Settings, mgr *CertManager, logger *slog.Logger) error {
	renewBefore := time.Duration(settings.Bootstrap.RenewBeforeDays) * 24 * time.Hour
	if renewBefore == 0 {
		renewBefore = DefaultRenewBeforeDays * 24 * time.Hour
	}

	certPath := settings.GRPC.TLS.CertPath
	keyPath := settings.GRPC.TLS.KeyPath

	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		logger.Warn("could not load on-disk cert for renewal check, triggering renewal",
			"cert_path", certPath, "error", err)
	} else {
		x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
		if err != nil {
			return fmt.Errorf("parse on-disk cert: %w", err)
		}
		if time.Until(x509Cert.NotAfter) > renewBefore {
			logger.Debug("cert still valid, skipping renewal",
				"expires_at", x509Cert.NotAfter,
				"renew_before_days", settings.Bootstrap.RenewBeforeDays)
			return nil
		}
		logger.Info("cert expiring soon, renewing",
			"expires_at", x509Cert.NotAfter,
			"renew_before_days", settings.Bootstrap.RenewBeforeDays)
	}

	newCert, err := EnsureCert(ctx, settings)
	if err != nil {
		return fmt.Errorf("fetch renewed cert: %w", err)
	}

	mgr.Update(newCert)
	logger.Info("TLS cert renewed and hot-swapped")
	return nil
}
