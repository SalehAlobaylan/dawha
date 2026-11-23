#!/usr/bin/env python3
"""The 1916 Admiralty plate's ARABIC COPY renderer (plan 034-A2, applied):
a generated copy of the historical scan that carries the READER lane's own
labels in Arabic, register-separated from the historical ink.

Contract (unstated parts carry plan 030-E's opener, label_text_ar until read):
- the historical INK is embedded byte for byte; the plate never shifts;
- every candidate box carries a ◆ marker in the reviewer's own register:
  a FILLED box (label_read_by set) renders its Arabic reading plus the
  reviewer's attribution; an unfilled box stays a marker alone — a proposal
  register, never presented as a reading;
- idempotent: same inputs — same bytes, same sidecar; --as-of pins the
  rendered date for reproducible audits.

Usage:
  python3 infra/local/build-plate-arabic.py \
      --plate admiralty-1916-tribal \
      --src infra/local/plate-region-candidates.json \
      --as-of 2026-11-19
"""

import re
import base64
import datetime
import hashlib
import json
import pathlib
import subprocess
import sys

ROOT = pathlib.Path(__file__).resolve().parents[2]
APP = ROOT / "infra" / "local"
OUT_DIR = APP / "plate-arabic"


def sha256_bytes(raw: bytes) -> str:
    return hashlib.sha256(raw).hexdigest()


LABEL_SQL = (
    "SELECT c.occurrence_id, c.label_text_ar, "
    "COALESCE(u.display_name_ar, '') "
    "FROM external_region_candidates c "
    "LEFT JOIN users u ON u.id = c.label_read_by "
    "WHERE c.plate_key = %s AND c.label_text_ar <> ''"
)


def read_labels(plate_key: str) -> dict:
    """One documented read: the reviewer lane's own labels. The container
    path is the repo's own; the machine proposes, the reviewer writes."""
    sql = LABEL_SQL % ("'" + plate_key + "'")
    result = subprocess.run(
        ["docker", "exec", "dawha-db-1", "psql", "-U", "dawha", "-d", "dawha", "-t", "-A", "-F", "\x1f", "-c", sql],
        capture_output=True, text=True, check=False,
    )
    if result.returncode != 0:
        raise SystemExit(f"psql failed: {result.stderr.strip()}")
    labels = {}
    for line in result.stdout.splitlines():
        parts = line.split("\x1f")
        if len(parts) == 3:
            labels[parts[0]] = {"text": parts[1], "reader": parts[2]}
    return labels


def build(plate_key: str, src: pathlib.Path, as_of: str) -> int:
    if not re.fullmatch(r"[a-z0-9-]+", plate_key):
        raise SystemExit(f"plate key must be a slug; got {plate_key!r}")
    doc = json.load(open(src, encoding="utf-8"))
    plate = next(p for p in doc["plates"] if p["key"] == plate_key)
    width, height = plate["plateWidthPx"], plate["plateHeightPx"]
    candidates = plate.get("candidates", [])
    labels = read_labels(plate_key)
    raster_digest = sha256_bytes((ROOT / ".data" / "source-storage" / "sources" / "plates" / "admiralty-1916-tribal-original.jpg").read_bytes())
    raster_b64 = base64.b64encode((ROOT / ".data" / "source-storage" / "sources" / "plates" / "admiralty-1916-tribal-original.jpg").read_bytes()).decode("ascii")

    markers = []
    readings = []
    for candidate in candidates:
        box = candidate["boxPx"]
        cx, cy = box["x"] + box["w"] / 2, box["y"] + box["h"] / 2
        row = labels.get(candidate["occurrenceId"], {})
        text = row.get("text")
        markers.append(f'<circle cx="{cx:.0f}" cy="{cy:.0f}" r="9" fill="#c9452b" fill-opacity="0.85" stroke="#fffdf5" stroke-width="2"/>')
        if text:
            reader = row.get("reader") or "مراجع"
            readings.append(
                f'<text x="{cx:.0f}" y="{cy - 20:.0f}" text-anchor="middle" font-size="30" font-family="Amiri, serif" fill="#2d2520">{text}</text>'
                f'<text x="{cx:.0f}" y="{cy + 18:.0f}" text-anchor="middle" font-size="16" fill="#8a5a2b">بقراءة {reader}</text>'
            )

    svg = (
        f'<svg xmlns="http://www.w3.org/2000/svg" width="{width}" height="{height}" viewBox="0 0 {width} {height}" dir="rtl">\n'
        f'<title>{plate_key} — النسخة العربية المولّدة ({as_of}); المُولّد يقترح ولا يقرأ</title>\n'
        f'<image x="0" y="0" width="{width}" height="{height}" xlink:href="data:image/jpeg;base64,{raster_b64}"/>\n'
        f'<g id="markers">{"".join(markers)}</g>\n'
        f'<g id="readings">{"".join(readings)}</g>\n'
        f'</svg>\n'
    )
    OUT_DIR.mkdir(parents=True, exist_ok=True)
    out = OUT_DIR / f"{plate_key}-arabic.svg"
    out.write_text(svg, encoding="utf-8")
    sidecar = out.with_suffix(".svg.sha256")
    sidecar.write_text(sha256_bytes(out.read_bytes()) + "  " + out.name + "\n", encoding="utf-8")
    print(f"wrote {out.relative_to(ROOT)} ({out.stat().st_size} bytes)")
    print(f"wrote {sidecar.relative_to(ROOT)}")
    return 0


if __name__ == "__main__":
    import argparse

    parser = argparse.ArgumentParser(description="the Arabic copy renderer (plan 030-E / 034-A2)")
    parser.add_argument("--plate", default="admiralty-1916-tribal")
    parser.add_argument("--src", default=None, help="path to the candidates artifact (defaults to the committed one)")
    parser.add_argument("--as-of", default=None, help="pin the rendered date for reproducible audits")
    args = parser.parse_args()
    src = pathlib.Path(args.src) if args.src else (APP / "plate-region-candidates.json")
    as_of = args.as_of or datetime.date.today().isoformat()
    sys.exit(build(args.plate, src, as_of))
