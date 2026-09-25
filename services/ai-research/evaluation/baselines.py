"""Reviewed quality baselines for the deterministic provider.

Each group is a small set of hand-reviewed cases. A case is not a score in
itself: it is an input, the outcome a reviewer decided is correct, and a note
explaining why. The metrics are computed against those decisions, so the report
records what the provider actually does today rather than what it should do.

The thresholds are set at the measured baseline. That makes the gate useful in
one direction - a change that quietly loses accuracy fails - and deliberately
useless in the other: passing these numbers is not a claim that the provider is
good, only that it has not got worse. The extraction and contradiction groups in
particular are baselines with visible weaknesses, and the report says so rather
than rounding them away.

No group here compares a vector path against a non-vector path. There is no
vector-only baseline in this repository, so nothing in this file supports a
GraphRAG or embedding-retrieval improvement claim, and the report states that.
"""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any

from app.main import (
    ContradictionRequest,
    DeterministicProvider,
    EntityReference,
    EntityResolutionRequest,
    ExtractionRequest,
    RerankDocument,
    RerankRequest,
    StatementInput,
)

# retrieval_precision_at_k bounds how much of the first k results may be
# irrelevant. At k = 1 this is the single number a reader notices: was the top
# hit actually evidence?
RETRIEVAL_PRECISION_AT = 1


def load_cases(name: str) -> list[dict[str, Any]]:
    """Read one reviewed case file. A blank line is ignored; anything else must parse."""
    path = Path(__file__).with_name(name)
    cases = []
    for line in path.read_text(encoding="utf-8").splitlines():
        if line.strip():
            cases.append(json.loads(line))
    return cases


def ratio(numerator: float, denominator: float) -> float:
    if denominator == 0:
        return 0.0
    return round(numerator / denominator, 4)


def f1(precision: float, recall: float) -> float:
    if precision + recall == 0:
        return 0.0
    return round(2 * precision * recall / (precision + recall), 4)


def evaluate_retrieval(provider: DeterministicProvider) -> dict[str, Any]:
    """Precision at k, recall and mean reciprocal rank over the reviewed relevance labels.

    A reviewed case marks each document relevant or not. The provider is never
    asked what it thinks is relevant; it ranks, and the labels decide.
    """
    cases = load_cases("retrieval_cases.jsonl")
    precisions: list[float] = []
    recalls: list[float] = []
    reciprocal_ranks: list[float] = []
    for case in cases:
        response = provider.rerank(
            RerankRequest(
                query=case["query"],
                documents=[
                    RerankDocument(id=doc["id"], text=doc["text"]) for doc in case["documents"]
                ],
            )
        )
        ranked = [document.id for document in response.documents]
        relevant = {doc["id"] for doc in case["documents"] if doc["relevant"]}
        top = ranked[:RETRIEVAL_PRECISION_AT]
        hits = len([doc for doc in top if doc in relevant])
        precisions.append(ratio(hits, len(top)) if top else 0.0)
        retrieved = {doc for doc in ranked if doc in relevant}
        recalls.append(ratio(len(retrieved), len(relevant)))
        first = next((index + 1 for index, doc in enumerate(ranked) if doc in relevant), 0)
        reciprocal_ranks.append(ratio(1.0, first) if first else 0.0)

    metrics = {
        "cases": len(cases),
        "precision_at_1": ratio(sum(precisions), len(precisions)) if precisions else 0.0,
        "recall": ratio(sum(recalls), len(recalls)) if recalls else 0.0,
        "mrr": ratio(sum(reciprocal_ranks), len(reciprocal_ranks)) if reciprocal_ranks else 0.0,
    }
    return {
        "metrics": metrics,
        "thresholds": {"precision_at_1": 0.5, "recall": 0.7, "mrr": 0.4},
        "notes": [
            "Relevance labels are the reviewer's, not the provider's.",
            "Reranking is token overlap, so precision at 1 is expected to be modest.",
        ],
    }


def evaluate_extraction(provider: DeterministicProvider) -> dict[str, Any]:
    """Claim-count accuracy, and entity precision and recall under a containment rule.

    Claim extraction is measured on the number of claims, because the provider
    splits a sentence at its relation term and the reviewer does not argue with
    where the cut fell. Entity extraction is measured with a containment rule: a
    predicted span counts as a hit when it contains a reviewed span. The
    provider is known to propose over-long spans, and this rule scores that as
    imprecise rather than pretending the boundary is agreed - which is why the
    entity numbers here are a baseline, not a target.
    """
    cases = load_cases("extraction_cases.jsonl")
    count_hits = 0
    predicted_entities = 0
    correct_entities = 0
    expected_entities = 0
    covered_entities = 0

    for case in cases:
        text = case["text"]
        entities = provider.extract_entities(ExtractionRequest(text=text)).entities
        claims = provider.extract_claims(ExtractionRequest(text=text)).claims
        if len(claims) == case["expected_claims"]:
            count_hits += 1
        wanted = case["expected_entities"]
        expected_entities += len(wanted)
        covered_entities += sum(1 for span in wanted if any(span in item.text for item in entities))
        predicted_entities += len(entities)
        correct_entities += sum(1 for item in entities if any(span in item.text for span in wanted))

    entity_precision = ratio(correct_entities, predicted_entities)
    entity_recall = ratio(covered_entities, expected_entities)
    metrics = {
        "cases": len(cases),
        "claim_count_accuracy": ratio(count_hits, len(cases)) if cases else 0.0,
        "entity_precision": entity_precision,
        "entity_recall": entity_recall,
        "entity_f1": f1(entity_precision, entity_recall),
        "mean_entities_per_case": ratio(predicted_entities, len(cases)) if cases else 0.0,
    }
    return {
        "metrics": metrics,
        "thresholds": {"claim_count_accuracy": 0.8, "entity_precision": 0.2, "entity_recall": 0.9},
        "notes": [
            "Entity spans are scored by containment: an over-long proposal counts as a hit,"
            " so it costs precision without costing recall.",
            "Entity precision is low because the provider proposes over-long spans, not because"
            " the labels are loose: the fixtures record what a reviewer would accept.",
        ],
    }


def evaluate_resolution(provider: DeterministicProvider) -> dict[str, Any]:
    """Top-1 accuracy, how often the match is labelled with the evidence it rests on,
    and how often an unknown name is left unresolved.

    The last number matters most for this product: resolving a name to the
    least-bad candidate is how a false identity gets asserted.
    """
    cases = load_cases("resolution_cases.jsonl")
    top_correct = 0
    label_correct = 0
    unknown_resolved = 0
    unknown_cases = 0

    for case in cases:
        response = provider.resolve_entity(
            EntityResolutionRequest(
                name=case["name"],
                candidates=[EntityReference(**candidate) for candidate in case["candidates"]],
            )
        )
        accepted = {value for value in case["expected_top"].split("|") if value}
        top = response.matches[0] if response.matches else None
        if (top.candidate_id if top else "") in accepted:
            top_correct += 1
        wanted_label = case["expected_matched_on"]
        if wanted_label and (top.matched_on if top else "") == wanted_label:
            label_correct += 1
        if not accepted:
            unknown_cases += 1
            # An unknown name must not come back with a positive score.
            if not top or top.score <= case.get("max_top_score", 0.0):
                unknown_resolved += 1

    total = len(cases)
    metrics = {
        "cases": total,
        "top1_accuracy": ratio(top_correct, total),
        "evidence_label_accuracy": ratio(label_correct, total),
        "unknown_left_unresolved": ratio(unknown_resolved, unknown_cases) if unknown_cases else 1.0,
    }
    return {
        "metrics": metrics,
        "thresholds": {
            "top1_accuracy": 0.8,
            "evidence_label_accuracy": 0.8,
            "unknown_left_unresolved": 1.0,
        },
        "notes": [
            "A name nobody recorded must resolve to nothing; that threshold is 1.0 on purpose.",
            "Cases with two acceptable candidates are credited to either.",
        ],
    }


def evaluate_contradiction(provider: DeterministicProvider) -> dict[str, Any]:
    """Pair precision and recall against the reviewed pair list, plus whether the
    no-contradiction verdict is right.

    The reviewed list is what a reader should be asked about. A pair that is not
    in it costs the reviewer time; a pair that is missing from it lets a real
    disagreement pass unnoticed.
    """
    cases = load_cases("contradiction_cases.jsonl")
    predicted_pairs = 0
    correct_pairs = 0
    reviewed_pairs = 0
    verdict_correct = 0

    for case in cases:
        response = provider.analyze_contradiction(
            ContradictionRequest(statements=[StatementInput(**item) for item in case["statements"]])
        )
        got = {f"{pair.left_id}|{pair.right_id}" for pair in response.pairs}
        want = set(case["expected_pairs"])
        predicted_pairs += len(got)
        correct_pairs += len(got & want)
        reviewed_pairs += len(want)
        if response.has_candidate_contradiction == case["expected_has_contradiction"]:
            verdict_correct += 1

    precision = ratio(correct_pairs, predicted_pairs)
    recall = ratio(correct_pairs, reviewed_pairs)
    metrics = {
        "cases": len(cases),
        "pair_precision": precision,
        "pair_recall": recall,
        "pair_f1": f1(precision, recall),
        "verdict_accuracy": ratio(verdict_correct, len(cases)) if cases else 0.0,
    }
    return {
        "metrics": metrics,
        # The verdict threshold is the measured baseline, not a target: two of the
        # six reviewed cases are known misses (a pair about different people is
        # proposed, and a migration disagreement is not), so the number sits where
        # it sits and a change in either direction is visible in the report.
        "thresholds": {"pair_precision": 0.4, "pair_recall": 0.5, "verdict_accuracy": 0.66},
        "notes": [
            "The provider keys contradiction terms on parentage wording, so a migration"
            " disagreement is missed.",
            "Pairs about different people are still proposed, so precision is 0.6 rather than 1.0.",
            "verdict_accuracy is a baseline with two known misses recorded in the fixtures,"
            " not a target.",
        ],
    }
