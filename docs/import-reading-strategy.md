# The import reading strategy: what the documents actually are, and what follows

**Status: measurement-driven recommendation. It proposes three plans; it does not implement
them.** Written 2026-10-21 as an addendum to `docs/ocr-and-import-decision.md`, which remains
the authoritative statement of what a reliable import needs and of the pilot as built.

That document is thorough and, in the parts that matter, correct: it separates reading from
connection from interpretation, refuses to let machine output become fact, requires independent
annotation before any accuracy claim, and records its own hosted-vision runs as failures.

This document adds four things it does not have:

1. **A measurement of the one chart we actually have**, which changes the architecture.
2. **The published evidence** for why the hosted-vision route produced the numbers it did.
3. **An audit of what the import actually writes**, which finds the pilot closed most of the
   materialization gaps and left two that matter.
4. **The Arabic reading and confidence evidence** that the provider shortlist in §3 of the
   decision document deliberately leaves unqualified.

---

## 1. The measurement, and why it changes the plan

The pilot has one document: the Jasser chart, authorised by SHA-256 for the hosted-vision
experiment. It sits on disk, private, and it is A4. Everything below is measured on it, with
no annotation and no model calls.

| Property | Measured | How |
| --- | --- | --- |
| Pages / size | 1, A4 (595×842 pt) | `pdfinfo` |
| SHA-256 | `5735335e…07fd` | matches `JasserChartSHA256`, `chart_analysis.go:29` |
| Fonts | **0** | `pdffonts` — no text layer at all |
| Vector paths | **0** | PDF object scan — nothing for a vector extractor to read |
| Images | 2 | one **1708×2592 JPEG at 236 ppi**, one 240×90 logo |
| `pdftotext` output | 1 code point | it is a pure scan |
| Ink segments found, 300 DPI | **9,345** | replica of `detectOrthogonalLineSegments` |
| Segments kept | **1,000** | `MaxLineCandidates`, `line_detector.go:12` |
| Segments **discarded** | **8,345 (89.3 %)** | |
| Same at 400 DPI | 12,065 found, 1,000 kept (**91.7 %** discarded) | |
| Same at 240 DPI | 7,069 found, 1,000 kept (**85.9 %** discarded) | |

### 1.1 It is not a box-and-line family tree

Look at the document. It is a **decorative painted tree** — a photograph-quality illustration
with brown branches, green grass, a blue sky and a waterfall — and the names are written in
black Arabic **on coloured fruit and leaf shapes** hanging from the branches.

There are no connector lines. There are no name boxes. The structure of the genealogy is
carried by the **painted foliage**, and the parent/child relation is implied by which branch a
fruit hangs from.

This matters enormously, and it invalidates the framing this document started with:

- **Native PDF vector geometry cannot help this document.** The file has zero vector content
  because it is a scan. And a *digitally authored* version of the same decorative tree would
  also have no connector lines to extract — the branches are painted. Plan 019 as originally
  written addresses a document class that this corpus may not contain at all.
- **A raster line detector is the wrong instrument entirely.** The detector thresholds at
  grey < 150 and looks for orthogonal ink runs. On a photograph of a landscape, what it finds is
  the landscape.

### 1.2 The truncation is not merely lossy — it is spatially destructive

At 300 DPI, of the 9,345 candidate segments, the 1,000 that survive are distributed like this
(rows = page quarters top to bottom, columns = page quarters left to right; `kept/discarded`):

```
     102/72      207/553     282/639     148/268
      38/296       0/977       0/980       0/243
      53/263       0/887       0/798       0/297
     170/398       0/768       0/517       0/389
```

**The right three-quarters of the page below the top quarter have zero surviving candidates.**
The sort before truncation is by `fixed` (`line_detector.go:46-51`), which is the row for
horizontal runs and the column for vertical ones, so the survivors are whatever has the
smallest row-or-column index. The truncation is systematic, not random.

Two more measurements make it worse:

- The **median candidate is 54 px** at 300 DPI (≈4.6 mm). That is a letter stroke or a grass
  edge, not a connector.
- **Only 10.2 % of the long segments survive.** 42.8 % of candidates are ≥60 px — long enough
  to be a box edge or a connector — and the cap preferentially discards them, because long
  segments are fewer and spread across the page.

### 1.3 Raising the DPI makes it worse

`minHorizontal = max(28, width/100)` (`line_detector.go:40-41`) scales with resolution, so more
pixels produce more qualifying runs, while `MaxLineCandidates` is a constant 1,000. The
discarded fraction rises with DPI: **85.9 % at 240, 89.3 % at 300, 91.7 % at 400**.

The product currently offers "re-OCR at 400 DPI" as a remedy for a poor scan. On this chart
that remedy makes the geometry loss strictly worse. The 400 DPI option is right for *reading*
and wrong for *geometry*, and the pipeline offers it for both.

### 1.4 Colour segmentation is the correct localiser — and it is free

The name carriers are saturated colour blobs on a distinct background. Segmenting by
saturation and connectivity, with no model:

| Render | Saturated blobs | Leaf-sized (1.5k–20k px) | With an interior name mark |
| --- | --- | --- | --- |
| 300 DPI | 1,579 | 148 | **135 (91.2 %)** |
| 600 DPI | 1,701 | 493 | **404 (81.9 %)** |

Two things fall out:

1. **The chart really does carry several hundred names.** 404 detectable name carriers at 600
   DPI is consistent with the decision document's "500-plus visible names"
   (`docs/ocr-and-import-decision.md:55`). That figure is credible.
2. **Resolution is not the lever, and an earlier draft of this document said otherwise.** The
   first measurement here compared *absolute pixel areas* and concluded that 600 DPI tripled
   the yield. That was an artefact of the band: an absolute area band moves a carrier into a
   different bucket when the render resolution changes. Measured as a fraction of page area,
   the carrier distribution at 300 and 600 DPI is the same:

   | name-carrying components, by area as a fraction of the page | 300 DPI | 600 DPI |
   | --- | --- | --- |
   | 1413: below 0.05 % of page | 605 | 648 |
   | 1413: 0.05 – 0.20 % | 8 | 8 |
   | 1413: above 0.20 % | 5 | 5 |
   | 1432: below 0.05 % of page | 993 | 1,057 |
   | 1432: 0.05 – 0.20 % | 27 | 29 |
   | 1432: above 0.20 % | 4 | 4 |

   The populations match; 600 DPI finds about **7 % more** components, not three times as many,
   and the extra is less fragmentation. **The localiser therefore needs no second render** —
   the 300 DPI the pipeline already produces is enough — and the 16 MP render cap is not in
   the way.

### 1.5 Tesseract cannot read these names, and the reason is not scan quality

Crop a single fruit tightly at 600 DPI and the name is large, crisp, black-on-green, and
**perfectly legible to a human**. Tesseract's `ara` model still fails:

| Input | Tesseract output |
| --- | --- |
| Tight 600 DPI crop of "محمد" | `2` |
| Same, globally binarised | `سد`, `(سدا`, `نسدد` |
| Same, colour-masked, local threshold | `!` |
| Whole page, 300 DPI, `--psm 11` | 342 garbage "words" — digits and fragments |

The binarised image shows why: the decorative display face **fuses the letters into a single
connected mass**. Tesseract's Arabic model is trained on printed Naskh, where letters connect
in the standard way with distinguishable baselines and dots. This face connects them
differently, and that is outside its training distribution.

This is the opposite of a resolution problem and the opposite of what the KITAB numbers
predict for clean print. It is a **font** problem, and it is invisible in every benchmark
because no benchmark contains this font.

**Consequence.** For this document class the reader must be a vision model, not Tesseract —
and it must be asked the *right* question.

---

### 1.6 The paired check, on two renderings of the same family

A second rendering of the same tree exists: **شجرة أسرة الجاسر ١٤١٣**, a different drawing of the
same facts. It is not a variant of the first — it is a different artefact.

| | ١٤١٣ | ١٤٣٢ |
| --- | --- | --- |
| Form | hand-drawn tree on white paper, names in elongated **leaf** shapes | photographic painted tree on a sky-and-grass background, names on coloured **fruit** shapes |
| Page | Letter, 612×792 pt | A4, 595×842 pt |
| Embedded image | 1439×2048 | 1708×2592 |
| Native resolution | **202 ppi** | 236 ppi |
| Text layer / vector content | none / none | none / none |
| Extra content | a vertical stack of pink ovals down the trunk (the direct-ancestor chain) and **two scroll panels of dense Arabic prose** — a mixed name/prose class | a second, smaller logo image |

The ink-run detector, the colour localiser, and the same cap, on both:

| Measured | ١٤١٣ | ١٤٣٢ |
| --- | --- | --- |
| Ink-run segments at 300 DPI | **2,501** | **9,345** |
| Ink runs discarded by the cap | **60.0 %** | **89.3 %** |
| Name carriers found by the implemented localiser, 300 DPI | **398** | **388** |
| …agreement between the two renderings | **2.6 %** | |

Three things fall out of this, and they are the first real numbers in this area.

**The photographic background is what breaks the ink detector.** Same detector, same cap, same
facts: the drawn chart produces 2,501 candidate segments and the photographic one 9,345, a
factor of 3.7. The threshold is not failing on the tree; it is failing on the sky and the grass.
That is a much sharper statement of the defect than "the cap is too low", and it means the
fix is the localiser, not a larger cap.

**The localiser agrees across the two renderings.** The implemented localiser — saturated
components, merged by the text they share, with the thresholds calibrated on this pair — finds
**398 carriers on the drawn chart and 388 on the photographic one: 2.6 % apart**, from two
entirely different drawings at different paper sizes, one of them at *lower* native
resolution, with different carrier shapes and a different background.

That agreement is what the thresholds were chosen for, and it is checked by a test
(`TestNameCarrierLocaliserAgreesAcrossTwoRenderingsOfTheSameFacts`) rather than asserted here.
Reaching it took three corrections that the measurements forced, and each is recorded in the
implementation:

- **The photographic background is what breaks the ink-run detector**, not the chart. Same
  detector, same cap, same facts: 2,501 candidate segments on the drawn chart and 9,345 on the
  photographic one. The threshold was reading the sky and the grass.
- **The merge must be anchored on the text and bounded by proximity.** Fragments of one leaf
  share a word; two different leaves never do. Without the proximity bound, one long stroke
  crossing a dense canopy merges every leaf it touches, and the merged group is then too large
  to be a name and is discarded — destroying the very carriers the merge was meant to repair.
- **The obvious alternative to pixel association is worse.** "The word lies within the shape's
  box" seemed more robust, and it is: on the photographic rendering it cut the carrier count
  from 386 to 172. Pixel evidence is both stronger and safer, because the anti-aliased edge of
  a letter is dark and saturated at once.

It is also worth stating what the pair does *not* show. Two renderings of one source are not
two independent sources, and a reader that misses the same name in both would be invisible to
this check. It bounds the error; it does not eliminate it.

**Carrier precision is much better on the drawn chart** (97.9 % against 81.9 %), because on
that chart the leaves are the only saturated things on the page, while on the photographic one
some saturated blobs are fruit that carry no name and some are background.

**What this does not yet measure.** These are *carriers*, not *names*. The localiser finds
where the names are; whether a reader gets them right is the paired check on the name sets,
which needs the reader from plan 019 and is the next number available.

### 1.7 Storing these correctly

These originals are irreplaceable family records, not inputs that can be re-fetched. The
corpus therefore stores them outside the repository, **read-only**, with a checksum and a
provenance record:

```
~/.dawha/corpus/
  sources/       the originals, chmod 444, sha256 in sources.json
  pages/         the renders the harness reads
  manifest.json  the harness manifest
  sources.json   provenance: sha256, page geometry, scan properties, permission, notes
```

`sources.json` records, per original: the checksum, the family, the title, where it came from,
what permission was given, the page geometry, the scan properties, and what the document
classifies as. It also records the `renderings` grouping, so the paired check knows that two
documents are two drawings of the same facts.

Two properties follow, and they are worth stating because the assets are irreplaceable:

- **A backup can be verified.** The checksum list means a restored copy can be proved
  identical to the original, without trusting the backup.
- **The corpus is outside the working tree**, so a path mistake cannot put a family document
  into a commit. Plan 018 adds a guard that fails if anything under the corpus path is
  tracked by git — a `.gitignore` entry is not a guard, and the plan 012 precedent is a corpus
  that leaked into `db/seeds/` and shipped.

The product side already does the right thing with an uploaded original: `source_files` keeps
the checksum and the byte size, the object is never deleted after a successful run, and the
worker re-verifies the checksum against the bytes it re-reads
(`internal/sourceprocessing/processor.go:153`). What the product does **not** have is a backup
story for the object store, and that is a separate concern from this plan.

### 1.8 Four more charts, three new families — and the corpus is not one class

Four more family charts arrived on 2026-10-22 (البليهي، الفراج، السكران، آل مقبل), plus a
356-page genealogical reference book ("القبائل العربية في السعودية"). Measured with the same
chain — 300 DPI, `-scale-to 4000`, `DetectNameCarriers`, count `name_group` regions:

| document | native | name groups | reader probe (8 largest crops, hosted) |
|---|---|---|---|
| jasser-1413 (drawn leaf) | 202 ppi | 474 | — (previous round) |
| jasser-1432 (painted fruit) | 236 ppi | 493 | — (previous round) |
| albalihi (printed poster) | 200 ppi | 123 | البليهي، محمد، عبدالله، إبراهيم بن صالح… |
| alfaraj (calligraphic) | 300 ppi | 531 | إبراهيم، فراج، عزام، سلمان، عبدالله |
| alsuqran (desert tree + table) | **96 ppi** | 787 | محمد، مسعود، علي، بلال |
| almuqbil (unclassified) | 300 ppi | 248 | وضحى، عبدالعزيز، عبدالله |
| tribes-book (reference book) | text layer | — | — (not a chart) |

Inspection of the renders settled the decisive question: **the corpus is not one document
class; it is at least five.**

1. **Painted fruit tree** (jasser-1432) — names on coloured fruit of a photographed painted tree.
2. **Drawn leaf tree** (jasser-1413) — names in drawn leaf shapes on paper, prose panels beside them.
3. **Printed oval canopy** (albalihi) — a 22×39-inch poster: names in *uniform green ovals* on a
   very dense canopy, a colour legend, prose panels, and two facsimiles of handwritten
   manuscripts embedded as images.
4. **Calligraphic tree** (alfaraj) — names in leaf and oval shapes, **and** branch names written
   as rotated calligraphy *along* the brown branches themselves.
5. **Desert tree with a table** (alsuqran) — leaves *and* rotated calligraphy on branches *and* a
   structured tabular list of names, on one page, at 96 ppi.

Three conclusions follow, and they are load-bearing:

**The reader generalises; the localiser does not.** Reading an isolated crop is the same task in
every class — real names came back from all five, from 96 to 300 ppi, three new families. The
localiser is a different story: it found only 123 groups in البليهي, far below the hundreds of
names visible, because **a uniform-colour canopy defeats the saturated-component carrier** —
adjacent ovals of the same green merge into one blob, and no per-carrier grouping can recover
them. Its colour assumptions hold on two of the five classes.

**Names are not always on shapes.** In الفراج and السكران an unknown share of the names is
calligraphy written along the branches, rotated with them. A carrier-driven localiser will never
propose those regions; only unconditioned ink grouping can. The reader reads them fine once
isolated — the probe proved it — which means the gap is a *proposal* gap, not a *reading* gap.

**Mixed content is the norm, not the exception.** The reader returned prose lines
("عنوان المجد في تاريخ نجد") alongside names from البليهي's panels, and both charts carry prose
panels, legends and manuscript facsimiles that are correct data despite not being names. The
import must separate name crops from prose crops rather than treating every crop as a name.

The architecture conclusion is unchanged but sharpened: **propose imperfectly, read well, and
let a person decide.** The review queue (`source_tree_suggestions`, plan 021) is exactly the
surface that absorbs an imperfect localiser, because a missed region costs a suggestion row, not
a missed person — the reviewer's own knowledge fills the gap. The localiser now runs its ink
grouping whether or not the page has coloured shapes: the name groups were always computed from
ink, and the only colour dependency was an early return on pages with no carriers (§1.9). What
remains open is not colour but content — separating a name from a prose line on a dense sheet.

Two concrete product blockers surfaced by the same batch, recorded here so they are not lost:

- **The upload limit refused a real chart — resolved.** آل مقبل is 58 MB and `MaxUploadBytes`
  was 25 MiB (`internal/sourceprocessing/types.go`). On 2026-10-22 it was raised to **128 MiB**:
  the corpus's largest flattened chart is 61,079,149 bytes at 300 ppi, smaller than the
  plausible 300 ppi rendering of the largest poster already in hand, and 61 MiB of live bytes
  during processing stays within the lease queue's budget
  (`TestValidateUploadInputAcceptsARealFamilyChart` pins the exact real size).
- **The book track has a text layer, which changes its problem.** The book's OCR layer works,
  its entries are already structured (a subject followed by numbered kinship lists —
  "ذوي عنان، ذوي محمد…"), and `identity.NormalizeArabicName` (NFKC) normalised a real page's
  51 presentation forms to zero while preserving that structure. For this book the track is a
  **structured-extraction problem, not an OCR problem** — §8's reader-ranked-last conclusion
  gets stronger, not weaker.

### 1.9 A website, a vector chart, and the first name-level check (2026-10-22)

Two more sources arrived, both in formats the pipeline could not open:

- **The "website" is one image.** `https://www.obaikan.net/ALALSHEEKH.htm` is a 393-byte
  FrontPage page whose whole body is `<img src="images/ALALSHEEKH.gif">` — a 1,962×2,919 GIF of
  the آل الشيخ tree. There is no HTML structure to extract: a crawler gets one `<img>` and no
  names. So "support a website" reduces to "support a raster image", which is the same input as
  a photograph, a screenshot, or an exported PNG.
- **A 2018 Illustrator chart with no text layer.** The آل الصباح chart
  (`شجرة آل الصباح حكام الكويت.pdf`) is one vector page, 4,251×2,977 pt: zero fonts, zero
  embedded raster images, no text layer. `pdftotext` returns a single byte. Rendered at
  300 DPI (`-scale-to 4000`, 4,000×2,801), the localiser proposes **2,259** name groups; the
  آل الشيخ GIF proposes **320**. Both numbers are counts, not accuracy.

**Raster images are now documents.** The source contract accepts `image/jpeg`, `image/png` and
`image/gif` up to 128 MiB and 40 megapixels of source; a raster has exactly one page, its pixel
size is the page size. Tesseract and the line detector run on the same bounded 4,000-px render
a PDF page gets; the name-reading path now uses source-resolution tiles (§1.10). Previews are
bounded at 2,400 px, and the original
is stored unchanged and checksummed. The web UI, page review, suggestions and tree drafts accept
images on the same surfaces as PDFs. GIF is included precisely because the published tree above
is a GIF; a single-frame decode is the page.

**The colour dependence was one early return.** The name groups were already built from *all*
dark-ink blobs — the coloured shapes only say which shape a name sits on. But `DetectNameCarriers`
returned `nil` when it found no carriers, so a genuinely black-and-white page lost its names
even though the ink pass would have found them. That return is gone; the reading's model string
is now `colour-text-anchor-v1+ink-name-groups-v1`. A synthetic black-ink-on-white page with no
saturated pixel yields its name groups, and a test pins that. The six older charts' counts are
unchanged (474/493 on the Jasser pair), because their pages have carriers and the ink pass was
already running.

**The first name-level paired check found a limit, not a disagreement.** Plan 018's free check
was implemented as `CompareCarrierTexts`: normalise each reading, pair exact names first and
then trigram variants with spaces removed (`عبدالله` / `عبد الله`), one pairing per occurrence,
and report a Dice agreement `2·matched / (left + right)` with the unpaired names on each side.
`cmd/ocr-eval --compare` runs it over a predictions file and prints the unmatched samples.

Run on the two Jasser renderings with 40 evenly spaced groups each, it paired **zero** names:
78 of 80 readings came back `unreadable`, and the remaining two were fragments (`عبالل`,
`بدالم`). Inspecting the crops explains it — the sampled groups are letter fragments (median
23×12 px on a 4,000-px page), not names. The 474/493 count agreement is therefore real but
shallow: the localiser and the reader are looking at the same chart at different granularities.
For this chart the next move is **grouping** (merge the fragments of one name) and a
name-sized crop, not another colour rule. The comparison itself is correct and unit-tested; it
will produce the accuracy number as soon as the proposals are name-sized. That is the honest
state of plan 018's free check, recorded rather than replaced by a count.

The same content problem shows on the new sources: a 12-crop probe on آل الصباح read a Quran
verse from the prose panel and fragments from the dense rows, and the آل الشيخ probe read the
credit line and prose panels. The reader is doing its job; the proposals include text that is
not a name. Separating names from prose — on the page, before reading — is the next localisation
measurement.

The three charts supplied as chat images on the same day — العجلان والعيب، أسرة الحيان، and
عبد الرحمن سعد الشرمي — are now in the corpus as PNG documents, and they sharpen the point:

| source | pixels | style | name groups | 12-crop reader probe |
|---|---|---|---|---|
| alajlan | 704×1173 | printed leaf canopy | 338 | 0 readable |
| alhayyan | 1,498×2,048 | desert tree, branch plaques + trunk panel | 471 | 1 fragment (`لا`) |
| alsharmi | 768×1,138 | painted fruit tree | 94 | 0 readable |
| alsabah (PDF) | 4,000×2,801 | vector boxed chart | 2,259 | 1 Quran verse + fragments |
| alalsheikh (GIF) | 1,962×2,919 | calligraphic tree | 320 | credit / prose / partial names |

**Accepting an image is not reading it.** The three PNGs are screenshot-resolution copies: in
the 704-pixel-wide العجلان chart a leaf cell is four to six pixels across. The pipeline accepts
them, stores them read-only, shows them to a reviewer, and writes page evidence — all correct
behaviour for evidence — but no OCR can recover names from that copy. The original or a
higher-resolution image is required, and the product should say so at the point where a page's
proposal sizes are visible rather than pretend the reading failed. The direct signal is already
in the evidence: the height of the name groups in pixels. A page whose median proposal is a
dozen pixels tall cannot be read by the current reader, and this is a property of the source,
not of the document class. The honest next step is to surface that page-level warning beside the
suggestions and ask for a better copy, not to lower the bar for what counts as a name.

### 1.10 Reading at the source's resolution, and the limit that moved (2026-10-22)

The 4,000-px page render is a **locating** size, not a **reading** size. On the آل الصباح
chart — 59 inches wide — it is 68 DPI. At 240 DPI the same page is 14,174×9,922 and a name is
about 70×36 px instead of 18×7 px. The page is vector art, so the detail was never lost in the
file; the preview threw it away.

The reading path now renders the page in 1,920-px tiles with 256-px overlap, runs the
localiser on each tile with the page-calibrated thresholds expressed in tile pixels, and
assigns each name to exactly one tile by its centre. The overlap gives a name centred in a
tile enough surrounding source to be complete, and the centre rule guarantees it is read once.
The آل الصباح page is 54 tiles and yields **3,014** name groups (against 2,259 at the old
4,000-px preview), with a median name height of **36 px**; the crops are complete names a
person reads at a glance (`أحمد`, `شيخة`, `عائشة`, `صالح`, `مريم`). The suggestions record
`colour-text-anchor-v1+ink-name-groups-v1+source-tiles-v1`.

The upload bound and the reading bound had disagreed: a 40-megapixel image was accepted and
stored, then refused by the chart-page opener at 16 megapixels, which looked like a chart with
no names. The opener now uses the source bound, and
`TestOpenChartPageAcceptsAnImageAboveThePreviewBound` pins a 16.8-megapixel image.

**The limit is now the reader, not the localiser.** On the آل الصباح chart, a clean boxed name
crop that a person reads as `أحمد` was read by DeepSeek V4.1 Flash as `معصرة` at 240 DPI,
`معمول` at 480, `معصم` at 600 and `معصرة` at 900; the full box at 1,200 DPI came back `حماة`.
A synthetic `محمد` in Geeza Pro was read correctly, so the model can read Arabic; it cannot
read this chart's calligraphic letterforms. Tesseract is not a substitute: it read the same box
correctly from one 8× crop, and returned nothing or `معمل` from others. Plan 019's STOP
condition anticipated this outcome — the class requires manual transcription or a better
original, and the honest product is a crisp crop in front of a person, not a claim of accuracy.

The painted and leaf charts still produce letter or leaf fragments. On آل مقبل at 240 DPI the
page-relative gap is 6.6 px and the localiser returns 260 fragments; raising the gap to 20–30 px
collapses branches and leaves instead of forming names. That is a different measurement — names
against leaf veins and prose — and not a parameter to turn until the count looks better. The
three screenshot images remain source-resolution failures (§1.9).

Two questions, two answers:

| question | answer on 2026-10-22 |
|---|---|
| can the localiser find a name at source resolution? | yes on a boxed chart: 54 tiles, name-sized crops |
| can the current reader transcribe this font? | no; the crops are right and the reading is the reviewer's |

The crops being right does not make the run cheap. The آل الصباح chart proposes 3,014 names,
and three views each is thousands of vision calls behind one synchronous request. The next
product step is a bounded, resumable read — a job with a cursor — not a longer timeout.

The run now reports the **median name height in source pixels** and warns below 18 px, with a
plain Arabic message asking for a higher-resolution original. The three chat images measure
**3 px** (العجلان والعيب, 338 groups), **11 px** (أسرة الحيان, 471) and **5 px** (الشرمي, 94),
so the warning fires on all three; آل الصباح measures 36 px and does not. That is the signal
the screenshots needed: they were accepted and stored, but their names are too few pixels tall
for any reader to have a fair chance. It is a warning and not a gate — the regions remain
reviewable, and a person may still read a small name better than the model.

## 2. The v8 experiment, and why it failed

The v8 run asked the model to **find names across a dense photographic sheet**: 139 provider
calls, 39 name regions against 500-plus visible names, 21 connector paths, no branch proposals
(`docs/ocr-and-import-decision.md:50-57`).

The measurement explains it. The task was *"describe this sheet and list the names you can
find"*, on a page whose ink structure yields 9,345 competing candidates and whose background is
a photograph. The published gap says exactly this should fail:

| Task | Classical CV + OCR | Best vision-language model | Gap |
| --- | --- | --- | --- |
| Node detection (F1) | **98.8 %** | 34.0 % | 2.9× |
| **Edge detection (F1)** | **66.7 %** | **10.7 %** | **6.2×** |
| Edge precision at that operating point | **85.5 %** | — | — |
| Node detection (F1), LoRA-tuned VLM | — | 74.9 % | — |
| Edge detection (F1), LoRA-tuned VLM | — | 46.1 % | — |

FlowExtract (APMS 2026) on real ISO-5807 flowchart PDFs; diagram2graph with Qwen2.5-VL-3B +
LoRA. The diagram literature agrees: frontier VLMs "struggle with arrow-following and
next-step reasoning."

**But the fix is not "a better vision model". The fix is to change what the model is asked.**

The v8 run's mistake is architectural, not parametric: it asked the model to *localise* on a
hard sheet. The task vision models are actually good at is **transcribing one isolated,
high-contrast, already-cropped word** — which is exactly what §1.4 makes available, at ~400
calls instead of one per sheet, and where the reference numbers (Azure 0.13 CER, Gemini 0.13)
actually apply.

So the corrected pipeline for this class is:

```
render at the reading resolution (300 DPI is enough)
   ↓
colour-segment the name carriers          (deterministic; 398 on the drawn chart,
                                          388 on the photographic one)
   ↓
crop each carrier, mask to the blob
   ↓
transcribe each crop with a vision model   (one isolated word per call)
   ↓
multi-view consensus per crop              (measured confidence, not a model's opinion)
```

Compare with what is built: render → ask the model to find names on the sheet → 39.

---

## 3. The structure is a separate problem, and it is the hard one

Nothing above recovers a *relationship*. The chart encodes descent in **painted branches**, and
the decision document already says the right thing at
`docs/ocr-and-import-decision.md:251-254`: "Nearest-neighbor geometry can propose, not
establish, kinship."

For a pictorial tree the question is harder still: a fruit's position and its branch tell you
an *ordering*, not necessarily a parent/child pair. The decision document's own §5 warns that
"a lineage link can mean descendant-of across omitted generations."

Three candidate sources of structure, in descending order of reliability:

1. **The painted branches themselves.** A branch tracer could follow the dark-brown branches
   against the sky and grass. This is a research problem, not a 20-minute job, and it is the
   honest open question in this area.
2. **The chart's own generational banding.** Where a chart orders generations top-to-bottom, the
   ordering is evidence of *generation*, not of a specific parent.
3. **Another source.** The family's own records, the lineage books, or the same chart in a
   box-and-line rendering.

Note the corollary: **the family tree PDF is a good source of *names* and a weak source of
*relationships* for this document class.** The decision document's own pilot design — reviewer
draws the edges, machine proposes the names — is closer to right than the reading pipeline it
is attached to.

---

## 4. What Arabic reading actually costs

The decision document's provider shortlist (`docs/ocr-and-import-decision.md:127-140`) is
careful and correct, and it explicitly declines to select a provider. This section supplies
the numbers behind that caution.

### 4.1 Printed Arabic

From KITAB-Bench (8,809 samples, 9 domains, 36 sub-domains), character error rate for Arabic
OCR:

| System | CER | Notes |
| --- | --- | --- |
| Gemini 2.0 Flash | **0.13** | best closed model |
| Azure Document Intelligence / Vision | **0.13** | best on diacritics and text elongation |
| Qaari (open) | 0.20 | |
| GPT-4o | ~0.31 | best at font handling |
| Qwen2.5-VL-7B | 1.20 | |
| Tesseract 5 | **0.54** | the local incumbent |
| EasyOCR | 0.58 | |
| PaddleOCR | 0.79 | |
| Surya | 4.95 | unusable on Arabic |

End-to-end document-to-markdown accuracy is lower than the OCR numbers: Gemini-2.0 Flash
reaches 65.65 % MARS and GPT-4o 65.12 %, while Qwen2-VL-7B collapses to 21.42 %.

**No single system wins across Arabic complexities**: Azure leads on diacritics and elongation,
GPT-4o on fonts and tilted text, Tesseract on clean single-column print. And **configuration
swings the score more than model choice does** — in one benchmark, changing image resolution
and output token limits moved a single model by 14 points.

§1.5 is the concrete counter-example to "Tesseract is fine for clean print": the Jasser names
are clean, large, high-contrast and still unreadable, because of the font.

### 4.2 The ranking implication

The gap between Tesseract at 0.54 and the best hosted models at 0.13 is real and fourfold. It
is **not** a reason to select a hosted provider yet, for three reasons that are all about this
corpus rather than the benchmark:

1. **The benchmark corpus is not this corpus.** Every number above is from public Arabic
   document collections that contain no family charts. Plan 018 measures the local baseline on
   the real corpus, against adjudicated gold.
2. **The chart is not running prose.** A chart is isolated words in large display faces. §1.5
   shows the failure mode is the *font*, and the published numbers say nothing about it.
3. **A provider decision is a privacy decision.**
   `docs/ocr-and-import-decision.md:513-515` requires that provider, region, retention
   behaviour and permitted data use be recorded before a route is enabled, and that local
   failures not silently trigger a cloud fallback. Adding a hosted provider is a
   data-processing decision about documents that may concern living people, and it is not the
   decision that plan 018 forces.

**Conclusion.** The hosted providers are now in scope, not merely challengers. The privacy
decision recorded in `docs/ocr-and-import-decision.md` §11 permits hosted processing for these
documents, and §1.5 shows the local reader cannot handle this font class at all. So the
comparison is not "local versus cloud" — it is **"which hosted reader, and does it beat the
local baseline where the local baseline works"** — and plan 018 measures that directly.

### 4.2.1 The privacy decision, and what it changes

**Decision, 2026-10-21: hosted processing is permitted.** The product owner has determined that
these family documents carry no high-privacy or sensitive content, and that documents may be
sent to hosted OCR and vision providers. Consequences, and they are substantial:

- **The hosted-vision adapter stops being an experiment.** The pilot currently pins one file
  by SHA-256 and treats every other document as ineligible
  (`internal/sourceprocessing/chart_analysis.go:29`). That restriction can be lifted to a
  policy rather than a hard-coded digest.
- **Plan 019's per-crop transcription arm gets a real reader**, rather than an interface with
  nothing behind it. That is the difference between a plan and a working feature.
- **The provider is selected by measurement, not by preference.** With several candidates now
  permissible, plan 018's comparison becomes the selection mechanism.
- **One caveat, stated once.** A 1432 AH chart contains grandchildren who are alive today.
  `PRODUCT.md:1402` and §43 require living-person rules for *published* trees, which is a
  separate decision from the reading path — but the reading path now handles documents about
  living people, and that is worth one line in the privacy record rather than an assumption.

Two rules from `docs/ocr-and-import-decision.md:513-515` survive the decision and are not
optional: a provider, region, retention behaviour and permitted-data-use must be **recorded
before the route is enabled**, and a local failure must **not** silently fall through to a
cloud route.

### 4.3 Books are a different reading problem, and it is not solved by a general model

For historical manuscripts the picture collapses. On the Muharaf corpus (1,334 line images),
Tesseract achieves 69.2 % CER and the best vision model 70.4 % — statistically
indistinguishable — and the best OCR-conditioned correction reaches 60.6 % CER. That study's
central finding is the **OCR-prior recoverability principle**: giving a vision model the OCR
text alongside the image helps only when the OCR output remains visually and textually
recoverable, and actively *hurts* when the prior is script-mismatched.

For **printed** books the ceiling is much higher but only with supervision. The OpenITI and
JSTOR study of Arabic periodicals found that all evaluated commercial Arabic OCR systems
performed below 75 % accuracy, making manual correction "prohibitive" — and then showed that
**Kraken**, a segmentationless sequence-to-sequence engine, reached **97.41 % character
accuracy** and 98.46 % Arabic-script-only accuracy on the same material, with only
**800–1,000 transcribed lines** of training data for that document's typeface.

The ceiling for Arabic printed text is very high, and what unlocks it is **typeface-specific
supervision**, not a bigger general model. §1.5 is the same lesson from the other direction:
the font is the whole problem.

---

## 4.1 The end-to-end path, proven on the one chart we have

The pipeline is implemented and it works. The chain, measured on the Jasser chart:

```
PDF (236 ppi JPEG scan, no text layer, no vector content)
  ↓  render at 300 DPI, the resolution the pipeline already uses
saturated components          411 carriers — the coloured shapes
  ↓  group the ink by proximity, and keep only what is name-sized
name groups                  493 on the photographic chart, 474 on the drawn one — 4 % apart
  ↓  crop each one, read it in isolation, three times, and vote
names                        read by the hosted reader
```

**493 and 474 are in the right place.** The decision document says the chart carries
"500-plus visible names" (`docs/ocr-and-import-decision.md:55`), and the localiser finds 474 of
them on the clean drawing and 493 on the photograph. That is the first independent check of that
figure, and it was made without an answer key.

**The reader works.** Six crops, read with the existing DeepSeek adapter:

| Crop | Status | Read as |
| --- | --- | --- |
| 1 | partial | `عبدالله` |
| 2 | readable | `سليمان` |
| 3 | readable | `عبدالله بن محمد` |
| 4 | partial | `تركي` |
| 5 | partial | `احمد` |
| 6 | partial | `محمد بن حاسر بن عبدالكريم بن محمد بن ابراهيم الجاسر` |

The first one was checked against the crop by eye and it is exactly right. This is the first
time in this area that a name has been read out of a family-tree PDF end to end, and it took
the whole decomposition to get there: the ink-run detector found the landscape, the v8
experiment found 39 names out of several hundred, and this path finds them.

**Three things the measurement corrected, each of which was a design decision that looked
right and was not:**

- **The unit is the NAME, not the shape.** A carrier on a painted tree is often a *dozen
  leaves*: they touch, and touching shapes are one component. Reading a carrier hands the
  reader a branch of the tree. The name is a run of ink, and grouping the ink by proximity
  gives the right unit: 352 name groups on the drawn chart and 408 on the photographic one,
  with a median group covering 0.001 % of the page.
- **The context crop must NOT be wider than the name.** A 3× context crop of a painted tree
  contains the *next* name as well, so the reader was asked for one occurrence, saw two names,
  and returned two readings for it — which the response schema correctly refuses. The tight
  crop of one isolated name is read accurately, and the robustness comes from reading it
  several times and voting, not from showing the reader more of the page.
- **A text block is not a name.** The ancestor stack down the trunk and the panels of prose
  come out as one enormous "name group". An area cap excludes them: after it, the largest
  group on either chart covers 0.42 % of the page, and the handful that are excluded are
  reported rather than silently dropped.
- **A fragment of a letter is not a name either.** The first grouping produced 1,900 groups
  on the photographic chart, most of them single strokes of JPEG-shattered letters, and the
  reader returned nothing for every one of them. A minimum height and a minimum area took it
  to 493, and the readings that came back were names.

**What the reader got**, from ten of the larger groups on the photographic chart: `لبيب`,
`الطفل`, `نور الحق`, `فيصل`, `حاكم`, `طارق`, `عبدالله`, `إبراهيم` — and one that was checked
against the crop by eye and was exactly right. The ten smallest groups were unreadable, which
is a *recall* question about the smallest names on the chart rather than a defect in the path.

**What is still not measured.** These are ten crops, read by one provider, with no answer key.
The numbers that matter — how often the reader is right without correction, and how often the
two renderings agree on the *names* rather than on the *carriers* — need the corpus and the
gold, which is plan 018. The command that produces the arm is `cmd/ocr-eval`.

## 5. Confidence is the hard part, and the model cannot supply it

- **Verbalized self-confidence** separates correct from incorrect extractions at AUROC
  **0.54–0.74** — at times worse than chance (ConfBench; Perception/Layout/Validation).
- **Mean token log-probability** reaches 0.705 and **self-consistency** over five samples
  0.744, but all three "collapse toward all-positive behaviour at any practical threshold":
  at threshold 0.5 they retain recall 1.000, meaning they flag everything or nothing
  (ExtractConf).
- **What works is disagreement-based confidence** — cross-view and cross-model disagreement
  combined with OCR and image-quality signals — reaching **0.928** AUROC.
- **VLMs fail by fabrication, pipeline OCR by omission**, and capped CER/WER hides the
  difference: uncapped-to-capped inflation reached 23× on full suites and 107.6× on degraded
  sets (Kim et al. 2026). On a chart, a fabricated name is indistinguishable from a real one.

This repository has already made the right call on the substance: `PageReading.Confidence` is
documented at `internal/sourceprocessing/types.go:260-262` as "the provider's raw score,
retained for comparison rather than treated as a calibrated probability," and the UI says so
in Arabic. The gap is that **there is no calibrated score to compare it against**.

**Multi-view consensus** is how to get one. Generate *k* augmented variants of the same crop
(padding, blur, grid-warp), transcribe each, align them with a Needleman–Wunsch-style aligner,
take a per-column vote; the confidence is the vote fraction — a physical quantity, not a
model's opinion. Measured on 622 historical death records: **+4 percentage points** of field
accuracy over a single pass, recovering 4–7 of the 7 name fields the single-pass baseline
missed entirely, with grid-warp giving the strongest confidence–accuracy correlation
(0.642).

Two rules it must not break:

- **Route, do not blanket-apply.** OCR-conditioned correction helps only where the OCR prior is
  recoverable, and hurts where the prior is script-mismatched. Enable the second reader per
  region, not per document.
- **Bands, not bare numbers.** The corporate family-tree extraction work — the closest prior
  art — concluded that numerical confidence values "can appear more precise than they truly
  are," and recommended a **band-based** scheme. The repository already has the vocabulary:
  `entity_resolution_candidates` stores `match_class` beside its numeric `score`
  (`db/migrations/0015_entity_resolution.sql:43`), and its queue index orders by
  `(review_status, match_class, score DESC, created_at DESC)` (`:68`).
- **The model can never pre-approve.** `candidateStatus()`
  (`internal/sourceprocessing/processor.go:1085`) forces every candidate status to
  `needs_review` regardless of what the model returned, and `internal/ai/client.go` rejects any
  AI response whose `review_required` is not true. A high-confidence extraction is a candidate
  that sorts first in a review queue, not a fact.

---

## 6. What the import writes today, and the four gaps plan 020 closed

An earlier draft of this analysis asserted that the platform "cannot bootstrap a person from a
document," on the evidence that `internal/sourceprocessing/review.go:122-177` materialises only
a `source_statement` for an entity candidate. **That claim was true of the review path and is
no longer true of the platform.** The tree-draft import has its own materialisation. Auditing
it gives a much sharper result.

### 6.1 What the tree-draft import writes

| Record | Where | Assessment |
| --- | --- | --- |
| `trees` (private) + `tree_versions` (draft) | `tree_drafts.go` | correct |
| `people` — one **new** row per unmatched node, `gender='unknown'`; a matching node is **linked**, no new row | `tree_import_link.go` | **gap 1 — closed** |
| `tree_nodes` binding person to the draft version | `tree_drafts.go` | correct |
| `tree_relationships`, `status='unresolved'`, `source_id` set | `tree_import_link.go` | correct |
| `source_tree_draft_imports` manifest (now also records the graph/alias ids) | `tree_drafts.go` | correct |
| `source_tree_import_node_evidence` — page evidence, label, region | `tree_drafts.go` | correct |
| `source_tree_import_edge_evidence` — page evidence, connector path | `tree_drafts.go` | correct |
| retraction ledger and `RetractTreeDraft` — now also reverses the graph rows, statements and aliases | `tree_drafts.go` | correct: reversible without a restore |
| `source_statements` (`extraction_method='imported'`) | `tree_import_link.go` | **gap 2 — closed** |
| `person_aliases` (`source_spelling`, verbatim; `kunyah` when identified) | `tree_import_link.go` | **gap 3 — closed** |
| `entity_relationships` (`status='unresolved'`) | `tree_import_link.go` | **gap 2 — closed** |
| `entity_resolution_candidates` (bounded, scoped to the import's run) | `tree_import_link.go` | **gap 1 — closed** |
| dry run with a binding report | `tree_import_link.go` + `/import-preview` | **gap 4 — closed** |

The pilot closed the materialisation, provenance and reversibility requirements. The four
identity, graph, vocabulary and dry-run gaps were closed on 2026-10-22 by plan 020; §6.6
records how each was closed and why each shape is the one it is.

### 6.2 Gap 1 — imported people are never matched against existing people

Every included node becomes a **new** `people` row (`tree_drafts.go:487`). Nothing compares it
to the rest of the database. `entity_resolution_candidates` is written in exactly one place —
`internal/entityresolution/service.go:274` — from a run that sweeps an entire entity type,
because `RunInput` carries only `EntityType` and no scope
(`internal/entityresolution/types.go:76-78`). The import never proposes a candidate.

A family tree naming `محمد بن عبدالله`, who already exists because a cousin's tree was
imported last month, therefore creates a **second** person with the same normalised name and
no link to the first. Nothing warns anyone. The duplicate becomes visible only if somebody
later runs a full resolution sweep — by which time the imported person may already be a node
in a published tree.

This is the failure `docs/ocr-and-import-decision.md:66` names, and the pilot is on the wrong
side of it. The correct shape is the one the decision document names at `:64`: the import
produces **candidates for the existing entity-resolution surface**, where a human confirms a
merge explicitly and can reverse it.

### 6.3 Gap 2 — imported relationships never enter the research graph

An imported relationship is a `tree_relationships` row with `status='unresolved'` and a
`source_id` (`tree_drafts.go:513`). It lives in the tree version and nowhere else. It is not an
`entity_relationships` row, not a `claims` row, not a `claim_evidence` row, and there is no
`source_statements` row for it.

**The import does not operate the platform.** It produces a private draft tree a researcher can
look at. It adds nothing to the evidence-aware lineage graph that the research agent reads,
that hybrid retrieval cites, or that the contradiction detector compares. For a platform whose
thesis is that family trees and lineage books will supply the data that operates it, this is
the gap that decides whether the feature is a viewer or a source.

The schema has had the slot since it was written: `source_statements.extraction_method` accepts
`'imported'` (`db/migrations/0003_research_and_jobs.sql:60`), and no code writes it —
`book_assertions.go:411` writes `'manual'`, `processor.go:876` and `review.go:141` write `'ai'`.

### 6.4 Gap 3 — the raw reading is destroyed and the classical name vocabulary is unused

`person_aliases` is written by `internal/identity/people.go:353` and
`internal/suggestions/service.go:912`, but **not** by the import. So:

- The **OCR'd spelling** is destroyed by normalisation at `tree_drafts.go:487`, which stores
  only `identity.NormalizeArabicName(node.NameAR)`. That function
  (`internal/identity/normalization.go:10`) strips the tatweel (`U+0640`), folds `أإآٱ→ا`,
  `ى→ي` and `ة→ه`. Right for matching, wrong for evidence: a later correction overwrites the
  reading, and a reviewer cannot see what was on the page. `alias_type='source_spelling'`
  exists precisely for this, and it is source-scoped
  (`internal/identity/validation.go:19-22`).
- The **classical Arabic name components** — *ism*, *kunyah*, *laqab*, *khitab*, *nisbah*,
  patronymic — have no home in the import, even though `person_aliases.alias_type` already
  accepts `kunyah`, `laqab`, `nisbah` and `source_spelling`. This is the vocabulary the
  manuscript-cataloguing literature uses, and it is the vocabulary §7 needs.

### 6.5 Gap 4 — there is no dry run

`ImportTreeDraft` (`tree_drafts.go:409`) writes the tree, the people, the nodes, the
relationships, the manifest and the evidence rows in one transaction. There is no
parse-validate-report phase. The requirement is explicit at
`docs/ocr-and-import-decision.md:396-398`, and `TreeDraftImportResult` already has the shape
for it (`ImportedPeople`, `ImportedRelations`, `StagedRelations`, `Warnings` —
`tree_drafts.go:497`, `:503-504`).

The collision count is the number a researcher most needs and currently cannot get, because
§6.2 says the import does not know.

### 6.6 Resolved: the four gaps in plan 020 (2026-10-22)

All four gaps above are **closed** by `internal/sourceprocessing/tree_import_link.go`, and
each closure keeps the audit's own rule: propose imperfectly, read well, let a person decide.

- **Linking, not duplicating (gaps 1).** `buildTreeImportPlan` runs the same vocabulary the
  extraction path uses — exact equality on the normalised name against `people` and
  `person_aliases`, at the same 0.5 threshold (`TreeImportMatchThreshold`). An occurrence that
  matches an existing person is **linked to that person**: no second `people` row is created,
  and the occurrence appears in the dry run as a collision with the existing person, the
  basis (canonical or alias) and the score. `TreeImportMatchThreshold` is the same threshold
  `resolveEntity` applies, so one decision uses one threshold on both paths.
- **Candidates, never merges (gap 1 continued).** An occurrence with no exact match becomes a
  new person, and if a bounded trigram lookup puts existing people in range
  (`similarity ≥ 0.45`, at most 5 per occurrence and 20 per import), the import writes an
  `entity_resolution_candidates` row — same shape the sweep writes, listed and acted on by the
  existing review surface — inside the import's own `entity_resolution_runs` run
  (`algorithm_version = 'tree-import-link-v1'`). The import writes no `entity_merges` row, no
  `merged_into_id`, and touches no `identity_status`.
- **The graph rows and the statements (gap 2).** Every reviewed, representable edge reaches
  the research graph in the same transaction as the tree: an `entity_relationships` row
  (`status='unresolved'`) and a `source_statements` row with
  `extraction_method='imported'` — the slot migration 0003 reserved — and
  `review_status='accepted'`, because a reviewer approved the connector. No `claims` row is
  created at import time; a claim waits for the human resolution of the fuzzy candidates. The
  statement's `locator_ar` names the draft edge, which the manifest binds to
  `source_tree_import_edge_evidence`, so the chain statement → draft edge → page evidence is
  walkable.
- **The reading survives, and the kunyah is captured (gap 3).** Every occurrence writes a
  `person_aliases` row with `alias_type='source_spelling'`, the **verbatim** string the
  reading produced, `normalized_value_ar = identity.NormalizeArabicName` of it, and the
  `source_id` it came from. A bounded helper records a `kunyah` alias when the reading opens
  with `أبو`/`أم` followed by a name it owns, including the عبد compound (`أبو عبد الله`); the
  split never rewrites the canonical name, so it stays reviewable and correctable through the
  alias rows.
- **A dry run that binds (gap 4).** `PreviewTreeDraftImport` runs the same validations and
  the same plan builder as the apply and writes none of it (`POST
  /api/v1/source-tree-drafts/{draftID}/import-preview`). `ImportTreeDraft` refuses to run
  unless the client carries the report back (`SourceFileChecksum` plus the six counts) and
  refuses when any of them no longer matches — a dry run the apply may ignore is not a dry
  run. Between the two, the draft revision is still pinned.
- **Retraction covers the graph.** `RetractTreeDraft` now reads the import manifest and
  deletes the source-scoped aliases, the statements and the entity relationship rows it
  wrote — refusing to touch any row whose `updated_at` moved past the import (a researcher's
  later change survives and is counted in `PreservedGraphRows`), and keeping any statement a
  later `claim_evidence` row cites. The report carries
  `retractedStatements/retractedRelationships/retractedAliases/preservedGraphRows`.

The one-use rule is deliberate: two occurrences of one name are two people, not one. A chart
of cousins can hold two عبداللهs, and collapsing them would be a silent merge of real
ancestors; the second occurrence becomes a new person with a warning, and their kinship
stays independent. Tests: `tree_import_link_test.go` (nine acceptances, including the
one-use rule, the binding refusal, the fuzzy candidate shape and the retraction's
keep-later-work rule).

---

## 7. Books: the database is the constraint set

The decision document's §6 is a good workflow for books and its §12 sequence is sound. One
architectural argument it does not make, and which changes the ordering of the work, is this:

**A lineage book's value is not its text. It is its nisba dictionary and its naming grammar.**
`Kitāb al-Ansāb` of al-Samʿānī indexes some 4,400 nisbas — the adjectival and relational
suffixes identifying origin, occupation, tribe and locality. The books cross-reference one
another by abbreviation, and the same abbreviation means different things in different works.

That makes the hard part of reading a book **semantic, not optical**. An unpointed reading of
`الصـ...` is hopeless in isolation. It is resolvable when exactly one `canonical_name_ar` in
the platform is within edit distance — and the platform's people, families, tribes and places
*are* that constraint set. The disambiguation resource is the database, and the database is
populated by the tree imports.

Three consequences:

1. **Books come after trees**, not alongside them. A book read before the trees are in has no
   constraint set and no way to disambiguate anything.
2. **The import must write to `person_aliases`** (§6.4), because that is where the nisba, the
   kunyah and the source spelling live.
3. **The OCR for books is a supervised track**, not a vendor selection (§4.3).

---

## 8. The recommendation

0. **Make the review screen the product.** The machine suggests, a person decides, and the
   person's corrections are the measurement. Everything below serves that loop; plan 021 is it.
1. **Measure first, on a real corpus, and include the document *class*, not just the
   digital/scanned split.** The one chart we have is a pictorial tree, and no amount of
   per-file tuning would have revealed that from the PDF metadata. Plan 018 does this, and
   classifies each document before measuring it.
2. **Replace "recover the connectors" with "localise the name carriers, then read each in
   isolation."** For pictorial charts the localiser is colour segmentation (§1.4); for
   box-and-line charts it is vector geometry where available and raster detection otherwise.
   The reader is a vision model for pictorial charts, because Tesseract cannot read the font
   (§1.5). Plan 019 does this, as a **new arm in the existing harness**, measured against the
   existing arm rather than asserted.
3. **Stop investing in hosted vision as a *localiser*, and start investing in it as a
   *reader*.** The 8 % result is a category result, not a prompt result: the v8 run asked the
   model to *find* names across a dense photographic sheet, which is where VLMs are 6× worse
   than CV. The same hosted model is the right tool for *transcribing an isolated crop*, which
   is a different and much winnable job — and with hosted processing now permitted
   (`docs/ocr-and-import-decision.md` §11), that is the production path for this font class,
   because the local reader cannot read it at all (§1.5).
4. **Close the identity gap before the import is used at scale.** Plan 020: match imported
   people against existing ones through the existing entity-resolution surface, add a dry run
   that reports collisions before anything is written, preserve the raw reading as a
   `source_spelling` alias, and write the import into the research graph as `source_statements`
   with `extraction_method='imported'` plus `entity_relationships`.
5. **Give the review queue a measured confidence**, derived from cross-view agreement and
   expressed as a **band** (§5).
6. **Keep everything else in the decision document unchanged.** The dry-run, preview, explicit
   conflict handling, idempotency, reversibility, prohibitions and per-row provenance it
   requires are still required, and the pilot still does not satisfy all of them.

## 9. Two landmines that are not this plan's problem but will become someone's

- **There is no committed PDF fixture.** A search of the repository finds no `.pdf` under any
  test or testdata directory; the only real PDF on the machine is the Jasser chart in
  `.data/source-storage/`, which is private by design and gitignored. So CI cannot exercise
  the PDF extractor, the page-evidence path or the tree-draft path end to end. The corpus in
  plan 018 must stay private, but the repository needs at least one **synthetic** PDF fixture —
  generated by the test, not a real family document — so the extraction path has a regression
  test that does not depend on a private file.
- **`upload_test.go:186` pins the format matrix across two languages.**
  `TestSourceFormatPanelMatchesTheSharedMatrix` requires every accepted type in Go to appear
  in `apps/web/src/components/SourceProcessingPanel.tsx` and vice versa. The working tree has
  already changed that matrix (`upload.go:151` adds `application/pdf`). Whoever next adds
  image formats must change that test and the component in the same commit, and must decide
  whether the format matrix becomes a matrix with a **confidence dimension** rather than a
  yes/no list — which is what `docs/ocr-and-import-decision.md:96-101` asks for.

## 10. The honest risk in all of this

The plans below are gated on **two Arabic-capable annotators and an adjudicator**, and the
decision document already flags that "reviewer availability is unknown." Every accuracy number
in the literature above is from a public corpus that does not contain this corpus, and the
measurement above is one document with no gold annotation.

**If the annotation cannot be staffed, none of this can be qualified, and the pilot remains
unqualified.** That is the true critical path, and it is not an engineering task. It is worth
saying plainly, because the alternative — shipping on the published benchmark numbers and the
Jasser smoke run — would be exactly the kind of claim `docs/ocr-and-import-decision.md` was
written to prevent.

One more thing the measurement changes about the risk. The documents we now know about are
**pictorial**, and §1.5 shows a mainstream OCR engine cannot read their font. That is a
stronger argument for the vision-model route than any benchmark number, and a stronger
argument for the privacy decision being unavoidable rather than optional: if a hosted reader
is required to read the font, then §4.2's "keep it local" conclusion cannot hold for this
document class, and the data-processing decision has to be made explicitly.

## 11. The plans

| Plan | Scope | Gate |
| --- | --- | --- |
| `plans/018-family-tree-import-benchmark.md` | Collect the corpus, produce the adjudicated gold, **classify each document's chart style**, run the local baseline **and the hosted readers**, and report the measured state. | Measured state recorded; the class and path that can be supported are named; nothing is qualified without independent review. |
| `plans/019-family-tree-import-name-localisation.md` | Replace the ink-run line detector with a **name-carrier localiser** — colour segmentation for pictorial charts, native vector geometry for box-and-line digital charts, the raster detector only as a fallback — and add a **per-crop vision transcription arm** with a provider enabled, to the harness. | The new arms are measured against the same gold, on the same documents, as additional pipelines in the same report. |
| `plans/020-family-tree-import-identity-linking.md` | Match imported people against existing ones through the entity-resolution surface, add a dry run that reports collisions, preserve the raw reading and the classical name components, and write the import into the research graph. | No imported person is created without an explicit collision report; nothing enters the research graph without a `source_statements` row carrying `extraction_method='imported'` and a resolvable evidence id. |
| `plans/021-family-tree-import-suggestion-review.md` | Put the machine's suggestions in the **normal** review screen — names with their crops, identity matches as explicit choices, and the reviewer's decisions logged as the measurement. | No suggestion, identity or relationship is accepted without a recorded reviewer; the accuracy number comes from the decision log, not a labelling campaign. |
| `plans/022-name-candidate-adjudication.md` | Make the spelling-candidate pipeline **measurable**: multi-key Arabic matching, an evidence-aware bounded ranking, a decision snapshot, a read-only replay report, and an optional local Liquid judge as a separate opinion. | A judge cannot fix a candidate pool that misses the truth; no model ordering, percentage, or quality claim is shown until the judge beats the deterministic baseline on held-out reviewer decisions. |

`docs/ocr-and-import-decision.md` remains the decision document. This file is the evidence and
the recommendation that sit underneath it.

**Status note, 2026-10-23 — the review bench is the product, and it reads the source
(uncommitted).** The suggestion queue is now a full-screen name workbench rather than the small modal
that showed a page preview and a reading. It has three parts: the source-resolution crop and
page map, the machine's reading with its alternatives, and a keyboard-first Arabic editor. The
parts that matter for an unreadable chart are the first and the third; the reader's text is a
hypothesis displayed beside them, never the authority.

- **The crop is rendered from the original, not the review preview.**
  `GET /api/v1/source-tree-suggestions/{id}/crop` renders a tight crop (20 % padding) and a
  wider context crop (up to five name-widths and four name-heights, with a 560×300 px floor)
  through the same source opener the reader uses. Both are capped at 1,600 px on the long side
  and 2.5 megapixels. A reviewer can zoom, increase contrast, and move through the suggested
  places on the page map; the crop is deliberately not the bounded page preview, which cannot
  settle a 36-px calligraphic name.
- **Reader alternatives are stored, not flattened.** Migration
  `0052_source_tree_suggestion_alternatives.sql` adds `alternatives jsonb`. A reading that
  produced several plausible strings returns them in the same order the reader ranked them,
  and the bench makes each one a one-click correction. The chosen reading and the alternatives
  both survive in the record.
- **The vision model can be asked for candidates on demand, one crop at a time.**
  `POST /api/v1/source-tree-suggestions/{id}/candidates` sends only the tight crop to
  `/v1/vision/chart/candidates`, receives at most eight names, and returns them to the editor.
  Nothing is written to the suggestion, the draft, or the graph. The button is deliberate: one
  paid call per name, no automatic sweep. The response is a spelling aid, not a second reading.
  A single probe on a crop a person reads as `أحمد` returned `محمد`, `أحمد`, `محمود`, `حميد`,
  `حمد`, `معهد`, `معد`, `أمجد`: the expected name is present, but one probe is not an accuracy
  rate and the interface does not present it as one. A second live probe, on a crop a person
  reads as `جاسم`, returned `علي`, `على`, `عبد`, `عيد`, `عبدالله`, `عليا`, `علا`, `عبيد` and did
  **not** include `جاسم`. Two probes are not a rate either; together they are why the candidate
  rail is labelled a hypothesis, sits outside the ranked spelling chips when it does not match,
  and never applies a name by itself.
- **The editor remembers the file, not the world.** Accepted and corrected names from the same
  file become spelling candidates for the next name; a short common-name lexicon and the
  reader's alternatives fill the rest. Normalisation is used only for matching
  (أ/إ/آ→ا، ى→ي، ة→ه، diacritics/tatweel removed). It never rewrites the displayed name, so a
  correction such as `عبدالله` is stored exactly as the reviewer wrote it.
- **The keyboard is the primary input.** `Enter` saves and advances; `a` accepts the machine
  reading; `r` rejects the place; `d` defers it; `j`/`k` move without deciding; `1`–`8` choose
  a candidate; `/` focuses the field. Filters expose names with no reading and low-confidence
  names, the page navigator follows the filtered queue, and the queue itself is bounded to 200
  suggestions per request and extended when it runs low.
- **The measurement is still the decisions.** The header reports the number of decisions and
  the unchanged reading rate beside it. The API calls the sample measurable only at 100
  accepted, corrected, or rejected decisions (deferrals do not carry a reading judgement), and
  the header shows a dash plus «لا نسبة بعد» below that floor rather than a percentage. The
  accuracy sentence is not a claim about the reader's quality on this font; it is what the
  reviewers have recorded so far.

The three chat PNGs measure median name heights of 3, 11 and 5 px. Their crops remain soft at
any zoom or contrast setting; the bench says so rather than pretending a sharpened thumbnail
recovered ink. The آل الصباح chart localises 3,014 proposals at a median height of 36 px, but
the reader still misreads its display face. For that class, the source crop plus the reviewer's
typing is the product, and the machine's reading is the record to compare against.

The crop endpoint is pinned by
`TestASuggestionCropComesFromTheOriginalSource`, including the test that the model receives
exactly the tight crop the reviewer sees (`bytes.Equal` against the crop response). The
candidate route refuses a reviewer without source access, and the AI service returns a
deduplicated, bounded list. The web-side candidate/queue helpers and the Arabic completion
ranking have unit tests. A manual browser pass on 2026-10-23 exercised the workbench at
1440×1000 and at 390×844: source crop and context, page-map centering, filters, keyboard
acceptance and correction, on-demand candidates, the accuracy floor, and the Arabic crop and
candidate 503 states. The existing 30-journey Playwright suite passes, but none of its journeys
opens the workbench; no automated journey for the workbench is committed yet. The
current answer completes part of plan 021: the record, queue, workbench, and decision
measurement are present; identity choices, relationship review, and folding the experimental
dialog into the normal draft flow remain open.

**Status note, 2026-10-26 — name candidates are measured; the local judge is a
separate opinion (uncommitted).** Plan 022's deterministic pipeline, measurement,
and the bounded local Liquid path are implemented. The separate active row is
enabled for local `d1:3b`; a later eight-name shadow pilot is too small for a
quality or adoption claim. The workbench derives
display/canonical/compact/particle/skeleton keys without rewriting the displayed spelling, so
`عبد الله` and `عبدالله` meet for matching while a correction keeps the reviewer's exact text.
Ranking still keeps at most eight chips and prefers a reader/file exact match over the common
lexicon; when the same key appears in the file, the file's display spelling wins and the
machine reading remains visible above the chips. The `جاسم` probe is kept as an abstention case
rather than a confident wrong list.

The decision ledger now stores what the reviewer saw. Migration
`0053_source_tree_suggestion_decision_context.sql` adds the at-most-12-entry candidate snapshot,
the selected candidate, input method, judge-output slot, normalizer/pool versions, and duration.
The server validates the evidence and writes it in the same transaction as the decision. A
malformed snapshot does not consume the decision: the web client retries once without evidence
and reports that the measurement was lost. Old clients still write the defaults, and missing
snapshots are counted as gaps.

`make name-eval` is the read-only replay. It reports candidate recall@8 and recall among any
shown candidate, no-candidate cases, all-wrong candidate sets, top-1/top-3/MRR, typed rate,
median time to decision, missing snapshots, and snapshots written by a different normalizer.
It also reports candidate-source coverage, breakdowns by file class, resolution, page, reader
confidence and date, judge abstention split by whether the truth is present, judge versus
deterministic top-1/top-3, a paired bootstrap interval, calibration buckets, and the
300-decision gate. Candidate names and file names are not printed. The command refuses to
reinterpret an old snapshot with today's keys.

The bounded local Liquid adapter, core client, config endpoint, shadow sampling, separate
active row, and optional tight-crop image path are implemented. Code remains fail-closed if
`NAME_JUDGE_MODE` is unset; `.env.example` enables active mode for local `d1:3b` with
`NAME_JUDGE_IMAGES=false`. The privacy decision (local inference) and the still-unrun
pre-registered go/no-go protocol are recorded in
[`docs/name-judge-privacy-and-evaluation.md`](name-judge-privacy-and-evaluation.md). No shadow
evaluation has met its sample and class gates, so no quality or lift claim exists.

What this does **not** establish: the local pilot has only eight truth-labelled shadow decisions
in one class, below the rate floor, so candidate recall remains unmeasured. Shadow mode has not produced 300
decisions across the three source classes. The separate active row is enabled for local Liquid,
but it is only an opinion: no candidate ordering, percentage, model quality, or lift claim is
supported. The machine reading remains a hypothesis, and manual transcription against the
source crop remains the honest path for the calligraphic chart.
