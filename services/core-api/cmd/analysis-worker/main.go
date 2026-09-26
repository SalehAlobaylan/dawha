// Command analysis-worker drains the PostgreSQL queue for the two analyses that
// no longer run inside HTTP requests: entity resolution and the research agent.
//
// It is a separate process from the API on purpose. The API accepts runs and
// writes them to the queue; this process claims them, does the slow work under a
// lease, and finishes them. A worker that shared the API's process would share its
// fate - a deploy, a crash, a scaling decision made for the wrong workload would
// all interrupt an investigation half way - and would make the queue's central
// promise untrue, which is that accepted work reaches a terminal state.
//
// The queue types this process takes are printed at startup, because the one
// deployment mistake that matters is starting the API without starting this, and
// the visible symptom of that mistake is runs that stay queued forever.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/ai"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/analysisworker"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/jobs"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/jobworker"
	"github.com/SalehAlobaylan/dawha/services/core-api/platform/db"
	"github.com/google/uuid"
)

func main() {
	logger := log.New(os.Stderr, "analysis-worker ", log.LstdFlags|log.LUTC)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.NewPool(ctx, db.PoolConfig{URL: os.Getenv("DATABASE_URL")})
	if err != nil || pool == nil {
		logger.Fatal("analysis worker requires DATABASE_URL")
	}
	defer pool.Close()

	queue := jobs.NewService(pool)
	queue.LeaseDuration = durationEnvironment("ANALYSIS_WORKER_LEASE_DURATION", jobs.DefaultLeaseDuration)
	queue.HeartbeatInterval = durationEnvironment("ANALYSIS_WORKER_HEARTBEAT_INTERVAL", jobs.DefaultHeartbeatInterval)
	provider := ai.NewHTTPClient(environmentValue("AI_RESEARCH_URL", "http://localhost:8000"))
	handler, err := analysisworker.New(pool, queue, provider, logger)
	if err != nil {
		logger.Fatal(err)
	}
	config := jobworker.Config{
		Jobs:       queue,
		WorkerID:   environmentValue("ANALYSIS_WORKER_ID", "analysis-worker-"+uuid.NewString()),
		Handlers:   []jobworker.Handler{handler},
		Logger:     logger,
		StaleAfter: jobworker.DefaultStaleAfter,
	}
	if err := jobworker.Run(ctx, config); err != nil && ctx.Err() == nil {
		logger.Fatal(err)
	}
	logger.Print("analysis worker stopped cleanly")
}

func environmentValue(name, fallback string) string {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	return value
}

// durationEnvironment reads a duration, refusing a value it cannot use. A
// heartbeat interval that silently fell back to the default would be invisible,
// and an interval of zero would be a worker that never renews at all.
func durationEnvironment(name string, fallback time.Duration) time.Duration {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		log.Fatalf("%s=%q is not a positive duration", name, value)
	}
	return parsed
}
