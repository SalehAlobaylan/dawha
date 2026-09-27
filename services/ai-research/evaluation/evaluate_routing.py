"""The routing group: route agreement over a labelled case set, and what the labels are.

`make ai-eval` prints this inside the routing group of the machine-readable report,
and this module is also runnable on its own (`python -m evaluation.evaluate_routing`).

Three things this file exists to say, in the JSON rather than beside the JSON:

1. **The labels are internally authored.** Every case is hand-written by somebody
   who also wrote, or could read, the routing code. This repository has no
   production traffic, so there is no query set to derive cases from, and the
   report says so in a field a reader sees without opening a document. A reader who
   takes `route_accuracy` as confidence in routing real questions has been told
   otherwise by the artifact itself.
2. **Per-case provenance is required, not optional.** A case without a `case_id`,
   an `origin` and a `labelled_by` fails the run. The alternative is a fixture set
   that grows one case at a time and silently mixes a reviewed set with something
   nobody reviewed.
3. **Cases that expose a mis-route are kept.** The set is not curated against the
   provider. Two cases in it are expected to fail and say so in their own text; they
   are in the set because a curated set would have removed exactly the evidence
   this phase needs.

The expected route, query type and reason code of a case are the LABEL. The
provider's answer is the MEASUREMENT. Nothing in this file compares them in the
other direction.
"""

from __future__ import annotations

import json
from collections import defaultdict
from pathlib import Path

from app.main import DeterministicProvider, RoutingRequest, routing_decision

ROUTES = ("ignore", "cheap", "deep")

# PROVENANCE_FIELDS are the fields every case must carry. A missing one is an error
# rather than a default, because a default is indistinguishable from a case somebody
# thought about and decided needed no note.
PROVENANCE_FIELDS = ("case_id", "origin", "labelled_by", "written_to_exercise")

# KNOWN_ORIGINS is the closed set. "hand_written" is the only value available in this
# repository today, and it is named rather than implied, so a future case derived
# from a real query set has to add its origin here and say what the queries were.
KNOWN_ORIGINS = ("hand_written",)

# LABEL_LIMITATION is the sentence that travels inside the report. It is one string
# on purpose: a limitation split across three fields can be quoted out of one of
# them, and "all groups hold their baselines" is exactly the sentence that will be
# quoted.
LABEL_LIMITATION = (
    "Every case in this set was hand-written by an author of this repository, and "
    "the expected route, query type and reason code in each case are that author's "
    "labels rather than a reviewed judgement about a real question. route_accuracy, "
    "query_type_accuracy and reason_code_accuracy are therefore agreement with "
    "internally authored labels. They are NOT accuracy on real questions, they are "
    "NOT historical accuracy, and they must not be quoted as confidence in any claim "
    "about a real user. This repository has no production traffic, so no case could "
    "be derived from a real query set; the report says available_real_query_set: false "
    "rather than leaving the absence to be noticed."
)


def load_cases() -> list[dict[str, object]]:
    """Every labelled case, with its provenance required.

    Raises on a case that is missing provenance, on an unknown origin, on a
    duplicate case id, and on a case claiming an origin this repository cannot
    honestly produce. Failing here is the point: a fixture set whose provenance has
    rotted is the failure this function exists to prevent.
    """
    cases_path = Path(__file__).with_name("routing_cases.jsonl")
    cases = []
    seen_ids: set[str] = set()
    for number, line in enumerate(cases_path.read_text(encoding="utf-8").splitlines(), 1):
        if not line.strip():
            continue
        case = json.loads(line)
        missing = [field for field in PROVENANCE_FIELDS if not case.get(field)]
        if missing:
            raise ValueError(
                f"routing_cases.jsonl line {number} is missing provenance field(s) "
                f"{', '.join(missing)}; every case must say where it came from and who "
                "labelled it"
            )
        if case["origin"] not in KNOWN_ORIGINS:
            raise ValueError(
                f"routing case {case['case_id']} claims origin {case['origin']!r}, which "
                f"is not one of {KNOWN_ORIGINS}. A case derived from a real query set has "
                "to carry the query it was derived from and say so here; it cannot be "
                "recorded as though it were a hand-written example."
            )
        if case["case_id"] in seen_ids:
            raise ValueError(f"routing case id {case['case_id']!r} appears twice")
        seen_ids.add(case["case_id"])
        cases.append(case)
    if not cases:
        raise ValueError(
            "routing_cases.jsonl is empty; an empty set would report a perfect accuracy"
        )
    return cases


def provenance(cases: list[dict[str, object]]) -> dict[str, object]:
    """The provenance of the set, in the shape the report carries.

    This is the block a reader of the JSON needs and the only place they should
    have to look: the counts, whether a real query set exists, and the limitation
    in a sentence that cannot be quoted without the rest.
    """
    by_origin: dict[str, int] = defaultdict(int)
    by_labeller: dict[str, int] = defaultdict(int)
    for case in cases:
        by_origin[str(case["origin"])] += 1
        by_labeller[str(case["labelled_by"])] += 1
    return {
        "labels": "internally_authored",
        "limitation": LABEL_LIMITATION,
        "cases": len(cases),
        "by_origin": dict(sorted(by_origin.items())),
        "by_labeller": dict(sorted(by_labeller.items())),
        "derived_from_real_query_set": sum(
            count for origin, count in by_origin.items() if origin != "hand_written"
        ),
        "available_real_query_set": False,
        "why_no_real_query_set": (
            "This repository has no production traffic and no configured model provider, so "
            "there is no query log to derive cases from. Labelling from a real query set is "
            "the only way these numbers become a claim about real questions, and it needs "
            "data this repository does not have. docs/cost-measurement.md names who has to "
            "collect it."
        ),
        "what_would_change_this": (
            "A de-identified export of real research questions with a reviewed route, query "
            "type and reason code per question, reviewed by somebody who did not write the "
            "routing code. Until then this set is a specification of what the routing code "
            "was supposed to do, and the agreement number says the code still does it."
        ),
    }


def evaluate() -> dict[str, object]:
    provider = DeterministicProvider()
    cases = load_cases()
    route_correct = 0
    type_correct = 0
    reason_correct = 0
    skipped = 0
    confusion: dict[str, dict[str, int]] = defaultdict(lambda: defaultdict(int))
    misroutes: list[dict[str, object]] = []
    for case in cases:
        request = RoutingRequest(
            text=case["text"],
            context=case.get("context"),
            operation=case["operation"],
            source_count=case["source_count"],
        )
        # A case marked fallback is routed through the FALLBACK path, which is the
        # decision this repository makes when the route call is unavailable. The
        # original fifteen never exercised it, so the coverage number below says so.
        if case.get("fallback"):
            actual = routing_decision(request, fallback=True)
        else:
            actual = provider.route(request)
        expected = case["expected_route"]
        route_correct += int(actual.route == expected)
        type_correct += int(actual.query_type == case["expected_query_type"])
        reason_correct += int(actual.reason_code == case["expected_reason_code"])
        skipped += int(actual.route in {"cheap", "ignore"})
        confusion[expected][actual.route] += 1
        disagreements = [
            name
            for name, want, got in (
                ("route", case["expected_route"], actual.route),
                ("query_type", case["expected_query_type"], actual.query_type),
                ("reason_code", case["expected_reason_code"], actual.reason_code),
            )
            if want != got
        ]
        if disagreements:
            # Reported, never repaired. A case that disagrees is evidence, and the
            # set is not curated against the provider.
            misroutes.append(
                {
                    "case_id": case["case_id"],
                    "field": disagreements,
                    "expected": {
                        "route": case["expected_route"],
                        "query_type": case["expected_query_type"],
                        "reason_code": case["expected_reason_code"],
                    },
                    "actual": {
                        "route": actual.route,
                        "query_type": actual.query_type,
                        "reason_code": actual.reason_code,
                    },
                    "exercises": case["written_to_exercise"],
                }
            )
    total = len(cases)
    per_route: dict[str, dict[str, float | int]] = {}
    for route in ROUTES:
        true_positive = confusion[route][route]
        false_positive = sum(confusion[other][route] for other in ROUTES if other != route)
        false_negative = sum(confusion[route][other] for other in ROUTES if other != route)
        precision = true_positive / max(true_positive + false_positive, 1)
        recall = true_positive / max(true_positive + false_negative, 1)
        f1 = 2 * precision * recall / max(precision + recall, 1e-12)
        per_route[route] = {
            "support": true_positive + false_negative,
            "precision": round(precision, 4),
            "recall": round(recall, 4),
            "f1": round(f1, 4),
        }
    return {
        "cases": total,
        "route_accuracy": round(route_correct / total, 4),
        "query_type_accuracy": round(type_correct / total, 4),
        "reason_code_accuracy": round(reason_correct / total, 4),
        "synthesis_skipped": skipped,
        "per_route": per_route,
        "confusion": {route: dict(values) for route, values in confusion.items()},
        "provenance": provenance(cases),
        "coverage": {
            "fallback_cases": sum(1 for case in cases if case.get("fallback")),
            "operations_exercised": sorted({str(case["operation"]) for case in cases}),
            "by_expected_route": {
                route: sum(1 for case in cases if case["expected_route"] == route)
                for route in ROUTES
            },
        },
        "misroutes": misroutes,
        "misroutes_note": (
            "Every case that disagrees with its own label, kept. These are the reason the "
            "accuracies above are not 1.0, and they are not defects in the fixture set: "
            "they are the cases the original fifteen hand-written greetings did not cover. "
            "Removing one would raise the number and delete the evidence."
        ),
    }


def main() -> int:
    report = evaluate()
    print(json.dumps(report, ensure_ascii=False, indent=2))
    route_accuracy = float(report["route_accuracy"])
    type_accuracy = float(report["query_type_accuracy"])
    return 0 if route_accuracy >= 0.8 and type_accuracy >= 0.75 else 1


if __name__ == "__main__":
    raise SystemExit(main())
