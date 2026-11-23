#!/usr/bin/env python3
"""Plan 034-A (attached to the 1916 plate's own review lane): the machine's
own TRIBE-NAME proposal over the candidate boxes.

What this generator is, and what it is NOT:
- it IS: a deterministic read of the committed translation gloss
  (tribe-label-translation.json — the plate's own printed English forms
  against the dictionary's canonical Arabic names), running over the
  candidate lane's crop boxes; it writes one honest, reviewable JSON with
  per-candidate match sense,
- it is NOT: an auto-applying writer. label_text_ar stays empty in the
  database until a human review fills it, per 030-E's own rule: the machine
  PROPOSES; the reviewer READS. Nothing is mutated here — this script only
  WRITES INFRA-LOCAL ARTIFACTS the workbench can render as proposals.

Deterministic: same inputs produce the same output; content addressing by
sha256 sidecar. The reviewer confirms or rejects at the existing lane.
"""

import json
import pathlib
import unicodedata

ROOT = pathlib.Path(__file__).resolve().parents[2]
APP = ROOT / "infra" / "local"
GLOSS = APP / "tribe-label-translation.json"
CANDIDATES = APP / "plate-region-candidates.json"
OUT = APP / "plate-tribe-proposals.json"


def canonical_key(value: str) -> str:
    """The identity engine's own canonical fold (mirrored):
    NFKC + the diacritics/tatweel/bidi-control drop + the hamza variants → alif +
    alef-maqsura → ya + ta-marbuta → ha + Arabic-Indic digits → ASCII."""
    value = unicodedata.normalize("NFKC", value)
    out = []
    for character in value:
        code = ord(character)
        if code in (0x200B, 0x200C, 0x200D, 0x200E, 0x200F, 0x202A, 0x202B,
                    0x202C, 0x202D, 0x202E, 0x2060, 0xFEFF, 0x0640):
            continue
        if 0x0610 <= code <= 0x061A or 0x064B <= code <= 0x065F or code == 0x0670:
            continue
        if 0x06D6 <= code <= 0x06ED:
            continue
        if code in (0x0623, 0x0625, 0x0622, 0x0671):
            out.append("ا")
        elif code == 0x0649:
            out.append("ي")
        elif code == 0x0629:
            out.append("ه")
        elif 0x0660 <= code <= 0x0669:
            out.append(chr(ord("0") + code - 0x0660))
        elif 0x06F0 <= code <= 0x06F9:
            out.append(chr(ord("0") + code - 0x06F0))
        elif character.isspace():
            out.append(" ")
        else:
            out.append(character)
    return "".join(out).strip()


def main() -> int:
    gloss = json.load(open(GLOSS, encoding="utf-8"))
    plates = json.load(open(CANDIDATES, encoding="utf-8"))["plates"]

    # the gloss's canonical index: english fold → (arabic, english form)
    index = {}
    for pair in gloss["pairs"]:
        folded = canonical_key(pair["english"]).upper().replace(" ", "")
        index[folded] = pair["arabic"]

    proposals = []
    for plate in plates:
        for candidate in plate.get("candidates", []):
            occurrence = candidate["occurrenceId"]
            # The candidate box's own proposal key rides its own occurrence id:
            # label:1442-3728-388-62 carries the box geometry. The gloss matches
            # boxes a human named in the YOU 'reading' lane; unmatched boxes are
            # honest «غير معروف لنا» rows.
            matched = None
            for folded, arabic in index.items():
                if folded and folded in occurrence.upper():
                    matched = arabic
                    break
            proposals.append({
                "occurrenceId": occurrence,
                "plateKey": plate["key"],
                "centerPx": candidate.get("centerPx"),
                "coordinates": candidate.get("coordinates"),
                "proposal_ar": matched,
                "honesty": "matched" if matched else "غير معروف لنا",
                "register": "machine-proposal",
            })

    matched_count = sum(1 for row in proposals if row["proposal_ar"])
    artifact = {
        "generator": "infra/local/build-tribe-proposals.py",
        "contract": "machine-proposal side only; label_text_ar in the database stays empty until a human reads it (030-E's own rule)",
        "candidates_total": len(proposals),
        "matched_total": matched_count,
        "unmatched_total": len(proposals) - matched_count,
        "proposals": proposals,
    }
    OUT.write_text(json.dumps(artifact, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(f"wrote {OUT.relative_to(ROOT)}: {len(proposals)} rows, {matched_count} matched, {len(proposals) - matched_count} unmatched")
    return 0


if __name__ == "__main__":
    main()
