# Plan 020: Make an import resolve identity, and make it reach the research graph

> **Executor instructions**: Every item here is a gap in the tree-draft import that was
> audited against `docs/ocr-and-import-decision.md`. Fix the cause, add the test that fails
> without the fix, and update the record. None of these may be closed by a comment, and none
> may be closed by silently merging or silently duplicating.

## Status

- **Priority**: P1
- **Effort**: L
- **Risk**: HIGH — this changes what an import writes into the shared lineage graph
- **Depends on**: plan 018 (the corpus and the measured state), and the tree-draft import in
  `services/core-api/internal/sourceprocessing/tree_drafts.go`
- **Category**: feature
- **Planned at**: 2026-10-21

## Why this matters

`docs/import-reading-strategy.md` §5 audits what the tree-draft import actually writes. Most of
it is right: it creates a private draft tree and version, materialises `people`, writes
`tree_relationships` with `status='unresolved'` and a `source_id`, records per-row provenance
in `source_tree_import_node_evidence` and `source_tree_import_edge_evidence`, and keeps a
retraction ledger so an import can be undone without a database restore. That is a
considerable achievement and it closes the materialisation, provenance and reversibility
requirements that an earlier reading of the code thought were open.

Four gaps remain, and this plan closes them.

**Gap 1 — imported people are never matched against existing people.** Every included node
becomes a new `people` row (`tree_drafts.go:487`). Nothing compares it to the rest of the
database. `entity_resolution_candidates` is written in exactly one place —
`internal/entityresolution/service.go:274` — from a run that sweeps an entire entity type,
because `RunInput` carries only `EntityType` and no scope
(`internal/entityresolution/types.go:76-78`). A family tree naming `محمد بن عبدالله` who
already exists therefore creates a **second** person with the same normalised name and no
link to the first. Nothing warns anyone. This is the failure
`docs/ocr-and-import-decision.md:66` names, and it is silent.

**Gap 2 — imported relationships never enter the research graph.** An imported relationship is a
`tree_relationships` row (`tree_drafts.go:513`). It is not an `entity_relationships` row, not a
`claims` row, not a `claim_evidence` row, and there is no `source_statements` row for it. The
import therefore produces a private tree a researcher can look at, and adds nothing to the
evidence-aware lineage graph that the research agent reads, that hybrid retrieval cites, or
that the contradiction detector compares. `source_statements.extraction_method` has accepted
`'imported'` since `db/migrations/0003_research_and_jobs.sql:60` and **no code writes it**.

**Gap 3 — the raw reading is destroyed and the classical name vocabulary is unused.** The
import stores only `identity.NormalizeArabicName(node.NameAR)` (`tree_drafts.go:487`), which
strips the tatweel, folds `أإآٱ→ا`, `ى→ي` and `ة→ه`
(`internal/identity/normalization.go:10`). What the page actually said is gone. Meanwhile
`person_aliases.alias_type` already accepts `kunyah`, `laqab`, `nisbah` and `source_spelling`
— the classical Arabic name components — and the import writes **no** `person_aliases` at all.

**Gap 4 — there is no dry run.** `ImportTreeDraft` (`tree_drafts.go:409`) writes everything in
one transaction, with no phase that changes nothing and reports what *would* happen. The
requirement is explicit at `docs/ocr-and-import-decision.md:396-398`.

Gap 1 and Gap 2 together mean the import does not yet "operate the platform", which is the
whole reason the feature exists.

## Current state

- **The review path cannot create people.** `internal/sourceprocessing/review.go:122-177`
  materialises only a `source_statement` for an entity candidate, and creates a claim only
  when both entity ids already point at existing rows. This is why the import needed its own
  materialisation, and it is why the import's materialisation must not be routed through the
  review path.
- **The entity-resolution surface is complete and reversible.** `entity_resolution_candidates`
  with `score` and `match_class`, `entity_resolution_reviews` with `decision` ∈ approve / reject
  / defer / reopen, and `entity_merges` with `state` ∈ applied / reversed and before/after
  snapshots (`db/migrations/0015_entity_resolution.sql:43`, `:71`, `:83`). `ReviewCandidate`,
  `MergeCandidate` and `ReverseMerge` are at `internal/entityresolution/service.go:559`, `:620`,
  `:705`, with optimistic concurrency on `expected_candidate_version`.
- **The resolution run is a full sweep.** `RunInput` has one field
  (`internal/entityresolution/types.go:76-78`), so a run covers an entire entity type. There is
  no way today to scope a run to the people one import created, which is why the import cannot
  reuse it as-is.
- **The match itself already exists.** `entityReferences`
  (`internal/sourceprocessing/processor.go:725`) is a single UNION ALL over eight arms —
  `people`, `person_aliases`, `families`, `family_aliases`, `tribes`, `tribe_aliases`,
  `places`, `historical_place_names` — matching on normalised name, and `resolveEntity`
  (`processor.go:693`) accepts the first match scoring ≥ 0.5. That is the matching vocabulary
  the import should reuse rather than reinvent.
- **The alias vocabulary is source-scoped and already correct.**
  `internal/identity/validation.go:19-22` makes `alias_type='source_spelling'` the only
  source-scoped alias type, and `internal/identity/people.go:353` already writes aliases with a
  `source_id`.
- **The result type is already the right shape.** `TreeDraftImportResult` carries
  `ImportedPeople`, `ImportedRelations`, `StagedRelations` and `Warnings`
  (`tree_drafts.go:497`, `:503-504`), and `Warnings` already carries the "the tree model cannot
  represent this meaning" case.
- **A research-graph write path exists for statements.** `internal/evidence/service.go:364`
  (`CreateStatement`) and `:422` (`ListClaims`), with `claims`, `claim_versions` and
  `claim_evidence` all present (`db/migrations/0003_research_and_jobs.sql:54`, `:70`, `:91`,
  `:102`).
- **The prohibition on silent merges is already enforced elsewhere.** A relationship is always
  created `unresolved` (`internal/identity/validation.go:33-35`), the model can never
  pre-approve a candidate (`internal/sourceprocessing/processor.go:1085`), and the merge surface
  requires an explicit authorised action. This plan must not weaken any of the three.

## Commands you will need

| Purpose | Command | Expected |
|---|---|---|
| Fast | `make verify` | exit 0 |
| Full | `COMPOSE_PROJECT_NAME=dawha POSTGRES_DB=<clean scratch db> make verify-full` | exit 0 |
| Harness tests | `python3 -m unittest discover -s tools/ocr-eval -p 'test_*.py'` | exit 0 |
| Browser | `COMPOSE_PROJECT_NAME=dawha POSTGRES_DB=<same scratch> make e2e` | exit 0, 0 fixme, 0 skip |

## Scope

**In scope**:

- `services/core-api/internal/sourceprocessing/tree_drafts.go` — the dry run, the collision
  report, the alias writes, the statement writes
- `services/core-api/internal/sourceprocessing/` — a new file for the import's identity
  matching, if the logic does not belong in `tree_drafts.go`
- `services/core-api/internal/entityresolution/` — a scoped run, if that is the chosen way to
  produce candidates
- `db/migrations/` — additive only, and only where a scoped run or a link needs a column
- `services/core-api/internal/httpapi/source_processing_handler.go` and
  `services/core-api/internal/httpapi/router.go` — the dry-run endpoint
- `apps/web/src/components/SourceTreeDraftDialog.tsx`, `apps/web/src/lib/api.ts`,
  `apps/web/src/types.ts` — the dry-run preview
- `docs/` — the correction of the status records

**Out of scope**:

- **Any OCR, geometry or reading change.** That is plan 019, and this plan must work with
  whatever reading path exists.
- **Automatic merging.** The import proposes candidates; a human confirms. The existing
  `entity_resolution_candidates` surface is the mechanism, and this plan must not bypass it,
  shortcut it, or auto-apply it.
- **Publishing a tree.** A published tree remains an explicit, separate authorised action.
- **Historical acceptance.** A reviewed assertion is a claim; it does not become an accepted
  historical fact because it was imported.
- **The lineage-book assertion path.** `book_assertions.go` already writes `source_statements`
  with `extraction_method='manual'` (`book_assertions.go:411`), which is correct: a
  human-entered quotation is manual work. This plan changes the *tree* path, and leaves the
  book path alone.
- **Changing the upload contract or the reading pipeline.**

## Steps

### Step 1: Make the dry run a real phase

`ImportTreeDraft` must gain a mode that changes nothing.

- **The dry run computes everything the apply run computes** — the same people, the same
  relationships, the same aliases, the same statements, the same candidates — and writes none
  of it. It is the same code path with the writes suppressed, not a parallel estimate, so the
  two cannot drift.
- **It reports, in order of the requirement at `docs/ocr-and-import-decision.md:396-398`:** how
  many people would be created, how many would be linked to an existing person, how many
  relationships would be written, how many relationships the platform cannot represent and
  would remain staged, how many statements and aliases would be written, and what could not be
  interpreted at all.
- **It reports collisions.** This is the number the researcher most needs and currently cannot
  get, and it is only computable once step 2 exists. Until then it must say "unknown" rather
  than report zero.
- **The apply run must refuse to proceed if the dry run has not been reviewed**, or must
  re-check the preconditions the dry run reported. A dry run that is advisory rather than
  binding is not a dry run.
- **The preview carries the exact target preconditions** — the draft revision, the source file
  checksum, the counts — so that an apply against a changed draft is refused rather than
  silently applied to different data.

### Step 2: Match imported people against existing people

This is the substantive change and it must not merge anything by itself.

- **Reuse the existing matching vocabulary.** `entityReferences`
  (`internal/sourceprocessing/processor.go:725`) already searches eight entity kinds by
  normalised name, including aliases, and `resolveEntity` (`processor.go:693`) already applies
  a ≥ 0.5 threshold. The import should use the same lookup and the same threshold so that a
  name matched by one path is matched by the other.
- **Propose, never merge.** For each imported occurrence, produce an
  `entity_resolution_candidates` row linking the proposed person to the candidate existing
  person, with `match_class`, `score`, `matching_signals`, `conflicting_signals` and an
  `explanation_ar` in Arabic — the same shape the resolution run writes
  (`internal/entityresolution/service.go:274`). A human then approves, rejects, defers or
  reopens it through the existing surface, and a merge is reversible through `entity_merges`.
- **A candidate must be scoped to the import.** The existing run sweeps an entire entity type
  (`internal/entityresolution/types.go:76-78`), which is the wrong shape for this: an import of
  60 people must not open candidates against the whole database. Either the run gains a scope,
  or the import writes candidates for its own run directly — and if it writes them directly it
  must set `run_id` to a real `entity_resolution_runs` row so the candidates are reviewable
  and auditable like any other.
- **The imported person must not be created until the decision is made.** An occurrence that
  matches an existing person above the threshold should be staged as a *link*, not written as
  a new `people` row and left for a later sweep to reconcile. An occurrence below the threshold
  becomes a new person.
- **The platform's existing prohibitions hold.** No silent merge, no silent duplicate, no
  automatic `identity_status` promotion, and no publication. A linked person keeps its
  `identity_status` unless a person changes it.

### Step 3: Preserve the raw reading, and use the classical name components

- **Every imported occurrence writes a `person_aliases` row** with `alias_type='source_spelling'`,
  the value being the **verbatim** string the reading produced, `normalized_value_ar` being
  `identity.NormalizeArabicName` of it, and `source_id` set to the source it came from. This is
  the source-scoped alias type the schema already has
  (`internal/identity/validation.go:19-22`), and it is what makes a later correction of the
  canonical name non-destructive.
- **The canonical name stays as read, and the raw reading stays beside it.** The point is that
  `canonical_name_ar` is what the reviewer approved, and the alias is what the page said.
- **The classical components are captured where they are identified.** `person_aliases.alias_type`
  already accepts `kunyah`, `laqab`, `nisbah` and `source_spelling`. An import that reads
  `أبو بكر بن أبي قاسم` should be able to record the kunyah and the patronymic chain as
  aliases, so that a later lineage-book reading can match against them. Whether the split is
  made by the reviewer or by a bounded helper is a decision this plan makes explicitly; what
  the plan requires is that the split is **reviewable and correctable**, never inferred
  silently into the canonical name.
- **A dotless or abbreviated reading is preserved verbatim even when it is wrong.** That is the
  point of `source_spelling`. The correction belongs in `canonical_name_ar`; the evidence
  belongs in the alias.

### Step 4: Write the import into the research graph

- **Every imported relationship gets a `source_statements` row** with
  `extraction_method='imported'`, `review_status='accepted'` (a reviewer approved the edge
  before the import), the exact page evidence it came from, and the locator. This uses the
  value the schema has accepted since `db/migrations/0003_research_and_jobs.sql:60` and that
  no code has ever written.
- **Every imported relationship gets an `entity_relationships` row**, `status='unresolved'`,
  `created_by` the importing actor, with `valid_from` / `valid_to` populated when the source
  states them. The predicate vocabulary is closed
  (`internal/identity/validation.go:29`), and the import must not extend it — a relationship
  whose meaning the platform cannot represent stays staged in the draft, exactly as
  `tree_drafts.go:503-504` already does.
- **The statement is the link between the graph and the evidence.** A `claim_evidence` row
  points at the statement and the passage; here the statement points at the page evidence and
  the connector path through `source_tree_import_edge_evidence`. The evidence chain must be
  walkable from a relationship in the graph back to the pixels on the page.
- **The import creates claims only where the platform's claim model can represent them.**
  `claims` requires entity endpoints (`db/migrations/0003_research_and_jobs.sql:70`), so a
  relationship between two occurrences that are not yet people cannot become a claim. It
  becomes an `entity_relationships` row, and a claim once the identities are resolved.
- **The graph rows and the tree rows are written in the same transaction.** A relationship that
  exists in the tree but not in the graph, or the reverse, is an inconsistent import.
- **The retraction ledger must cover the new rows.** `RetractTreeDraft` currently reverses the
  tree; it must also retract the `entity_relationships`, the `source_statements` and the
  aliases, without deleting anything a later researcher has since changed. A row edited after
  the import must not be silently deleted by a retraction — the manifest already records the
  import, so the retraction can refuse to touch a row whose `updated_at` has moved.

### Step 5: The review surface must show it

- **The dry-run preview** is a first-class view, not a log line: the counts, the collisions, the
  staged relationships, the excluded regions, and the target preconditions.
- **A collision is visible as a collision.** An occurrence proposed against an existing person
  shows the existing person, the match score, the match class, and the evidence that produced
  the match — before the import is applied.
- **Nothing in this surface accepts a merge.** The link is a proposal; the merge happens in the
  entity-resolution review surface, which already requires an authorised action and can reverse
  one.

## Test plan

- **The dry run writes nothing.** Every table the apply run would touch is asserted unchanged
  after a dry run, and the reported counts equal the apply run's effects exactly.
- **A collision is detected.** A draft naming a person who already exists produces a
  candidate, and the apply run does **not** create a second `people` row for that occurrence.
- **Nothing merges without a person.** No `entity_merges` row with `state='applied'` exists
  after an import, and no `people.merged_into_id` is set by an import.
- **A candidate is scoped.** An import of 3 people produces candidates only against people the
  import could plausibly match, and the candidate's `run_id` resolves to a real
  `entity_resolution_runs` row.
- **The raw reading survives.** After an import, the `people` row's `canonical_name_ar` is the
  reviewed string and a `person_aliases` row with `alias_type='source_spelling'` holds the
  verbatim reading, with `source_id` set. Normalisation of the alias must not have been applied
  to the stored value.
- **The graph is written.** After an import, an `entity_relationships` row, a
  `source_statements` row with `extraction_method='imported'`, and a `source_tree_import_edge_evidence`
  row exist for a reviewed edge, and the evidence chain resolves from the relationship to the
  page evidence.
- **A relationship the platform cannot represent stays staged** and is reported, and writes no
  `entity_relationships` row.
- **Retraction reverses everything the import wrote**, and refuses to delete a row that was
  modified after the import.
- **Existing prohibitions still hold.** An import publishes nothing, accepts no historical
  claim, and sets no `identity_status` above `unreviewed`.
- **The whole journey is tested against a real PostgreSQL**, not a double, and the test
  leaves no synthetic actor behind — the `tools/dbtestguard` requirement.

## Done criteria

- [ ] A dry run reports people, links, relationships, staged relationships, statements, aliases
      and uninterpretable content, and writes nothing.
- [ ] The apply run refuses to proceed against a draft whose preconditions changed.
- [ ] Every imported occurrence is matched against existing people through the existing
      entity-resolution surface, and **no duplicate person is created for a match**.
- [ ] No import merges, and no import sets `identity_status` above `unreviewed`.
- [ ] Every imported relationship writes a `source_statements` row with
      `extraction_method='imported'` and an `entity_relationships` row, in the same transaction
      as the tree rows.
- [ ] The evidence chain resolves from a graph relationship back to the page evidence.
- [ ] Every imported occurrence writes a `person_aliases` row with
      `alias_type='source_spelling'` holding the verbatim reading.
- [ ] The retraction reverses the graph rows, the statements and the aliases, and refuses to
      touch rows changed since the import.
- [ ] The review surface shows the dry-run preview and every collision as a collision.
- [ ] `docs/ocr-and-import-decision.md` §5 and §8, `docs/phase-status.md` Phase 14 and
      `docs/import-reading-strategy.md` §5 state the corrected position, and no document still
      claims that the import does not reach the research graph.
- [ ] `make verify`, `make verify-full` and `make e2e` pass; `git diff --check` passes.
- [ ] `plans/README.md` updated.

## STOP conditions

- Stop if matching imported people against existing people requires auto-merging to be useful.
  If the only way to avoid duplicates is to merge without a human, the design is wrong: the
  answer is a review queue, not a silent merge.
- Stop if the scoped candidate proposal cannot be made to write `entity_resolution_candidates`
  rows that the existing review surface can actually list and act on. A parallel review surface
  for import candidates would be a second, weaker version of the one that already exists.
- Stop if the evidence chain from a graph relationship back to the page cannot be made
  resolvable. An imported relationship without a resolvable evidence id is a fact, and this
  repository does not create facts.
- Stop if a retraction cannot be made to refuse to delete a row that a later researcher
  changed. A retraction that destroys later work is worse than no retraction.
- Stop if a verification gate fails twice.

## Maintenance notes

- The matching thresholds must be the same ones the rest of the platform uses
  (`internal/sourceprocessing/processor.go:345`). Two thresholds for one decision is the class
  of defect plan 016 exists to prevent.
- `extraction_method='imported'` is now a used value, and any query that branches on
  `extraction_method` must handle it. Check `internal/research/workspace.go` and
  `internal/evidence/service.go` before adding a branch on it.
- The `source_spelling` alias is the only record of what the page actually said. Any future
  change to the import must keep writing it, and any future correction workflow must not
  delete it.
- The books path depends on this one. §6 of `docs/import-reading-strategy.md` argues that the
  database is the constraint set that disambiguates unpointed lineage text; that argument only
  holds if the trees write their aliases, and it is why this plan precedes any book automation.
