#!/usr/bin/env python3
"""Plan 030-F: divide a plate's proposals into bounded tile tasks.

What this lane does, and what it refuses to do
----------------------------------------------
A plate's proposal queue is a flat list of crops; a reviewer cannot hold 74
of them in one sitting, and an unbounded queue hides the work's shape. This
builder overlays a square grid (1024 px) on each plate's neatline frame — the
frame plan 030-D already measured — and emits one tile task per square that
holds at least one proposal's centre. It is Tasking-Manager's task grid at
plate scale: claim a square, work what is inside it, and let a second
reviewer's independent pass retire it.

It deliberately does NOT store which proposals belong to which tile. The
membership is derived at read time from the crop boxes themselves (a
centre-in-square test against the tile's own box, the same arithmetic this
script uses), so a later proposals run that appends rows is picked up by the
existing tiles with no re-sync, and no member list can go stale. The artifact
records per-tile counts for the human reading it; the database stores only
squares.

The grid is deterministic: same frame, same tile size, same tiles — and the
same tile_id for the same square, so re-running never forks the work queue.
Plates with no proposals (the Dickson plate, whose route is reviewer-entered
anchor towns) contribute none.

Run:    python3 infra/local/build-plate-tile-tasks.py
Needs:  infra/local/plate-georefs.json (030-D frames) and
        infra/local/plate-region-candidates.json (030-E proposals).
"""

from __future__ import annotations

import json
import sys
from datetime import date
from pathlib import Path

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[1]
GEOREF = HERE / "plate-georefs.json"
CANDIDATES = HERE / "plate-region-candidates.json"
ARTIFACT = HERE / "plate-tile-tasks.json"
SEED = HERE / "seed-plate-tile-tasks.sql"

TILE_SIZE = 1024
# The artifact's own bound: a plate whose proposals sprawl past this many
# tiles is reported truncated rather than silently complete.
MAX_TILES_PER_PLATE = 400


def main() -> int:
    georef = load(GEOREF)
    candidates = load(CANDIDATES)
    georef_plates = {plate["key"]: plate for plate in georef["plates"]}
    candidate_plates = {plate["key"]: plate for plate in candidates["plates"]}

    out_plates = []
    for key in sorted(candidate_plates):
        plate = candidate_plates[key]
        source_id = plate["sourceId"]
        frame = frame_of(georef_plates.get(key))
        if frame is None:
            out_plates.append({
                "key": key,
                "sourceId": source_id,
                "tiles": [],
                "tileCount": 0,
                "truncated": False,
                "note": "no 030-D frame; no grid",
            })
            continue

        assignments: dict[tuple[int, int], list[str]] = {}
        outside = 0
        for candidate in plate["candidates"]:
            box = candidate["cropPx"]
            centre_x = box["x"] + box["w"] / 2
            centre_y = box["y"] + box["h"] / 2
            if not (frame["x0"] <= centre_x < frame["x1"] and frame["y0"] <= centre_y < frame["y1"]):
                outside += 1
                continue
            col = int((centre_x - frame["x0"]) // TILE_SIZE)
            row = int((centre_y - frame["y0"]) // TILE_SIZE)
            assignments.setdefault((row, col), []).append(candidate["occurrenceId"])
        if outside:
            # The proposal lane promises inside-the-neatline boxes; a proposal
            # outside the frame is a fact to record, not to paper over.
            print(f"{key}: {outside} proposal(s) outside the frame", file=sys.stderr)

        tiles = []
        truncated = False
        for row, col in sorted(assignments):
            if len(tiles) >= MAX_TILES_PER_PLATE:
                truncated = True
                break
            x = frame["x0"] + col * TILE_SIZE
            y = frame["y0"] + row * TILE_SIZE
            tiles.append({
                "tileId": f"tile:{key}:r{row}c{col}",
                "row": row,
                "col": col,
                "boxPx": {
                    "x": x,
                    "y": y,
                    "w": min(TILE_SIZE, frame["x1"] - x),
                    "h": min(TILE_SIZE, frame["y1"] - y),
                },
                # For the human reading the artifact only; the database does
                # not store membership (derived at read time).
                "proposalCount": len(assignments[(row, col)]),
                "proposals": sorted(assignments[(row, col)]),
            })

        out_plates.append({
            "key": key,
            "sourceId": source_id,
            "tileSize": TILE_SIZE,
            "frame": {
                "x0": frame["x0"], "y0": frame["y0"],
                "x1": frame["x1"], "y1": frame["y1"],
            },
            "tileCount": len(tiles),
            "proposalCount": sum(tile["proposalCount"] for tile in tiles),
            "truncated": truncated,
            "tiles": tiles,
        })

    ARTIFACT.write_text(json.dumps({
        "meta": {
            "generator": "infra/local/build-plate-tile-tasks.py",
            "generatedAt": date.today().isoformat(),
            "tileSize": TILE_SIZE,
            "method": (
                "a square grid over each plate's 030-D neatline frame; one tile "
                "task per square holding at least one proposal's crop-box centre"
            ),
            "membershipRule": (
                "NOT stored: membership is derived at read time from the crop "
                "boxes against the tile's own box (centre-in-square, the same "
                "arithmetic as this script), so a later proposals run that "
                "appends rows is picked up by the existing tiles with no re-sync"
            ),
            "certaintyRule": (
                "a tile task is work allocation, not a verdict; retirement "
                "needs two passes by different reviewers that agree, per "
                "stipulation 2 (plan 030), and nothing here writes a reading"
            ),
            "maxTilesPerPlate": MAX_TILES_PER_PLATE,
        },
        "plates": out_plates,
    }, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(f"wrote {ARTIFACT.relative_to(ROOT)}")

    write_seed(out_plates)
    return 0


def frame_of(plate: dict | None) -> dict | None:
    """The plate's inner frame as a half-open pixel rectangle, floored to
    whole pixels so the grid arithmetic is integer-exact everywhere."""
    if plate is None:
        return None
    georef = plate.get("georef")
    if not isinstance(georef, dict) or not isinstance(georef.get("frame"), dict):
        return None
    frame = georef["frame"]
    try:
        x0 = int(frame["inner_left"])
        y0 = int(frame["inner_top"])
        x1 = int(frame["inner_right"])
        y1 = int(frame["inner_bottom"])
    except (KeyError, TypeError, ValueError):
        return None
    if x1 <= x0 or y1 <= y0:
        return None
    return {"x0": x0, "y0": y0, "x1": x1, "y1": y1}


def write_seed(plates: list[dict]) -> None:
    lines = [
        "-- Plan 030-F: tile tasks over a plate's proposals.",
        "-- Generated by infra/local/build-plate-tile-tasks.py; do not edit.",
        f"-- Generated: {date.today().isoformat()}",
        "--",
        "-- One row per square of the plate's neatline grid that holds at least",
        "-- one proposal. Membership is NOT stored: it is derived at read time",
        "-- from the crop boxes. ON CONFLICT DO NOTHING, so re-applying the",
        "-- seed can never disturb a claim, a pass or a retirement.",
        "",
    ]
    total = 0
    for plate in plates:
        if not plate["tiles"]:
            lines.append(f"-- {plate['key']}: no tiles ({plate.get('note', 'no proposals')})")
            lines.append("")
            continue
        lines.append(f"-- {plate['key']}: {plate['tileCount']} tiles, "
                     f"{plate['proposalCount']} proposals covered")
        for tile in plate["tiles"]:
            total += 1
            box = json.dumps(tile["boxPx"], separators=(",", ":"))
            assert "$json$" not in box
            lines.append(
                "INSERT INTO region_tile_tasks (source_id, plate_key, tile_id, box_px, status)")
            lines.append("VALUES (")
            lines.append(f"  '{plate['sourceId']}', {sql_str(plate['key'])}, "
                         f"{sql_str(tile['tileId'])},")
            lines.append(f"  $json${box}$json$::jsonb, 'open')")
            lines.append("ON CONFLICT (source_id, tile_id) DO NOTHING;")
        lines.append("")
    SEED.write_text("\n".join(lines) + "\n", encoding="utf-8")
    print(f"wrote {SEED.relative_to(ROOT)} ({total} tile rows)")


def load(path: Path) -> dict:
    with path.open(encoding="utf-8") as handle:
        return json.load(handle)


def sql_str(value: str) -> str:
    return "'" + value.replace("'", "''") + "'"


if __name__ == "__main__":
    sys.exit(main())
