#!/usr/bin/env python3
"""Generate local Arabic Tesseract comparison files from a private page manifest.

Requires Pillow and a Tesseract installation with the Arabic (`ara`) model. The
manifest and images stay on disk; this script makes no network requests.
"""

from __future__ import annotations

import argparse
import csv
import io
import json
import math
import shutil
import subprocess
import sys
import time
from collections import defaultdict
from pathlib import Path
from typing import Any

TILE_SIZE = 1024
TILE_OVERLAP = 256
DUPLICATE_IOU = 0.65


def read_manifest(path: Path) -> list[dict[str, Any]]:
    try:
        value = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as error:
        raise ValueError(f"cannot read manifest {path}: {error}") from error
    if not isinstance(value, list):
        raise ValueError("manifest must be a JSON array of document records")
    seen: set[str] = set()
    for document in value:
        if not isinstance(document, dict) or not isinstance(document.get("document_id"), str):
            raise ValueError("every manifest document needs a string document_id")
        if document["document_id"] in seen:
            raise ValueError(f"duplicate manifest document_id {document['document_id']!r}")
        seen.add(document["document_id"])
        if document.get("source_class") not in {"chart", "book", "mixed"}:
            raise ValueError(f"invalid source_class for {document['document_id']!r}")
        if document.get("split", "unspecified") not in {"development", "held_out", "unspecified"}:
            raise ValueError(f"invalid split for {document['document_id']!r}")
        pages = document.get("pages")
        if not isinstance(pages, list) or not pages:
            raise ValueError(f"document {document['document_id']!r} needs at least one page")
        page_ids = set()
        for page in pages:
            if not isinstance(page, dict) or not isinstance(page.get("page_id"), str) or not isinstance(page.get("image_path"), str):
                raise ValueError("each page needs page_id and image_path strings")
            if page["page_id"] in page_ids:
                raise ValueError(f"duplicate page_id {page['page_id']!r} in {document['document_id']!r}")
            page_ids.add(page["page_id"])
    return value


def load_image(path: Path):
    try:
        from PIL import Image, ImageOps
    except ImportError as error:
        raise RuntimeError("Pillow is required: python3 -m pip install Pillow") from error
    try:
        image = Image.open(path)
        image.load()
        # Respect orientation metadata without scaling pixel content.
        return ImageOps.exif_transpose(image).convert("RGB")
    except Exception as error:
        raise ValueError(f"cannot decode page image {path}: {error}") from error


def png_bytes(image) -> bytes:
    output = io.BytesIO()
    image.save(output, format="PNG", optimize=False)
    return output.getvalue()


def tess_lines(image_bytes: bytes, image_name: str) -> list[dict[str, Any]]:
    command = ["tesseract", "stdin", "stdout", "-l", "ara", "--psm", "11", "tsv"]
    result = subprocess.run(command, input=image_bytes, capture_output=True, check=False)
    if result.returncode:
        error = result.stderr.decode("utf-8", errors="replace").strip()
        raise RuntimeError(f"Tesseract failed for {image_name}: {error or result.returncode}")
    rows = csv.DictReader(io.StringIO(result.stdout.decode("utf-8", errors="replace")), delimiter="\t")
    grouped: dict[tuple[str, ...], list[dict[str, Any]]] = defaultdict(list)
    for row in rows:
        text = (row.get("text") or "").strip()
        try:
            confidence = float(row.get("conf", "-1"))
            left, top, width, height = (int(row[key]) for key in ("left", "top", "width", "height"))
            level = int(row.get("level", "0"))
        except (TypeError, ValueError, KeyError):
            continue
        if not text or level != 5 or width <= 0 or height <= 0:
            continue
        key = tuple(row.get(column, "") or "" for column in ("page_num", "block_num", "par_num", "line_num"))
        grouped[key].append({"text": text, "confidence": confidence, "left": left, "top": top, "width": width, "height": height, "word_num": int(row.get("word_num", "0") or 0)})
    output = []
    for key, words in grouped.items():
        words.sort(key=lambda word: word["word_num"])
        x0 = min(word["left"] for word in words)
        y0 = min(word["top"] for word in words)
        x1 = max(word["left"] + word["width"] for word in words)
        y1 = max(word["top"] + word["height"] for word in words)
        output.append({
            "name_ar": " ".join(word["text"] for word in words),
            "status": "readable",
            "region_px": (x0, y0, x1, y1),
            "confidence": sum(word["confidence"] for word in words) / len(words),
            "line_key": key,
        })
    return output


def tile_bounds(width: int, height: int, size: int = TILE_SIZE, overlap: int = TILE_OVERLAP):
    stride = size - overlap
    def starts(length: int) -> list[int]:
        result = [0]
        while result[-1] + size < length:
            result.append(result[-1] + stride)
        return result

    xs, ys = starts(width), starts(height)
    return [(x, y, min(size, width - x), min(size, height - y)) for y in sorted(set(ys)) for x in sorted(set(xs))]


def _region_bounds(region: dict[str, Any], width: int, height: int) -> tuple[int, int, int, int] | None:
    try:
        x, y, w, h = (float(region[key]) for key in ("x", "y", "width", "height"))
    except (KeyError, TypeError, ValueError):
        return None
    if not all(math.isfinite(item) for item in (x, y, w, h)) or x < 0 or y < 0 or w <= 0 or h <= 0 or x + w > 1.000001 or y + h > 1.000001:
        return None
    bounds = (max(0, int(x * width)), max(0, int(y * height)), min(width, math.ceil((x + w) * width)), min(height, math.ceil((y + h) * height)))
    return bounds if bounds[2] > bounds[0] and bounds[3] > bounds[1] else None


def iou(left: tuple[float, float, float, float], right: tuple[float, float, float, float]) -> float:
    x0, y0 = max(left[0], right[0]), max(left[1], right[1])
    x1, y1 = min(left[2], right[2]), min(left[3], right[3])
    intersection = max(0, x1 - x0) * max(0, y1 - y0)
    union = (left[2] - left[0]) * (left[3] - left[1]) + (right[2] - right[0]) * (right[3] - right[1]) - intersection
    return intersection / union if union else 0


def dedupe(lines: list[dict[str, Any]]) -> list[dict[str, Any]]:
    kept: list[dict[str, Any]] = []
    for candidate in sorted(lines, key=lambda row: row["confidence"], reverse=True):
        if any(iou(candidate["region_px"], existing["region_px"]) >= DUPLICATE_IOU for existing in kept):
            continue
        kept.append(candidate)
    return sorted(kept, key=lambda row: (row["region_px"][1], row["region_px"][0]))


def normalize_lines(lines: list[dict[str, Any]], page_id: str, width: int, height: int, pipeline: str):
    output = []
    for index, line in enumerate(dedupe(lines)):
        x0, y0, x1, y1 = line["region_px"]
        output.append({
            "id": f"{pipeline}-{page_id}-{index:06d}",
            "page_id": page_id,
            "name_ar": line["name_ar"],
            "status": line["status"],
            "region": {"x": x0 / width, "y": y0 / height, "width": (x1 - x0) / width, "height": (y1 - y0) / height},
        })
    return output


def process_page(document: dict[str, Any], page: dict[str, Any], manifest_path: Path, pipeline: str):
    image_path = Path(page["image_path"])
    if not image_path.is_absolute():
        image_path = manifest_path.parent / image_path
    image = load_image(image_path)
    width, height = image.size
    started = time.perf_counter()
    input_bytes = 0
    operation_count = 0
    readings: list[dict[str, Any]] = []
    if pipeline == "full_page_tesseract":
        encoded = png_bytes(image)
        input_bytes += len(encoded)
        readings.extend(tess_lines(encoded, page["page_id"]))
        operation_count += 1
    elif pipeline == "tiled_tesseract":
        for x, y, crop_width, crop_height in tile_bounds(width, height):
            encoded = png_bytes(image.crop((x, y, x + crop_width, y + crop_height)))
            input_bytes += len(encoded)
            tile_readings = tess_lines(encoded, f"{page['page_id']} tile ({x},{y})")
            operation_count += 1
            for line in tile_readings:
                x0, y0, x1, y1 = line["region_px"]
                line["region_px"] = (x + x0, y + y0, x + x1, y + y1)
                readings.append(line)
    elif pipeline == "reviewed_branch_tesseract":
        branches = page.get("branches", [])
        for branch in branches:
            region = _region_bounds(branch.get("region", {}), width, height)
            if region is None:
                raise ValueError(f"invalid branch region on {document['document_id']}/{page['page_id']}")
            x0, y0, x1, y1 = region
            crop = image.crop((x0, y0, x1, y1))
            for tx, ty, crop_width, crop_height in tile_bounds(crop.width, crop.height):
                encoded = png_bytes(crop.crop((tx, ty, tx + crop_width, ty + crop_height)))
                input_bytes += len(encoded)
                tile_readings = tess_lines(encoded, f"{page['page_id']} reviewed branch")
                operation_count += 1
                for line in tile_readings:
                    lx0, ly0, lx1, ly1 = line["region_px"]
                    line["region_px"] = (x0 + tx + lx0, y0 + ty + ly0, x0 + tx + lx1, y0 + ty + ly1)
                    readings.append(line)
    else:
        raise ValueError(f"unknown pipeline {pipeline!r}")
    return normalize_lines(readings, page["page_id"], width, height, pipeline), time.perf_counter() - started, input_bytes, operation_count, width, height


def generate(manifest_path: Path, documents: list[dict[str, Any]], pipeline: str) -> list[dict[str, Any]]:
    output = []
    for document in documents:
        predicted_pages, nodes, processing_seconds, image_bytes, operation_count = [], [], 0.0, 0, 0
        for page in document["pages"]:
            detected, elapsed, byte_count, operations, width, height = process_page(document, page, manifest_path, pipeline)
            nodes.extend(detected)
            processing_seconds += elapsed
            image_bytes += byte_count
            operation_count += operations
            predicted_pages.append({"page_id": page["page_id"], "processed": True, "width": width, "height": height})
        output.append({
            "document_id": document["document_id"],
            "source_class": document["source_class"],
            "pages": predicted_pages,
            "nodes": nodes,
            "branches": [],
            "connectors": [],
            "edges": [],
            "processing_seconds": processing_seconds,
            "cost_usd": 0,
            "provider_call_count": 0,
            "ocr_operation_count": operation_count,
            "input_image_bytes": image_bytes,
            "pipeline": pipeline,
            "branch_provenance": "reviewer_corrected" if pipeline == "reviewed_branch_tesseract" else None,
            "limits": {"tile_size_pixels": TILE_SIZE, "tile_overlap_pixels": TILE_OVERLAP, "tesseract_language": "ara", "page_upscaling": False},
        })
    return output


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--manifest", type=Path, required=True, help="Private JSON manifest with local page image paths")
    parser.add_argument("--output-dir", type=Path, required=True, help="Private output directory; three prediction JSONL files are written")
    args = parser.parse_args()
    if not shutil.which("tesseract"):
        parser.error("Tesseract is not installed or not on PATH")
    try:
        languages = subprocess.run(["tesseract", "--list-langs"], capture_output=True, text=True, check=True).stdout.splitlines()
        if "ara" not in languages:
            parser.error("Tesseract Arabic model 'ara' is not installed")
        documents = read_manifest(args.manifest.resolve())
        args.output_dir.mkdir(parents=True, exist_ok=True)
        for pipeline in ("full_page_tesseract", "tiled_tesseract", "reviewed_branch_tesseract"):
            rows = generate(args.manifest.resolve(), documents, pipeline)
            output = args.output_dir / f"{pipeline}.jsonl"
            output.write_text("".join(json.dumps(row, ensure_ascii=False) + "\n" for row in rows), encoding="utf-8")
            print(f"wrote {output.name}: {len(rows)} documents")
    except (OSError, ValueError, RuntimeError, subprocess.SubprocessError) as error:
        print(f"baseline error: {error}", file=sys.stderr)
        return 2
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
