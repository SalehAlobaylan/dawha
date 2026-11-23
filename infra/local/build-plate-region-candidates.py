#!/usr/bin/env python3
"""Plan 030-E: propose region-label crops on a georeferenced plate.

What this lane does, and what it refuses to do
----------------------------------------------
A plate is ink. This builder finds ink that *looks like printed text*: small
components of letter-like size and fill, grouped into a single text-line box,
inside the neatline, on a plate whose georeference plan 030-D already published.
It then projects each box centre through that transform and writes one
`external_region_candidates` row.

It does not read the text. It cannot tell a tribal name from a town name, from
the map's own title cartouche, and it does not try. `label_text_ar` stays empty
and `label_read_by` stays NULL; only a reviewer fills them, and only a reviewer
decides that a crop names a tribe. That is the whole point of the lane: the
machine narrows a 7,163 x 7,170 sheet down to bounded crops a person can read,
and nothing it writes is a fact.

The detector is deterministic and offline — no hosted vision model, no per-tile
approval to obtain, and no claim of OCR. It is the "machine proposes, human
reads" contract the rest of the repository already uses for names (plans
019/021), applied to a plate.

Known, measured limits, stated rather than hidden:
  * the proposal is a text *block*, so a town name printed immediately above a
    tribal name can share one box (the reviewer sees both in the crop and reads
    the tribal one);
  * a strongly rotated or curved label can split into two proposals;
  * the title cartouche and the scale bar are text too, and they are proposed
    like everything else, because the machine cannot know they are not tribes.
These are properties of the proposal, not defects to be papered over: the
reviewer's rejection is the mechanism that removes them.

Run:    python3 infra/local/build-plate-region-candidates.py
Needs:  numpy, macOS `sips` for the BMP derivative, the stored plate file, and
        `infra/local/plate-georefs.json` from 030-D.
"""

from __future__ import annotations

import hashlib
import importlib.util
import json
import math
import sys
from datetime import date
from pathlib import Path

import numpy as np

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[1]
GEOREF = HERE / "plate-georefs.json"
ARTIFACT = HERE / "plate-region-candidates.json"
SEED = HERE / "seed-plate-region-candidates.sql"
STORAGE = ROOT / ".data" / "source-storage"

# Plan 030-E registers which source row owns each plate. The 030-D seed created
# these rows; the file checksum is re-verified below, so a plate cannot be
# measured under another plate's source id.
SOURCE_FOR_PLATE = {
    "admiralty-1916-tribal": "b0000000-0000-0000-0000-00000000e004",
}

# Detector parameters. All lengths are downscaled pixels; the artifact records
# the factor, so a reader can convert every bound back to full resolution.
DOWNSCALE = 2
INK_THRESHOLD = 165

LETTER_MIN_AREA, LETTER_MAX_AREA = 6, 1500
LETTER_MIN_H, LETTER_MAX_H = 6, 40
LETTER_MIN_W, LETTER_MAX_W = 2, 44
LETTER_MIN_FILL = 0.10
LETTER_MAX_ASPECT = 6.0

GROUP_GAP_FACTOR = 0.90
GROUP_DY_FACTOR = 0.40
GROUP_OVERLAP_FACTOR = 0.40
GROUP_HEIGHT_RATIO = 0.60

BOX_MIN_LETTERS = 3
BOX_MIN_W, BOX_MAX_W = 18, 620
BOX_MIN_H, BOX_MAX_H = 14, 90
BOX_MIN_MEDIAN_H = 11

FRAME_INSET_PX = 10
CROP_PAD_PX = 8
CANDIDATE_LIMIT = 80


def load_georef() -> dict:
    with GEOREF.open(encoding="utf-8") as handle:
        return json.load(handle)


def plate_builder():
    """The 030-D module, imported for its image decoding and cache contract."""
    spec = importlib.util.spec_from_file_location(
        "dawha_plate_georef", HERE / "build-plate-georef.py")
    if spec is None or spec.loader is None:
        raise RuntimeError("cannot load build-plate-georef.py")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


# ---------------------------------------------------------------------------
# connected components, run-based
# ---------------------------------------------------------------------------

def components(binary: np.ndarray) -> np.ndarray:
    """Label 4-connected components; return (area, x0, x1, y0, y1) per label.

    A run-based union-find rather than a pixel loop: the plate is 3,600 x 3,600
    after downscaling, and the stipple means there are many tiny components even
    though there is very little ink. Row runs keep the Python loop bounded by
    ink, not by pixels.
    """
    height, width = binary.shape
    parent: list[int] = []
    stats: list[list[int]] = []

    def find(x: int) -> int:
        while parent[x] != x:
            parent[x] = parent[parent[x]]
            x = parent[x]
        return x

    def union(a: int, b: int) -> None:
        ra, rb = find(a), find(b)
        if ra != rb:
            parent[max(ra, rb)] = min(ra, rb)

    prev: list[tuple[int, int, int]] = []
    for y in range(height):
        row = binary[y]
        edges = np.diff(np.concatenate(([0], row.view(np.int8), [0])))
        starts = np.flatnonzero(edges == 1)
        ends = np.flatnonzero(edges == -1)
        current: list[tuple[int, int, int]] = []
        j = 0
        for s, e in zip(starts.tolist(), ends.tolist()):
            label = None
            while j < len(prev) and prev[j][1] <= s:
                j += 1
            k = j
            while k < len(prev) and prev[k][0] < e:
                other = prev[k][2]
                if label is None:
                    label = other
                else:
                    union(label, other)
                k += 1
            if label is None:
                label = len(parent)
                parent.append(label)
                stats.append([0, s, e - 1, y, y])
            entry = stats[label]
            entry[0] += e - s
            if s < entry[1]:
                entry[1] = s
            if e - 1 > entry[2]:
                entry[2] = e - 1
            entry[4] = y
            current.append((s, e, label))
        prev = current

    for i in range(len(parent)):
        root = find(i)
        if root == i:
            continue
        source, target = stats[i], stats[root]
        target[0] += source[0]
        target[1] = min(target[1], source[1])
        target[2] = max(target[2], source[2])
        target[3] = min(target[3], source[3])
        target[4] = max(target[4], source[4])

    return np.array([stats[i] for i in range(len(parent)) if find(i) == i],
                    dtype=np.float64)


# ---------------------------------------------------------------------------
# text-line proposal
# ---------------------------------------------------------------------------

def letter_components(gray: np.ndarray, frame: dict) -> np.ndarray:
    """Letter-sized components inside the neatline, as (area,x0,x1,y0,y1)."""
    height, width = gray.shape
    hh, ww = height // DOWNSCALE * DOWNSCALE, width // DOWNSCALE * DOWNSCALE
    small = gray[:hh, :ww].reshape(
        hh // DOWNSCALE, DOWNSCALE, ww // DOWNSCALE, DOWNSCALE).min(axis=(1, 3))

    comps = components(small < INK_THRESHOLD)
    if not len(comps):
        return comps

    area = comps[:, 0]
    x0, x1 = comps[:, 1], comps[:, 2]
    y0, y1 = comps[:, 3], comps[:, 4]
    w, h = x1 - x0 + 1, y1 - y0 + 1
    fill = area / (w * h)

    left = frame["inner_left"] / DOWNSCALE + FRAME_INSET_PX
    right = frame["inner_right"] / DOWNSCALE - FRAME_INSET_PX
    top = frame["inner_top"] / DOWNSCALE + FRAME_INSET_PX
    bottom = frame["inner_bottom"] / DOWNSCALE - FRAME_INSET_PX

    keep = (
        (area >= LETTER_MIN_AREA) & (area <= LETTER_MAX_AREA)
        & (h >= LETTER_MIN_H) & (h <= LETTER_MAX_H)
        & (w >= LETTER_MIN_W) & (w <= LETTER_MAX_W)
        & (fill >= LETTER_MIN_FILL)
        & (w / h <= LETTER_MAX_ASPECT)
        & (x0 >= left) & (x1 <= right) & (y0 >= top) & (y1 <= bottom)
    )
    return comps[keep]


def group_lines(letters: np.ndarray) -> list[dict]:
    """Group letters into single-baseline text lines.

    Two letters join when their sizes are compatible, their vertical centres are
    close, they overlap vertically, and the horizontal gap between them is small
    relative to their height. Every bound is a fraction of the measured letter
    height, so the grouping does not depend on one plate's type size.
    """
    count = len(letters)
    if count < BOX_MIN_LETTERS:
        return []

    area, x0, x1, y0, y1 = (letters[:, i] for i in range(5))
    h = y1 - y0 + 1
    fill = area / ((x1 - x0 + 1) * h)
    centre = (y0 + y1) / 2.0
    median_h = float(np.median(h))

    parent = list(range(count))

    def find(a: int) -> int:
        while parent[a] != a:
            parent[a] = parent[parent[a]]
            a = parent[a]
        return a

    def union(a: int, b: int) -> None:
        ra, rb = find(a), find(b)
        if ra != rb:
            parent[max(ra, rb)] = min(ra, rb)

    cell = int(max(8.0, 1.5 * median_h))
    buckets: dict[tuple[int, int], list[int]] = {}
    for i in range(count):
        for gy in range(int(y0[i] // cell) - 1, int(y1[i] // cell) + 2):
            for gx in range(int(x0[i] // cell) - 1, int(x1[i] // cell) + 2):
                buckets.setdefault((gy, gx), []).append(i)

    for indexes in buckets.values():
        for a in range(len(indexes)):
            for b in range(a + 1, len(indexes)):
                i, j = indexes[a], indexes[b]
                if find(i) == find(j):
                    continue
                taller = max(h[i], h[j])
                if min(h[i], h[j]) / taller < GROUP_HEIGHT_RATIO:
                    continue
                if abs(centre[i] - centre[j]) > GROUP_DY_FACTOR * taller:
                    continue
                overlap = min(y1[i], y1[j]) - max(y0[i], y0[j]) + 1
                if overlap < GROUP_OVERLAP_FACTOR * min(h[i], h[j]):
                    continue
                gap = max(x0[i], x0[j]) - min(x1[i], x1[j]) - 1
                if gap > GROUP_GAP_FACTOR * taller:
                    continue
                union(i, j)

    groups: dict[int, list[int]] = {}
    for i in range(count):
        groups.setdefault(find(i), []).append(i)

    boxes = []
    for members in groups.values():
        idx = np.array(members)
        bx0, bx1 = int(x0[idx].min()), int(x1[idx].max())
        by0, by1 = int(y0[idx].min()), int(y1[idx].max())
        bw, bh = bx1 - bx0 + 1, by1 - by0 + 1
        median_letter = float(np.median(h[idx]))
        if len(idx) < BOX_MIN_LETTERS:
            continue
        if not (BOX_MIN_W <= bw <= BOX_MAX_W and BOX_MIN_H <= bh <= BOX_MAX_H):
            continue
        if median_letter < BOX_MIN_MEDIAN_H:
            continue
        heights = h[idx]
        uniformity = 0.0
        if float(heights.mean()) > 0:
            uniformity = 1.0 - float(heights.std()) / float(heights.mean())
        boxes.append({
            "letters": int(len(idx)),
            "x": bx0, "y": by0, "w": bw, "h": bh,
            "medianLetterH": median_letter,
            "meanFill": float(np.mean(fill[idx])),
            "heightUniformity": max(0.0, min(1.0, uniformity)),
        })
    return boxes


def confidence(box: dict) -> float:
    """How text-like the box looks — never how likely it is a tribe.

    Equal parts: enough letter-sized components, letters that fill their boxes
    like type rather than like dots, and a consistent type size across the line.
    """
    letters = min(1.0, box["letters"] / 10.0)
    fill = min(1.0, box["meanFill"] / 0.55)
    return round(min(1.0, 0.45 * letters + 0.35 * fill
                     + 0.20 * box["heightUniformity"]), 4)


# ---------------------------------------------------------------------------
# projection
# ---------------------------------------------------------------------------

def projection(model: dict):
    """The published forward transform, read from the artifact — not re-fitted."""
    ax, ay = model["apex"]["x"], model["apex"]["y"]
    n = model["coneConstant"]
    lon0 = model["centralLongitude"]
    c0, c1 = model["latitude"]["coef"]

    def to_geo(px: float, py: float) -> tuple[float, float]:
        lon = lon0 + math.degrees(math.atan2(px - ax, py - ay)) / n
        lat = c0 * math.hypot(px - ax, py - ay) + c1
        return lon, lat

    return to_geo


def inside_frame(box: dict, frame: dict, scale: int) -> bool:
    left = box["x"] * scale
    right = (box["x"] + box["w"]) * scale
    top = box["y"] * scale
    bottom = (box["y"] + box["h"]) * scale
    return (left >= frame["inner_left"] and right <= frame["inner_right"]
            and top >= frame["inner_top"] and bottom <= frame["inner_bottom"])


def build_candidates(gray: np.ndarray, frame: dict, model: dict,
                     to_geo) -> tuple[list[dict], int]:
    letters = letter_components(gray, frame)
    boxes = group_lines(letters)
    detected = len(boxes)

    candidates = []
    for box in boxes:
        box["confidence"] = confidence(box)
        if not inside_frame(box, frame, DOWNSCALE):
            continue
        scale = DOWNSCALE
        full = {"x": box["x"] * scale, "y": box["y"] * scale,
                "w": box["w"] * scale, "h": box["h"] * scale}
        pad = CROP_PAD_PX
        x0 = max(int(frame["inner_left"]), full["x"] - pad)
        y0 = max(int(frame["inner_top"]), full["y"] - pad)
        x1 = min(int(frame["inner_right"]), full["x"] + full["w"] + pad)
        y1 = min(int(frame["inner_bottom"]), full["y"] + full["h"] + pad)
        crop = {"x": x0, "y": y0, "w": x1 - x0, "h": y1 - y0}
        cx = full["x"] + full["w"] / 2.0
        cy = full["y"] + full["h"] / 2.0
        lon, lat = to_geo(cx, cy)
        crop_slice = gray[crop["y"]:crop["y"] + crop["h"],
                          crop["x"]:crop["x"] + crop["w"]].astype(np.float32)
        ink = float(np.mean(1.0 - crop_slice / 255.0)) if crop_slice.size else 0.0
        candidates.append({
            "occurrenceId": f"label:{full['x']}-{full['y']}-{full['w']}-{full['h']}",
            "cropPx": crop,
            "boxPx": full,
            "centerPx": [round(cx, 1), round(cy, 1)],
            "coordinates": {"lon": round(lon, 6), "lat": round(lat, 6)},
            "inkDensity": round(ink, 4),
            "componentCount": box["letters"],
            "labelConfidence": box["confidence"],
            "medianLetterHeightPx": round(box["medianLetterH"] * scale, 1),
        })

    candidates.sort(key=lambda c: (-c["labelConfidence"], -c["componentCount"],
                                   c["boxPx"]["y"], c["boxPx"]["x"]))
    for rank, candidate in enumerate(candidates):
        candidate["rank"] = rank
    return candidates, detected


# ---------------------------------------------------------------------------
# main
# ---------------------------------------------------------------------------

def main() -> int:
    artifact_in = load_georef()
    builder = plate_builder()

    out_plates = []
    for plate in artifact_in["plates"]:
        georef = plate["georef"]
        if georef.get("method") != "graticule_labels":
            print(f"{plate['key']}: no transform ({georef.get('method')}); "
                  "no label proposals")
            continue
        source_id = SOURCE_FOR_PLATE.get(plate["key"])
        if not source_id:
            print(f"{plate['key']}: no sources row registered in this builder",
                  file=sys.stderr)
            return 1
        src = STORAGE / plate["file"]
        if not src.exists():
            print(f"missing plate file: {src}", file=sys.stderr)
            return 1
        raw = src.read_bytes()
        digest = hashlib.sha256(raw).hexdigest()
        if digest != plate["fileSha256"]:
            print(f"{plate['key']}: stored file does not match the georeference "
                  "artifact checksum", file=sys.stderr)
            return 1

        gray, bmp = builder.gray_for(src, plate["key"])
        frame = georef["frame"]
        model = georef["model"]
        to_geo = projection(model)
        candidates, detected = build_candidates(gray, frame, model, to_geo)
        selected = candidates[:CANDIDATE_LIMIT]

        out_plates.append({
            "key": plate["key"],
            "sourceId": source_id,
            "file": plate["file"],
            "fileSha256": digest,
            "plateWidthPx": int(gray.shape[1]),
            "plateHeightPx": int(gray.shape[0]),
            "transform": {
                "model": model["kind"],
                "heldOutRmsKm": georef["heldOut"]["combinedKm"],
                "townRmsKm": georef["townCheck"]["rmsKm"],
                "needsReview": georef["needsReview"],
                "reviewReasons": georef["reviewReasons"],
            },
            "detectedTextLines": detected,
            "selected": len(selected),
            "truncated": detected > len(selected),
            "candidateLimit": CANDIDATE_LIMIT,
            "candidates": selected,
        })
        print(f"{plate['key']}: {detected} text-line proposals, "
              f"{len(selected)} selected (limit {CANDIDATE_LIMIT}), "
              f"transform {georef['heldOut']['combinedKm']} km")

    ARTIFACT.write_text(json.dumps({
        "meta": {
            "generator": "infra/local/build-plate-region-candidates.py",
            "generatedAt": date.today().isoformat(),
            "method": (
                "deterministic text-line proposal over a georeferenced plate: "
                "letter-sized connected components are grouped into single-baseline "
                "boxes inside the neatline, and each box centre is projected through "
                "the plan 030-D conic transform published in plate-georefs.json. "
                "Nothing is read: label_text_ar stays empty until a reviewer fills it."
            ),
            "notOCR": (
                "these are proposals, not readings and not tribal claims; the title "
                "cartouche, town names and tribal names are all proposed alike because "
                "the detector cannot know which is which"
            ),
            "knownLimits": [
                "a town name printed directly above a tribal name can share one box",
                "a strongly rotated or curved label can split into two proposals",
                "the title cartouche and scale bar are proposed like any other text",
            ],
            "downscale": DOWNSCALE,
            "inkThreshold": INK_THRESHOLD,
            "candidateLimit": CANDIDATE_LIMIT,
            "confidenceDefinition": (
                "0.45*min(1, letters/10) + 0.35*min(1, meanFill/0.55) + "
                "0.20*heightUniformity; it measures how much the box looks like a "
                "printed text line, never how likely it is a tribe"
            ),
            "labelCentreNote": (
                "a printed label is typeset beside the feature it names, so the box "
                "centre can sit up to half the label's width away from that feature "
                "(Baghdad's box centre, for instance, reads about 1 degree east of "
                "the city). The coordinates locate the proposal, not a surveyed point, "
                "and a reviewer's boundary trace must not be inferred from a label "
                "centre"
            ),
            "vertexCap": 160,
            "certaintyRule": (
                "a candidate is `suggested` and must never be rendered as a fact; "
                "acceptance is a named reviewer's decision recorded in the history "
                "table, and any geometry derived through the transform stays "
                "`approximate` (plan 030/032)"
            ),
        },
        "plates": out_plates,
    }, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(f"wrote {ARTIFACT.relative_to(ROOT)}")

    write_seed(out_plates)
    return 0


def write_seed(plates: list[dict]) -> None:
    lines = [
        "-- Plan 030-E: machine-proposed region-label crops on a georeferenced plate.",
        "-- Generated by infra/local/build-plate-region-candidates.py; do not edit.",
        f"-- Generated: {date.today().isoformat()}",
        "--",
        "-- Each row is a proposal: an empty label_text_ar, status 'suggested', and",
        "-- a crop box in plate pixels plus the projected centre. ON CONFLICT DO",
        "-- NOTHING, so re-applying the seed can never overwrite a reviewer's answer.",
        "",
    ]
    total = 0
    for plate in plates:
        lines.append(f"-- {plate['key']}: {plate['selected']} of "
                     f"{plate['detectedTextLines']} detected text lines")
        for candidate in plate["candidates"]:
            total += 1
            crop = json.dumps(candidate["cropPx"], separators=(",", ":"))
            assert "$json$" not in crop
            lines.append(
                "INSERT INTO external_region_candidates (source_id, plate_key, "
                "occurrence_id, crop_px, coordinates, ink_density, component_count, "
                "label_confidence, transform_rms_km, rank)")
            lines.append("VALUES (")
            lines.append(f"  '{plate['sourceId']}', {sql_str(plate['key'])}, "
                         f"{sql_str(candidate['occurrenceId'])},")
            lines.append(f"  $json${crop}$json$::jsonb,")
            lines.append("  ST_SetSRID(ST_MakePoint("
                         f"{candidate['coordinates']['lon']}, "
                         f"{candidate['coordinates']['lat']}), 4326),")
            lines.append(f"  {candidate['inkDensity']}, {candidate['componentCount']}, "
                         f"{candidate['labelConfidence']}, "
                         f"{plate['transform']['heldOutRmsKm']}, {candidate['rank']})")
            lines.append("ON CONFLICT (source_id, occurrence_id) DO NOTHING;")
        lines.append("")
    SEED.write_text("\n".join(lines) + "\n", encoding="utf-8")
    print(f"wrote {SEED.relative_to(ROOT)} ({total} candidate rows)")


def sql_str(value: str) -> str:
    return "'" + value.replace("'", "''") + "'"


if __name__ == "__main__":
    sys.exit(main())
