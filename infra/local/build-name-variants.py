#!/usr/bin/env python3
"""Plan 028-E: Wikidata Arabic alternate names for the curated places (CC0).

The place search branch already reads `historical_place_names` (ILIKE 85 /
trigram 70 — `search/service.go`), so a variant lands in search as data with
zero search-code change. This script is the only writer of that data, and its
contract is:

1. Resolve each curated Q-id's Arabic `skos:altLabel` in small SPARQL batches
   (the verified pattern from `build-reference-layer.py`, no SERVICE block).
2. Keep Arabic alternates only, normalise them with the Go normaliser's exact
   rules (`identity.NormalizeArabicName`), dedupe against the canonical name
   and against each other, and classify `name_type`: a small curated historic
   set (يثرب، طيبة، بكة، اليمامة…) reads `historical`, everything else
   `alternative`. Wikidata carries no historic flag; GeoNames' `isHistoric`
   would drive this column instead (see GeoNames fallback note below).
3. Emit (a) a canonical JSON artifact `infra/local/name-variants.json` with a
   provenance header (endpoint, license, fetched date, sha256 of the subset)
   and (b) a guarded SQL seed `infra/local/seed-name-variants.sql` that
   inserts one `sources` row (license + endpoint + fetched-at + checksum in
   `metadata`) and `INSERT ... SELECT ... WHERE EXISTS (place) ON CONFLICT DO
   NOTHING` per variant. The `WHERE EXISTS` guard is what lets the seed cover
   the full curated shortlist (مكة، المدينة، الطائف…) while landing only the
   variants whose canonical place exists: the demo seed has no مكة/المدينة
   place, so those rows no-op there and land wherever the place exists.

Run:  python3 infra/local/build-name-variants.py
Writes: infra/local/name-variants.json and infra/local/seed-name-variants.sql

Network: the script needs the internet. When it cannot reach Wikidata it
falls back to the embedded fetched snapshot below (fetched 2026-11-08 via the
same query, so the committed artifacts are reproducible offline), and says so
in the artifact header. Re-run with a network to refresh `fetchedAt` and the
checksum; the variant table itself is stable unless Wikidata changes.

GeoNames fallback: if the SA.zip + alternateNamesv2 archive (CC-BY-4.0) is
ever used instead, its `isHistoric` flag maps directly onto `name_type`
(historical vs alternative) and its attribution (`GeoNames CC-BY-4.0`) goes
in the same `sources.metadata` slot as the CC0 note below. The per-id
alternatenames API is the documented small alternative within the 10k/day
quota. Neither is fetched by this script: the committed subset is Wikidata's.
"""
import hashlib
import json
import sys
import unicodedata
import urllib.parse
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
ARTIFACT = ROOT / "infra" / "local" / "name-variants.json"
SEED = ROOT / "infra" / "local" / "seed-name-variants.sql"
USER_AGENT = "dawha-name-variants/1.0 (research artifact generator)"
SPARQL = "https://query.wikidata.org/sparql"

# (canonical name as in `places.canonical_name_ar`, Q-id). Only Q-ids verified
# against their Arabic labels on 2026-11-08 (wbsearchentities + label check).
CURATED: list[tuple[str, str]] = [
    ("الرياض", "Q3692"),
    ("الأحساء", "Q27136"),  # الهفوف item; الأحساء arrives as its altLabel
    ("حجاز", "Q169977"),
    ("العلا", "Q27242"),
    ("مكة المكرمة", "Q5806"),
    ("المدينة المنورة", "Q35484"),
    ("الطائف", "Q182640"),
]

# Historic names the curated set knows as pre-modern names rather than
# spelling variants. With Wikidata this is curation; with GeoNames it would
# be `isHistoric`. Keep it explicit so the column is never a guess.
HISTORIC: set[str] = {"يثرب", "طيبة", "بكة", "اليمامة", "أم القرى", "البلد الأمين"}

# Fetched snapshot 2026-11-08 (skos:altLabel, lang=ar), used when the network
# is unavailable. Covers all CURATED Q-ids above.
FALLBACK_ALTS: list[tuple[str, str, str]] = [
    # (qid, rdfs:label ar, alt ar)
    ("Q3692", "الرياض", "عاصمة السعودية"),
    ("Q3692", "الرياض", "العاصمة السعودية"),
    ("Q3692", "الرياض", "رياض"),
    ("Q3692", "الرياض", "اليمامة"),
    ("Q27136", "الهفوف", "هفوف"),
    ("Q27136", "الهفوف", "الأحساء"),
    ("Q169977", "الحجاز", "منطقة الحجاز"),
    ("Q169977", "الحجاز", "حجاز"),
    ("Q27242", "العُـلا", "مدينة العلا"),
    ("Q27242", "العُـلا", "محافظه العلا"),
    ("Q27242", "العُـلا", "العُـلا"),
    ("Q5806", "مكة المكرمة", "مكه المكرمه"),
    ("Q5806", "مكة المكرمة", "مكّة المكرمة"),
    ("Q5806", "مكة المكرمة", "بكة"),
    ("Q5806", "مكة المكرمة", "البلد الأمين"),
    ("Q5806", "مكة المكرمة", "أم القرى"),
    ("Q35484", "المدينة المنورة", "مدينة رسول الله"),
    ("Q35484", "المدينة المنورة", "مدينة الرسول"),
    ("Q35484", "المدينة المنورة", "المدينه المنوره"),
    ("Q35484", "المدينة المنورة", "يثرب"),
    ("Q35484", "المدينة المنورة", "المدينة"),
    ("Q35484", "المدينة المنورة", "طيبة"),
    ("Q182640", "الطائف", "طائف"),
    ("Q182640", "الطائف", "الطّائف"),
]

# Descriptive tropes, not name variants: the curated subset excludes them so
# search does not jump on a description ("capital of…") as if it were a name.
DESCRIPTIVE: set[str] = {"عاصمة السعودية", "العاصمة السعودية", "منطقة الحجاز"}


def normalize(value: str) -> str:
    """`identity.NormalizeArabicName`, ported exactly: NFKC, drop Mn + U+0640,
    أإآٱ→ا, ى→ي, ة→ه, punctuation/space → single space."""
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


def sparql(query: str) -> list[dict]:
    url = SPARQL + "?format=json&query=" + urllib.parse.quote(query)
    request = urllib.request.Request(url, headers={"User-Agent": USER_AGENT})
    with urllib.request.urlopen(request, timeout=60) as response:
        return json.load(response)["results"]["bindings"]


def fetch_alts(qids: list[str]) -> tuple[list[tuple[str, str, str]], str]:
    """Small batches of 4: the SERVICE/label pattern that times out Code Mode
    is avoided here — plain VALUES + rdfs:label, exactly the verified shape."""
    out: list[tuple[str, str, str]] = []
    for start in range(0, len(qids), 4):
        group = qids[start:start + 4]
        values = " ".join(f"wd:{qid}" for qid in group)
        query = (
            "PREFIX wd: <http://www.wikidata.org/entity/>\n"
            "PREFIX skos: <http://www.w3.org/2004/02/skos/core#>\n"
            "PREFIX rdfs: <http://www.w3.org/2000/01/rdf-schema#>\n"
            f"SELECT ?item ?itemLabel ?alt WHERE {{ VALUES ?item {{ {values} }} "
            "?item rdfs:label ?itemLabel . FILTER(LANG(?itemLabel)=\"ar\") "
            "OPTIONAL { ?item skos:altLabel ?alt . FILTER (LANG(?alt)=\"ar\") } } LIMIT 300"
        )
        for binding in sparql(query):
            item = binding["item"]["value"].rsplit("/", 1)[-1]
            label = binding["itemLabel"]["value"]
            alt = binding.get("alt", {}).get("value", "")
            if alt:
                out.append((item, label, alt))
    return out, "live"


def main() -> int:
    qids = [qid for _, qid in CURATED]
    try:
        fetched, provenance = fetch_alts(qids)
        fetched_at = __import__("datetime").date.today().isoformat()
    except Exception as exc:  # offline: reproduce from the verified snapshot
        print(f"network unavailable ({exc}); using fetched snapshot 2026-11-08", file=sys.stderr)
        fetched = list(FALLBACK_ALTS)
        provenance = "snapshot-2026-11-08"
        fetched_at = "2026-11-08"

    canonical_by_qid = {qid: canonical for canonical, qid in CURATED}
    entries: dict[tuple[str, str], dict] = {}
    for qid, _label, alt in fetched:
        canonical = canonical_by_qid.get(qid)
        if canonical is None:
            continue
        if alt in DESCRIPTIVE:
            continue
        normalized = normalize(alt)
        # An alternate that normalises to the canonical name itself is not a
        # variant: it restates the place's own name (الأحساء as an altLabel of
        # the الهفوف item, حجاز as an altLabel of الحجاز). Inserting it would
        # only double-count the row search already finds.
        if not normalized or normalized == normalize(canonical):
            continue
        key = (canonical, normalized)
        name_type = "historical" if alt in HISTORIC else "alternative"
        entries[key] = {
            "canonical": canonical,
            "qid": qid,
            "name_ar": alt,
            "normalized": normalized,
            "name_type": name_type,
        }

    ordered = sorted(entries.values(), key=lambda e: (e["canonical"], e["normalized"]))
    if not ordered:
        print("no variants resolved; refusing to write an empty artifact", file=sys.stderr)
        return 1

    subset = json.dumps(
        [{"canonical": e["canonical"], "name_ar": e["name_ar"], "normalized": e["normalized"], "name_type": e["name_type"]} for e in ordered],
        ensure_ascii=False,
        sort_keys=True,
    )
    checksum = hashlib.sha256(subset.encode("utf-8")).hexdigest()
    artifact = {
        "meta": {
            "source": "Wikidata SPARQL skos:altLabel (query.wikidata.org), Arabic alternates only",
            "license": "CC0 — all Wikidata structured data is released to the public domain",
            "endpoint": "https://query.wikidata.org/sparql",
            "fetchedAt": fetched_at,
            "provenance": provenance,
            "generator": "infra/local/build-name-variants.py",
            "checksumSha256": checksum,
            "note": "Descriptive altLabels (عاصمة السعودية…) are excluded; see DESCRIPTIVE in the generator. "
                    "GeoNames SA.zip + alternateNamesv2 (CC-BY-4.0, isHistoric → name_type) is the documented fallback, not fetched here.",
        },
        "variants": ordered,
    }
    ARTIFACT.write_text(json.dumps(artifact, ensure_ascii=False, indent=2) + "\n")

    def sql_str(value: str) -> str:
        return "'" + value.replace("'", "''") + "'"

    lines = [
        "-- Plan 028-E: Wikidata Arabic alternate names (CC0). Generated by",
        "-- infra/local/build-name-variants.py; do not edit by hand.",
        "--",
        f"-- Source: Wikidata SPARQL skos:altLabel, {len(ordered)} variants for {len(CURATED)} curated places.",
        f"-- License: CC0. Fetched: {fetched_at}. Subset sha256: {checksum}.",
        "",
        "INSERT INTO sources (id, title_ar, source_type, visibility, metadata)",
        "VALUES (",
        "  'b0000000-0000-0000-0000-00000000e001',",
        "  'أسماء بديلة من ويكي بيانات (CC0)', 'website', 'public',",
        f"  jsonb_build_object('license', 'CC0', 'license_note', 'Wikidata structured data; retrieved via SPARQL skos:altLabel', 'endpoint', 'https://query.wikidata.org/sparql', 'fetched_at', {sql_str(fetched_at)}, 'variant_sha256', {sql_str(checksum)}, 'generator', 'infra/local/build-name-variants.py')",
        ")",
        "ON CONFLICT (id) DO NOTHING;",
        "",
    ]
    for entry in ordered:
        lines.append(
            "-- Variant of (canonical) — see name-variants.json for its Q-id."
        )
        lines.append(
            "INSERT INTO historical_place_names (place_id, name_ar, normalized_name_ar, name_type, source_id)"
        )
        lines.append(
            "SELECT p.id, "
            + sql_str(entry["name_ar"]) + ", "
            + sql_str(entry["normalized"]) + ", "
            + sql_str(entry["name_type"])
            + ", 'b0000000-0000-0000-0000-00000000e001'"
        )
        lines.append(
            "FROM places p WHERE p.canonical_name_ar = " + sql_str(entry["canonical"])
        )
        lines.append("ON CONFLICT (place_id, name_type, normalized_name_ar) DO NOTHING;")
        lines.append("")
    SEED.write_text("\n".join(lines))
    print(f"wrote {ARTIFACT} ({len(ordered)} variants, sha256 {checksum[:12]}…) and {SEED}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
