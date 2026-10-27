-- What the reviewer saw when they decided, kept beside the decision.
--
-- The first suggestion ledger recorded the machine's reading and the reviewer's
-- answer. That answers "was the reader right?" It cannot answer "was the right
-- name even in the candidate list?" or "did the ranking put it near the top?"
-- This migration adds the evidence the review bench already has at decision
-- time: the shown candidates, the one the reviewer chose, how it was entered, an
-- optional judge answer, and the versioned normalizer/pool that produced it.
--
-- The candidate snapshot is REVIEW EVIDENCE, not a suggestion and not a fact.
-- Machine and AI candidates are stored as "what the reviewer saw"; the machine's
-- reading remains in source_tree_suggestions.suggested_text_ar and the
-- reviewer's answer remains in decided_text_ar. Nothing here accepts a name.
ALTER TABLE source_tree_suggestion_decisions
  ADD COLUMN candidate_snapshot jsonb NOT NULL DEFAULT '[]'::jsonb
    CHECK (jsonb_typeof(candidate_snapshot) = 'array'),
  ADD COLUMN selected_candidate text,
  ADD COLUMN input_method text NOT NULL DEFAULT 'legacy'
    CHECK (input_method IN ('legacy', 'reader', 'candidate', 'typed', 'none')),
  ADD COLUMN judge_output jsonb NOT NULL DEFAULT '{}'::jsonb
    CHECK (jsonb_typeof(judge_output) = 'object'),
  ADD COLUMN normalizer_version text NOT NULL DEFAULT 'legacy',
  ADD COLUMN candidate_pool_version text NOT NULL DEFAULT 'legacy',
  ADD COLUMN duration_ms integer
    CHECK (duration_ms IS NULL OR duration_ms >= 0);
