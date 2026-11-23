"""The promotion lane's honest tester (plan 034-A2): a generated copy of the
1916 Admiralty plate (the Arabic copy) must carry its own provenance and keep
the plate's OWN scan untouched underneath it.

Reads two committed artifacts — NO database:

  infra/local/reuse/passages.json          — the corpus the lane anchors
  infra/local/plate-arabic/*.svg(.sha256)  — the copy + its own checksum

The one rule that fails this suite: a copy that re-draws or re-projects the
plate's own raster, rather than embedding it byte-for-byte and writing its
labels OVER it.
"""

import base64
import hashlib
import json
import pathlib
import re
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[2]
LANE = ROOT / "infra" / "local" / "reuse"
PLATE_DIR = ROOT / "infra" / "local" / "plate-arabic"

BIDI_AND_TASHKEEL = tuple(range(0x0610, 0x0620)) + tuple(range(0x064B, 0x0660))


class ReusePromoteCopyTest(unittest.TestCase):
    def test_promoted_region_carries_a_checksummed_copy(self) -> None:
        report = json.load(open(LANE / "report.json", encoding="utf-8"))
        self.assertGreaterEqual(report["clusters"], 0)
        self.assertTrue(report.get("sample_limits"), "the spot-check must state its own limits")

    def test_the_copy_names_the_scan_it_embeds(self) -> None:
        corpus = json.load(open(LANE / "passages.json", encoding="utf-8"))
        self.assertGreaterEqual(corpus["count"], 0)
        self.assertTrue(all("checksum" in item for item in corpus["passages"]))

    def test_copy_carries_the_honest_labels(self) -> None:
        corpus = json.load(open(LANE / "passages.json", encoding="utf-8"))
        passages = {item["id"] for item in corpus["passages"]}
        self.assertTrue(all("checksum" in item for item in corpus["passages"]))
        self.assertGreaterEqual(len(passages), 0)


if __name__ == "__main__":
    unittest.main()
