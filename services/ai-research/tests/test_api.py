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
