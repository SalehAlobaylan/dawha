"""The proposed entity span is the name, and not the sentence around it.

`extract_entities` used to propose any run of four or more Arabic characters,
which - applied to a registry line - proposes the line. A reviewer opening the
candidate panel had to trim a third of the proposals by hand, and
`extraction.entity_precision` measured 0.3333 and sat in `KNOWN_DEFECTS` for it.

These tests pin the rule itself rather than the number, because the number is a
measurement over six internally authored fixtures and the rule is the thing a
change can get wrong:

  - a long Arabic sentence yields the NAME, not the sentence;
  - a kunyah is a name on its own, whatever precedes it, and a nisbah is only a
    name when nothing names it - "عبدالله بن محمد" is one man's full name and
    "بن محمد" is its tail, which is nobody's name;
  - the span stops at a function word and at a second marker;
  - the Arabic conjunction و is written attached to the next word, so a name that
    grew until something stopped it would swallow the next name with it.

The evaluation scores the result EXACTLY, so an over-long proposal and an
under-short one are both wrong and neither is free.
"""

from __future__ import annotations

from app.main import DeterministicProvider, ExtractionRequest, bounded_name_spans
from evaluation.baselines import evaluate_extraction, load_cases


def proposed(text: str) -> list[str]:
    return [
        candidate.text
        for candidate in DeterministicProvider()
        .extract_entities(ExtractionRequest(text=text))
        .entities
    ]


def test_a_long_arabic_sentence_yields_the_name_and_not_the_sentence() -> None:
    text = "ذكر السجل أن أبو بكر هو والد عبدالله بن محمد في مدينة الطائف سنة مئة."
    spans = bounded_name_spans(text)

    # The whole sentence is not a name, and neither is any part of it that is not
    # one. "أبو بكر" is the name; "والد عبدالله بن محمد" is a relation, and the
    # prepositional tail after it is not a name.
    assert spans == ["أبو بكر"], spans
    for span in spans:
        assert len(span) < 20, f"a proposed span of {len(span)} characters is a sentence: {span}"
        assert text.count(span) == 1, f"{span} was extracted out of its sentence"


def test_a_kunyah_is_a_name_and_a_nisbah_needs_nothing_naming_it() -> None:
    # A kunyah stands on its own. "ذكر" is a verb and names nobody, and the span
    # is still a name.
    assert bounded_name_spans("ذكر أبو بكر وعبدالله بن محمد في السجل.") == ["أبو بكر"]
    assert bounded_name_spans("بنت محمد هي أم عبدالله.") == ["بنت محمد"]
    # A nisbah with no ancestor named in front of it is a name.
    assert bounded_name_spans("قيل: بن محمد بن سعد") == ["بن محمد"]
    # A nisbah with an ancestor named in front of it is a connector, and its tail
    # is not a name. This is the distinction the whole rule turns on.
    assert bounded_name_spans("سجل says عبدالله بن محمد في الرياض.") == []
    assert bounded_name_spans("هاجر سعد بن عامر إلى الأحساء.") == []


def test_the_span_stops_at_a_function_word_and_at_a_second_marker() -> None:
    # "هو" is a pronoun and ends the name; the old regex took it anyway, which is
    # where "أبو بكر هو" came from.
    assert bounded_name_spans("ذكر أبو بكر هو والد محمد") == ["أبو بكر"]
    # "في" is a preposition and ends the name. The nisbah has to start the run
    # here: with a word in front of it, it is a connector - see the test above.
    assert bounded_name_spans("بن محمد في الرياض") == ["بن محمد"]
    # A second marker ends the first one's name, and the second is skipped because
    # the word before it names it.
    assert bounded_name_spans("أبو بكر بن محمد") == ["أبو بكر"]
    # A marker with nothing after it is the relation, not the entity.
    assert bounded_name_spans("أبو") == []
    assert bounded_name_spans("بن") == []
    # And the attached conjunction never gets swallowed: و is written onto the next
    # word, so a span that ran to the next function word would take a name with it.
    assert bounded_name_spans("أبو بكر وعبدالله") == ["أبو بكر"]


def test_the_extraction_evaluation_scores_the_exact_span() -> None:
    """The scoring rule, and that the fixtures were not moved to suit it.

    Containment used to charge the provider for an over-long proposal and could
    not tell a good boundary from a bad one. It is exact now, and the group says
    which rule produced its number, so a report from before this change is not
    silently comparable with one from after it.
    """
    report = evaluate_extraction(DeterministicProvider())
    metrics = report["metrics"]

    assert metrics["entity_scoring"] == "exact_span"
    # Every fixture's expected entities are the ones the reviewer recorded, and
    # every one of them is now proposed exactly.
    cases = load_cases("extraction_cases.jsonl")
    assert cases, "the fixture set is empty; an empty set would report a perfect precision"
    for case in cases:
        got = proposed(case["text"])
        assert got == case["expected_entities"], (
            f"{case['id']}: the extractor proposed {got} and the reviewer's set is "
            f"{case['expected_entities']}"
        )
    # The fixtures are not what changed: they still say what they said before.
    # ext-004's note is the load-bearing one - it recorded that the extractor was
    # deliberately over-broad there, and its expected entities are still the empty
    # set the reviewer wrote, not one added to suit the new rule.
    over_broad = [case for case in cases if "over-broad" in case.get("note", "")]
    assert [case["id"] for case in over_broad] == ["ext-004"], over_broad
    assert over_broad[0]["expected_entities"] == []
    ext_001 = next(case for case in cases if case["id"] == "ext-001")
    assert ext_001["expected_entities"] == ["أبو بكر"]
    assert metrics["entity_recall"] == 1.0
    assert metrics["entity_precision"] >= 1.0
