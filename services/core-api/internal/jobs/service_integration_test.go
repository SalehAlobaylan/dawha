package jobs

import (
	"context"
	"os"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/platform/db"
	"github.com/google/uuid"
)

func TestFailAdvancesAttemptsOnce(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not set")
	}
	pool, err := db.NewPool(context.Background(), db.PoolConfig{URL: databaseURL})
	if err != nil || pool == nil {
		t.Fatal("database is unavailable")
	}
	defer pool.Close()
	service := NewService(pool)
	job, err := service.Enqueue(context.Background(), EnqueueInput{
		Type:           "phase14_attempt_test",
		Payload:        []byte(`{}`),
		IdempotencyKey: "phase14-attempt-" + uuid.NewString(),
		MaxAttempts:    3,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM jobs WHERE id = $1`, job.Job.ID)
	for attempt := 1; attempt <= 3; attempt++ {
		claimed, err := service.Claim(context.Background(), ClaimInput{WorkerID: "phase14-attempt-worker", Type: "phase14_attempt_test"})
		if err != nil {
			t.Fatal(err)
		}
		failed, err := service.Fail(context.Background(), claimed.ID, FailInput{WorkerID: "phase14-attempt-worker", Error: "intentional"})
		if err != nil {
			t.Fatal(err)
		}
		if failed.Attempts != attempt {
			t.Fatalf("attempt = %d, want %d", failed.Attempts, attempt)
		}
		if attempt < 3 {
			if failed.Status != "queued" {
				t.Fatalf("status = %s, want queued", failed.Status)
			}
			if _, err := pool.Exec(context.Background(), `UPDATE jobs SET run_at = now() WHERE id = $1`, claimed.ID); err != nil {
				t.Fatal(err)
			}
		} else if failed.Status != "dead" {
			t.Fatalf("status = %s, want dead", failed.Status)
		}
	}
}
