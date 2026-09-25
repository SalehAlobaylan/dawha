-- apps/web/e2e/cleanup.sql
--
-- Removes exactly the rows a browser acceptance run created, and nothing else.
--
-- The scope is the synthetic actors the suite registers: users whose address is
-- e2e-<label>-<random>@example.invalid (e2e/fixtures.ts newAccount), plus
-- everything reachable from them. The ~480 seeded and development users, their
-- trees, and the demo seed are never touched, because every statement below is
-- scoped to those ids and the transaction fails loudly rather than widening.
--
-- Why this is not a blanket DELETE FROM users: about fifty tables reference
-- users(id) without ON DELETE CASCADE (trees.owner_id, sources.created_by,
-- audit_log.actor_id, tree_invitations.inviter_id and the rest), so that
-- statement would abort. The deletes below are therefore in dependency order,
-- leaf first, and the tables that do cascade are left to cascade.
--
-- The whole thing is one transaction, so a failure leaves the database exactly
-- as it was found rather than half-cleaned. Two callers run this file: the
-- Playwright global teardown (apps/web/e2e/global-teardown.ts, through pg, on
-- every run) and `make e2e-clean` (through psql, for leftovers from a run that
-- was killed before its teardown).
--
-- If a future migration adds a table the suite populates whose foreign key to
-- one of these parents is neither CASCADE nor SET NULL, the delete of that
-- parent aborts the transaction and the run reports it instead of leaking the
-- rows silently. That is the intended failure mode.
--
-- One row class is deliberately left behind: the research run a signed-out
-- journey creates. An unauthenticated query has no actor_id and no question_id,
-- so there is nothing to connect it to an e2e-* account, and the only way to
-- find it is a time window - which is also how a cleanup ends up deleting
-- somebody's own work. It is counted in the report at the end of this file and
-- printed by the teardown, so the residue is visible instead of silent.

BEGIN;

-- The synthetic set, snapshotted before anything is deleted.
CREATE TEMP TABLE synthetic_users ON COMMIT DROP AS
  SELECT id FROM users WHERE email LIKE 'e2e-%@example.invalid';

CREATE TEMP TABLE synthetic_trees ON COMMIT DROP AS
  SELECT id FROM trees
   WHERE owner_id IN (SELECT id FROM synthetic_users)
      OR forked_by IN (SELECT id FROM synthetic_users);

CREATE TEMP TABLE synthetic_tree_versions ON COMMIT DROP AS
  SELECT id FROM tree_versions
   WHERE created_by IN (SELECT id FROM synthetic_users)
      OR published_by IN (SELECT id FROM synthetic_users)
      OR tree_id IN (SELECT id FROM synthetic_trees);

CREATE TEMP TABLE synthetic_sources ON COMMIT DROP AS
  SELECT id FROM sources WHERE created_by IN (SELECT id FROM synthetic_users);

CREATE TEMP TABLE synthetic_claims ON COMMIT DROP AS
  SELECT id FROM claims WHERE created_by IN (SELECT id FROM synthetic_users);

CREATE TEMP TABLE synthetic_questions ON COMMIT DROP AS
  SELECT id FROM open_questions WHERE created_by IN (SELECT id FROM synthetic_users);

CREATE TEMP TABLE synthetic_people ON COMMIT DROP AS
  SELECT id FROM people WHERE created_by IN (SELECT id FROM synthetic_users);

CREATE TEMP TABLE synthetic_tree_nodes ON COMMIT DROP AS
  SELECT id FROM tree_nodes WHERE tree_version_id IN (SELECT id FROM synthetic_tree_versions);

CREATE TEMP TABLE synthetic_source_files ON COMMIT DROP AS
  SELECT id FROM source_files WHERE source_id IN (SELECT id FROM synthetic_sources);

-- A proposal from a signed-out visitor carries no submitter, so it is found
-- through the tree it was made on rather than through an actor.
CREATE TEMP TABLE synthetic_suggestions ON COMMIT DROP AS
  SELECT id FROM suggestions
   WHERE submitted_by IN (SELECT id FROM synthetic_users)
      OR tree_id IN (SELECT id FROM synthetic_trees)
      OR version_id IN (SELECT id FROM synthetic_tree_versions)
      OR node_id IN (SELECT id FROM synthetic_tree_nodes)
      OR question_id IN (SELECT id FROM synthetic_questions);

CREATE TEMP TABLE synthetic_disputes ON COMMIT DROP AS
  SELECT id FROM disputes
   WHERE created_by IN (SELECT id FROM synthetic_users)
      OR resolved_by IN (SELECT id FROM synthetic_users);

-- 1. Rows that hang off a source, a claim or a person through a foreign key with
--    no ON DELETE CASCADE, so they have to go before their parent does.
DELETE FROM spatial_evidence
 WHERE source_passage_id IN (SELECT id FROM source_passages WHERE source_id IN (SELECT id FROM synthetic_sources));
DELETE FROM geographic_associations
 WHERE source_id IN (SELECT id FROM synthetic_sources)
    OR claim_id IN (SELECT id FROM synthetic_claims);
DELETE FROM migration_events
 WHERE source_id IN (SELECT id FROM synthetic_sources)
    OR claim_id IN (SELECT id FROM synthetic_claims);
DELETE FROM person_aliases WHERE person_id IN (SELECT id FROM synthetic_people);
DELETE FROM claim_evidence WHERE claim_id IN (SELECT id FROM synthetic_claims);
-- suggestion_reviews.suggestion_id cascades, but a review by a synthetic
-- reviewer of somebody else's suggestion is still a synthetic row.
DELETE FROM suggestion_reviews WHERE reviewer_id IN (SELECT id FROM synthetic_users);
DELETE FROM suggestion_change_sets WHERE suggestion_id IN (SELECT suggestions.id FROM suggestions WHERE submitted_by IN (SELECT id FROM synthetic_users));
DELETE FROM question_notes
 WHERE created_by IN (SELECT id FROM synthetic_users)
    OR question_id IN (SELECT id FROM synthetic_questions);

-- 2. Analyses that RESTRICT the deletion of a tree or a tree version. Nothing in
--    the journeys runs them today, and the delete of the tree would abort if one
--    ever did.
DELETE FROM platform_findings WHERE created_by IN (SELECT id FROM synthetic_users);
DELETE FROM geospatial_intelligence_runs
 WHERE requested_by IN (SELECT id FROM synthetic_users)
    OR tree_id IN (SELECT id FROM synthetic_trees);
DELETE FROM temporal_analysis_runs
 WHERE requested_by IN (SELECT id FROM synthetic_users)
    OR tree_id IN (SELECT id FROM synthetic_trees);
DELETE FROM research_agent_runs
 WHERE requested_by IN (SELECT id FROM synthetic_users)
    OR tree_id IN (SELECT id FROM synthetic_trees);
-- research_agent_evidence and research_question_candidates cascade from
-- research_agent_runs; their claim, source and statement edges are SET NULL, so
-- nothing points at a row that is about to disappear.

-- 3. Collaboration. tree_invitations.inviter_id, tree_collaborators.invited_by,
--    tree_forks.forked_by and tree_forks.parent_tree_id are all NO ACTION, so
--    the user delete and the tree delete both depend on these going first.
DELETE FROM tree_invitations
 WHERE inviter_id IN (SELECT id FROM synthetic_users)
    OR invitee_user_id IN (SELECT id FROM synthetic_users)
    OR tree_id IN (SELECT id FROM synthetic_trees);
DELETE FROM tree_collaborators
 WHERE invited_by IN (SELECT id FROM synthetic_users)
    OR user_id IN (SELECT id FROM synthetic_users)
    OR tree_id IN (SELECT id FROM synthetic_trees);
DELETE FROM tree_forks
 WHERE forked_by IN (SELECT id FROM synthetic_users)
    OR parent_tree_id IN (SELECT id FROM synthetic_trees)
    OR parent_version_id IN (SELECT id FROM synthetic_tree_versions)
    OR tree_id IN (SELECT id FROM synthetic_trees);

-- 4. The tree itself, deepest row first. tree_nodes.person_id is NO ACTION, so a
--    node has to go before the person it names.
DELETE FROM tree_change_log
 WHERE tree_id IN (SELECT id FROM synthetic_trees)
    OR tree_version_id IN (SELECT id FROM synthetic_tree_versions);
DELETE FROM tree_relationships WHERE tree_version_id IN (SELECT id FROM synthetic_tree_versions);
DELETE FROM tree_nodes WHERE tree_version_id IN (SELECT id FROM synthetic_tree_versions);
-- suggestions.tree_id cascades, but suggestions.question_id is SET NULL, so a
-- suggestion raised against a synthetic question is not covered by either.
DELETE FROM suggestions
 WHERE id IN (SELECT id FROM synthetic_suggestions);
DELETE FROM tree_versions
 WHERE created_by IN (SELECT id FROM synthetic_users)
    OR published_by IN (SELECT id FROM synthetic_users)
    OR tree_id IN (SELECT id FROM synthetic_trees);
DELETE FROM trees WHERE id IN (SELECT id FROM synthetic_trees);

-- 5. Sources and the work they queued. source_statements.source_passage_id is NO
--    ACTION, so a statement has to go before the passage it sits in. A job names
--    its source in the payload rather than in a foreign key, so it is matched on
--    that, and the cast is guarded because a future job type may carry no uuid.
DELETE FROM jobs
 WHERE payload ? 'source_id'
   AND payload->>'source_id' ~ '^[0-9a-fA-F-]{36}$'
   AND (payload->>'source_id')::uuid IN (SELECT id FROM synthetic_sources);
DELETE FROM source_statements WHERE source_id IN (SELECT id FROM synthetic_sources);
DELETE FROM source_passages WHERE source_id IN (SELECT id FROM synthetic_sources);
DELETE FROM sources WHERE id IN (SELECT id FROM synthetic_sources);
-- source_files, source_candidates, source_processing_runs, source_dependencies,
-- source_characterization_runs and question_sources all CASCADE from sources.

-- 6. Claims, questions, disputes and research runs. Their evidence, notes,
--    answers and graph rows cascade from these four.
DELETE FROM claims WHERE id IN (SELECT id FROM synthetic_claims);
DELETE FROM open_questions WHERE id IN (SELECT id FROM synthetic_questions);
DELETE FROM disputes WHERE id IN (SELECT id FROM synthetic_disputes);
DELETE FROM research_runs WHERE actor_id IN (SELECT id FROM synthetic_users);

-- 7. People. tree_nodes are already gone (step 4) and temporal_analysis_runs
--    names a person with NO ACTION (step 2), so this delete can only fail if
--    somebody's own work cites a synthetic person, which is worth a loud abort.
DELETE FROM people WHERE id IN (SELECT id FROM synthetic_people);

-- 8. The actors themselves, and the audit trail of the run. audit_log.actor_id is
--    NO ACTION, so the trail has to go before the users do - and it is matched on
--    the entity as well as the actor, because the background worker and an
--    anonymous visitor both write audit rows with no actor at all. An audit row
--    about a synthetic entity is a row of this run's, whatever wrote it.
DELETE FROM audit_log
 WHERE actor_id IN (SELECT id FROM synthetic_users)
    OR entity_id IN (SELECT id FROM synthetic_trees)
    OR entity_id IN (SELECT id FROM synthetic_tree_versions)
    OR entity_id IN (SELECT id FROM synthetic_sources)
    OR entity_id IN (SELECT id FROM synthetic_source_files)
    OR entity_id IN (SELECT id FROM synthetic_suggestions)
    OR entity_id IN (SELECT id FROM synthetic_people)
    OR entity_id IN (SELECT id FROM synthetic_questions)
    OR entity_id IN (SELECT id FROM synthetic_claims);
-- user_credentials, user_profiles, user_roles and auth_sessions cascade from users.
DELETE FROM users WHERE id IN (SELECT id FROM synthetic_users);

-- 9. The proof, inside the same transaction, so a failure rolls the whole thing
--    back rather than leaving a half-cleaned database.
--
--    Every foreign key in this schema is immediate and validated (checked in the
--    loop below), which is what makes an ordered delete sufficient: PostgreSQL
--    verifies such a constraint at the end of every statement, so a delete that
--    completes cannot have left a child pointing at a row that is gone. A
--    deferrable or NOT VALID constraint would void that argument, so the count
--    of those is asserted to be zero rather than assumed.
SET CONSTRAINTS ALL IMMEDIATE;

DO $$
DECLARE
  soft_constraints integer;
  synthetic_left integer;
  orphans integer;
BEGIN
  SELECT count(*) INTO soft_constraints
    FROM pg_constraint
   WHERE contype = 'f'
     AND connamespace = 'public'::regnamespace
     AND (condeferrable OR NOT convalidated);
  IF soft_constraints > 0 THEN
    RAISE EXCEPTION 'e2e cleanup: % foreign keys are deferrable or NOT VALID, so a successful delete no longer proves the absence of orphans', soft_constraints;
  END IF;

  SELECT count(*) INTO synthetic_left
    FROM users
   WHERE email LIKE 'e2e-%@example.invalid';
  IF synthetic_left > 0 THEN
    RAISE EXCEPTION 'e2e cleanup: % synthetic users survived the delete', synthetic_left;
  END IF;

  -- A nullable foreign key is not an orphan: 22 seeded audit rows and 4 seeded
  -- sources carry a NULL actor, which is a fact about those rows rather than a
  -- dangling reference. Only a non-NULL value with no parent counts.
  SELECT
      (SELECT count(*) FROM tree_versions v WHERE v.tree_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM trees t WHERE t.id = v.tree_id))
    + (SELECT count(*) FROM tree_nodes n WHERE n.tree_version_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM tree_versions v WHERE v.id = n.tree_version_id))
    + (SELECT count(*) FROM tree_relationships r WHERE r.tree_version_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM tree_versions v WHERE v.id = r.tree_version_id))
    + (SELECT count(*) FROM tree_nodes n WHERE n.person_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM people p WHERE p.id = n.person_id))
    + (SELECT count(*) FROM source_files f WHERE f.source_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM sources s WHERE s.id = f.source_id))
    + (SELECT count(*) FROM source_passages p WHERE p.source_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM sources s WHERE s.id = p.source_id))
    + (SELECT count(*) FROM source_statements s WHERE s.source_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM sources src WHERE src.id = s.source_id))
    + (SELECT count(*) FROM tree_invitations i WHERE i.inviter_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM users u WHERE u.id = i.inviter_id))
    + (SELECT count(*) FROM tree_collaborators c WHERE c.invited_by IS NOT NULL AND NOT EXISTS (SELECT 1 FROM users u WHERE u.id = c.invited_by))
    + (SELECT count(*) FROM sources s WHERE s.created_by IS NOT NULL AND NOT EXISTS (SELECT 1 FROM users u WHERE u.id = s.created_by))
    + (SELECT count(*) FROM open_questions q WHERE q.created_by IS NOT NULL AND NOT EXISTS (SELECT 1 FROM users u WHERE u.id = q.created_by))
    + (SELECT count(*) FROM question_notes n WHERE n.created_by IS NOT NULL AND NOT EXISTS (SELECT 1 FROM users u WHERE u.id = n.created_by))
    + (SELECT count(*) FROM claims c WHERE c.created_by IS NOT NULL AND NOT EXISTS (SELECT 1 FROM users u WHERE u.id = c.created_by))
    + (SELECT count(*) FROM people p WHERE p.created_by IS NOT NULL AND NOT EXISTS (SELECT 1 FROM users u WHERE u.id = p.created_by))
    + (SELECT count(*) FROM audit_log a WHERE a.actor_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM users u WHERE u.id = a.actor_id))
    + (SELECT count(*) FROM trees t WHERE t.owner_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM users u WHERE u.id = t.owner_id))
  INTO orphans;
  IF orphans > 0 THEN
    RAISE EXCEPTION 'e2e cleanup: % rows would be left without their parent', orphans;
  END IF;
END $$;

COMMIT;

-- What a caller sees when it runs this file by hand. The last column is the one
-- row class the suite cannot clean behind its own back: a research run started
-- by a signed-out visitor has no actor and no question, so nothing links it to
-- an e2e-* account. It is reported rather than deleted, because it looks exactly
-- like a research query a developer ran in their own session - and one of those
-- exists in this repository's development database right now. The Playwright
-- teardown prints the same number at the end of every run.
SELECT
  (SELECT count(*) FROM users WHERE email LIKE 'e2e-%@example.invalid') AS synthetic_users_left,
  (SELECT count(*) FROM trees WHERE owner_id NOT IN (SELECT id FROM users WHERE email LIKE 'e2e-%@example.invalid')) AS non_synthetic_trees_kept,
  (SELECT count(*) FROM sources WHERE created_by NOT IN (SELECT id FROM users WHERE email LIKE 'e2e-%@example.invalid')) AS non_synthetic_sources_kept,
  (SELECT count(*) FROM research_runs WHERE actor_id IS NULL AND question_id IS NULL) AS unattributable_research_runs_left;
