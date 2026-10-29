# Name review: crop visibility and bad spot proposals

## Follow-up: removing branch placeholders from the live queue

The previous UI pass left the old machine proposals active. That was insufficient: the reviewer still encountered the same branch fragments. The selected chart has now actually been refreshed.

- Its 88 untouched legacy proposals are retained in history and retired from the active queue.
- Inspection of the first refreshed pass also caught yellow box borders and dark decorative patches. Detector v3 requires the light-yellow lettering evidence to contrast against a green leaf background and uses the dominant text polarity instead of appending dark tree components as more names. The prior v1/v2 algorithms remain available for pinned runs.
- The final local pass completed with 50 text-region candidates, zero candidates below the fragment floor, and no automatic name acceptance. This is a conservative subset, not a completeness or precision score. The original image is low resolution and these names still need manual reading.
- All six existing human decision records are byte-for-byte unchanged, verified with a digest before and after the refresh. No human rejection was fabricated for discarded machine geometry.

Full-file detection now stages new rows until every page/tile succeeds. Publishing swaps the untouched machine queue atomically, appends `redetected` events, and preserves reviewed, reopened and geometrically edited rows. New detections that substantially overlap protected human geometry are retired as duplicates; a one-pixel rejected fragment cannot suppress an entire word. Failed/incomplete scans leave the prior queue available. A later requested scan wins over an older scan that finishes late. Single-spot rereads do not replace the file queue. Library and review counts exclude staged work. Migration `0061_name_localisation_publication.sql` adds the staging flag and audit event.

Verification includes database-backed publication, staging, failure and human-work preservation tests; dark-decoration and yellow-border negatives; the existing name job, retry, cancellation and geometry tests; web tests, lint/build, generated database models and live browser checks. The earlier findings below describe the first investigation, before this refresh was applied.

The review screen at `/names/c45dfab0-7ffc-44bb-b6c9-428dacbff821` was inspected against the saved source, its stored regions, and the running UI. This follows the library and import work in `chart-library-ux.md`.

## Findings

- The selected chart is a 768 × 1138 raster with light yellow names on green leaves. The localiser primarily searched for dark ink. It found veins, branch edges and isolated marks while missing much of the lettering.
- At inspection, 87 of 89 pending proposals met the new fragment floor. The selected proposal covered approximately 1 × 2 source pixels. There were five existing rejections. These are observations about this chart, not a corpus-wide accuracy estimate.
- The context image showed neighbouring names without indicating which tiny spot was selected. The page map started at page width, cropped off most of this portrait chart, and used a subtle rectangle that was nearly invisible for a one-pixel region.
- The crop's displayed “100%” meant a fitted scale that could already enlarge the source twice. Zoom carried between selections and had no fit reset or lower steps. Large zoom could also move the selected part outside the scroll viewport.
- Repair required editing four percentages in a separate column, making it hard to connect the values to the image.

## Implemented behavior

The existing ivory/olive workbench, RTL reading flow and decision controls are retained. The generated reference anchors the changes to two adjacent evidence figures, a restrained orange locator, an amber fragment warning and a full-page map. The source images remain real source pixels.

The crop endpoint now returns crop-local geometry from the exact source frame and clamped render bounds. This metadata is calculated after artifact retrieval, so cached and freshly rendered crops have the same locator. The context marker is placed on the image itself, without an object-fit letterbox. The page has an orange target and crosshairs, a full-page default, and “إظهار الموضع” to zoom and locate the current proposal.

Fit caps enlargement at native scale. Explicit zoom percentages refer to source pixels, include smaller steps, and reset when moving to another name. A fit control restores the whole crop. Zoomed scroll containers center the selected region.

For an existing fragment, a separate bounded review image shows nearby source pixels instead of enlarging a dot. Its marker still represents the original proposed boundary. This display image is derived from the cached context and does not change the tight image sent to readers, immutable artifact identity, source pixels, or stored region. The warning and next-action panel explain that a fragment may be a drawing or part of a letter, and offer repair or rejection.

Repair now supports dragging a replacement rectangle on the page. Percent fields remain available. Preview and save controls stay beside the map; changing the rectangle invalidates the preview, and saving remains disabled until the new bounds have been previewed. The live test drew a rectangle, fetched its server preview, and cancelled it.

The first detector update used `colour-text-anchor-v1+contrast-name-groups-v2`; the follow-up above supersedes it with v3. New detection runs exclude proposals with a source-pixel short side below 5, long side below 10, or area below 64 pixels. Light yellow lettering is detected through local contrast against darker saturated neighbours. Dark and light components are grouped separately to avoid joining letters to branches. Light groups supplement dark groups only when light lettering dominates; overlapping groups are deduplicated. No semantic recognition of every decorative shape is claimed.

Durable runs already pinned to the previous detector retain that detector when resumed. Their evidence is not silently relabelled as results of a different algorithm.

## Data and limits

During the initial UI pass, no saved suggestion was deleted, accepted, rejected or geometrically revised. At that stage the inspected chart still had 89 pending suggestions and five rejections. Bad proposals were only flagged; this gap is corrected by the applied refresh described in the follow-up above. This avoids silently replacing the evidence or creating an additional live queue while the user is reviewing it.

The raster's original resolution limits what can be read. Zoom cannot recover missing letter detail. Large decorative shapes can still pass geometric checks, and some names remain undetected. The corpus check measures consistency between two renderings, not localisation precision or transcription accuracy. A labelled positive/negative corpus is still needed for those metrics.

## Verification

- Web: 120 tests, including native fit scale, zoom out, reset on next name, crop-local marker coordinates, drawing bounds, and preventing save before preview.
- Go: sourceprocessing and httpapi suites; source-space/clamped geometry, bounded fragment view, light-letter/blank-leaf fixtures, thin-stem/dot negatives, and the saved low-resolution chart regression.
- Database-backed crop acceptance: geometry survives cached reads and existing reader/artifact behavior is preserved, using isolated test schemas.
- Paired-chart consistency check passed: name-group counts within 8.2% in the measured pair, below its existing 15% guard.
- Live desktop: target locating, rectangle drawing, server preview and cancellation. Phone viewport: 391 × 845 CSS pixels, no horizontal overflow; full-page map fits its viewport.
- Lint, production build and diff whitespace checks.

Local visual evidence: `tmp/ux-crops/before.jpg`, `tmp/ux-crops/after-desktop.jpg`, and `tmp/ux-crops/after-mobile.jpg`. These local screenshots are ignored by Git.
