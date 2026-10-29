# Research tools consolidation

## Scope and presentation

The existing Landing, My trees and Geomap remain first-class pages. Their artwork
and compositions are preserved. Research keeps Dawha's ivory paper, rust accent,
Arabic type, ruled evidence panels and original question-orbit illustration.
The generated reference (`exec-c63c55e7-2b9f-429f-915a-dacd2ab3ecfd.png` in the
session's generated-images directory) was based on screenshots of the original
Research and Questions pages, and specifies consolidation without a redesign.

## Final destinations

| Previous destination | Current task location |
| --- | --- |
| Research and Questions | `/research`: Questions and Evidence search |
| Identity resolution and Contradictions | `/research?view=review`, with two review views |
| Unified search and Dictionary | `/search`: Content search and Record index |
| Jobs / manual worker console | `/sources?view=processing`: source-file processing status |

The sidebar has two Research links: Research office and Search/index. There is
no duplicate Research Tools catalogue. Old URLs redirect to their corresponding
views, preserving relevant query, selection and scan references.

## Incomplete paths corrected

- Removed the Research draft button that only closed an unsaved form, inert tabs,
  fake filter counts and unlabelled sample comparisons from the working flow.
  The original question artwork remains around a single real question index.
- Question creation writes through the existing API, retains input on failure,
  and opens the saved investigation after success. Questions show actual counts,
  states and updated times and can be filtered without duplicating an index.
- Investigations have separate Evidence, Tree/places, Analysis, History and
  State/notes views. State, priority, notes and related sources are persisted.
  Claims use named record selection; evidence uses a source/statement selector.
- Search and dictionary results retain exact record, source, question and
  published-version destinations. Selected dictionary records and search state
  survive reloads in their URL.
- Lexical passage results previously omitted the source ID; the API now returns
  source ID, passage ID and locator, while keeping the existing visibility rule.
- Valid stored UUIDs with legacy version bits were rejected by an overly narrow
  browser check. They now load their real workspace instead of sample content.
  Only the three explicit `q-*` demonstration addresses use labelled examples.
- Review access follows current platform roles returned by `/auth/me`; reads do
  not grant permissions. Research review is available to researchers/moderators/
  administrators, while identity merge and reversal remain moderator/admin work.
- Scan addresses retain their run ID. Review data refreshes after completion;
  queued/running scans cannot be started again through an enabled button.
  Contradiction history is filterable and created questions have direct links.
- Reversal uses an inline reason form instead of a browser prompt. Diagnostic
  JSON stays in expandable details rather than dominating the review cards.
- Source processing shows the chosen source's files and real state, polls active
  work, and links failures to the existing source recovery workflow. Manual job
  claiming/completion and arbitrary JSON payloads are removed from the user UI;
  backend worker APIs and processing services remain available.

## Verification boundaries

Automated component tests exercise the question save/failure journey, evidence
query/citation links, exact search/index destinations, processing migration,
role restrictions, scan completion, saved notes and legacy UUID behavior.
Database fixtures verify current session roles without privilege changes, and
passage destinations with public/private source isolation. These fixtures use
isolated schemas, not the user's browser session or research records.

Live checks use the existing session, sources and investigation without saving
new notes, claims, merges or decisions. Desktop and mobile checks preserve the
original artwork and primary-page navigation.

Final validation: 146 web tests passed; lint and production build passed; auth
and search tests passed against isolated PostgreSQL fixtures; HTTP API checks
passed. Live desktop/mobile verification covered the real stored investigation,
its legacy UUID, public record details and published-version links, account-aware
review gating, and the existing alsharmi source's processing status. Research,
investigation, content search and record index fit a 391px mobile viewport with
no horizontal page overflow. The user's existing session and saved decisions
were preserved.
