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
	"github.com/SalehAlobaylan/dawha/services/core-api/platform/telemetry"
	"github.com/google/uuid"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// Telemetry is off unless TELEMETRY_METRICS_ENABLED says otherwise, and a
	// disabled registry records nothing and starts no listener. That is what keeps
	// `make verify` and `make e2e` unaffected: this worker is started by both.
	telemetryConfig := telemetry.ConfigFromEnvironment(os.Getenv)
	telemetryConfig.ServiceName = "dawha-source-processing"
	_, metrics := telemetry.New(telemetryConfig)
	stopMetrics, err := metrics.StartExporter(ctx, telemetry.ExporterConfig{
		Enabled: telemetryConfig.Metrics,
		Addr:    telemetryConfig.MetricsAddr,
	})
	if err != nil {
		log.Fatalf("the metrics exporter could not start: %v", err)
	}
	defer func() { _ = stopMetrics() }()
	log.Printf("telemetry: %s", telemetryConfig.Describe())

	pool, err := db.NewPool(ctx, db.PoolConfig{URL: os.Getenv("DATABASE_URL"), Tracer: telemetry.NewQueryTracer(metrics)})
	if err != nil || pool == nil {
		log.Fatal("source processing worker requires DATABASE_URL")
	}
	defer pool.Close()
	// The SAME store resolution the API does, from the same environment. The two
	// used to each call storage.NewLocal, which meant a deployment that configured
	// a bucket had a worker reading from a directory the API was not writing to.
	storeConfig := storage.ConfigFromEnvironment(os.Getenv)
	storeConfig.LocalRoot = environmentValue("SOURCE_STORAGE_DIR", "../../.data/source-storage")
	store, err := storage.New(ctx, storeConfig)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("source storage: driver=%s", storageDriverName(store))
	provider := ai.NewClient(ai.NewHTTPProvider(environmentValue("AI_RESEARCH_URL", "http://localhost:8000")).WithMetrics(metrics))
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
		// The depth is read once per poll, and only while telemetry is on. It is a
		// count of rows and nothing else, so a depth gauge built from it holds no
		// copy of what is queued.
		if metrics.Enabled() {
			if depth, depthErr := jobService.Depth(ctx, sourceprocessing.SourceProcessJobType); depthErr == nil {
				metrics.QueueDepth(telemetry.JobTypeFor(sourceprocessing.SourceProcessJobType), depth)
			}
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
		// The request id the uploading request left in the payload, so a failure
		// in here names the upload that caused it rather than only the job.
		jobRequestID := telemetry.RequestIDFromPayload(sourceprocessing.JobRequestIDFields(job.Payload))
		jobStarted := time.Now()
		jobContext := telemetry.WithRequestID(ctx, jobRequestID)
		if err := processor.Process(jobContext, claim); err != nil {
			metrics.QueueJob(telemetry.JobTypeFor(sourceprocessing.SourceProcessJobType), telemetry.JobFailed, time.Since(jobStarted))
			// A worker that lost its lease has nothing to say about the job any
			// more: the attempt that owns it now will fail it, retry it or finish
			// it. Recording a failure here would be one worker answering for
			// another, and requeueing would put the same work in flight twice.
			if errors.Is(err, jobs.ErrLeaseLost) || errors.Is(err, jobs.ErrForbidden) {
				log.Printf("abandon source job %s request_id=%s: %v", job.ID, requestIDOrNone(jobRequestID), err)
				metrics.QueueJob(telemetry.JobTypeFor(sourceprocessing.SourceProcessJobType), telemetry.JobAbandoned, time.Since(jobStarted))
				continue
			}
			log.Printf("process source job %s request_id=%s: %v", job.ID, requestIDOrNone(jobRequestID), err)
			if _, failErr := jobService.Fail(ctx, job.ID, jobs.FailInput{WorkerID: workerID, LeaseToken: claim.Lease.Token, Error: err.Error()}); failErr != nil {
				log.Printf("fail source job %s: %v", job.ID, failErr)
			}
			continue
		}
		if _, err := jobService.Complete(ctx, job.ID, jobs.CompleteInput{WorkerID: workerID, LeaseToken: claim.Lease.Token}); err != nil {
			log.Printf("complete source job %s request_id=%s: %v", job.ID, requestIDOrNone(jobRequestID), err)
		}
		metrics.QueueJob(telemetry.JobTypeFor(sourceprocessing.SourceProcessJobType), telemetry.JobCompleted, time.Since(jobStarted))
	}
	log.Print("source processing worker stopped")
}

// requestIDOrNone prints "none" rather than an empty field, so a line about a job
// with no request behind it is not read as a line about a request with an
// unreadable id.
func requestIDOrNone(id string) string {
	if id == "" {
		return "none"
	}
	return id
}

// StaleRecoveryWindow is how long a claim has to go without a heartbeat before
// the job is handed back to the queue. It is deliberately much longer than the
// lease: the lease is how fast a worker learns it lost the job, and this is how
// fast the queue gets the job back from a process that is gone without saying so.
const StaleRecoveryWindow = 15 * time.Minute

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
