#!/usr/bin/env python3
"""Plan 034-A (the DATA JOB): read the live database through the documented
container path and write the committed passage artifacts.

This is the GENERATOR side of the lane: it is the ONLY script in this package
that touches the database (and only through the documented docker path). It
writes, byte-for-byte:

  infra/local/reuse/passages.json        — the passage corpus
  infra/local/reuse/passages.json.sha256 -- the corpus's own checksum

Everything downstream (the clustering engine, its tests) reads the COMMITTED
artifacts — pure, offline, standard library only, never the database. This is
the discipline the whole infra tree keeps: scripts, not screenshots; jobs
touch data, gates touch checks.

Idempotent: re-running re-reads the same committed rows and rewrites the same
files byte for byte.
"""

import argparse
import datetime
import hashlib
import json
import pathlib
import subprocess
import sys

ROOT = pathlib.Path(__file__).resolve().parents[2]
LANE = ROOT / "infra" / "local" / "reuse"
CONTAINER = "dawha-db-1"
FIELD_SEP = "\x1f"


def sha256_file(path: pathlib.Path) -> str:
    digest = hashlib.sha256()
    with open(path, "rb") as handle:
        for chunk in iter(lambda: handle.read(65536), b""):
            digest.update(chunk)
    return digest.hexdigest()


def read_passages() -> list[dict]:
    sql = """
        SELECT sp.id::text, sp.source_id::text, sf.checksum_sha256,
               sp.normalized_text_ar, COALESCE(sp.locator_ar, ''), COALESCE(s.title_ar, '')
        FROM source_passages sp
        JOIN sources s ON s.id = sp.source_id
        LEFT JOIN source_files sf ON sf.id = sp.source_file_id
        WHERE sp.normalized_text_ar <> '' AND s.visibility = 'public'
        ORDER BY sp.source_id, sp.sequence_number, sp.id
    """
    result = subprocess.run(
        ["docker", "exec", CONTAINER, "psql", "-U", "dawha", "-d", "dawha", "-t", "-A", "-F", FIELD_SEP, "-c", sql],
        capture_output=True, text=True, check=True,
    )
    passages = []
    for line in result.stdout.splitlines():
        if not line:
            continue
        parts = line.split(FIELD_SEP)
        if len(parts) != 6:
            continue
        passages.append({
            "id": parts[0],
            "source_id": parts[1],
            "checksum": parts[2],
            "normalized_text_ar": parts[3],
            "locator_ar": parts[4],
            "source_title_ar": parts[5],
        })
    return passages


def main() -> int:
    parser = argparse.ArgumentParser(description="refresh the committed reuse corpus from the live database")
    parser.add_argument("--skip-db", action="store_true", help="keep the committed passages.json as-is (re-use it)")
    args = parser.parse_args()

    LANE.mkdir(parents=True, exist_ok=True)
    corpus_path = LANE / "passages.json"
    if not args.skip_db or not corpus_path.exists():
        passages = read_passages()
        corpus = {"generated_at": datetime.date.today().isoformat(), "count": len(passages), "passages": passages}
        corpus_path.write_text(json.dumps(corpus, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
        print(f"wrote {corpus_path.relative_to(ROOT)} ({len(passages)} passages)")
    else:
        print(f"kept {corpus_path.relative_to(ROOT)}")
        corpus = json.loads(corpus_path.read_text(encoding="utf-8"))

    sidecar = corpus_path.with_suffix(".json.sha256")
    sidecar.write_text(sha256_file(corpus_path) + "  " + corpus_path.name + "\n", encoding="utf-8")
    print(f"wrote {sidecar.relative_to(ROOT)} ({sha256_file(corpus_path)[:16]}…)")
    print(f"corpus: {corpus['count']} passages")
    return 0


if __name__ == "__main__":
    sys.exit(main())
