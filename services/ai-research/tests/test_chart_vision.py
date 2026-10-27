from __future__ import annotations

import asyncio
import base64

import httpx
import pytest
from fastapi import FastAPI
from fastapi.testclient import TestClient
from pydantic import ValidationError

from app.chart_vision import (
    CandidateResponse,
    ChartBox,
    LocateResponse,
    NameReading,
    ReadBatchResponse,
    _validation_detail,
    create_chart_vision_router,
)


class StubVision:
    async def locate(self, image: bytes, tile_id: str) -> LocateResponse:
        assert image.startswith(b"\x89PNG")
        assert tile_id == "tile_1"
        return LocateResponse.model_validate(
            {
                "branches": [],
                "labels": [
                    {
                        "box": {"x": 0.1, "y": 0.2, "width": 0.1, "height": 0.1},
                        "branch_index": None,
                        "legibility": "uncertain",
                    }
                ],
                "connectors": [],
                "residual_regions": [],
                "completeness_note": "review required",
            }
        )

    async def read(self, items: list[tuple[str, bytes, bytes]]) -> ReadBatchResponse:
        return ReadBatchResponse.model_validate(
            {
                "readings": [
                    {
                        "occurrence_id": item[0],
                        "status": "partial",
                        "text_ar": "عبد...",
                        "alternatives": ["عبيد"],
                        "uncertain_spans": ["... "],
                    }
                    for item in items
                ]
            }
        )

    async def suggest(self, image: bytes):
        assert image.startswith(b"\x89PNG")
        return CandidateResponse.model_validate(
            {"candidates": ["محمد", "أحمد", "محمد", " "]}
        )


def png(width: int = 1, height: int = 1) -> str:
    # PNG signature + IHDR. The route checks shape and size before the injected adapter.
    raw = (
        b"\x89PNG\r\n\x1a\n"
        + b"\x00\x00\x00\rIHDR"
        + width.to_bytes(4, "big")
        + height.to_bytes(4, "big")
        + b"\x08\x02\x00\x00\x00"
    )
    return base64.b64encode(raw).decode("ascii")


def test_validation_diagnostics_report_schema_path_without_echoing_values() -> None:
    with pytest.raises(ValidationError) as caught:
        ChartBox.model_validate({"x": 1.2, "y": 0.1, "width": 0.1, "height": 0.1})

    detail = _validation_detail("vision_geometry_invalid", caught.value)
    assert detail.startswith("vision_geometry_invalid:x:")
    assert "less_than_equal" in detail
    assert "1.2" not in detail


def test_chart_locator_accepts_normalized_geometry_and_rejects_out_of_range() -> None:
    app = FastAPI()
    app.include_router(create_chart_vision_router(StubVision()))
    client = TestClient(app)

    good = client.post("/v1/vision/chart/locate", json={"tile_id": "tile_1", "image_base64": png()})
    bad = client.post("/v1/vision/chart/locate", json={"tile_id": "tile_1"})
    assert good.status_code == 200
    assert good.json()["labels"][0]["legibility"] == "uncertain"
    assert bad.status_code == 422

    import pytest
    from pydantic import ValidationError

    from app.chart_vision import ChartBox

    with pytest.raises(ValidationError):
        ChartBox.model_validate({"x": 0.8, "y": 0.2, "width": 0.4, "height": 0.1})


def test_chart_reader_requires_exact_unique_occurrences() -> None:
    app = FastAPI()
    app.include_router(create_chart_vision_router(StubVision()))
    client = TestClient(app)
    payload = {
        "items": [
            {
                "occurrence_id": "name_1",
                "tight_image_base64": png(),
                "context_image_base64": png(),
            }
        ]
    }

    response = client.post("/v1/vision/chart/read", json=payload)
    duplicate = client.post(
        "/v1/vision/chart/read", json={"items": [payload["items"][0], payload["items"][0]]}
    )
    assert response.status_code == 200
    assert response.json()["readings"][0]["status"] == "partial"
    assert duplicate.status_code == 422


def test_name_candidate_endpoint_returns_a_deduplicated_bounded_list() -> None:
    app = FastAPI()
    app.include_router(create_chart_vision_router(StubVision()))
    client = TestClient(app)

    response = client.post(
        "/v1/vision/chart/candidates", json={"image_base64": png()}
    )
    assert response.status_code == 200
    # The endpoint's value is a short list a person can choose from, not a
    # second authoritative reading.
    assert response.json()["candidates"] == ["محمد", "أحمد"]


def test_chart_vision_refuses_non_png_and_overlarge_image() -> None:
    app = FastAPI()
    app.include_router(create_chart_vision_router(StubVision()))
    client = TestClient(app)

    bad = client.post(
        "/v1/vision/chart/locate",
        json={
            "tile_id": "tile_1",
            "image_base64": base64.b64encode(b"not png data!!!!!!!").decode(),
        },
    )
    huge = client.post(
        "/v1/vision/chart/locate", json={"tile_id": "tile_1", "image_base64": png(5000, 5000)}
    )
    assert bad.status_code == 422
    assert huge.status_code == 422


def test_readable_output_must_contain_visible_text() -> None:
    import pytest
    from pydantic import ValidationError

    with pytest.raises(ValidationError):
        NameReading.model_validate(
            {
                "occurrence_id": "name_1",
                "status": "readable",
                "text_ar": "",
                "alternatives": [],
                "uncertain_spans": [],
            }
        )


def test_partial_and_unreadable_labels_cannot_hide_missing_text() -> None:
    import pytest
    from pydantic import ValidationError

    with pytest.raises(ValidationError):
        NameReading.model_validate(
            {
                "occurrence_id": "name_1",
                "status": "partial",
                "text_ar": "",
                "alternatives": [],
                "uncertain_spans": [],
            }
        )
    with pytest.raises(ValidationError):
        NameReading.model_validate(
            {
                "occurrence_id": "name_1",
                "status": "unreadable",
                "text_ar": "سالم",
                "alternatives": [],
                "uncertain_spans": [],
            }
        )


def test_deepseek_request_uses_inline_original_images_and_documented_user_isolation() -> None:
    import json

    from app.chart_vision import LOCATE_SCHEMA, MODEL, DeepSeekChartVision

    observed = {}

    def handler(request: httpx.Request) -> httpx.Response:
        observed["url"] = str(request.url)
        observed["authorization"] = request.headers["authorization"]
        observed["payload"] = json.loads(request.content)
        observed["calls"] = observed.get("calls", 0) + 1
        return httpx.Response(
            200,
            json={"choices": [{"finish_reason": "stop", "message": {"content": "{}"}}]},
        )

    adapter = DeepSeekChartVision(
        api_key="test-only-secret", transport=httpx.MockTransport(handler)
    )
    assert (
        asyncio.run(
            adapter._interact(
                "blind visual pass", [b"png bytes"], LOCATE_SCHEMA, 16384, "tile_private_id"
            )
        )
        == "{}"
    )

    assert observed["url"] == "https://api.deepseek.com/chat/completions"
    assert observed["authorization"] == "Bearer test-only-secret"
    assert observed["calls"] == 1
    payload = observed["payload"]
    assert payload["model"] == MODEL == "deepseek-flash"
    assert payload["messages"][0]["role"] == "system"
    assert "blind visual pass" in payload["messages"][0]["content"]
    assert "JSON Schema" in payload["messages"][0]["content"]
    assert payload["response_format"] == {"type": "json_object"}
    assert payload["max_tokens"] == 16384
    assert payload["thinking"] == {"type": "disabled"}
    assert "reasoning_effort" not in payload
    assert "tools" not in payload
    image = payload["messages"][1]["content"][1]
    assert image == {
        "type": "image_url",
        "image_url": {"url": "data:image/png;base64,cG5nIGJ5dGVz", "detail": "original"},
    }
    assert len(payload["user_id"]) == 64
    assert "tile_private_id" not in payload["user_id"]
    assert "store" not in payload


def test_deepseek_length_finish_reason_is_explicitly_reported_as_truncation() -> None:
    import pytest
    from fastapi import HTTPException

    from app.chart_vision import DeepSeekChartVision

    def handler(request: httpx.Request) -> httpx.Response:
        return httpx.Response(
            200,
            json={"choices": [{"finish_reason": "length", "message": {"content": "{"}}]},
        )

    adapter = DeepSeekChartVision(api_key="test-only", transport=httpx.MockTransport(handler))
    with pytest.raises(HTTPException, match="truncated"):
        asyncio.run(
            adapter._interact(
                "blind visual pass", [b"png bytes"], {"type": "object"}, 1024, "tile_1"
            )
        )


def test_deepseek_refusal_empty_and_unfinished_responses_are_blocked() -> None:
    import pytest
    from fastapi import HTTPException

    from app.chart_vision import DeepSeekChartVision

    responses = [
        {"choices": [{"finish_reason": "content_filter", "message": {"content": ""}}]},
        {"choices": [{"finish_reason": "stop", "message": {"content": ""}}]},
        {"choices": []},
        {"choices": [{"finish_reason": "stop", "message": {"refusal": "refused"}}]},
    ]
    for body in responses:
        adapter = DeepSeekChartVision(
            api_key="test-only",
            transport=httpx.MockTransport(
                lambda request, body=body: httpx.Response(200, json=body)
            ),
        )
        with pytest.raises(HTTPException):
            asyncio.run(adapter._interact("prompt", [b"png"], {"type": "object"}, 100, "tile"))


def test_deepseek_credentials_and_transient_errors_are_reported_without_provider_body() -> None:
    import pytest
    from fastapi import HTTPException

    from app.chart_vision import DeepSeekChartVision

    missing = DeepSeekChartVision(api_key="")
    with pytest.raises(HTTPException) as no_key:
        asyncio.run(missing._interact("prompt", [b"png"], {"type": "object"}, 100, "tile"))
    assert no_key.value.status_code == 503

    for status, expected in ((401, 503), (429, 502)):
        adapter = DeepSeekChartVision(
            api_key="test-only",
            transport=httpx.MockTransport(
                lambda request, status=status: httpx.Response(status, text="secret provider body")
            ),
        )
        with pytest.raises(HTTPException) as error:
            asyncio.run(adapter._interact("prompt", [b"png"], {"type": "object"}, 100, "tile"))
        assert error.value.status_code == expected
        assert "secret provider body" not in str(error.value.detail)


def test_candidate_cache_scope_is_per_crop_not_a_shared_constant() -> None:
    # The provider's user_id is a cache-isolation field. Candidate calls must not
    # share one prefix scope, or a crop could reuse another name's cached context.
    import json

    from app.chart_vision import DeepSeekChartVision

    observed: list[str] = []

    def handler(request: httpx.Request) -> httpx.Response:
        payload = json.loads(request.content)
        observed.append(payload["user_id"])
        return httpx.Response(
            200,
            json={
                "choices": [
                    {
                        "finish_reason": "stop",
                        "message": {"content": json.dumps({"candidates": ["جاسم"]})},
                    }
                ]
            },
        )

    adapter = DeepSeekChartVision(
        api_key="test-only", transport=httpx.MockTransport(handler)
    )
    first = asyncio.run(adapter.suggest(b"crop one"))
    second = asyncio.run(adapter.suggest(b"crop two"))
    third = asyncio.run(adapter.suggest(b"crop one"))

    assert first.candidates == ["جاسم"]
    assert second.candidates == ["جاسم"]
    assert third.candidates == ["جاسم"]
    assert len(observed) == 3
    assert all(len(value) == 64 for value in observed)
    assert observed[0] == observed[2]
    assert observed[0] != observed[1]
