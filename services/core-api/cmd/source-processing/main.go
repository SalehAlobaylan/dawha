package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/ai"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/jobs"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/sourceprocessing"
	"github.com/SalehAlobaylan/dawha/services/core-api/platform/db"
	"github.com/SalehAlobaylan/dawha/services/core-api/platform/storage"
	"github.com/google/uuid"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := db.NewPool(ctx, db.PoolConfig{URL: os.Getenv("DATABASE_URL")})
	if err != nil || pool == nil {
		log.Fatal("source processing worker requires DATABASE_URL")
	}
	defer pool.Close()
	store, err := storage.NewLocal(environmentValue("SOURCE_STORAGE_DIR", "../../.data/source-storage"))
	if err != nil {
		log.Fatal(err)
	}
	provider := ai.NewHTTPClient(environmentValue("AI_RESEARCH_URL", "http://localhost:8000"))
	jobService := jobs.NewService(pool)
	processor := sourceprocessing.NewService(pool, store, jobService, provider, sourceprocessing.NewTextExtractor())
	workerID := environmentValue("SOURCE_WORKER_ID", "source-worker-"+uuid.NewString())
	pollInterval := durationEnvironment("SOURCE_WORKER_POLL_INTERVAL", 2*time.Second)
	log.Printf("source processing worker started: %s", workerID)
	lastRecovery := time.Now()
	for {
		if time.Since(lastRecovery) >= time.Minute {
			recovered, err := jobService.RecoverStale(ctx, 15*time.Minute)
			if err != nil {
				log.Printf("recover stale jobs: %v", err)
			} else if err := processor.RequeueRecovered(ctx, recovered.Jobs); err != nil {
				log.Printf("reset recovered source runs: %v", err)
			}
			lastRecovery = time.Now()
		}
		job, err := jobService.Claim(ctx, jobs.ClaimInput{WorkerID: workerID, Type: sourceprocessing.SourceProcessJobType})
		if errors.Is(err, jobs.ErrNotFound) {
			if !sleepContext(ctx, pollInterval) {
				break
			}
			continue
		}
		if err != nil {
			log.Printf("claim source job: %v", err)
			if !sleepContext(ctx, pollInterval) {
				break
			}
			continue
		}
		if err := processor.Process(ctx, job); err != nil {
			log.Printf("process source job %s: %v", job.ID, err)
			if _, failErr := jobService.Fail(ctx, job.ID, jobs.FailInput{WorkerID: workerID, Error: err.Error()}); failErr != nil {
				log.Printf("fail source job %s: %v", job.ID, failErr)
			}
			continue
		}
		if _, err := jobService.Complete(ctx, job.ID, jobs.CompleteInput{WorkerID: workerID}); err != nil {
			log.Printf("complete source job %s: %v", job.ID, err)
		}
	}
	log.Print("source processing worker stopped")
}

func environmentValue(name, fallback string) string {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	return value
}

func durationEnvironment(name string, fallback time.Duration) time.Duration {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func sleepContext(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
