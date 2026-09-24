from __future__ import annotations

import json
from collections import defaultdict
from pathlib import Path

from app.main import DeterministicProvider, RoutingRequest

ROUTES = ("ignore", "cheap", "deep")


def evaluate() -> dict[str, object]:
    provider = DeterministicProvider()
    cases_path = Path(__file__).with_name("routing_cases.jsonl")
    cases = [
        json.loads(line)
        for line in cases_path.read_text(encoding="utf-8").splitlines()
        if line.strip()
    ]
    route_correct = 0
    type_correct = 0
    reason_correct = 0
    skipped = 0
    confusion: dict[str, dict[str, int]] = defaultdict(lambda: defaultdict(int))
    for case in cases:
        request = RoutingRequest(
            text=case["text"],
            context=case.get("context"),
            operation=case["operation"],
            source_count=case["source_count"],
        )
        actual = provider.route(request)
        expected = case["expected_route"]
        route_correct += int(actual.route == expected)
        type_correct += int(actual.query_type == case["expected_query_type"])
        reason_correct += int(actual.reason_code == case["expected_reason_code"])
        skipped += int(actual.route in {"cheap", "ignore"})
        confusion[expected][actual.route] += 1
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
    }


def main() -> int:
    report = evaluate()
    print(json.dumps(report, ensure_ascii=False, indent=2))
    route_accuracy = float(report["route_accuracy"])
    type_accuracy = float(report["query_type_accuracy"])
    return 0 if route_accuracy >= 0.8 and type_accuracy >= 0.75 else 1


if __name__ == "__main__":
    raise SystemExit(main())
