package jobs

import (
	"context"
	"encoding/json"
	"time"
)

type Status string

const (
	Queued    Status = "queued"
	Running   Status = "running"
	Succeeded Status = "succeeded"
	Failed    Status = "failed"
	Dead      Status = "dead"
)

type Job struct {
	ID             string
	Type           string
	Payload        json.RawMessage
	Attempt        int
	MaxAttempts    int
	RunAt          time.Time
	IdempotencyKey string
}

type Queue interface {
	Enqueue(ctx context.Context, job Job) (string, error)
	Claim(ctx context.Context, workerID string, limit int) ([]Job, error)
	Complete(ctx context.Context, jobID string) error
	Fail(ctx context.Context, jobID string, lastError string) error
}
