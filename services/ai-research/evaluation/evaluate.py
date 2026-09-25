"""Runs every configured quality group and writes one machine-readable report.

`make ai-eval` and the CI ai-research job both run this. The routing group from
evaluation.evaluate_routing is kept and included rather than replaced, so the
existing check still reports on its own.

Exit status is 0 when every group's metrics hold their thresholds, and 1
otherwise. Thresholds are baselines: passing them means the provider has not got
worse, not that it is good. The report says that too, and it says plainly that
there is no vector-only baseline here and therefore no GraphRAG or embedding
improvement is claimed.

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
    evaluate_contradiction,
    evaluate_extraction,
    evaluate_resolution,
    evaluate_retrieval,
)
from evaluation.evaluate_routing import evaluate as evaluate_routing

# The thresholds for the routing group, kept with the group so the report is the
# single place a reader has to look to know what "passing" means.
ROUTING_THRESHOLDS = {"route_accuracy": 0.8, "query_type_accuracy": 0.75}

VECTOR_BASELINE = {
    "available": False,
    "compared": "none",
    "note": (
        "No vector-only retrieval baseline exists in this repository, so no GraphRAG, "
        "embedding or hybrid-retrieval improvement is claimed or measured here. The "
        "retrieval group measures the deterministic token-overlap reranker against "
        "reviewed labels and nothing else."
    ),
}


def routing_group() -> dict[str, Any]:
    report = evaluate_routing()
    metrics = {
        "cases": report["cases"],
        "route_accuracy": report["route_accuracy"],
        "query_type_accuracy": report["query_type_accuracy"],
        "reason_code_accuracy": report["reason_code_accuracy"],
        "per_route": report["per_route"],
    }
    return {
        "metrics": metrics,
        "thresholds": ROUTING_THRESHOLDS,
        "notes": [
            "The reviewed routing cases are unchanged from before the baselines were added.",
            "per_route carries the confusion counts so a drop can be traced to a route.",
        ],
    }


def build_report() -> dict[str, Any]:
    provider = DeterministicProvider()
    groups = {
        "routing": routing_group(),
        "retrieval": evaluate_retrieval(provider),
        "extraction": evaluate_extraction(provider),
        "resolution": evaluate_resolution(provider),
        "contradiction": evaluate_contradiction(provider),
    }
    failures: list[str] = []
    for name, group in groups.items():
        for metric, threshold in group["thresholds"].items():
            value = group["metrics"].get(metric)
            if value is None:
                failures.append(f"{name}.{metric} was not measured")
            elif value < threshold:
                failures.append(f"{name}.{metric} = {value} is below the baseline {threshold}")
    return {
        "provider": "DeterministicProvider",
        "model": "deterministic-semantic-control-v1",
        "vector_baseline": VECTOR_BASELINE,
        "groups": groups,
        "failures": failures,
        "ok": not failures,
    }


def print_summary(report: dict[str, Any]) -> None:
    print("Dawha AI quality baselines")
    print(f"  provider: {report['provider']}")
    print(f"  vector-only baseline: {'yes' if report['vector_baseline']['available'] else 'no'}")
    print(f"  {report['vector_baseline']['note']}")
    for name, group in report["groups"].items():
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
    print("")
    if report["ok"]:
        print("all configured groups hold their baselines")
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
