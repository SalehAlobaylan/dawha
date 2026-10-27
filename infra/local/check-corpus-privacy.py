#!/usr/bin/env python3
"""Fail when a private family document, or anything derived from one, is tracked by git.

These originals are irreplaceable family records. The corpus lives outside the
working tree precisely so that a path mistake cannot put one in a commit, but
"outside the tree" is a habit, not a guarantee. This is the guarantee.

Three checks, cheapest first:

  1. the corpus directory is outside the repository;
  2. no tracked file lives under a corpus-shaped path;
  3. no tracked file is a corpus artefact - by content shape, or by having the
     sha256 of a recorded original.

The third is the one that matters and it has no false positives: a file whose
bytes hash to a recorded original *is* that original.

Usage:  python3 infra/local/check-corpus-privacy.py [--corpus DIR]
Exit:   0 clean, 2 a leak was found.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import subprocess
import sys
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent.parent

# A tracked path containing one of these is treated as a corpus artefact unless
# it is reviewed below.
CORPUS_PATH_PARTS = {"corpus", "ocr-corpus", "dawha-corpus", "benchmark-data"}

# Extensions that could hold an original family document.
DOCUMENT_SUFFIXES = {".pdf", ".png", ".jpg", ".jpeg", ".tif", ".tiff", ".webp", ".gif"}

# Corpus artefacts are data, not code or prose: the manifest, the provenance
# record, the adjudicated gold, and a prediction file. They are recognised by
# their PARSED structure, so a document that merely describes the format - the
# harness itself, the format guide, this file - is not a false positive.
DATA_SUFFIXES = {".json", ".jsonl"}


def json_records(path: Path) -> list[object]:
    """Parse a .json array/object or a .jsonl file into records, or [] if it is not data."""
    try:
        if path.stat().st_size > 8 << 20:
            return []
        text = path.read_text(encoding="utf-8")
    except (OSError, UnicodeDecodeError):
        return []
    records: list[object] = []
    try:
        payload = json.loads(text)
    except json.JSONDecodeError:
        for line in text.splitlines():
            line = line.strip()
            if not line:
                continue
            try:
                records.append(json.loads(line))
            except json.JSONDecodeError:
                return []
        return records
    if isinstance(payload, list):
        return payload
    return [payload]


def artefact_shape(record: object) -> str | None:
    """The shape that makes a parsed record a corpus artefact, if it is one."""
    if not isinstance(record, dict):
        return None
    if "document_id" in record and "source_class" in record:
        return "manifest/gold record"
    if "adjudication" in record and isinstance(record.get("adjudication"), dict):
        adjudication = record["adjudication"]
        if "adjudicator_id" in adjudication:
            return "adjudicated gold"
    if "document_id" in record and "reference_id" in record:
        return "prediction record"
    if "schema_version" in record and "annotation_mode" in record:
        return "annotation record"
    return None


def looks_like_data_artefact(path: Path) -> str | None:
    for record in json_records(path):
        shape = artefact_shape(record)
        if shape is not None:
            return shape
    return None


def run_git(*args: str) -> str:
    result = subprocess.run(
        ("git", *args),
        cwd=REPO_ROOT,
        check=True,
        capture_output=True,
        text=True,
    )
    return result.stdout


def sha256_of(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1 << 20), b""):
            digest.update(chunk)
    return digest.hexdigest()


def looks_like_text_artefact(path: Path) -> str | None:
    """The shape that makes a data file a corpus artefact, if it is one."""
    for record in json_records(path):
        shape = artefact_shape(record)
        if shape is not None:
            return shape
    return None


def recorded_originals(corpus: Path) -> dict[str, str]:
    """sha256 -> document id, from the corpus provenance record."""
    source = corpus / "sources.json"
    if not source.exists():
        return {}
    try:
        payload = json.loads(source.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as error:
        print(f"corpus-privacy: cannot read {source}: {error}", file=sys.stderr)
        return {}
    recorded: dict[str, str] = {}
    for entry in payload.get("sources", []):
        checksum = str(entry.get("sha256", "")).lower()
        document = str(entry.get("document_id", "unknown"))
        if len(checksum) == 64:
            recorded[checksum] = document
    return recorded


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--corpus",
        default=os.environ.get("DAWHA_CORPUS_DIR", str(Path.home() / ".dawha" / "corpus")),
        help="the private corpus directory (default: $DAWHA_CORPUS_DIR or ~/.dawha/corpus)",
    )
    arguments = parser.parse_args()
    corpus = Path(arguments.corpus).expanduser().resolve()

    tracked = [line for line in run_git("ls-files", "-z").split("\0") if line]
    originals = recorded_originals(corpus)
    findings: list[str] = []

    # 1. the corpus must live outside the working tree
    try:
        corpus.relative_to(REPO_ROOT)
    except ValueError:
        pass
    else:
        findings.append(f"the corpus directory is inside the repository: {corpus}")

    # 2. no tracked file under a corpus-shaped path
    for relative in tracked:
        parts = set(Path(relative).parts)
        if parts & CORPUS_PATH_PARTS:
            findings.append(f"tracked file under a corpus-shaped path: {relative}")

    # 3. no tracked file is a corpus artefact
    for relative in tracked:
        path = REPO_ROOT / relative
        if not path.is_file():
            continue
        suffix = path.suffix.lower()
        if suffix in DOCUMENT_SUFFIXES:
            try:
                checksum = sha256_of(path)
            except OSError:
                continue
            if checksum in originals:
                findings.append(
                    f"tracked file is a recorded corpus original "
                    f"({originals[checksum]}): {relative}"
                )
            continue
        if suffix in DATA_SUFFIXES:
            marker = looks_like_data_artefact(path)
            if marker is not None:
                findings.append(f"tracked file looks like a corpus artefact [{marker}]: {relative}")
    if findings:
        print("corpus-privacy: private family documents must not be tracked by git", file=sys.stderr)
        for finding in findings:
            print(f"  - {finding}", file=sys.stderr)
        print(
            "corpus-privacy: keep the corpus outside the working tree, at "
            f"{corpus}, and untrack anything listed above with `git rm --cached`",
            file=sys.stderr,
        )
        return 2

    print(
        f"corpus-privacy: clean ({len(tracked)} tracked files checked, "
        f"{len(originals)} recorded original(s) checked by checksum)"
    )
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except subprocess.CalledProcessError as error:
        print(f"corpus-privacy: git failed: {error}", file=sys.stderr)
        sys.exit(2)
