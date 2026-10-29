# Saved charts and name review: deeper UX audit

## Presentation boundary

The original editorial landing page at `/` is preserved: its hero, orbital artwork,
metric cards, research panels, invitation and footer belong to the product's
identity. The earlier replacement of Home with the library was reverted after
user feedback. Keep the workbench at `/tree`, chart-specific pages under
`/tree/charts/:fileId`, and guided import at `/import`. The old `/names` routes
redirect to the corresponding workbench view or inner review page. Simplify task navigation without stripping the existing
visual compositions, typography or brand details. The original Sources artwork
and catalogue presentation are restored around the working evidence workspace;
demonstration content remains explicitly labelled.

## What the existing local data actually contains

Read-only inspection of the local development database on 2026-10-28 found:

| Record | Count | Why it was hard to find |
| --- | ---: | --- |
| Uploaded family-chart sources | 11 | Stored as private sources, omitted from the digital-tree list |
| Name-suggestion records | 6,138 | Accessible only through a selected source's advanced processing controls |
| Pending names | 6,027 | No cross-chart review index or visible entry point |
| Deferred names | 14 | Saved, but required finding and reopening the original source |
| Accepted/corrected/rejected current names | 97 | Existing decisions remained in the ledger |
| Digital tree rows | 2 | Only one has versions; the second is a demo seed without any version |

No chart or name-review data was removed by the earlier UX work. A signed-out
browser could not see the private chart files. The earlier browser verification
also replaced the real local workspace session with a test session on another
localhost port. The existing reviewer session has been restored, and connected
state is now explicit in the sidebar and header. Future live verification must
keep test authentication out of the user’s active browser session.

The first pass made the upload
form clearer but did not provide an inventory of existing work and incorrectly
classified name review as an advanced feature.

The second demo tree's zero versions are a separate seed-data condition. The UI
does not fabricate a tree version or represent an uploaded image as a completed
digital tree.

## Journey problems and corrections

1. **A collection route opened one object.** `/tree` chose the first accessible
   digital tree, with other trees hidden in a selector. It now opens a searchable
   library containing uploaded charts and digital trees. Individual trees retain
   their existing `/tree/:treeId` and version addresses and have a return link.
2. **Uploaded charts vanished from the product's tree vocabulary.** They now stay
   visible throughout processing, name review, connection review, and import.
   The library distinguishes uploaded charts from digital trees in plain Arabic.
3. **Saved work needed a clear destination.** `/tree` is the saved-work library,
   with real counts and useful actions. Home retains its original presentation
   and links into that library. Demonstration data is labelled, and dashboard
   failures keep the brand hero visible while showing a data-unavailable panel.
4. **Name suggestions had no stable entry point.** My trees now contains a
   visible review view at `/tree?view=review`, with pending/deferred counts, a
   deferred-only filter and direct start/continue actions. Chart titles open an
   overview; review actions open `/tree/charts/:fileId/names`. Old `/names` links
   redirect without losing chart context. Upload and source pages link directly.
5. **Counts failed to explain what remained.** Each chart shows pending and
   deferred work and saved decisions. The review index includes deferred work
   when filtering for charts that need review. Counts are computed on the server
   and exclude superseded occurrences; failed queries show unknown counts.
6. **An import resumed by file could start another draft.** The index returns the
   latest persisted draft reference. Connection/import links carry it, and the
   import page checks for an existing draft before creating one. Imported and
   retracted states remain addressable.
7. **Large name queues hid the remainder.** The review workspace initially loads
   200 names, and its automatic prefetch only activated after decisions reduced
   that batch. It now has an explicit load-more action and explains that its
   spelling/page filters apply to the currently loaded names. Browsing further
   requires no review decision.

## Information architecture (user priorities, 2026-10-29)

The sidebar has three explicit priority groups, separated by rules and spacing.
The original landing and Sources artwork, typography and warm palette remain.
The landing hero's main shortcuts open My trees and Geomap; research remains
available lower in navigation and in its original editorial sections.

| Priority | Group | Destinations |
| --- | --- | --- |
| First | مساحتك | Landing `/`, My trees `/tree`, Geomap `/places` |
| Second | اكتشف | Browse platform trees `/explore`, Sources `/sources` |
| Third | البحث | Research office `/research`, Search/index `/search` |

- **My trees:** uploaded charts the viewer can review, plus digital trees they
  own or explicitly collaborate on. Search and filters persist in the URL;
  records are not truncated to five. Signed-out readers receive sign-in guidance.
- **Workbench views:** All work includes every saved chart and digital tree;
  Review includes charts with pending/deferred names and one focused review
  action per row; Digital trees contains the editable/published digital records.
  These are internal views of My trees, with no duplicate sidebar destination.
- **Chart inner pages:** `/tree/charts/:fileId` shows actual counts, processing
  state, next action, and saved connection-draft context. It links to the inner
  name-review page, connections/import, and the source record. Closing name
  review returns to that chart. History and deferred shortcuts select the
  appropriate queue on the first request, rather than opening pending work.
- **Browse platform trees:** public trees with a published version. A tree's
  latest published snapshot is shown even if its owner has a newer draft. Links
  open that exact version, keep discovery highlighted, and return to browsing.
- **Sources:** bibliographic records and evidence work, with direct name-review
  links for chart files and the original catalogue presentation.
- **Import:** a task action inside My trees, covering upload,
  name review, connections, confirmation, and the resulting private digital tree.
- **Research:** questions, evidence queries and data review are inner views of
  the Research office. Search and the record index share their own destination.
  Processing status belongs to Sources. Geomap remains first-class.

`GET /api/v1/trees?scope=workspace` returns owned/explicitly shared trees.
`GET /api/v1/trees?scope=public` returns public published snapshots. The original
unscoped endpoint keeps its existing behavior for older consumers. Scoped lists
never substitute a synthetic tree on failure. No records are moved or deleted.

Name suggestions, human decisions, connection drafts, and digital trees remain
separate records. These changes make their navigation explicit without turning
an undecided machine reading into a person or a relationship.

## Data access and verification

`GET /api/v1/chart-library` is a read-only aggregate. Public source titles remain
public, but file IDs, review counts, reading runs, and draft references use the
same source-review permission policy as the existing workbench. Private sources
are omitted for unauthorized viewers. The aggregate uses one query rather than
one processing/candidate request per chart and preserves records awaiting upload.

Database-backed tests run in isolated fixture schemas and verify private-source
isolation, public-title-only access, counts excluding superseded occurrences,
and records without files. Browser-level component tests verify more than five
charts, digital-tree coexistence, landing presentation and outage behavior, deferred work, direct review
URLs, sign-in guidance, partial-query failure, and saved-draft continuation.

The live browser was signed in with the existing development reviewer, after
which all 11 private charts and the published tree were visible. Direct review
of the existing Sugaran chart showed its 676 pending names, 14 deferred names,
and 111 prior decisions. Existing review data was inspected without changing any
name decisions or preparing a new reading run.


Navigation verification on 2026-10-29 confirmed all 11 uploaded charts in My trees,
the existing platform tree in Browse trees, exact published-version navigation,
and preservation of the existing reviewer session. Component tests cover group
ordering, a single workbench entry, signed-out workbench behavior, public browsing and
search/failure states. Database tests cover owner/collaborator isolation, public
visibility, unpublished-tree exclusion, and published-versus-draft selection.


## Consolidation after duplicate-page feedback

The separate name-review catalogue was redundant with My trees. `/names` now
replaces its history entry with `/tree?view=review`, preserving search/deferred
filters. `/names/:fileId?queue=...` similarly redirects to the chart's review.
The overview, name review, and connections page share a chart navigation bar.
All work retains charts that are completed, unprepared, processing, or missing a
file. Only charts with pending/deferred names enter the task queue. Browsing an
overview never starts a reading run, creates a connection draft, or changes a
human decision. Preparation and import remain explicit actions.

Visual reference: `tmp/ux-workbench/before.jpg` was used to generate an iterative
UI reference (`exec-78df823c-d29b-40f8-a24d-f4fc255d8f54.png` in the session's
`.codex/generated_images` folder). The brief preserves the right sidebar,
ivory/rust palette, Arabic type and ruled tables, removes the duplicate sidebar
link, and adds thin internal navigation tabs plus a focused review queue.
