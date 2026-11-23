-- Plan 030-F: a deterministic order for the append-only decision trails.
--
-- Two decisions on one candidate can land in the same clock tick (a reopen
-- followed by a re-verdict, or the tile lane's two passes), and the tables'
-- primary keys are random UUIDs — so "latest decision per reviewer" ordered
-- by created_at alone is ambiguous exactly when it matters most. A monotonically
-- increasing sequence per table fixes the order for good: the row inserted
-- later is the later decision, always, whoever's clock said what.
--
-- The tile lane derives a reviewer's pass from their latest decision per
-- candidate (`regionworkbench.passVerdicts`), so this ordering is what makes
-- two-person retirement deterministic; the regionreview lane's own history
-- reads use it too.
ALTER TABLE external_region_candidate_decisions
  ADD COLUMN sequence_number bigserial;

ALTER TABLE region_tile_task_events
  ADD COLUMN sequence_number bigserial;
