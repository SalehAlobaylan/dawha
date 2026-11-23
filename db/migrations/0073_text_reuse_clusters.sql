-- Plan 034-A: the text-reuse lane's own tables — clusters are EVIDENCE-NEUTRAL
-- (a cluster links passages; it asserts «هذه النصوص متقاربة نصياً» and nothing
-- semantic). The membership invariants the plan pins are enforced HERE and by
-- the engine's own tests:
--
--   * every member carries its passage (FK, no orphan) — no member without a
--     passage;
--   * no cluster with fewer than two members — a single passage is a singleton
--     lineage, and a singleton is its OWN lineage, not an invented cluster;
--   * every passage enters the lane exactly once per pair (the unique key);
--   * the similarity + the excerpt stay with the member (no free-floating
--     numbers).
--
-- Idempotent by the IF NOT EXISTS guards; safe to re-run.
CREATE TABLE IF NOT EXISTS text_reuse_clusters (
  id text PRIMARY KEY,
  method text NOT NULL DEFAULT 'jaccard_shingle_5'
    CHECK (method IN ('jaccard_shingle_5', 'ewolf_verbatim')),
  threshold double precision NOT NULL DEFAULT 0.55
    CHECK (threshold >= 0 AND threshold <= 1),
  pair_count integer NOT NULL DEFAULT 0
    CHECK (pair_count >= 0),
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS text_reuse_members (
  cluster_id text NOT NULL REFERENCES text_reuse_clusters(id) ON DELETE CASCADE,
  passage_id uuid NOT NULL REFERENCES source_passages(id) ON DELETE CASCADE,
  similarity double precision NOT NULL
    CHECK (similarity >= 0 AND similarity <= 1),
  -- The member's own receipt: the excerpt the cluster's preview can show side
  -- by side (a HUMAN-checkable excerpt, not a sample of a sample).
  excerpt text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (cluster_id, passage_id)
);

CREATE INDEX IF NOT EXISTS text_reuse_members_passage_idx ON text_reuse_members (passage_id);
