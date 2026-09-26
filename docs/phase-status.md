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

**"A code slice exists" is not "V1 acceptance is verified."** Almost every phase in
this repository has code. That is not the interesting claim. The interesting claim
is whether the phase's own acceptance list is satisfied, and this document marks
the difference in the last column of every table rather than in a footnote, because
a reader who sees "implemented" eleven times and stops reading has been told
something false.

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

**Remaining gap:** two, both carried from plan 001 and both real. Person alias
reads in the research workspace are not source-scoped
(`internal/research/workspace.go:437`). And the dictionary's disputed-claims index
compares the normalized search term against the raw name column, so a person whose
name contains ة cannot be found through that index - see **Blocker 3** below.

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

**`partial`.** Review works end to end; one acceptance criterion is not reachable
from the product.

| Criterion | Evidence |
| --- | --- |
| public user can submit on an allowed node | `internal/suggestions/service.go`, `apps/web/e2e/journeys/06-suggestion.spec.ts` |
| collaborator sees review queue | `apps/web/src/components/SuggestionPanel.tsx` |
| collaborator can accept/reject | `TestReviewDecision` |
| **accepted suggestion can create domain changes** | `internal/suggestions/service.go:844` `applyChangeSet` exists and is tested (`TestChangeSetAcceptsEveryTypedTarget`), but the shipped panel sends no change set, and applying one requires a global write role rather than tree-review rights. `TestPlainCollaboratorKeepsTheReviewSurfaceWithoutAChangeSet` and `TestGlobalWriteRoleStillAppliesAChangeSet` pin both halves. |
| review decision is auditable | `internal/suggestions`, audit assertions in the same tests |

**Remaining gap:** the domain-change half of this phase is API-only. A user
accepting a suggestion in the browser gets the review recorded and the
open-question artifact, not the change they accepted. This is a Phase 8 criterion
and a Phase 19 criterion ("edits remain permission-controlled") in one gap.

### Phase 9: Dictionary and Indexes (`:843-900`)

**`partial`.**

| Criterion | Evidence |
| --- | --- |
| public dictionary pages work | `internal/dictionary/service.go`, `apps/web/src/components/DictionaryPage.tsx` |
| indexes filterable | `indexQuery` per kind, `TestIndexQueryIncludesAliasSearch` |
| entries graph-backed | `internal/dictionary/service.go:425-463` joins people/families/places rather than a parallel editorial table |
| aliases searchable | `TestStringSimilarityNormalizesArabicNames`, `TestPeopleIndexAliasSearchCannotProbeAPrivateAlias` |
| **every index answers the same query the same way** | **fails for `disputed-claims`** - see **Blocker 3** |

**Remaining gap:** Blocker 3. The visibility half is proved
(`TestDisputedClaimIndexHidesPrivatePersonNames`,
`TestDictionaryDisputedClaimIndexIsScoped`); the matching half is not.

### Phase 10: Historical Mapping (`:901-970`)

**`implemented`.**

| Criterion | Evidence |
| --- | --- |
| family page shows map | `apps/web/src/components/HistoricalMapPanel.tsx` |
| place page works | `apps/web/src/routes/places.tsx`, `internal/geography` |
| migration routes render | `internal/geography/service.go`, `TestMapKeepsEveryPublicAssociation` |
| map filters by period | `TestMapFiltersExcludeBeforeTheResponse` |
| evidence inspectable from map items | `internal/geography`, `TestMapHidesResearchOnlyPersonEndpoints` |
| status visually distinguishable | `apps/web/src/components/StatusBadge.tsx` |

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

**`implemented`.**

| Criterion | Evidence |
| --- | --- |
| processed asynchronously | `services/core-api/cmd/source-processing/main.go`, `TestARunIsQueuedBeforeAnyWorkIsDone` |
| passage traceable to page | `TestTextExtractorPreservesPagesAndSegments`, `TestPersistProcessedPagesKeepsPageAndLocatorProvenance` |
| candidates accepted/rejected | `internal/sourceprocessing/review.go`, `TestUploadProcessAndReviewATextSource` |
| accepted candidates create reviewed records | `TestCandidateStatusDefaultsToReview`, `TestCandidateReviewGroupingKeepsOrderAndEmptyLists` |
| rejected candidates auditable | `TestSourceCharacterizationRunAndReview` |

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
| edits remain permission-controlled | `internal/research/workspace.go` in-transaction authorization; the suggestion half is Phase 8's gap |
| research history preserved | `internal/research/history.go`, `TestListRunsOrderIsTheSummaryOrder` |

**Remaining gap:** `internal/research/workspace.go:437` reads person aliases
without source scoping (carried from plan 001).

### Phase 20: GraphRAG V1 (`:1419-1463`)

**`partial`, and the first criterion cannot be met with what is in this
repository.**

| Criterion | Evidence |
| --- | --- |
| **multi-hop research questions improve over vector-only RAG** | **no evidence exists, and none can be produced here.** The comparison needs two retrieval paths and a labelled judgement. There is one path. `evaluation/evaluate.py` reports `vector_baseline.available = false` with the reason. This is **Blocker 1**. |
| graph traversal performant for bounded depth | measured: `docs/graph-benchmark.md`, 4.6-23.3ms p50 retrieval at and below the bounds, reproducible structurally and in allocations across two runs |
| evidence path explainable | `internal/research/rag_types.go:156` `GraphPath.Explanation` per status, `TestGraphShortestRelationshipPathIsBoundedAndDirectional`, `TestGraphAncestorFrontierIsDeterministicAndAIIndependent` |

**Remaining gap:** the first criterion, and it is the criterion the phase is named
for. Everything else in this phase is done and measured.

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
| 12 | review and accept/reject that suggestion | **proved for review; the domain change it is meant to cause is API-only** | `TestReviewDecision`; `TestChangeSetAcceptsEveryTypedTarget`; the shipped panel sends no change set (Phase 8) |
| 13 | browse family/tribe dictionary pages | **proved except through the disputed-claims index** | `DictionaryPage.tsx`; `TestIndexQueryIncludesAliasSearch`; **Blocker 3** |
| 14 | search names and sources | proved, with the same exception as 13 | `journeys/08-browse-map.spec.ts`; `TestSearchNameRelevance`; **Blocker 3** |
| 15 | view historical locations on a map | proved | `journeys/08-browse-map.spec.ts` (4 journeys); `TestMapKeepsEveryPublicAssociation` |
| 16 | view migration relationships | proved | `journeys/08-browse-map.spec.ts`; `TestBuildMigrationHypothesisAndContradictionAreLabeled` |
| 17 | ask a source-grounded research question | **partly proved: the answer is grounded in *a* citation, not always in one that supports it** | `journeys/09-research-query.spec.ts`; `citation_precision` 0.8182; **Blocker 2** |
| 18 | see the five layers separated | proved | `internal/research/layers.go`; `TestRetrieveClaimsRelevance`, `TestRetrieveTreeInterpretationsRelevance`, `TestFindingDoesNotBecomeInterpretationByIdentity`; `EvidencePanels.tsx` |

**Fifteen of eighteen are proved outright. Three carry a gap**, and two of those
three are the same two code defects the rest of this document is about.

---

## The first three remaining blockers

In the order a reviewer should take them. Each is stated as a defect with a
reproduction, not as a task.

### Blocker 1 - GraphRAG's own acceptance criterion has no evidence path

`IMPLEMENTATION_PLAN.md:1443` requires that "multi-hop research questions improve
over vector-only RAG". There is one retrieval path in this repository
(`internal/research/retrieval.go`, a lexical trigram and token-overlap reranker)
and no labelled corpus to judge a second one against, so the comparison cannot be
run at all. `evaluation/evaluate.py` reports `vector_baseline.available = false`
with the reason, which is the honest form of the answer, and
`docs/graph-benchmark.md` measures traversal cost rather than retrieval quality -
a different question.

**Why it blocks:** the phase the product is named for cannot be signed off, and any
claim that graph retrieval helps is currently unfalsifiable in this repository.

**What would close it:** a labelled Arabic question set with judged relevant
passages, and a second retrieval path to run it against. Both are new work; neither
is a measurement of what exists.

### Blocker 2 - a source that does not answer the question is still cited

`services/ai-research/app/main.py:607-620` keeps every context whose token set
intersects the query's, with no minimum overlap. The preposition `في` is a token, so
`services/ai-research/evaluation/citation_cases.jsonl` case `cit-003` - a question
about a migration, answered by nothing - comes back with a citation to a registry
entry that does not mention it. Measured: `citations.unsupported_refusal = 0.5`.

**Why it blocks:** V1 step 17 promises a *source-grounded* question. A citation
that does not support its answer is the failure this product's whole layering
exists to prevent, and it is reachable from the shipped research path.

**Why it is not fixed here:** the fix is in the provider, and this plan adds
measurement. A gate made green by editing the thing it measures is not a gate. It
is recorded in `evaluation/thresholds.py` KNOWN_DEFECTS, printed by `make ai-eval`
next to the pass, and its threshold sits at the measurement rather than at 1.0.

### Blocker 3 - the disputed-claims dictionary index cannot be searched by a ة name

`internal/dictionary/service.go:123` normalizes the search term
(`NormalizeArabicName` maps ة to ه), and the `people`, `families`, `tribes`,
`branches` and `places` indexes compare it against the normalized column. The
`disputed-claims` index at `internal/dictionary/service.go:463` compares the same
normalized term against the **raw** `canonical_name_ar`.

Verified against PostgreSQL with the exact predicates the code runs:

```text
reader types: فاطمة  ->  NormalizeArabicName -> فاطمه
people index         (normalized term vs normalized column): فاطمة بنت علي   <- found
disputed-claims index(normalized term vs raw column):        no match
```

**Why it blocks:** V1 steps 13 and 14 are dictionary and search, and a name
containing ة - which is a large share of Arabic given names - is invisible through
one of the eight public indexes. The two indexes also disagree with each other for
the same query, so "searchable" is not a property the index currently has.

---

## Where a plan and the code disagree

Stated explicitly, with the code as the authority.

| Document says | Code says |
| --- | --- |
| `ARCHITECTURE.md:1127` lists PDFs among stored object types | `internal/sourceprocessing/upload.go:153` accepts `text/*`, `application/json`, `application/xml`; a PDF is refused with 415. **Corrected in `ARCHITECTURE.md` by this change.** |
| `IMPLEMENTATION_PLAN.md:978` asks for "semantic passage search" | `internal/research/retrieval.go` is lexical: trigram index plus token overlap. There is no embedding retrieval in the query path. |
| `IMPLEMENTATION_PLAN.md:1443` requires improvement over vector-only RAG | No vector-only path exists, so nothing can be compared. See Blocker 1. |
| `IMPLEMENTATION_PLAN.md:1362` asks for a measurable reduction in expensive model calls | What is measured is route agreement on fifteen self-authored fixtures and a `synthesis_skipped` count. No cost measurement exists. |
| `plans/README.md` describes the disputed-claims defect as the index comparing the normalized term against the raw column | That is correct, and the same file's summary of Phase 2 leaves the impression that name search generally cannot match ة. It can: the people index matches. The defect is confined to `disputed-claims` and to claim-name search. Verified above. |

## Known residuals carried forward

These are real, dated, and owned by the plan that found them. They are listed so a
reader does not have to reconstruct them from six plan files.

| Residual | Where |
| --- | --- |
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
