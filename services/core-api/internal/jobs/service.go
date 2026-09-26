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
	// ErrLeaseLost is the answer a worker gets when the job it is holding is no
	// longer its claim to write: the token on the row is a different one, or the
	// lease ran out before the write arrived. It is deliberately its own error
	// rather than a flavour of ErrConflict, because the caller's response
	// differs - a worker that lost its lease stops and waits to be retried, while
	// a worker facing a conflict is looking at a job somebody else already
	// finished or failed.
	ErrLeaseLost = errors.New("job lease is no longer held")
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
	// LeaseToken is the claim token the worker was handed. It is optional so the
	// operator-facing job API keeps the shape it has always had, and it is
	// checked whenever it is present, so every in-process worker is fenced by it.
	LeaseToken string `json:"lease_token,omitempty"`
}

type FailInput struct {
	WorkerID string `json:"worker_id"`
	Error    string `json:"error"`
	// LeaseToken is the claim token, checked exactly as it is on completion.
	LeaseToken string `json:"lease_token,omitempty"`
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
	// LeaseToken, LeaseExpiresAt and HeartbeatAt are the fencing fields. They
	// are additive, so every existing reader of a job - the HTTP list, the run
	// status endpoints, the operator CLI - sees the same JSON it saw before plus
	// three more keys it ignores.
	LeaseToken     string     `json:"leaseToken,omitempty"`
	LeaseExpiresAt *time.Time `json:"leaseExpiresAt,omitempty"`
	HeartbeatAt    *time.Time `json:"heartbeatAt,omitempty"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

// Lease converts a claimed job into the claim a worker holds.
func (j JobView) Lease(workerID string) Lease {
	expires := j.LeaseExpiresAt
	return Lease{JobID: j.ID, WorkerID: workerID, Token: j.LeaseToken, ExpiresAt: expires}
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
	// LeaseDuration and HeartbeatInterval are fields rather than constants at
	// every use site so a worker can tighten them, and so a test can watch a
	// lease expire inside a second instead of inside a minute and a half. Zero
	// means the package default, so the zero value is a correct service.
	LeaseDuration     time.Duration
	HeartbeatInterval time.Duration
}

// Executor is the slice of pgx the queue needs, exported because a processor
// has to be able to prove its lease inside the same transaction as the rows it
// is protecting, and naming the transaction type it can be handed is part of
// that contract.
type Executor interface {
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

func (s *Service) enqueue(ctx context.Context, q Executor, input EnqueueInput) (EnqueueResult, error) {
	input, err := validateEnqueue(input)
	if err != nil {
		return EnqueueResult{}, err
	}
	var runAt any
	if input.RunAt != nil {
		runAt = input.RunAt.UTC()
	}
	var key any
	if input.IdempotencyKey != "" {
		key = input.IdempotencyKey
	}
	// An immediate job takes run_at from the database rather than from the
	// process that asked for it. Every later comparison against run_at - the
	// claim's `run_at <= now()`, the backoff a failure schedules, the recovery
	// pass - is made by PostgreSQL, so writing a value from another clock is the
	// one way to enqueue a job that is ready and still not claimable. A caller
	// that wants a specific instant passes one, and that is kept as given.
	row := q.QueryRow(ctx, `
		INSERT INTO jobs (type, payload, status, priority, run_at, idempotency_key, max_attempts)
		VALUES ($1, $2, 'queued', $3, COALESCE($4::timestamptz, now()), $5, $6)
		ON CONFLICT (idempotency_key) DO NOTHING
		RETURNING `+jobColumns,
		input.Type, input.Payload, input.Priority, runAt, key, input.MaxAttempts)
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
		UPDATE jobs AS j SET status = 'running', locked_at = now(), locked_by = $1,
		       lease_token = gen_random_uuid(),
		       lease_expires_at = now() + ($3 * interval '1 second'),
		       heartbeat_at = now(),
		       updated_at = now()
		FROM candidate AS c WHERE j.id = c.id
		RETURNING `+prefixedJobColumns,
		input.WorkerID, input.Type, s.claimLeaseSeconds())
	job, err := scanJob(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return JobView{}, ErrNotFound
	}
	return job, err
}

// claimLeaseSeconds is the lifetime the claim is minted with. The claim mints
// the token itself with gen_random_uuid() rather than taking one from the
// caller, because a token the claimant chooses is a token a second claimant can
// guess, and this value is the only thing standing between a stale worker and a
// committed result.
func (s *Service) claimLeaseSeconds() int {
	seconds := int(s.leaseDuration() / time.Second)
	if seconds < 1 {
		seconds = 1
	}
	return seconds
}

// Complete finishes a job on behalf of the worker that owns it.
//
// Two guards, and both of them are needed. The claim token, when the caller
// supplies one, says this is the same claim the worker was handed - a worker
// that lost its lease and came back with a stale token is refused even if its
// worker id still matches. The live-lease predicate says the claim has not run
// out - a worker whose lease expired while it was busy is refused even when
// nothing else has claimed the job yet, because "nobody has taken it" is not a
// promise that nobody will.
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
	input.LeaseToken = strings.TrimSpace(input.LeaseToken)
	job, err := scanJob(s.Pool.QueryRow(ctx, completeJobQuery, id, workerID, nullableLeaseToken(input.LeaseToken)))
	if errors.Is(err, pgx.ErrNoRows) {
		return JobView{}, s.stateError(ctx, id, CompleteInput{WorkerID: workerID, LeaseToken: input.LeaseToken})
	}
	return job, err
}

// Fail records the reason a job failed and requeues or kills it.
//
// The owner check is the same pair Complete uses, for the same reason: a worker
// that lost its claim must not be able to requeue the job out from under the
// worker that now holds it, which is how a job that is being processed
// correctly gets handed to a second worker mid-flight.
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
	input.LeaseToken = strings.TrimSpace(input.LeaseToken)
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return JobView{}, err
	}
	defer tx.Rollback(ctx)
	var status, lockedBy string
	var token pgtype.Text
	var expiresAt pgtype.Timestamptz
	var attempts, maxAttempts int
	if err := tx.QueryRow(ctx, `SELECT status, COALESCE(locked_by, ''), lease_token::text, lease_expires_at, attempts, max_attempts FROM jobs WHERE id = $1 FOR UPDATE`, id).Scan(&status, &lockedBy, &token, &expiresAt, &attempts, &maxAttempts); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return JobView{}, ErrNotFound
		}
		return JobView{}, err
	}
	if status != "running" || lockedBy != workerID {
		return JobView{}, ErrConflict
	}
	if err := matchLease(token, expiresAt, input.LeaseToken); err != nil {
		return JobView{}, err
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

// matchLease is the shared decision behind Renew, Complete and Fail. It answers
// one question - is the caller still the owner of this claim? - and returns the
// error a worker acts on, so the three paths cannot drift apart.
//
// An empty expected token is the operator-facing path, which predates leases and
// has no claim to present. It still requires a live lease, so an expired claim
// is refused whichever way the caller identifies itself.
func matchLease(token pgtype.Text, expiresAt pgtype.Timestamptz, expected string) error {
	if expected != "" && textValue(token) != expected {
		return ErrForbidden
	}
	if !expiresAt.Valid || !expiresAt.Time.After(time.Now()) {
		return ErrLeaseLost
	}
	return nil
}

func nullableLeaseToken(token string) any {
	if token == "" {
		return nil
	}
	return token
}

// RecoverStale hands abandoned work back to the queue, whichever type it is.
func (s *Service) RecoverStale(ctx context.Context, olderThan time.Duration) (RecoverResult, error) {
	return s.recoverStale(ctx, olderThan, "")
}

// RecoverStaleOfType is the same recovery narrowed to one job type.
//
// Once more than one consumer shares the queue, the unfiltered recovery belongs
// to whichever process runs it first: it takes the job out of running, and
// whoever gets there second never sees it. A consumer that owns the run row of
// a job type therefore has to be the one that recovers that type, or the run row
// is left reading as running work that nobody owns.
func (s *Service) RecoverStaleOfType(ctx context.Context, olderThan time.Duration, jobType string) (RecoverResult, error) {
	return s.recoverStale(ctx, olderThan, strings.TrimSpace(jobType))
}

func (s *Service) recoverStale(ctx context.Context, olderThan time.Duration, jobType string) (RecoverResult, error) {
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
			WHERE status = 'running' AND locked_at < now() - ($1 * interval '1 second') AND ($2 = '' OR type = $2)
			FOR UPDATE SKIP LOCKED
		)
		UPDATE jobs AS j SET status = CASE WHEN j.attempts + 1 >= j.max_attempts THEN 'dead' ELSE 'queued' END,
		       attempts = j.attempts + 1,
		       run_at = CASE WHEN j.attempts + 1 >= j.max_attempts THEN now() ELSE now() + (interval '1 second' * power(2, LEAST(j.attempts, 10))) END,
		       locked_at = NULL, locked_by = NULL,
		       lease_token = NULL, lease_expires_at = NULL, heartbeat_at = NULL,
		       last_error = COALESCE(j.last_error, 'worker lock expired'), updated_at = now()
		FROM stale AS s WHERE j.id = s.id
		RETURNING `+prefixedJobColumns, seconds, jobType)
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

// jobColumns is the one list of columns every job read and every job write
// returns. It used to be spelled out on each of the eight statements that needed
// it, which is how a fencing column gets added to the claim and forgotten on the
// recovery path; one constant is the fix.
const jobColumns = `id, type, payload, status, priority, attempts, max_attempts, run_at, locked_at, locked_by, last_error, idempotency_key, lease_token::text, lease_expires_at, heartbeat_at, created_at, updated_at`

// prefixedJobColumns is the same list for the one statement that aliases the
// target table. `id` exists on both sides of the claim's join, so an unqualified
// RETURNING there would be ambiguous rather than merely untidy.
const prefixedJobColumns = `j.id, j.type, j.payload, j.status, j.priority, j.attempts, j.max_attempts, j.run_at, j.locked_at, j.locked_by, j.last_error, j.idempotency_key, j.lease_token::text, j.lease_expires_at, j.heartbeat_at, j.created_at, j.updated_at`

const jobSelect = `SELECT ` + jobColumns + ` FROM jobs`

// completeJobQuery fences on the claim and on the clock. `$3` is the caller's
// lease token, or NULL for the operator-facing call that has no claim to
// present; either way `lease_expires_at > now()` applies, so a claim that ran
// out cannot be completed by anyone who is not the current owner.
const completeJobQuery = `
	UPDATE jobs SET status = 'succeeded', run_at = now(), last_error = NULL,
	       locked_at = NULL, locked_by = NULL,
	       lease_token = NULL, lease_expires_at = NULL, heartbeat_at = NULL, updated_at = now()
	WHERE id = $1 AND status = 'running' AND locked_by = $2
	  AND ($3::uuid IS NULL OR lease_token = $3)
	  AND lease_expires_at > now()
	RETURNING ` + jobColumns

// renewJobQuery is the heartbeat. It extends the lease and refreshes locked_at
// in the same statement, so the fifteen-minute stale-recovery window reads a
// live worker as live without the window itself having to shrink.
const renewJobQuery = `
	UPDATE jobs SET locked_at = now(), lease_expires_at = now() + ($3 * interval '1 second'), heartbeat_at = now(), updated_at = now()
	WHERE id = $1 AND status = 'running' AND locked_by = $2 AND lease_token = $4 AND lease_expires_at > now()
	RETURNING ` + jobColumns

// renewJobExec is the same update without the RETURNING, for the caller that
// only needs the row count - a transaction that is about to write and wants to
// know it is still allowed to.
const renewJobExec = `
	UPDATE jobs SET locked_at = now(), lease_expires_at = now() + ($3 * interval '1 second'), heartbeat_at = now(), updated_at = now()
	WHERE id = $1 AND status = 'running' AND locked_by = $2 AND lease_token = $4 AND lease_expires_at > now()`

const failJobQuery = `
	UPDATE jobs SET status = $1, run_at = $2, last_error = $3, attempts = attempts + 1,
	       locked_at = NULL, locked_by = NULL,
	       lease_token = NULL, lease_expires_at = NULL, heartbeat_at = NULL, updated_at = now()
	WHERE id = $4
	RETURNING ` + jobColumns

func (s *Service) byIdempotencyKey(ctx context.Context, q Executor, key string) (JobView, error) {
	job, err := scanJob(q.QueryRow(ctx, jobSelect+` WHERE idempotency_key = $1`, key))
	if errors.Is(err, pgx.ErrNoRows) {
		return JobView{}, ErrNotFound
	}
	return job, err
}

// stateError explains a refused write. It reads the same three facts the update
// checked - who owns the row, under which token, and whether the lease is still
// live - so the error a worker sees names the reason the database gave rather
// than a guess made from the worker string alone.
func (s *Service) stateError(ctx context.Context, id uuid.UUID, input CompleteInput) error {
	var status, owner string
	var token pgtype.Text
	var expiresAt pgtype.Timestamptz
	if err := s.Pool.QueryRow(ctx, `SELECT status, COALESCE(locked_by, ''), lease_token::text, lease_expires_at FROM jobs WHERE id = $1`, id).Scan(&status, &owner, &token, &expiresAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if status != "running" {
		return ErrConflict
	}
	if owner != input.WorkerID {
		return ErrForbidden
	}
	return matchLease(token, expiresAt, strings.TrimSpace(input.LeaseToken))
}

func scanJob(row pgx.Row) (JobView, error) {
	var job JobView
	var payload []byte
	var lockedAt, leaseExpiresAt, heartbeatAt pgtype.Timestamptz
	var lockedBy, lastError, idempotencyKey, leaseToken pgtype.Text
	if err := row.Scan(&job.ID, &job.Type, &payload, &job.Status, &job.Priority, &job.Attempts, &job.MaxAttempts, &job.RunAt, &lockedAt, &lockedBy, &lastError, &idempotencyKey, &leaseToken, &leaseExpiresAt, &heartbeatAt, &job.CreatedAt, &job.UpdatedAt); err != nil {
		return JobView{}, err
	}
	job.Payload = append(json.RawMessage(nil), payload...)
	if lockedAt.Valid {
		value := lockedAt.Time
		job.LockedAt = &value
	}
	if leaseExpiresAt.Valid {
		value := leaseExpiresAt.Time
		job.LeaseExpiresAt = &value
	}
	if heartbeatAt.Valid {
		value := heartbeatAt.Time
		job.HeartbeatAt = &value
	}
	job.LockedBy = textValue(lockedBy)
	job.LastError = textValue(lastError)
	job.IdempotencyKey = textValue(idempotencyKey)
	job.LeaseToken = textValue(leaseToken)
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
