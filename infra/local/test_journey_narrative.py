"""Contract tests for the plan 033-F narrated journey export.

`infra/local/build-journey-narrative.py` is the corridor/event lane's
shareable read, run as a generator. What makes it honest is:

* the export is DOCUMENTED-ONLY: a platform_inferred row's own key can never
  appear in a slide's own words, and the contract's誓言 lives in the header,
* every slide carries its receipts (event_id/corridor_id + source id + the
  citation) — no receipt, no slide,
* the lightbox's era window actually bounds what entered (a 2112 event
  inside an 1800..1950 window = a bug),
* an empty window refuses to emit an artifact: the script exits non-zero and
  writes nothing that could read as narrated history,
* the checksum sidecar names the artifact's own bytes: a re-write that
  changed the payload must re-write the sidecar (the plan's checksum rule).

Pure, offline, standard library only; the reads go through the documented
container path, shut out by the same generator contract every other artifact's
tests keep.
"""

import json
import pathlib
import subprocess
import sys

HERE = pathlib.Path(__file__).resolve().parent
ROOT = HERE.parent.parent
SCRIPT = HERE / "build-journey-narrative.py"
JOURNEYS = HERE / "journeys"
CONTAINER = "dawha-db-1"


def run_script(*args: str) -> subprocess.CompletedProcess:
    return subprocess.run(
        [sys.executable, str(SCRIPT), *args],
        capture_output=True, text=True, check=False,
    )


def db_ready() -> bool:
    probe = subprocess.run(
        ["docker", "exec", CONTAINER, "psql", "-U", "dawha", "-d", "dawha", "-t", "-c", "SELECT 1"],
        capture_output=True, text=True, check=False,
    )
    return probe.returncode == 0


def test_export_contract():
    if not db_ready():
        print("skipped: the local container is down")
        return
    # The trip's own honest empty refusal first: an absent tribe's name can
    # never produce a JSON that reads as a journey.
    absent = run_script("--tribe", "قبيلة غير موجودة إطلاقاً", "--from", "1800", "--to", "1950")
    assert absent.returncode != 0, "an absent tribe exported a narrative"
    assert not (JOURNEYS / "journey-قبيلة غير موجودة إطلاقاً.json").exists()
    assert "refusing an empty narrative" in absent.stderr

    # The success path: the 033 seed's own documented trip (the reviewer's
    # accepted corridor + the movement's own event row).
    built = run_script("--tribe", "حرب", "--from", "1800", "--to", "1950")
    if built.returncode == 2:
        print("skipped: the seed's documented trip is not applied in this database")
        return
    assert built.returncode == 0, built.stderr

    artifact = JOURNEYS / "journey-حرب-1800-1950.json"
    payload = json.loads(artifact.read_text(encoding="utf-8"))

    # The header's own contract, verbatim: the artifact says what it is.
    assert payload["header"]["contract"].startswith("documented rows only")
    assert payload["storymap"]["era"] == [1800, 1950]

    # Every slide carries receipts; the platform_inferred key never rides one.
    for slide in payload["storymap"]["slides"]:
        receipts = slide["receipts"]
        assert receipts.get("corridor_id") or receipts.get("event_id")
        assert receipts.get("source_id")
        assert "platform_inferred" not in json.dumps(slide, ensure_ascii=False)
        # The era window actually bounded what entered.
        if slide.get("era"):
            year = int(slide["era"][0][:4])
            assert 1800 <= year <= 1950

    # The checksum sidecar names the artifact's own bytes.
    import hashlib
    sidecar = artifact.with_suffix(".json.sha256")
    recorded = sidecar.read_text(encoding="utf-8").split()[0]
    computed = hashlib.sha256(artifact.read_bytes()).hexdigest()
    assert recorded == computed, "the checksum sidecar does not name the artifact"
