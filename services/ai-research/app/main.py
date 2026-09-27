from __future__ import annotations

import hashlib
import re
import unicodedata
from itertools import combinations
from typing import Literal, Protocol

from fastapi import FastAPI
from pydantic import BaseModel, Field, field_validator, model_validator

_DIACRITICS = re.compile(r"[\u064B-\u065F\u0670\u06D6-\u06ED]")
_SPACES = re.compile(r"\s+")
_SENTENCES = re.compile(r"[^.!?\n]+[.!?]?")
_ARABIC_SEQUENCE = re.compile(r"[؀-ۿ][؀-ۿ\s]{2,60}[؀-ۿ]")
_RELATION_TERMS = (
    "والد",
    "أبو",
    "ابو",
    "ابن",
    "بنت",
    "زوج",
    "أخ",
    "أخت",
    "ينتمي",
    "هاجر",
    "سكن",
    "يقول",
    "father",
    "mother",
    "spouse",
)
_CONTRADICTION_TERMS = (
    "والد",
    "أبو",
    "ابو",
    "father",
    "والدة",
    "mother",
    "تناقض",
    "تعارض",
    "contradiction",
    "conflict",
)
_ROUTING_QUERY_TERMS = {
    "source_evidence": (
        "مصدر",
        "دليل",
        "نص",
        "صفحة",
        "سجل",
        "مرجع",
        "اقتباس",
        "source",
        "evidence",
        "document",
        "record",
    ),
    "identity": ("هوية", "شخص", "اسم", "لقب", "اسماء", "alias", "identity", "person"),
    "relationship": (
        "والد",
        "والدة",
        "ابو",
        "أبو",
        "ابن",
        "بنت",
        "زوج",
        "قرابة",
        "نسب",
        "علاقة",
        "تناقض",
        "تعارض",
        "رواية",
        "روايات",
        "father",
        "parent",
        "family",
    ),
    "geography": ("مكان", "مدينة", "قرية", "هاجر", "هجرة", "مسار", "جغراف", "place", "location"),
}
_ROUTING_DEEP_TERMS = (
    "تحقق",
    "تحقيق",
    "قارن",
    "مقارنة",
    "تناقض",
    "تعارض",
    "تتبع",
    "اثبات",
    "إثبات",
    "اصل",
    "مسار",
    "بحث",
    "investigate",
    "compare",
    "contradiction",
    "trace",
    "prove",
    "explain",
)
_ROUTING_NOISE = {
    "مرحبا",
    "السلام عليكم",
    "شكرا",
    "شكرا لكم",
    "كيف حالك",
    "spam",
    "اعلان",
}

app = FastAPI(
    title="Dawha Research API",
    version="0.2.0",
    description="Research assistance that never decides historical truth.",
)


def normalize_arabic_name(value: str) -> str:
    normalized = unicodedata.normalize("NFKC", value.strip())
    normalized = _DIACRITICS.sub("", normalized)
    normalized = normalized.replace("أ", "ا").replace("إ", "ا").replace("آ", "ا")
    normalized = normalized.replace("ى", "ي").replace("ة", "ه")
    normalized = _SPACES.sub(" ", normalized)
    return normalized.strip()


def normalized_text(value: str) -> str:
    return normalize_arabic_name(value).casefold()


# THE ROUTING NORMALIZATION IS A SHARED VOCABULARY, AND THE SPECIFICATION IS
# identity.NormalizeArabicName in services/core-api/internal/identity.
#
# The routing decision exists twice: `routing_decision` here, and
# `ai.FallbackRoute` in services/core-api/internal/ai/routing.go, which is what
# runs when this service cannot be reached. The two cannot be one function - they
# are two languages and a shared runtime between a Go API and a Python service is
# not a dependency this repository should take - so they share a *vocabulary*
# instead, and the vocabulary is a written list of steps that both implement.
#
# The steps, in order:
#
#   1. NFKC normalization.
#   2. Drop combining marks (the Arabic diacritics, including the tatweel).
#   3. أ إ آ ٱ -> ا, ى -> ي, ة -> ه.
#   4. Every space, every punctuation mark becomes a single space. This is
#      the step the two implementations used to disagree about, and it is
#      load-bearing: a query carrying no word at all, only punctuation, reduces
#      to nothing here, so `not value` holds and the route is `ignore` with the
#      reason code `noise`. It is exactly the answer fixture rt-019 is labelled
#      with, and exactly what the Go fallback already did.
#
#      "Punctuation" means the Unicode general categories Go's `unicode.IsPunct`
#      covers - Pc, Pd, Pe, Pf, Pi, Po, Ps - and not the symbol categories, so
#      the predicate is written against the categories rather than against a
#      hand-listed set of characters that would silently stop covering the next
#      script. A digit is not punctuation and a letter is not punctuation, so
#      Arabic text and Arabic-Indic digits survive step 4 and only the marks
#      around them go.
#   5. Collapse runs of whitespace and trim.
#
# Step 4 is why this is a function of its own rather than a tweak to
# `normalize_arabic_name`. That function is the one behind the name-normalization
# endpoint, entity resolution, embedding and claim extraction, and it PRESERVES
# punctuation on purpose: a stored or displayed name keeps the punctuation the
# record spells it with. Changing it would change those responses, which is a
# different decision with a different blast radius. Routing is the one place that
# asks "what words did this mean", and it is the only place that needs
# punctuation to disappear.
#
# The one invariant a term table has to hold for both sides to agree: no term may
# normalize to the empty string, because an empty needle is contained in
# everything. Every entry in `_ROUTING_QUERY_TERMS`, `_ROUTING_DEEP_TERMS`,
# `_CONTRADICTION_TERMS` and `_ROUTING_NOISE` is an Arabic or English word, and
# `TestTheGoFallbackAndTheProviderReachTheSameDecisionOnEveryLabelledCase` is
# what would notice if one stopped being true.
#
# `TestTheGoFallbackAndTheProviderReachTheSameDecisionOnEveryLabelledCase` in
# services/core-api/internal/ai runs both implementations over all thirty-three
# labelled cases and requires them to agree on every one. That test is what keeps
# this list honest, and it is also what a reader should run first before editing
# either side.
def normalized_routing_text(value: str) -> str:
    """`value` folded the way the routing decision needs it, on both sides.

    See the note above: the specification is `identity.NormalizeArabicName`, and
    this is that function's step list applied to the routing path only.
    """
    normalized = unicodedata.normalize("NFKC", value.strip())
    normalized = _DIACRITICS.sub("", normalized)
    normalized = normalized.replace("أ", "ا").replace("إ", "ا").replace("آ", "ا")
    normalized = normalized.replace("ٱ", "ا").replace("ى", "ي").replace("ة", "ه")
    folded = []
    for character in normalized:
        if character.isspace() or unicodedata.category(character).startswith("P"):
            folded.append(" ")
        else:
            folded.append(character)
    return _SPACES.sub(" ", "".join(folded)).strip().casefold()


def tokenize(value: str) -> set[str]:
    return {token for token in re.findall(r"[\w؀-ۿ]+", normalized_text(value)) if token}


# A shared function word is not grounding.
#
# `tokenize` keeps every word, so the question "في أي سنة هاجرت القبيلة إلى
# الأحساء؟" and a registry line "قيد في سجل الرياض، صفحة تسع." intersect on the
# single token "في" - the preposition - and on nothing that any of them is about.
# Counting that intersection as support made a source that says nothing about the
# question into a citation, and a reader who opens it learns nothing, which is
# the failure this product's whole source-grounding premise exists to prevent.
#
# So a context is cited only when it shares at least one *content* token with the
# question: the tokenizer's own output minus the words below. They are the words
# that mean nothing on their own - articles, pronouns, prepositions, conjunctions,
# the question words and the auxiliaries - in Arabic and in English.
#
# The list is short on purpose. A longer one starts removing words that do carry
# meaning, and a shorter one lets a shared preposition back in; either mistake
# would move a number nobody read the code for. Three citation fixtures pin both
# directions and the case between them, so changing a word here has to be a
# deliberate edit rather than a quiet one: cit-003 shares only a function word
# and must not be cited, cit-007 shares a content word and must be, and cit-008
# shares both and must still be.
#
# Every entry is written in the form `tokenize` returns, which is
# `normalize_arabic_name` followed by `casefold`: أ/إ/آ are ا, ى is ي, ة is ه, and
# English is lowercased. "الى" for "إلى" here is the tokenizer's output, not a
# typo, and adding an un-normalised spelling would silently never match.
#
# The Arabic clitics (و, ف, ب, ك, ل, ال) are not listed separately: the tokenizer
# does not split them off, so "والد" is one token and the words below are only
# ever matched whole.
_FUNCTION_WORDS = frozenset(
    {
        # Arabic: articles, pronouns, prepositions, conjunctions, question words,
        # auxiliaries, negation and demonstratives.
        "ال",
        "الي",
        "التي",
        "الذي",
        "الذين",
        "الذان",
        "اللواتي",
        "ذات",
        "ذاك",
        "هذا",
        "هذه",
        "ذلك",
        "تلك",
        "هؤلاء",
        "هو",
        "هي",
        "هم",
        "هما",
        "نحن",
        "انت",
        "انتم",
        "انا",
        "ان",
        "انها",
        "انه",
        "ايضا",
        "في",
        "من",
        "الى",
        "على",
        "عن",
        "مع",
        "بين",
        "حول",
        "لدى",
        "منذ",
        "دون",
        "حتى",
        "او",
        "ثم",
        "لكن",
        "اذا",
        "كما",
        "ما",
        "ماذا",
        "اي",
        "اين",
        "متي",
        "كيف",
        "هل",
        "كم",
        "لماذا",
        "كان",
        "كانت",
        "يكون",
        "قد",
        "لقد",
        "ليس",
        "لا",
        "لم",
        "لن",
        "غير",
        # English: the same categories.
        "a",
        "about",
        "after",
        "all",
        "an",
        "and",
        "any",
        "are",
        "as",
        "at",
        "be",
        "been",
        "but",
        "by",
        "can",
        "could",
        "did",
        "do",
        "does",
        "for",
        "from",
        "had",
        "has",
        "have",
        "he",
        "her",
        "here",
        "him",
        "his",
        "how",
        "i",
        "if",
        "in",
        "into",
        "is",
        "it",
        "its",
        "may",
        "might",
        "must",
        "no",
        "not",
        "of",
        "on",
        "or",
        "our",
        "she",
        "should",
        "so",
        "than",
        "that",
        "the",
        "their",
        "them",
        "then",
        "there",
        "these",
        "they",
        "this",
        "those",
        "to",
        "was",
        "we",
        "were",
        "what",
        "when",
        "where",
        "which",
        "who",
        "whom",
        "whose",
        "why",
        "will",
        "with",
        "would",
        "you",
        "your",
    }
)


def content_tokens(value: str) -> set[str]:
    """The tokens of `value` that carry meaning on their own.

    This is the tokenizer's output minus the function words above, and it is the
    only difference between "these two texts are about the same thing" and "these
    two texts both contain a preposition".
    """
    return tokenize(value) - _FUNCTION_WORDS


def routing_contains_any(value: str, terms: tuple[str, ...]) -> bool:
    """Whether a routing value carries any of the routing terms.

    The value and the terms both go through `normalized_routing_text`, so a term
    matches the same way it does in the Go fallback's `routingContainsAny`.
    """
    normalized = normalized_routing_text(value)
    return any(normalized_routing_text(term) in normalized for term in terms)


def routing_query_type(value: str) -> str:
    normalized = normalized_routing_text(value)
    scores = {
        query_type: sum(
            normalized_routing_text(term) in normalized for term in terms
        )
        for query_type, terms in _ROUTING_QUERY_TERMS.items()
    }
    best_type = "general"
    best_score = 0
    for query_type in ("relationship", "source_evidence", "identity", "geography"):
        if scores[query_type] > best_score:
            best_type = query_type
            best_score = scores[query_type]
    return best_type


def routing_decision(
    request: "RoutingRequest", fallback: bool = False
) -> "RoutingDecision":
    value = normalized_routing_text(request.text)
    query_type = routing_query_type(value)
    source_bearing = request.source_count > 0 or bool((request.context or "").strip())
    contradiction = routing_contains_any(
        value, _CONTRADICTION_TERMS
    ) or request.operation == "contradiction"
    if not value or (not source_bearing and value in _ROUTING_NOISE):
        route = "ignore"
        reason_code = "noise"
        score = 0.05
    elif contradiction:
        route = "deep"
        reason_code = "contradiction_signal"
        score = 0.9
    elif request.operation in {"duplicate_detection", "contradiction"}:
        route = "deep"
        reason_code = "operation_requires_deep"
        score = 0.82
    elif routing_contains_any(value, _ROUTING_DEEP_TERMS):
        route = "deep"
        reason_code = "multi_step"
        score = 0.86
    elif (
        source_bearing
        and request.source_count >= 2
        and query_type in {"relationship", "identity"}
    ):
        route = "deep"
        reason_code = "multi_step"
        score = 0.8
    elif source_bearing:
        route = "cheap"
        reason_code = "source_context"
        score = 0.7
    else:
        route = "cheap"
        reason_code = "simple_lookup"
        score = 0.58
    continue_investigation = route == "deep" or contradiction or request.source_count >= 2
    return RoutingDecision(
        route=route,
        query_type=query_type,
        reason_code=reason_code,
        source_bearing=source_bearing,
        potential_contradiction=contradiction,
        continue_investigation=continue_investigation,
        operational_score=score,
        model="deterministic-semantic-control-v1",
        fallback=fallback,
    )


def clamp(value: float) -> float:
    return round(max(0.0, min(1.0, value)), 4)


# THE PROPOSED ENTITY SPAN IS THE NAME, AND NOT THE SENTENCE AROUND IT.
#
# `extract_entities` used to propose two things. The first was a kunyah or nisbah
# form, taken by a regex over the marker followed by up to two more words. The
# second was ANY run of four or more Arabic characters, taken by
# `_ARABIC_SEQUENCE` - which, applied to a whole sentence, proposes the sentence.
# A registry line came back as one candidate of
# "ذكر السجل أن أبو بكر هو والد عبدالله بن محمد", and a reviewer opening the
# candidate panel had to trim every one of those by hand. That is the defect
# `KNOWN_DEFECTS` recorded under `extraction.entity_precision` and the reason the
# evaluation scored entity spans by containment: an over-long proposal contains
# the right answer, so the rule could not tell a good boundary from a bad one.
#
# So a proposed span is now BOUNDED, and the rule is about Arabic onomastics
# rather than a length threshold:
#
#  1. A person name starts at a KUNYAH - أبو, أبو, بنت. A kunyah is a name on its
#     own: "أبو بكر" is a man's name and "بنت محمد" is a form of address. It
#     always starts a name, whatever precedes it - including a verb, because
#     "ذكر أبو بكر" mentions a man and does not name one before him.
#  2. A person name starts at a NISBAH - ابن, بن - only when nothing names it. A
#     nisbah on its own is a parentage connector waiting for an ancestor, and
#     given one it is a connector inside a longer expression: "عبدالله بن محمد" is
#     one man's full name and "بن محمد" is its tail, which is not anybody's name.
#     The word that names a marker is any word that is not a function word, so
#     "ذكر السجل أن أبو بكر" and "بنت محمد هي أم عبدالله" are not parentage
#     formulas - "أن" and "هي" are function words and name nobody.
#  3. Either way the name is the marker and the ONE word after it, because that is
#     the shape a kunyah or a nisbah name has: "أبو بكر", "بنت محمد", "بن محمد".
#     A name is not allowed to run further than that, and the reason is specific:
#     the conjunction و in Arabic prose is written ATTACHED to the next word
#     ("ذكر أبو بكر وعبدالله بن محمد"), so a name that grew until a function word
#     or a marker stopped it would swallow the conjunction and the next name with
#     it. Bounding at one word cannot do that, because there is nothing between
#     the given name and the conjunction to mistake for part of the name.
#
#     The cost of bounding at one is stated rather than hidden: a name written with
#     a further qualifier - "أبو بكر الصديق" - is proposed without the qualifier.
#     That costs RECALL, never precision, which is the opposite of the defect this
#     replaces, and it is measured in the evaluation report rather than argued here.
#
#  4. A marker with no word after it is the relation, not the entity.
#
# The other cost of bounding is also stated: a bare personal name with no marker
# around it is no longer proposed. The reviewer's own fixtures
# (`extraction_cases.jsonl`) record an empty expected set for two sentences that
# are nothing but bare names in a relation - ext-003 and ext-004 - so this is
# what the reviewer asked for, and a lexical proposer that offers a name it cannot
# bound is a review queue rather than a candidate list.
#
# `_ARABIC_SEQUENCE` stays, and now does the one job it is good at: splitting a
# line into the runs of Arabic that a name can be made of. Punctuation and Latin
# text end a run, so a name is never proposed across them.
_KUNYAH_MARKERS = frozenset({"ابو", "بنت"})
_NISBAH_MARKERS = frozenset({"ابن", "بن"})
_NAME_MARKERS = _KUNYAH_MARKERS | _NISBAH_MARKERS
_NAME_WORDS_AFTER_MARKER = 1


def _is_name_word(value: str) -> bool:
    """Whether a word can be the name a marker introduces.

    Not a function word, and not another marker: a second marker means the first
    one's name ended where the second begins.
    """
    normalized = normalized_text(value)
    if not normalized or normalized in _FUNCTION_WORDS:
        return False
    return normalized not in _NAME_MARKERS


def bounded_name_spans(text: str) -> list[str]:
    """The person-name spans of `text`, in the order they appear.

    See the note above: the span is the name, bounded on both sides, and a
    nisbah that something already names is a connector rather than a name.
    """
    spans: list[str] = []
    for run in _ARABIC_SEQUENCE.finditer(text):
        words = [match for match in re.finditer(r"[؀-ۿ]+", run.group(0))]
        for index, word in enumerate(words):
            marker = normalized_text(word.group(0))
            if marker not in _NAME_MARKERS:
                continue
            if marker in _NISBAH_MARKERS and index > 0 and _is_name_word(
                words[index - 1].group(0)
            ):
                # Something names this nisbah, so it is a parentage connector and
                # its tail is not a name.
                continue
            end = index + 1 + _NAME_WORDS_AFTER_MARKER
            if end > len(words) or not _is_name_word(words[end - 1].group(0)):
                # A marker with no name after it is the relation, not the entity.
                continue
            spans.append(text[run.start() + word.start() : run.start() + words[end - 1].end()])
    return spans


def relation_parts(value: str, predicate: str) -> tuple[str, str]:
    index = value.find(predicate)
    if index < 0:
        return value[:100], ""
    subject = value[:index].strip(" ،,.؟?")
    object_text = value[index + len(predicate) :].strip(" ،,.؟?")
    return subject or value[:100], object_text


class NameNormalizationRequest(BaseModel):
    value: str = Field(min_length=1, max_length=500)

    @field_validator("value")
    @classmethod
    def non_blank_value(cls, value: str) -> str:
        if not value.strip():
            raise ValueError("value must not be blank")
        return value


class NameNormalizationResponse(BaseModel):
    original: str
    normalized: str = Field(min_length=1)
    method: Literal["deterministic_arabic_normalization"] = "deterministic_arabic_normalization"
    preserves_original: Literal[True] = True


class EmbeddingRequest(BaseModel):
    text: str = Field(min_length=1, max_length=20000)
    dimensions: int = Field(default=1536, ge=16, le=1536)


class EmbeddingResponse(BaseModel):
    embedding: list[float] = Field(min_length=16, max_length=1536)
    dimensions: int
    model: str = "deterministic-foundation"
    deterministic: Literal[True] = True

    @model_validator(mode="after")
    def dimensions_match_embedding(self) -> "EmbeddingResponse":
        if self.dimensions != len(self.embedding):
            raise ValueError("dimensions must match embedding length")
        return self


class ClassificationRequest(BaseModel):
    text: str = Field(min_length=1, max_length=20000)
    labels: list[str] = Field(min_length=2, max_length=20)
    context: str | None = Field(default=None, max_length=5000)

    @field_validator("labels")
    @classmethod
    def unique_labels(cls, value: list[str]) -> list[str]:
        cleaned = [item.strip() for item in value]
        if any(not item for item in cleaned) or len({item.casefold() for item in cleaned}) != len(
            cleaned
        ):
            raise ValueError("labels must be non-empty and unique")
        return cleaned


class ClassificationCandidate(BaseModel):
    label: str = Field(min_length=1, max_length=200)
    confidence: float = Field(ge=0, le=1)
    rationale: str = Field(min_length=1, max_length=500)


class ClassificationResponse(BaseModel):
    candidates: list[ClassificationCandidate] = Field(min_length=1, max_length=20)
    model: str = "deterministic-foundation"
    review_required: Literal[True] = True


class RoutingRequest(BaseModel):
    text: str = Field(min_length=1, max_length=20000)
    context: str | None = Field(default=None, max_length=20000)
    operation: Literal[
        "research",
        "search",
        "suggestion",
        "source_processing",
        "duplicate_detection",
        "contradiction",
    ] = "research"
    source_count: int = Field(default=0, ge=0, le=10000)


class RoutingDecision(BaseModel):
    route: Literal["ignore", "cheap", "deep"]
    query_type: Literal["source_evidence", "identity", "relationship", "geography", "general"]
    reason_code: Literal[
        "noise",
        "simple_lookup",
        "source_context",
        "multi_step",
        "contradiction_signal",
        "operation_requires_deep",
        "uncertainty",
    ]
    source_bearing: bool
    potential_contradiction: bool
    continue_investigation: bool
    operational_score: float = Field(ge=0, le=1)
    model: str = "deterministic-semantic-control-v1"
    fallback: bool = False
    review_required: Literal[True] = True


class ExtractionRequest(BaseModel):
    text: str = Field(min_length=1, max_length=50000)


class EntityCandidate(BaseModel):
    text: str = Field(min_length=1, max_length=500)
    entity_type: Literal["person", "family", "tribe", "place", "unknown"]
    confidence: float = Field(ge=0, le=1)
    status: Literal["unreviewed", "needs_review"] = "unreviewed"
    rationale: str = Field(min_length=1, max_length=500)


class EntityExtractionResponse(BaseModel):
    entities: list[EntityCandidate]
    model: str = "deterministic-foundation"
    review_required: Literal[True] = True


class ClaimCandidate(BaseModel):
    subject_text: str = Field(min_length=1, max_length=500)
    predicate: str = Field(min_length=1, max_length=200)
    object_text: str | None = Field(default=None, max_length=500)
    confidence: float = Field(ge=0, le=1)
    status: Literal["unreviewed", "needs_review"] = "needs_review"
    rationale: str = Field(min_length=1, max_length=500)


class ClaimExtractionResponse(BaseModel):
    claims: list[ClaimCandidate]
    model: str = "deterministic-foundation"
    review_required: Literal[True] = True


class EntityReference(BaseModel):
    id: str = Field(min_length=1, max_length=100)
    name: str = Field(min_length=1, max_length=500)
    aliases: list[str] = Field(default_factory=list, max_length=50)


class EntityResolutionRequest(BaseModel):
    name: str = Field(min_length=1, max_length=500)
    candidates: list[EntityReference] = Field(default_factory=list, max_length=100)


class EntityMatch(BaseModel):
    candidate_id: str
    candidate_name: str
    score: float = Field(ge=0, le=1)
    matched_on: Literal["canonical", "alias", "token_overlap", "none"]


class EntityResolutionResponse(BaseModel):
    matches: list[EntityMatch]
    model: str = "deterministic-foundation"
    review_required: Literal[True] = True


class StatementInput(BaseModel):
    id: str = Field(min_length=1, max_length=100)
    text: str = Field(min_length=1, max_length=20000)


class ContradictionRequest(BaseModel):
    statements: list[StatementInput] = Field(min_length=2, max_length=50)


class ContradictionPair(BaseModel):
    left_id: str
    right_id: str
    confidence: float = Field(ge=0, le=1)
    rationale: str = Field(min_length=1, max_length=1000)
    status: Literal["needs_review"] = "needs_review"


class ContradictionResponse(BaseModel):
    has_candidate_contradiction: bool
    pairs: list[ContradictionPair]
    model: str = "deterministic-foundation"
    review_required: Literal[True] = True


class RerankDocument(BaseModel):
    id: str = Field(min_length=1, max_length=100)
    text: str = Field(min_length=1, max_length=20000)


class RerankRequest(BaseModel):
    query: str = Field(min_length=1, max_length=5000)
    documents: list[RerankDocument] = Field(min_length=1, max_length=100)


class RerankedDocument(BaseModel):
    id: str
    score: float = Field(ge=0, le=1)
    excerpt: str = Field(min_length=1, max_length=1000)


class RerankResponse(BaseModel):
    documents: list[RerankedDocument] = Field(min_length=1, max_length=100)
    model: str = "deterministic-foundation"


class SourceContext(BaseModel):
    id: str = Field(min_length=1, max_length=100)
    title: str = Field(min_length=1, max_length=500)
    text: str = Field(min_length=1, max_length=20000)


class ResearchQueryRequest(BaseModel):
    query: str = Field(min_length=1, max_length=5000)
    contexts: list[SourceContext] = Field(default_factory=list, max_length=50)


class ResearchCitation(BaseModel):
    source_id: str
    title: str
    excerpt: str = Field(min_length=1, max_length=1000)


class ResearchQueryResponse(BaseModel):
    answer: str = Field(min_length=1, max_length=10000)
    citations: list[ResearchCitation]
    model: str = "deterministic-foundation"
    review_required: Literal[True] = True


class ResearchProvider(Protocol):
    def embed(self, text: str, dimensions: int) -> list[float]: ...

    def classify(self, request: ClassificationRequest) -> ClassificationResponse: ...

    def route(self, request: RoutingRequest) -> RoutingDecision: ...

    def extract_entities(self, request: ExtractionRequest) -> EntityExtractionResponse: ...

    def extract_claims(self, request: ExtractionRequest) -> ClaimExtractionResponse: ...

    def resolve_entity(self, request: EntityResolutionRequest) -> EntityResolutionResponse: ...

    def analyze_contradiction(self, request: ContradictionRequest) -> ContradictionResponse: ...

    def rerank(self, request: RerankRequest) -> RerankResponse: ...

    def research_query(self, request: ResearchQueryRequest) -> ResearchQueryResponse: ...


class DeterministicProvider:
    def embed(self, text: str, dimensions: int) -> list[float]:
        digest = hashlib.sha512(normalized_text(text).encode("utf-8")).digest()
        values: list[float] = []
        for index in range(dimensions):
            byte = digest[index % len(digest)]
            next_byte = digest[(index * 7 + 13) % len(digest)]
            values.append(round((byte + next_byte) / 255 - 1, 6))
        return values

    def classify(self, request: ClassificationRequest) -> ClassificationResponse:
        text_tokens = tokenize(request.text)
        candidates: list[ClassificationCandidate] = []
        for label in request.labels:
            label_tokens = tokenize(label)
            overlap = len(text_tokens & label_tokens) / max(len(label_tokens), 1)
            candidates.append(
                ClassificationCandidate(
                    label=label,
                    confidence=clamp(0.25 + overlap * 0.7),
                    rationale="تداخل لفظي بين النص والتصنيف، ولا يثبت حقيقة تاريخية.",
                )
            )
        candidates.sort(key=lambda item: item.confidence, reverse=True)
        return ClassificationResponse(candidates=candidates)

    def route(self, request: RoutingRequest) -> RoutingDecision:
        return routing_decision(request)

    def extract_entities(self, request: ExtractionRequest) -> EntityExtractionResponse:
        values: list[str] = []
        for value in bounded_name_spans(request.text):
            if value and value not in values:
                values.append(value)
        entities = [
            EntityCandidate(
                text=value,
                entity_type="person"
                if any(term in value for term in ("ابن", "ابو", "أبو", "بنت"))
                else "unknown",
                confidence=0.55
                if any(term in value for term in ("ابن", "ابو", "أبو", "بنت"))
                else 0.35,
                rationale="مرشّح لغوي يحتاج مراجعة، وليس هوية مؤكدة.",
            )
            for value in values[:50]
        ]
        return EntityExtractionResponse(entities=entities)

    def extract_claims(self, request: ExtractionRequest) -> ClaimExtractionResponse:
        claims: list[ClaimCandidate] = []
        for sentence in _SENTENCES.findall(request.text):
            clean = _SPACES.sub(" ", sentence).strip()
            lowered = normalized_text(clean)
            if not any(term in lowered for term in _RELATION_TERMS):
                continue
            predicate = next((term for term in _RELATION_TERMS if term in clean), "")
            if not predicate:
                predicate = next((term for term in _RELATION_TERMS if term in lowered), "ذكر")
            subject, object_text = relation_parts(clean, predicate)
            claims.append(
                ClaimCandidate(
                    subject_text=subject[:500],
                    predicate=predicate,
                    object_text=object_text[:500] if object_text else None,
                    confidence=0.45,
                    rationale="علاقة مستخرجة من مؤشرات لغوية؛ يلزمها مراجعة ومصدر.",
                )
            )
        return ClaimExtractionResponse(claims=claims[:50])

    def resolve_entity(self, request: EntityResolutionRequest) -> EntityResolutionResponse:
        query = normalized_text(request.name)
        query_tokens = tokenize(request.name)
        matches: list[EntityMatch] = []
        for candidate in request.candidates:
            canonical = normalized_text(candidate.name)
            aliases = {normalized_text(alias) for alias in candidate.aliases}
            if canonical == query:
                score, matched_on = 1.0, "canonical"
            elif query in aliases:
                score, matched_on = 0.95, "alias"
            elif query_tokens & tokenize(candidate.name):
                score, matched_on = (
                    clamp(len(query_tokens & tokenize(candidate.name)) / max(len(query_tokens), 1)),
                    "token_overlap",
                )
            else:
                score, matched_on = 0.0, "none"
            matches.append(
                EntityMatch(
                    candidate_id=candidate.id,
                    candidate_name=candidate.name,
                    score=score,
                    matched_on=matched_on,
                )
            )
        matches.sort(key=lambda item: item.score, reverse=True)
        return EntityResolutionResponse(matches=matches[:20])

    def analyze_contradiction(self, request: ContradictionRequest) -> ContradictionResponse:
        pairs: list[ContradictionPair] = []
        for left, right in combinations(request.statements, 2):
            left_text = normalized_text(left.text)
            right_text = normalized_text(right.text)
            if any(term in left_text for term in _CONTRADICTION_TERMS) and any(
                term in right_text for term in _CONTRADICTION_TERMS
            ):
                left_tokens = tokenize(left.text)
                right_tokens = tokenize(right.text)
                if left_tokens != right_tokens:
                    pairs.append(
                        ContradictionPair(
                            left_id=left.id,
                            right_id=right.id,
                            confidence=0.55,
                            rationale="تعارض لغوي محتمل بين عبارتين؛ يحتاج إلى فحص المصادر.",
                        )
                    )
        return ContradictionResponse(has_candidate_contradiction=bool(pairs), pairs=pairs)

    def rerank(self, request: RerankRequest) -> RerankResponse:
        query_tokens = tokenize(request.query)
        documents = []
        for document in request.documents:
            document_tokens = tokenize(document.text)
            score = clamp(len(query_tokens & document_tokens) / max(len(query_tokens), 1))
            documents.append(
                RerankedDocument(id=document.id, score=score, excerpt=document.text[:1000])
            )
        documents.sort(key=lambda item: item.score, reverse=True)
        return RerankResponse(documents=documents)

    def research_query(self, request: ResearchQueryRequest) -> ResearchQueryResponse:
        # Ranking is unchanged and still counts every shared token, because the
        # order of the contexts is a retrieval decision and this function is not
        # where it is made. What changed is the filter below: a context becomes a
        # citation only when it shares a *content* token with the question, so a
        # preposition both texts happen to contain is no longer enough to cite a
        # source that says nothing about the question.
        query_tokens = tokenize(request.query)
        query_content = content_tokens(request.query)
        ranked = sorted(
            request.contexts, key=lambda item: len(query_tokens & tokenize(item.text)), reverse=True
        )
        # The filter runs before the five-citation cap, not after it, so a
        # context that shares only a function word cannot use up one of the five
        # slots and push out a source that does answer. The cap still applies to
        # the citations, which is what cit-005 measures: six supporting sources,
        # five returned.
        citations = [
            ResearchCitation(source_id=item.id, title=item.title, excerpt=item.text[:1000])
            for item in ranked
            if query_content & content_tokens(item.text)
        ][:5]
        answer = "هذه صياغة بحثية أولية وليست إجابة تاريخية؛ راجع المصادر المرتبطة قبل الاعتماد."
        if not citations:
            answer = "لم أجد سياقاً كافياً في المواد المرسلة؛ لا أستطيع بناء جواب موثوق من دون مصدر."
        return ResearchQueryResponse(answer=answer, citations=citations)


provider: ResearchProvider = DeterministicProvider()


@app.get("/healthz")
def healthz() -> dict[str, str]:
    return {"status": "ok", "service": "ai-research", "mode": "deterministic-foundation"}


@app.get("/readyz")
def readyz() -> dict[str, str]:
    return {"status": "ready", "service": "ai-research", "database": "not_checked"}


@app.post("/v1/normalize-name", response_model=NameNormalizationResponse)
def normalize_name(request: NameNormalizationRequest) -> NameNormalizationResponse:
    return NameNormalizationResponse(
        original=request.value, normalized=normalize_arabic_name(request.value)
    )


@app.post("/v1/embed", response_model=EmbeddingResponse)
def embed(request: EmbeddingRequest) -> EmbeddingResponse:
    values = provider.embed(request.text, request.dimensions)
    return EmbeddingResponse(embedding=values, dimensions=len(values))


@app.post("/v1/classify", response_model=ClassificationResponse)
def classify(request: ClassificationRequest) -> ClassificationResponse:
    return provider.classify(request)


@app.post("/v1/route", response_model=RoutingDecision)
def route(request: RoutingRequest) -> RoutingDecision:
    return provider.route(request)


@app.post("/v1/extract/entities", response_model=EntityExtractionResponse)
def extract_entities(request: ExtractionRequest) -> EntityExtractionResponse:
    return provider.extract_entities(request)


@app.post("/v1/extract/claims", response_model=ClaimExtractionResponse)
def extract_claims(request: ExtractionRequest) -> ClaimExtractionResponse:
    return provider.extract_claims(request)


@app.post("/v1/resolve/entity", response_model=EntityResolutionResponse)
def resolve_entity(request: EntityResolutionRequest) -> EntityResolutionResponse:
    return provider.resolve_entity(request)


@app.post("/v1/analyze/contradiction", response_model=ContradictionResponse)
def analyze_contradiction(request: ContradictionRequest) -> ContradictionResponse:
    return provider.analyze_contradiction(request)


@app.post("/v1/rerank", response_model=RerankResponse)
def rerank(request: RerankRequest) -> RerankResponse:
    return provider.rerank(request)


@app.post("/v1/research/query", response_model=ResearchQueryResponse)
def research_query(request: ResearchQueryRequest) -> ResearchQueryResponse:
    return provider.research_query(request)


@app.get("/")
def root() -> dict[str, str]:
    return {"service": "dawha-ai-research", "status": "foundation"}
