-- New evidence semantics live beside the original ledger. No old decision is
-- updated, reinterpreted, or given a judge result after its answer is known.
CREATE TABLE source_name_review_boundary (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 installed_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 version text NOT NULL UNIQUE DEFAULT 'name-review-v2'
);
INSERT INTO source_name_review_boundary(version) VALUES ('name-review-v2');
CREATE TABLE source_name_lexicon_revisions (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 source_file_id uuid NOT NULL REFERENCES source_files(id) ON DELETE CASCADE,
 revision text NOT NULL CHECK(length(revision)=64),
 context jsonb NOT NULL CHECK(jsonb_typeof(context)='object'),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(source_file_id,revision)
);
CREATE TABLE source_name_evaluation_groups (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 created_by uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 group_key text NOT NULL UNIQUE CHECK(length(group_key)=64),
 partition text NOT NULL CHECK(partition IN ('development','held_out')),
 contract jsonb NOT NULL CHECK(jsonb_typeof(contract)='object'),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE source_name_evaluation_members (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 source_file_id uuid NOT NULL UNIQUE REFERENCES source_files(id) ON DELETE CASCADE,
 group_id uuid NOT NULL REFERENCES source_name_evaluation_groups(id) ON DELETE CASCADE,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
-- Plan 026 will append repair/split/merge edges through the same provenance
-- contract. History anywhere in the connected occurrence lineage disallows a
-- new blind observation, including a deferral or an old reviewed parent.
CREATE TABLE source_name_occurrence_lineage (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 parent_id uuid NOT NULL REFERENCES source_tree_suggestions(id) ON DELETE CASCADE,
 child_id uuid NOT NULL REFERENCES source_tree_suggestions(id) ON DELETE CASCADE,
 kind text NOT NULL CHECK(kind IN ('repair','split','merge')),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 CHECK(parent_id<>child_id), UNIQUE(parent_id,child_id)
);
CREATE INDEX source_name_lineage_child ON source_name_occurrence_lineage(child_id);
CREATE TABLE source_name_review_exposures (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 suggestion_id uuid NOT NULL UNIQUE REFERENCES source_tree_suggestions(id) ON DELETE CASCADE,
 lexicon_id uuid NOT NULL REFERENCES source_name_lexicon_revisions(id) ON DELETE CASCADE,
 context jsonb NOT NULL CHECK(jsonb_typeof(context)='object'),
 eligible boolean NOT NULL,
 eligibility_reason text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 deadline timestamptz NOT NULL DEFAULT clock_timestamp()+interval '15 seconds',
 sealed_at timestamptz
);
CREATE TABLE source_name_candidate_preparations (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 exposure_id uuid NOT NULL UNIQUE REFERENCES source_name_review_exposures(id) ON DELETE CASCADE,
 suggestion_id uuid NOT NULL REFERENCES source_tree_suggestions(id) ON DELETE CASCADE,
 signature text NOT NULL CHECK(length(signature)=64),
 snapshot jsonb NOT NULL CHECK(jsonb_typeof(snapshot)='array'),
 provenance jsonb NOT NULL CHECK(jsonb_typeof(provenance)='array'),
 versions jsonb NOT NULL CHECK(jsonb_typeof(versions)='object'),
 eligible boolean NOT NULL,
 eligibility_reason text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX source_name_preparation_suggestion ON source_name_candidate_preparations(suggestion_id);
CREATE TABLE source_name_judge_requests (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 preparation_id uuid NOT NULL UNIQUE REFERENCES source_name_candidate_preparations(id) ON DELETE CASCADE,
 signature text NOT NULL CHECK(length(signature)=64),
 payload jsonb NOT NULL CHECK(jsonb_typeof(payload)='object'),
 identity jsonb NOT NULL CHECK(jsonb_typeof(identity)='object'),
 job_id uuid REFERENCES jobs(id) ON DELETE SET NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 expires_at timestamptz NOT NULL DEFAULT clock_timestamp()+interval '7 days'
);
CREATE TABLE source_name_judge_results (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 request_id uuid NOT NULL UNIQUE REFERENCES source_name_judge_requests(id) ON DELETE CASCADE,
 signature text NOT NULL CHECK(length(signature)=64),
 result jsonb NOT NULL CHECK(jsonb_typeof(result)='object'),
 latency_ms integer NOT NULL CHECK(latency_ms BETWEEN 0 AND 120000),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE source_name_decision_evidence (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 decision_id uuid NOT NULL UNIQUE REFERENCES source_tree_suggestion_decisions(id) ON DELETE CASCADE,
 exposure_id uuid REFERENCES source_name_review_exposures(id) ON DELETE SET NULL,
 preparation_id uuid REFERENCES source_name_candidate_preparations(id) ON DELETE SET NULL,
 schema_version text NOT NULL CHECK(schema_version='name-review-v2'),
 gap_reason text NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);

-- Reading metadata returned through a legacy client has no provable before-edit
-- boundary. Such an occurrence can still be reviewed, but never gains a new
-- blind boundary on reopening in the new client.
CREATE TABLE source_name_untracked_exposures (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 suggestion_id uuid NOT NULL UNIQUE REFERENCES source_tree_suggestions(id) ON DELETE CASCADE,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
