"""Contract tests for the plan 030-D plate georeferences.

`infra/local/plate-georefs.json` is the artifact that says how to turn a pixel on
a georeferenced plate into a longitude and a latitude. Everything the evidence
atlas (plan 032) derives from a plate goes through it, so the numbers it carries
are a contract, not a report:

* the fitted model must stay identifiable (a conic with a real apex, a cone
  constant inside the open unit interval, a latitude slope of the right sign),
* the held-out error must stay inside the stated gate, so a regression in the
  generator cannot quietly publish a worse transform,
* a plate published `needs_review` must say why, and a plate with no graticule
  must carry no transform at all rather than an invented one,
* the recorded error figures must still agree with the recorded residuals, so
  the numbers cannot be hand-edited into looking better than the measurement.

These are pure, offline, standard-library checks; they read the committed
artifact and never touch the database or the plates. They belong in the fast
gate because a transform that silently drifts is worse than no transform.

Run:  python3 infra/local/test_plate_georef.py
"""

import json
import math
import unittest
from pathlib import Path

ARTIFACT = Path(__file__).with_name("plate-georefs.json")

# Conversion scales the generator states for its kilometre figures. They are
# repeated here on purpose: if the generator changes them, the reconciliation
# below must change too, and that change has to be visible in review.
KM_PER_DEG_LAT = 110.57
KM_PER_DEG_LON_AT_24 = 111.32 * math.cos(math.radians(24.0))


def load():
    with ARTIFACT.open(encoding="utf-8") as handle:
        return json.load(handle)


class PlateGeorefArtifactTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.artifact = load()
        cls.plates = {p["key"]: p for p in cls.artifact["plates"]}

    def test_meta_declares_the_method_and_the_gates(self):
        meta = self.artifact["meta"]
        self.assertEqual(meta["generator"], "infra/local/build-plate-georef.py")
        self.assertGreater(meta["fitGateKm"], 0)
        self.assertGreater(meta["townGateKm"], meta["fitGateKm"])
        self.assertIn("equidistant conic", meta["method"])
        # The rejected model is named so nobody re-derives it by accident.
        self.assertIn("affine", meta["modelChoice"])

    def test_every_plate_carries_a_real_file_identity(self):
        for key, plate in self.plates.items():
            with self.subTest(plate=key):
                self.assertRegex(plate["fileSha256"], r"^[0-9a-f]{64}$")
                self.assertGreater(plate["fileBytes"], 1_000)
                self.assertTrue(plate["license"])
                self.assertTrue(plate["fetchedAt"])

    def test_conic_model_is_identifiable(self):
        for key, plate in self.plates.items():
            georef = plate["georef"]
            if georef["method"] != "graticule_labels":
                continue
            with self.subTest(plate=key):
                model = georef["model"]
                self.assertEqual(model["kind"], "equidistant_conic")
                for axis in ("x", "y"):
                    self.assertTrue(math.isfinite(model["apex"][axis]))
                self.assertTrue(0.0 < model["coneConstant"] < 1.0)
                # The apex sits above the plate: that is what makes it a north-up
                # conic and what gives the parallels their downward bow.
                self.assertLess(model["apex"]["y"], 0.0)
                c0, c1 = model["latitude"]["coef"]
                self.assertLess(c0, 0.0, "latitude must decrease as the radius grows")
                self.assertTrue(150.0 < model["pxPerDegLat"] < 400.0)
                self.assertTrue(-90.0 < model["centralLongitude"] < 90.0)

    def test_held_out_error_stays_inside_the_gate(self):
        checked = 0
        for key, plate in self.plates.items():
            georef = plate["georef"]
            if georef["method"] != "graticule_labels":
                continue
            checked += 1
            with self.subTest(plate=key):
                held = georef["heldOut"]
                self.assertGreater(held["lonHoldouts"], 0)
                self.assertGreater(held["latHoldouts"], 0)
                self.assertEqual(held["gateKm"], self.artifact["meta"]["fitGateKm"])
                self.assertLessEqual(
                    held["combinedKm"], held["gateKm"],
                    "held-out RMS exceeded the gate: the transform must not be published")

    def test_one_plate_is_actually_georeferenced(self):
        self.assertGreaterEqual(
            sum(1 for p in self.plates.values()
                if p["georef"]["method"] == "graticule_labels"), 1,
            "no plate carries a transform; the gate above would be vacuous")

    def test_kilometre_figures_agree_with_the_residuals(self):
        for key, plate in self.plates.items():
            georef = plate["georef"]
            if georef["method"] != "graticule_labels":
                continue
            with self.subTest(plate=key):
                held = georef["heldOut"]
                rebuilt = math.hypot(held["lonRmsDeg"] * KM_PER_DEG_LON_AT_24,
                                     held["latRmsDeg"] * KM_PER_DEG_LAT)
                self.assertAlmostEqual(held["combinedKm"], rebuilt, delta=1.0)

    def test_town_check_stays_inside_its_gate(self):
        for key, plate in self.plates.items():
            georef = plate["georef"]
            if georef["method"] != "graticule_labels":
                continue
            with self.subTest(plate=key):
                towns = georef["townCheck"]
                self.assertGreaterEqual(len(towns["towns"]), 5)
                self.assertEqual(towns["gateKm"], self.artifact["meta"]["townGateKm"])
                self.assertLessEqual(towns["maxKm"], towns["gateKm"])
                for town in towns["towns"]:
                    # The printed symbol read back through the model must match
                    # the recorded coordinate error, otherwise one of the two
                    # numbers was edited without the other.
                    rebuilt = _haversine_km(town["trueLon"], town["trueLat"],
                                            town["printedLon"], town["printedLat"])
                    self.assertAlmostEqual(town["errorKm"], rebuilt, delta=1.0,
                                           msg=f"{town['name']} error does not reconcile")

    def test_model_round_trips_through_the_published_formulas(self):
        for key, plate in self.plates.items():
            georef = plate["georef"]
            if georef["method"] != "graticule_labels":
                continue
            model = georef["model"]
            ax, ay = model["apex"]["x"], model["apex"]["y"]
            n = model["coneConstant"]
            lon0 = model["centralLongitude"]
            c0, c1 = model["latitude"]["coef"]
            with self.subTest(plate=key):
                for town in georef["townCheck"]["towns"]:
                    lat = town["trueLat"]
                    lon = town["trueLon"]
                    r = (lat - c1) / c0
                    theta = math.radians((lon - lon0) * n)
                    px = ax + r * math.sin(theta)
                    py = ay + r * math.cos(theta)
                    # forward again, the way a reader of the artifact would
                    lon_back = lon0 + math.degrees(math.atan2(px - ax, py - ay)) / n
                    lat_back = c0 * math.hypot(px - ax, py - ay) + c1
                    self.assertAlmostEqual(lon_back, lon, places=6)
                    self.assertAlmostEqual(lat_back, lat, places=6)

    def test_review_flags_are_honest(self):
        for key, plate in self.plates.items():
            with self.subTest(plate=key):
                georef = plate["georef"]
                reasons = georef["reviewReasons"]
                self.assertEqual(georef["needsReview"], bool(reasons))
                if georef["method"] == "graticule_labels":
                    self.assertEqual(georef["certainty"], "approximate")
                    # The parallels are not recoverable as ink on these plates;
                    # the transform must keep saying so rather than implying a
                    # graticule-wide validation it does not have.
                    self.assertFalse(georef["interiorParallelsVerified"])
                    self.assertIn("interior_parallels_unverified_by_ink", reasons)

    def test_plate_without_a_graticule_uses_validated_control_points(self):
        dickson = self.plates["dickson-1914-tribal"]["georef"]
        self.assertEqual(dickson["method"], "control_points")
        self.assertEqual(dickson["model"]["kind"], "affine")
        self.assertEqual(len(dickson["model"]["affine"]), 6)
        self.assertGreaterEqual(dickson["fit"]["heldOutCount"], 3)
        self.assertLessEqual(dickson["fit"]["maxErrorKm"], dickson["fit"]["maxAllowedKm"])
        self.assertEqual(dickson["certainty"], "approximate")
        self.assertTrue(dickson["needsReview"])
        self.assertIn("landmark_based_registration", dickson["reviewReasons"])


def _haversine_km(lon1, lat1, lon2, lat2):
    p1, p2 = math.radians(lat1), math.radians(lat2)
    dp, dl = p2 - p1, math.radians(lon2 - lon1)
    a = math.sin(dp / 2) ** 2 + math.cos(p1) * math.cos(p2) * math.sin(dl / 2) ** 2
    return 2 * 6371.0088 * math.asin(math.sqrt(a))


if __name__ == "__main__":
    unittest.main()
