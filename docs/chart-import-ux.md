> Presentation correction: the original landing page, Sources hero and catalogue,
> and header/sidebar brand details have been restored. The guided import and
> dedicated tree/name-review routes remain. Simplifying a journey must preserve
> the website's established visual identity.

# Chart import journey

The subsequent [saved-chart and name-review audit](chart-library-ux.md) expands
this first pass with a real library, direct name-review routes, preserved landing presentation,
and explicit access to existing work. Its navigation guidance supersedes the
four-destination arrangement described below.

The import journey now lives at `/import`. The interface keeps Dawha's Arabic
RTL layout, typography, and paper/ink palette.

## Problems found

- Import was hidden inside Sources. Researchers had to know that “Family document”
  was the source type that unlocked tree import.
- Creating source metadata and uploading the chart were separate tasks. Quotes,
  claims, dependency analysis, name-reading experiments, and tree creation competed
  for attention on the same long page.
- The sidebar gave nine features equal priority and included placeholder actions.
- Sources displayed both saved records and a separate demonstration catalogue with
  unrelated counts, making it unclear which records users could work with.
- Review required two checkboxes per name, could lose changes on dismiss, and
  offered no way to restore an excluded name. Tall page previews pushed connection
  controls below the visible workspace.
- The final import preview crashed when the API returned `null` for empty collision
  lists. The live browser walkthrough reproduced this and the client now normalizes
  both empty collisions and warnings.

## The resulting journey

1. Choose **استيراد مشجر** from the sidebar, home, tree, or Sources page.
2. Choose/drop a PDF and name the tree. The filename suggests an editable name.
   Sign-in is checked before uploading; its return URL preserves the destination.
   Format, empty-file, and 128 MiB limits are checked before creating a source.
3. **رفع ومتابعة** creates the family-document source and uploads the file in one
   action. The saved source ID survives an upload failure so retrying does not
   create duplicate source records.
4. Processing status appears on the same page. The source/file URL survives reload.
   Failed processing offers retry; a missing file directs the researcher to its
   saved source. Sources has a direct **متابعة استيراد المشجر** action per file.
5. **مراجعة الأسماء والوصلات** opens the existing conservative review workspace.
   The draft ID is added to the URL. Human verification and explicit connection
   tracing remain necessary; machine readings do not establish relationships.
6. One checkbox verifies and includes a name. Excluded names can be restored.
   Zoom/fit controls preserve page proportions and keep connection tools nearby.
   Unsaved edits are protected on dismiss and browser navigation.
7. **متابعة إلى ملخص الشجرة** saves edits and loads the import preview. It always
   presents a separate confirmation, including for imports without collisions.
   Editing invalidates that confirmation. Proposed connections must be traced or
   removed before continuing.
8. **تأكيد وإنشاء الشجرة الخاصة** creates the private draft tree. The completion
   state links directly to it. Publishing remains a separate tree action.

## Navigation and disclosure

Four primary destinations remain visible: Home, Trees, Sources, and Research.
Search, Questions, Places, Identity matching, Contradictions, the public index,
and job operations are available under **أدوات البحث**; the group opens on those
routes. Import is a persistent primary action. Placeholder saved/settings links
and nonfunctional top-bar controls were removed; top-bar search opens real search.

Sources now shows saved records with search and a bounded list. Source quotes,
claims, dependency analysis, and experimental reading tools remain available
through explicit disclosures. The existing non-chart source upload flow remains
available through **مصدر جديد**.

## Verification

Automated coverage exercises validation, sign-in gating, failed-upload retry,
URL recovery, missing-file recovery, explicit final confirmation, saving before
preview, unsaved-close protection, and nullable preview lists. The existing name
bench browser helper was updated to open its advanced-tools disclosure.

A real browser walkthrough against an isolated test database uploaded a synthetic
PDF, reloaded it, entered and reviewed two names, traced their parent connection,
saved the review, confirmed import, and opened the resulting private draft with
two people and one relationship. Desktop and narrow mobile layouts were inspected.
