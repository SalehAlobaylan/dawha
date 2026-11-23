"""Contract tests for the plan 030-F tile-task grid.

`infra/local/plate-tile-tasks.json` is the workbench's work allocation. What
makes it honest is not that the squares look tidy — it is that the grid is
deterministic, covers every proposal exactly once, stores no membership that
could go stale, and stays visibly work allocation rather than verdicts:

* the meta states the derivation rule (centre-in-square against the tile's
  own box) and why membership is NOT stored,
* the grid is reproducible: recomputing it from the 030-D frame and the
  030-E proposals yields the same tiles with the same tile ids,
* every tile box lies inside its plate's frame, is aligned to the grid
  origin, and is at most one tile square in each dimension,
* every proposal's centre falls in EXACTLY one tile (a partition, no gaps,
  no overlaps),
* the seed stores squares only — no membership column, ON CONFLICT DO
  NOTHING so a re-apply can never disturb a claim, pass or retirement,
* a plate with no proposals contributes no tiles, and records why.

Pure, offline, standard library only; it reads three committed artifacts and
never touches the database or the plates.

Run:  python3 infra/local/test_plate_tile_tasks.py
"""

import json
import re
import unittest
from pathlib import Path

HERE = Path(__file__).resolve().parent
TILE_ARTIFACT = HERE / "plate-tile-tasks.json"
GEOREF = HERE / "plate-georefs.json"
CANDIDATES = HERE / "plate-region-candidates.json"
SEED = HERE / "seed-plate-tile-tasks.sql"


def load(path: Path) -> dict:
    with path.open(encoding="utf-8") as handle:
        return json.load(handle)


def frame_of(plate: dict) -> dict | None:
    georef = plate.get("georef")
    if not isinstance(georef, dict) or not isinstance(georef.get("frame"), dict):
        return None
    frame = georef["frame"]
    try:
        return {
            "x0": int(frame["inner_left"]),
            "y0": int(frame["inner_top"]),
            "x1": int(frame["inner_right"]),
            "y1": int(frame["inner_bottom"]),
        }
    except (KeyError, TypeError, ValueError):
        return None


def recompute_tiles(frame: dict, proposals: list[dict], tile_size: int) -> dict[tuple[int, int], list[str]]:
    """An independent reimplementation of the grid, to pin determinism."""
    assignments: dict[tuple[int, int], list[str]] = {}
    for candidate in proposals:
        box = candidate["cropPx"]
        centre_x = box["x"] + box["w"] / 2
        centre_y = box["y"] + box["h"] / 2
        if not (frame["x0"] <= centre_x < frame["x1"] and frame["y0"] <= centre_y < frame["y1"]):
            continue
        col = int((centre_x - frame["x0"]) // tile_size)
        row = int((centre_y - frame["y0"]) // tile_size)
        assignments.setdefault((row, col), []).append(candidate["occurrenceId"])
    return assignments


class TileTaskArtifactTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.tiles = load(TILE_ARTIFACT)
        cls.meta = cls.tiles["meta"]
        cls.georef = {p["key"]: p for p in load(GEOREF)["plates"]}
        cls.candidate_plates = {p["key"]: p for p in load(CANDIDATES)["plates"]}
        cls.plates = {p["key"]: p for p in cls.tiles["plates"]}
        cls.seed = SEED.read_text(encoding="utf-8")

    def test_meta_states_the_rule_and_why_membership_is_not_stored(self):
        meta = self.meta
        self.assertIn("centre", meta["method"])
        self.assertIn("NOT stored", meta["membershipRule"])
        # The reason matters as much as the rule: a stale member list is the
        # failure mode this design exists to make impossible.
        self.assertIn("re-sync", meta["membershipRule"])
        self.assertIn("stipulation 2", meta["certaintyRule"])
        self.assertEqual(meta["tileSize"], 1024)

    def test_grid_is_reproducible_from_the_frame_and_the_proposals(self):
        for key, plate in self.plates.items():
            if not plate["tiles"]:
                continue
            frame = frame_of(self.georef[key])
            self.assertIsNotNone(frame, f"{key}: tiles without a 030-D frame")
            assignments = recompute_tiles(
                frame, self.candidate_plates[key]["candidates"], plate["tileSize"])
            expected_ids = {
                f"tile:{key}:r{row}c{col}" for row, col in assignments
            }
            actual_ids = {tile["tileId"] for tile in plate["tiles"]}
            self.assertEqual(actual_ids, expected_ids,
                             f"{key}: the grid is not the deterministic grid")
            for tile in plate["tiles"]:
                match = re.fullmatch(r"tile:%s:r(\d+)c(\d+)" % re.escape(key), tile["tileId"])
                self.assertIsNotNone(match, tile["tileId"])
                row, col = int(match.group(1)), int(match.group(2))
                self.assertIn((row, col), assignments)
                self.assertEqual(tile["proposalCount"], len(assignments[(row, col)]))

    def test_every_tile_box_is_inside_the_frame_and_grid_aligned(self):
        for key, plate in self.plates.items():
            if not plate["tiles"]:
                continue
            frame = frame_of(self.georef[key])
            size = plate["tileSize"]
            for tile in plate["tiles"]:
                box = tile["boxPx"]
                self.assertEqual((box["x"] - frame["x0"]) % size, 0, f"{tile['tileId']}: x alignment")
                self.assertEqual((box["y"] - frame["y0"]) % size, 0, f"{tile['tileId']}: y alignment")
                for edge, inner in (("x", "x0"), ("y", "y0")):
                    self.assertGreaterEqual(box[edge], frame[inner], tile["tileId"])
                self.assertLessEqual(box["x"] + box["w"], frame["x1"], tile["tileId"])
                self.assertLessEqual(box["y"] + box["h"], frame["y1"], tile["tileId"])
                self.assertLessEqual(box["w"], size, tile["tileId"])
                self.assertLessEqual(box["h"], size, tile["tileId"])

    def test_every_proposal_lands_in_exactly_one_tile(self):
        for key, plate in self.plates.items():
            if not plate["tiles"]:
                continue
            tile_boxes = [(tile["tileId"], tile["boxPx"]) for tile in plate["tiles"]]
            for candidate in self.candidate_plates[key]["candidates"]:
                box = candidate["cropPx"]
                centre = (box["x"] + box["w"] / 2, box["y"] + box["h"] / 2)
                inside = [
                    tile_id for tile_id, tile_box in tile_boxes
                    if tile_box["x"] <= centre[0] < tile_box["x"] + tile_box["w"]
                    and tile_box["y"] <= centre[1] < tile_box["y"] + tile_box["h"]
                ]
                self.assertEqual(len(inside), 1,
                                 f"{candidate['occurrenceId']}: lands in {inside}")

    def test_a_plate_without_proposals_contributes_none_and_says_why(self):
        for key, plate in self.plates.items():
            if plate["tiles"]:
                continue
            candidate_plate = self.candidate_plates.get(key, {"candidates": []})
            self.assertFalse(candidate_plate["candidates"],
                             f"{key}: no tiles but proposals exist")
            self.assertTrue(plate.get("note"), f"{key}: absence unrecorded")

    def test_seed_stores_squares_only_and_is_idempotent(self):
        inserts = re.findall(
            r"INSERT INTO region_tile_tasks \(([^)]*)\)", self.seed)
        self.assertTrue(inserts)
        for columns in inserts:
            names = {column.strip() for column in columns.split(",")}
            self.assertNotIn("candidates", names)
            self.assertNotIn("membership", names)
            self.assertNotIn("candidate_ids", names)
        # A re-apply can never overwrite a claim, a pass or a retirement.
        self.assertEqual(self.seed.count("ON CONFLICT (source_id, tile_id) DO NOTHING"),
                         len(inserts))
        # Every tile row is planted open; a seed must not forge progress.
        self.assertEqual(self.seed.count(", 'open')"), len(inserts))

    def test_seed_rows_reference_the_registered_source_ids(self):
        registered = {plate["sourceId"] for plate in self.candidate_plates.values()}
        used = set(re.findall(r"INSERT INTO region_tile_tasks[^(]*\([^)]*\)\s*VALUES\s*\(\s*'([0-9a-f-]+)'", self.seed))
        self.assertTrue(used)
        for source_id in used:
            self.assertIn(source_id, registered,
                          f"seed references unregistered source {source_id}")

    def test_proposal_counts_reconcile_with_the_candidate_artifact(self):
        for key, plate in self.plates.items():
            if not plate["tiles"]:
                continue
            total = plate["proposalCount"]
            self.assertEqual(total, len(self.candidate_plates[key]["candidates"]),
                             f"{key}: covered {total} of "
                             f"{len(self.candidate_plates[key]['candidates'])} proposals")


if __name__ == "__main__":
    unittest.main()
