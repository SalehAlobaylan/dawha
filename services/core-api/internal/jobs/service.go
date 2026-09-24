package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrDatabaseUnavailable = errors.New("jobs database is unavailable")
	ErrNotFound            = errors.New("job resource not found")
	ErrForbidden           = errors.New("job access is forbidden")
	ErrValidation          = errors.New("job input is invalid")
	ErrConflict            = errors.New("job state conflict")
)

type EnqueueInput struct {
	Type           string          `json:"type"`
	Payload        json.RawMessage `json:"payload"`
	Priority       int             `json:"priority"`
	RunAt          *time.Time      `json:"run_at,omitempty"`
	IdempotencyKey string          `json:"idempotency_key"`
	MaxAttempts    int             `json:"max_attempts"`
}

type ClaimInput struct {
	WorkerID string `json:"worker_id"`
	Type     string `json:"type"`
}

type CompleteInput struct {
	WorkerID string `json:"worker_id"`
}

type FailInput struct {
	WorkerID string `json:"worker_id"`
	Error    string `json:"error"`
}

type ListFilter struct {
	Status string
	Type   string
	Limit  int
}

type JobView struct {
	ID             string          `json:"id"`
	Type           string          `json:"type"`
	Payload        json.RawMessage `json:"payload"`
	Status         string          `json:"status"`
	Priority       int             `json:"priority"`
	Attempts       int             `json:"attempts"`
	MaxAttempts    int             `json:"max_attempts"`
	RunAt          time.Time       `json:"runAt"`
	LockedAt       *time.Time      `json:"lockedAt,omitempty"`
	LockedBy       string          `json:"lockedBy,omitempty"`
	LastError      string          `json:"lastError,omitempty"`
	IdempotencyKey string          `json:"idempotencyKey,omitempty"`
	CreatedAt      time.Time       `json:"createdAt"`
	UpdatedAt      time.Time       `json:"updatedAt"`
}

type EnqueueResult struct {
	Job     JobView `json:"job"`
	Created bool    `json:"created"`
}

type RecoverResult struct {
	Recovered int       `json:"recovered"`
	Jobs      []JobView `json:"jobs"`
}

type Service struct {
	Pool *pgxpool.Pool
}

type dbExecutor interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{Pool: pool}
}

func (s *Service) Enqueue(ctx context.Context, input EnqueueInput) (EnqueueResult, error) {
	if err := s.ready(); err != nil {
		return EnqueueResult{}, err
	}
	return s.enqueue(ctx, s.Pool, input)
}

func (s *Service) EnqueueTx(ctx context.Context, tx pgx.Tx, input EnqueueInput) (EnqueueResult, error) {
	if s == nil || s.Pool == nil || tx == nil {
		return EnqueueResult{}, ErrDatabaseUnavailable
	}
	return s.enqueue(ctx, tx, input)
}

func (s *Service) enqueue(ctx context.Context, q dbExecutor, input EnqueueInput) (EnqueueResult, error) {
	input, err := validateEnqueue(input)
	if err != nil {
		return EnqueueResult{}, err
	}
	runAt := time.Now().UTC()
	if input.RunAt != nil {
		runAt = input.RunAt.UTC()
	}
	var key any
	if input.IdempotencyKey != "" {
		key = input.IdempotencyKey
	}
	row := q.QueryRow(ctx, `
		INSERT INTO jobs (type, payload, status, priority, run_at, idempotency_key, max_attempts)
		VALUES ($1, $2, 'queued', $3, $4, $5, $6)
		ON CONFLICT (idempotency_key) DO NOTHING
		RETURNING id, type, payload, status, priority, attempts, max_attempts, run_at, locked_at, locked_by, last_error, idempotency_key, created_at, updated_at
	`, input.Type, input.Payload, input.Priority, runAt, key, input.MaxAttempts)
	job, err := scanJob(row)
	if err == nil {
		return EnqueueResult{Job: job, Created: true}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) || input.IdempotencyKey == "" {
		return EnqueueResult{}, err
	}
	job, err = s.byIdempotencyKey(ctx, q, input.IdempotencyKey)
	if err != nil {
		return EnqueueResult{}, err
	}
	return EnqueueResult{Job: job, Created: false}, nil
}

func (s *Service) List(ctx context.Context, filter ListFilter) ([]JobView, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	filter, err := validateListFilter(filter)
	if err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx, jobSelect+` WHERE ($1 = '' OR status = $1) AND ($2 = '' OR type = $2) ORDER BY priority DESC, created_at DESC LIMIT $3`, strings.ToLower(strings.TrimSpace(filter.Status)), strings.TrimSpace(filter.Type), filter.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]JobView, 0)
	for rows.Next() {
		item, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) Get(ctx context.Context, jobID string) (JobView, error) {
	if err := s.ready(); err != nil {
		return JobView{}, err
	}
	id, err := uuid.Parse(strings.TrimSpace(jobID))
	if err != nil {
		return JobView{}, ErrNotFound
	}
	job, err := scanJob(s.Pool.QueryRow(ctx, jobSelect+` WHERE id = $1`, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return JobView{}, ErrNotFound
		}
		return JobView{}, err
	}
	return job, nil
}

func (s *Service) Claim(ctx context.Context, input ClaimInput) (JobView, error) {
	if err := s.ready(); err != nil {
		return JobView{}, err
	}
	workerID, err := validateWorker(input.WorkerID)
	if err != nil {
		return JobView{}, err
	}
	input.WorkerID = workerID
	input.Type, err = validateJobType(input.Type)
	if err != nil {
		return JobView{}, err
	}
	row := s.Pool.QueryRow(ctx, `
		WITH candidate AS (
			SELECT id FROM jobs
			WHERE status = 'queued' AND run_at <= now() AND ($2 = '' OR type = $2)
			ORDER BY priority DESC, run_at, created_at
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		UPDATE jobs AS j SET status = 'running', locked_at = now(), locked_by = $1, updated_at = now()
		FROM candidate AS c WHERE j.id = c.id
		RETURNING j.id, j.type, j.payload, j.status, j.priority, j.attempts, j.max_attempts, j.run_at, j.locked_at, j.locked_by, j.last_error, j.idempotency_key, j.created_at, j.updated_at
	`, input.WorkerID, input.Type)
	job, err := scanJob(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return JobView{}, ErrNotFound
	}
	return job, err
}

func (s *Service) Complete(ctx context.Context, jobID string, input CompleteInput) (JobView, error) {
	if err := s.ready(); err != nil {
		return JobView{}, err
	}
	id, err := uuid.Parse(strings.TrimSpace(jobID))
	if err != nil {
		return JobView{}, ErrNotFound
	}
	workerID, err := validateWorker(input.WorkerID)
	if err != nil {
		return JobView{}, err
	}
	job, err := scanJob(s.Pool.QueryRow(ctx, completeJobQuery, id, workerID))
	if errors.Is(err, pgx.ErrNoRows) {
		return JobView{}, s.stateError(ctx, id, workerID)
	}
	return job, err
}

func (s *Service) Fail(ctx context.Context, jobID string, input FailInput) (JobView, error) {
	if err := s.ready(); err != nil {
		return JobView{}, err
	}
	id, err := uuid.Parse(strings.TrimSpace(jobID))
	if err != nil {
		return JobView{}, ErrNotFound
	}
	workerID, err := validateWorker(input.WorkerID)
	if err != nil {
		return JobView{}, err
	}
	input.Error = strings.TrimSpace(input.Error)
	if input.Error == "" || len([]rune(input.Error)) > 4000 {
		return JobView{}, ErrValidation
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return JobView{}, err
	}
	defer tx.Rollback(ctx)
	var status, lockedBy string
	var attempts, maxAttempts int
	if err := tx.QueryRow(ctx, `SELECT status, locked_by, attempts, max_attempts FROM jobs WHERE id = $1 FOR UPDATE`, id).Scan(&status, &lockedBy, &attempts, &maxAttempts); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return JobView{}, ErrNotFound
		}
		return JobView{}, err
	}
	if status != "running" || lockedBy != workerID {
		return JobView{}, ErrConflict
	}
	nextAttempts := attempts + 1
	status = "queued"
	runAt := time.Now().UTC()
	if nextAttempts >= maxAttempts {
		status = "dead"
	} else {
		runAt = runAt.Add(backoff(nextAttempts))
	}
	job, err := scanJob(tx.QueryRow(ctx, failJobQuery, status, runAt, input.Error, id))
	if err != nil {
		return JobView{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return JobView{}, err
	}
	return job, nil
}

func (s *Service) RecoverStale(ctx context.Context, olderThan time.Duration) (RecoverResult, error) {
	if err := s.ready(); err != nil {
		return RecoverResult{}, err
	}
	if olderThan <= 0 {
		olderThan = 15 * time.Minute
	}
	seconds := int(olderThan / time.Second)
	if seconds < 1 {
		seconds = 1
	}
	rows, err := s.Pool.Query(ctx, `
		WITH stale AS (
			SELECT id, attempts, max_attempts FROM jobs
			WHERE status = 'running' AND locked_at < now() - ($1 * interval '1 second')
			FOR UPDATE SKIP LOCKED
		)
		UPDATE jobs AS j SET status = CASE WHEN j.attempts + 1 >= j.max_attempts THEN 'dead' ELSE 'queued' END,
		       attempts = j.attempts + 1,
		       run_at = CASE WHEN j.attempts + 1 >= j.max_attempts THEN now() ELSE now() + (interval '1 second' * power(2, LEAST(j.attempts, 10))) END,
		       locked_at = NULL, locked_by = NULL,
		       last_error = COALESCE(j.last_error, 'worker lock expired'), updated_at = now()
		FROM stale AS s WHERE j.id = s.id
		RETURNING j.id, j.type, j.payload, j.status, j.priority, j.attempts, j.max_attempts, j.run_at, j.locked_at, j.locked_by, j.last_error, j.idempotency_key, j.created_at, j.updated_at
	`, seconds)
	if err != nil {
		return RecoverResult{}, err
	}
	defer rows.Close()
	items := make([]JobView, 0)
	for rows.Next() {
		item, err := scanJob(rows)
		if err != nil {
			return RecoverResult{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return RecoverResult{}, err
	}
	return RecoverResult{Recovered: len(items), Jobs: items}, nil
}

func (s *Service) CanOperate(ctx context.Context, userID string) (bool, error) {
	if err := s.ready(); err != nil {
		return false, err
	}
	id, err := uuid.Parse(strings.TrimSpace(userID))
	if err != nil {
		return false, ErrForbidden
	}
	var allowed bool
	if err := s.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM user_roles WHERE user_id = $1 AND role IN ('researcher', 'moderator', 'admin'))`, id).Scan(&allowed); err != nil {
		return false, err
	}
	return allowed, nil
}

func (s *Service) ready() error {
	if s == nil || s.Pool == nil {
		return ErrDatabaseUnavailable
	}
	return nil
}

const jobSelect = `SELECT id, type, payload, status, priority, attempts, max_attempts, run_at, locked_at, locked_by, last_error, idempotency_key, created_at, updated_at FROM jobs`

const completeJobQuery = `UPDATE jobs SET status = 'succeeded', run_at = now(), last_error = NULL, locked_at = NULL, locked_by = NULL, updated_at = now() WHERE id = $1 AND status = 'running' AND locked_by = $2 RETURNING id, type, payload, status, priority, attempts, max_attempts, run_at, locked_at, locked_by, last_error, idempotency_key, created_at, updated_at`

const failJobQuery = `UPDATE jobs SET status = $1, run_at = $2, last_error = $3, attempts = attempts + 1, locked_at = NULL, locked_by = NULL, updated_at = now() WHERE id = $4 RETURNING id, type, payload, status, priority, attempts, max_attempts, run_at, locked_at, locked_by, last_error, idempotency_key, created_at, updated_at`

func (s *Service) byIdempotencyKey(ctx context.Context, q dbExecutor, key string) (JobView, error) {
	job, err := scanJob(q.QueryRow(ctx, jobSelect+` WHERE idempotency_key = $1`, key))
	if errors.Is(err, pgx.ErrNoRows) {
		return JobView{}, ErrNotFound
	}
	return job, err
}

func (s *Service) stateError(ctx context.Context, id uuid.UUID, workerID string) error {
	var status, owner string
	if err := s.Pool.QueryRow(ctx, `SELECT status, COALESCE(locked_by, '') FROM jobs WHERE id = $1`, id).Scan(&status, &owner); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if status == "running" && owner != workerID {
		return ErrForbidden
	}
	return ErrConflict
}

func scanJob(row pgx.Row) (JobView, error) {
	var job JobView
	var payload []byte
	var lockedAt pgtype.Timestamptz
	var lockedBy, lastError, idempotencyKey pgtype.Text
	if err := row.Scan(&job.ID, &job.Type, &payload, &job.Status, &job.Priority, &job.Attempts, &job.MaxAttempts, &job.RunAt, &lockedAt, &lockedBy, &lastError, &idempotencyKey, &job.CreatedAt, &job.UpdatedAt); err != nil {
		return JobView{}, err
	}
	job.Payload = append(json.RawMessage(nil), payload...)
	if lockedAt.Valid {
		value := lockedAt.Time
		job.LockedAt = &value
	}
	job.LockedBy = textValue(lockedBy)
	job.LastError = textValue(lastError)
	job.IdempotencyKey = textValue(idempotencyKey)
	return job, nil
}

func validateEnqueue(input EnqueueInput) (EnqueueInput, error) {
	input.Type = strings.TrimSpace(input.Type)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if input.Type == "" || len([]rune(input.Type)) > 100 || input.Priority < -100 || input.Priority > 100 || len([]rune(input.IdempotencyKey)) > 200 {
		return EnqueueInput{}, ErrValidation
	}
	if len(input.Payload) == 0 {
		input.Payload = json.RawMessage(`{}`)
	}
	if len(input.Payload) > 1<<20 || !json.Valid(input.Payload) {
		return EnqueueInput{}, ErrValidation
	}
	if input.MaxAttempts == 0 {
		input.MaxAttempts = 3
	}
	if input.MaxAttempts < 1 || input.MaxAttempts > 10 {
		return EnqueueInput{}, ErrValidation
	}
	return input, nil
}

func validateJobType(value string) (string, error) {
	value = strings.TrimSpace(value)
	if len([]rune(value)) > 100 {
		return "", ErrValidation
	}
	return value, nil
}

func validateWorker(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || len([]rune(value)) > 100 {
		return "", ErrValidation
	}
	return value, nil
}

func validateListFilter(filter ListFilter) (ListFilter, error) {
	filter.Status = strings.ToLower(strings.TrimSpace(filter.Status))
	filter.Type = strings.TrimSpace(filter.Type)
	if filter.Status != "" && filter.Status != "queued" && filter.Status != "running" && filter.Status != "succeeded" && filter.Status != "failed" && filter.Status != "dead" {
		return ListFilter{}, ErrValidation
	}
	if len([]rune(filter.Type)) > 100 {
		return ListFilter{}, ErrValidation
	}
	if filter.Limit == 0 {
		filter.Limit = 50
	}
	if filter.Limit < 1 || filter.Limit > 200 {
		return ListFilter{}, ErrValidation
	}
	return filter, nil
}

func backoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 10 {
		attempt = 10
	}
	return time.Duration(1<<uint(attempt-1)) * time.Second
}

func textValue(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}
