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
	jobService.HeartbeatInterval = durationEnvironment("SOURCE_WORKER_HEARTBEAT_INTERVAL", jobs.DefaultHeartbeatInterval)
	jobService.LeaseDuration = durationEnvironment("SOURCE_WORKER_LEASE_DURATION", jobs.DefaultLeaseDuration)
	processor := sourceprocessing.NewService(pool, store, jobService, provider, sourceprocessing.NewTextExtractor())
	workerID := environmentValue("SOURCE_WORKER_ID", "source-worker-"+uuid.NewString())
	pollInterval := durationEnvironment("SOURCE_WORKER_POLL_INTERVAL", 2*time.Second)
	log.Printf("source processing worker started: %s (lease %s, heartbeat %s)", workerID, jobService.LeaseDuration, jobService.HeartbeatInterval)
	lastRecovery := time.Now()
	for {
		if time.Since(lastRecovery) >= time.Minute {
			// Recovery is narrowed to this worker's own job type. With more than
			// one consumer sharing the queue, an unfiltered pass belongs to
			// whichever process runs it first, and the second one never sees the
			// job it would have had to reset the run row for.
			recovered, err := jobService.RecoverStaleOfType(ctx, StaleRecoveryWindow, sourceprocessing.SourceProcessJobType)
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
		claim := sourceprocessing.Claim{Job: job, Lease: job.Lease(workerID)}
		if err := processor.Process(ctx, claim); err != nil {
			// A worker that lost its lease has nothing to say about the job any
			// more: the attempt that owns it now will fail it, retry it or finish
			// it. Recording a failure here would be one worker answering for
			// another, and requeueing would put the same work in flight twice.
			if errors.Is(err, jobs.ErrLeaseLost) || errors.Is(err, jobs.ErrForbidden) {
				log.Printf("abandon source job %s: %v", job.ID, err)
				continue
			}
			log.Printf("process source job %s: %v", job.ID, err)
			if _, failErr := jobService.Fail(ctx, job.ID, jobs.FailInput{WorkerID: workerID, LeaseToken: claim.Lease.Token, Error: err.Error()}); failErr != nil {
				log.Printf("fail source job %s: %v", job.ID, failErr)
			}
			continue
		}
		if _, err := jobService.Complete(ctx, job.ID, jobs.CompleteInput{WorkerID: workerID, LeaseToken: claim.Lease.Token}); err != nil {
			log.Printf("complete source job %s: %v", job.ID, err)
		}
	}
	log.Print("source processing worker stopped")
}

// StaleRecoveryWindow is how long a claim has to go without a heartbeat before
// the job is handed back to the queue. It is deliberately much longer than the
// lease: the lease is how fast a worker learns it lost the job, and this is how
// fast the queue gets the job back from a process that is gone without saying so.
const StaleRecoveryWindow = 15 * time.Minute

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
