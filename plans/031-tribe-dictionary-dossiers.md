# Plan 031: Tribe dictionary dossiers with evidence badges, hints, and challenges (معجم قبائل جغرافي)

> **Executor instructions**: The dossier is a receipts surface, not a biography engine.
> Every claim rendered shows its status chip (`documented | platform_inferred | disputed |
> unresolved`) and every badge counts what a named reviewer accepted — never raw passages.
> Do not weaken the visibility predicates; a dossier tab that mixes statuses without chips
> is a defect, not a style choice. No destructive tribal merges in this plan (plan 005's
> recorded deferral stands); the duplicate lane is a crosswalk + canonical pointer only.

Sources of truth: `docs/tribal-territory-solutions.md` §S3/v2 (Scite smart citations,
FamilySearch record hints, WikiTree challenge / Geni merge precedents) and
`docs/tribal-territory-sources.md` (seed tribes verified live).

## Status

- **Priority**: P1 (makes S2's factory consumable by researchers)
- **Effort**: L total, staged
- **Risk**: MED (challenge flow touches the claims/dispute machinery — reuse, don't fork)
- **Depends on**: 030-A (tribe seeds), 030-C (evidence to display); dictionary + claims surfaces exist
- **Category**: identity + provenance-UX
- **Planned at**: post-029 working tree, 2026-11-15

## First principle: what already exists

| Reuse | Where | Feeds |
| --- | --- | --- |
| Per-entity evidence endpoint pattern (accepted-only default, bounded, moderator lane) | `services/core-api/internal/geography/place_evidence.go` | A: `GET /api/v1/tribes/{tribeID}/evidence` |
| Tribe dictionary detail/search | `services/core-api/internal/dictionary/service.go` | dossier page shell |
| Claims + disputes vocabulary (`review_status` cycles, disputed index) | `db/migrations/0003_research_and_jobs.sql` | D: challenges |
| Pairing decision history (reviewer answer beside proposal) | `db/migrations/0063_external_place_pairings.sql`, `db/migrations/0065_pairing_decision_history.sql` | E: duplicate crosswalk decisions |
| Name-candidate nisba vocabulary + review screen hook | `services/core-api/internal/sourceprocessing/name_candidate_common.json`, `SourceSuggestionReview.tsx` | F: nisba hook |
| Suggestion-vs-decision contract (machine reading beside human answer) | `db/migrations/0051_source_tree_suggestions.sql` | B/C semantics |

## Features this plan delivers

| Feature | Surface | Phase |
| --- | --- | --- |
| بطاقة القبيلة (dossier): أسماء / نسب / ديار / حركة / أثر — tabs, each with its own status chip | dictionary detail page | A |
| شارات الأدلة (evidence badges): ✓ يدعم / ✗ يناقض / ○ بلا دليل tallies per claim | dossier tabs + geography evidence endpoints | B |
| إرشادات الاستيراد (record hints): new source → hint cards on matching dossiers | hint queue + dossier cards | C |
| الطعون (challenge): structured dispute → `disputed` until adjudicated | claims machinery + dossier link | D |
| مِسلك التكرار (duplicate lane): alias-based duplicate detection → crosswalk + canonical pointer | moderator-only | E |
| كُنية القبيلة (nisba hook): reading «الحربي» suggests حرب dossier | review screen + dictionary | F |

## Binding stipulations

1. **Badge semantics (interim, honest)**: until plan 034 lands, tallies count *distinct
   `sources` rows* behind accepted items — never passage counts (two passages from one
   source ≈ one receipt). A `sources`-distinct tally is recomputed server-side per request
   and captioned «حسب المصادر المتميزة — قبل تحليل الاستقلال».
2. **Bounded tabs**: every tab's list endpoint carries `LIMIT` + `truncated` (007/028 rule).
3. **No tab without a chip**: undocumented absence is stated («لا شواهد موثقة بعد»), never
   silently blank (the 028-B caption discipline).
4. **Dossiers cite, never assert**: any text the dossier renders beyond tab labels must come
   from a passage or a claim row.

## Phase A — dossier shell + tribe evidence endpoint (3 days, P1)

**Delivered (2026-11-17).**

- `GET /api/v1/tribes/{tribeID}/evidence` (`internal/geography/tribe_evidence.go`) — the
  place-evidence contract verbatim over the tribe's two receipt lanes: the **spatial
  receipts** (spatial_evidence behind `geographic_associations` with
  `entity_type='tribe'`, sourced through the passage, else the association's own
  source id) and the **claim receipts** (the tribe's claims' `claim_evidence` rows,
  each tied to its passage or through its statement to its source). Accepted-only by
  default — the two lanes' acceptance vocabularies differ by table (review_status vs
  the claims' own cycle, where documented/supported are the reviewed-positive states)
  — the moderator `?include=needs_review` reads the same records one level deeper and
  is role-checked there, not trusted from the flag. The spatial lane carries the place
  folio's two visibility walls (the place must be readable, the owning source must be
  too); the claims lane rides the claim system's own unit rule — a claim with one
  private source anywhere in its evidence is not public, so none of its rows reach a
  plain reader. One bounded window (LIMIT 50 + truncated) over the merged folio.
- The dossier's detail shape (`dictionary/service.go` `tribeDetail`) now fills
  المواضع (the associations' places), الادعاءات, and المصادر — with a new
  `tribeSources` read (associations' sources ∪ the claims' evidence sources,
  policy-filtered). Aliases existed; the Q-id crosswalk is still 030-C's pending
  lane (stated, not faked).
- The page: the tribe dossier carries سجل الشواهد — receipt rows with their status
  chips, the source owning each quote, the stated «لا شواهد موثقة بعد» empty state,
  a truncation caption when cut, and (for a moderator) the needs-review band toggle.
- Tests: `TribeEvidence` unit gates mirror the place-evidence discipline
  (private-source invisibility pinned, the moderation lanes' roles, malformed ids);
  the journey `13b-tribe-folio.spec.ts` runs it in the browser (reader and moderator
  sees, relative assertions). Journey 13b's registration also extended
  `e2e/cleanup.sql`'s known rows (tribes/places a synthetic actor published) — the
  file's documented loud-failure mode did its job.

**Acceptance**: private-source evidence invisible to plain reader (unit test pins, as
place-evidence does); tabs render bounded; journey extension relative-only.

## Phase B — badges (2–3 days, P1, needs A + 030-C)

**Delivered (2026-11-17).**

- The badge trio lives on every dossier's `ClaimReference` (`internal/dictionary/service.go`):
  `supportingSources` (✓ يدعم), `contradictingSources` (✗ يناقض — includes `refutes`),
  `mentionOnlySources` (سياق / إشارة) — each a **DISTINCT sources** count behind the
  claim's `claim_evidence` rows (the receipt's owning source via its passage, else via
  its statement), **recomputed per request**, and **per reader**: the source policy
  runs inside each tally, so a badge is the reader's own answer — a private source
  stays in its owner's number and out of everyone else's. Two tallies never partition
  a source: one source may stand on both sides of a claim.
- `Detail.BadgeCaption` carries the derivation caption (`EvidenceBadgeCaption`) beside
  every dossier's tallies: "الشارات تُحسب حسب المصادر المتميزة خلف شواهد الادعاء — قبل
  تحليل الاستقلال" — the interim rule of stipulation 1, until plan 034's reuse-aware
  independence engine replaces the derivation (and its caption).
- The claims read is now `entityClaims(entityType, …)` — one subject/object scoping
  helper for every dossier kind. This ALSO fixed a real defect this work surfaced:
  `tribeDetail` had been calling the person lens' claims read, so a tribe's own claims
  would never have reached its dossier.
- The page: the claims block renders the badge trio per claim («بلا شواهد بعد» when
  all three are zero — ○ بلا دليل is a state, not an absence), with the caption
  served in the response rendered beneath the block; the raw `evidenceCount` is gone
  from the shape (no passage counts anywhere on the surface).
- Acceptance test: `TestTribeBadgeTalliesMatchAnIndependentDerivation` — the 028-B
  comparator pattern: the fixture carries three receipts from ONE source (the badge
  is one), a source standing on both sides of one claim, the private contaminant that
  keeps a claim out of the reader's folio while counting in its owner's, and the
  independent hand-SQL derivation asserted equal to the served tallies.

**Acceptance** (superseded by Delivered): badge totals equal an independent SQL derivation in the test harness (the
`TestRegionEraCounts…` comparator pattern); no client-side counting.

## Phase C — record hints (3–4 days, P2, needs A + 030 running)

**Delivered (2026-11-17).**

- Migration `0069_tribe_passage_hints.sql`: the suggestion rows (one per
  (tribe, passage), carrying the matched forms as the receipt's own words) and
  the append-only decision table (per reviewer, per card. The card = the
  queue's own unit, a (tribe, source) pair, `UNIQUE (tribe_id, source_passage_id)`).
- The matcher (`internal/tribehints`, `MatchTribeHints` over ONE source): the
  dictionary's own forms (canonical + aliases, normalized) matched against a
  passage's normalized text, its raw text, and its arabic locator — the 030-B
  rows' own entry index, so an English-OCR'd passage whose locator says
  «بداية مدخل حرب» still earns its mention. Idempotent: a re-run adds the
  forms only new aliases carry and never touches a decided hint's state.
- The queue: bounded cards (LIMIT 100 + truncated), one per (tribe, source)
  with the passage count and the «N إشارة» tally; a dismissed card is hidden
  for THE DISMISSING REVIEWER ONLY (the decision table is reviewer-scoped;
  the hint rows stay suggested; the card stays for everyone else).
- **Hints never auto-apply**: the match is machine, the application is a
  reviewer's accept — one POST writes the documented receipt in the same
  transaction as the hint rows' own acceptance: a claim (`mentioned_in`,
  `documented`, created_by = the reviewer) with each passage as its
  contextualizing `claim_evidence` row — the receipt the badge trio's
  mention-only column counts, visible through the dossier's evidence folio.
- The matcher ran LIVE over the real Lorimer source (e003): 4 hints suggested
  (حرب، شمّر، قحطان، مطير — each via its Arabic entry locator), one accepted
  (حرب) — and the accepted receipt read back through the tribe's own evidence
  endpoint as a plain reader, with the dossier's badge showing mention-only = 1
  under the served caption. Dev-seed defect fixed on the way: the 030-A tribes
  had landed `private` under migration 0039's flipped insert default (a starter
  catalog no index could see); the generator now inserts `public` and RESTATES
  it on conflict, and the seed was re-applied.
- The ingest-completion automation (the plan-006 job wiring) is deliberately
  deferred: the matcher is a bounded per-source run over its endpoint, and the
  routes are the moderator's own — nothing in any lane applies a hint, so the
  automation's arrival changes nothing about the guarantees.
- The UI: the tribe dossier's إرشادات الاستيراد block (the moderator band):
  the cards with the «N إشارة» tallies, the accept (ثبِّت الإشارات) and the
  dismiss (أخفِ البطاقة) actions, and the stated absences. Journeys stay
  relative; the lane's acceptance criteria (never-auto-apply / per-reviewer
  dismissal / bounded) are all pinned at the service level in
  `internal/tribehints/service_test.go` and the new package is registered in
  the CI required-package manifest.

**Acceptance** (superseded by Delivered): hints never auto-apply; a dismissed hint never reappears for the same
reviewer; queue bounded.

## Phase D — challenges (3–4 days, P2, needs A)

**Delivered (2026-11-17).**

- `POST /api/v1/claims/{claimID}/challenge` (`internal/questions/challenge.go` — the
  questions package is where the dispute machinery lives, so the challenge grows it
  rather than forking it): **one transaction** opens the dispute thread
  (`disputes` + `dispute_claims` at position `opposes`) and flips the claim's own row
  to `disputed`, guarded by `WHERE status IN ('documented','supported')` — a rejected
  claim's state is already its answer, a disputed one has its thread. The challenger
  = any signed-in reader; the claim must be READABLE by them (the claim policy — a
  claim a reader cannot read answers as missing, not as a refusal naming it).
- **The simultaneity is structural**: the flip is the row, so the dossier's chip, the
  badge row, the disputed-claims index and the map's disputed register all read the
  same row — there is no second state to drift; the test pins the dictionary's claim
  read carrying `disputed` the moment the challenge committed.
- `POST /api/v1/disputes/{disputeID}/adjudicate` — the moderator's close: the outcome
  the challenged claims land in is spelled (`documented` keeps / `rejected` retires),
  the resolution text is required (5000-rune bound), and the settle touches only the
  dispute's own `opposes` claims currently sitting in `disputed` — a thread's other
  linked claims (context/support rows) are never touched. Moderator-only by the same
  two-role gate every review lane answers to — the challenge's author cannot close
  their own thread, and the collaboration set never adjudicates.
- The UI: the طعن button on each dossier claim row (the inline challenge form: the
  thread's title + the grounds) with the chip flipping in place; and the adjudication
  column in the question workspace (`DisputeAdjudicationPanel`): the moderator's
  outcome select + the resolution text per open thread, with the stated refusal for
  everybody else.
- Tests: the arc (`claim → challenged+disputed → adjudicated+settled`) unit-tested
  including the 403s, the outcome vocabulary, the re-adjudication refusal; the chips'
  simultaneity pinned at the row; the audit trail asserted. Journey
  `13c-challenge.spec.ts` runs the arc in the browser (reader challenges, cannot
  self-close, the moderator settles, the same row re-reads documented) — relative
  assertions only. The dispute's own tables are unchanged: **no migration belongs to
  this phase**.

**Acceptance** (superseded by Delivered): dispute → status → close arc unit-tested; the challenged claim's badge shows
the dispute on every surface simultaneously.

## Phase E — duplicate crosswalk lane (3 days, P2, needs A)

**Delivered (2026-11-18).**

- Migration `0070_tribe_duplicate_lane.sql`: the candidates (the ordered pair
  SORTED BY CANONICAL NAME so a pair is one row forever, both matched forms
  verbatim, the similarity as the builder measured it) + the append-only
  decisions on the 0065 pattern, whose `diff` carries the reviewer's canonical
  pick beside the proposal.
- The offline builder `infra/local/build-tribe-pairings.py` (the 028-F pattern):
  reads the COMMITTED tribe subset (`tribe-seeds.json`), compares every pair's
  forms (the same trigram proxy: SequenceMatcher·0.5 + trigram Jaccard·0.5),
  emits `tribe-pairings.json` + an idempotent seed (`ON CONFLICT DO
  NOTHING`). First run over the real seeds: **4 candidates** (حرب⇄شمّر/0.60,
  حرب℄مطير/0.57, شمّر⇄مطير/0.62, عتيبة⇄عنزة/0.60 — all through the سّhared
  «قبيلة …» alias prefix), applied to the stack.
- The lane (`internal/tribepairings`): bounded list per status (ordered by the
  measured similarity), and ONE decide endpoint over the three effects:
  - `link_alias` — the LOSER's matched form inserts as the canonical's alias
    (_values verbatim + the normalized key_, `ON CONFLICT DO NOTHING`), so the
    dossier's أسماء tab reads it;
  - `canonize` — an append-only `entity_relationships` row
    (`predicate='canonical_pointer'`, `documented`, created_by = the reviewer)
    points the loser at the canonical;
  - `reject` — nothing but the decision.
  The canonical must be ONE OF THE PAIR (a third-party pick = 400 — a merge by
  another name); a second decide = 409; the history is moderator-readable,
  append-only.
- **The lane's own law, pinned by four tests**: no statement anywhere writes a
  `tribes` row — the link-alias test asserts the loser's canonical name stayed
  verbatim while exactly one alias row appeared on the winner; the canonize
  test asserts the pointer and ZERO alias rows; the reject test asserts
  nothing moved at all. The gates are the same review-role pair as every lane;
  history/journey relative only.
- The page: the tribe dossier's مِسلك التكرار block (the moderator band) — the
  pair's own cards with the matched forms + the similarity as a percent, the
  canonical radio (this tribe / the other), and the three actions.
- Deliberately NOT here: merging `tribes` rows (plan 005's recorded deferral
  stands — the merge's own machinery is a future plan's charter), and the
  duplicate queue's cross-tribe walk (the dictionary's own index IS the walk:
  each tribe's dossier carries its pairs; the moderator lane sees them beside
  the receipts).

**Acceptance** (superseded by Delivered): no `DELETE`/`UPDATE` on `tribes` from this lane; decisions append-only.

## Phase F — nisba hook (2 days, P2, needs A)

**Delivered (2026-11-17).**

- The matcher (`internal/sourceprocessing/tribe_nisba.go`): the machine's reading's
  FINAL token must (a) end in ي and be nisba-shaped, (b) sit in the repository's own
  embedded common-name corpus (`name_candidate_common.json` — the gate is the repo's
  data, not an invented morphology rule), and (c) resolve to a dictionary tribe —
  the root carries the definite-article strip AND the ta-marbuta form, so
  «العتيبي» reaches عتيبة (normalized عتيبه) and «سعيد الحربي» reaches حرب (the
  canonical directly, or the aliases whose final token is the root: «بني حرب»).
- The emission: the review context (`NamePreparationContext.tribalSuggestion`) carries
  the card from `BeginNameReview` — and it is **recomputed on the cached-exposure path
  too**: a card is never frozen into a stored context, so a tribe added after the
  first read still earns its card on reload. The card = `TribeID/TribeName/NisbaForm` —
  a link into the dossier, nothing else.
- **Suggestion-only, pinned twice in the acceptance test** (`tribe_nisba_acceptance_test.go`,
  run over the REAL pipeline: three name-groups read through the local reader):
  the hook records ZERO decision rows, and the dictionary grew by exactly the
  rows the test itself planted — the ledger's schema and rows are the reviewer's own,
  and NO migration belongs to this phase (the ledger schema is unchanged, the literal
  acceptance criterion). The cached path's card freshness is asserted too.
- The screen (`SourceSuggestionReview.tsx`): the كُنية card renders in the
  اقتراحات lane beside the other candidate cards — «الاسم المقروء يحمل نسبة إلى
  … — اقتراح بلا إثبات انتماء» — and opens the tribe's dossier (the index deep link).
- The journey: extended `11-name-bench.spec.ts` ("a nisba reading shows the tribe
  card as a suggestion only") — steps two seeded readings, asserts exactly one
  carries the card with the membership disclaimer, pins zero decision rows, relative
  assertions only. The pure matcher (the token gate, the ta-marbuta patterns, the
  vocabulary gate) is covered in `tribe_nisba_matcher_test.go`.

**Acceptance** (superseded by Delivered): suggestion-only; decision ledger unchanged schema; e2e relative assertions.

## Execution order

| # | Phase | Effort | Depends on |
| --- | --- | --- | --- |
| 1 | A dossier + endpoint | 3d | 030-A |
| 2 | B badges | 2–3d | A |
| 3 | F nisba hook | 2d | A |
| 4 | D challenges | 3–4d | A, 030-C |
| 5 | C hints | 3–4d | A, 030 factory |
| 6 | E duplicate lane | 3d | A |
| 7 | (034 later upgrades B to independence-weighted tallies) | — | 034 |

Total ≈ 3 weeks of focused work; every phase ships and demo-verifies alone.

## Verification gates

Delivered state (2026-11-18): all six phases (A/B/C/D/E/F) shipped and gated;
`make verify` green; the DB-backed suites green; journey extensions (11's nisba
case, 13b, 13c) relative-only. One deliberate residue: the tribes' merge
machinery stays plan 005's recorded deferral; the lane's own undo is data.

`make verify` per phase (geography/dictionary endpoints, badge derivation tests);
journey extensions relative-only; `make verify-full` before close; status rows in
`docs/phase-status.md` + `plans/README.md` per phase.
