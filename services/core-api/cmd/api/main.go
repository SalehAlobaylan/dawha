package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/httpapi"
	"github.com/SalehAlobaylan/dawha/services/core-api/platform/db"
	"github.com/SalehAlobaylan/dawha/services/core-api/platform/telemetry"
)

func main() {
	logger := telemetry.NewLogger(telemetry.Config{ServiceName: "dawha-core-api", Environment: environment()})
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	pool, err := db.NewPool(ctx, db.PoolConfig{URL: os.Getenv("DATABASE_URL")})
	if err != nil {
		logger.Error("database initialization failed", "error", err)
		os.Exit(1)
	}
	if pool != nil {
		defer pool.Close()
	}

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
