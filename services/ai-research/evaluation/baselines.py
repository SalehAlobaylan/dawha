"""Reviewed quality baselines for the deterministic provider.

Each group is a small set of hand-reviewed cases. A case is not a score in
itself: it is an input, the outcome a reviewer decided is correct, and a note
explaining why. The metrics are computed against those decisions, so the report
records what the provider actually does today rather than what it should do.

The thresholds are set at the measured baseline and live in
evaluation.thresholds under one version. That makes the gate useful in one
direction - a change that quietly loses accuracy fails - and deliberately
useless in the other: passing these numbers is not a claim that the provider is
good, only that it has not got worse. The extraction and contradiction groups in
particular are baselines with visible weaknesses, and the report says so rather
than rounding them away.

No group here compares a vector path against a non-vector path. There is no
vector-only baseline in this repository, so nothing in this file supports a
GraphRAG or embedding-retrieval improvement claim, and the report states that.
The citation group added by plan 009 measures the same provider as the rest; see
evaluate_citations for what it deliberately does not measure.
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
    ResearchQueryRequest,
    SourceContext,
    StatementInput,
    normalize_arabic_name,
)
from evaluation.thresholds import thresholds_for

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
        "thresholds": thresholds_for("retrieval"),
        "notes": [
            "Relevance labels are the reviewer's, not the provider's.",
            "Reranking is token overlap, so precision at 1 is expected to be modest.",
        ],
    }


def _exact_span(value: str) -> str:
    """A span as a comparable value.

    The reviewer's expected entities and the provider's proposals are compared as
    names, not as the raw strings they happen to be: `normalize_arabic_name` is
    the normalization this repository already applies before a term is matched
    against a name column, so "أبو بكر" and "ابو بكر" are one span under it. It
    is applied to BOTH sides, which is what keeps this an exact-span rule rather
    than a second normalization that only helps the provider.
    """
    return normalize_arabic_name(value).casefold()


def evaluate_extraction(provider: DeterministicProvider) -> dict[str, Any]:
    """Claim-count accuracy, and entity precision and recall on the EXACT span.

    Claim extraction is measured on the number of claims, because the provider
    splits a sentence at its relation term and the reviewer does not argue with
    where the cut fell.

    Entity extraction is measured on the exact span: a proposed entity is a hit
    when it IS a reviewed span, and a reviewed span is covered when it IS a
    proposed entity. It used to be measured by containment - a proposal counted
    when it CONTAINED a reviewed span - which is what let a proposal as long as a
    whole sentence score as a hit. Containment could not tell a good boundary from
    a bad one, and it is the reason `extraction.entity_precision` sat at 0.3333
    while the provider proposed twelve candidates for six sentences.

    The scoring rule changed with the provider in plan 016, so a
    `entity_precision` from a report whose group notes still say containment is not
    comparable with one from this report, and `entity_scoring` below says which
    rule produced the number.
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
        wanted_spans = {_exact_span(span) for span in wanted}
        proposed_spans = {_exact_span(item.text) for item in entities}
        # Set membership rather than a count of matches, so a provider that
        # proposed the same span twice is charged for it twice in precision - it
        # put two candidates in front of a reviewer - while coverage counts each
        # reviewed span once.
        covered_entities += len(wanted_spans & proposed_spans)
        predicted_entities += len(entities)
        correct_entities += sum(1 for span in proposed_spans if span in wanted_spans)

    entity_precision = ratio(correct_entities, predicted_entities)
    entity_recall = ratio(covered_entities, expected_entities)
    metrics = {
        "cases": len(cases),
        "claim_count_accuracy": ratio(count_hits, len(cases)) if cases else 0.0,
        "entity_scoring": "exact_span",
        "entity_precision": entity_precision,
        "entity_recall": entity_recall,
        "entity_f1": f1(entity_precision, entity_recall),
        "mean_entities_per_case": ratio(predicted_entities, len(cases)) if cases else 0.0,
    }
    return {
        "metrics": metrics,
        "thresholds": thresholds_for("extraction"),
        "notes": [
            "Entity spans are scored EXACTLY: a proposal is a hit when it is a reviewed span, "
            "not when it contains one. entity_scoring in the metrics says which rule produced "
            "the number, because a report from before plan 016 scored containment and the two "
            "are not comparable.",
            "Entity precision rose from a measured 0.3333 because the provider stopped "
            "proposing whole sentences, not because the fixtures moved: the expected entities "
            "in extraction_cases.jsonl are byte-identical to what the reviewer recorded, and no "
            "threshold moved with the number.",
            "The bounding costs recall on a bare personal name with no kunyah or nisbah marker "
            "around it, which this lexical proposer no longer offers. Two of the six fixtures "
            "(ext-003, ext-004) record an empty expected set for sentences that are nothing but "
            "names in a relation, so that trade is the reviewer's and is measured here rather "
            "than argued.",
            f"A precision of {entity_precision} is {correct_entities} correct proposals out of "
            f"{predicted_entities} across {len(cases)} fixtures. It is agreement with "
            "internally authored labels on a small set, NOT a quality claim about entity "
            "extraction, and it must not be quoted as one.",
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
        "thresholds": thresholds_for("resolution"),
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
        "thresholds": thresholds_for("contradiction"),
        "notes": [
            "The provider keys contradiction terms on parentage wording, so a migration"
            " disagreement is missed.",
            "Pairs about different people are still proposed, so precision is 0.6 rather than 1.0.",
            "verdict_accuracy is a baseline with two known misses recorded in the fixtures,"
            " not a target.",
        ],
    }


def evaluate_citations(provider: DeterministicProvider) -> dict[str, Any]:
    """Citation grounding: does the answer point at sources that say what it claims.

    Five numbers, and the split between them matters more than any single value.

    Two are safety properties and are held at 1.0. `source_id_fidelity` asks
    whether every citation names a context that was actually sent: a citation to
    something nobody provided is a fabricated source, which is the one failure
    this product cannot absorb. `unsupported_refusal` asks whether a question the
    sources do not answer came back with no citation at all rather than an
    unsupported one. `excerpt_fidelity` is the same idea one level down: the
    quotation a citation carries must be a literal substring of the source it
    points at, so a citation cannot display text the source does not contain.

    Three are quality. `citation_precision` charges the response for citing a
    source that shares vocabulary with the question but does not answer it.
    `support_recall` charges it for leaving out a source that does answer. And
    `grounded_answer_rate` is the coarse version of both: when a supporting
    source exists, did anything come back at all.

    What this group does NOT measure, and what no group in this file measures:
    whether the answer prose is right. The deterministic provider returns one of
    two fixed sentences, so an answer-quality metric built on it would measure
    those two strings. The refusal wording is pinned by a test instead, in
    tests/test_evaluation.py, and the answer's own review_required flag is what
    the product relies on.
    """
    cases = load_cases("citation_cases.jsonl")
    cited_total = 0
    cited_supporting = 0
    supporting_total = 0
    supporting_cited = 0
    excerpts_faithful = 0
    ids_faithful = 0
    refusal_cases = 0
    refusals_honoured = 0
    grounded_cases = 0
    grounded_answered = 0
    cases_with_citations = 0

    for case in cases:
        response = provider.research_query(
            ResearchQueryRequest(
                query=case["query"],
                contexts=[
                    SourceContext(id=item["id"], title=item["title"], text=item["text"])
                    for item in case["contexts"]
                ],
            )
        )
        sent = {item["id"]: item["text"] for item in case["contexts"]}
        supporting = {item["id"] for item in case["contexts"] if item["supports"]}
        expected = set(case["expected_citations"])
        cited = {citation.source_id for citation in response.citations}

        cited_total += len(cited)
        cited_supporting += len(cited & supporting)
        supporting_total += len(supporting)
        supporting_cited += len(supporting & cited)
        for citation in response.citations:
            if citation.source_id in sent and citation.excerpt in sent[citation.source_id]:
                excerpts_faithful += 1
            if citation.source_id in sent:
                ids_faithful += 1
        if cited:
            cases_with_citations += 1
        if not expected:
            refusal_cases += 1
            if not cited:
                refusals_honoured += 1
        else:
            grounded_cases += 1
            if cited:
                grounded_answered += 1

    metrics = {
        "cases": len(cases),
        "citations": cited_total,
        "cases_with_citations": cases_with_citations,
        "citation_precision": ratio(cited_supporting, cited_total),
        "support_recall": ratio(supporting_cited, supporting_total),
        "excerpt_fidelity": ratio(excerpts_faithful, cited_total),
        "source_id_fidelity": ratio(ids_faithful, cited_total),
        "unsupported_refusal": ratio(refusals_honoured, refusal_cases) if refusal_cases else 1.0,
        "grounded_answer_rate": ratio(grounded_answered, grounded_cases) if grounded_cases else 1.0,
    }
    return {
        "metrics": metrics,
        "thresholds": thresholds_for("citations"),
        "notes": [
            "The supporting labels are the reviewer's, not the provider's: the provider is "
            "never asked what it thinks supports the question.",
            "excerpt_fidelity, source_id_fidelity and grounded_answer_rate are held at 1.0 "
            "because they are safety properties, not quality ones.",
            "unsupported_refusal is held at 1.0 for the same reason. It was 0.5 and a recorded "
            "defect until plan 012: research_query counted any shared token as grounding, so a "
            "source that shared only the preposition 'في' with the question was cited. It now "
            "requires a shared content token - the tokenizer's output minus the function words "
            "listed next to the filter in app/main.py - and cit-003 refuses instead of citing a "
            "registry line that never mentions the question.",
            "support_recall cannot reach 1.0 while the response keeps at most five "
            "citations: cit-005 sends six supporting sources, so one is dropped by design.",
            "This group does not measure answer prose. The deterministic provider returns "
            "one of two fixed sentences, so an answer-quality metric here would measure "
            "those strings; the refusal wording is pinned in tests/test_evaluation.py.",
            "The citation fixtures were written by the plan-009 executor against the same "
            "rubric as the plan-004 set; cit-007 and cit-008 were added and the whole set "
            "re-read by the plan-012 executor when the content-token filter landed.",
        ],
    }
