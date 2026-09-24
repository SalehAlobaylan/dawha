-- name: ClaimJobs :many
WITH claimed AS (
  SELECT id
  FROM jobs
  WHERE status = 'queued' AND run_at <= now()
  ORDER BY priority DESC, run_at ASC
  FOR UPDATE SKIP LOCKED
  LIMIT sqlc.arg(limit_count)
)
UPDATE jobs
SET status = 'running', locked_at = now(), locked_by = sqlc.arg(worker_id), attempts = attempts + 1, updated_at = now()
FROM claimed
WHERE jobs.id = claimed.id
RETURNING jobs.*;

-- name: CompleteJob :exec
UPDATE jobs
SET status = 'succeeded', locked_at = NULL, locked_by = NULL, updated_at = now()
WHERE id = $1 AND status = 'running';

-- name: FailJob :exec
UPDATE jobs
SET status = CASE WHEN attempts >= sqlc.arg(max_attempts) THEN 'dead' ELSE 'queued' END,
    run_at = now() + make_interval(secs => sqlc.arg(backoff_seconds)::integer),
    locked_at = NULL,
    locked_by = NULL,
    last_error = sqlc.arg(error_message),
    updated_at = now()
WHERE id = sqlc.arg(job_id);
