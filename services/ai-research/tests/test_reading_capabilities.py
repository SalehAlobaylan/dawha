import asyncio

import httpx
import pytest
from fastapi import FastAPI, HTTPException
from fastapi.testclient import TestClient

from app import chart_vision


@pytest.mark.parametrize("mode", ["local", "off", "typo", ""])
def test_non_hosted_mode_never_discovers_a_credential(monkeypatch, mode):
    monkeypatch.setenv("NAME_READER_MODE", mode)
    monkeypatch.setenv("CHART_VISION_ENABLED", "true")
    monkeypatch.setenv("DEEPSEEK_API_KEY", "test-only")
    monkeypatch.setattr(
        chart_vision, "_load_chart_vision_key", lambda: pytest.fail("credential lookup")
    )
    model = chart_vision.configured_chart_vision()
    assert not model.enabled
    assert not model.api_key


def test_hosted_mode_requires_a_separate_explicit_enable_flag(monkeypatch):
    monkeypatch.setenv("NAME_READER_MODE", "hosted")
    monkeypatch.setenv("CHART_VISION_ENABLED", "invalid")
    monkeypatch.setattr(
        chart_vision, "_load_chart_vision_key", lambda: pytest.fail("credential lookup")
    )
    assert not chart_vision.configured_chart_vision().enabled


def test_capabilities_are_configuration_only_and_never_expose_a_key():
    model = chart_vision.DeepSeekChartVision("test-only", enabled=False)
    app = FastAPI()
    app.include_router(chart_vision.create_chart_vision_router(model))
    response = TestClient(app).get("/v1/vision/chart/capabilities")
    assert response.json() == {
        "enabled": False, "configured": False,
        "reason": "disabled_by_policy", "version": chart_vision.MODEL,
    }
    assert "test-only" not in response.text


def test_policy_denial_happens_before_any_hosted_transport():
    def forbidden(request):
        pytest.fail("hosted request was sent")

    model = chart_vision.DeepSeekChartVision(
        "test-only", enabled=False, transport=httpx.MockTransport(forbidden),
    )
    with pytest.raises(HTTPException) as error:
        asyncio.run(model._interact("fixture", [], {}, 1, "fixture"))
    assert error.value.status_code == 403


def test_capability_metadata_passes_the_real_app_body_boundary():
    from app.main import app

    client = TestClient(app)
    response = client.get("/v1/vision/chart/capabilities")
    assert response.status_code == 200
    assert "configured" in response.json()
    assert client.get(
        "/v1/vision/chart/capabilities", headers={"content-length": "1"}
    ).status_code == 400
    assert client.get(
        "/v1/vision/chart/capabilities", headers={"transfer-encoding": "chunked"}
    ).status_code == 400
