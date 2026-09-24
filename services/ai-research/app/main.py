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
_NAME_CONTEXT = re.compile(r"(?<![؀-ۿ])(?:ابن|أبو|ابو|بنت|بن)\s+[؀-ۿ]+(?:\s+[؀-ۿ]+)?")
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
_CONTRADICTION_TERMS = ("والد", "أبو", "ابو", "father", "والدة", "mother")

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


def tokenize(value: str) -> set[str]:
    return {token for token in re.findall(r"[\w؀-ۿ]+", normalized_text(value)) if token}


def clamp(value: float) -> float:
    return round(max(0.0, min(1.0, value)), 4)


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

    def extract_entities(self, request: ExtractionRequest) -> EntityExtractionResponse:
        values: list[str] = []
        for match in _NAME_CONTEXT.finditer(request.text):
            value = _SPACES.sub(" ", match.group(0)).strip()
            if value and value not in values:
                values.append(value)
        for match in _ARABIC_SEQUENCE.finditer(request.text):
            value = _SPACES.sub(" ", match.group(0)).strip()
            if len(value) >= 4 and value not in values:
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
        query_tokens = tokenize(request.query)
        ranked = sorted(
            request.contexts, key=lambda item: len(query_tokens & tokenize(item.text)), reverse=True
        )
        citations = [
            ResearchCitation(source_id=item.id, title=item.title, excerpt=item.text[:1000])
            for item in ranked[:5]
            if query_tokens & tokenize(item.text)
        ]
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
