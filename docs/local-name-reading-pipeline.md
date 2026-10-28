# Local name preparation and review

The source panel's **قراءة الملف واستخراج الأسماء** action queues local work and
returns promptly. Its progress describes source preparation, region detection,
and crop reading. It is independent of the invisible shadow judge. A completed
run means its declared processing stages finished; it does not establish that
any name is correct. Only a human review supplies truth.

## Using the workbench

1. Open a visual source file and request name preparation. The server returns
   HTTP 202 with a durable run. Reloading the source restores the latest run.
2. Review saved regions while OCR continues. Detection checkpoints commit before
   transcription; unavailable OCR does not discard the geometry.
3. Use **إيقاف الإعداد** to explicitly cancel, or **استئناف المراحل المتبقية** to
   retry incomplete stages. Completed units and earlier hypotheses remain saved.
   Closing a dialog or changing names only aborts display requests, not the job.
4. In the name bench, **إعادة القراءة محلياً** prepares one existing region. The
   comparison disclosure preserves the variant texts and raw engine scores. It
   does not copy a hypothesis into the draft, reorder candidates, or accept it.

The normal reader uses no hosted service and does not use Liquid for OCR.
`NAME_READER_MODE=local` and `CHART_VISION_ENABLED=false` are the defaults.
`off` still permits detection with explicit `reading_unavailable` outcomes.
Installing a new engine/data configuration creates another versioned run;
resuming a run requires its pinned configuration. A repeated identical prepare
or local reread reuses the existing run. Resume repeats only failed/incomplete
units. A new hypothesis never replaces decided evidence. Prior selected machine
readings also remain stable while a reviewer may be editing them.

## Jobs and immutable evidence

`source_name_reading_runs` pins the original checksum, local-only policy,
renderer/geometry/localiser/preprocessing/reader identities, engine version,
Arabic data fingerprint, and crop budget. The public reader identity hashes
these processing settings; it contains no source identifier. Checkpointed
`source_name_reading_units` own tile detection and individual crop attempts.
`source_name_readings` stores each immutable attempt, including unavailable
attempts, beside the logical suggestion. New jobs use `name_reading_run_id`,
not the unrelated `processing_run_id` relation.

The existing PostgreSQL jobs service handles transactional enqueue, leases,
heartbeats, scoped recovery, and fencing. Each write verifies both the live job
lease and the run generation. Explicit cancellation revokes the lease; late
results cannot write. Worker recovery preserves completed units. Retries and
resumes create no duplicate suggestions. The name worker recovers only its own
job type and runs one durable name job at a time.

Run states are `queued`, `running`, `partial`, `succeeded`, `failed`, and
`cancelled`. Outcomes distinguish `readable`, `unreadable`,
`reading_unavailable`, and `manual_review`. Illegible ink is not a service outage.
Stage timings, cache hits, saved units, and source-pixel warnings are processing
measurements, never accuracy percentages.

## Private frames and crop reuse

Frames retain native raster pixels with DPI 0, or PDF analysis pixels at 240 DPI.
They are measured from the original, not substituted from a 300-DPI OCR page or
browser preview. Conflicting dimensions, coordinate spaces, or DPI are rejected.

The configured private storage holds tight/context PNGs under generated cache
keys. Identities include the source checksum, frame, page, normalized region,
actual crop bounds, geometry/renderer/preprocessing versions, and DPI. Identical
renders coalesce. Every request rechecks source authorization before a cache
lookup, and no artifact URL is public or permanently signed. After preparation,
a text-only judge reads the stored measurements with zero original reads or PDF
subprocesses. Image attachments remain disabled for shadow collection.

Each PNG is limited to 1,600 pixels per side, 2.5 million pixels, and 10 MB;
a pair is limited to 20 MB. Cache storage is limited to 256 MiB per source file,
with oldest/expired entries evicted as needed. Entries expire after 30 days; the
worker prunes bounded expired batches. Originals and historical hypotheses are
never deleted by cache maintenance. A geometry/checksum/version change gives a
new identity. The browser holds at most the active/next pair and prefetches only
one adjacent crop after the active crop is ready.

## Bounded local experiment

The installed Arabic Tesseract reader attempts at most three variants under one
`NAME_READER_CROP_BUDGET` (default 12 seconds, configurable from 1 to 30 seconds):

- the raw padded tight crop, segmentation mode 7;
- a bounded local-threshold grayscale variant, segmentation mode 7;
- the raw crop with segmentation mode 6, optionally masking a coloured carrier.

Raw pixels remain available. Masking retains dark ink and is an experiment; it
cannot recover missing source pixels. Tiny name heights can route directly to
manual review. Clipped regions use only the two raw segmentation variants and
retain their outputs for manual review rather than selecting a name. Low contrast
can also bypass OCR. These routing reasons stay with the evidence. Empty OCR is
`unreadable`; missing tools/data or failed subprocesses are unavailable. Variants
keep raw text, segmentation identity, duration, bytes, completion/error status,
and engine score. Exact view agreement is recorded separately from engine score;
neither authorizes acceptance. CPU slots, image/output sizes, subprocess time,
run/tile/region counts, and retries are bounded. No failed local crop falls back
to a hosted model, and no model is downloaded by this workflow.

## Operator tools

Browser controls use the current authenticated session. Operator commands use a
mode-0600 file containing an existing local session token, configured through
`NAME_PIPELINE_SESSION_FILE`. The client verifies the loopback Core API's service
identity before sending a credential or internal selector; it refuses redirects
and remote API hosts. Tokens and input selectors are never printed.

```sh
make name-pipeline-doctor
make name-pipeline-prepare NAME_PIPELINE_ARGS="--file <source-file-uuid>"
make name-pipeline-status
make name-pipeline-resume
make name-pipeline-cancel
make name-pipeline-local-reread NAME_PIPELINE_ARGS="--suggestion <suggestion-uuid>"
```

Commands print aggregate JSON with counts, outcomes, timings, cache hits and
source-pixel warnings. A mode-0600 local cursor remembers the last run without
printing its identifier or storing a session token. `status --file` finds a
file's latest preparation; `--run` can select an internal run explicitly.
`NAME_PIPELINE_API_URL` overrides the loopback Core API URL. Queued/running and
successful operations return 0; partial/failed runs or refused configuration
return 2, and an unavailable/wrong service returns 3.

HTTP surfaces are authenticated and source-authorized:

- `POST /api/v1/source-files/{fileID}/suggestions/read`
- `GET /api/v1/source-files/{fileID}/name-reading-runs/latest`
- `GET /api/v1/name-reading-runs/{runID}`
- `POST /api/v1/name-reading-runs/{runID}/resume` or `/cancel`
- `POST /api/v1/source-tree-suggestions/{suggestionID}/local-reread`
- `GET /api/v1/source-tree-suggestions/{suggestionID}/local-reread/latest`
- `GET /api/v1/source-tree-suggestions/{suggestionID}/local-readings`

## Verification and evaluation limits

All database-writing tests use the guarded dedicated local `*_pipeline_test`
database and temporary storage. Scoped package gates require every selected
DB-backed package to run; the full acceptance gate retains the full manifest.
Test fixtures never become labels in the review database. Failed isolated
scopes are retained for diagnosis.

The private development benchmark records completion counts and time/bytes/
operations by class and variant without gold labels or new human decisions.
It measures processing behaviour, not name correctness. Existing human labels,
snapshots, source checksums and the pre-registered class map remain unchanged.
The 100/300 evaluation floors and class coverage requirements are unchanged;
Plan 024 does not complete Plan 022's human shadow evaluation.


### Private development run — 2026-10-28

The selection was frozen before processing: the first pending region in each of
two registered files per class, ordered by file/page/occurrence/id. No decision
label was read to choose it. Six authenticated `local-reread` commands created
six completed durable runs with the installed Tesseract 5.5.2 Arabic data and
the pinned 12-second crop budget. The private preregistration and aggregate
report are retained under `~/.dawha/name-eval/plan024-benchmark*.json`.

| Registered class | Crops | Text hypotheses / manual review | OCR operations | OCR duration (ms) | Variant PNG bytes |
|---|---:|---:|---:|---:|---:|
| `clean_print` | 2 | 1 / 1 | 5 | 897 | 344,232 |
| `calligraphic_dense` | 2 | 2 / 0 | 6 | 308 | 58,535 |
| `low_resolution_screenshot` | 2 | 1 / 1 | 3 | 111 | 1,875 |

The report separately records each raw/adaptive/segmentation variant's count,
outcome, duration and bytes. One clipped crop retained two raw views for manual
review; one tiny region bypassed OCR. All fourteen executed variants returned
text, which supplies no evidence that their text is correct. No unavailable
attempt or hosted fallback occurred. Total crop processing time was 9,102 ms,
including private preparation; the six sequential CLI/poll/cache cycles took
23,122 ms. Twelve authorized repeated crop fetches took 3–12 ms each on this
machine. Those measurements are a small development sample, not a service SLA.

All six pre-existing suggestions remained byte-for-byte equivalent as database
JSON. All 111 original decisions and snapshots, recorded source checksums and
class-map bytes matched their pre-implementation fingerprints. The corpus check
also verified all 12 recorded originals. The evaluation replay still has 80 truth
labels, 79 qualifying shadow decisions in one class, and `ready=false`.
