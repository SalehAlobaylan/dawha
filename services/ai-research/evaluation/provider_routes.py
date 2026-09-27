"""The provider's own decision for every labelled routing case, as JSON.

`make ai-eval` prints the provider's agreement with the labels. It does not, and
cannot, print what the *other* implementation of that decision does - the Go
fallback in `services/core-api/internal/ai/routing.go` is not reachable from
Python. Until now the only thing comparing the two was an inference: the Go test
asserted its own divergences from the labels, the evaluation reported the
provider's, and rt-019 was declared a disagreement because it appeared in one set
and not the other. That is a reading of two reports, not a comparison, and it is
what let a punctuation-only query be routed one way with the AI service up and
another way with it down for as long as the residual table existed.

So this module is the other half. It runs the provider over the same fixture the
evaluation reads, using the same rule for a case marked `fallback`, and prints
what the provider decided. The Go test then derives BOTH decisions for all
thirty-three cases in one process and requires the divergence set to be empty.

Run it directly to see the provider's own answers:

    cd services/ai-research && .venv/bin/python -m evaluation.provider_routes

The output shape is one object per case: `case_id`, `route`, `query_type`,
`reason_code`. Nothing else, because nothing else is compared.
"""

from __future__ import annotations

import json
import sys
from typing import Any

from app.main import DeterministicProvider, RoutingRequest, routing_decision
from evaluation.evaluate_routing import load_cases


def provider_decisions() -> list[dict[str, Any]]:
    """What the provider decides for every labelled case, in fixture order.

    The `fallback` flag is honoured the way `evaluate_routing` honours it: a case
    marked `fallback` is routed through `routing_decision(..., fallback=True)`,
    which is the same decision function with the flag set. That is the honest
    pairing for the Go side too, because `ai.FallbackRoute` is the only routing
    decision this repository has in Go.
    """
    provider = DeterministicProvider()
    answers: list[dict[str, Any]] = []
    for case in load_cases():
        request = RoutingRequest(
            text=case["text"],
            context=case.get("context"),
            operation=case["operation"],
            source_count=case["source_count"],
        )
        if case.get("fallback"):
            decision = routing_decision(request, fallback=True)
        else:
            decision = provider.route(request)
        answers.append(
            {
                "case_id": case["case_id"],
                "route": decision.route,
                "query_type": decision.query_type,
                "reason_code": decision.reason_code,
            }
        )
    return answers


def main() -> int:
    json.dump(provider_decisions(), sys.stdout, ensure_ascii=False, indent=2)
    sys.stdout.write("\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
