from fastapi.testclient import TestClient

from app.main import app

client = TestClient(app)


def test_healthz() -> None:
    response = client.get("/healthz")

    assert response.status_code == 200
    assert response.json()["status"] == "ok"


def test_normalize_name_preserves_original() -> None:
    response = client.post("/v1/normalize-name", json={"value": "  عَبْدُ الله  "})

    assert response.status_code == 200
    body = response.json()
    assert body["original"] == "  عَبْدُ الله  "
    assert body["normalized"] == "عبد الله"
    assert body["preserves_original"] is True


def test_embed_is_deterministic_and_structured() -> None:
    first = client.post("/v1/embed", json={"text": "عبدالله", "dimensions": 32})
    second = client.post("/v1/embed", json={"text": " عبدالله ", "dimensions": 32})

    assert first.status_code == 200
    assert first.json() == second.json()
    assert len(first.json()["embedding"]) == 32
    assert all(-1 <= value <= 1 for value in first.json()["embedding"])
    assert first.json()["deterministic"] is True


def test_candidate_extraction_requires_review() -> None:
    response = client.post(
        "/v1/extract/entities",
        json={"text": "ذكر أبو بكر وعبدالله بن محمد في السجل."},
    )

    assert response.status_code == 200
    body = response.json()
    assert body["entities"]
    assert body["review_required"] is True
    assert all(item["status"] in {"unreviewed", "needs_review"} for item in body["entities"])


def test_contradiction_and_research_queries_are_grounded() -> None:
    contradiction = client.post(
        "/v1/analyze/contradiction",
        json={
            "statements": [
                {"id": "a", "text": "يقول المصدر إن والد عبدالله محمد."},
                {"id": "b", "text": "يقول المصدر الآخر إن والد عبدالله صالح."},
            ]
        },
    )
    research = client.post(
        "/v1/research/query",
        json={
            "query": "من كان والد عبدالله؟",
            "contexts": [{"id": "s1", "title": "مصدر", "text": "إن والد عبدالله محمد."}],
        },
    )

    assert contradiction.status_code == 200
    assert contradiction.json()["has_candidate_contradiction"] is True
    assert contradiction.json()["pairs"][0]["status"] == "needs_review"
    assert research.status_code == 200
    assert research.json()["citations"][0]["source_id"] == "s1"
    assert research.json()["review_required"] is True


def test_invalid_classification_labels_are_rejected() -> None:
    response = client.post("/v1/classify", json={"text": "نص", "labels": ["موضوع", "موضوع"]})

    assert response.status_code == 422


def test_remaining_structured_outputs_are_reviewable() -> None:
    classification = client.post(
        "/v1/classify",
        json={"text": "ذكر محمد", "labels": ["شخص", "مكان"]},
    )
    claims = client.post(
        "/v1/extract/claims",
        json={"text": "يقول المصدر إن والد محمد صالح."},
    )
    resolution = client.post(
        "/v1/resolve/entity",
        json={
            "name": "محمد",
            "candidates": [{"id": "p1", "name": "محمد", "aliases": ["أبو محمد"]}],
        },
    )
    rerank = client.post(
        "/v1/rerank",
        json={"query": "محمد", "documents": [{"id": "s1", "text": "ذكر محمد"}]},
    )

    assert classification.status_code == 200
    assert classification.json()["candidates"]
    assert claims.status_code == 200
    assert claims.json()["claims"][0]["status"] == "needs_review"
    assert claims.json()["claims"][0]["object_text"]
    assert resolution.status_code == 200
    assert resolution.json()["matches"][0]["candidate_id"] == "p1"
    assert rerank.status_code == 200
    assert rerank.json()["documents"][0]["id"] == "s1"


def test_blank_name_is_rejected() -> None:
    response = client.post("/v1/normalize-name", json={"value": "   "})

    assert response.status_code == 422
