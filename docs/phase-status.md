# Phase status: what is built, and what is proved

**Read this before any other document in this repository.** `IMPLEMENTATION_PLAN.md`
is a description of an intended system. This file is a description of the one in
this checkout, and where the two disagree, this file is the one that is true.

The document answers two questions a reviewer has to be able to answer without
reading commit history:

1. For each of Phases 0-24, is there code, and is the phase's own acceptance
   criteria list satisfied with evidence?
2. Of the eighteen things that make up V1
   (`IMPLEMENTATION_PLAN.md:1964-1992`), which are proved and which are not?

## The status vocabulary, and the distinction that matters

| Status | Means |
| --- | --- |
| `implemented` | Every acceptance criterion in the phase has code behind it and a test that would fail if the behaviour broke. |
| `partial` | A code slice exists and works, and at least one acceptance criterion of the phase is unmet, unmeasurable, or only reachable through a path the product does not ship. |
| `not started` | No code slice exists. |

**"A code slice exists" is not "V1 acceptance is verified."** All twenty-five
phases in this repository have code. That is not the interesting claim, and on its
own it is close to worthless. The interesting claim is whether the phase's own
acceptance list is satisfied: twenty-one phases are `implemented` on that basis and
two are `partial`, and every table below says which criterion is missing rather
than leaving the reader to infer it from a status word.

Two more words used below:

- **Proved** means: a named test exercises the behaviour, and that test runs in
  `make verify-full` or `make e2e` against a real PostgreSQL.
- **Measured** means: a number exists, with the command that produced it. A
  measurement is not the same as a pass; `docs/graph-benchmark.md` records
  measurements that argue against a change.

Nothing in this document claims a phase is done on the strength of a file existing.
Where the only evidence is a file, the row says so.

---

## Phases 0-24

### Phase 0: Engineering Foundation (`IMPLEMENTATION_PLAN.md:119-228`)

**`implemented`.** Every criterion is met and the "one command" claim is literal.

| Criterion | Evidence |
| --- | --- |
| web app starts locally | `package.json` (`npm run dev`, five supervised processes) |
| Go API starts locally | `services/core-api/cmd/api/main.go` |
| Python AI service starts locally | `services/ai-research/app/main.py` |
| PostGIS and pgvector present | `db/migrations/0001_extensions.sql`, asserted by `services/core-api/tools/dbtestguard/main.go` |
| health checks work | `services/core-api/internal/health`, `/healthz` and `/readyz` in `app/main.py` |
| CI passes | `.github/workflows/ci.yml`, six jobs (web, core-api, generated, security, ai-research, database) |

**Remaining gap:** `npm run dev` needs a working `pip` for `make install` first,
and the compose project name is derived from the working directory, so a second
checkout starts a second PostgreSQL on port 55432. Both are documented in
`README.md`; neither is a code defect.

### Phase 1: Authentication and Authorization (`:229-304`)

**`implemented`.**

| Criterion | Evidence |
| --- | --- |
| registration/login works | `services/core-api/internal/auth`, `apps/web/e2e/journeys/01-register.spec.ts` |
| protected routes work | `services/core-api/internal/httpapi/router.go` |
| collaborator permissions representable | `services/core-api/internal/collaboration/service.go` |
| authorization enforced server-side | in-transaction checks, `TestIdentityAuthorizationMatrix` in `internal/identity/acceptance_postgres_test.go` |
| permission tests exist | `internal/httpapi/rate_limit_test.go`, `internal/visibility/visibility_test.go`, the acceptance suites above |

**Remaining gap:** none known.

### Phase 2: Identity Model (`:305-421`)

**`implemented`.**

| Criterion | Evidence |
| --- | --- |
| create/edit people | `internal/identity/people.go`, `TestCreatedPersonIsResearchOnlyByConstruction` |
| aliases work | `internal/identity/people.go:355`, `TestAliasUniquenessAndSourceVisibility` |
| families/tribes/branches | `internal/identity`, `TestReferenceFamiliesRefuseUpdateAndDelete` |
| places | `internal/identity/reference.go`, `TestPlaceGeometryIsOptional` |
| approximate dates | `internal/identity/validation.go:155` `parseDateRange`, `TestParsePersonDatesRejectsInvertedRange` |
| entity relationships | `internal/identity/relationships.go`, `TestResolvedRelationshipIsNotEditable` |
| normalized search fields generated | `internal/identity/normalization.go:10`, written on every insert |

**Remaining gap:** one, carried from plan 001. Person alias reads in the research
workspace are not source-scoped (`internal/research/workspace.go:437`). The second
gap this section used to list - the disputed-claims index comparing the normalized
search term against the raw name column, so a name containing ة was unreachable -
was **Blocker 3** and is closed; see below.

### Phase 3: Tree Model (`:422-499`)

**`implemented`.**

| Criterion | Evidence |
| --- | --- |
| user can create a tree | `internal/trees/service.go`, `TestCreateTreeStartsADraftOwnedByTheCreator` |
| user can add people | `internal/trees/service.go`, `TestValidatePersonInputNormalizesDefaults` |
| parent-child relations render | `apps/web/src/components/TreeWorkspace.tsx`, `apps/web/src/lib/tree-view.ts` |
| immutable published versions | `TestPublishedVersionCannotBeChanged`, `TestPublishedPersonResistsEveryMutation` |
| public users can browse published trees | `apps/web/e2e/journeys/02-create-publish-tree.spec.ts` |
| drafts remain private | `internal/visibility`, `TestPrivateSourceIsInvisibleToAnotherResearcher` (same policy, different resource) |

### Phase 4: Collaboration (`:500-548`)

**`implemented`.**

| Criterion | Evidence |
| --- | --- |
| owner can invite | `internal/collaboration/service.go`, `TestInvitedCollaboratorCanEditTheDraft` |
| collaborators edit per permission | `TestOnlyTheOwnerCanInviteOrRevoke` |
| unauthorized users cannot edit | `internal/collaboration/acceptance_postgres_test.go` |
| edits are auditable | `TestMutationsRollBackWithTheirAuditEvent` |
| owner can revoke | `TestRevokedInvitationCannotBeAccepted`, `TestRemovingACollaboratorRevokesTheirAccess` |

### Phase 5: Forking and Tree Diff (`:549-610`)

**`implemented`.**

| Criterion | Evidence |
| --- | --- |
| public tree can be forked | `internal/trees/forks.go`, `TestForkPublishedVersionCopiesInterpretationAndProvenance` |
| fork independently editable | `TestForkRefusesAnUnpublishedVersion` (the negative half) |
| original unchanged | `TestForkPublishedVersionCopiesInterpretationAndProvenance` |
| diff explains changes | `internal/trees/forks.go`, `TestBuildTreeDiffMatchesPeopleSemantically`, `apps/web/src/components/ForkDiffPanel.tsx` |
| parentage impact visible | `internal/research/graph_impact.go`, `TestGraphRelationshipImpactIsDeterministicAndAIIndependent` |

### Phase 6: Source and Evidence Model (`:611-709`)

**`implemented`.**

| Criterion | Evidence |
| --- | --- |
| source created | `internal/evidence/service.go` |
| file uploaded | `internal/sourceprocessing/upload.go:153`, `TestUploadProcessAndReviewATextSource` |
| passage cited, statement recorded | `internal/evidence`, `TestClaimEvidenceChainIsVersionedAndAudited` |
| claim references evidence | `internal/evidence`, `TestAddEvidenceReturnsAScopedClaim` |
| counter-evidence supported | `internal/evidence`, `TestStatementConflictsOnlyKeepKnownReferences` |
| claim version history preserved | `TestClaimEvidenceChainIsVersionedAndAudited` |

**Remaining gap, and a documentation defect fixed by this plan:** V1 accepts
`text/*`, `application/json` and `application/xml` and refuses everything else
with a 415 (`internal/sourceprocessing/upload.go:153`,
`TestUploadRefusesAnUnsupportedFormatWithTheAcceptedList`). `ARCHITECTURE.md:1127`
still lists PDFs among stored object types. That line is now corrected.

### Phase 7: Disputes and Open Questions (`:710-786`)

**`implemented`.**

| Criterion | Evidence |
| --- | --- |
| competing claims coexist | `internal/claims`, `TestClaimConflicts` |
| question links to both | `internal/questions/service.go`, `TestQuestionCanLinkItsDisputeAndKeepTheCount` |
| evidence per side | `TestStatementConflictsOnlyKeepKnownReferences` |
| resolving preserves history | `TestQuestionUpdateIsRolledBackWhenTheAuditInsertFails`, `TestQuestionNoteIsRolledBackWithItsAuditEvent` |
| question can be reopened | `internal/questions/service.go` status transitions, `TestOpenQuestionCanBeCreatedWorkedAndLeftOpen` |

### Phase 8: Public Suggestions (`:787-842`)

**`implemented`.** Review works end to end, and the domain change an acceptance
causes is now expressible in the product rather than only through the API.

| Criterion | Evidence |
| --- | --- |
| public user can submit on an allowed node | `internal/suggestions/service.go`, `apps/web/e2e/journeys/06-suggestion.spec.ts` |
| collaborator sees review queue | `apps/web/src/components/SuggestionPanel.tsx` |
| collaborator can accept/reject | `TestReviewDecision` |
| **accepted suggestion can create domain changes** | `internal/suggestions/service.go` `applyChangeSet`, `apps/web/src/components/SuggestionPanel.tsx` `ChangeSetComposer`, and `apps/web/e2e/journeys/10-change-set.spec.ts`, which accepts a suggestion with a change set, reads the resulting alias back off the person page, and asserts the audit trail and the stored change set. The authorization rule is unchanged and still tested: `TestChangeSetAcceptsEveryTypedTarget`, `TestPlainCollaboratorKeepsTheReviewSurfaceWithoutAChangeSet`, `TestGlobalWriteRoleStillAppliesAChangeSet`. |
| review decision is auditable | `internal/suggestions`, audit assertions in the same tests |

**Remaining gap:** none, with one rule the product cannot change. Applying a change
set writes to the shared research tables, so it takes the same global write role the
evidence service requires rather than tree-review rights. The browser is not told
who holds that role - `/api/v1/auth/me` reports no roles - so the panel does not
guess: it sends the change set, explains the 403 the API returns, stops offering the
button, and leaves accept-without-change-set available. That is the only path a
reviewer without the role has, and `journeys/10-change-set.spec.ts` asserts it still
works after the refusal.

### Phase 9: Dictionary and Indexes (`:843-900`)

**`implemented`.**

| Criterion | Evidence |
| --- | --- |
| public dictionary pages work | `internal/dictionary/service.go`, `apps/web/src/components/DictionaryPage.tsx` |
| indexes filterable | `indexQuery` per kind, `TestIndexQueryIncludesAliasSearch` |
| entries graph-backed | `internal/dictionary/service.go:419-478` joins people/families/places rather than a parallel editorial table |
| aliases searchable | `TestStringSimilarityNormalizesArabicNames`, `TestPeopleIndexAliasSearchCannotProbeAPrivateAlias` |
| every index answers the same query the same way | `internal/httpapi/name_index_agreement_test.go` `TestPeopleFacingIndexesResolveOneTermIdentically` drives the nine people-facing routes over one corpus and requires the same verdict from all of them. The visibility half is `TestDisputedClaimIndexHidesPrivatePersonNames` and `TestDictionaryDisputedClaimIndexIsScoped`. |

**Remaining gap:** none. This row was **Blocker 3** - the `disputed-claims` index
compared the normalized term against the raw canonical name, so a name containing ة
answered in five indexes and not in that one - and the same asymmetry was in the
search claim stage and in both historical-place-name predicates. All four now read
the normalized column. What caught it was the absence of a cross-index test rather
than the presence of a bug, so the test that would have caught it now exists and is
part of `make verify-full`.

### Phase 10: Historical Mapping (`:901-970`)

**`implemented`**, on the six acceptance criteria - and one line of
`IMPLEMENTATION_PLAN.md:947` that is not one of them is answered below rather than
left unmentioned.

| Criterion | Evidence |
| --- | --- |
| family page shows map | `apps/web/src/components/HistoricalMapPanel.tsx` |
| place page works | `apps/web/src/routes/places.tsx`, `internal/geography` |
| migration routes render | `internal/geography/service.go`, `TestMapKeepsEveryPublicAssociation` |
| map filters by period | `TestMapFiltersExcludeBeforeTheResponse` |
| evidence inspectable from map items | `internal/geography`, `TestMapHidesResearchOnlyPersonEndpoints` |
| status visually distinguishable | `apps/web/src/components/StatusBadge.tsx` |

#### What date display actually is in V1

`IMPLEMENTATION_PLAN.md:947` asks, under "Time filtering", for a "year/range
filter" and for "Hijri or Gregorian display support where practical". The first is
implemented; the second is a decision, and it is written down here because the
status document did not mention it at all.

**What is stored.** Every temporal column in the schema is a PostgreSQL `date`, and
none of them carries a calendar, an era or a per-record calendar column:
`people.birth_date_from` / `birth_date_to` / `death_date_from` / `death_date_to`,
`claims.time_from` / `time_to`, `branches.valid_from` / `valid_to`,
`historical_place_names.valid_from` / `valid_to`, `geographic_associations.time_from`
/ `time_to`, `migration_events.time_from` / `time_to`,
`sources.publication_date_from` / `publication_date_to`. Approximation is carried by
nullable bounds and by `migration_events.certainty`, which is `('precise',
'approximate', 'uncertain')` - a column that grades the record's own precision.

**What is rendered.** One vocabulary, produced once in
`services/core-api/internal/dates` and sent to the client as a rendered string:

| The record holds | It renders as |
| --- | --- |
| both bounds | `1120 — 1185`, and `1120 — 1185 (تقديرية)` when `certainty` says so |
| an open bound | `من 1120` or `حتى 1185` |
| no bound | `غير محددة` |
| a period a person wrote down | their own words, verbatim: `قبل ١١٥٠هـ` |

Every surface that shows a period is a reader of that one rule. The tree node years
(`internal/trees/service.go` `formatYears`) and a map feature
(`internal/geography/service.go`, which now carries a `period` field) are the same
function; the place index (`apps/web/src/routes/places.tsx`) renders a recorded
string through `apps/web/src/lib/periods.ts`, which is a pass-through with a test
that fails if anyone "helpfully" converts it. Pinned by
`TestFormatRangeCoversEveryShapeTheSchemaHolds`,
`TestFormatRangeWithCertaintyNamesOnlyTheEstimates`,
`TestMapFeatureRendersItsPeriodThroughTheDateContract`, and the three cases in
`apps/web/src/lib/periods.test.ts`.

**What does not exist.** No calendar conversion, anywhere, in either direction. There
is no `hijri` handling in `apps/web` or in the API; the `هـ` in the demo period
strings is a character in a string, not a converted value.

#### The decision: a date-display contract, not a conversion

**Chosen: (a), a documented contract. Not (b), a conversion helper.** The reasoning,
so a later reader can disagree with it rather than guess at it:

1. **There is nothing to convert from.** A conversion needs a calendar on the value.
   The schema stores a bare `date` with no calendar column, and a row does not say
   whether `1120-01-01` is a Hijri year recorded by a manuscript or a proleptic
   Gregorian year recorded by a modern editor. Converting it picks one silently, in
   the presentation layer, where no test sees it and no reviewer can refute it. That
   is the failure mode this product's whole layering exists to prevent, applied to
   its own output.
2. **A conversion contradicts this phase's own criterion.** `IMPLEMENTATION_PLAN.md`
   requires that uncertain history is never rendered as hard fact, and the schema
   honours that with nullable bounds and a `certainty` column. A converted date is
   day-precise. Printing one for a record whose `time_to` is NULL asserts a bound the
   record explicitly declines to fix.
3. **It is a product decision, not a bug fix.** Which calendar is authoritative,
   whether a per-record calendar column is added (a migration and a backfill over
   every temporal column), and what a date of unknown calendar should display as -
   these are decisions about the record model. Plan 013's STOP conditions name a data
   migration as a stop, and this is one.
4. **The precision is not defensible either way.** A tabular Hijri conversion is
   accurate to about a day and can be off by one; an observation-based one moves with
   the sighted moon. Neither is "the date" for a tenth-century record whose bound is a
   decade, so the conversion would be a real number attached to an approximate claim -
   and the smallest honest alternative is to show the bound the record actually has.
5. **A conversion would need a dependency.** Plan 013's third STOP condition is a
   third-party calendar library. The honest version of (b) - hand-rolled arithmetic
   tables in Go - is a second calendar implementation to keep correct for a display
   decision that should not be made yet.

What would reopen it: a source record that states its own calendar, a calendar column
in the schema, and a decision about which is authoritative. Until a record says which
calendar it is in, the platform has nothing to convert and should say so.

### Phase 11: Search V1 (`:971-1025`)

**`implemented`.**

| Criterion | Evidence |
| --- | --- |
| exact Arabic name search | `internal/search/service.go`, `TestSearchNameRelevance` |
| alias search | `TestEntityReferencesFindSeededAlias` |
| fuzzy/normalized name search | `TestStringSimilarityNormalizesArabicNames` |
| semantic passage search | `internal/research/retrieval.go`, `TestRetrieveLexicalPassagesRelevance` |
| filters work | `TestValidateInputRejectsInvalidFilters` |
| no OpenSearch required | there is no OpenSearch dependency; `internal/search` is PostgreSQL only |

**Remaining gap:** "semantic" here is lexical trigram plus token overlap, not
embeddings. The word in the plan is stronger than the implementation, and the
`docs/graph-benchmark.md` and evaluation notes are explicit about the difference.

### Phase 12: PostgreSQL-Backed Job System (`:1026-1084`)

**`implemented`**, and stronger than the criteria require: jobs are leased and
fenced, which the plan did not ask for.

| Criterion | Evidence |
| --- | --- |
| multiple workers cannot claim same job | `platform/jobs/jobs.go`, `internal/jobs/lease.go`, `TestClaimHandsTheJobToExactlyOneWorker`, `TestExpiredLeaseIsFencedEvenForTheMatchingWorker` |
| retry works | `TestFailAdvancesAttemptsOnce`, `TestIdempotentEnqueueReturnsTheOriginalJob` |
| failures visible | `internal/jobs/service.go` `last_error`, `apps/web/src/components/JobsPage.tsx` |
| stuck jobs recoverable | `TestRecoverReturnsAnAbandonedJobToTheQueue`, `TestRecoveredJobRejectsTheWorkerThatLostIt` |
| workers idempotent where practical | `TestIdempotentEnqueueReturnsTheOriginalJob`, `internal/jobworker` |

### Phase 13: AI Research Service Foundation (`:1085-1132`)

**`implemented`.**

| Criterion | Evidence |
| --- | --- |
| Go can call AI service | `internal/ai/client.go`, `TestHTTPClientCallsAllStableContracts` |
| providers replaceable | `internal/ai/client.go` provider interface, `TestHTTPProviderRejectsMalformedSuccess` |
| outputs validated | `TestValidateEmbeddingRejectsZeroVector`, pydantic models in `app/main.py` |
| timeouts/retries configured | `internal/ai/client.go:205-217`, `TestHTTPClientHonorsTimeout`, `TestHTTPProviderRetriesTransientFailures`, `TestBackoffIsBounded` |
| AI failures do not corrupt state | `TestResearchServiceRejectsMissingDependencies`, in-transaction rollback tests |

### Phase 14: Source Processing Pipeline (`:1133-1193`)

**`implemented`** on every acceptance criterion, with one stage of the pipeline
diagram at `IMPLEMENTATION_PLAN.md:1144` deliberately absent.

| Criterion | Evidence |
| --- | --- |
| processed asynchronously | `services/core-api/cmd/source-processing/main.go`, `TestARunIsQueuedBeforeAnyWorkIsDone` |
| passage traceable to page | `TestTextExtractorPreservesPagesAndSegments`, `TestPersistProcessedPagesKeepsPageAndLocatorProvenance` |
| candidates accepted/rejected | `internal/sourceprocessing/review.go`, `TestUploadProcessAndReviewATextSource` |
| accepted candidates create reviewed records | `TestCandidateStatusDefaultsToReview`, `TestCandidateReviewGroupingKeepsOrderAndEmptyLists` |
| rejected candidates auditable | `TestSourceCharacterizationRunAndReview` |

**The absent stage: `text/OCR extraction` (`IMPLEMENTATION_PLAN.md:1144`).** The
pipeline diagram names it as the second stage, after `Source upload`. It is not
implemented and there is no partial version of it: there is no OCR, no PDF and no
image ingestion anywhere in the repository. An upload is text or one of two
structured text formats or it is refused with `415` at the HTTP boundary, before
storage and before a job is enqueued - `internal/sourceprocessing/upload.go:153`
declares the whole accepted set (`text/*`, `application/json`, `application/xml`),
and `UnsupportedContentError` names it back to the caller.

**This is the deliberate V1 decision, recorded in `plans/003-source-format-contract.md:57`**,
which chose text-only V1 over bounded PDF/OCR extraction and required the choice to
be written into the API error message, the UI copy and the tests - all three of which
now read the same list from `upload.go`. `ARCHITECTURE.md:1139-1155` states the same
thing for a reader who starts from the architecture document, and
`apps/web/e2e/journeys/05-attach-source.spec.ts` proves the refusal in a browser.
The phase is `implemented` rather than `partial` because none of its five acceptance
criteria mentions a format: the criteria are about asynchrony, page-level
traceability, review and audit, and all five are met. The stage that is missing is in
the diagram, not in the criteria, and saying so here is what keeps the row from
reading as a claim that this version reads scans.

**Remaining gap:** `internal/sourceprocessing/review.go:217` and `:253` order by
`created_at` with no tiebreak, so two rows created in the same transaction can come
back in either order. Cosmetic today, unstable tomorrow.

### Phase 15: Hybrid RAG (`:1194-1240`)

**`implemented`** for three of four criteria; the fourth is a property of the
architecture rather than a test.

| Criterion | Evidence |
| --- | --- |
| answers cite evidence | `internal/research/rag_service.go`, `internal/research/rag_types.go` `EvidenceRefs` |
| conflicting sources represented | `internal/research/layers.go`, `TestRetrieveTreeInterpretationsRelevance`, `TestRetrieveClaimsRelevance` |
| system can answer "evidence is insufficient" | `app/main.py:617-619`; pinned by `test_an_unsupported_answer_refuses_in_words_and_not_only_in_metrics` |
| responses never silently promote AI output to fact | `review_required` is `Literal[True]` on every AI response type in `app/main.py`; layers keep `platform findings` separate from `tree interpretation` |

**Remaining gap:** the citations this phase produces are the ones the citation
group measures, and one of its two safety properties is not met - **Blocker 2**.

### Phase 16: Entity Resolution V1 (`:1241-1295`)

**`implemented`.**

| Criterion | Evidence |
| --- | --- |
| duplicate candidates generated | `internal/entityresolution/service.go`, queued scan, `TestARunIsQueuedBeforeAnyWorkIsDone`, `TestGenerateCandidatesCanonicalizesPairs` |
| explanation shows matching/conflicting signals | `internal/entityresolution/scoring.go:238` `scorePair`, `TestScorePairUsesExplainableSignals`, `TestScorePairCapsExplicitConflicts` |
| merge requires human review | `MergeCandidate` at `internal/entityresolution/service.go:620` is role-gated |
| merge can be reversed | `ReverseMerge` at `internal/entityresolution/service.go:705` restores the prior state, `TestMergedPersonIsNotEditable` in `internal/identity/acceptance_postgres_test.go` covers the merged state it restores |

**Remaining gap:** merges exist for the reference families; `family_aliases` and
`tribe_aliases` writes and any living-person policy remain out of scope by plan
005's own statement, and this document does not claim otherwise.

### Phase 17: Contradiction Detection V1 (`:1296-1340`)

**`implemented`.**

| Criterion | Evidence |
| --- | --- |
| findings generated asynchronously | `services/core-api/cmd/contradiction-detector/main.go`, `internal/contradiction` |
| finding links to claims/entities | `internal/contradiction/scanner.go`, `internal/contradiction/service.go` |
| user can dismiss/confirm/investigate | `apps/web/src/routes/contradictions.tsx` |
| finding can create an open question | `internal/questions` from a finding, `TestFindingDoesNotBecomeInterpretationByIdentity` |

**Remaining gap:** the detector's precision and recall are baselines with two
recorded misses (`evaluation/contradiction_cases.jsonl`, KNOWN_DEFECTS in
`evaluation/thresholds.py`). It proposes pairs about different people and misses
migration disagreements.

### Phase 18: Jev Semantic Control Layer (`:1341-1374`)

**`partial`.**

| Criterion | Evidence |
| --- | --- |
| measurable reduction in expensive model calls | `internal/ai/routing.go`, `synthesis_skipped` in the routing report, `TestFallbackRouteUsesOperationalPaths` |
| routing quality evaluated against a test set | `evaluation/routing_cases.jsonl`, `route_accuracy` 1.0 and `query_type_accuracy` 0.8667 against reviewed labels |
| fallback path exists | `app/main.py` `routing_decision(fallback=True)`, `TestFallbackRouteUsesOperationalPaths` |
| no historical status depends directly on Jev output | true by construction: routing returns `route`/`query_type`/`reason_code` and a `review_required` literal, and the evaluation report now states in the routing group's own notes that these numbers are agreement with reviewed labels and **not** historical accuracy |

**Remaining gap:** "measurable reduction in expensive model calls" is measured as
`route_accuracy` and `synthesis_skipped` against a fifteen-case fixture set the
same author wrote. It is not a cost measurement against real traffic, and the
report says so.

### Phase 19: Research Workspace (`:1375-1418`)

**`implemented`**, with one criterion partially met.

| Criterion | Evidence |
| --- | --- |
| investigate without jumping across screens | `apps/web/src/components/QuestionWorkspace.tsx`, `apps/web/e2e/journeys/09-research-query.spec.ts` |
| all findings traceable to evidence | `internal/research/workspace.go`, `TestResearchRunMetadataIsResearchOnly` |
| edits remain permission-controlled | `internal/research/workspace.go` in-transaction authorization; the suggestion half is `internal/suggestions` `canWriteGlobally`, unchanged, and the panel now says so when it answers 403 |
| research history preserved | `internal/research/history.go`, `TestListRunsOrderIsTheSummaryOrder` |

**Remaining gap:** `internal/research/workspace.go:437` reads person aliases
without source scoping (carried from plan 001).

### Phase 20: GraphRAG V1 (`:1419-1463`)

**`partial`, and the first criterion cannot be met with what is in this
repository.**

| Criterion | Evidence |
| --- | --- |
| **multi-hop research questions improve over vector-only RAG** | **measured, and the criterion is still open — this is Blocker 1, restated.** Both retrieval paths exist and both ran: `docs/retrieval-measurement.md` scores vector-only, hybrid and graph-augmented over 29 labelled Arabic questions and a 42-passage corpus that `make retrieval-report` provisions into an isolated schema and drops again. On the 17 multi-hop questions, vector-only reaches recall@5 0.118 (MRR 0.078), hybrid 1.000 (MRR 0.784), graph-augmented 1.000 (MRR 0.784), against a stated variance of 0.248. The difference is far outside the noise and is still **not** evidence for the criterion, because the only embedding provider this repository may use returns a SHA-512 hash of the input rather than a semantic vector: cosine(query, 42 other corpus texts) averages 0.068 and 28 of 42 come out positive. The "vector-only" arm is a permutation, so a difference against it is a difference against noise. |
| graph traversal performant for bounded depth | measured: `docs/graph-benchmark.md`, 4.6-23.3ms p50 retrieval at and below the bounds, reproducible structurally and in allocations across two runs |
| evidence path explainable | `internal/research/rag_types.go:156` `GraphPath.Explanation` per status, `TestGraphShortestRelationshipPathIsBoundedAndDirectional`, `TestGraphAncestorFrontierIsDeterministicAndAIIndependent` |

**Remaining gap:** the first criterion, and it is the criterion the phase is named
for. Everything else in this phase is done and measured. What changed under plan
014 is that the gap is now a measurement rather than an absence: the corpus, the
question set and the harness exist, the arms run, and the report says what the
numbers do and do not support. Three preconditions stand between here and a signed
criterion, and all three are named in `docs/retrieval-measurement.md`: a semantic
embedding provider, graph evidence merged into the fused ranking rather than
appended after it, and a few hundred judged multi-hop questions from a production
query log.

### Phase 21: Source Dependency Analysis (`:1464-1501`)

**`implemented`.**

| Criterion | Evidence |
| --- | --- |
| dependency graph visible | `internal/research/graph_source_dependency.go`, `apps/web/src/components/SourceDependencyPanel.tsx` |
| evidence views warn about dependent sources | `internal/sourceprocessing/review.go`, `TestSourceCharacterizationRedactsPrivateRelatedSources` |
| dependency does not automatically invalidate a source | `internal/research/graph_source_dependency.go` returns `StructuralOnly: true` with no evidence refs; `TestGraphSourceDependencyNeighborhoodIsBoundedAndPrivate` asserts `StructuralOnly` and an empty `EvidenceRefs` |

**Measured, and worth reading before raising the bounds:** community detection
allocates 153 MiB per call on a 200-node star, and its cost tracks the number of
merges rather than the number of edges. `docs/graph-benchmark.md` has the numbers
and the mechanism. It is a Go loop, not a database limit.

### Phase 22: Advanced Temporal and Statistical Analysis (`:1502-1538`)

**`implemented`.**

| Criterion | Evidence |
| --- | --- |
| statistics based only on qualified data | `internal/temporalanalysis`, `TestTemporalAnalysisRunUsesQualifiedReferencePopulation` |
| reference population documented | `TestReferenceBandRequiresMinimumPopulation`, `TestReferenceBandAndComparison` |
| anomaly explanations show comparison basis | `internal/temporalanalysis/statistics.go` |
| no score mislabeled as historical probability | `TestBuildIntervalObservationRejectsImpossibleChronology`, `TestValidGenerationChronologyRejectsParentDeathBeforeChildBirth`, `apps/web/src/components/TemporalAnalysisPanel.tsx` |

### Phase 23: Advanced Geospatial Intelligence (`:1539-1576`)

**`implemented`.**

| Criterion | Evidence |
| --- | --- |
| all inferred geography labeled | `internal/geospatialintelligence`, `TestGeospatialIntelligenceRunSeparatesSourceAndInferredLayers` |
| source-backed and inferred layers separate | `TestBuildClustersUsesSpatialEdgesAndSeparatesLayers` |
| ambiguous places left unresolved | `TestResolveMentionsKeepsAmbiguousHistoricalNamesUnresolved` |

**Remaining gap:** `TestBuildMigrationHypothesisAndContradictionAreLabeled` covers
the labelling; the migration hypothesis itself is `platform_inferred`, never
`source`, which is the property the criterion is really about.

### Phase 24: Research Agent (`:1577-2038`)

**`implemented`.**

| Criterion | Evidence |
| --- | --- |
| traceable investigation steps | `internal/researchagent/stages.go`, `TestResearchAgentRunProducesTraceableBoundedPackage` |
| citations support outputs | `EvidenceRef` per step, `TestAWorkerWithoutAClaimCannotWriteTheInvestigation` |
| counter-evidence included | `internal/researchagent/types.go` `Gap`/`Recommendation`, `TestAnInvestigationResumesFromWhereItStopped` |
| unresolved state allowed | `GapCount` on the run, `TestResearchAgentRejectsInvalidAndUnauthorizedRuns` |
| actions bounded by permissions | `TestAnInvestigationStaysReadOnlyWhenItRunsInAWorker`, `TestAWorkerWithoutAClaimCannotWriteTheInvestigation` |

**Remaining gaps, carried from plan 006 and verified here:**
`internal/researchagent/stages.go:59-70` cannot match a NULL source id - the
predicate is `($1 = '' OR s.id = $1::uuid)`, so a NULL parameter is neither `''`
nor equal to a uuid and the stage returns nothing. And a step can report one more
evidence item than it wrote (`internal/researchagent/service.go:418` writes
`len(refs)` while the refs are persisted separately).

---

## The eighteen steps of V1 (`IMPLEMENTATION_PLAN.md:1964-1992`)

Marked against evidence, not against intent. "Proved" means a test in
`make verify-full` or a journey in `make e2e` exercises it against a real database.

| # | Step | Status | Evidence |
| --- | --- | --- | --- |
| 1 | register | proved | `journeys/01-register.spec.ts`; `TestValidEmailRejectsWhitespace` |
| 2 | create a family tree | proved | `journeys/02-create-publish-tree.spec.ts`; `TestCreateTreeStartsADraftOwnedByTheCreator` |
| 3 | invite collaborators | proved | `journeys/04-invite-collaborator.spec.ts` (5 journeys); `TestInvitedCollaboratorCanEditTheDraft` |
| 4 | publish a version | proved | `journeys/02-create-publish-tree.spec.ts`; `TestPublishClosesTheDraftAndOpensTheNext` |
| 5 | fork another published tree | proved | `journeys/03-fork-tree.spec.ts`; `TestForkPublishedVersionCopiesInterpretationAndProvenance` |
| 6 | compare two tree versions | proved | `apps/web/src/components/ForkDiffPanel.tsx`; `TestBuildTreeDiffMatchesPeopleSemantically` |
| 7 | attach a historical source | proved | `journeys/05-attach-source.spec.ts`; `TestUploadProcessAndReviewATextSource` |
| 8 | create a claim from that source | proved | `internal/evidence`; `TestAddEvidenceReturnsAScopedClaim` |
| 9 | represent competing claims | proved | `journeys/07-question-dispute.spec.ts`; `TestClaimConflicts` |
| 10 | create an open research question | proved | `journeys/07-question-dispute.spec.ts`; `TestOpenQuestionCanBeCreatedWorkedAndLeftOpen` |
| 11 | receive a public Arabic suggestion | proved | `journeys/06-suggestion.spec.ts`; `internal/suggestions` |
| 12 | review and accept/reject that suggestion | proved, and the domain change an acceptance causes is expressible in the browser | `journeys/10-change-set.spec.ts` (accepts with a change set, asserts the record, the audit trail and the stored change set); `TestReviewDecision`; `TestChangeSetAcceptsEveryTypedTarget` |
| 13 | browse family/tribe dictionary pages | proved | `DictionaryPage.tsx`; `TestIndexQueryIncludesAliasSearch`; `TestEveryNameIndexAnswersForBothSpellingsOfATaMarbutaName`; **Blocker 3**, closed |
| 14 | search names and sources | proved | `journeys/08-browse-map.spec.ts`; `TestSearchNameRelevance`; `TestClaimStageAnswersForBothSpellingsOfTheSubjectName`; `TestPeopleFacingIndexesResolveOneTermIdentically`; **Blocker 3**, closed |
| 15 | view historical locations on a map | proved | `journeys/08-browse-map.spec.ts` (4 journeys); `TestMapKeepsEveryPublicAssociation` |
| 16 | view migration relationships | proved | `journeys/08-browse-map.spec.ts`; `TestBuildMigrationHypothesisAndContradictionAreLabeled` |
| 17 | ask a source-grounded research question | proved: a citation that does not support the answer is refused | `journeys/09-research-query.spec.ts`; `citations.unsupported_refusal` 1.0 against a threshold of 1.0, `cit-003` / `cit-007` / `cit-008`; **Blocker 2**, closed in plan 012 |
| 18 | see the five layers separated | proved | `internal/research/layers.go`; `TestRetrieveClaimsRelevance`, `TestRetrieveTreeInterpretationsRelevance`, `TestFindingDoesNotBecomeInterpretationByIdentity`; `EvidencePanels.tsx` |

**All eighteen are proved outright.** That took three plans and four blockers, and
the record of how is worth keeping: step 17 was proved by plan 012, whose row was
still carrying the pre-plan-012 wording until this commit, and steps 12, 13 and 14
were proved by this one - 12 by making the change set expressible in the browser, and
13 and 14 by fixing the four raw-versus-normalized predicates and adding the
cross-index test whose absence let the defect through. A status row that is quietly
out of date is the failure this document exists to prevent, so the correction is in
the same commit as the code.

What is left is not a gap in the eighteen but a standing limit of the repository:
nothing here claims multi-hop retrieval improves on vector-only retrieval. The reason
used to be that there was no second retrieval path to compare against, which was
wrong - there are two, and plan 014 measured them. The reason now is that the only
permitted embedding provider returns a hash, so the comparison has no semantic
baseline (**Blocker 1**, restated with the numbers in
`docs/retrieval-measurement.md`).

---

## The first three remaining blockers

In the order a reviewer should take them. Each is stated as a defect with a
reproduction, not as a task.

### Blocker 1 - GraphRAG's own acceptance criterion has no evidence path - RESTATED by plan 014

**This text was wrong about the repository and is corrected here.** It used to say
there is "one retrieval path in this repository (`internal/research/retrieval.go`, a
lexical trigram and token-overlap reranker)" and that closing the blocker needed "a
labelled Arabic question set with judged relevant passages, and a second retrieval
path to run it against."

The second retrieval path was never missing. `retrieveVectorPassages`
(`internal/research/retrieval.go:70-122`) scores passages with
`1 - (sp.embedding <=> $1::vector)`, `execute` has always called both legs and fused
them (`internal/research/rag_service.go:77-85`), and `rag_service.go:387-392`
persists `Score.Lexical`, `Score.Vector`, `Score.Rerank` and `Score.Combined`
separately. What was missing was **data**: `source_passages` held three rows and none
had an embedding, because `db/seeds/` is pure SQL and the only writer of that column
is the source-processing worker. Every test, demo and benchmark had therefore been
running the lexical leg alone, silently.

Plan 014 supplied the data, the labels and the harness, and measured all three arms.
The criterion is **still open**, and the reason is now specific:

`IMPLEMENTATION_PLAN.md:1443` requires that "multi-hop research questions improve
over vector-only RAG". On 17 labelled multi-hop questions, hybrid beats vector-only
by 0.882 recall@5 and graph-augmented matches hybrid exactly. That difference is real
and it is not evidence, because `DeterministicProvider.embed` is a SHA-512 digest of
the normalized text rather than a semantic embedding, so the vector-only arm ranks
the corpus by coincidence. The graph leg contributed four passages across three of
the seventeen cases and never changed a top five, for three separate reasons: nine
cases where every cited passage was already in the hybrid's list, four `geographic_path`
cases whose evidence refs carry no passage or statement id and are therefore
uncitable, and one case whose only edge is an `unresolved` claim that
`evidence_connection` filters out. The full numbers, the stated variance and the
method are in `docs/retrieval-measurement.md`; the machine-readable run is
`docs/benchmarks/retrieval-arms.json`.

**Why it still blocks:** the phase the product is named for cannot be signed off.
The blocker is no longer "there is nothing to measure" - it is "the only permitted
embedding provider cannot produce an embedding, so the comparison has no semantic
baseline to improve on".

**What would close it**, in the order that matters:

1. **A semantic embedding provider.** Without one there is no vector-only arm to
   improve on, at any corpus size. This is a decision, not a measurement, and it is
   the binding constraint today.
2. **Graph evidence merged into the fused ranking.** `graphPassageCitations` appends
   after a thirty-item fused list, so a graph passage cannot reach a top-five slot
   while the passage legs return more than five candidates. No corpus size fixes
   that; it is an ordering decision.
3. **A few hundred judged multi-hop questions** from a production query log, with
   judgements by somebody who did not write the questions. At 17 cases the bar is
   0.248; clearing a ten-point difference needs about 381 cases per arm.

`evaluation/evaluate.py` still reports `vector_baseline.available = false`, and that
is still the honest form of the answer: the Python evaluation has no corpus and no
provider, so the comparison lives in the Go harness and its report. The reason text
there has been updated to point at the measurement rather than to claim there is
nothing to measure.

### Blocker 2 - a source that does not answer the question is still cited - FIXED in plan 012

**Status: closed.** `research_query` in `services/ai-research/app/main.py` no longer
counts any shared token as grounding. A context is cited only when it shares a
*content* token with the question: the existing tokenizer's output minus an
explicit, documented stopword set of Arabic and English function words, written out
next to the filter. The preposition `في` was the load-bearing case -
`services/ai-research/evaluation/citation_cases.jsonl` case `cit-003`, a question
about a migration answered by nothing, used to come back citing a registry entry
that never mentions it. It now refuses. `citations.unsupported_refusal` measures
**1.0** and its threshold is 1.0; the entry is no longer in
`evaluation/thresholds.py` `KNOWN_DEFECTS`, because a defect leaves that list when
the provider changes, not when the number moves.

**Why it was a blocker:** V1 step 17 promises a *source-grounded* question. A
citation that does not support its answer is the failure this product's whole
layering exists to prevent, and it was reachable from the shipped research path.

**What holds it there now:** `cit-003` (a function word alone is not grounding),
`cit-007` (a content word is) and `cit-008` (a content word alongside a function
word still is) pin all three directions, and `unsupported_refusal` is one of the
safety floors in `thresholds.describe()` with its threshold at 1.0.

### Blocker 3 - a ة name was invisible in four of the people-facing indexes - FIXED in plan 013

**Status: closed.** `ListIndex` normalizes the search term
(`NormalizeArabicName` maps ة to ه) and `validateInput` does the same for search, so
a term handed to a predicate is always normalized. Four predicates compared that
normalized term against a **raw** column:

| Site | Column it read | Now reads |
| --- | --- | --- |
| `internal/dictionary/service.go` `disputed-claims` branch | `people.canonical_name_ar` | `people.normalized_name_ar` |
| `internal/dictionary/service.go` `places` branch | `historical_place_names.name_ar` | `historical_place_names.normalized_name_ar` |
| `internal/search/service.go` `searchClaims` | `people.canonical_name_ar` | `people.normalized_name_ar` |
| `internal/search/service.go` `searchNames`, place branch | `historical_place_names.name_ar` | `historical_place_names.normalized_name_ar` |

Every one of them now matches the column that already holds the same normalization.
Nothing is re-normalized inside a predicate - a `lower(replace(...))` over a stored
column is the unindexed scan `db/migrations/0042_passage_lexical_trigram_index.sql`
declined to justify - and no stored
column was rewritten, so this is a query fix and not a migration.

**What the reproduction looked like before the fix**, against PostgreSQL with the
exact predicates the code ran:

```text
reader types: فاطمة  ->  NormalizeArabicName -> فاطمه
people index         (normalized term vs normalized column): فاطمة بنت علي   <- found
disputed-claims index(normalized term vs raw column):        no match
```

**Why it blocked:** V1 steps 13 and 14 are dictionary and search, and a name
containing ة - a large share of Arabic given names - was invisible through four of
the people-facing indexes. The indexes also disagreed with each other for the same
query, so "searchable" was not a property they had.

**What holds it there now.** Each index answers for both spellings of a name carrying
ة, and `TestPeopleFacingIndexesResolveOneTermIdentically` in
`internal/httpapi/name_index_agreement_test.go` drives the nine people-facing routes
over one corpus and requires the same verdict from every one of them. Against the old
predicates that single test names the four routes that disagreed:

```text
with the canonical spelling the people-facing indexes did not agree: [dictionary people=true
 dictionary families=true dictionary tribes=true dictionary branches=true dictionary places=false
 dictionary disputed-claims=false search names (person)=true search names (place historical name)=false
 search claims (subject name)=false]
```

That cross-index test is the part the defect actually lacked. Every route's own test
had passed, because the demo corpus is Arabic family names - exactly the shape that
hides a ة/ه difference.

---

## Where a plan and the code disagree

Stated explicitly, with the code as the authority.

| Document says | Code says |
| --- | --- |
| `ARCHITECTURE.md:1127` lists PDFs among stored object types | `internal/sourceprocessing/upload.go:153` accepts `text/*`, `application/json`, `application/xml`; a PDF is refused with 415. **Corrected in `ARCHITECTURE.md` by this change.** |
| `IMPLEMENTATION_PLAN.md:947` asks for "Hijri or Gregorian display support where practical" | There is no calendar conversion in either direction, and the decision is deliberate rather than an omission: every temporal column is a bare `date` with no calendar, so there is nothing to convert FROM, and a converted date would be day-precise where the record is a decade. One date-display contract instead, in `internal/dates`. Reasoning under **Phase 10** above. |
| `IMPLEMENTATION_PLAN.md:1144` names a "text/OCR extraction" pipeline stage | There is no OCR, no PDF and no image ingestion. `internal/sourceprocessing/upload.go:153` accepts `text/*`, `application/json` and `application/xml` and refuses anything else with 415, which is the decision `plans/003-source-format-contract.md:57` required and `ARCHITECTURE.md:1139-1155` already states. Reasoning under **Phase 14** above. |
| `IMPLEMENTATION_PLAN.md:978` asks for "semantic passage search" | **Corrected by plan 014.** This row used to say "There is no embedding retrieval in the query path", and that was wrong. `internal/research/retrieval.go:70-122` scores passages with pgvector (`1 - (sp.embedding <=> $1::vector)`) and `rag_service.go:77-85` calls the lexical and vector legs and fuses them. What was true is narrower: the seeded corpus carried no embeddings, so the vector leg had nothing to score and every test, demo and benchmark had been running lexical-only. `cmd/embedding-backfill` fills the column through the same `/embed` contract the worker calls, and the semantic search is real - but it is real over hashed vectors, not semantic ones. See `docs/retrieval-measurement.md`. |
| `IMPLEMENTATION_PLAN.md:1443` requires improvement over vector-only RAG | The vector-only path exists and was measured; the criterion is still open because the provider's embedding is a hash, so the baseline is a permutation. See Blocker 1 and `docs/retrieval-measurement.md`. |
| `IMPLEMENTATION_PLAN.md:1362` asks for a measurable reduction in expensive model calls | What is measured is route agreement on fifteen self-authored fixtures and a `synthesis_skipped` count. No cost measurement exists. |
| `plans/README.md` describes the disputed-claims defect as the index comparing the normalized term against the raw column | That is correct, and the same file's summary of Phase 2 leaves the impression that name search generally cannot match ة. It always could: the people, families, tribes and branches indexes match. The defect was confined to `disputed-claims`, to claim-name search, and to the two historical-place-name predicates - four sites, all now fixed. Verified above. |

## Known residuals carried forward

These are real, dated, and owned by the plan that found them. They are listed so a
reader does not have to reconstruct them from six plan files.

| Residual | Where |
| --- | --- |
| The raw-versus-normalized asymmetry **Blocker 3** was about still exists where the schema has no normalized column to read: `internal/research/retrieval.go:325-336` matches a normalized term against `tree_nodes.display_name_ar`, and the `sources` and `open_questions` indexes in `internal/dictionary/service.go` and `internal/search/service.go` match against `title_ar` and `description_ar`. None of those four tables carries a normalized column, so closing them is a migration with a backfill over every row - a data decision, not a query fix, and the STOP condition plan 013 set for itself. They are not people-name indexes, which is why they were out of Step 1's scope; they are the same shape and they are named here rather than left for a reader to find. |
| The research result order was not a total order, so equal-scored passages came back in a different order on every request | **FIXED in plan 014.** `internal/research/retrieval.go` built the fused candidate list by walking a `map[string]*Citation`; Go randomises map iteration and both sorts are stable, so every tie inherited the map's order. The order is now rerank score, then combined score, then passage id - the id being the same final tie-break the retrieval SQL already used. Ordering only: no score, weight, threshold, cap or membership changed. Pinned by `TestFusedOrderIsIdenticalAcrossRuns` and `TestFusedOrderBreaksTiesByPassageID`, which fail against the pre-fix code. Found because the retrieval measurement's `per_case` lists reordered between two runs of the same commit; see `docs/retrieval-measurement.md` |
| `S3Store` is contract-tested against a double and has never spoken to a real endpoint | `platform/storage/s3.go`, `platform/storage/s3_test.go`; a deployment must run one live presign before trusting it |
| Rate limits are in-process and IP-keyed; no cross-replica enforcement | `platform/ratelimit/ratelimit.go`; a multi-replica deployment needs a shared limiter |
| The upload quarantine hook was deliberately not built | no requirement in this repository specifies the policy |
| `search_sources` cannot match a NULL source id | `internal/researchagent/stages.go:59-70` |
| A per-step evidence count can exceed what was written | `internal/researchagent/service.go:418` |
| `source_files` / `source_processing_runs` order by `created_at` with no tiebreak | `internal/sourceprocessing/review.go:217`, `:253` |
| Candidate passage text is still inline | plan 007's own note; the panel renders every candidate |
| The development database accumulates test users from packages that seed the public schema | fixture schemas are isolated and dropped; the packages that predate that harness are not |
| Community detection allocates 153 MiB per call on a 200-node star | `internal/research/graph_source_dependency_communities.go:216-271`, measured in `docs/graph-benchmark.md` |
| Neighborhood fingerprints are stable within a database, not across equivalent ones | `source_dependencies.id` is a random uuid and the query orders candidate edges by it; measured in `docs/graph-benchmark.md` |

## Maintenance

Update this file in the same commit as anything that changes a phase's status, a
V1 step's evidence, or a blocker. `infra/local/check-doc-links.py` fails when a
path or a line range cited here stops existing, so a stale row cannot survive a
refactor silently - but it cannot tell that a row became *wrong*, only that it
became unreadable. That part is still on the author.
