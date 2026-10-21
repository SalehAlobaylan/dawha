# Arabic chart and lineage OCR benchmark

The benchmark separates three outcomes: finding each label occurrence, transcribing it, and
tracing/interpreting its connections. The tools are ready to collect evidence; they do not qualify
a model automatically. The held-out targets are at least 99% exact-name precision and 95%
exact-name recall for a declared chart class. Accuracy stays pending until independent Arabic
review, adjudication, supported-class approval, and enough held-out document evidence exist.

## Private, blind annotation

Use `tools/ocr-eval/label.html` locally in a browser. It is a standalone offline tool: page images
and labels are read in the browser and are not uploaded. Render every physical PDF page from the
original at its native source quality, keep a private page-to-source manifest, and do not upscale a
poor scan. Two Arabic readers independently load the same pages without seeing model proposals or
each other's labels. For every page, each reader:

1. Marks every visible name occurrence separately, including repeated identical names; records the
   exact visible Arabic, readable/partial/unreadable status, and any unclear span.
2. Marks branch regions, traces visible connector paths, and records relationship endpoints only
   when the source provides explicit support. The tool requires evidence text for each relationship.
3. Inspects residual page areas and confirms full-page review only after that pass.

Export each reader's independent JSON file. The adjudicator loads both files and the exact source
page images, inspects highlighted reader regions, resolves every disagreement or one-reader-only
item, and exports one adjudicated gold JSONL row per document. Each page records its dimensions and
SHA-256 digest of the original rendered image bytes. Adjudication requires the two annotations and
the loaded page image to match on page number, dimensions, and digest, so a different scan with the
same dimensions cannot satisfy source verification. Equal name strings never pair occurrences: the
tool compares page and geometry. Name agreement requires close geometry plus matching transcription
and status; every disagreement remains open until a person chooses a label or marks it absent.
Relationships always require an explicit adjudicator decision, even when both readers recorded the
same endpoints and predicate. The adjudicator sees the reader evidence and endpoint/connector
overlays, and a relationship cannot be exported if either endpoint was excluded. Keep the reviewer
identity and per-item decision ledger. Do not treat OCR or normal import review as gold annotation.

Annotation exports use schema version 2. A page has `page_id`, page number, source dimensions,
`image_sha256` for the exact rendered source image, `coverage_confirmed`, optional verbatim page text,
and its text status. `nodes` are individual
occurrences with stable IDs, `page_id`, a normalized top-left `[0,1]` `region`, exact `name_ar`,
`name_status`, and `unreadable_spans_ar` when partial. `branches` have page-relative regions;
`connectors` have page-relative point paths and a trace kind; `edges` refer to occurrence IDs and
carry a predicate plus visible `evidence_ar`. Gold export is blocked unless both readers confirmed
every page and the adjudicator loaded matching source images with the recorded page dimensions.
The adjudicator's JSONL gold uses these same entities with adjudicator-created stable IDs. All
source images, independent annotations, and gold labels stay in a permissioned private location
outside the shared repository. The adjudicated gold preserves page dimensions and image fingerprints
alongside the reader IDs and item-level decisions.

## Local Tesseract baselines

`tools/ocr-eval/run_baselines.py` compares full-page Arabic Tesseract, 1024-pixel tiles with
256-pixel overlap, and tiled Tesseract restricted to reviewer-corrected branch regions. It never
uses the network. Install Pillow and Tesseract with the `ara` model in the local environment. The
page images and manifest are kept private; paths are not copied into prediction output.

The private manifest is a JSON array. Image paths are relative to the manifest directory unless
absolute. Branches in the third baseline must be corrected by a reviewer before the run:

```json
[
  {
    "document_id": "jasser",
    "source_class": "chart",
    "split": "development",
    "pages": [
      {
        "page_id": "p1",
        "image_path": "pages/p1.png",
        "branches": [
          {"region": {"x": 0.10, "y": 0.15, "width": 0.35, "height": 0.70}}
        ]
      }
    ]
  }
]
```

Run all three baselines and score them against the same adjudicated gold:

```sh
python3 tools/ocr-eval/run_baselines.py \
  --manifest /private/benchmark/manifest.json \
  --output-dir /private/benchmark/predictions

python3 tools/ocr-eval/evaluate.py \
  --gold /private/benchmark/gold.jsonl \
  --predictions full-page=/private/benchmark/predictions/full_page_tesseract.jsonl \
  --predictions tiled=/private/benchmark/predictions/tiled_tesseract.jsonl \
  --predictions reviewed-branches=/private/benchmark/predictions/reviewed_branch_tesseract.jsonl \
  --output /private/benchmark/comparison.json
```

Provider/vision prediction JSONL can be added with another `--predictions label=path` argument.
Predictions need stable local occurrence IDs and `reference_id` values from a blinded alignment
pass. Align by page and source location, then compare text; never align on equal Arabic strings.
Keep unaligned proposals in the prediction set so they count against precision. Record the
`reference_id` on branch, connector, and tile predictions too. The report records operation count,
provider calls, input bytes, runtime, review time, and cost when those fields are available.

## Scoring contract

- **Exact-name precision** is correctly transcribed, correctly localized readable occurrences
  divided by all proposed name occurrences. **Exact-name recall** uses all fully readable gold
  occurrences as its denominator. Text comparison uses NFC canonical composition only; it does not
  rewrite Arabic spelling, diacritics, tatweel, or spaces.
- **Detection recall** counts localized occurrences even when the label is partial or unreadable.
  Partial-label and unreadable-label detection are reported separately. Name detection requires an
  occurrence alignment and at least 0.50 region IoU on the same page.
- **Search-normalized name and CER scores** are diagnostics only. They fold selected alef/ya forms
  and remove Arabic marks/tatweel; they cannot satisfy the exact target.
- **Branch precision/recall** compare actual polygon overlap when polygons are present; rectangles
  are treated as polygons. **Connector precision/recall** compare traced paths with a discrete
  Frechet distance, not merely shared endpoints.
- **Relationship-structure scores** compare connector hypotheses after localized endpoint mapping.
  End-to-end **relationship scores** require exact-name endpoint mapping as well. Neither metric
  permits proximity, matching names, or a branch hierarchy to create a relationship.
- Report omissions, duplicate proposals, abstentions, unreadable detections, branch/tile/page
  coverage, runtime, cost, and per-document scores beside pooled results. Two correct names out of
  500 must report 100% precision and 0.4% recall, with 498 omissions.

The evaluator uses a document-cluster bootstrap for descriptive 95% intervals. Pages, tiles, crops,
related families, templates, rescans, and degradations are not independent documents. Keep related
charts and source editions in one split; use Jasser for development and add three to five separately
annotated held-out charts for the initial evaluation. A small corpus and a zero-error bootstrap
interval do not demonstrate general reliability.

The evaluator deliberately always returns `pending_independent_review`, even when point targets
are reached. It cannot establish annotator independence, adjudication quality, sufficient
uncertainty bounds, or which chart class the product supports. No workflow may enable assisted
proposals for a class until human review attests to the held-out evidence and the platform gate is
explicitly configured for that class and pipeline version.

Tests:

```sh
python3 -m unittest discover -s tools/ocr-eval -p 'test_*.py'
node --test tools/ocr-eval/test_labeler.mjs
```

Do not commit private family PDFs, crops, names, outputs, or labels. Hosted processing is permitted
only for the exact authorized Jasser source under the paid-project inline-image, `store=false`
policy. The first benchmark has not been run: no independent Jasser gold or hosted comparison is
present in this repository.
