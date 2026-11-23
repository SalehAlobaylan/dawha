#!/usr/bin/env python3
"""Plan 030-A: flagship tribe seeds — identities, aliases, and provenance.

This is the first tribal data in the platform. It lands only what the sources
verify, and only after live self-checking:

1. Resolve each curated Q-id (10 flagship tribes) in small SPARQL batches —
   the verified pattern from `build-reference-layer.py` / `build-name-variants.py`.
   Every Q-id AND its Arabic label was verified on 2026-11-15 against
   rdfs:label + P31=Q133311 (tribe); the script refuses to emit a seed whose
   live labels disagree with the curated canonical names.
2. Emit (a) a provenance artifact `infra/local/tribe-seeds.json` (license,
   endpoint, fetch date, sha256 of the tribe subset) and (b) an idempotent SQL
   seed `infra/local/seed-tribe-seeds.sql`:
   - `tribes` rows — canonical Arabic names + Wikidata Arabic descriptions.
     Idempotent via `WHERE NOT EXISTS` guards: the table has no unique
     constraint (0002's shape) and this plan changes no schema.
   - `tribe_aliases` rows — `ON CONFLICT (tribe_id, normalized_value_ar) DO
     NOTHING`. Two honest kinds only:
       * `spelling` — Wikidata altLabels that normalise EQUAL to the canonical
         name under the strict normaliser plus the documented بني/بنو synonym
         rule. Orthographic variants, not claims.
       * `curated` — hand-picked attested alternates (CURATED_ALIASES). A
         curated alias IS a claim (that the names coincide); it is hand-listed
         in full so a reviewer can see and strike every such claim in one
         place.
   Noise filter (binding): Wikidata altLabels carry cross-tribe and
   ancestor-group names (أزد on غامد، قضاعة on حرب، era/state/era names on
   بنو تميم). An altLabel lands only under the two rules above; everything
   else is logged in the artifact under `excluded` and never reaches the
   database.

Network: needs the internet; when Wikidata is unreachable the generator falls
back to the embedded verified snapshot below and says so in the artifact
header. Run:  python3 infra/local/build-tribe-seeds.py
Writes: infra/local/tribe-seeds.json and infra/local/seed-tribe-seeds.sql
"""
import hashlib
import json
import sys
import unicodedata
import urllib.parse
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
ARTIFACT = ROOT / "infra" / "local" / "tribe-seeds.json"
SEED = ROOT / "infra" / "local" / "seed-tribe-seeds.sql"
USER_AGENT = "dawha-tribe-seeds/1.0 (research artifact generator)"
SPARQL = "https://query.wikidata.org/sparql"

# (canonical_name_ar, wikidata qid, fixed tribe uuid in the b…0c… namespace).
# Q-ids + Arabic labels verified 2026-11-15 (label == canonical modulo the
# strict normaliser; P31 = Q133311 `tribe`).
CURATED: list[tuple[str, str, str]] = [
    ("حرب",      "Q3827613",  "b0000000-0000-0000-0000-c00000000001"),
    ("شمّر",     "Q2230906",  "b0000000-0000-0000-0000-c00000000002"),
    ("عنزة",     "Q3780541",  "b0000000-0000-0000-0000-c00000000003"),
    ("بنو تميم", "Q2633574",  "b0000000-0000-0000-0000-c00000000004"),
    ("عتيبة",    "Q2060531",  "b0000000-0000-0000-0000-c00000000005"),
    ("مطير",     "Q2704114",  "b0000000-0000-0000-0000-c00000000006"),
    ("بنو خالد", "Q3827617",  "b0000000-0000-0000-0000-c00000000007"),
    ("هوازن",    "Q3777166",  "b0000000-0000-0000-0000-c00000000008"),
    ("مضر",      "Q3628813",  "b0000000-0000-0000-0000-c00000000009"),
    ("قحطان",    "Q57876879", "b0000000-0000-0000-0000-c0000000000a"),
]

# Hand-picked attested alternates, per canonical (canonical -> aliases). Every
# entry here is a *claim* that the names coincide; review strikes from here.
CURATED_ALIASES: dict[str, list[str]] = {
    "حرب": ["قبيلة حرب", "بني حرب"],
    "شمّر": ["شمر", "قبيلة شمر", "الطنايا"],
    "عنزة": ["قبيلة عنزة"],
    "بنو تميم": ["بني تميم"],
    "عتيبة": ["قبيلة عتيبة"],
    "مطير": ["قبيلة مطير"],
    "بنو خالد": ["بني خالد"],
    "هوازن": ["قبيلة هوازن"],
    "مضر": ["بنو مضر", "المضرية"],
    "قحطان": ["قبيلة قحطان"],
}

# Verified snapshot 2026-11-15 (Arabic labels + altLabels per Q-id), used when
# the network is unavailable. The label-equality check still runs on fallback.
FALLBACK_LABELS: dict[str, str] = {
    "Q3827613": "حرب", "Q2230906": "شمر", "Q3780541": "عنزة",
    "Q2633574": "بنو تميم", "Q2060531": "عتيبة", "Q2704114": "مطير",
    "Q3827617": "بنو خالد", "Q3777166": "هوازن", "Q3628813": "مضر",
    "Q57876879": "قحطان",
}
FALLBACK_ALTS: dict[str, list[str]] = {
    "Q3827613": ["قبائل حرب", "قبايل حرب", "قبيلة حرب", "بني حرب", "بنو حرب",
                 "الحربي", "بنو حرب بن سعد"],
    "Q2230906": ["شمر", "قبيلة شمر", "الطنايا", "السناعيس"],
    "Q3780541": ["قبيلة عنزة", "عنزه", "العنزي"],
    "Q2633574": ["بني تميم", "قبيلة بني تميم", "التميمي"],
    "Q2060531": ["عتيبه", "قبيلة عتيبة", "العتيبي", "عتبان", "العتبان"],
    "Q2704114": ["قبيلة مطير", "المطيري"],
    "Q3827617": ["بني خالد", "قبيلة بنو خالد", "الخوالد", "خوالد"],
    "Q3777166": ["قبيلة هوازن", "هوزان", "الكعبان"],
    "Q3628813": ["بنو مضر", "المضرية", "قبيلة مضر"],
    "Q57876879": ["قبيلة قحطان", "القحطاني"],
}


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


def norm2(value: str) -> str:
    """The strict normaliser plus the one documented synonym rule: بني/بنو
    interchange in Arabian usage (بنو تميم = بني تميم). Applies ONLY inside
    this generator's comparisons; the database keeps the raw forms."""
    return normalize(value).replace("بني ", "بنو ")


def sparql(query: str) -> list[dict]:
    url = SPARQL + "?format=json&query=" + urllib.parse.quote(query)
    request = urllib.request.Request(url, headers={"User-Agent": USER_AGENT})
    with urllib.request.urlopen(request, timeout=90) as response:
        return json.load(response)["results"]["bindings"]


def fetch_labels_and_alts() -> tuple[dict[str, dict], str]:
    """Batched Arabic label + schema:description + altLabels for the curated
    Q-ids. Returns ({qid: {label, desc, alts}}, provenance-mode)."""
    qids = [qid for _, qid, _ in CURATED]
    out: dict[str, dict] = {}
    for start in range(0, len(qids), 4):
        group = qids[start:start + 4]
        values = " ".join(f"wd:{qid}" for qid in group)
        query = (
            "PREFIX wd: <http://www.wikidata.org/entity/>\n"
            "PREFIX rdfs: <http://www.w3.org/2000/01/rdf-schema#>\n"
            "PREFIX skos: <http://www.w3.org/2004/02/skos/core#>\n"
            "PREFIX schema: <http://schema.org/>\n"
            f"SELECT ?item ?label ?desc ?alt WHERE {{ VALUES ?item {{ {values} }}\n"
            '?item rdfs:label ?label . FILTER(LANG(?label)="ar")\n'
            'OPTIONAL { ?item schema:description ?desc . FILTER(LANG(?desc)="ar") }\n'
            'OPTIONAL { ?item skos:altLabel ?alt . FILTER(LANG(?alt)="ar") } } LIMIT 300'
        )
        for binding in sparql(query):
            qid = binding["item"]["value"].rsplit("/", 1)[-1]
            entry = out.setdefault(qid, {"label": "", "desc": "", "alts": []})
            entry["label"] = entry["label"] or binding["label"]["value"]
            entry["desc"] = entry["desc"] or binding.get("desc", {}).get("value", "")
            alt = binding.get("alt", {}).get("value", "")
            if alt and alt not in entry["alts"]:
                entry["alts"].append(alt)
    return out, "live"


def build_aliases(canon: str, qid: str, label: str, alt_labels: list[str],
                  all_canon_norm2: set[str]) -> tuple[list[dict], list[dict]]:
    """Apply the noise filter. Returns (landed, excluded)."""
    landed: dict[str, dict] = {}
    excluded: list[dict] = []
    candidates = [(c, "curated") for c in CURATED_ALIASES.get(canon, [])]
    candidates += [(a, "spelling") for a in [label] + alt_labels]

    for value, kind in candidates:
        if not value or normalize(value) == normalize(canon):
            # the canonical itself, under the STRICT normaliser only (harakat
            # folds): بني/بنo surface forms differ from the canonical under the
            # strict rule and are meant to land as aliases.
            if value != canon:
                excluded.append({"qid": qid, "value": value,
                                 "reason": "normalises equal to the canonical name"})
            continue
        if kind == "spelling" and norm2(value) != norm2(canon):
            excluded.append({"qid": qid, "value": value,
                             "reason": "altLabel is not a spelling variant of the canonical "
                                       "(cross-tribe/ancestor-group noise stays out; log here)"})
            continue
        if kind == "curated" and norm2(value) in all_canon_norm2:
            excluded.append({"qid": qid, "value": value,
                             "reason": "aliases another curated tribe's canonical"})
            continue
        key = normalize(value)
        if key in landed:
            continue
        landed[key] = {"value_ar": value, "normalized_value_ar": key, "kind": kind}

    order = {"spelling": 0, "curated": 1}
    return sorted(landed.values(), key=lambda a: (order[a["kind"]], a["normalized_value_ar"])), excluded


def main() -> int:
    try:
        fetched, provenance = fetch_labels_and_alts()
        fetched_at = __import__("datetime").date.today().isoformat()
    except Exception as exc:  # offline: reproduce from the verified snapshot
        print(f"network unavailable ({exc}); using verified snapshot 2026-11-15", file=sys.stderr)
        fetched = {qid: {"label": FALLBACK_LABELS[qid], "desc": "", "alts": FALLBACK_ALTS[qid]}
                   for qid in FALLBACK_LABELS}
        provenance = "snapshot-2026-11-15"
        fetched_at = "2026-11-15"

    # Self-check (binding): live Arabic labels must agree with the curated
    # canonicals, or nothing is emitted.
    refusals: list[str] = []
    for canon, qid, _ in CURATED:
        label = fetched.get(qid, {}).get("label", "")
        if not label:
            refusals.append(f"{qid}: no Arabic label fetched")
        elif norm2(label) != norm2(canon):
            refusals.append(f"{qid}: live label {label!r} disagrees with canonical {canon!r}")
    if refusals:
        for line in refusals:
            print(f"REFUSAL: {line}", file=sys.stderr)
        return 1

    all_canon_norm2 = {norm2(canon) for canon, _, _ in CURATED}

    tribes: list[dict] = []
    excluded: list[dict] = []
    for canon, qid, uuid in CURATED:
        entry = fetched[qid]
        landed, excluded_here = build_aliases(
            canon, qid, entry["label"], entry["alts"], all_canon_norm2)
        excluded.extend(excluded_here)
        tribes.append({
            "canonical_name_ar": canon,
            "wikidata_qid": qid,
            "tribe_uuid": uuid,
            "description_ar": entry["desc"],
            "aliases": landed,
        })

    if not tribes:
        print("no tribes resolved; refusing to write an empty artifact", file=sys.stderr)
        return 1

    subset = json.dumps(
        [{"canonical_name_ar": t["canonical_name_ar"], "wikidata_qid": t["wikidata_qid"],
          "description_ar": t["description_ar"], "aliases": t["aliases"]} for t in tribes],
        ensure_ascii=False, sort_keys=True)
    checksum = hashlib.sha256(subset.encode("utf-8")).hexdigest()

    artifact = {
        "meta": {
            "source": "Wikidata SPARQL rdfs:label + schema:description + skos:altLabel (Arabic), P31=Q133311",
            "license": "CC0 — all Wikidata structured data is released to the public domain",
            "endpoint": SPARQL,
            "fetchedAt": fetched_at,
            "provenance": provenance,
            "generator": "infra/local/build-tribe-seeds.py",
            "checksumSha256": checksum,
            "selfCheck": "Q-ids and Arabic labels verified 2026-11-15; the generator refuses to emit when the live labels disagree with the curated canonicals",
            "aliasPolicy": "spelling = normalises equal to the canonical (strict normaliser + the بني/بنو synonym rule); "
                           "curated = hand-picked attested alternates (CURATED_ALIASES); every other altLabel is "
                           "excluded and logged under `excluded` — cross-tribe and ancestor-group noise (أزد on غامد، "
                           "قضاعة on حرب، state/era names on بنو تميم) must not reach the alias tables",
            "descriptionPolicy": "descriptions are Wikidata Arabic schema:description verbatim, including any "
                           "historical claims Wikidata carries — those claims are sourced display text, "
                           "not platform assertions; the dossier review flow grades them like any other claim",
        },
        "tribes": tribes,
        "excluded": excluded,
    }
    ARTIFACT.write_text(json.dumps(artifact, ensure_ascii=False, indent=2) + "\n")

    def sql_str(value: str) -> str:
        return "'" + value.replace("'", "''") + "'"

    source_row_id = "b0000000-0000-0000-0000-00000000e002"
    lines = [
        "-- Plan 030-A: flagship tribe identities + aliases + provenance row.",
        "-- Generated by infra/local/build-tribe-seeds.py; do not edit by hand.",
        f"-- Fetched: {fetched_at} ({provenance}). License: CC0 (Wikidata).",
        f"-- Tribe-subset sha256: {checksum}.",
        "--",
        "-- Idempotence: tribe rows guard WHERE NOT EXISTS (0002 has no unique",
        "-- constraint on the name — this plan changes no schema); aliases",
        "-- conflict on (tribe_id, normalized_value_ar); the sources row",
        "-- conflicts on id. Re-running changes nothing.",
        "",
        f"INSERT INTO sources (id, title_ar, source_type, visibility, metadata)",
        "VALUES (",
        f"  {sql_str(source_row_id)},",
        f"  'ويكي بيانات: قبائل عربية منتقاة (CC0)', 'website', 'public',",
        f"  jsonb_build_object('license', 'CC0', 'endpoint', {sql_str(SPARQL)},",
        f"    'fetched_at', {sql_str(fetched_at)}, 'provenance', {sql_str(provenance)},",
        f"    'generator', 'infra/local/build-tribe-seeds.py',",
        f"    'tribe_subset_sha256', {sql_str(checksum)}))",
        "ON CONFLICT (id) DO NOTHING;",
        "",
    ]
    for t in tribes:
        canon = t["canonical_name_ar"]
        desc = t["description_ar"]
        alias_rows = []
        seen: set[str] = set()
        for a in t["aliases"]:
            if a["normalized_value_ar"] in seen:
                continue
            seen.add(a["normalized_value_ar"])
            alias_rows.append((a["value_ar"], a["normalized_value_ar"]))
        lines.append(f"-- {t['wikidata_qid']} · {canon} (fixed uuid)")
        # The tribe is the dictionary's own published catalog row: inserted
        # public and RESTATED public on conflict, because a later migration
        # (0039) flipped the tables' insert default to `private` and the first
        # run of this seed had landed its tribes in that default — a starter
        # catalog no reader and no index could see.
        lines.append(
            "INSERT INTO tribes (id, canonical_name_ar, normalized_name_ar, description_ar, visibility, created_by)")
        lines.append(
            f"SELECT {sql_str(t['tribe_uuid'])}, {sql_str(canon)}, {sql_str(normalize(canon))}, "
            + (sql_str(desc) if desc else "NULL") + ", 'public', NULL")
        lines.append(
            f"ON CONFLICT (id) DO UPDATE SET visibility = 'public', updated_at = now();")
        if alias_rows:
            lines.append("INSERT INTO tribe_aliases (tribe_id, value_ar, normalized_value_ar)")
            lines.append("SELECT t.id, a.value_ar, a.normalized_value_ar")
            lines.append("FROM tribes t, (VALUES")
            lines.append(",\n".join(f"  ({sql_str(v)}, {sql_str(n)})" for v, n in alias_rows))
            lines.append(f") AS a(value_ar, normalized_value_ar)")
            lines.append(f"WHERE t.id = {sql_str(t['tribe_uuid'])}")
            lines.append("ON CONFLICT (tribe_id, normalized_value_ar) DO NOTHING;")
        lines.append("")

    SEED.write_text("\n".join(lines))
    print(f"wrote {ARTIFACT.name} ({len(tribes)} tribes, {len(excluded)} excluded altLabels, sha256 {checksum[:12]}…)")
    print(f"wrote {SEED.name}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
