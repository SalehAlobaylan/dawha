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

# VECTOR_BASELINE is the answer to a question this repository cannot answer *here*.
#
# It is `available: false` because this Python evaluation has no corpus and no
# embedding provider, not because the repository has one retrieval path. That
# distinction used to be blurred in this file's own comment, and the blur was
# load-bearing: it is what let `docs/phase-status.md` record "there is no embedding
# retrieval in the query path", which was false. `retrieveVectorPassages` has always
# existed, `execute` has always called both legs, and plan 014 gave the vector leg a
# corpus to score by backfilling embeddings through the same `/embed` contract the
# worker calls.
#
# The comparison now exists, and it is somewhere else: `make retrieval-report` runs
# it over the seeded corpus and a labelled Arabic question set, and
# `docs/retrieval-measurement.md` holds the numbers. It is not here because this
# harness measures a provider in isolation with hand-written fixtures, and a
# retrieval comparison needs a database, a corpus with embeddings, and judged
# relevance - none of which is what this file is for. Putting a number here would
# mean inventing one.
#
# The field stays `false` and stays a failure rather than a pass, because an
# unmeasured quality claim is the thing this report exists to prevent. What changed
# is only that the reason now names where the measurement is instead of claiming
# there is nothing to measure.
VECTOR_BASELINE = {
    "available": False,
    "compared": "none",
    "reason": (
        "This harness evaluates the provider in isolation over hand-written fixtures. "
        "It has no corpus, no database and no judged relevance, so it cannot compare "
        "two retrieval paths; that comparison is not missing from the repository, it "
        "lives in a different command. `make retrieval-report` scores vector-only, "
        "hybrid and graph-augmented retrieval over the seeded corpus and a labelled "
        "Arabic question set, and docs/retrieval-measurement.md holds the result."
    ),
    "consequence": (
        "No GraphRAG, embedding or hybrid-retrieval improvement is claimed or measured "
        "HERE. The retrieval group measures the token-overlap reranker against reviewed "
        "labels and nothing else; docs/graph-benchmark.md measures graph traversal cost "
        "rather than retrieval quality; and docs/retrieval-measurement.md measures "
        "retrieval quality. Read all three before quoting any of them."
    ),
    "note": (
        "The measured result of that comparison, for a reader who lands here first: on "
        "the seventeen multi-hop questions, hybrid beats vector-only by 0.882 recall@5 "
        "against a stated variance of 0.248, and Phase 20's criterion is STILL OPEN. "
        "The difference is not evidence, because this provider's embed is a SHA-512 "
        "digest of the input rather than a semantic embedding, so the vector-only arm "
        "is a permutation. This field predates plan 009 and plan 014 and remains false; "
        "what plan 009 added is the citations group, and what plan 014 added is the "
        "corpus, the labels and the three arms."
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
        # The label provenance and the mis-routes are siblings of the metrics, not
        # prose beside the file, so a reader who opens report.json and reads nothing
        # else still learns that the labels are internally authored and which cases
        # the provider disagrees with. That is the whole reason they are here: this
        # report is the artifact a decision gets quoted from.
        "provenance": report["provenance"],
        "coverage": report["coverage"],
        "misroutes": report["misroutes"],
        "notes": [
            "The fifteen cases reviewed before plan 015 are unchanged; plan 015 added "
            f"{report['cases'] - 15} more and recorded provenance for all "
            f"{report['cases']}. Every existing case still passes its own label.",
            "The added cases target the fallback path, the ignore path and the "
            "cheap/deep boundary, because those are what fifteen hand-written greetings "
            "least cover. `coverage` reports how many fallback cases there are and which "
            "of the six operations the set touches.",
            f"{len(report['misroutes'])} case(s) disagree with their own label and are "
            "KEPT. `misroutes` lists them with the shape each was written to exercise. A "
            "fixture set curated against the provider would have removed them, and the "
            "number would be higher and worthless.",
            "per_route carries the confusion counts so a drop can be traced to a route.",
            "route_accuracy, query_type_accuracy and reason_code_accuracy are agreement "
            "with internally authored labels. They are not accuracy on real questions, and "
            "they are not historical accuracy: they must not be quoted as confidence in a "
            "historical claim. Read `provenance` before quoting any of them.",
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
        # The provenance and the mis-routes are printed here rather than left in the
        # JSON, because the sentence a reader will quote from this run's output is
        # this one and it must not be quotable without the limitation beside it.
        provenance = group.get("provenance")
        if provenance:
            print(f"  labels: {provenance['labels']}")
            by_origin = json.dumps(provenance["by_origin"], ensure_ascii=False)
            print(f"  cases by origin: {by_origin}")
            derived = provenance["derived_from_real_query_set"]
            available = "yes" if provenance["available_real_query_set"] else "no"
            print(f"  cases derived from a real query set: {derived}")
            print(f"  a real query set is available: {available}")
            print(f"  limitation: {provenance['limitation']}")
        misroutes = group.get("misroutes") or []
        if misroutes:
            print(f"  mis-routes kept ({len(misroutes)}):")
            for misroute in misroutes:
                print(
                    f"    {misroute['case_id']} {','.join(misroute['field'])}: "
                    f"expected {misroute['expected']['route']}/"
                    f"{misroute['expected']['query_type']}/{misroute['expected']['reason_code']}, "
                    f"got {misroute['actual']['route']}/{misroute['actual']['query_type']}/"
                    f"{misroute['actual']['reason_code']}"
                )
                print(f"      exercises: {misroute['exercises']}")
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
