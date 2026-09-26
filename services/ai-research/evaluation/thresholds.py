"""Every AI evaluation threshold, in one place, under one version.

The thresholds used to live inside each group's evaluation function, which meant
that answering "what is this gate actually holding the provider to?" meant
reading five functions and remembering which one had been changed last. They also
meant that a threshold edit looked like any other code edit, and that the report
carried no way to tell one review of the numbers from the next.

So they live here, together, with a version and the date they were last reviewed.
Three rules, and the report depends on all three:

1. A threshold is the measured baseline, not a target. Passing these numbers
   means the provider has not got worse. It does not mean the provider is good,
   and no document in this repository may say otherwise on the strength of a
   green report.
2. Changing a threshold is a version bump. Bump THRESHOLDS_VERSION in the same
   commit, and say in the message which group moved and why. A threshold that
   moves silently is indistinguishable from a regression being accepted.
3. A threshold is 1.0 where the property is a safety property rather than a
   quality one. `unknown_left_unresolved` is 1.0 because resolving a name nobody
   recorded asserts a false identity; `excerpt_fidelity` and `source_id_fidelity`
   are 1.0 because a citation whose text or whose source id is not real is a
   fabricated source. Where a safety property is NOT being met, the threshold sits
   at the measured value and the shortfall is recorded in KNOWN_DEFECTS - never at
   1.0, which would be a gate that fails on arrival and teaches people to ignore
   gates.

The values below are the ones measured on commit 4e54553 with the fixtures in
this directory. The measured value of every metric is in the report next to its
threshold, so a reader can see the gap rather than infer it.
"""

from __future__ import annotations

from typing import Any

# THRESHOLDS_VERSION identifies this set of numbers. It goes into the report, so
# two reports can be compared only when their thresholds are comparable, and a
# threshold change is visible in a diff of report files.
THRESHOLDS_VERSION = "2026-09-26.1"

# THRESHOLDS_REVIEWED_ON is the day a human read the measured values against the
# fixtures and agreed to hold them. It is a date and not a version because it does
# not change the gate; it records when the judgement was made.
THRESHOLDS_REVIEWED_ON = "2026-09-26"

THRESHOLDS: dict[str, dict[str, float]] = {
    # Routing decides what an operation costs, so a drop here is a cost change
    # before it is a quality change. Thresholds predate the other groups.
    "routing": {
        "route_accuracy": 0.8,
        "query_type_accuracy": 0.75,
    },
    # The reranker is token overlap, so precision at 1 is modest by
    # construction. Recall and MRR are higher because almost every reviewed case
    # has one obviously relevant passage among three.
    "retrieval": {
        "precision_at_1": 0.5,
        "recall": 0.7,
        "mrr": 0.4,
    },
    # Entity precision is 0.33 measured, against a 0.2 floor, because the
    # provider proposes over-long spans and the containment rule charges it for
    # that. The number is a baseline with a known weakness, not a target; see
    # the note in evaluate_extraction.
    "extraction": {
        "claim_count_accuracy": 0.8,
        "entity_precision": 0.2,
        "entity_recall": 0.9,
    },
    # A name nobody recorded must resolve to nothing. That threshold is 1.0 on
    # purpose and is the one number in this file that must never be lowered.
    "resolution": {
        "top1_accuracy": 0.8,
        "evidence_label_accuracy": 0.8,
        "unknown_left_unresolved": 1.0,
    },
    # Two of the six reviewed contradiction cases are known misses, recorded in
    # the fixtures: a pair about different people is proposed, and a migration
    # disagreement is not. verdict_accuracy sits at the measured 0.6667 so a
    # change in either direction is visible.
    "contradiction": {
        "pair_precision": 0.4,
        "pair_recall": 0.5,
        "verdict_accuracy": 0.6667,
    },
    # Citation grounding, added by plan 009. Two of these are safety properties
    # rather than quality ones and are held at 1.0: a citation to a source that
    # was never sent, or an answer presented as grounded with nothing behind it.
    #
    # unsupported_refusal sits at 0.5 because that is what the fixtures measure,
    # and the number is a defect rather than an achievement - see KNOWN_DEFECTS.
    # It is the one threshold in this file that a reader should expect to move
    # upward, and moving it up means fixing the provider, not moving the number.
    "citations": {
        "citation_precision": 0.5,
        "support_recall": 0.7,
        "excerpt_fidelity": 1.0,
        "source_id_fidelity": 1.0,
        "unsupported_refusal": 0.5,
        "grounded_answer_rate": 1.0,
    },
}

# KNOWN_DEFECTS names every metric whose measured value is a known weakness rather
# than a healthy baseline.
#
# It exists because "all groups hold their baselines" is a sentence a reader will
# quote, and a quoted sentence that hides a known defect is worse than a red
# build. Each entry says what the defect is, which code causes it, what it costs
# the product, and what would change the number. A metric listed here still holds
# its threshold - the gate is there to catch a change, not to record a verdict on
# code this plan is not allowed to change - but the report prints these next to
# the pass, and docs/phase-status.md lists the ones that are real blockers.
#
# A defect leaves this list when its metric moves above its threshold for a
# reason other than the threshold moving. Raising the threshold to meet a defect
# is the one thing that must never happen: it converts a measurement into a
# description.
KNOWN_DEFECTS: dict[str, dict[str, str]] = {
    "citations.unsupported_refusal": {
        "summary": (
            "A question the sources do not answer can still come back with a citation, "
            "because the reranker counts any shared token as grounding - including a "
            "preposition."
        ),
        "cause": (
            "app/main.py research_query keeps every context whose token set intersects the "
            "query's, with no minimum overlap; citation fixture cit-003 shares only the "
            "token 'في' with its question and is cited anyway."
        ),
        "consequence": (
            "A reader can be shown a source that does not contain the answer. For a product "
            "whose premise is that every claim is traceable to a source, a citation that "
            "does not support anything is the failure mode that matters most."
        ),
        "fix": (
            "Require a minimum content-token overlap, or a content term, before a context "
            "becomes a citation. That is a change to app/main.py and is deliberately not made "
            "here: this plan adds measurement, and a gate that was made green by editing the "
            "thing it measures is not a gate."
        ),
    },
    "extraction.entity_precision": {
        "summary": "The extractor proposes spans that are much longer than the entity.",
        "cause": "app/main.py extract_entities matches any run of four or more Arabic characters.",
        "consequence": (
            "A reviewer opens a candidate panel where a third of the proposals are over-long."
        ),
        "fix": (
            "Bound the proposed span to the name, and score exact spans rather than containment."
        ),
    },
    "contradiction.pair_precision": {
        "summary": "Pairs about different people are proposed as contradiction candidates.",
        "cause": (
            "app/main.py analyze_contradiction compares wording and never checks that the two "
            "statements are about the same subject."
        ),
        "consequence": "The review queue carries pairs a reviewer has to open to dismiss.",
        "fix": "Require a shared subject before proposing a pair.",
    },
    "contradiction.verdict_accuracy": {
        "summary": (
            "A migration disagreement is missed, because the detector keys on parentage wording."
        ),
        "cause": (
            "app/main.py _CONTRADICTION_TERMS has no migration vocabulary; fixture ctr-006 "
            "records the miss."
        ),
        "consequence": "A real disagreement about a place passes unnoticed.",
        "fix": (
            "Add migration and place vocabulary to the contradiction terms and extend the fixtures."
        ),
    },
}

# GROUPS is the order the report prints them in. A reader who opens the report to
# answer one question should not have to know which function produced which
# section.
GROUPS = (
    "routing",
    "retrieval",
    "extraction",
    "resolution",
    "contradiction",
    "citations",
)


def thresholds_for(group: str) -> dict[str, float]:
    """The thresholds for one group, or an empty mapping for a group with none.

    An unknown group returning nothing rather than raising is deliberate: the
    report builder asks every configured group for its thresholds, and a group
    added without thresholds should be reported as unthresholded rather than
    crash the run that would have told somebody about it.
    """
    return dict(THRESHOLDS.get(group, {}))


def describe() -> dict[str, Any]:
    """The threshold metadata the report carries."""
    return {
        "version": THRESHOLDS_VERSION,
        "reviewed_on": THRESHOLDS_REVIEWED_ON,
        "policy": (
            "every threshold is the measured baseline rather than a target: holding "
            "them means the provider has not got worse, not that it is good. Raising "
            "a threshold to the measured value is a version bump, never a silent edit."
        ),
        "safety_floors": [
            "resolution.unknown_left_unresolved",
            "citations.excerpt_fidelity",
            "citations.source_id_fidelity",
            "citations.grounded_answer_rate",
        ],
    }


def defect_for(group: str, metric: str) -> dict[str, str] | None:
    """The recorded defect for one metric, if there is one."""
    return KNOWN_DEFECTS.get(f"{group}.{metric}")


def defects_for(group: str, metrics: dict[str, Any]) -> list[dict[str, Any]]:
    """Every recorded defect in one group, with the measured value beside it."""
    found = []
    for metric in sorted(metrics):
        defect = defect_for(group, metric)
        if defect is None:
            continue
        found.append({"metric": metric, "measured": metrics[metric], **defect})
    return found
