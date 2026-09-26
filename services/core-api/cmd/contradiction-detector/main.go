package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/contradiction"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/jobs"
	"github.com/SalehAlobaylan/dawha/services/core-api/platform/db"
	"github.com/SalehAlobaylan/dawha/services/core-api/platform/telemetry"
	"github.com/google/uuid"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	telemetryConfig := telemetry.ConfigFromEnvironment(os.Getenv)
	telemetryConfig.ServiceName = "dawha-contradiction-worker"
	_, metrics := telemetry.New(telemetryConfig)
	stopMetrics, err := metrics.StartExporter(ctx, telemetry.ExporterConfig{
		Enabled: telemetryConfig.Metrics,
		Addr:    telemetryConfig.MetricsAddr,
	})
	if err != nil {
		log.Fatalf("the metrics exporter could not start: %v", err)
	}
	defer func() { _ = stopMetrics() }()

	pool, err := db.NewPool(ctx, db.PoolConfig{URL: os.Getenv("DATABASE_URL"), Tracer: telemetry.NewQueryTracer(metrics)})
	if err != nil || pool == nil {
		log.Fatal("contradiction worker requires DATABASE_URL")
	}
	defer pool.Close()
	jobService := jobs.NewService(pool)
	processor := contradiction.NewService(pool, jobService)
	workerID := environmentValue("CONTRADICTION_WORKER_ID", "contradiction-worker-"+uuid.NewString())
	pollInterval := durationEnvironment("CONTRADICTION_WORKER_POLL_INTERVAL", 2*time.Second)
	log.Printf("contradiction worker started: %s", workerID)
	lastRecovery := time.Now()
	for {
		if time.Since(lastRecovery) >= time.Minute {
			recovered, recoverErr := jobService.RecoverStale(ctx, 15*time.Minute)
			if recoverErr != nil {
				log.Printf("recover stale contradiction jobs: %v", recoverErr)
			} else if err := processor.RequeueRecovered(ctx, recovered.Jobs); err != nil {
				log.Printf("reset recovered contradiction runs: %v", err)
			}
			lastRecovery = time.Now()
		}
		if metrics.Enabled() {
			if depth, depthErr := jobService.Depth(ctx, contradiction.JobType); depthErr == nil {
				metrics.QueueDepth(telemetry.JobTypeFor(contradiction.JobType), depth)
			}
		}
		job, err := jobService.Claim(ctx, jobs.ClaimInput{WorkerID: workerID, Type: contradiction.JobType})
		if errors.Is(err, jobs.ErrNotFound) {
			if !sleepContext(ctx, pollInterval) {
				break
			}
			continue
		}
		if err != nil {
			log.Printf("claim contradiction job: %v", err)
			if !sleepContext(ctx, pollInterval) {
				break
			}
			continue
		}
		if err := processor.Process(ctx, job); err != nil {
			log.Printf("process contradiction job %s: %v", job.ID, err)
			if _, failErr := jobService.Fail(ctx, job.ID, jobs.FailInput{WorkerID: workerID, Error: err.Error()}); failErr != nil {
				log.Printf("fail contradiction job %s: %v", job.ID, failErr)
			}
			continue
		}
		if _, err := jobService.Complete(ctx, job.ID, jobs.CompleteInput{WorkerID: workerID}); err != nil {
			log.Printf("complete contradiction job %s: %v", job.ID, err)
		}
	}
	log.Print("contradiction worker stopped")
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
