-- Plan 006: owner-checked lease fencing for the PostgreSQL job queue.
--
-- `locked_by` alone cannot fence a worker. It names a worker, not a claim: the
-- same worker id can hold two claims over the life of a process (a restart, a
-- second process on the same host, a reclaim after stale recovery), so a
-- comparison against it cannot tell "the worker that owns this job right now"
-- from "a worker string that was written once and never cleared". What is
-- missing is a value that changes on every claim and is checked on every write,
-- which is what these three columns add.
--
--   lease_token       minted by the claim itself, so it identifies the claim and
--                     not the process. It is compared on every renewal, every
--                     completion, every failure and every processing write, so a
--                     worker that lost its lease is rejected instead of being
--                     trusted because it still recognises its own name.
--   lease_expires_at  the instant after which the claim is no longer the owner's
--                     to act on. A worker that missed its heartbeat past this
--                     point is fenced even if nothing else has claimed the job,
--                     which is the conservative direction: refusing to write is
--                     recoverable by a retry, writing twice is not.
--   heartbeat_at      when the owner last proved it was alive. Observability, and
--                     the column a reader looks at to explain why a job sat in
--                     running with no progress.
--
-- The stale-recovery window is deliberately unchanged. Recovery still uses
-- locked_at, which a heartbeat also refreshes, so the fifteen-minute window
-- stays the outer safety net for a process that died outright, while the lease
-- is the inner fence that turns "somebody else may own this now" into a fact a
-- worker can act on within a minute.
ALTER TABLE jobs
  ADD COLUMN lease_token uuid,
  ADD COLUMN lease_expires_at timestamptz,
  ADD COLUMN heartbeat_at timestamptz;

-- A running job whose lease has run out is the only state worth finding by
-- lease: a partial index keeps the recovery audit from touching the queued rows
-- that dominate the table.
CREATE INDEX jobs_lease_idx ON jobs (lease_expires_at) WHERE status = 'running';

-- A claim that never gets completed must be recoverable, and so must a claim
-- whose owner is alive but has stopped heartbeating. locked_at is what recovery
-- already reads, so the index that serves it is the one to add.
CREATE INDEX jobs_stale_lock_idx ON jobs (locked_at) WHERE status = 'running';
