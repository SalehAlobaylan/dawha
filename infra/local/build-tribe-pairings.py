#!/usr/bin/env python3
"""Plan 031-E: المِسلك — offline tribe-duplicate candidates.

The machine proposes pairings; reviewers adjudicate. Nothing here calls the
network at review time: the candidates are written once, here, as `suggested`
rows, and the lane only ever displays them as proposals. The lane's effects
belong to the reviewer: link-alias, canonical pointer, or reject — and no
statement anywhere in this lane deletes or updates a `tribes` row.

1. Read the committed tribe subset (`infra/local/tribe-seeds.json`): each
   tribe's canonical name and its verified aliases (the forms the dictionary
   actually carries).
2. For every ordered pair of different tribes, compare their forms with a
   character-trigram similarity (the same proxy `build-pairings.py` uses for
   the pg_trgm reading: difflib SequenceMatcher·0.5 + shared-trigram
   Jaccard·0.5), taking the best form pair of the two tribes as the pair's
   own receipt («пावne both matched strings ride the row verbatim»).
3. Pairs at or above the gate are emitted as `suggested` candidates; the
   ordered pair is normalized so (a, b) appears once (`a < b` by canonical
   name), and rows are deterministic: same artifact, same candidates.
4. Emit `seed-tribe-pairings.sql`: `INSERT … ON CONFLICT DO NOTHING`, so a
   re-run cannot overwrite a decision, and the artifact records the gate and
   each candidate's own components.

Run:    python3 infra/local/build-tribe-pairings.py
Writes: infra/local/tribe-pairings.json + infra/local/seed-tribe-pairings.sql
"""

from __future__ import annotations

import json
import sys
from datetime import date
from difflib import SequenceMatcher
from pathlib import Path

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[1]
SEEDS = HERE / "tribe-seeds.json"
ARTIFACT = HERE / "tribe-pairings.json"
SEED = HERE / "seed-tribe-pairings.sql"

SIMILARITY_GATE = 0.55


def normalize(value: str) -> str:
    return " ".join(value.split())


def trigrams(value: str) -> set[str]:
    padded = f"  {normalize(value)} "
    return {padded[i : i + 3] for i in range(len(padded) - 2)}


def similarity(left: str, right: str) -> float:
    """The pg_trgm proxy the 028-F builder standardized: half ordering-
    matcher, half trigram Jaccard. A component, not a verdict."""
    left, right = normalize(left), normalize(right)
    if not left or not right:
        return 0.0
    if left == right:
        return 1.0
    matcher = SequenceMatcher(None, left, right)
    left_grams, right_grams = trigrams(left), trigrams(right)
    jaccard = len(left_grams & right_grams) / len(left_grams | right_grams)
    return round(matcher.ratio() * 0.5 + jaccard * 0.5, 4)


def best_pair(tribe_a: dict, tribe_b: dict) -> tuple[float, str, str] | None:
    """The strongest form pair of two tribes, as candidates' own receipts."""
    forms_a = [tribe_a["canonical_name_ar"], *[alias["value_ar"] for alias in tribe_a["aliases"]]]
    forms_b = [tribe_b["canonical_name_ar"], *[alias["value_ar"] for alias in tribe_b["aliases"]]]
    best: tuple[float, str, str] | None = None
    for form_a in forms_a:
        for form_b in forms_b:
            score = similarity(form_a, form_b)
            if best is None or score > best[0]:
                best = (score, form_a, form_b)
    return best


def main() -> int:
    seeds = json.loads(SEEDS.read_text(encoding="utf-8"))
    tribes = seeds["tribes"]

    candidates = []
    for index, tribe_a in enumerate(tribes):
        for tribe_b in tribes[index + 1 :]:
            pair = best_pair(tribe_a, tribe_b)
            if pair is None:
                continue
            score, matched_a, matched_b = pair
            if score < SIMILARITY_GATE:
                continue
            # The ordered pair is canonical by name: (a, b) so (b, a) can never
            # fork the same dispute into two rows.
            first, second = sorted((tribe_a, tribe_b), key=lambda tribe: tribe["canonical_name_ar"])
            matched_first = matched_a if first["tribe_uuid"] == tribe_a["tribe_uuid"] else matched_b
            matched_second = matched_b if matched_first is matched_a else matched_a
            candidates.append({
                "tribeAId": first["tribe_uuid"],
                "tribeBId": second["tribe_uuid"],
                "tribeA": first["canonical_name_ar"],
                "tribeB": second["canonical_name_ar"],
                "matchedA": matched_first,
                "matchedB": matched_second,
                "nameSimilarity": score,
            })

    CRC = sorted(candidates, key=lambda candidate: (candidate["tribeA"], candidate["tribeB"]))

    ARTIFACT.write_text(json.dumps({
        "meta": {
            "generator": "infra/local/build-tribe-pairings.py",
            "generatedAt": date.today().isoformat(),
            "similarityGate": SIMILARITY_GATE,
            "similarityProxy": "difflib SequenceMatcher·0.5 + shared-trigram Jaccard·0.5 (the 028-F proxy for pg_trgm similarity)",
            "pairOrderRule": "the pair's row stores the two tribes sorted by canonical name, so a pair is one row forever",
            "laneRule": "no statement of this lane deletes or updates a tribes row; the effects are an alias insert, a pointer relationship, or a rejection, and the decisions are append-only (plan 005's deferral stands)",
            "certaintyRule": "a candidate is a measurable proposal; the reviewer's own answer lands in the decision history beside it",
        },
        "candidates": CRC,
    }, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(f"wrote {ARTIFACT.relative_to(ROOT)} ({len(candidates)} candidates over {SIMILARITY_GATE})")

    write_seed(candidates)
    return 0


def write_seed(candidates: list[dict]) -> None:
    lines = [
        "-- Plan 031-E: tribe-duplicate candidates (المِسلك).",
        "-- Generated by infra/local/build-tribe-pairings.py; do not edit.",
        f"-- Generated: {date.today().isoformat()}",
        "--",
        "-- ON CONFLICT DO NOTHING: a re-run can never overwrite a reviewer's",
        "-- decision, and the lane never writes a tribes row.",
        "",
    ]
    for candidate in candidates:
        lines.append("INSERT INTO tribe_duplicate_candidates")
        lines.append("  (tribe_a_id, tribe_b_id, matched_a, matched_b, name_similarity, status)")
        lines.append("VALUES")
        lines.append(f"  ('{candidate['tribeAId']}', '{candidate['tribeBId']}', "
                     f"'{candidate['matchedA'].replace(chr(39), chr(39) * 2)}', "
                     f"'{candidate['matchedB'].replace(chr(39), chr(39) * 2)}', "
                     f"{candidate['nameSimilarity']}, 'suggested')")
        lines.append("ON CONFLICT (tribe_a_id, tribe_b_id) DO NOTHING;")
        lines.append("")
    SEED.write_text("\n".join(lines) + "\n", encoding="utf-8")
    print(f"wrote {SEED.relative_to(ROOT)} ({len(candidates)} candidate rows)")


if __name__ == "__main__":
    sys.exit(main())
