#!/usr/bin/env python3
"""Plan 030-B: Lorimer Vol. II (Geographical & Statistical, 1908) tribal
entries → source_passages, as the first real textual receipts.

Honesty contract:

1. The source file is the archive.org scan's OCR text (public domain: Crown
   pre-1957 / US pre-1929). The generator ensures the file exists under the
   local storage root with a stable key, records its sha256, and never
   paraphrases: passages are the OCR verbatim, OCR warts included. The
   passage rows say `ocr` in every locator so a reader never mistakes OCR
   noise for an edited text.
2. Extraction heuristic (stated, not hidden): each entry is found by an
   ALL-CAPS anchor line and cut at a window boundary (~90 lines / 2400 chars,
   trimmed to a paragraph edge; the opening of an entry carries the tribe's
   derivation + whereabouts + sections, which is what the atlas needs). Page
   numbers are NOT recoverable from the djvu text — the locator names the
   entry instead of inventing page numbers.
3. Absences are recorded, not papered over: Hawazin and Mudar have no entry
   in this volume (both were historical constructs by 1908); the artifact
   lists every spelling searched.
4. Everything is idempotent: sources row on id, passages on
   (source_id, sequence_number). Re-running changes nothing.

The seed lands `sources` + `source_files` + `source_passages` only — no
statements/claims: association of a passage to a tribe is review work
(plan 030-C's lane and plan 031's dossiers), never part of ingestion.

Run:    python3 infra/local/build-gazetteer-passages.py [path-to-ocr.txt]
Writes: infra/local/tribe-gazetteer-passages.json
        infra/local/seed-tribe-gazetteers.sql
        (and ensures the source file sits under SOURCE_STORAGE_DIR)
"""
import hashlib
import json
import os
import re
import shutil
import sys
import unicodedata
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
ARTIFACT = ROOT / "infra" / "local" / "tribe-gazetteer-passages.json"
SEED = ROOT / "infra" / "local" / "seed-tribe-gazetteers.sql"
USER_AGENT = "dawha-gazetteer-passages/1.0 (research artifact generator)"

ARCHIVE_URL = ("https://archive.org/download/in.ernet.dli.2015.206964/"
               "2015.206964.Gazetteer-Of_djvu.txt")
ARCHIVE_ITEM = "in.ernet.dli.2015.206964"

# Default storage root mirrors .env.example's SOURCE_STORAGE_DIR relative to
# services/core-api (the server resolves ../../.data/source-storage).
STORAGE_DIR = ROOT / ".data" / "source-storage"
STORAGE_KEY = "sources/lorimer-1908-vol2-gazetteer-djvu.txt"

SOURCE_ROW_ID = "b0000000-0000-0000-0000-00000000e003"

# (tribe canonical, entry anchor regex, fixed passage uuid slot). The anchor
# regexes were verified against the real OCR text on 2026-11-15; Lorimer's
# entry titles use 'APOSTROPHES for the hamza and "(BANI)" inversions.
# Per-entry discovery spec. Two discovery modes, both verified against the
# real OCR on 2026-11-15:
#   header  — the entry's ALL-CAPS title line (tolerates OCR'd page numbers)
#   def     — the entry's definitional sentence; discovery walks back up to
#             `backtrack` lines for a header line and starts there (several
#             1908 headers are OCR-garbled mid-line: "HHTIlII" for QAHTAN,
#             "'AKIZAH" for 'ANIZAH), falling back to the definition line.
# بنو تميم has TWO entries in this volume (the Najdi tribe — whose opening is
# unrecoverable in this OCR, so its Distribution section is used; and the
# separate Mesopotamian Bani Tamim entry).
ENTRY_SPECS: list[dict] = [
    {"tribe": "حرب",      "find": r"Singular\s+Harbi\s*\.\s*A\s+great",        "backtrack": 6,
     "label": "بداية مدخل حرب (من سطر التعريف)"},
    {"tribe": "شمّر",     "find": r"^'?\s*SHAMMAR\s*$",                        "backtrack": 0,
     "label": "بداية مدخل شمر (مع جبل شمر) — المدخل المشترك للقبيلة والإمارة"},
    {"tribe": "عنزة",     "find": r"Singular\s+'?Anizi",                       "backtrack": 6,
     "label": "بداية مدخل عنزة (من سطر التعريف)"},
    {"tribe": "عتيبة",    "find": r"singular\s+is\s+.{0,8}Ataibi",             "backtrack": 10,
     "label": "بداية مدخل عتيبة (من سطر التعريف)"},
    {"tribe": "مطير",     "find": r"A\s+great\s+and\s+typical\s+.?\s*Bedouin", "backtrack": 6,
     "label": "بداية مدخل مطير (من سطر التعريف)"},
    {"tribe": "بنو خالد", "find": r"singular\s+of\s+the\s+name\s+is\s+Khalidi", "backtrack": 6,
     "label": "بداية مدخل خالد (بني) (من سطر التعريف)"},
    {"tribe": "قحطان",    "find": r"The\s+Qahtan\s+are\s+an\s+important\s+tribe", "backtrack": 8,
     "label": "بداية مدخل قحطان (من سطر التعريف)"},
    {"tribe": "بنو تميم", "find": r"BistriMion|Distribution\.—",               "backtrack": 0,
     "label": "مقطع التوزيع من مدخل تميم (بني) — النجد",
     "near": r"Bani\s+Tamim", "pick": "first"},
    {"tribe": "بنو تميم", "find": r"A\s+large\s+and\s+powerful\s+Arab\s+tribe", "backtrack": 6,
     "label": "بداية مدخل تميم (بني) (II) — فرع ما بين الكارون وشط العرب",
     "near": r"TAMIM", "pick": "first"},
]

# Spelling variants searched for the two tribes this volume does NOT cover.
ABSENCE_SEARCHES: dict[str, list[str]] = {
    "هوازن": ["HAWAZIN", "HOWAZIN", "HUWAZIN", "HAWAZ"],
    "مضر": ["MUDAR", "MODAR", "MUZAR", "MU?DAR"],
}

NEXT_ENTRY = re.compile(r"^[A-Z0-9 ''\(\)\.\-]{4,40}$")  # candidate header line
WINDOW_LINES = 90
WINDOW_CHARS = 2400


def normalize(value: str) -> str:
    """`identity.NormalizeArabicName`, ported exactly (028-E precedent). For
    the English OCR text this mostly folds punctuation to spaces."""
    value = unicodedata.normalize("NFKC", value)
    out: list[str] = []
    for ch in value:
        if unicodedata.category(ch) == "Mn" or ch == "\u0640":
            continue
        if ch in "أإآٱ":
            ch = "ا"
        elif ch == "ى":
            ch = "ي"
        elif ch == "ة":
            ch = "ه"
        if ch.isspace() or unicodedata.category(ch).startswith("P"):
            out.append(" ")
        else:
            out.append(ch)
    return " ".join("".join(out).split())


def ensure_source_file(argv_path: str | None) -> Path:
    """The real bytes, wherever they came from, end up under the storage root
    with a stable key; the seed's `source_files` row points at that key."""
    target = STORAGE_DIR / STORAGE_KEY
    if target.exists() and target.stat().st_size > 1_000_000:
        return target
    if argv_path:
        src = Path(argv_path).expanduser().resolve()
        if not src.exists():
            print(f"source file not found: {src}", file=sys.stderr)
            return sys.exit(1)
    else:
        print(f"downloading {ARCHIVE_URL} ...", file=sys.stderr)
        target.parent.mkdir(parents=True, exist_ok=True)
        tmp = target.with_suffix(".part")
        urllib.request.urlretrieve(
            urllib.request.Request(ARCHIVE_URL, headers={"User-Agent": USER_AGENT}).full_url, tmp)
        src = tmp
    target.parent.mkdir(parents=True, exist_ok=True)
    shutil.copyfile(src, target)
    return target


def is_header_line(line: str) -> bool:
    """A short ALL-CAPS line = a candidate entry/sub-entry header. Sub-headings
    ('1. ORIGIN.') carry digits/period-ordinal patterns and are excluded by the
    no-digit rule only when they also lack a trailing page number."""
    if not NEXT_ENTRY.match(line.strip()):
        return False
    body = line.strip().rstrip("0123456789 ").strip()
    return len(body.replace(" ", "")) >= 3


def extract_entry(lines: list[str], spec: dict) -> list[dict]:
    """Definition/header-anchored extraction. Finds the `find` regex, walks
    back up to `backtrack` lines for a header-ish line, and cuts a bounded
    window at a paragraph edge. Returns the extracted candidates."""
    find = re.compile(spec["find"], re.M if spec["find"].startswith("^") else 0)
    out: list[dict] = []
    for i, line in enumerate(lines):
        if not find.search(line):
            continue
        if spec.get("near"):
            near = re.compile(spec["near"])
            context = "\n".join(lines[max(0, i - 4):i + 40])
            if not near.search(context):
                continue  # a real-but-unrelated entry (e.g. a different tribe's Distribution)
        start = i
        if spec["backtrack"]:
            header = re.compile(r"^[A-Z0-9 ''\(\)\.\-]{3,40}$")
            for back in range(1, spec["backtrack"] + 1):
                candidate = lines[i - back].strip() if i - back >= 0 else ""
                if header.match(candidate):
                    start = i - back
                    break
        window = lines[start:start + WINDOW_LINES]
        cut = "\n".join(window)[:WINDOW_CHARS]
        cut = cut[:cut.rfind("\n\n")] if "\n\n" in cut else cut
        # drop trailing header lines bleeding in from the next entry
        tail_lines = cut.split("\n")
        while tail_lines and (is_header_line(tail_lines[-1]) or not tail_lines[-1].strip()):
            tail_lines.pop()
        text = "\n".join(tail_lines).strip()
        if len(text) < 200:
            continue
        out.append({"start_line": start + 1, "text": text})
        break  # one passage per spec entry
    return out


def main() -> int:
    argv_path = sys.argv[1] if len(sys.argv) > 1 else None
    source_file = ensure_source_file(argv_path)
    raw = source_file.read_bytes()
    file_sha = hashlib.sha256(raw).hexdigest()
    lines = raw.decode("utf-8", errors="replace").split("\n")

    entries: list[dict] = []
    absences: list[dict] = []
    for spec in ENTRY_SPECS:
        found = extract_entry(lines, spec)
        if not found:
            absences.append({"tribe": spec["tribe"], "label": spec.get("label"),
                             "searched": spec["find"],
                             "note": "no qualifying extract in this volume's OCR"})
            continue
        for f in found:
            entries.append({
                "tribe": spec["tribe"],
                "locator_ar": spec["label"] + " — نص OCR كما هو",
                "start_line": f["start_line"],
                "text": f["text"],
            })

    # the declared absences, verified against every listed spelling
    for tribe, spellings in ABSENCE_SEARCHES.items():
        hits = {s: bool(re.search(rf"^{s}\b", "\n".join(lines), re.M)) for s in spellings}
        if any(hits.values()):
            absences.append({"tribe": tribe, "searched": spellings,
                             "note": f"anchor spellings found as cross-references only: "
                                     f"{[s for s, hit in hits.items() if hit]}"})
        else:
            absences.append({"tribe": tribe, "searched": spellings,
                             "note": "no entry and no cross-reference in this volume"})

    if not entries:
        print("no entries extracted; refusing to write empty artifacts", file=sys.stderr)
        return 1

    subset = json.dumps(
        [{"tribe": e["tribe"], "locator_ar": e["locator_ar"], "text": e["text"]} for e in entries],
        ensure_ascii=False, sort_keys=True)
    subset_sha = hashlib.sha256(subset.encode("utf-8")).hexdigest()

    artifact = {
        "meta": {
            "source": "Lorimer, J.G. — Gazetteer of the Persian Gulf, Oman and Central Arabia, "
                      "Vol. II (Geographical and Statistical), Calcutta 1908; archive.org OCR text",
            "license": "public_domain (UK Crown publication pre-1957; US publication pre-1929)",
            "archiveItem": ARCHIVE_ITEM,
            "archiveUrl": ARCHIVE_URL,
            "fetchedAt": __import__("datetime").date.today().isoformat(),
            "generator": "infra/local/build-gazetteer-passages.py",
            "fileSha256": file_sha,
            "fileBytes": len(raw),
            "storageKey": STORAGE_KEY,
            "passageSha256": subset_sha,
            "extraction": f"verbatim OCR openings (≤{WINDOW_CHARS} chars at a paragraph edge) — "
                          "entry-level locators, page numbers intentionally absent (djvu text carries none)",
            "honesty": "passages are the OCR as-is including its noise; no statement or claim rows "
                       "are produced here — tribe association is review work (030-C / 031)",
        },
        "entries": entries,
        "absences": absences,
    }
    ARTIFACT.write_text(json.dumps(artifact, ensure_ascii=False, indent=2) + "\n")

    def sql_str(value: str) -> str:
        return "'" + value.replace("'", "''") + "'"

    lines_sql = [
        "-- Plan 030-B: Lorimer Vol. II (1908) tribal entries as source_passages.",
        "-- Generated by infra/local/build-gazetteer-passages.py; do not edit by hand.",
        f"-- License: public domain (Crown pre-1957 / US pre-1929). Fetched: {artifact['meta']['fetchedAt']}.",
        f"-- File sha256: {file_sha}. Passage-subset sha256: {subset_sha}.",
        "--",
        "-- The `source_files` row points at the real bytes under the storage",
        "-- root (this seed assumes the file is in place; the generator puts it",
        "-- there). Passages are OCR verbatim with entry-level locators.",
        "",
        "INSERT INTO sources (id, title_ar, author_ar, source_type, publication_date_from,",
        "                     citation_ar, location_ar, metadata)",
        "VALUES (",
        f"  {sql_str(SOURCE_ROW_ID)},",
        "  'معجم الخليج وعُمان ووسط الجزيرة العربية — المجلد الثاني (الجغرافي والإحصائي)',",
        "  'جاي جي لوريمر (J. G. Lorimer)', 'book', '1908-01-01',",
        "  'Lorimer, J.G., Gazetteer of the Persian Gulf, Oman and Central Arabia, Vol. II, Geographical and Statistical, Calcutta: Superintendent Government Printing, 1908.',",
        "  'Calcutta',",
        f"  jsonb_build_object('license', 'public_domain', 'license_note', 'UK Crown publication pre-1957; US publication pre-1929',",
        f"    'archive_item', {sql_str(ARCHIVE_ITEM)}, 'archive_url', {sql_str(ARCHIVE_URL)},",
        f"    'fetched_at', {sql_str(artifact['meta']['fetchedAt'])}, 'file_sha256', {sql_str(file_sha)},",
        f"    'generator', 'infra/local/build-gazetteer-passages.py', 'passage_subset_sha256', {sql_str(subset_sha)}))",
        "ON CONFLICT (id) DO NOTHING;",
        "",
        "INSERT INTO source_files (source_id, storage_key, original_filename_ar, mime_type, byte_size, checksum_sha256)",
        f"SELECT {sql_str(SOURCE_ROW_ID)}, {sql_str(STORAGE_KEY)}, '2015.206964.Gazetteer-Of_djvu.txt',",
        f"       'text/plain', {len(raw)}, {sql_str(file_sha)}",
        "WHERE NOT EXISTS (SELECT 1 FROM source_files WHERE storage_key = " + sql_str(STORAGE_KEY) + ");",
        "",
    ]
    for seq, e in enumerate(entries, start=1):
        lines_sql.append(f"-- {e['tribe']} — OCR text starting at source line {e['start_line']}")
        lines_sql.append("INSERT INTO source_passages (source_id, sequence_number, page_number, locator_ar, text_ar, normalized_text_ar)")
        lines_sql.append(f"SELECT {sql_str(SOURCE_ROW_ID)}, {seq}, NULL, {sql_str(e['locator_ar'])}, {sql_str(e['text'])}, {sql_str(normalize(e['text']))}")
        lines_sql.append("ON CONFLICT (source_id, sequence_number) DO NOTHING;")
        lines_sql.append("")
    SEED.write_text("\n".join(lines_sql))

    print(f"wrote {ARTIFACT.name}: {len(entries)} passages "
          f"({', '.join(sorted({e['tribe'] for e in entries}))}); "
          f"absences recorded: {', '.join(a['tribe'] for a in absences)}")
    print(f"wrote {SEED.name}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
