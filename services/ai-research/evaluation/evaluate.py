"""Runs every configured quality group and writes one machine-readable report.

`make ai-eval` and the CI ai-research job both run this. The routing group from
evaluation.evaluate_routing is kept and included rather than replaced, so the
existing check still reports on its own.

Exit status is 0 when every group's metrics hold their thresholds, and 1
otherwise. Thresholds come from evaluation.thresholds under one version, and the
version is in the report: two reports are only comparable when their thresholds
are, and a report that does not say which thresholds it held to is not evidence of
anything. They are baselines: passing them means the provider has not got worse,
not that it is good. The report says that too, and it says plainly that there is
no vector-only baseline here and therefore no GraphRAG or embedding improvement
is claimed.

A group can also be reported as unavailable. A group that cannot be measured
honestly with what this repository contains is recorded as unavailable with the
reason, and it is a failure, not a pass: an unmeasured quality claim is the thing
this report exists to prevent. Nothing is estimated to fill a gap.

The report path comes from AI_EVAL_REPORT; it defaults to evaluation/report.json
next to this module, which is where a developer looks for it and where CI
collects it.
"""

from __future__ import annotations

import json
import os
import sys
from pathlib import Path
from typing import Any

from app.main import DeterministicProvider
from evaluation.baselines import (
    evaluate_citations,
    evaluate_contradiction,
    evaluate_extraction,
    evaluate_resolution,
    evaluate_retrieval,
)
from evaluation.evaluate_routing import evaluate as evaluate_routing
from evaluation.thresholds import (
    GROUPS,
    KNOWN_DEFECTS,
    THRESHOLDS_VERSION,
    defects_for,
    describe,
    thresholds_for,
)

# VECTOR_BASELINE is the answer to a question this repository cannot answer.
#
# A vector-only versus graph-augmented comparison needs two retrieval paths over
# the same corpus and a labelled judgement of which is better. There is one path
# here (a deterministic token-overlap reranker) and no labelled corpus, so any
# number in this slot would be invented. Plan 004 established the precedent and
# this file keeps it: the group is reported as unavailable, with the reason,
# rather than filled with a number nobody measured.
VECTOR_BASELINE = {
    "available": False,
    "compared": "none",
    "reason": (
        "There is one retrieval path in this repository - a deterministic token-overlap "
        "reranker - and no labelled corpus to judge a second one against. A vector-only "
        "baseline would need both."
    ),
    "consequence": (
        "No GraphRAG, embedding or hybrid-retrieval improvement is claimed or measured "
        "here. The retrieval group measures the token-overlap reranker against reviewed "
        "labels and nothing else, and the graph benchmark in docs/graph-benchmark.md "
        "measures graph traversal, not retrieval quality."
    ),
    "note": (
        "This field predates plan 009 and is unchanged by it. What plan 009 adds is the "
        "citations group, which is a grounding measurement and not a retrieval comparison."
    ),
}


def routing_group() -> dict[str, Any]:
    report = evaluate_routing()
    metrics = {
        "cases": report["cases"],
        "route_accuracy": report["route_accuracy"],
        "query_type_accuracy": report["query_type_accuracy"],
        "reason_code_accuracy": report["reason_code_accuracy"],
        "synthesis_skipped": report["synthesis_skipped"],
        "per_route": report["per_route"],
    }
    return {
        "metrics": metrics,
        "thresholds": thresholds_for("routing"),
        "notes": [
            "The reviewed routing cases are unchanged from before the baselines were added.",
            "per_route carries the confusion counts so a drop can be traced to a route.",
            "route_accuracy and query_type_accuracy are agreement with reviewed labels on "
            "texts the reviewer wrote. They are not historical accuracy on real questions, "
            "and must not be quoted as confidence in a historical claim.",
        ],
    }


def build_groups() -> dict[str, dict[str, Any]]:
    provider = DeterministicProvider()
    return {
        "routing": routing_group(),
        "retrieval": evaluate_retrieval(provider),
        "extraction": evaluate_extraction(provider),
        "resolution": evaluate_resolution(provider),
        "contradiction": evaluate_contradiction(provider),
        "citations": evaluate_citations(provider),
    }


def build_report() -> dict[str, Any]:
    groups = build_groups()
    failures: list[str] = []
    for name in GROUPS:
        group = groups.get(name)
        if group is None:
            failures.append(
                f"{name} is configured in evaluation.thresholds.GROUPS but no group produced it"
            )
            continue
        thresholds = group.get("thresholds") or {}
        if not thresholds:
            failures.append(f"{name} reported no thresholds, so nothing was checked")
        for metric, threshold in thresholds.items():
            value = group["metrics"].get(metric)
            if value is None:
                failures.append(f"{name}.{metric} was not measured")
            elif value < threshold:
                failures.append(f"{name}.{metric} = {value} is below the baseline {threshold}")
    # A group that produced metrics nobody set a threshold for is a group whose
    # numbers are decoration, and the report says so rather than passing quietly.
    for name, group in groups.items():
        if name not in GROUPS:
            failures.append(
                f"{name} produced metrics but is not listed in evaluation.thresholds.GROUPS"
            )
    unconfigured = sorted(set(groups) - set(GROUPS))
    if unconfigured:
        failures.append(
            "group(s) produced metrics without a configured entry: " + ", ".join(unconfigured)
        )

    # A recorded defect is not a failure: the gate exists to catch a change, and
    # a defect that was true yesterday is still true today. But it travels with
    # the numbers, in the report and in the printed summary, so "all groups hold
    # their baselines" can never be quoted without the list of things that are
    # known to be wrong.
    known_defects: list[dict[str, Any]] = []
    for name in GROUPS:
        group = groups.get(name)
        if group is None:
            continue
        for defect in defects_for(name, group["metrics"]):
            known_defects.append({"group": name, **defect})
    unrecorded = sorted(
        set(KNOWN_DEFECTS) - {f"{item['group']}.{item['metric']}" for item in known_defects}
    )
    if unrecorded:
        failures.append(
            "KNOWN_DEFECTS names metric(s) no group reported: " + ", ".join(unrecorded)
        )

    return {
        "provider": "DeterministicProvider",
        "model": "deterministic-semantic-control-v1",
        "thresholds": describe(),
        "thresholds_version": THRESHOLDS_VERSION,
        "vector_baseline": VECTOR_BASELINE,
        "groups": groups,
        "known_defects": known_defects,
        "failures": failures,
        "ok": not failures,
    }


def print_summary(report: dict[str, Any]) -> None:
    print("Dawha AI quality baselines")
    print(f"  provider: {report['provider']}")
    print(
        f"  thresholds version: {report['thresholds_version']}"
        f" (reviewed {report['thresholds']['reviewed_on']})"
    )
    print(f"  {report['thresholds']['policy']}")
    print(f"  vector-only baseline: {'yes' if report['vector_baseline']['available'] else 'no'}")
    print(f"  {report['vector_baseline']['consequence']}")
    for name in GROUPS:
        group = report["groups"].get(name)
        if group is None:
            print(f"\n[{name}]")
            print("  UNAVAILABLE: configured but not produced by this run")
            continue
        print(f"\n[{name}]")
        for metric, threshold in group["thresholds"].items():
            value = group["metrics"].get(metric)
            if isinstance(value, dict):
                print(f"  {metric}: {json.dumps(value, ensure_ascii=False)}")
                continue
            mark = "ok " if isinstance(value, (int, float)) and value >= threshold else "LOW"
            print(f"  {metric}: {value} (baseline {threshold}) {mark}")
        for note in group["notes"]:
            print(f"  note: {note}")
    if report["known_defects"]:
        print("\n[known defects] holding their thresholds, and still wrong:")
        for defect in report["known_defects"]:
            print(
                f"  {defect['group']}.{defect['metric']} = {defect['measured']}: "
                f"{defect['summary']}"
            )
            print(f"    cause: {defect['cause']}")
            print(f"    fix: {defect['fix']}")
    print("")
    if report["ok"]:
        print(
            "all configured groups hold their baselines"
            + (
                f"; {len(report['known_defects'])} known defect(s) recorded above"
                " are NOT fixed by that"
                if report["known_defects"]
                else ""
            )
        )
        return
    print("baseline failures:")
    for failure in report["failures"]:
        print(f"  - {failure}")


def main() -> int:
    report = build_report()
    target = Path(os.environ.get("AI_EVAL_REPORT") or Path(__file__).with_name("report.json"))
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print_summary(report)
    print(f"\nreport written to {target}")
    return 0 if report["ok"] else 1


if __name__ == "__main__":
    sys.exit(main())
