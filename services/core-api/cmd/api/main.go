package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/ai"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/httpapi"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/jobs"
	"github.com/SalehAlobaylan/dawha/services/core-api/platform/db"
	"github.com/SalehAlobaylan/dawha/services/core-api/platform/ratelimit"
	"github.com/SalehAlobaylan/dawha/services/core-api/platform/storage"
	"github.com/SalehAlobaylan/dawha/services/core-api/platform/telemetry"
)

func main() {
	telemetryConfig := telemetry.ConfigFromEnvironment(os.Getenv)
	telemetryConfig.ServiceName = "dawha-core-api"
	logger, metrics := telemetry.New(telemetryConfig)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	logger.Info("telemetry configured", "configuration", telemetryConfig.Describe())

	stopMetrics, err := metrics.StartExporter(ctx, telemetry.ExporterConfig{
		Enabled: telemetryConfig.Metrics,
		Addr:    telemetryConfig.MetricsAddr,
	})
	if err != nil {
		// A deployment that asked for an exporter and did not get one says so and
		// stops. A service quietly running with no exporter is a service whose
		// dashboards are empty for reasons nobody was told about.
		logger.Error("the metrics exporter could not start", "error", err)
		os.Exit(1)
	}
	defer func() { _ = stopMetrics() }()

	pool, err := db.NewPool(ctx, db.PoolConfig{URL: os.Getenv("DATABASE_URL"), Tracer: telemetry.NewQueryTracer(metrics)})
	if err != nil {
		logger.Error("database initialization failed", "error", err)
		os.Exit(1)
	}
	if pool != nil {
		defer pool.Close()
	}
	jobsService := jobs.NewService(pool)
	// One resolution of the storage configuration, shared with the workers. The
	// driver is chosen by configuration and fails closed: STORAGE_DRIVER=s3 with
	// no bucket stops the process here rather than quietly writing uploads to a
	// directory inside the container.
	storeConfig := storage.ConfigFromEnvironment(os.Getenv)
	storeConfig.LocalRoot = environmentValue("SOURCE_STORAGE_DIR", "../../.data/source-storage")
	// The local driver's signed URLs point back at this service, so it needs to
	// know its own external address. Unset means signed access is unavailable,
	// which is a different thing from storage being unavailable.
	if storeConfig.SigningBaseURL == "" {
		storeConfig.SigningBaseURL = environmentValue("PUBLIC_BASE_URL", "")
	}
	sourceStore, err := storage.New(ctx, storeConfig)
	if err != nil {
		logger.Error("source storage initialization failed", "error", err)
		os.Exit(1)
	}
	logger.Info("source storage configured", "driver", storageDriverName(sourceStore))
	aiClient := ai.NewClient(ai.NewHTTPProvider(environmentValue("AI_RESEARCH_URL", "http://localhost:8000")).WithMetrics(metrics))

	// The abuse-control configuration. An unparseable value stops the process
	// rather than falling back to a default, because a limit that silently became
	// something else is a limit nobody reviewed.
	rateLimits, err := ratelimit.ConfigFromEnvironment(os.Getenv)
	if err != nil {
		logger.Error("rate limit configuration is invalid", "error", err)
		os.Exit(1)
	}
	logger.Info("rate limits configured", "configuration", rateLimits.Describe())
	logger.Info("demo mode configured", "enabled", demoMode())

	port := os.Getenv("CORE_API_PORT")
	if port == "" {
		port = "8080"
	}
	server := &http.Server{
		Addr: ":" + port,
		Handler: httpapi.NewRouter(httpapi.Dependencies{
			DB:            pool,
			Logger:        logger,
			WebOrigin:     os.Getenv("WEB_ORIGIN"),
			SecureCookies: os.Getenv("APP_ENV") == "production",
			Jobs:          jobsService,
			AI:            aiClient,
			SourceStorage: sourceStore,
			RateLimits:    rateLimits,
			Metrics:       metrics,
			// Off unless DEMO_MODE says otherwise. The local stack and the browser
			// acceptance stack set it explicitly; a deployment that has not been told
			// to serve synthetic data is not served synthetic data.
			DemoMode: demoMode(),
		}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		logger.Info("core api listening", "port", port, "database_configured", pool != nil)
		if serveErr := server.ListenAndServe(); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			logger.Error("core api stopped unexpectedly", "error", serveErr)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
		os.Exit(1)
	}
	logger.Info("core api stopped")
}

func environment() string {
	value := os.Getenv("APP_ENV")
	if value == "" {
		return "development"
	}
	return value
}

// demoMode reads DEMO_MODE. Off unless it is set to something that reads as true,
// because the two mistakes available here are not symmetric: a deployment serving
// synthetic data believing it is real is worse than a deployment refusing a
// convenience.
func demoMode() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("DEMO_MODE"))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// storageDriverName names the adapter for a log line. Storage configuration, not
// content: it says which bucket or which directory, and never a key.
func storageDriverName(store storage.Store) string {
	switch store.(type) {
	case *storage.S3Store:
		return storage.DriverS3
	case *storage.LocalStore:
		return storage.DriverLocal
	default:
		return "unknown"
	}
}

func environmentValue(name, fallback string) string {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	return value
}
