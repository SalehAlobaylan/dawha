from __future__ import annotations

import asyncio
import base64
import json

import httpx
import pytest
from fastapi import FastAPI, HTTPException
from fastapi.testclient import TestClient
from pydantic import ValidationError

from app.name_judge import (
    LIQUID_PROMPT_VERSION,
    JevNameJudge,
    JudgeCandidate,
    JudgeCrop,
    JudgeRequest,
    JudgeResponse,
    JudgeState,
    LiquidNameJudge,
    configured_name_judge,
    create_name_judge_router,
    name_judge_images_enabled,
)

PNG = (
    b"\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"
    + (10).to_bytes(4, "big")
    + (10).to_bytes(4, "big")
    + b"\x08\x06\x00\x00\x00"
)
IMAGE_BASE64 = base64.b64encode(PNG).decode("ascii")


def judge_request(
    *,
    values: tuple[str, ...] = ("محمد", "أحمد"),
    image: bool = False,
    mode: str = "shadow",
) -> JudgeRequest:
    candidates = [
        JudgeCandidate(
            value=value,
            source="reader" if index == 1 else "common",
            rank=index,
        )
        for index, value in enumerate(values, start=1)
    ]
    return JudgeRequest(
        state=JudgeState(
            reading="محمد",
            alternatives=["أحمد"],
            file_context=["عبدالله"],
            crop=JudgeCrop(
                width_px=120,
                height_px=48,
                source_width_px=600,
                source_height_px=400,
                name_height_px=32,
            ),
        ),
        candidates=candidates,
        mode=mode,
        image_base64=IMAGE_BASE64 if image else None,
    )


def provider_response(
    *,
    choice: str = "c1",
    probabilities: dict[str, float] | None = None,
    confidence: float = 0.61,
    readable: bool | None = None,
    extra: dict[str, object] | None = None,
) -> httpx.Response:
    body = {
        "choice": choice,
        "probabilities": probabilities
        if probabilities is not None
        else {"c1": 0.61, "c2": 0.3, "unclear": 0.09},
        "confidence": confidence,
        "readable": readable,
    }
    if extra:
        body.update(extra)
    return httpx.Response(
        200,
        json={
            "choices": [
                {
                    "finish_reason": "stop",
                    "message": {"content": json.dumps(body, ensure_ascii=False)},
                }
            ]
        },
    )


def liquid_response(
    *,
    choice: str = "c1",
    probabilities: dict[str, float] | None = None,
    confidence: float = 0.61,
    readable: bool | None = None,
    answer_type: str = "choice",
) -> httpx.Response:
    answers: dict[str, object] = {
        "choice": {
            "type": answer_type,
            "choice": choice,
            "probabilities": probabilities
            if probabilities is not None
            else {"c1": 0.61, "c2": 0.3, "unclear": 0.09},
            "confidence": confidence,
        }
    }
    if readable is not None:
        answers["readable"] = {
            "type": "noul",
            "noul": 1.0 if readable else 0.0,
        }
    return httpx.Response(
        200,
        json={
            "model": "d1:3b",
            "answers": answers,
            "usage": {"input_tokens": 10, "output_tokens": 2},
        },
    )


def test_jev_request_uses_option_keys_and_maps_the_choice_back() -> None:
    observed: dict[str, object] = {}

    def handler(request: httpx.Request) -> httpx.Response:
        observed["url"] = str(request.url)
        observed["authorization"] = request.headers["authorization"]
        observed["payload"] = json.loads(request.content)
        return provider_response(choice="c2")

    adapter = JevNameJudge(
        api_key="test-only-secret",
        base_url="https://jev.test/v1/chat/completions",
        model="jev-pinned",
        prompt_version="name-judge-v1",
        transport=httpx.MockTransport(handler),
    )
    result = asyncio.run(adapter.judge(judge_request()))

    assert observed["url"] == "https://jev.test/v1/chat/completions"
    assert observed["authorization"] == "Bearer test-only-secret"
    payload = observed["payload"]
    assert payload["model"] == "jev-pinned"
    assert payload["temperature"] == 0
    prompt = payload["messages"][1]["content"]
    assert '"key":"c1"' in prompt
    assert '"key":"c2"' in prompt
    assert "source_id" not in json.dumps(payload)
    assert "family_id" not in json.dumps(payload)
    assert "account_id" not in json.dumps(payload)
    assert "reviewer_id" not in json.dumps(payload)
    assert '"image_mode":false' in prompt

    assert result.choice == "أحمد"
    assert result.abstained is False
    assert result.probabilities == {"محمد": 0.61, "أحمد": 0.3, "unclear": 0.09}
    assert result.confidence == 0.61
    assert result.readable is None
    assert result.provider == "jev"
    assert result.model == "jev-pinned"
    assert result.prompt_version == "name-judge-v1"
    assert result.cache_hit is False


def test_an_out_of_options_choice_is_rejected() -> None:
    adapter = JevNameJudge(
        api_key="test-only",
        base_url="https://jev.test/v1/chat/completions",
        model="jev-pinned",
        transport=httpx.MockTransport(lambda request: provider_response(choice="c3")),
    )
    with pytest.raises(HTTPException) as error:
        asyncio.run(adapter.judge(judge_request()))
    assert error.value.status_code == 422
    assert "outside the options" in str(error.value.detail)


def test_duplicate_missing_and_out_of_range_probabilities_are_rejected() -> None:
    cases = [
        {"c1": 0.7, "c2": 0.3},  # missing unclear
        {"c1": 0.7, "unclear": 0.1},  # missing c2
        {"c1": 1.2, "c2": -0.1, "unclear": -0.1},
    ]
    for probabilities in cases:
        adapter = JevNameJudge(
            api_key="test-only",
            base_url="https://jev.test/v1/chat/completions",
            model="jev-pinned",
            transport=httpx.MockTransport(
                lambda request, p=probabilities: provider_response(probabilities=p)
            ),
        )
        with pytest.raises(HTTPException) as error:
            asyncio.run(adapter.judge(judge_request()))
        assert error.value.status_code == 422


def test_confidence_and_free_text_are_rejected() -> None:
    adapter = JevNameJudge(
        api_key="test-only",
        base_url="https://jev.test/v1/chat/completions",
        model="jev-pinned",
        transport=httpx.MockTransport(
            lambda request: provider_response(confidence=1.2)
        ),
    )
    with pytest.raises(HTTPException) as error:
        asyncio.run(adapter.judge(judge_request()))
    assert error.value.status_code == 422

    adapter = JevNameJudge(
        api_key="test-only",
        base_url="https://jev.test/v1/chat/completions",
        model="jev-pinned",
        transport=httpx.MockTransport(
            lambda request: provider_response(
                extra={"explanation": "أعتقد أن الاسم محمد"}
            )
        ),
    )
    with pytest.raises(HTTPException) as error:
        asyncio.run(adapter.judge(judge_request()))
    assert error.value.status_code == 422


def test_timeout_and_5xx_are_provider_unavailable() -> None:
    def timeout_handler(request: httpx.Request) -> httpx.Response:
        raise httpx.TimeoutException("timed out", request=request)

    timeout_adapter = JevNameJudge(
        api_key="test-only",
        base_url="https://jev.test/v1/chat/completions",
        model="jev-pinned",
        transport=httpx.MockTransport(timeout_handler),
    )
    with pytest.raises(HTTPException) as error:
        asyncio.run(timeout_adapter.judge(judge_request()))
    assert error.value.status_code == 503

    server_adapter = JevNameJudge(
        api_key="test-only",
        base_url="https://jev.test/v1/chat/completions",
        model="jev-pinned",
        transport=httpx.MockTransport(
            lambda request: httpx.Response(500, text="provider secret body")
        ),
    )
    with pytest.raises(HTTPException) as error:
        asyncio.run(server_adapter.judge(judge_request()))
    assert error.value.status_code == 503
    assert "provider secret body" not in str(error.value.detail)


def test_missing_credentials_are_a_setup_error_not_a_guess() -> None:
    adapter = JevNameJudge(
        api_key="",
        base_url="https://jev.test/v1/chat/completions",
        model="jev-pinned",
    )
    with pytest.raises(HTTPException) as error:
        asyncio.run(adapter.judge(judge_request()))
    assert error.value.status_code == 503
    assert "JEV_API_KEY" in str(error.value.detail)


def test_latest_model_is_rejected() -> None:
    adapter = JevNameJudge(
        api_key="test-only",
        base_url="https://jev.test/v1/chat/completions",
        model="latest",
    )
    with pytest.raises(HTTPException) as error:
        asyncio.run(adapter.judge(judge_request()))
    assert error.value.status_code == 503


def test_the_same_state_and_option_set_is_cached_without_crossing_candidate_sets() -> None:
    calls = 0

    def handler(request: httpx.Request) -> httpx.Response:
        nonlocal calls
        calls += 1
        return provider_response()

    adapter = JevNameJudge(
        api_key="test-only",
        base_url="https://jev.test/v1/chat/completions",
        model="jev-pinned",
        transport=httpx.MockTransport(handler),
    )
    first = asyncio.run(adapter.judge(judge_request()))
    second = asyncio.run(adapter.judge(judge_request()))
    assert calls == 1
    assert first.cache_hit is False
    assert second.cache_hit is True

    asyncio.run(adapter.judge(judge_request(values=("محمد", "محمود"))))
    assert calls == 2


def test_liquid_request_uses_systemone_choice_keys_and_maps_the_choice_back() -> None:
    observed: dict[str, object] = {}

    def handler(request: httpx.Request) -> httpx.Response:
        observed["url"] = str(request.url)
        observed["authorization"] = request.headers.get("authorization")
        observed["payload"] = json.loads(request.content)
        return liquid_response(choice="c2")

    adapter = LiquidNameJudge(
        base_url="http://127.0.0.1:11434/v1/systemone",
        model="d1:3b",
        prompt_version="name-judge-liquid-v1",
        transport=httpx.MockTransport(handler),
    )
    result = asyncio.run(adapter.judge(judge_request()))

    assert observed["url"] == "http://127.0.0.1:11434/v1/systemone"
    assert observed["authorization"] is None
    payload = observed["payload"]
    assert payload["model"] == "d1:3b"
    state = payload["state"]
    assert state["state"]["reading"] == "محمد"
    assert state["candidates"][1] == {
        "key": "c2",
        "value": "أحمد",
        "source": "common",
        "rank": 2,
    }
    assert state["image_mode"] is False
    questions = payload["questions"]
    assert set(questions) == {"choice"}
    assert questions["choice"]["type"] == "choice"
    assert set(questions["choice"]["criteria"]) == {"c1", "c2", "unclear"}
    assert "unclear" in questions["choice"]["instructions"]
    assert "images" not in payload
    encoded = json.dumps(payload)
    for forbidden in (
        "source_id",
        "sourceId",
        "family_id",
        "account_id",
        "reviewer_id",
        "page_image",
    ):
        assert forbidden not in encoded

    assert result.choice == "أحمد"
    assert result.abstained is False
    assert result.probabilities == {"محمد": 0.61, "أحمد": 0.3, "unclear": 0.09}
    assert result.confidence == 0.61
    assert result.readable is None
    assert result.provider == "liquid"
    assert result.model == "d1:3b"
    assert result.prompt_version == "name-judge-liquid-v1"
    assert result.cache_hit is False


def test_liquid_unclear_is_an_abstention_not_a_name() -> None:
    adapter = LiquidNameJudge(
        base_url="http://127.0.0.1:11434/v1/systemone",
        model="d1:3b",
        transport=httpx.MockTransport(
            lambda request: liquid_response(
                choice="unclear",
                probabilities={"c1": 0.1, "c2": 0.2, "unclear": 0.7},
                confidence=0.7,
            )
        ),
    )
    result = asyncio.run(adapter.judge(judge_request()))
    assert result.abstained is True
    assert result.choice is None
    assert result.probabilities["unclear"] == 0.7


def test_liquid_rejects_a_choice_outside_the_options_and_bad_numbers() -> None:
    for response in (
        liquid_response(choice="c3"),
        liquid_response(choice="خالد"),
        liquid_response(probabilities={"c1": 0.7, "c2": 0.3}),
        liquid_response(probabilities={"c1": 0.7, "c2": 0.2, "c3": 0.1, "unclear": 0.0}),
        liquid_response(probabilities={"c1": 1.2, "c2": -0.1, "unclear": -0.1}),
        liquid_response(probabilities={"c1": True, "c2": 0.3, "unclear": 0.7}),
        liquid_response(confidence=1.2),
    ):
        adapter = LiquidNameJudge(
            base_url="http://127.0.0.1:11434/v1/systemone",
            model="d1:3b",
            transport=httpx.MockTransport(lambda request, body=response: body),
        )
        with pytest.raises(HTTPException) as error:
            asyncio.run(adapter.judge(judge_request()))
        assert error.value.status_code == 422


def test_liquid_rejects_malformed_answers() -> None:
    malformed = [
        {"model": "d1:3b"},
        {"model": "d1:3b", "answers": {}},
        {"model": "d1:3b", "answers": {"choice": {"type": "noul", "noul": 0.5}}},
        {
            "model": "d1:3b",
            "answers": {
                "choice": {
                    "type": "choice",
                    "choice": "c1",
                    "probabilities": {"c1": 0.61, "c2": 0.3, "unclear": 0.09},
                    "confidence": 0.61,
                },
                "readable": {"type": "noul", "noul": "yes"},
            },
        },
    ]
    for body in malformed:
        adapter = LiquidNameJudge(
            base_url="http://127.0.0.1:11434/v1/systemone",
            model="d1:3b",
            transport=httpx.MockTransport(
                lambda request, payload=body: httpx.Response(200, json=payload)
            ),
        )
        with pytest.raises(HTTPException) as error:
            asyncio.run(adapter.judge(judge_request(image=True)))
        assert error.value.status_code == 422


def test_liquid_image_mode_sends_only_raw_base64_and_asks_readability() -> None:
    observed: dict[str, object] = {}

    def handler(request: httpx.Request) -> httpx.Response:
        observed["payload"] = json.loads(request.content)
        return liquid_response(readable=True)

    adapter = LiquidNameJudge(
        base_url="http://127.0.0.1:11434/v1/systemone",
        model="d1:3b",
        transport=httpx.MockTransport(handler),
    )
    result = asyncio.run(adapter.judge(judge_request(image=True)))
    payload = observed["payload"]
    assert payload["images"] == [IMAGE_BASE64]
    assert "data:image" not in json.dumps(payload)
    assert payload["state"]["image_mode"] is True
    assert payload["questions"]["readable"]["type"] == "noul"
    assert set(payload["questions"]["readable"]["criteria"]) == {"false", "true"}
    assert "context_image" not in json.dumps(payload)
    assert "source_id" not in json.dumps(payload)
    assert result.readable is True


def test_liquid_timeout_5xx_and_malformed_json_are_unavailable() -> None:
    def timeout_handler(request: httpx.Request) -> httpx.Response:
        raise httpx.TimeoutException("timed out", request=request)

    timeout_adapter = LiquidNameJudge(
        base_url="http://127.0.0.1:11434/v1/systemone",
        model="d1:3b",
        transport=httpx.MockTransport(timeout_handler),
    )
    with pytest.raises(HTTPException) as error:
        asyncio.run(timeout_adapter.judge(judge_request()))
    assert error.value.status_code == 503

    server_adapter = LiquidNameJudge(
        base_url="http://127.0.0.1:11434/v1/systemone",
        model="d1:3b",
        transport=httpx.MockTransport(
            lambda request: httpx.Response(500, text="provider secret body")
        ),
    )
    with pytest.raises(HTTPException) as error:
        asyncio.run(server_adapter.judge(judge_request()))
    assert error.value.status_code == 503
    assert "provider secret body" not in str(error.value.detail)

    malformed_adapter = LiquidNameJudge(
        base_url="http://127.0.0.1:11434/v1/systemone",
        model="d1:3b",
        transport=httpx.MockTransport(
            lambda request: httpx.Response(200, text="not json")
        ),
    )
    with pytest.raises(HTTPException) as error:
        asyncio.run(malformed_adapter.judge(judge_request()))
    assert error.value.status_code == 422


def test_liquid_needs_a_base_url_and_pinned_model_but_no_local_key() -> None:
    adapter = LiquidNameJudge(base_url="", model="d1:3b")
    with pytest.raises(HTTPException) as error:
        asyncio.run(adapter.judge(judge_request()))
    assert error.value.status_code == 503
    assert "LIQUID_BASE_URL" in str(error.value.detail)

    adapter = LiquidNameJudge(
        base_url="http://127.0.0.1:11434/v1/systemone",
        model="latest",
    )
    with pytest.raises(HTTPException) as error:
        asyncio.run(adapter.judge(judge_request()))
    assert error.value.status_code == 503
    assert "LIQUID_MODEL" in str(error.value.detail)


def test_the_configured_provider_defaults_to_liquid_and_can_still_select_jev(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.delenv("NAME_JUDGE_PROVIDER", raising=False)
    monkeypatch.delenv("LIQUID_BASE_URL", raising=False)
    monkeypatch.delenv("LIQUID_MODEL", raising=False)
    monkeypatch.delenv("LIQUID_API_KEY", raising=False)
    adapter = configured_name_judge()
    assert isinstance(adapter, LiquidNameJudge)
    assert adapter.base_url == "http://127.0.0.1:11434/v1/systemone"
    assert adapter.model == "d1:3b"
    assert adapter.prompt_version == LIQUID_PROMPT_VERSION
    assert adapter.api_key == ""

    monkeypatch.setenv("NAME_JUDGE_PROVIDER", "jev")
    assert isinstance(configured_name_judge(), JevNameJudge)

    monkeypatch.setenv("NAME_JUDGE_PROVIDER", "unknown")
    unsupported = configured_name_judge()
    with pytest.raises(HTTPException) as error:
        asyncio.run(unsupported.judge(judge_request()))
    assert error.value.status_code == 503
    assert "NAME_JUDGE_PROVIDER" in str(error.value.detail)


def test_liquid_cache_is_scoped_to_the_bounded_candidate_set() -> None:
    calls = 0

    def handler(request: httpx.Request) -> httpx.Response:
        nonlocal calls
        calls += 1
        return liquid_response()

    adapter = LiquidNameJudge(
        base_url="http://127.0.0.1:11434/v1/systemone",
        model="d1:3b",
        transport=httpx.MockTransport(handler),
    )
    first = asyncio.run(adapter.judge(judge_request()))
    second = asyncio.run(adapter.judge(judge_request()))
    assert calls == 1
    assert first.cache_hit is False
    assert second.cache_hit is True
    assert second.provider == "liquid"
    asyncio.run(adapter.judge(judge_request(values=("محمد", "محمود"))))
    assert calls == 2


def test_image_mode_sends_only_the_tight_crop_when_enabled() -> None:
    observed: dict[str, object] = {}

    def handler(request: httpx.Request) -> httpx.Response:
        observed["payload"] = json.loads(request.content)
        return provider_response(readable=True)

    adapter = JevNameJudge(
        api_key="test-only",
        base_url="https://jev.test/v1/chat/completions",
        model="jev-pinned",
        transport=httpx.MockTransport(handler),
    )
    result = asyncio.run(adapter.judge(judge_request(image=True)))
    assert result.readable is True
    content = observed["payload"]["messages"][1]["content"]
    assert content[0]["type"] == "text"
    assert content[1]["image_url"]["url"].startswith("data:image/png;base64,")
    assert "context_image" not in json.dumps(observed["payload"])
    assert "source_id" not in json.dumps(observed["payload"])


def test_the_route_rejects_an_image_when_image_mode_is_off() -> None:
    class StubJudge:
        async def judge(self, request: JudgeRequest) -> JudgeResponse:
            raise AssertionError("the route must not call the provider for a disabled image")

    app = FastAPI()
    app.include_router(create_name_judge_router(StubJudge(), images_enabled=False))
    client = TestClient(app)
    response = client.post(
        "/v1/names/judge",
        json=judge_request(image=True).model_dump(mode="json"),
    )
    assert response.status_code == 422
    assert response.json()["detail"] == "name_judge_image_mode_disabled"


def test_the_route_validates_a_provider_choice_independently() -> None:
    class StubJudge:
        async def judge(self, request: JudgeRequest) -> JudgeResponse:
            return JudgeResponse(
                choice="اسم خارج القائمة",
                abstained=False,
                probabilities={"محمد": 0.5, "أحمد": 0.4, "unclear": 0.1},
                confidence=0.5,
                model="jev-pinned",
                prompt_version="name-judge-v1",
            )

    app = FastAPI()
    app.include_router(create_name_judge_router(StubJudge()))
    client = TestClient(app)
    response = client.post(
        "/v1/names/judge",
        json=judge_request().model_dump(mode="json"),
    )
    assert response.status_code == 422


def test_readability_without_an_image_is_rejected_at_the_route() -> None:
    class StubJudge:
        async def judge(self, request: JudgeRequest) -> JudgeResponse:
            return JudgeResponse(
                choice="محمد",
                abstained=False,
                probabilities={"محمد": 0.7, "أحمد": 0.2, "unclear": 0.1},
                confidence=0.7,
                readable=False,
                model="jev-pinned",
                prompt_version="name-judge-v1",
            )

    app = FastAPI()
    app.include_router(create_name_judge_router(StubJudge()))
    client = TestClient(app)
    response = client.post(
        "/v1/names/judge",
        json=judge_request().model_dump(mode="json"),
    )
    assert response.status_code == 422


def test_the_route_returns_the_mapped_choice_without_identifiers() -> None:
    class StubJudge:
        async def judge(self, request: JudgeRequest) -> JudgeResponse:
            assert request.state.reading == "محمد"
            assert request.state.file_context == ["عبدالله"]
            return JudgeResponse(
                choice="أحمد",
                abstained=False,
                probabilities={"محمد": 0.2, "أحمد": 0.7, "unclear": 0.1},
                confidence=0.7,
                model="jev-pinned",
                prompt_version="name-judge-v1",
            )

    app = FastAPI()
    app.include_router(create_name_judge_router(StubJudge(), images_enabled=True))
    client = TestClient(app)
    response = client.post(
        "/v1/names/judge",
        json=judge_request(image=True).model_dump(mode="json"),
    )
    assert response.status_code == 200
    assert response.json()["choice"] == "أحمد"
    assert response.json()["promptVersion"] == "name-judge-v1"
    assert response.json()["cacheHit"] is False
    assert "source_id" not in response.text
    assert "family_id" not in response.text
    assert "reviewer" not in response.text


def test_extra_request_fields_are_forbidden() -> None:
    payload = judge_request().model_dump(mode="json")
    payload["source_id"] = "private-source"
    with pytest.raises(ValidationError):
        JudgeRequest.model_validate(payload)


def test_image_mode_defaults_false(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.delenv("NAME_JUDGE_IMAGES", raising=False)
    assert name_judge_images_enabled() is False
    monkeypatch.setenv("NAME_JUDGE_IMAGES", "true")
    assert name_judge_images_enabled() is True
