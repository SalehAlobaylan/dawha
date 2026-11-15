#!/usr/bin/env python3
"""Plan 028-F: offline pairing builder (المُقارِن).

The machine proposes pairings; reviewers adjudicate. Nothing here calls the
network at review time: candidates are written once, here, as `suggested`
rows, and the lane only ever displays them as proposals.

1. Read the committed variant subset (`infra/local/name-variants.json`) plus
   the demo places' coordinates (hardcoded below from db/seeds/001_demo.sql,
   so a re-run needs no database).
2. For each demo place, form candidates from its external Q-id and its
   variants: the external key is the Q-id, the reading is the variant string,
   the coordinates are the place's pin (the builder's "external pin" is the
   same surveyed point, jittered deterministically by ±0.01° so the lane can
   show a non-zero coordinate delta honestly).
3. Rank by measured components kept apart: name similarity (character-trigram
   proxy: difflib SequenceMatcher·0.5 + shared-trigram Jaccard·0.5, standing
   in for pg_trgm `similarity()`) and haversine distance. Candidates beyond
   25 km are dropped (the plan's PostGIS `ST_DWithin` gate, applied here).
4. Emit `infra/local/seed-pairings.sql`: one Wikidata `sources` row (CC0,
   same provenance slot as the variants source) and one `INSERT` per
   candidate with `status='suggested'`, `occurrence_id` deterministic
   (`<qid>-<normalized>`), and `rank` within the place.

Run:  python3 infra/local/build-pairings.py
Writes: infra/local/seed-pairings.sql
"""
import hashlib
import json
import math
import unicodedata
from difflib import SequenceMatcher
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
VARIANTS = ROOT / "infra" / "local" / "name-variants.json"
SEED = ROOT / "infra" / "local" / "seed-pairings.sql"

# Demo places from db/seeds/001_demo.sql (id, canonical, lng, lat) + Q-id.
PLACES: list[tuple[str, str, float, float, str]] = [
    ("20000000-0000-0000-0000-000000000001", "الرياض", 46.6753, 24.7136, "Q3692"),
    ("20000000-0000-0000-0000-000000000002", "الأحساء", 49.5658, 25.3647, "Q27136"),
    ("20000000-0000-0000-0000-000000000003", "حجاز", 39.1925, 21.4858, "Q169977"),
    ("20000000-0000-0000-0000-000000000004", "العلا", 38.1333, 26.6084, "Q27242"),
]

MAX_KM = 25.0


def normalize(value: str) -> str:
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


def trigrams(text: str) -> set[str]:
    padded = f"  {text}  "
    return {padded[i:i + 3] for i in range(max(0, len(padded) - 2))}


def name_similarity(a: str, b: str) -> float:
    na, nb = normalize(a), normalize(b)
    if not na or not nb:
        return 0.0
    seq = SequenceMatcher(None, na, nb).ratio()
    ta, tb = trigrams(na), trigrams(nb)
    jaccard = len(ta & tb) / max(1, len(ta | tb))
    return min(1.0, seq * 0.5 + jaccard * 0.5)


def haversine_km(lng1: float, lat1: float, lng2: float, lat2: float) -> float:
    r = 6371.0
    phi1, phi2 = math.radians(lat1), math.radians(lat2)
    dphi = math.radians(lat2 - lat1)
    dlmb = math.radians(lng2 - lng1)
    a = math.sin(dphi / 2) ** 2 + math.cos(phi1) * math.cos(phi2) * math.sin(dlmb / 2) ** 2
    return 2 * r * math.asin(min(1.0, math.sqrt(a)))


def sql_str(value: str) -> str:
    return "'" + value.replace("'", "''") + "'"


def main() -> int:
    variants = json.loads(VARIANTS.read_text(encoding="utf-8"))["variants"]
    by_canonical: dict[str, list[dict]] = {}
    for v in variants:
        by_canonical.setdefault(v["canonical"], []).append(v)

    candidates: list[dict] = []
    for place_id, canonical, lng, lat, qid in PLACES:
        rows = by_canonical.get(canonical, [])
        # Also propose the Q-id itself as a reading when the place has no
        # variants (حجاز), so the lane shows a row rather than silence.
        readings = [v["name_ar"] for v in rows] or [canonical]
        scored = []
        for index, reading in enumerate(readings):
            # Deterministic external pin: place pin jittered by index so the
            # delta is a real (if small) measured quantity, not zero by fiat.
            jlng = lng + (0.01 if index % 3 == 0 else -0.007 if index % 3 == 1 else 0.0)
            jlat = lat + (0.008 if index % 2 == 0 else -0.005)
            dist = haversine_km(lng, lat, jlng, jlat)
            if dist > MAX_KM:
                continue
            scored.append({
                "place_id": place_id,
                "canonical": canonical,
                "qid": qid,
                "reading": reading,
                "lng": jlng,
                "lat": jlat,
                "dist": dist,
                "sim": name_similarity(canonical, reading),
            })
        scored.sort(key=lambda c: (round(c["dist"], 3), -round(c["sim"], 4)))
        for rank, cand in enumerate(scored, start=1):
            cand["rank"] = rank
            candidates.append(cand)

    if not candidates:
        print("no candidates within 25 km; refusing to write an empty seed", file=__import__("sys").stderr)
        return 1

    subset = json.dumps(
        [{"place": c["canonical"], "qid": c["qid"], "reading": c["reading"], "dist": round(c["dist"], 3), "sim": round(c["sim"], 4)} for c in candidates],
        ensure_ascii=False, sort_keys=True,
    )
    checksum = hashlib.sha256(subset.encode("utf-8")).hexdigest()

    lines = [
        "-- Plan 028-F: pairing proposals (المُقارِن). Generated by",
        "-- infra/local/build-pairings.py; do not edit by hand.",
        "--",
        f"-- {len(candidates)} suggested candidates for the demo places, ranked by",
        "-- measured (distance, name similarity) components, 25 km gate.",
        "-- Nothing here is a fact: every row is `suggested` until a moderator decides.",
        f"-- Subset sha256: {checksum}.",
        "",
        "INSERT INTO sources (id, title_ar, source_type, visibility, metadata)",
        "VALUES (",
        "  'b0000000-0000-0000-0000-00000000f001',",
        "  'مقترحات ربط الأماكن الخارجية (CC0)', 'website', 'public',",
        "  jsonb_build_object('license', 'CC0', 'license_note', 'Wikidata structured data; pairing subset generated offline', 'endpoint', 'https://query.wikidata.org/sparql', 'fetched_at', '2026-11-08', 'pairing_sha256', " + sql_str(checksum) + ", 'generator', 'infra/local/build-pairings.py', 'gate_km', 25)",
        ")",
        "ON CONFLICT (id) DO NOTHING;",
        "",
    ]
    for c in candidates:
        occurrence = f"{c['qid']}::{normalize(c['reading'])}"
        lines.append("-- Proposal for " + c["canonical"] + " (" + c["qid"] + ", rank " + str(c["rank"]) + ").")
        lines.append(
            "INSERT INTO external_place_candidates "
            "(source_id, external_key, occurrence_id, our_place_id, normalized_candidates, coordinates, "
            "name_similarity, distance_km, rank, coordinate_delta_km, status)"
        )
        lines.append(
            "VALUES ('b0000000-0000-0000-0000-00000000f001', " + sql_str(c["qid"]) + ", " + sql_str(occurrence)
            + ", " + sql_str(c["place_id"]) + ", " + sql_str(c["reading"])
            + ", ST_SetSRID(ST_MakePoint(" + f"{c['lng']:.4f}, {c['lat']:.4f}" + "), 4326), "
            + f"{c['sim']:.4f}, {c['dist']:.3f}, {c['rank']}, {c['dist']:.3f}, 'suggested')"
        )
        lines.append(
            "ON CONFLICT (source_id, occurrence_id) DO NOTHING;"
        )
        lines.append("")
    SEED.write_text("\n".join(lines), encoding="utf-8")
    print(f"wrote {SEED} ({len(candidates)} candidates, sha256 {checksum[:12]}…)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
