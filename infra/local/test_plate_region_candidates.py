"""Contract tests for the plan 030-E region-label proposals.

`infra/local/plate-region-candidates.json` is what the review lane shows beside
a plate crop. The properties that matter are not "the boxes look nice" — they
are that every proposal is bounded, inside the plate, projected through the
*published* transform, and still visibly a proposal rather than a reading:

* the artifact states the detector's limits (town name can share a box, curved
  label can split, cartouche is text too) so no consumer can present a box as a
  parsed tribal name,
* every crop lies inside the neatline of the plate it names, and its box lies
  inside its crop,
* every recorded coordinate is reproducible from the plan 030-D model in
  `plate-georefs.json` — the proposal must not carry a second, drifted transform,
* the recorded projection floor equals that artifact's held-out RMS, so the
  uncertainty cannot be quietly improved on,
* no candidate carries a label reading, an era, or a decided status: a machine
  writes proposals, a reviewer writes answers,
* a plate with no transform contributes nothing, because there is nothing to
  project through.

Pure, offline, standard library only; it reads two committed artifacts and
never touches the database or the plates.

Run:  python3 infra/local/test_plate_region_candidates.py
"""

import json
import math
import unittest
from pathlib import Path

HERE = Path(__file__).resolve().parent
ARTIFACT = HERE / "plate-region-candidates.json"
GEOREF = HERE / "plate-georefs.json"


def load(path: Path) -> dict:
    with path.open(encoding="utf-8") as handle:
        return json.load(handle)


class RegionCandidateArtifactTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.artifact = load(ARTIFACT)
        cls.georef = {p["key"]: p for p in load(GEOREF)["plates"]}
        cls.plates = {p["key"]: p for p in cls.artifact["plates"]}

    def test_meta_states_the_limits_and_the_rule(self):
        meta = self.artifact["meta"]
        self.assertEqual(meta["generator"],
                         "infra/local/build-plate-region-candidates.py")
        self.assertIn("notOCR", meta)
        self.assertTrue(meta["notOCR"].strip())
        self.assertGreaterEqual(len(meta["knownLimits"]), 3)
        self.assertIn("confidenceDefinition", meta)
        # A label's box centre is not the feature it names; the artifact must say
        # so, because the next surface will draw that centre on a map.
        self.assertIn("half the label's width", meta["labelCentreNote"])
        self.assertEqual(meta["vertexCap"], 160)
        self.assertGreater(meta["candidateLimit"], 0)
        self.assertIn("never be rendered as a fact", meta["certaintyRule"])

    def test_only_a_georeferenced_plate_contributes(self):
        self.assertIn("admiralty-1916-tribal", self.plates)
        self.assertNotIn("dickson-1914-tribal", self.plates,
                         "a plate without a transform cannot project a proposal")
        for key, plate in self.plates.items():
            source = self.georef[key]["georef"]
            self.assertEqual(source["method"], "graticule_labels")
            self.assertTrue(plate["candidates"])

    def test_projection_floor_matches_the_published_georeference(self):
        for key, plate in self.plates.items():
            with self.subTest(plate=key):
                published = self.georef[key]["georef"]
                self.assertEqual(plate["transform"]["heldOutRmsKm"],
                                 published["heldOut"]["combinedKm"])
                self.assertEqual(plate["transform"]["townRmsKm"],
                                 published["townCheck"]["rmsKm"])
                self.assertEqual(plate["transform"]["needsReview"],
                                 published["needsReview"])
                self.assertEqual(plate["transform"]["reviewReasons"],
                                 published["reviewReasons"])

    def test_limits_are_honest(self):
        meta = self.artifact["meta"]
        for key, plate in self.plates.items():
            with self.subTest(plate=key):
                self.assertEqual(plate["candidateLimit"], meta["candidateLimit"])
                self.assertLessEqual(plate["selected"], plate["candidateLimit"])
                self.assertEqual(plate["truncated"],
                                 plate["detectedTextLines"] > plate["selected"])
                self.assertGreaterEqual(plate["detectedTextLines"],
                                        plate["selected"])

    def test_every_crop_is_inside_the_plate_neatline(self):
        for key, plate in self.plates.items():
            frame = self.georef[key]["georef"]["frame"]
            self.assertEqual(plate["plateWidthPx"] > frame["inner_right"], True)
            self.assertEqual(plate["plateHeightPx"] > frame["inner_bottom"], True)
            for candidate in plate["candidates"]:
                with self.subTest(plate=key, occurrence=candidate["occurrenceId"]):
                    crop = candidate["cropPx"]
                    box = candidate["boxPx"]
                    self.assertGreater(crop["w"], 0)
                    self.assertGreater(crop["h"], 0)
                    self.assertGreaterEqual(crop["x"], frame["inner_left"])
                    self.assertGreaterEqual(crop["y"], frame["inner_top"])
                    self.assertLessEqual(crop["x"] + crop["w"], frame["inner_right"])
                    self.assertLessEqual(crop["y"] + crop["h"],
                                         frame["inner_bottom"])
                    # the box the detector proposed sits inside the padded crop
                    self.assertGreaterEqual(box["x"], crop["x"])
                    self.assertGreaterEqual(box["y"], crop["y"])
                    self.assertLessEqual(box["x"] + box["w"],
                                         crop["x"] + crop["w"])
                    self.assertLessEqual(box["y"] + box["h"],
                                         crop["y"] + crop["h"])

    def test_occurrence_ids_are_unique_and_derive_from_the_box(self):
        seen = set()
        for key, plate in self.plates.items():
            for candidate in plate["candidates"]:
                with self.subTest(plate=key, occurrence=candidate["occurrenceId"]):
                    self.assertNotIn(candidate["occurrenceId"], seen)
                    seen.add(candidate["occurrenceId"])
                    box = candidate["boxPx"]
                    expected = f"label:{box['x']}-{box['y']}-{box['w']}-{box['h']}"
                    self.assertEqual(candidate["occurrenceId"], expected)

    def test_coordinates_reproduce_the_published_transform(self):
        for key, plate in self.plates.items():
            model = self.georef[key]["georef"]["model"]
            ax, ay = model["apex"]["x"], model["apex"]["y"]
            n = model["coneConstant"]
            lon0 = model["centralLongitude"]
            c0, c1 = model["latitude"]["coef"]
            for candidate in plate["candidates"]:
                with self.subTest(plate=key, occurrence=candidate["occurrenceId"]):
                    px, py = candidate["centerPx"]
                    lon = lon0 + math.degrees(math.atan2(px - ax, py - ay)) / n
                    lat = c0 * math.hypot(px - ax, py - ay) + c1
                    self.assertAlmostEqual(candidate["coordinates"]["lon"], lon,
                                           places=5)
                    self.assertAlmostEqual(candidate["coordinates"]["lat"], lat,
                                           places=5)
                    # the plate's own bounds: a projected centre cannot sit
                    # outside the sheet the box was measured on
                    self.assertTrue(0.0 <= px <= plate["plateWidthPx"])
                    self.assertTrue(0.0 <= py <= plate["plateHeightPx"])

    def test_scores_are_components_not_a_single_verdict(self):
        for key, plate in self.plates.items():
            for candidate in plate["candidates"]:
                with self.subTest(plate=key, occurrence=candidate["occurrenceId"]):
                    self.assertGreaterEqual(candidate["componentCount"], 3)
                    self.assertGreaterEqual(candidate["inkDensity"], 0.0)
                    self.assertLessEqual(candidate["inkDensity"], 1.0)
                    self.assertGreater(candidate["labelConfidence"], 0.0)
                    self.assertLessEqual(candidate["labelConfidence"], 1.0)
                    self.assertGreater(candidate["medianLetterHeightPx"], 0.0)
                    self.assertNotIn("labelTextAr", candidate)
                    self.assertNotIn("eraLabel", candidate)
                    self.assertNotIn("status", candidate)

    def test_ranks_are_a_total_order_over_the_selected_set(self):
        for key, plate in self.plates.items():
            ranks = [c["rank"] for c in plate["candidates"]]
            self.assertEqual(ranks, list(range(len(plate["candidates"]))))

    def test_the_proposal_set_is_bounded_and_non_empty(self):
        total = sum(p["selected"] for p in self.plates.values())
        self.assertGreater(total, 0, "the lane produced nothing to review")
        self.assertLessEqual(total, self.artifact["meta"]["candidateLimit"])


if __name__ == "__main__":
    unittest.main()
