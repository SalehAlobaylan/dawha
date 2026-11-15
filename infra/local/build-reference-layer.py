#!/usr/bin/env python3
"""Regenerate the atlas reference-caption artifact from Wikidata (CC0).

Every number or caption the atlas draws from this file must be derivable, so the
script that produces it keeps a narrow contract:

1. Resolve each curated entry by its Arabic label (preferred label first, alias
   otherwise) against items that carry P625 coordinates.
2. Disambiguate namesakes by wikibase:sitelinks (the canonical city beats
   stubs/districts sharing its name) and drop the rest.
3. For wide seas whose P625 centroid lies outside the visible frame, keep the
   caption's editorial placement and record the item id (placement:
   'editorial'), so the reference stays re-derivable without pretending the item
   agrees with exactly that coordinate.
4. Emit the artifact with a provenance header. Nothing here may redesign the
   drawn register: captions render as reference ribs, not as archived places.

Run:  python3 infra/local/build-reference-layer.py
Writes: apps/web/src/assets/reference-places.json
"""
import json
import re
import sys
import urllib.parse
import urllib.request
from dataclasses import dataclass
from pathlib import Path

ARTIFACT = Path(__file__).resolve().parents[2] / "apps/web/src/assets/reference-places.json"
USER_AGENT = "dawha-atlas-reference/1.0 (research artifact generator)"
SPARQL = "https://query.wikidata.org/sparql"

# (needle, kind, captionLabel). The caption label is what the atlas draws; the
# needle is what we resolve against, so a caption may keep a shorter name
# ("مكة") than the item's preferred label ("مكة المكرمة").
CURATED: list[tuple[str, str, str]] = [
    ("البحر الأحمر", "sea", "البحر الأحمر"),
    ("الخليج العربي", "sea", "الخليج العربي"),
    ("بحر العرب", "sea", "بحر العرب"),
    ("جبل أحد", "mountain", "جبل أحد"),
    ("جبل طويق", "mountain", "جبل طويق"),
    ("جبال السروات", "mountain", "جبال السروات"),
    ("جبل شار", "mountain", "جبل شار"),
    ("مكة المكرمة", "city", "مكة"),
    ("المدينة المنورة", "city", "المدينة"),
    ("الطائف", "city", "الطائف"),
    ("تبوك", "city", "تبوك"),
    ("الدمام", "city", "الدمام"),
    ("حائل", "city", "حائل"),
    ("نجران", "city", "نجران"),
    ("الهفوف", "city", "الهفوف"),
    ("الرس", "city", "الرس"),
    ("البصرة", "city", "البصرة"),
    ("الكوفة", "city", "الكوفة"),
]

# Wide seas whose P625 centroid falls outside the atlas frame keep an editorial
# caption placement; the item id is recorded beside it.
EDITORIAL_SEA_PLACEMENTS = {
    "بحر العرب": (56.3, 15.1),
}

# A caption whose Arabic display name differs from the source item's Arabic
# preferred label is recorded, never silently transformed. Kept in sync with
# what Wikidata prefers at generation time is the generator's job; a stale note
# is better than a silent rename, and the artifact diff will show it.
KNOWN_NAMING_NOTES = {
    "الخليج العربي": "الخليج الفارسي",
}

WKT_POINT = re.compile(r"^Point\((-?\d+(?:\.\d+)?) (-?\d+(?:\.\d+)?)\)$")


@dataclass
class Resolution:
    needle: str
    qid: str
    longitude: float
    latitude: float
    preferred: bool


def sparql(query: str) -> list[dict]:
    url = SPARQL + "?format=json&query=" + urllib.parse.quote(query)
    request = urllib.request.Request(url, headers={"User-Agent": USER_AGENT})
    with urllib.request.urlopen(request, timeout=60) as response:
        payload = json.load(response)
    return payload["results"]["bindings"]


def point_of(raw: str) -> tuple[float, float]:
    match = WKT_POINT.match(raw.strip())
    if not match:
        raise ValueError(f"unexpected WKT literal: {raw!r}")
    return float(match.group(1)), float(match.group(2))


def resolve_batch(needles: list[str]) -> dict[str, list[Resolution]]:
    """One preferred-label pass over a batched VALUES clause (no SERVICE block:
    it is not needed for queries whose answer is an id and a point)."""
    values = " ".join(f'"{needle}"@ar' for needle in needles)
    query = f"""SELECT ?qid ?coord ?needleVal WHERE {{
  VALUES ?needle {{ {values} }}
  ?item wdt:P625 ?coord ; rdfs:label ?needle .
  BIND(?needle AS ?needleVal)
  BIND(STRAFTER(STR(?item), "http://www.wikidata.org/entity/") AS ?qid)
}} LIMIT {len(needles) * 8}"""
    found: dict[str, list[Resolution]] = {}
    for binding in sparql(query):
        longitude, latitude = point_of(binding["coord"]["value"])
        found.setdefault(binding["needleVal"]["value"], []).append(Resolution(
            needle=binding["needleVal"]["value"],
            qid=binding["qid"]["value"],
            longitude=longitude,
            latitude=latitude,
            preferred=True,
        ))
    return found


def resolve_alias(needle: str) -> list[Resolution]:
    query = f"""SELECT ?qid ?coord WHERE {{
  ?item wdt:P625 ?coord ; skos:altLabel "{needle}"@ar .
  BIND(STRAFTER(STR(?item), "http://www.wikidata.org/entity/") AS ?qid)
}} LIMIT 8"""
    out = []
    for binding in sparql(query):
        longitude, latitude = point_of(binding["coord"]["value"])
        out.append(Resolution(needle=needle, qid=binding["qid"]["value"],
                              longitude=longitude, latitude=latitude, preferred=False))
    return out


def sitelinks(qids: list[str]) -> dict[str, int]:
    if not qids:
        return {}
    values = " ".join(f"wd:{qid}" for qid in qids)
    query = f"""SELECT ?qid ?sitelinks WHERE {{
  VALUES ?item {{ {values} }}
  ?item wikibase:sitelinks ?sitelinks .
  BIND(STRAFTER(STR(?item), "http://www.wikidata.org/entity/") AS ?qid)
}}"""
    out: dict[str, int] = {}
    for binding in sparql(query):
        out[binding["qid"]["value"]] = int(binding["sitelinks"]["value"])
    return out


def pick(resolutions: list[Resolution]) -> Resolution | None:
    if not resolutions:
        return None
    preferred = [r for r in resolutions if r.preferred]
    pool = preferred or resolutions
    if len(pool) == 1:
        return pool[0]
    ranked = sitelinks([r.qid for r in pool])
    return max(pool, key=lambda r: ranked.get(r.qid, 0))


def round3(value: float) -> float:
    return round(value, 3)


def main() -> int:
    resolutions: dict[str, Resolution | None] = {}
    for start in range(0, len(CURATED), 6):
        group = CURATED[start:start + 6]
        needles = [needle for needle, _kind, _label in group]
        batch = resolve_batch(needles)
        for needle in needles:
            if needle in batch:
                resolutions[needle] = pick(batch[needle])
    # Names that exist only as Arabic aliases fall back to the alias pattern.
    for needle, kind, _label in CURATED:
        if resolutions.get(needle) is None and kind in ("sea", "city"):
            resolutions[needle] = pick(resolve_alias(needle))

    entries: list[dict] = []
    unresolved: list[str] = []
    naming_notes: list[dict] = []
    for needle, kind, caption in CURATED:
        resolved = resolutions.get(needle)
        if resolved is None:
            unresolved.append(needle)
            continue
        placement = "wikidata"
        longitude, latitude = round3(resolved.longitude), round3(resolved.latitude)
        if needle in EDITORIAL_SEA_PLACEMENTS:
            placement = "editorial"
            longitude, latitude = EDITORIAL_SEA_PLACEMENTS[needle]
        note = KNOWN_NAMING_NOTES.get(caption)
        if note:
            naming_notes.append({"caption": caption, "qid": resolved.qid, "sourceLabelAr": note})
        entry = {
            "label": caption,
            "kind": kind,
            "qid": resolved.qid,
            "longitude": longitude,
            "latitude": latitude,
            "placement": placement,
        }
        entries.append(entry)

    artifact = {
        "meta": {
            "source": "Wikidata SPARQL + sitelinks disambiguation (query.wikidata.org)",
            "license": "CC0 — all Wikidata structured data is released to the public domain",
            "fetchedAt": __import__("datetime").date.today().isoformat(),
            "generator": "infra/local/build-reference-layer.py",
            "disambiguation": "candidate items ranked by wikibase:sitelinks; namesakes were dropped",
            "namingNotes": naming_notes,
            "unresolved": unresolved,
        },
        "entries": entries,
    }
    ARTIFACT.write_text(json.dumps(artifact, ensure_ascii=False, indent=2) + "\n")
    print(f"wrote {ARTIFACT} with {len(entries)} entries")
    if unresolved:
        print("unresolved (report in the artifact header):", ", ".join(unresolved))
    return 0


if __name__ == "__main__":
    sys.exit(main())
