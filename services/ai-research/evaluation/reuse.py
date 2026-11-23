"""The reuse-detection group (plan 034-D): the pair classifier the reuse
engine already runs, measured against hand-reviewed Arabic pairs.

A reuse decision is DETERMINISTIC here on purpose: the plan's own stipulation
is "no LLM in this phase", and the engine the atlas deploys is the same
shingle-overlap machine this group measures — the same threshold, the same
shingle size, the same three-key normalization. A case is a pair of passages
and the reviewer's own verdict:

    identical    — a lineage the engine SHOULD link (a near-verbatim copy);
    paraphrased  — same substance, different words: the engine MUST NOT link
                   it (linking a paraphrase would inflate corroboration with
                   one author's own rewording — exactly the independence
                   correction the engine exists to make);
    distinct     — different subjects entirely.

The metrics are ordinary precision/recall/specificity over the reviewer's
verdicts, with the mislabeled cases KEPT (the coherence the rest of the
harness keeps: a fixture set curated against the engine would be worthless).
"""

from __future__ import annotations

import json
import unicodedata
from pathlib import Path
from typing import Any

from .thresholds import thresholds_for

HERE = Path(__file__).resolve().parent

REUSE_SHINGLE_SIZE = 5
REUSE_THRESHOLD = 0.55

# The three labels the reviewer's own verdicts use.
REUSE_LABELS = ("identical", "paraphrased", "distinct")


def load_cases(name: str) -> list[dict[str, Any]]:
    with open(HERE / name, encoding="utf-8") as handle:
        return [json.loads(line) for line in handle if line.strip()]


def canonical_key(value: str) -> str:
    """The reuse engine's own canonical key (the mirror of the Go identity
    engine's normalization)."""
    value = unicodedata.normalize("NFKC", value)
    out = []
    for character in value:
        code = ord(character)
        # Bidi controls + tatweel + tashkeel + Quranic annotations drop.
        if code in (0x200B, 0x200C, 0x200D, 0x200E, 0x200F, 0x202A, 0x202B,
                    0x202C, 0x202D, 0x202E, 0x2060, 0xFEFF, 0x0640):
            continue
        if 0x0610 <= code <= 0x061A or 0x064B <= code <= 0x065F or code == 0x0670:
            continue
        if 0x06D6 <= code <= 0x06ED:
            continue
        if code in (0x0623, 0x0625, 0x0622, 0x0671):
            out.append("\u0627")
        elif code == 0x0649:
            out.append("\u064A")
        elif code == 0x0629:
            out.append("\u0647")
        elif 0x0660 <= code <= 0x0669:
            out.append(chr(ord("0") + code - 0x0660))
        elif 0x06F0 <= code <= 0x06F9:
            out.append(chr(ord("0") + code - 0x06F0))
        elif character.isspace():
            out.append(" ")
        else:
            out.append(character)
    return " ".join("".join(out).split())


def shingles(canonical_text: str, size: int = REUSE_SHINGLE_SIZE) -> set[str]:
    """Word 5-grams over the canonical text; a passage shorter than the
    window contributes its whole (canonical) text as one shingle."""
    tokens = canonical_text.split()
    if not tokens:
        return set()
    if len(tokens) < size:
        return {" ".join(tokens)}
    return {" ".join(tokens[index:index + size]) for index in range(len(tokens) - size + 1)}


def jaccard(left: set[str], right: set[str]) -> float:
    if not left and not right:
        return 0.0
    union = left | right
    if not union:
        return 0.0
    return len(left & right) / len(union)


def predicts_link(left: str, right: str) -> bool:
    """The engine's own verdict: link iff the Jaccard crosses the threshold."""
    return jaccard(shingles(canonical_key(left)), shingles(canonical_key(right))) >= REUSE_THRESHOLD


def evaluate_reuse() -> dict[str, Any]:
    """The reuse group's read: precision/recall/specificity over the
    reviewer's own verdicts."""
    cases = load_cases("reuse_cases.jsonl")
    true_positives = 0
    false_positives = 0
    false_negatives = 0
    true_negatives = 0
    disagreements: list[dict[str, Any]] = []

    for case in cases:
        label = case["label"]
        predicted = predicts_link(case["left"], case["right"])
        expected = label == "identical"
        if predicted and expected:
            true_positives += 1
        elif predicted and not expected:
            false_positives += 1
            disagreements.append({"id": case["id"], "label": label, "predicted": "link"})
        elif not predicted and expected:
            false_negatives += 1
            disagreements.append({"id": case["id"], "label": label, "predicted": "no-link"})
        else:
            true_negatives += 1

    linked_total = true_positives + false_positives
    expected_total = true_positives + false_negatives
    refused_total = true_negatives + false_positives
    precision = true_positives / linked_total if linked_total else 0.0
    recall = true_positives / expected_total if expected_total else 0.0
    specificity = true_negatives / refused_total if refused_total else 0.0

    metrics = {
        "cases": len(cases),
        "precision": round(precision, 4),
        "recall": round(recall, 4),
        "distinct_refusal_rate": round(specificity, 4),
    }
    return {
        "metrics": metrics,
        "thresholds": thresholds_for("reuse"),
        "provenance": {
            "labels": (
                "internally authored (plan 034 baseline); the verdicts are the reviewer's "
                "own, and the mislabeled case(s) stay in the file"
            ),
            "language": "Arabic only (the corpus's own language); the classical register",
            "by_origin": {"internally_authored": 10, "external": 0},
            "available_real_query_set": False,
            "derived_from_real_query_set": False,
            "derived_from_real_query_set_note": (
                "the pairwise labels are authored for this baseline, not mined from a "
                "real query log; distinct_refusal_rate's 1.0 threshold is a safety floor "
                "rather than a recall ceiling"
            ),
            "limitation": (
                "ten hand-authored pairs: precision/recall over them measure AGREEMENT "
                "with one reviewer's labels, not engine quality over a corpus; the real "
                "corpus's own clusters live in the committed artifacts "
                "(infra/local/reuse/)"
            ),
        },
        "coverage": {
            "identical": sum(1 for case in cases if case["label"] == "identical"),
            "paraphrased": sum(1 for case in cases if case["label"] == "paraphrased"),
            "distinct": sum(1 for case in cases if case["label"] == "distinct"),
        },
        "disagreements": disagreements,
        "notes": [
            (
                "The verdict machine is DETERMINISTIC (shingle 5 + Jaccard at "
                f"{REUSE_THRESHOLD}) — the same shape the offline engine runs; the plan's "
                "own no-LLM-in-this-phase stipulation is why no provider is in this loop."
            ),
            (
                "precision/recall are agreement with internally authored labels over a "
                "SMALL hand-written set; they are not accuracy over a real corpus and must "
                "not be quoted as one. The committed corpus artifacts "
                "(infra/local/reuse/) are where the real corpus's own clusters live."
            ),
            (
                "The paraphrase cases' refusal is the independence correction's own shape: "
                "linking a paraphrase would DOUBLE-COUNT one author's own rewording as "
                "corroboration — exactly what the engine exists to stop."
            ),
        ],
    }
