import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parent))
import run_baselines  # noqa: E402


class FakeImage:
    def __init__(self, width, height):
        self.size = (width, height)

    def crop(self, bounds):
        left, top, right, bottom = bounds
        return FakeImage(right - left, bottom - top)


class BaselineTests(unittest.TestCase):
    def test_tiles_use_fixed_overlap_and_cover_the_final_edges(self):
        bounds = run_baselines.tile_bounds(2300, 1800)
        self.assertEqual(len(bounds), 9)
        self.assertEqual(bounds[:3], [(0, 0, 1024, 1024), (768, 0, 1024, 1024), (1536, 0, 764, 1024)])
        self.assertEqual(bounds[-1], (1536, 1536, 764, 264))
        for y in range(1800):
            self.assertTrue(any(top <= y < top + height for _, top, _, height in bounds))
        for x in range(2300):
            self.assertTrue(any(left <= x < left + width for left, _, width, _ in bounds))

    def test_branch_regions_reject_out_of_bounds_and_empty_pixel_boxes(self):
        self.assertEqual(
            run_baselines._region_bounds({"x": 0.5, "y": 0.25, "width": 0.5, "height": 0.75}, 101, 99),
            (50, 24, 101, 99),
        )
        self.assertIsNone(run_baselines._region_bounds({"x": 0.9, "y": 0.1, "width": 0.2, "height": 0.2}, 100, 100))
        self.assertIsNone(run_baselines._region_bounds({"x": 0.1, "y": 0.1, "width": 0.0, "height": 0.2}, 100, 100))

    def test_deduplication_preserves_repeated_names_in_distinct_regions(self):
        lines = [
            {"name_ar": "محمد", "confidence": 90, "region_px": (10, 10, 40, 30)},
            {"name_ar": "محمد", "confidence": 70, "region_px": (12, 11, 42, 31)},
            {"name_ar": "محمد", "confidence": 80, "region_px": (210, 110, 240, 130)},
        ]
        self.assertEqual(len(run_baselines.dedupe(lines)), 2)

    def test_operation_count_measures_empty_tile_calls(self):
        image = FakeImage(1800, 100)
        with patch.object(run_baselines, "load_image", return_value=image), patch.object(run_baselines, "png_bytes", return_value=b"png"), patch.object(run_baselines, "tess_lines", return_value=[]) as read:
            result = run_baselines.process_page({}, {"page_id": "p1", "image_path": "unused.png"}, Path("manifest.json"), "tiled_tesseract")
        self.assertEqual(read.call_count, 3)
        self.assertEqual(result[3], 3)

    def test_manifest_rejects_duplicate_pages(self):
        with tempfile.TemporaryDirectory() as directory:
            manifest = Path(directory) / "manifest.json"
            manifest.write_text(
                '[{"document_id":"d","source_class":"chart","pages":['
                '{"page_id":"p1","image_path":"one.png"},'
                '{"page_id":"p1","image_path":"two.png"}]}]',
                encoding="utf-8",
            )
            with self.assertRaisesRegex(ValueError, "duplicate page_id"):
                run_baselines.read_manifest(manifest)


if __name__ == "__main__":
    unittest.main()
