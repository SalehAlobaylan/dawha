#!/usr/bin/env python3
"""Extract review proposals from a local copy of an atlas source item."""

import argparse
import csv
import hashlib
import io
import json
import re
import subprocess
import tempfile
import xml.etree.ElementTree as ET
from collections import defaultdict
from pathlib import Path


def run(*args):
    result = subprocess.run(args, check=True, stdout=subprocess.PIPE)
    return result.stdout.decode("utf-8")


def image_lines(image, language):
    words = csv.DictReader(
        io.StringIO(run("tesseract", str(image), "stdout", "-l", language, "tsv")),
        delimiter="\t",
    )
    lines = defaultdict(list)
    for word in words:
        if word["level"] == "5" and word["text"].strip():
            line = (word["block_num"], word["par_num"], word["line_num"])
            lines[line].append(word)

    proposals = []
    for words in lines.values():
        boxes = [
            tuple(int(word[axis]) for axis in ("left", "top", "width", "height"))
            for word in words
        ]
        x = min(box[0] for box in boxes)
        y = min(box[1] for box in boxes)
        right = max(left + width for left, _, width, _ in boxes)
        bottom = max(top + height for _, top, _, height in boxes)
        proposals.append({
            "original": " ".join(word["text"] for word in words),
            "selector": {"pixelRect": [x, y, right - x, bottom - y]},
            "ocrWordConfidence": [float(word["conf"]) for word in words],
        })
    return proposals


def pdf_proposals(path, page, language, dpi, prefer_ocr):
    options = ("-f", str(page), "-l", str(page))
    if not prefer_ocr:
        text = run("pdftotext", *options, str(path), "-").strip()
        if text:
            return "pdftotext", {}, [{"original": text, "selector": {"pdfPage": page}}]
    with tempfile.TemporaryDirectory() as temp:
        render = Path(temp) / "page"
        run("pdftoppm", *options, "-r", str(dpi), "-png", "-singlefile", str(path), str(render))
        proposals = image_lines(render.with_suffix(".png"), language)
    for proposal in proposals:
        proposal["selector"]["pdfPage"] = page
    return "tesseract", {"pdfPage": page, "renderDpi": dpi}, proposals


def svg_proposals(path):
    root = ET.parse(path).getroot()
    proposals = []
    for index, element in enumerate(root.iter("{http://www.w3.org/2000/svg}text"), start=1):
        text = "".join(element.itertext()).strip()
        if text:
            proposals.append({
                "original": text,
                "selector": {"svgTextOrdinal": index, "elementId": element.get("id")},
            })
    return proposals


def vtt_proposals(path):
    cues = []
    pattern = re.compile(r"(\S+)\s+-->\s+(\S+)")
    for block in re.split(r"\n\s*\n", path.read_text(encoding="utf-8-sig")):
        lines = block.splitlines()
        for index, line in enumerate(lines[:2]):
            match = pattern.match(line)
            if match and index + 1 < len(lines):
                cues.append({
                    "original": "\n".join(lines[index + 1:]),
                    "selector": {"start": match[1], "end": match[2]},
                })
                break
    return cues


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("file", type=Path)
    parser.add_argument("--item", required=True, help="Catalogue item key")
    parser.add_argument("--page", type=int, help="One-based PDF page")
    parser.add_argument("--ocr", action="store_true", help="Read the PDF page as an image")
    parser.add_argument("--outline-render", type=Path, help="Local raster rendered from the same outlined SVG")
    parser.add_argument("--language", default="ara+eng", help="Tesseract languages")
    parser.add_argument("--dpi", type=int, default=200)
    args = parser.parse_args()

    path = args.file.resolve(strict=True)
    suffix = path.suffix.lower()
    method = ""
    frame = {}
    if suffix == ".pdf":
        if not args.page or args.page < 1 or not 72 <= args.dpi <= 400:
            parser.error("PDF staging requires a positive --page and --dpi between 72 and 400")
        method, frame, proposals = pdf_proposals(path, args.page, args.language, args.dpi, args.ocr)
    elif suffix in {".jpg", ".jpeg", ".png", ".tif", ".tiff"}:
        method = "tesseract"
        proposals = image_lines(path, args.language)
    elif suffix == ".svg":
        if args.outline_render:
            render = args.outline_render.resolve(strict=True)
            with render.open("rb") as handle:
                frame["outlineRenderSHA256"] = hashlib.file_digest(handle, "sha256").hexdigest()
            method = "tesseract"
            proposals = image_lines(render, args.language)
        else:
            method = "svg-text"
            proposals = svg_proposals(path)
            if not proposals:
                parser.error("No SVG text found. For outlined letters, supply a rendered image with --outline-render.")
    elif suffix == ".vtt":
        method = "webvtt"
        proposals = vtt_proposals(path)
    else:
        parser.error("Expected PDF, image, SVG or WebVTT")

    if method == "tesseract":
        engine = run("tesseract", "--version").splitlines()[0]
    elif method == "pdftotext":
        version = subprocess.run(["pdftotext", "-v"], check=True, capture_output=True, text=True)
        engine = version.stderr.splitlines()[0]
    else:
        engine = "dawha-atlas-staging-v1"

    with path.open("rb") as source:
        digest = hashlib.file_digest(source, "sha256").hexdigest()
    print(json.dumps({
        "itemKey": args.item,
        "sha256": digest,
        "method": method,
        "engine": engine,
        "frame": frame,
        "language": args.language if method == "tesseract" else None,
        "reviewStatus": "suggested",
        "proposals": proposals,
    }, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
