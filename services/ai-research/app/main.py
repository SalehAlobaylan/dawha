from __future__ import annotations

import re
import unicodedata
from typing import Literal

from fastapi import FastAPI
from pydantic import BaseModel, Field

app = FastAPI(
    title="Dawha Research API",
    version="0.1.0",
    description="Research assistance that never decides historical truth.",
)

_DIACRITICS = re.compile(r"[\u064B-\u065F\u0670\u06D6-\u06ED]")
_SPACES = re.compile(r"\s+")


class NameNormalizationRequest(BaseModel):
    value: str = Field(min_length=1, max_length=500)


class NameNormalizationResponse(BaseModel):
    original: str
    normalized: str
    method: Literal["deterministic_arabic_normalization"] = "deterministic_arabic_normalization"
    preserves_original: Literal[True] = True


def normalize_arabic_name(value: str) -> str:
    normalized = unicodedata.normalize("NFKC", value.strip())
    normalized = _DIACRITICS.sub("", normalized)
    normalized = normalized.replace("أ", "ا").replace("إ", "ا").replace("آ", "ا")
    normalized = normalized.replace("ى", "ي").replace("ة", "ه")
    normalized = _SPACES.sub(" ", normalized)
    return normalized.strip()


@app.get("/healthz")
def healthz() -> dict[str, str]:
    return {"status": "ok", "service": "ai-research", "mode": "deterministic-foundation"}


@app.get("/readyz")
def readyz() -> dict[str, str]:
    return {"status": "ready", "service": "ai-research", "database": "not_checked"}


@app.post("/v1/normalize-name", response_model=NameNormalizationResponse)
def normalize_name(request: NameNormalizationRequest) -> NameNormalizationResponse:
    return NameNormalizationResponse(
        original=request.value,
        normalized=normalize_arabic_name(request.value),
    )


@app.get("/")
def root() -> dict[str, str]:
    return {"service": "dawha-ai-research", "status": "foundation"}
