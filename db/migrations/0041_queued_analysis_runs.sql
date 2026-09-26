-- Plan 006: let the two synchronous analyses wait in the queue.
--
-- Entity resolution loaded every person and family, scored them in pairs and
-- wrote the result inside the HTTP request that asked for it. A research agent
-- investigation ran eleven stages in one transaction, in one request. Both are
-- slow enough to outlive a request deadline, and both held request resources for
-- the duration. Neither has a natural place to be interrupted half way, and
-- neither can honestly report a result it has not finished computing.
--
-- So the run rows gain what a queued run needs and the status vocabularies gain
-- the one word they were missing.
--
--   'queued' is the new starting state, and it is the point of the change: a run
--   that has been accepted and not yet finished reads as queued, never as
--   succeeded with nothing behind it. A run only reaches 'succeeded' once every
--   stage it was planned for has been written.
--
--   stage records how far a run got, so a resumed attempt can be read rather
--   than guessed at, and a stalled run says which step it stalled on.
--
--   job_id is the queue row that owns the run. It is the join that lets a worker
--   find the work it holds, and it is what a reader uses to tell a run nobody
--   started from a run whose worker died.
--
-- execution_mode gains 'asynchronous' so the row says how it was actually run.
-- The old CHECK asserted the word 'synchronous' as though nothing else were
-- possible, which is the schema version of the assumption this migration
-- removes.
--
-- scope is the investigation's own description of what it was asked: the source
-- it is about, the person, the place, the years. It is a column because a resumed
-- run is a run some other process continues, and a worker that has to reconstruct
-- the question from the run's own row cannot be handed a scope it was not given.
-- The report carries the same shape, but only once a run is finished, which is
-- exactly too late for the attempt that has to finish it.
--
-- started_at and completed_at on entity_resolution_runs are new because the
-- table had no way to say when a run ran; the research agent table already had
-- them. Both are nullable, so a queued run has no start until a worker claims
-- it - which is the honest reading rather than a start stamped at enqueue time.
ALTER TABLE entity_resolution_runs
  ADD COLUMN job_id uuid REFERENCES jobs(id) ON DELETE SET NULL,
  ADD COLUMN stage text NOT NULL DEFAULT 'queued',
  ADD COLUMN started_at timestamptz,
  ADD COLUMN completed_at timestamptz;

ALTER TABLE entity_resolution_runs
  DROP CONSTRAINT entity_resolution_runs_status_check,
  ADD CONSTRAINT entity_resolution_runs_status_check
    CHECK (status IN ('queued', 'running', 'succeeded', 'failed')),
  ALTER COLUMN status SET DEFAULT 'queued';

CREATE INDEX entity_resolution_runs_job_idx ON entity_resolution_runs (job_id) WHERE job_id IS NOT NULL;

ALTER TABLE research_agent_runs
  ADD COLUMN job_id uuid REFERENCES jobs(id) ON DELETE SET NULL,
  ADD COLUMN stage text NOT NULL DEFAULT 'queued',
  ADD COLUMN scope jsonb NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE research_agent_runs
  DROP CONSTRAINT research_agent_runs_status_check,
  ADD CONSTRAINT research_agent_runs_status_check
    CHECK (status IN ('queued', 'running', 'succeeded', 'failed')),
  ALTER COLUMN status SET DEFAULT 'queued';

ALTER TABLE research_agent_runs
  DROP CONSTRAINT research_agent_runs_execution_mode_check,
  ADD CONSTRAINT research_agent_runs_execution_mode_check
    CHECK (execution_mode IN ('synchronous', 'asynchronous'));

CREATE INDEX research_agent_runs_job_idx ON research_agent_runs (job_id) WHERE job_id IS NOT NULL;
