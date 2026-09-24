ALTER TABLE research_runs
  ADD COLUMN semantic_route text CHECK (semantic_route IN ('ignore', 'cheap', 'deep')),
  ADD COLUMN semantic_route_model text,
  ADD COLUMN semantic_route_reason text CHECK (semantic_route_reason IN ('noise', 'simple_lookup', 'source_context', 'multi_step', 'contradiction_signal', 'operation_requires_deep', 'uncertainty')),
  ADD COLUMN semantic_route_fallback boolean NOT NULL DEFAULT false,
  ADD COLUMN semantic_route_score double precision CHECK (semantic_route_score IS NULL OR (semantic_route_score >= 0 AND semantic_route_score <= 1)),
  ADD COLUMN semantic_route_source_bearing boolean NOT NULL DEFAULT false,
  ADD COLUMN semantic_route_potential_contradiction boolean NOT NULL DEFAULT false,
  ADD COLUMN semantic_route_continue_investigation boolean NOT NULL DEFAULT false,
  ADD COLUMN synthesis_attempted boolean NOT NULL DEFAULT false;

CREATE INDEX research_runs_semantic_route_idx ON research_runs (semantic_route, created_at DESC);
