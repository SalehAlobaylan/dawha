"""The evaluation harness's own tests.

`make ai-eval` measures the provider. These tests measure the measurement: that
every fixture file is the shape its metric assumes, that every group has a
threshold under one version, and that a group which stops producing a number is
reported as a failure rather than quietly skipped.

The last one is the important one. An evaluation gate that passes when a metric
disappears is worse than no gate, because it converts a measurement into a
description, and the failure this repository is arranged to prevent is a document
claiming something nobody checked.
"""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any

import pytest

from app.main import DeterministicProvider
from evaluation import evaluate
from evaluation.baselines import (
    evaluate_citations,
    evaluate_contradiction,
    evaluate_extraction,
    evaluate_resolution,
    evaluate_retrieval,
    load_cases,
)
from evaluation.thresholds import (
    GROUPS,
    KNOWN_DEFECTS,
    THRESHOLDS,
    THRESHOLDS_REVIEWED_ON,
    THRESHOLDS_VERSION,
    defects_for,
    thresholds_for,
)

EVALUATION_DIR = Path(__file__).resolve().parent.parent / "evaluation"

FIXTURE_FILES = (
    "routing_cases.jsonl",
    "retrieval_cases.jsonl",
    "extraction_cases.jsonl",
    "resolution_cases.jsonl",
    "contradiction_cases.jsonl",
    "citation_cases.jsonl",
)

# The routing fixtures are the oldest set. They identify a case by its text
# rather than by an id, and they predate the convention every later fixture
# follows: a `reviewed_by` and a note saying why the case is there. They are left
# as they are - they predate this file and the plan asked for them to be kept -
# and the exemption is named here, with its reason, so "every reviewed case says
# who reviewed it" stays true of every set except one that is visibly an
# exception rather than an unnoticed gap.
LEGACY_FIXTURES = ("routing_cases.jsonl",)


def test_every_fixture_file_parses_and_is_not_empty() -> None:
    for name in FIXTURE_FILES:
        cases = load_cases(name)
        assert cases, f"{name} has no cases"
        if name in LEGACY_FIXTURES:
            assert len(cases) == len({case["text"] for case in cases}), (
                f"{name} has a duplicate case"
            )
            continue
        assert len(cases) == len({case["id"] for case in cases}), f"{name} has a duplicate case id"


def test_every_reviewed_case_records_who_reviewed_it_and_why() -> None:
    """A case without a reviewer and a reason is an assertion, not a review.

    This is the difference between a fixture and a wish, and it is the only thing
    standing between a number and a reader's trust in it.
    """
    for name in FIXTURE_FILES:
        if name in LEGACY_FIXTURES:
            continue
        for case in load_cases(name):
            assert case.get("reviewed_by"), f"{name} {case['id']} does not say who reviewed it"
            note = case.get("note", "")
            assert len(note) > 40, f"{name} {case['id']} does not explain why the case is there"


def test_citation_fixtures_are_internally_consistent() -> None:
    for case in load_cases("citation_cases.jsonl"):
        ids = [item["id"] for item in case["contexts"]]
        assert len(ids) == len(set(ids)), f"{case['id']} has a duplicate context id"
        for expected in case["expected_citations"]:
            assert expected in ids, (
                f"{case['id']} expects a citation to {expected}, which was not sent"
            )
        supporting = {item["id"] for item in case["contexts"] if item["supports"]}
        assert set(case["expected_citations"]) == supporting, (
            f"{case['id']} expects {case['expected_citations']} but marks "
            f"{sorted(supporting)} as supporting"
        )
        assert isinstance(case["contexts"], list)


def test_every_threshold_is_a_probability_and_every_group_has_one() -> None:
    for group in GROUPS:
        thresholds = thresholds_for(group)
        assert thresholds, f"{group} has no thresholds"
        for metric, value in thresholds.items():
            assert isinstance(value, float), f"{group}.{metric} is not a float"
            assert 0.0 <= value <= 1.0, f"{group}.{metric} = {value} is not a rate"
    assert set(THRESHOLDS) == set(GROUPS), (
        "a group has thresholds but is not in GROUPS, or the reverse"
    )
    assert THRESHOLDS_VERSION and THRESHOLDS_REVIEWED_ON


def test_every_safety_floor_is_one() -> None:
    """A safety property held at 0.9 is a safety property that can fail silently."""
    floors = (
        "resolution.unknown_left_unresolved",
        "citations.excerpt_fidelity",
        "citations.source_id_fidelity",
        "citations.grounded_answer_rate",
    )
    for key in floors:
        group, metric = key.split(".", 1)
        assert thresholds_for(group)[metric] == 1.0, f"{key} is not held at 1.0"


def test_every_known_defect_names_a_metric_that_exists() -> None:
    """A defect recorded against a metric nobody reports is a defect nobody is fixing."""
    groups = evaluate.build_groups()
    for key in KNOWN_DEFECTS:
        group, metric = key.split(".", 1)
        assert group in groups, f"{key} names a group that does not run"
        assert metric in groups[group]["metrics"], f"{key} names a metric that is not reported"
        found = defects_for(group, groups[group]["metrics"])
        reported = [item["metric"] for item in found]
        assert reported == sorted(reported), f"{key} is not reported in order"
        for entry in found:
            for field in ("summary", "cause", "consequence", "fix"):
                assert len(entry[field]) > 30, f"{key} has no usable {field}"


def test_a_known_defect_that_is_also_below_its_threshold_is_still_a_failure() -> None:
    """Recording a defect must not become a way to pass.

    A metric that is a known defect AND under its threshold is a regression the
    gate should still catch. The report's `ok` is computed from thresholds alone,
    which is what makes this true: the defect list never suppresses a failure.
    """
    report = evaluate.build_report()
    assert report["ok"], report["failures"]
    for defect in report["known_defects"]:
        group = report["groups"][defect["group"]]
        threshold = group["thresholds"][defect["metric"]]
        assert defect["measured"] >= threshold, (
            f"{defect['group']}.{defect['metric']} is {defect['measured']}, under its threshold "
            f"{threshold}: that is a failure, and the defect list must not hide it"
        )


def test_report_names_its_threshold_version_and_the_unavailable_baseline() -> None:
    report = evaluate.build_report()
    assert report["thresholds_version"] == THRESHOLDS_VERSION
    assert report["thresholds"]["reviewed_on"] == THRESHOLDS_REVIEWED_ON
    assert report["vector_baseline"]["available"] is False
    assert report["vector_baseline"]["reason"]
    assert report["vector_baseline"]["consequence"]


def test_report_lists_every_group_and_never_presents_a_routing_label_as_confidence() -> None:
    report = evaluate.build_report()
    for group in GROUPS:
        assert group in report["groups"], f"{group} did not run"
        assert report["groups"][group]["thresholds"], f"{group} reported no thresholds"
    routing_notes = " ".join(report["groups"]["routing"]["notes"])
    assert "not historical accuracy" in routing_notes
    assert "confidence" in routing_notes


def test_a_group_that_stops_measuring_is_a_failure(monkeypatch: pytest.MonkeyPatch) -> None:
    """The property that makes this a gate: a missing number is not a pass."""

    def broken_extraction(provider: DeterministicProvider) -> dict[str, Any]:
        report = evaluate_extraction(provider)
        report["metrics"].pop("claim_count_accuracy")
        return report

    monkeypatch.setattr(evaluate, "evaluate_extraction", broken_extraction)
    report = evaluate.build_report()
    assert not report["ok"]
    assert any("claim_count_accuracy was not measured" in failure for failure in report["failures"])


def test_a_group_with_no_thresholds_is_a_failure(monkeypatch: pytest.MonkeyPatch) -> None:
    def broken_citations(provider: DeterministicProvider) -> dict[str, Any]:
        report = evaluate_citations(provider)
        report["thresholds"] = {}
        return report

    monkeypatch.setattr(evaluate, "evaluate_citations", broken_citations)
    report = evaluate.build_report()
    assert not report["ok"]
    assert any("reported no thresholds" in failure for failure in report["failures"])


def test_a_metric_below_its_threshold_fails_the_report(monkeypatch: pytest.MonkeyPatch) -> None:
    """The gate has to be able to fail, or it is decoration."""

    def worse_resolution(provider: DeterministicProvider) -> dict[str, Any]:
        report = evaluate_resolution(provider)
        report["metrics"]["unknown_left_unresolved"] = 0.5
        return report

    monkeypatch.setattr(evaluate, "evaluate_resolution", worse_resolution)
    report = evaluate.build_report()
    assert not report["ok"]
    assert any("unknown_left_unresolved" in failure for failure in report["failures"])


def test_the_report_written_to_disk_is_the_report_that_was_checked(tmp_path: Path) -> None:
    target = tmp_path / "report.json"
    report = evaluate.build_report()
    target.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    reloaded = json.loads(target.read_text(encoding="utf-8"))
    assert reloaded["thresholds_version"] == THRESHOLDS_VERSION
    assert reloaded["ok"] is True
    assert set(reloaded["groups"]) == set(GROUPS)


def test_groups_are_reproducible() -> None:
    """Two runs of the same fixtures must produce the same numbers."""
    provider = DeterministicProvider()
    for evaluate_group in (evaluate_retrieval, evaluate_extraction, evaluate_resolution,
                           evaluate_contradiction, evaluate_citations):
        first = evaluate_group(provider)["metrics"]
        second = evaluate_group(provider)["metrics"]
        assert first == second, f"{evaluate_group.__name__} is not reproducible"


def test_an_unsupported_answer_refuses_in_words_and_not_only_in_metrics() -> None:
    """The refusal prose, pinned here rather than scored in the citation group.

    The citation group measures whether a citation came back. This measures what
    the service says when none did, because a reader who gets a confident answer
    with no citation behind it has been told something the system does not know.
    """
    from app.main import ResearchQueryRequest

    provider = DeterministicProvider()
    response = provider.research_query(
        ResearchQueryRequest(
            query="ما اسم القائد الوارد في مخطوط、保健؟", contexts=[]
        )
    )
    assert response.citations == []
    assert "لا أستطيع" in response.answer
    assert response.review_required is True
    grounded = provider.research_query(
        ResearchQueryRequest(
            query="من كان والد عبدالله؟",
            contexts=[{"id": "s1", "title": "سجل", "text": "والد عبدالله محمد."}],
        )
    )
    assert grounded.citations
    assert "ليست إجابة تاريخية" in grounded.answer


def test_the_evaluation_directory_has_no_unread_fixture() -> None:
    """A fixture file nobody loads is a set of numbers nobody produced."""
    on_disk = {path.name for path in EVALUATION_DIR.glob("*_cases.jsonl")}
    assert on_disk == set(FIXTURE_FILES), (
        f"fixture files on disk {sorted(on_disk)} do not match the ones the harness reads "
        f"{sorted(FIXTURE_FILES)}"
    )
