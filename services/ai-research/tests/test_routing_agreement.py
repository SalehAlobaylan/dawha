"""The routing decision's own half of the agreement it has with the Go fallback.

`internal/ai/routing.go` and `app/main.py` are two implementations of one
decision. The Go test
`TestTheGoFallbackAndTheProviderReachTheSameDecisionOnEveryLabelledCase` runs both
over all thirty-three labelled cases and requires the divergence set to be empty;
it skips when this service's interpreter is not beside the Go package, because a
Go package must not require a Python service to build.

So the provider's half needs its own pins here, or a checkout without the Go tree
beside it has nothing to catch a regression. These are the three things a change
to `normalized_routing_text` could break:

  1. the punctuation-only query rt-019 is labelled with is ignored rather than
     routed cheap, which is the case the two implementations disagreed about;
  2. the normalizer drops punctuation without dropping letters or digits, so the
     "fix" cannot become "strip the Arabic" - which is exactly what a
     hand-written character class does;
  3. a term table entry that normalizes to nothing matches EVERY value, because
     the empty string is contained in everything.
"""

from __future__ import annotations

import unicodedata

from app.main import (
    _CONTRADICTION_TERMS,
    _ROUTING_DEEP_TERMS,
    _ROUTING_NOISE,
    _ROUTING_QUERY_TERMS,
    RoutingRequest,
    normalized_routing_text,
    normalized_text,
    routing_decision,
    routing_query_type,
)


def test_a_query_of_pure_punctuation_is_ignored_and_not_routed_cheap() -> None:
    """rt-019, the case the two implementations disagreed about.

    A query carrying no word a researcher could have meant should be ignored
    rather than sent to a synthesis path. The Go fallback already reduced it to
    nothing; this service kept the punctuation, matched no term and routed it
    cheap, so the route depended on whether the service was up.
    """
    decision = routing_decision(RoutingRequest(text="؟؟؟", operation="research"))

    assert decision.route == "ignore"
    assert decision.reason_code == "noise"
    assert decision.query_type == "general"
    # The fallback path is the same decision with the flag set, and the Go side
    # only ever has the fallback. Both have to agree.
    fallback = routing_decision(RoutingRequest(text="؟؟؟", operation="research"), fallback=True)
    assert fallback.route == "ignore"
    # The query type has to be general too: a value that is empty must not score
    # against any term, and an empty term is contained in an empty value.
    assert routing_query_type("؟؟؟") == "general"
    assert routing_query_type("") == "general"


def test_the_routing_normalizer_drops_punctuation_and_keeps_letters_and_digits() -> None:
    # Letters and digits survive. A normalizer that removed the script rather
    # than the punctuation would also make rt-019 pass, and would make every
    # term match every value.
    assert normalized_routing_text("ذكر الرحيل ١٢٣") == "ذكر الرحيل ١٢٣"
    assert normalized_routing_text("عبد الله بن محمد") == "عبد الله بن محمد"
    # Punctuation becomes a separator rather than vanishing, so a mark between
    # two words does not weld them together.
    assert normalized_routing_text("والد،محمد") == "والد محمد"
    assert normalized_routing_text("سجل. نص") == "سجل نص"
    # Diacritics, alef variants, ya and ta marbuta are folded, as on the Go side.
    assert normalized_routing_text("عَبْدُ الله") == "عبد الله"
    assert normalized_routing_text("أحمد") == "احمد"
    assert normalized_routing_text("فاطمة") == "فاطمه"
    # And the whole of a punctuation-only value goes.
    assert normalized_routing_text("؟؟؟") == ""
    assert normalized_routing_text("!!!...") == ""
    assert normalized_routing_text("   ") == ""
    # The name normalizer is a different function and keeps punctuation: a stored
    # name has to keep the marks the record spells it with, and this is the
    # reason routing does not use it.
    assert normalized_text("والد،محمد") == "والد،محمد"
    assert normalized_routing_text("والد،محمد") != normalized_text("والد،محمد")


def test_no_routing_term_normalizes_to_nothing() -> None:
    """An empty needle is contained in everything.

    Every term here goes through the normalizer before it is looked for, so a
    term that folded to the empty string would match every value and the query
    type would be decided by table order rather than by the question. The
    normalizer is shared with the Go fallback, so this is an invariant both sides
    need and neither side can see at compile time.
    """
    terms: list[str] = []
    for group in _ROUTING_QUERY_TERMS.values():
        terms.extend(group)
    terms.extend(_ROUTING_DEEP_TERMS)
    terms.extend(_CONTRADICTION_TERMS)
    terms.extend(_ROUTING_NOISE)
    for term in terms:
        assert normalized_routing_text(term), f"routing term {term!r} normalizes to nothing"


def test_the_punctuation_normalization_is_unicode_category_based() -> None:
    """The separator test is a category test, not a hand-listed set of characters.

    A hand-listed character class is how the first attempt at this fix went
    wrong: it included the whole Arabic block, so every Arabic word folded to
    nothing and every term matched every value. The rule is now written against
    the Unicode general categories, so the next script is covered without a
    second edit, and a letter can never be classified as punctuation.
    """
    # Nothing classified as a punctuation category may survive.
    for codepoint in range(0x0000, 0x0700):
        character = chr(codepoint)
        if not unicodedata.category(character).startswith("P"):
            continue
        assert normalized_routing_text(f"a{character}b") in {"a b", "ab"}, (
            f"U+{codepoint:04X} is punctuation and should separate, not survive"
        )
    # And Arabic letters, which are in the same block as Arabic punctuation, are
    # not punctuation and must survive.
    for codepoint in (0x0627, 0x0628, 0x0645, 0x0647, 0x0646, 0x0631):
        assert normalized_routing_text(chr(codepoint)) == chr(codepoint).casefold()
