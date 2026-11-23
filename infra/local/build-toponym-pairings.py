#!/usr/bin/env python3
"""Plan 030-C: toponym spans over the Lorimer passages → pairing candidates.

The machine reads; reviewers decide. This script:

1. Reads the committed Phase-B artifact (`tribe-gazetteer-passages.json`) —
   no database, no network needed for the text.
2. Extracts capitalized toponym-like token sequences (1–3 words) per passage
   with an explicit stoplist (sentence-initial English function words,
   Lorimer's own structural nouns). Candidates are capped per passage and
   ranked multi-word-first; everything extracted is logged, including what
   was dropped, in the artifact.
3. Resolves each unique toponym against Wikidata (wbsearchentities en →
   entity; then one batched wbgetentities for the Arabic labels and types).
   Only toponyms that RESOLVE land in the lane: the pairing lane's contract
   (`0063`) requires a non-empty `external_key`, and a string we cannot
   identify has nothing for a reviewer to accept. Unresolved strings are
   recorded under `unresolved` in the artifact — a future run re-tries them.
4. Matches each resolved toponym's Arabic/English names against the demo
   places (hardcoded from `db/seeds/001_demo.sql`, the `build-pairings.py`
   precedent, so a re-run needs no database) — exact or difflib ≥ 0.80.
   `name_similarity` keeps the measured component; `our_place_id` stays NULL
   where nothing matched, and the review service already refuses to accept
   such rows into a vacuum (plan 028-F's rule, kept verbatim).
5. Emits `infra/local/seed-toponym-pairings.sql`: rows into the EXISTING
   `external_place_candidates` table, `source_id` = the Phase-A Wikidata
   sources row (the Q-id IS that source's identifier), Lorimer provenance
   embedded in `occurrence_id` (`lorimer-v2:<passage-seq>:<slug>`) and in the
   reading text itself. No new tables, no new sources rows, no geometry: the
   toponym candidates are pure identity proposals.

Run:    python3 infra/local/build-toponym-pairings.py
Writes: infra/local/toponym-pairings.json
        infra/local/seed-toponym-pairings.sql
"""
import difflib
import hashlib
import json
import re
import sys
import time
import unicodedata
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
PASSAGES = ROOT / "infra" / "local" / "tribe-gazetteer-passages.json"
ARTIFACT = ROOT / "infra" / "local" / "toponym-pairings.json"
SEED = ROOT / "infra" / "local" / "seed-toponym-pairings.sql"
CACHE = ROOT / ".data" / "cache" / "toponym-search-resolutions-v2.json"
USER_AGENT = "dawha-toponym-pairings/1.0 (research artifact generator)"

SOURCE_ROW_ID = "b0000000-0000-0000-0000-00000000e002"  # Phase-A Wikidata sources row
MAX_PER_PASSAGE = 8
MIN_SIM = 0.80

# Demo places from db/seeds/001_demo.sql — the same four `build-pairings.py`
# hardcodes, plus the surface forms reviewers will see in Latin text.
PLACES: list[dict] = [
    {"id": "20000000-0000-0000-0000-000000000001", "name_ar": "الرياض",
     "forms": ["riyadh", "er riyadh", "ar riyadh", "al riyadh"]},
    {"id": "20000000-0000-0000-0000-000000000002", "name_ar": "الأحساء",
     "forms": ["al hasa", "al ahsa", "hasa", "lahsa", "hofuf", "hufuf", "el hassa", "al hassa"]},
    {"id": "20000000-0000-0000-0000-000000000003", "name_ar": "حجاز",
     "forms": ["hejaz", "hijaz", "hedjaz", "al hijaz"]},
    {"id": "20000000-0000-0000-0000-000000000004", "name_ar": "العلا",
     "forms": ["al ula", "alula", "al-`ula", "dadān", "dedan"]},
]

# Sentence-initial / structural words that must never read as toponyms.
STOP = {
    "the", "a", "an", "and", "or", "but", "if", "of", "in", "on", "at", "to",
    "for", "from", "with", "by", "as", "is", "are", "was", "were", "be", "been",
    "their", "there", "these", "those", "they", "them", "then", "than", "that",
    "this", "its", "it", "his", "her", "he", "she", "we", "our", "you", "your",
    "which", "who", "whom", "whose", "what", "when", "where", "while", "how",
    "all", "also", "any", "some", "such", "no", "not", "nor", "only", "other",
    "each", "both", "few", "more", "most", "much", "many", "several", "about",
    "above", "after", "again", "against", "among", "amongst", "around", "before",
    "behind", "below", "beneath", "beside", "between", "beyond", "during",
    "except", "inside", "into", "near", "off", "onto", "outside", "over",
    "past", "since", "through", "throughout", "till", "toward", "towards",
    "under", "until", "up", "upon", "within", "without", "according", "owing",
    "district", "province", "section", "sub-district", "part", "parts", "north",
    "south", "east", "west", "central", "southern", "northern", "eastern",
    "western", "first", "second", "third", "one", "two", "three", "four",
    "saudi", "arabia", "arabian", "turkey", "turkish", "ottoman", "arab",
    "bedouin", "tribe", "tribes", "capital", "government", "population",
    "estimated", "water", "waters", "date", "dates", "camels", "sheep", "goats",
    "name", "named", "place", "places", "sea", "gulf", "desert", "sand",
    "sands", "powerful", "great", "large", "small", "principal", "principally",
}

TOPONYM_RE = re.compile(r"\b[A-Z][a-zA-Z'’\-]{2,}(?:\s+[A-Z][a-zA-Z'’\-]{2,}){0,2}\b")

# Lorimer structural words / citation artifacts / tribe-name words: a bare
# tribal name is a SUBJECT here, not a place, and must not enter the place
# lane. (Compounds with a geographic head — Jabal Shammar, Wadi Sirhan —
# are kept: the geo-prefix rule below fires first.)
TRIBE_WORDS = {
    "harb", "shammar", "shammer", "mutair", "mutayr", "ataibah", "utaybah",
    "otaibah", "tamim", "khalid", "qahtan", "qatan", "hawazin", "mudar",
    "ajman", "bani", "banu", "harbi", "khalidi", "tamimi", "atiaibi",
    "qahaiaij", "shammari", "mutairi",
}
STRUCTURAL_WORDS = {
    "singular", "distribution", "see", "proper", "agent", "limit", "limits",
    "life", "origin", "position", "appearance", "climate", "[...,]", "ibn",
}

PLACE_HINT = re.compile(
    r"in (Saudi|Iraq|Oman|Yemen|Syria|Jordan|Kuwait|Qatar|the east|Arabia)|"
    r"town|village|oasis|district|region|wadi|river|mountain|valley|fort|"
    r"principality|province|port|peninsula|desert|settlement", re.I)
# The region gate: a toponym read from an Arabian gazetteer must resolve to an
# entity in (orelta' around) the region, else the Latin spelling matched some
# other language's word (Khaibar → Khyber Pass in Pakistan; Hail → Hailuoto in
# Finland). Country Q-ids verified 2026-11-15.
REGION_QIDS: set[str] = {
    "Q851",  # Saudi Arabia
    "Q796",  # Iraq
    "Q858",  # Syria
    "Q878",  # Egypt
    "Q881",  # Bahrain
    "Q846",  # Qatar
    "Q865",  # United Arab Emirates
    "Q810",  # Jordan
    "Q805",  # Yemen
    "Q819",  # Oman
    "Q833",  # Kuwait
    "Q822",  # Lebanon
    "Q794",  # Iran (for the Persian-side toponyms the entries cite)
    "Q219060",  # Palestine
}
NON_PLACE_HINT = re.compile(
    r"given name|surname|family name|researcher|football|politician|scholar|"
    r"writer|poet|tribe|tribal|clan|sect|religion|sunni|shia|muslim|islam|"
    r"dynasty|language|dialect", re.I)


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


def slug(text: str) -> str:
    return re.sub(r"[^a-z0-9]+", "-", text.lower()).strip("-")


def extract_candidates(passages: list[dict]) -> tuple[list[dict], list[dict]]:
    """Per-passage toponym candidates: dedup inside the passage, capped,
    multi-word first, order-of-appearance tiebreak. The sequence number is the
    ordering the Phase-B seed assigns (enumerate from 1), so occurrence ids in
    the lane line up with the passage rows in the database."""
    out: list[dict] = []
    dropped: list[dict] = []
    for seq, passage in enumerate(passages, start=1):
        found: dict[str, str] = {}
        for match in TOPONYM_RE.finditer(passage["text"]):
            phrase = re.sub(r"\s+", " ", match.group(0)).strip()
            words = phrase.split()
            lowered = [w.lower().strip("',’-") for w in words]
            # all-caps spillover from OCR'd section headers ("SHAMMAR", "TAMIM");
            # the 80% rule also catches mixed-glyph OCR of headers ("TAMlM")
            letters = [ch for ch in phrase if ch.isalpha()]
            if len(phrase) >= 4 and letters and sum(ch.isupper() for ch in letters) / len(letters) >= 0.8:
                dropped.append({"passage": seq, "phrase": phrase, "reason": "all-caps header spillover"})
                continue
            # collapsed duplicate headers ("HARB HARB Singular")
            if len(words) > 1 and words[0].lower() == words[1].lower():
                dropped.append({"passage": seq, "phrase": phrase, "reason": "duplicated header"})
                continue
            # structural words and bare tribe names are subjects, not places
            if any(w in STRUCTURAL_WORDS or w in TRIBE_WORDS for w in lowered):
                # keep compounds led by a geographic head ("Jabal Shammar", "Wadi Hamdh")
                if not re.match(r"^(Wadi|Jabal|Bahr|Bir|Qasr|Khabr|Khirba|Ain|Al)\b", phrase):
                    dropped.append({"passage": seq, "phrase": phrase,
                                    "reason": "structural word or bare tribe name"})
                    continue
            if any(w.lower() in STOP for w in words):
                dropped.append({"passage": seq, "phrase": phrase, "reason": "stopword or structural noun"})
                continue
            key = slug(phrase)
            if key in found:
                continue
            found[key] = phrase
        ranked = sorted(found.items(),
                        key=lambda kv: (0 if " " in kv[1] else 1, list(found).index(kv[0])))
        kept = 0
        for key, phrase in ranked:
            if kept >= MAX_PER_PASSAGE:
                dropped.append({"passage": seq, "phrase": phrase, "reason": f"per-passage cap {MAX_PER_PASSAGE}"})
                continue
            out.append({"passage": seq, "slug": key, "phrase": phrase})
            kept += 1
    return out, dropped


def http_json(url: str, tries: int = 4) -> dict:
    for attempt in range(tries):
        try:
            req = urllib.request.Request(url, headers={"User-Agent": USER_AGENT})
            with urllib.request.urlopen(req, timeout=45) as response:
                return json.load(response)
        except urllib.error.HTTPError as exc:
            if exc.code == 429 and attempt < tries - 1:
                wait = 20 * (attempt + 1)
                print(f"  (429; waiting {wait}s)", file=sys.stderr)
                time.sleep(wait)
                continue
            raise
    raise RuntimeError("rate limited")


def search_picks(phrase: str) -> list[dict]:
    """wbsearchentities (en), top-5. Returns the raw candidate list."""
    url = ("https://www.wikidata.org/w/api.php?" + urllib.parse.urlencode(
        {"action": "wbsearchentities", "search": phrase, "language": "en",
         "limit": "5", "format": "json"}))
    try:
        result = http_json(url)
    except Exception as exc:
        print(f"  search failed for {phrase!r}: {exc}", file=sys.stderr)
        return []
    return result.get("search", [])


def decide(picks: list[dict], details: dict[str, dict]) -> dict | None:
    """Geography-gated choice among the picks, using the SHARED detail table:

    - a hit whose country (P17) lies outside the region is rejected (the Latin
      spelling matched elsewhere in the world: Khaibar → Khyber Pass);
    - a human (P31=Q5) is rejected outright;
    - the best remaining hit: description reads like a place → accepted;
      neutral descriptions pass only if P17 is in-region.
    None is honest-unresolved, never a force-fit."""
    if not picks:
        return None

    def rank(qid: str) -> int:
        info = details.get(qid)
        if not info:
            return 9
        desc = info["description_en"]
        if info["p31"] == "Q5":
            return 9  # a human
        if info["p17"] and info["p17"] not in REGION_QIDS:
            return 9  # right string, wrong region
        non_place = bool(NON_PLACE_HINT.search(desc))
        place = bool(PLACE_HINT.search(desc))
        saudi = info["p17"] == "Q851"
        if non_place:
            return 8  # a tribe/sect/person-adjacent reading is never a place row
        if saudi and place:
            return 0
        if saudi:
            return 1
        if info["p17"] and place:
            return 2  # other in-region (Iran for the Persian-side toponyms)
        return 8  # no country and no geographic signal: too weak to land

    scored = sorted((rank(p["id"]), p["id"]) for p in picks)
    for r, qid in scored:
        if r in (0, 1, 2):
            info = details.get(qid, {})
            return {"qid": qid,
                    "label_en": info.get("label_en") or next(p["label"] for p in picks if p["id"] == qid),
                    "label_ar": info.get("label_ar", ""),
                    "description_en": info.get("description_en", "")}
    return None


def fetch_entity_details(qids: list[str]) -> dict[str, dict]:
    """Batched labels/descriptions/P17/P31 for candidate disambiguation."""
    out: dict[str, dict] = {}
    for start in range(0, len(qids), 20):
        chunk = qids[start:start + 20]
        url = ("https://www.wikidata.org/w/api.php?" + urllib.parse.urlencode(
            {"action": "wbgetentities", "ids": "|".join(chunk),
             "props": "labels|descriptions|claims", "languages": "en|ar",
             "format": "json"}))
        try:
            result = http_json(url)
        except Exception as exc:
            print(f"  detail fetch failed for {chunk[0]}…: {exc}", file=sys.stderr)
            continue
        for qid, entity in result.get("entities", {}).items():
            p17_vals = [c["mainsnak"]["datavalue"]["value"]["id"]
                        for c in entity.get("claims", {}).get("P17", [])
                        if c.get("mainsnak", {}).get("datavalue", {}).get("value", {}).get("id")]
            p31_vals = [c["mainsnak"]["datavalue"]["value"]["id"]
                        for c in entity.get("claims", {}).get("P31", [])
                        if c.get("mainsnak", {}).get("datavalue", {}).get("value", {}).get("id")]
            out[qid] = {
                "label_en": entity.get("labels", {}).get("en", {}).get("value", ""),
                "label_ar": entity.get("labels", {}).get("ar", {}).get("value", ""),
                "description_en": entity.get("descriptions", {}).get("en", {}).get("value", ""),
                "p17": p17_vals[0] if p17_vals else "",
                "p31": p31_vals[0] if p31_vals else "",
            }
        time.sleep(2)
    return out


def fetch_arabic_labels(qids: list[str]) -> dict[str, str]:
    """Arabic labels for the resolved Q-ids (chunked, rate-limit aware)."""
    out: dict[str, str] = {}
    for start in range(0, len(qids), 20):
        chunk = qids[start:start + 20]
        url = ("https://www.wikidata.org/w/api.php?" + urllib.parse.urlencode(
            {"action": "wbgetentities", "ids": "|".join(chunk),
             "props": "labels", "languages": "ar", "format": "json"}))
        try:
            result = http_json(url)
        except Exception as exc:
            print(f"  label fetch failed for {chunk[0]}…: {exc}", file=sys.stderr)
            continue
        for qid, entity in result.get("entities", {}).items():
            label = entity.get("labels", {}).get("ar", {}).get("value", "")
            if label:
                out[qid] = label
        time.sleep(2)
    return out


def match_place(labels: list[str]) -> tuple[str | None, float]:
    """Best demo-place match across Arabic labels and Latin forms."""
    best: tuple[str | None, float] = (None, 0.0)
    for place in PLACES:
        for label in labels:
            if not label:
                continue
            low = label.lower()
            ratio = 0.0
            for form in place["forms"]:
                if low == form:
                    ratio = max(ratio, 1.0)
                else:
                    ratio = max(ratio, difflib.SequenceMatcher(None, low, form).ratio())
            if ratio > best[1]:
                best = (place["id"], ratio)
    return best


def main() -> int:
    if not PASSAGES.exists():
        print("Phase-B artifact missing; run build-gazetteer-passages.py first", file=sys.stderr)
        return 1
    passages = json.loads(PASSAGES.read_text())["entries"]

    candidates, dropped = extract_candidates(passages)
    if not candidates:
        print("no toponym candidates extracted", file=sys.stderr)
        return 1

    unique = {c["slug"]: c["phrase"] for c in candidates}
    print(f"{len(candidates)} candidates ({len(unique)} unique) — resolving…", file=sys.stderr)

    resolved: dict[str, dict | None] = {}
    search_results: dict[str, list[dict]] = {}
    cached: dict = {}
    if CACHE.exists():
        cached = json.loads(CACHE.read_text())

    for i, (slug_key, phrase) in enumerate(sorted(unique.items()), start=1):
        if slug_key in cached:
            search_results[slug_key] = cached[slug_key]
            continue
        search_results[slug_key] = search_picks(phrase)
        if i % 10 == 0:
            print(f"  … {i}/{len(unique)}", file=sys.stderr)
        time.sleep(2)
    CACHE.parent.mkdir(parents=True, exist_ok=True)
    CACHE.write_text(json.dumps(search_results, ensure_ascii=False))
    print(f"  ({sum(1 for k in unique if k in cached)} cached / {len(unique)} total)", file=sys.stderr)

    all_ids = sorted({p["id"] for picks in search_results.values() for p in picks})
    all_details = fetch_entity_details(all_ids)

    for slug_key, phrase in sorted(unique.items()):
        resolved[slug_key] = decide(search_results.get(slug_key, []), all_details)

    # (Arabic labels arrive inside the shared detail table — no second fetch)

    rows: list[dict] = []
    for c in candidates:
        hit = resolved.get(c["slug"])
        if not hit:
            continue  # unresolved strings are recorded, not landed (see below)
        place_id, similarity = (match_place([hit["label_en"], hit.get("label_ar", ""), c["phrase"]])
                                if hit else (None, 0.0))
        if place_id is None or similarity < MIN_SIM:
            # the threshold is binding: a 0.44 string-similarity is not an
            # identity, it is noise. The candidate still lands — but WITHOUT a
            # place claim (the reviewer defers or rejects it there).
            place_id, similarity = None, 0.0
        rows.append({
            "source_id": SOURCE_ROW_ID,
            "external_key": hit["qid"],
            "occurrence_id": f"lorimer-v2:{c['passage']}:{c['slug']}",
            "our_place_id": place_id,
            "reading": f"قراءة لوريمر: '{c['phrase']}' — ويكي بيانات: {hit['label_en']}"
                       + (f" ({hit['label_ar']})" if hit.get("label_ar") else "")
                       + (f" — {hit['description_en']}" if hit["description_en"] else ""),
            "name_similarity": round(similarity, 4),
            "passage": c["passage"],
            "phrase": c["phrase"],
        })

    # rank within the source's queue by similarity, the 028-F convention
    for rank, row in enumerate(sorted(rows, key=lambda r: -r["name_similarity"]), start=1):
        row["rank"] = rank

    if not rows:
        print("no resolved candidates; refusing to write empty artifacts", file=sys.stderr)
        return 1

    # one entry per slug for the unresolved record
    unresolved: list[dict] = []
    seen_slugs: set[str] = set()
    for c in candidates:
        if c["slug"] in seen_slugs or resolved.get(c["slug"]):
            continue
        seen_slugs.add(c["slug"])
        unresolved.append({"slug": c["slug"], "phrase": c["phrase"]})

    artifact = {
        "meta": {
            "source": "toponym spans over infra/local/tribe-gazetteer-passages.json (Plan 030-B)",
            "lane": "existing external_place_candidates (0063), source row = Phase-A Wikidata (e002)",
            "license": "CC0 (Wikidata identity resolution); Lorimer text public domain (030-B)",
            "generator": "infra/local/build-toponym-pairings.py",
            "fetchedAt": __import__("datetime").date.today().isoformat(),
            "reviewerRule": "candidates with our_place_id NULL can only be rejected or deferred — "
                            "the pairing service refuses to accept into a vacuum (028-F)",
            "extraction": f"capitalized 1–3-word sequences, stoplist-filtered, {MAX_PER_PASSAGE}/passage cap, "
                          "multi-word ranked first",
            "resolution": "wbsearchentities → batched P17/P31 gate (region-checked, humans rejected); "
                          "no hit passes → recorded unresolved, never force-fitted",
        },
        "candidates": rows,
        "unresolved": unresolved,
        "dropped": dropped,
    }
    ARTIFACT.write_text(json.dumps(artifact, ensure_ascii=False, indent=2) + "\n")

    def sql_str(value: str) -> str:
        return "'" + value.replace("'", "''") + "'"

    lines = [
        "-- Plan 030-C: toponym candidates over the Lorimer passages.",
        "-- Generated by infra/local/build-toponym-pairings.py; do not edit by hand.",
        f"-- {len(rows)} resolved candidates into the existing pairing lane.",
        "-- source_id = the Phase-A Wikidata row (e002): the external key IS a Q-id.",
        "-- Lorimer provenance travels inside occurrence_id + the reading text.",
        "-- Idempotent on (source_id, occurrence_id).",
        "",
    ]
    for row in rows:
        lines.append(
            "INSERT INTO external_place_candidates "
            "(source_id, external_key, occurrence_id, our_place_id, normalized_candidates, name_similarity, rank, status)")
        lines.append("VALUES ("
            + sql_str(row["source_id"]) + ", "
            + sql_str(row["external_key"]) + ", "
            + sql_str(row["occurrence_id"]) + ", "
            + (sql_str(row["our_place_id"]) if row["our_place_id"] else "NULL") + ", "
            + sql_str(row["reading"]) + ", "
            + str(row["name_similarity"]) + ", "
            + str(row["rank"]) + ", 'suggested')")
        lines.append("ON CONFLICT (source_id, occurrence_id) DO NOTHING;")
        lines.append("")
    SEED.write_text("\n".join(lines))

    print(f"wrote {ARTIFACT.name}: {len(rows)} candidates, "
          f"{len(artifact['unresolved'])} unresolved, {len(dropped)} dropped")
    print(f"wrote {SEED.name}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
