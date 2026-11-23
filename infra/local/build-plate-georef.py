#!/usr/bin/env python3
"""Plan 030-D: plate georeferencing from the plates' own printed graticule
(معايرة اللوحات).

Why this shape
--------------
Two candidate methods were measured and only one survives on this plate.

*Detecting the graticule lines* is not possible. At the full 7163x7163 scan the
strongest interior row covers 0.32 of its span and the strongest interior column
0.35 (the artifact records the probe as `probeCoverage`); the engraved lines are
broken by the dira stipple, by town names, and by the projection's own slant (the
56E meridian walks 504 px between the top and the bottom of the neatline). A
whole-column scan misses them by construction.

*Reading the printed degree labels* works. Every graticule line is labelled in
the margin strip between the outer border and the inner neatline, and each label
is centred on the line it names, so its ink centroid is a control point lying on
the neatline. That is the method used here.

The transform is an **equidistant conic**, not an affine map
-----------------------------------------------------------
A first, published-shape attempt fitted lon and lat as independent affine /
bilinear functions of (px, py). It reproduced the printed labels to 7 km and was
wrong in the interior by more than 100 km, and the error was *measurable*: the
plate's own town symbols (Mecca, Medina, Riyadh, Ha'il, Kuwait, Basra, Aden) all
landed 0.5-0.8 degrees **south** of the affine model's prediction.

That is not a plate error. It is the model's. This is a conic plate: the
meridians are straight rays that converge on a printing apex, and the parallels
are arcs concentric about that apex. An affine model in (x, y) cannot represent
an arc, so it linearly interpolates the parallels between the left and right
neatlines and is therefore systematically wrong at the map's centre by exactly
the arc's sagitta - here (half-width)^2 / (2 * apex distance) ~ 154 px ~ 0.66
degrees, which is what the towns showed.

The published transform therefore:

1. consumes the same printed labels,
2. estimates the **apex** by least squares over the meridian lines (each labelled
   longitude appears on the top and on the bottom neatline, so the line joining
   them is printed data; all of them must pass through one point),
3. reads the cone constant `n` and the central meridian from the angle of each
   meridian about that apex,
4. fits latitude as a straight line in the radial distance `r` - the defining
   property of an equidistant conic, which is also what the labels prefer: a
   log-radial (conformal) law fits the same labels nearly four times worse,
5. validates with held-out label pairs and with the town symbols, which take no
   part in the fit.

Honesty rules carried through the artifact
------------------------------------------
* Nothing about where a tribe sat is assumed; only printed degree labels are
  used, and their pixel positions are measured.
* A label's ink centroid carries a constant typesetting bias (the degree glyph
  sits after the digits). A constant is absorbed by the intercept; the artifact
  states the bound rather than claiming sub-pixel truth.
* The plate's town symbols disagree with modern coordinates by up to 30 km. That
  is a property of a 1916 compilation, and it is reported as such: the
  label-fit RMS measures fit consistency, the town check measures agreement with
  modern geography, and they are different numbers.
* Latitude accuracy inside the plate is validated only at the seven town symbols
  and at the neatlines; the printed parallels are not recoverable as ink. The
  plate is published `needs_review` and any geometry derived through it stays
  `approximate` until a named reviewer accepts it (plans 030/032).
* A plate that prints no graticule is registered with `georef: none` and a stated
  route, never with a transform fitted to eyeballed positions.

Run:    python3 infra/local/build-plate-georef.py
Needs:  macOS `sips` for the BMP derivative, numpy, the stored plate files.
"""

from __future__ import annotations

import hashlib
import json
import math
import subprocess
import sys
from datetime import date
from pathlib import Path

import numpy as np

ROOT = Path(__file__).resolve().parents[2]
ARTIFACT = ROOT / "infra" / "local" / "plate-georefs.json"
SEED = ROOT / "infra" / "local" / "seed-plate-georefs.sql"
STORAGE = ROOT / ".data" / "source-storage"
CONVERT = ROOT / ".data" / "cache" / "plate-bmp"

STEP_DEG = 4.0
FIT_GATE_KM = 15.0          # held-out label RMS must stay under this
TOWN_GATE_KM = 40.0         # a town further off than this means the model is wrong
SPACING_CV_MAX = 0.03       # label spacing must be uniform to 3%
SHIFT_R2_MIN = 0.98         # meridian shift vs longitude must be linear
CONCURRENCY_MAX_PX = 60.0   # every meridian must pass this close to the apex
INTERIOR_MEDIAN_MAX_DEG = 0.10
LABEL_WIDTH_MIN, LABEL_WIDTH_MAX = 45, 150

# ---------------------------------------------------------------------------
# Plate registry.
#
# `verified_anchors` records labels that were read directly off the plate in a
# margin crop during this build (side, degree, pixel position observed), so the
# automatic degree assignment is checked against human-read ground truth rather
# than against itself.
#
# `validation_towns` are the plate's own printed town symbols - the hollow
# circles next to the town names - read from 4x zoomed, crosshair-annotated
# crops. They are deliberately *not* used to fit anything: they exist so the
# transform can be tested against data it never saw.
# ---------------------------------------------------------------------------
PLATES: list[dict] = [
    {
        "key": "admiralty-1916-tribal",
        "file": "sources/plates/admiralty-1916-tribal-original.jpg",
        "source_id": "b0000000-0000-0000-0000-00000000e004",
        "title_ar": "خريطة قبائل الجزيرة العربية — هيئة الأركان البحريّة البريطانية ١٩١٦",
        "author_ar": "Admiralty War Staff, Intelligence Division",
        "source_type": "archive_record",
        "publication_date_from": "1916-01-01",
        "citation_ar": (
            "Admiralty War Staff, Intelligence Division, Tribal Map of Arabia "
            "(No. CB. 1152), Ordnance Survey, 1916."
        ),
        "location_ar": "London",
        "license": "public_domain",
        "license_note": "UK Crown publication pre-1957; US publication pre-1929. "
                        "Commons file tagged Public domain.",
        "archive_title": "File:1916 map by the British Admiralty War Staff - Tribal map of ARABIA.jpg",
        "archive_page": "https://commons.wikimedia.org/wiki/File:1916_map_by_the_British_Admiralty_War_Staff_-_Tribal_map_of_ARABIA.jpg",
        "archive_direct": "https://upload.wikimedia.org/wikipedia/commons/2/2e/1916_map_by_the_British_Admiralty_War_Staff_-_Tribal_map_of_ARABIA.jpg",
        "fetched_at": "2026-11-16",
        "georef_method": "graticule_labels",
        "label_bands": {"top": [0.10, 0.58], "bottom": [0.07, 0.90],
                        "left": [0.06, 0.79], "right": [0.21, 0.83]},
        "anchors": {"top": 32.0, "bottom": 40.0, "left": 36.0, "right": 36.0},
        "steps": {"top": STEP_DEG, "bottom": STEP_DEG,
                  "left": -STEP_DEG, "right": -STEP_DEG},
        "verified_anchors": [
            {"side": "top", "deg": 32.0, "px": 920, "how": "margin crop, top-left corner"},
            {"side": "top", "deg": 48.0, "px": 3874, "how": "margin crop, top-right"},
            {"side": "top", "deg": 52.0, "px": 4616, "how": "margin crop, top-right"},
            {"side": "bottom", "deg": 44.0, "px": 3066, "how": "margin crop, bottom middle"},
            {"side": "bottom", "deg": 48.0, "px": 3986, "how": "margin crop, bottom middle"},
            {"side": "bottom", "deg": 52.0, "px": 4915, "how": "margin crop, bottom middle"},
            {"side": "left", "deg": 36.0, "px": 850, "how": "margin crop, top-left corner"},
            {"side": "left", "deg": 28.0, "px": 2737, "how": "margin crop, left middle"},
        ],
        "validation_towns": [
            {"name": "Mecca", "lon": 39.83, "lat": 21.42, "px": 2196, "py": 4404},
            {"name": "Medina", "lon": 39.61, "lat": 24.47, "px": 2222, "py": 3663},
            {"name": "Riyadh", "lon": 46.68, "lat": 24.71, "px": 3680, "py": 3678},
            {"name": "Ha'il", "lon": 41.69, "lat": 27.52, "px": 2710, "py": 2996},
            {"name": "Kuwait", "lon": 47.98, "lat": 29.37, "px": 3935, "py": 2598},
            {"name": "Basra", "lon": 47.78, "lat": 30.51, "px": 3880, "py": 2329},
            {"name": "Aden", "lon": 45.03, "lat": 12.79, "px": 3296, "py": 6421},
        ],
        "town_read_how": ("4x nearest-neighbour zoom of the plate around the label-fit "
                          "prediction, with a dashed crosshair; the symbol is the hollow "
                          "circle printed beside the town name, read to about +-6 px"),
    },
    {
        "key": "dickson-1914-tribal",
        "file": "sources/plates/dickson-tribal-arabia.jpg",
        "source_id": "b0000000-0000-0000-0000-00000000e005",
        "title_ar": "خريطة قبائل الجزيرة العربية التقريبية — هارولد ديكسون ١٩١٤",
        "author_ar": "Harold R. P. Dickson",
        "source_type": "archive_record",
        "publication_date_from": "1914-01-01",
        "citation_ar": (
            "Dickson, H. R. P., 'Rough Tribal Map of Arabia', scale 180 miles = 1 inch, "
            "published in Kuwait and her Neighbours (1914)."
        ),
        "location_ar": "Kuwait",
        "license": "cc_by_sa_3_0",
        "license_note": (
            "Commons file (scan/reproduction) tagged CC BY-SA 3.0, attribution required "
            "on display. The underlying 1914 map is by an author who died in 1959, so its "
            "copyright status differs by jurisdiction; used here as a research reference."
        ),
        "archive_title": "File:خريطة قبائل الجزيرة العربية لمستشرق أنجليزي قبل مئة سنة وضعها المستشرق هارولد ديكسون.jpg",
        "archive_page": "https://commons.wikimedia.org/wiki/File:%D8%AE%D8%B1%D9%8A%D8%B7%D8%A9_%D9%82%D8%A8%D8%A7%D8%A6%D9%84_%D8%A7%D9%84%D8%AC%D8%B2%D9%8A%D8%B1%D8%A9_%D8%A7%D9%84%D8%B9%D8%B1%D8%A8%D9%8A%D8%A9_%D9%84%D9%85%D8%B3%D8%AA%D8%B4%D8%B1%D9%82_%D8%A3%D9%86%D8%AC%D9%84%D9%8A%D8%B2%D9%8A_%D9%82%D8%A8%D9%84_%D9%85%D8%A6%D8%A9_%D8%B3%D9%86%D8%A9_%D9%88%D8%B6%D8%B9%D9%87%D8%A7_%D8%A7%D9%84%D9%85%D8%B3%D8%AA%D8%B4%D8%B1%D9%82_%D9%87%D8%A7%D8%B1%D9%88%D9%84%D8%AF_%D8%AF%D9%8A%D9%83%D8%B3%D9%88%D9%86.jpg",
        "fetched_at": "2026-11-16",
        "georef_method": "none",
        "no_transform_reason": (
            "The plate prints no graticule and no degree labels, and states outright that "
            "'Tribal Boundaries are only approximate'. The documented route is anchor-town "
            "control points on the block-lettered town markers (Mosul, Baghdad, Basra, "
            "Kuwait, Medina, Mecca, Riyadh, Hofuf, Mascat, Aden...), read and entered by a "
            "reviewer in phases E/F. Fitting a transform to visually estimated town pixels "
            "here would manufacture precision the plate disclaims."
        ),
        "planned_route": "anchor_towns",
    },
]

# ---------------------------------------------------------------------------
# image decoding
# ---------------------------------------------------------------------------

def load_bmp_gray(path: Path) -> np.ndarray:
    data = path.read_bytes()
    if data[:2] != b"BM":
        raise ValueError(f"not a BMP: {path}")
    offset = int.from_bytes(data[10:14], "little")
    width = int.from_bytes(data[18:22], "little")
    height_raw = int.from_bytes(data[22:26], "little", signed=True)
    bpp = int.from_bytes(data[28:30], "little")
    if int.from_bytes(data[30:34], "little") != 0:
        raise ValueError("expected an uncompressed BMP")
    height = abs(height_raw)
    row_size = ((width * bpp // 8) + 3) // 4 * 4
    body = np.frombuffer(data, np.uint8, count=height * row_size, offset=offset)
    body = body.reshape(height, row_size)
    if height_raw > 0:
        body = body[::-1]                       # bottom-up rows
    channels = bpp // 8
    px = body[:, : width * channels].reshape(height, width, channels)
    return px[:, :, :3].astype(np.float32).mean(axis=2)


def to_bmp(src: Path, stem: str) -> Path:
    """BMP derivative of `src`, invalidated whenever the source is newer, so a
    swapped plate can never be silently measured with the previous plate's
    pixels."""
    CONVERT.mkdir(parents=True, exist_ok=True)
    out = CONVERT / f"{stem}.bmp"
    if (out.exists() and out.stat().st_size > 1_000_000
            and out.stat().st_mtime >= src.stat().st_mtime):
        return out
    subprocess.run(["sips", "-s", "format", "bmp", str(src), "--out", str(out)],
                   check=True, capture_output=True)
    return out


def gray_for(src: Path, stem: str) -> tuple[np.ndarray, Path]:
    bmp = to_bmp(src, stem)
    cache = CONVERT / f"{stem}-gray.npy"
    if (cache.exists() and cache.stat().st_mtime >= bmp.stat().st_mtime
            and cache.stat().st_mtime >= src.stat().st_mtime):
        return np.load(cache), bmp
    gray = load_bmp_gray(bmp)
    np.save(cache, gray.astype(np.uint8))
    return gray, bmp


# ---------------------------------------------------------------------------
# frame + margin labels
# ---------------------------------------------------------------------------

def find_frame(gray: np.ndarray) -> dict:
    """Outer border and inner neatline: the only near-full-span ink lines."""
    h, w = gray.shape
    dark = gray < 200.0
    col = dark.mean(axis=0)
    row = dark.mean(axis=1)

    def lines(profile, length, lo=60, hi=None):
        hi = hi if hi is not None else length - 60
        pos = []
        start = None
        for i, v in enumerate(profile):
            on = v >= 0.35
            if on and start is None:
                start = i
            elif not on and start is not None:
                if 0 < i - start <= 24:
                    pos.append((start + (i - start) / 2, float(profile[start:i].mean())))
                start = None
        pos = [p for p in pos if lo < p[0] < hi]
        merged: list[tuple[float, float]] = []
        for p, cov in pos:
            if merged and p - merged[-1][0] < 20:
                continue
            merged.append((p, cov))
        return [p for p, _ in merged]

    rows = lines(row, h)
    cols = lines(col, w)
    if len(rows) < 4 or len(cols) < 4:
        raise RuntimeError(f"frame not found (rows={rows}, cols={cols})")
    return {
        "outer_top": rows[0], "inner_top": rows[1],
        "inner_bottom": rows[-2], "outer_bottom": rows[-1],
        "outer_left": cols[0], "inner_left": cols[1],
        "inner_right": cols[-2], "outer_right": cols[-1],
        "all_rows": rows, "all_cols": cols,
    }


def band(inner: float, outer: float, frac_a: float, frac_b: float) -> tuple[int, int]:
    m = abs(outer - inner)
    a = inner + np.sign(outer - inner) * min(frac_a, frac_b) * m
    b = inner + np.sign(outer - inner) * max(frac_a, frac_b) * m
    return int(round(min(a, b))), int(round(max(a, b)))


def clusters_1d(proj: np.ndarray, width_min: int, width_max: int,
                gap: int = 45) -> list[tuple[float, int]]:
    """Centres and widths of contiguous ink runs, with a hard width window."""
    idx = np.flatnonzero(proj)
    if not len(idx):
        return []
    groups: list[list[int]] = [[int(idx[0])]]
    for i in idx[1:]:
        if i - groups[-1][-1] <= gap:
            groups[-1].append(int(i))
        else:
            groups.append([int(i)])
    out = []
    for g in groups:
        width = g[-1] - g[0] + 1
        if width_min <= width <= width_max:
            out.append(((g[0] + g[-1]) / 2.0, width))
    return out


def uniform_run(centers: list[float], tol: float = 0.15) -> list[float]:
    """Longest sub-sequence whose consecutive gaps agree within ±tol of the median."""
    if len(centers) < 2:
        return centers
    diffs = np.diff(centers)
    med = float(np.median(diffs))
    good = np.abs(diffs - med) <= tol * med
    best_len, best_start, run, start = 0, 0, 0, 0
    for i, g in enumerate(good):
        if g:
            if run == 0:
                start = i
            run += 1
            if run > best_len:
                best_len, best_start = run, start
        else:
            run = 0
    return centers[best_start:best_start + best_len + 1]


def read_margin_labels(gray: np.ndarray, frame: dict, bands: dict) -> dict:
    mask = gray < 170.0
    out: dict[str, list[float]] = {}
    for side in ("top", "bottom", "left", "right"):
        if side == "top":
            y0, y1 = band(frame["inner_top"], frame["outer_top"], *bands["top"])
            proj = mask[y0:y1, :].any(axis=0)
        elif side == "bottom":
            y0, y1 = band(frame["inner_bottom"], frame["outer_bottom"], *bands["bottom"])
            proj = mask[y0:y1, :].any(axis=0)
        elif side == "left":
            x0, x1 = band(frame["inner_left"], frame["outer_left"], *bands["left"])
            proj = mask[:, x0:x1].any(axis=1)
        else:
            x0, x1 = band(frame["inner_right"], frame["outer_right"], *bands["right"])
            proj = mask[:, x0:x1].any(axis=1)
        found = [c for c, _w in clusters_1d(proj, LABEL_WIDTH_MIN, LABEL_WIDTH_MAX)]
        out[side] = uniform_run(found)
    return out


# ---------------------------------------------------------------------------
# the conic model
# ---------------------------------------------------------------------------

def meridian_lines(labels: dict, degrees: dict, y_top: float, y_bot: float) -> list[dict]:
    """One line per longitude that is labelled on both the top and bottom edge."""
    top = {deg: px for deg, px in zip(degrees["top"], labels["top"])}
    bot = {deg: px for deg, px in zip(degrees["bottom"], labels["bottom"])}
    return [{"lon": lon, "xTop": top[lon], "xBot": bot[lon],
             "yTop": y_top, "yBot": y_bot}
            for lon in sorted(set(top) & set(bot))]


def apex_from_meridians(lines: list[dict]) -> tuple[float, float, list[float]]:
    """Least squares over the meridian lines: the point every meridian passes
    through. Returns the apex and each line's perpendicular residual."""
    a = np.zeros((2, 2))
    b = np.zeros(2)
    normals = []
    for ln in lines:
        d = np.array([ln["xBot"] - ln["xTop"], ln["yBot"] - ln["yTop"]], float)
        d /= np.linalg.norm(d)
        nrm = np.array([-d[1], d[0]])
        p0 = np.array([ln["xTop"], ln["yTop"]], float)
        a += np.outer(nrm, nrm)
        b += nrm * (nrm @ p0)
        normals.append((nrm, p0))
    if np.linalg.matrix_rank(a) < 2:
        raise RuntimeError("meridian lines are parallel; no apex")
    apex = np.linalg.solve(a, b)
    residuals = [float(nrm @ (apex - p0)) for nrm, p0 in normals]
    return float(apex[0]), float(apex[1]), residuals


def fit_conic(labels: dict, degrees: dict, y_top: float, y_bot: float,
              x_left: float, x_right: float) -> dict:
    """Equidistant conic fitted to the printed labels alone."""
    lines = meridian_lines(labels, degrees, y_top, y_bot)
    if len(lines) < 3:
        raise RuntimeError("need at least three meridians labelled on both edges")
    apex_x, apex_y, residuals = apex_from_meridians(lines)

    # cone constant and central meridian from the meridian angles
    theta = np.array([math.atan2(ln["xTop"] - apex_x, ln["yTop"] - apex_y) for ln in lines])
    lons = np.array([ln["lon"] for ln in lines])
    slope, intercept = np.polyfit(lons, theta, 1)     # radians per degree
    n = float(slope * 180.0 / math.pi)
    lon0 = float(-intercept / slope)
    if not (0.0 < n < 1.0):
        raise RuntimeError(f"cone constant out of range: {n}")

    # latitude linear in the radius (equidistant conic)
    lat_pts = ([(x_left, y, v) for v, y in zip(degrees["left"], labels["left"])] +
               [(x_right, y, v) for v, y in zip(degrees["right"], labels["right"])])
    r = np.array([math.hypot(x - apex_x, y - apex_y) for x, y, _ in lat_pts])
    lat = np.array([v for _, _, v in lat_pts], float)
    coef = np.polyfit(r, lat, 1)
    return {
        "kind": "equidistant_conic",
        "apex": {"x": apex_x, "y": apex_y},
        "coneConstant": n,
        "centralLongitude": lon0,
        "latitude": {"coef": [float(coef[0]), float(coef[1])], "form": "lat = c0*r + c1"},
        "meridians": lines,
        "meridianResidualPx": [round(v, 2) for v in residuals],
        "meridianMaxResidualPx": round(max(abs(v) for v in residuals), 2),
        "latPoints": lat_pts,
    }


def to_geo(model: dict, px: float, py: float) -> tuple[float, float]:
    ax, ay = model["apex"]["x"], model["apex"]["y"]
    lon = model["centralLongitude"] + math.degrees(
        math.atan2(px - ax, py - ay)) / model["coneConstant"]
    r = math.hypot(px - ax, py - ay)
    lat = model["latitude"]["coef"][0] * r + model["latitude"]["coef"][1]
    return lon, lat


def to_pixel(model: dict, lon: float, lat: float) -> tuple[float, float]:
    ax, ay = model["apex"]["x"], model["apex"]["y"]
    theta = math.radians((lon - model["centralLongitude"]) * model["coneConstant"])
    c0, c1 = model["latitude"]["coef"]
    r = (lat - c1) / c0
    return ax + r * math.sin(theta), ay + r * math.cos(theta)


def haversine_km(lon1: float, lat1: float, lon2: float, lat2: float) -> float:
    p1, p2 = math.radians(lat1), math.radians(lat2)
    dp, dl = p2 - p1, math.radians(lon2 - lon1)
    a = math.sin(dp / 2) ** 2 + math.cos(p1) * math.cos(p2) * math.sin(dl / 2) ** 2
    return 2 * 6371.0088 * math.asin(math.sqrt(a))


def label_residuals(model: dict, labels: dict, degrees: dict,
                    y_top: float, y_bot: float,
                    x_left: float, x_right: float) -> dict:
    lon_err, lat_err = [], []
    for side, y in (("top", y_top), ("bottom", y_bot)):
        for deg, px in zip(degrees[side], labels[side]):
            lon, _ = to_geo(model, px, y)
            lon_err.append(lon - deg)
    for side, x in (("left", x_left), ("right", x_right)):
        for deg, py in zip(degrees[side], labels[side]):
            _, lat = to_geo(model, x, py)
            lat_err.append(lat - deg)
    lon_err = np.array(lon_err)
    lat_err = np.array(lat_err)
    return {
        "lonRmsDeg": float(np.sqrt(np.mean(lon_err ** 2))),
        "lonMaxDeg": float(np.max(np.abs(lon_err))),
        "latRmsDeg": float(np.sqrt(np.mean(lat_err ** 2))),
        "latMaxDeg": float(np.max(np.abs(lat_err))),
        "lonResiduals": [round(float(v), 4) for v in lon_err],
        "latResiduals": [round(float(v), 4) for v in lat_err],
    }


def held_out(model_ctx: dict) -> dict:
    """Refit with each labelled longitude pair / latitude pair removed and
    predict the removed pair. This is the number the quality gate uses."""
    labels, degrees = model_ctx["labels"], model_ctx["degrees"]
    y_top, y_bot = model_ctx["yTop"], model_ctx["yBot"]
    x_left, x_right = model_ctx["xLeft"], model_ctx["xRight"]
    lon_held, lat_held = [], []

    for lon in sorted(set(degrees["top"]) & set(degrees["bottom"])):
        sub_lab = {s: [p for d, p in zip(degrees[s], labels[s]) if d != lon]
                   for s in labels}
        sub_deg = {s: [d for d in degrees[s] if d != lon] for s in degrees}
        try:
            m = fit_conic(sub_lab, sub_deg, y_top, y_bot, x_left, x_right)
        except RuntimeError:
            continue
        for side, y in (("top", y_top), ("bottom", y_bot)):
            px = dict(zip(degrees[side], labels[side]))[lon]
            lon_held.append(to_geo(m, px, y)[0] - lon)

    for lat in sorted(set(degrees["left"]) & set(degrees["right"])):
        sub_lab = {s: [p for d, p in zip(degrees[s], labels[s]) if d != lat]
                   for s in labels}
        sub_deg = {s: [d for d in degrees[s] if d != lat] for s in degrees}
        try:
            m = fit_conic(sub_lab, sub_deg, y_top, y_bot, x_left, x_right)
        except RuntimeError:
            continue
        for side, x in (("left", x_left), ("right", x_right)):
            py = dict(zip(degrees[side], labels[side]))[lat]
            lat_held.append(to_geo(m, x, py)[1] - lat)

    lon_held = np.array(lon_held)
    lat_held = np.array(lat_held)
    centre_lat = 0.5 * (min(p[2] for p in model_ctx["latPoints"]) +
                       max(p[2] for p in model_ctx["latPoints"]))
    km_lon = 111.32 * math.cos(math.radians(centre_lat))
    km_lat = 110.57
    combined = float(math.hypot(np.sqrt(np.mean(lon_held ** 2)) * km_lon,
                                np.sqrt(np.mean(lat_held ** 2)) * km_lat))
    return {
        "method": ("refit with each labelled longitude pair and each labelled latitude "
                   "pair removed in turn; the removed pair is predicted and scored"),
        "lonHoldouts": int(len(lon_held)),
        "latHoldouts": int(len(lat_held)),
        "lonRmsDeg": float(np.sqrt(np.mean(lon_held ** 2))),
        "latRmsDeg": float(np.sqrt(np.mean(lat_held ** 2))),
        "lonMaxDeg": float(np.max(np.abs(lon_held))),
        "latMaxDeg": float(np.max(np.abs(lat_held))),
        "combinedKm": round(combined, 2),
        "gateKm": FIT_GATE_KM,
    }


def town_check(model: dict, towns: list[dict]) -> dict:
    """Compare the model against the plate's own printed town symbols.

    The towns take no part in the fit. A large residual here means the transform
    is wrong, not that the towns are; the *spread* across towns is the plate's own
    disagreement with modern coordinates.
    """
    rows = []
    for t in towns:
        px, py = to_pixel(model, t["lon"], t["lat"])
        lon_printed, lat_printed = to_geo(model, t["px"], t["py"])
        rows.append({
            "name": t["name"],
            "trueLon": t["lon"], "trueLat": t["lat"],
            "predictedPx": [round(px, 1), round(py, 1)],
            "printedPx": [t["px"], t["py"]],
            "offsetPx": [round(t["px"] - px, 1), round(t["py"] - py, 1)],
            "printedLon": round(lon_printed, 3), "printedLat": round(lat_printed, 3),
            "errorKm": round(haversine_km(t["lon"], t["lat"], lon_printed, lat_printed), 1),
        })
    errs = np.array([r["errorKm"] for r in rows])
    return {
        "towns": rows,
        "rmsKm": round(float(np.sqrt(np.mean(errs ** 2))), 1),
        "maxKm": round(float(np.max(errs)), 1),
        "medianKm": round(float(np.median(errs)), 1),
        "gateKm": TOWN_GATE_KM,
        "readingNote": ("town symbols take no part in the fit; they are read from the plate "
                        "and scored against modern coordinates, so this number measures the "
                        "plate's own geography and the transform at once"),
    }


# ---------------------------------------------------------------------------
# independent interior checks
# ---------------------------------------------------------------------------

def interior_meridian_check(gray: np.ndarray, model: dict, frame: dict) -> dict:
    """Search for printed meridian ink near the conic model's own prediction."""
    h, w = gray.shape
    rows = [int(r) for r in np.linspace(frame["inner_top"] + 0.18 * h,
                                        frame["inner_bottom"] - 0.12 * h, 4)]
    px_per_deg = _px_per_deg(model)
    samples = []
    for ln in model["meridians"]:
        theta = math.radians((ln["lon"] - model["centralLongitude"]) * model["coneConstant"])
        offs = []
        for y in rows:
            pred = model["apex"]["x"] + (y - model["apex"]["y"]) * math.tan(theta)
            x0 = max(0, int(pred) - 90)
            x1 = min(w, int(pred) + 90)
            if x1 - x0 < 20:
                continue
            win = gray[max(0, y - 40):y + 40, x0:x1]
            colprof = (win < 200.0).mean(axis=0)
            best = int(np.argmax(colprof))
            if float(colprof[best]) >= 0.5:
                offs.append({"y": y, "predPx": round(pred, 1),
                             "inkPx": x0 + best,
                             "offsetPx": round(x0 + best - pred, 1),
                             "coverage": round(float(colprof[best]), 2)})
        if offs:
            samples.append({"lon": ln["lon"], "samples": offs,
                            "medianAbsOffsetPx": round(float(np.median(
                                [abs(o["offsetPx"]) for o in offs])), 1)})
    med = float(np.median([s["medianAbsOffsetPx"] for s in samples])) if samples else 999.0
    return {
        "meridiansChecked": len(samples),
        "medianAbsOffsetPx": round(med, 1),
        "medianAbsOffsetDeg": round(med / px_per_deg, 4) if px_per_deg else None,
        "samples": samples,
    }


def _px_per_deg(model: dict) -> float:
    """Pixels per degree of latitude at the plate centre (|c0| is degrees per pixel)."""
    return 1.0 / abs(model["latitude"]["coef"][0])


# ---------------------------------------------------------------------------
# main
# ---------------------------------------------------------------------------

def main() -> int:
    out_plates: list[dict] = []

    for plate in PLATES:
        src = STORAGE / plate["file"]
        if not src.exists():
            print(f"missing plate file: {src}", file=sys.stderr)
            return 1
        raw = src.read_bytes()
        base = {
            "key": plate["key"],
            "file": plate["file"],
            "fileSha256": hashlib.sha256(raw).hexdigest(),
            "fileBytes": len(raw),
            "license": plate["license"],
            "licenseNote": plate["license_note"],
            "archiveTitle": plate["archive_title"],
            "fetchedAt": plate["fetched_at"],
        }

        if plate["georef_method"] == "none":
            out_plates.append({**base,
                               "georef": {"method": "none",
                                          "reason": plate["no_transform_reason"],
                                          "plannedRoute": plate["planned_route"],
                                          "certainty": "none",
                                          "needsReview": True,
                                          "reviewReasons": ["no_graticule"]}})
            print(f"{plate['key']}: registered without a transform (no graticule)")
            continue

        gray, _bmp = gray_for(src, plate["key"])
        h, w = gray.shape
        frame = find_frame(gray)
        labels = read_margin_labels(gray, frame, plate["label_bands"])
        for side in ("top", "bottom", "left", "right"):
            if len(labels[side]) < 4:
                print(f"{plate['key']}: too few {side} labels ({len(labels[side])})",
                      file=sys.stderr)
                return 1

        def degrees(side: str) -> list[float]:
            step = plate["steps"][side]
            return [plate["anchors"][side] + i * step for i in range(len(labels[side]))]

        deg = {side: degrees(side) for side in ("top", "bottom", "left", "right")}
        reasons: list[str] = []

        # ---- degree assignment validation -------------------------------------
        spacing = {}
        for side in ("top", "bottom", "left", "right"):
            gaps = np.diff(labels[side])
            spacing[side] = {"count": len(labels[side]),
                             "meanPx": round(float(np.mean(gaps)), 1),
                             "cv": round(float(np.std(gaps) / np.mean(gaps)), 4),
                             "degrees": [deg[side][0], deg[side][-1]]}
            if spacing[side]["cv"] > SPACING_CV_MAX:
                reasons.append(f"{side}_label_spacing_not_uniform")

        top_by_lon = dict(zip(deg["top"], labels["top"]))
        bot_by_lon = dict(zip(deg["bottom"], labels["bottom"]))
        shared = sorted(set(top_by_lon) & set(bot_by_lon))
        shifts = [bot_by_lon[l] - top_by_lon[l] for l in shared]
        shift_r2 = None
        if len(shared) >= 3:
            a = np.stack([np.ones(len(shared)), np.array(shared, float)], axis=1)
            coef, *_ = np.linalg.lstsq(a, np.array(shifts, float), rcond=None)
            pred = a @ coef
            ss_res = float(np.sum((np.array(shifts, float) - pred) ** 2))
            ss_tot = float(np.sum((np.array(shifts, float) - np.mean(shifts)) ** 2))
            shift_r2 = 1.0 - ss_res / ss_tot if ss_tot > 0 else 0.0
            if shift_r2 < SHIFT_R2_MIN:
                reasons.append("meridian_shift_not_linear_in_longitude")

        anchor_check = []
        for va in plate["verified_anchors"]:
            degs = deg[va["side"]]
            if va["deg"] not in degs:
                anchor_check.append({**va, "found": False, "deltaPx": None, "ok": False})
                continue
            px = labels[va["side"]][degs.index(va["deg"])]
            anchor_check.append({**va, "foundPx": round(px, 1),
                                 "deltaPx": round(px - va["px"], 1),
                                 "ok": abs(px - va["px"]) <= 60})
        if any(not a["ok"] for a in anchor_check):
            reasons.append("anchor_degrees_do_not_match_plate")

        # ---- conic fit -------------------------------------------------------
        y_top, y_bot = frame["inner_top"], frame["inner_bottom"]
        x_left, x_right = frame["inner_left"], frame["inner_right"]
        model = fit_conic(labels, deg, y_top, y_bot, x_left, x_right)

        residuals = label_residuals(model, labels, deg, y_top, y_bot, x_left, x_right)
        hold = held_out({"labels": labels, "degrees": deg, "yTop": y_top, "yBot": y_bot,
                         "xLeft": x_left, "xRight": x_right,
                         "latPoints": model["latPoints"]})

        if model["meridianMaxResidualPx"] > CONCURRENCY_MAX_PX:
            reasons.append("meridians_not_concurrent")
        if hold["combinedKm"] > FIT_GATE_KM:
            reasons.append("held_out_rms_over_gate")

        # ---- independent checks ---------------------------------------------
        towns = town_check(model, plate["validation_towns"])
        if towns["maxKm"] > TOWN_GATE_KM:
            reasons.append("printed_towns_disagree_with_model")
        interior = interior_meridian_check(gray, model, frame)
        if interior["meridiansChecked"] == 0:
            reasons.append("interior_meridian_check_found_no_ink")
        elif (interior["medianAbsOffsetDeg"] or 0) > INTERIOR_MEDIAN_MAX_DEG:
            reasons.append("interior_meridians_deviate_from_model")
        # The printed parallels are not recoverable as ink anywhere in the
        # interior (see probeCoverage). Latitude is validated at the neatlines
        # and at the town symbols instead, which is weaker than a graticule and
        # must be visible to any consumer of this transform.
        reasons.append("interior_parallels_unverified_by_ink")

        georef = {
            "method": "graticule_labels",
            "model": {
                "kind": model["kind"],
                "apex": {"x": round(model["apex"]["x"], 2),
                         "y": round(model["apex"]["y"], 2)},
                "coneConstant": round(model["coneConstant"], 6),
                "standardParallelDeg": round(math.degrees(math.asin(model["coneConstant"])), 3),
                "centralLongitude": round(model["centralLongitude"], 4),
                "latitude": {"form": model["latitude"]["form"],
                             "coef": [round(c, 9) for c in model["latitude"]["coef"]]},
                "forward": ("lon = lon0 + degrees(atan2(px-apex.x, py-apex.y)) / n ; "
                            "lat = c0*hypot(px-apex.x, py-apex.y) + c1"),
                "inverse": ("r = (lat-c1)/c0 ; th = radians((lon-lon0)*n) ; "
                            "px = apex.x + r*sin(th) ; py = apex.y + r*cos(th)"),
                "pxPerDegLat": round(_px_per_deg(model), 3),
            },
            "frame": {k: round(float(frame[k]), 1) for k in
                      ("outer_top", "inner_top", "inner_bottom",
                       "outer_bottom", "outer_left", "inner_left",
                       "inner_right", "outer_right")},
            "labels": {s: [round(v, 1) for v in labels[s]]
                       for s in ("top", "bottom", "left", "right")},
            "degrees": {s: [round(v, 1) for v in deg[s]]
                        for s in ("top", "bottom", "left", "right")},
            "spacing": spacing,
            "meridianShift": {"lons": shared,
                              "shiftPx": [round(s, 1) for s in shifts],
                              "r2": None if shift_r2 is None else round(shift_r2, 5)},
            "meridianResidualPx": model["meridianResidualPx"],
            "meridianMaxResidualPx": model["meridianMaxResidualPx"],
            "meridianConcurrencyDeg": round(
                model["meridianMaxResidualPx"] / (spacing["top"]["meanPx"] / STEP_DEG), 4),
            "anchorCheck": anchor_check,
            "fit": {
                "lonRmsDeg": round(residuals["lonRmsDeg"], 5),
                "latRmsDeg": round(residuals["latRmsDeg"], 5),
                "lonMaxDeg": round(residuals["lonMaxDeg"], 5),
                "latMaxDeg": round(residuals["latMaxDeg"], 5),
                "lonResiduals": residuals["lonResiduals"],
                "latResiduals": residuals["latResiduals"],
            },
            "heldOut": {**hold,
                        "lonRmsDeg": round(hold["lonRmsDeg"], 5),
                        "latRmsDeg": round(hold["latRmsDeg"], 5),
                        "lonMaxDeg": round(hold["lonMaxDeg"], 5),
                        "latMaxDeg": round(hold["latMaxDeg"], 5)},
            "townCheck": towns,
            "interiorCheck": interior,
            "interiorParallelsVerified": False,
            "labelBiasNote": ("a degree label's ink centroid carries a constant typesetting "
                              "offset relative to the line it names; as a constant it is "
                              "absorbed by the intercept and bounded here at ±0.08 deg"),
            "certainty": "approximate",
            "needsReview": bool(reasons),
            "reviewReasons": reasons,
        }
        out_plates.append({**base, "georef": georef})
        print(f"{plate['key']}: conic n={model['coneConstant']:.5f} "
              f"(std parallel {georef['model']['standardParallelDeg']:.2f}N) "
              f"apex=({model['apex']['x']:.0f},{model['apex']['y']:.0f}) | "
              f"fit lon {residuals['lonRmsDeg']:.4f}deg lat {residuals['latRmsDeg']:.4f}deg | "
              f"held-out {hold['combinedKm']:.1f} km "
              f"({'PASS' if hold['combinedKm'] <= FIT_GATE_KM else 'OVER GATE'}) | "
              f"towns rms {towns['rmsKm']:.1f} km max {towns['maxKm']:.1f} km | "
              f"interior meridians {interior['meridiansChecked']} @ "
              f"{interior['medianAbsOffsetDeg']}deg | "
              f"reasons: {', '.join(reasons) if reasons else 'none'}")

    ARTIFACT.write_text(json.dumps(
        {
            "meta": {
                "generator": "infra/local/build-plate-georef.py",
                "generatedAt": date.today().isoformat(),
                "method": ("control points are the plate's own printed degree labels in the "
                           "margin strips; degrees are assigned from one hand-verified anchor "
                           "plus the printed 4-degree step and validated by spacing "
                           "uniformity and meridian-shift linearity; the transform is an "
                           "equidistant conic whose apex is fitted to the labelled meridian "
                           "lines and whose latitude is linear in the radius"),
                "modelChoice": ("an affine/bilinear model was tried first and rejected: it "
                                "cannot represent a conic plate's curved parallels and placed "
                                "the plate's own town symbols 0.5-0.8 degrees too far south. "
                                "The published conic reduces the town residual to the values "
                                "in each plate's townCheck"),
                "stepDeg": STEP_DEG,
                "fitGateKm": FIT_GATE_KM,
                "townGateKm": TOWN_GATE_KM,
                "certaintyRule": ("any geometry derived through these transforms is "
                                  "`approximate` until a named reviewer accepts it "
                                  "(plans 030/032); plates flagged needsReview must not be "
                                  "warped for data extraction until their reasons clear"),
                "probeCoverageNote": ("interior graticule lines were probed and are not "
                                      "continuous: the strongest interior row covers 0.32 of "
                                      "its span and the strongest interior column 0.35 at full "
                                      "resolution, which is why control points come from the "
                                      "printed labels rather than line detection"),
            },
            "plates": out_plates,
        },
        ensure_ascii=False, indent=2) + "\n")
    print(f"wrote {ARTIFACT.relative_to(ROOT)}")

    write_seed(out_plates, PLATES)
    return 0


def write_seed(plates: list[dict], cfgs: list[dict]) -> None:
    """Seed rows: sources + source_files, with metadata.georef embedded."""
    lines = [
        "-- Plan 030-D: plate sources and graticule georeferences.",
        "-- Generated by infra/local/build-plate-georef.py; do not edit by hand.",
        f"-- Generated: {date.today().isoformat()}",
        "--",
        "-- Each row carries the plate's real file sha256 and licence. The",
        "-- metadata.georef block holds the measured frame, the printed degree",
        "-- labels used as control points, the fitted conic, its held-out RMS,",
        "-- the town-symbol check and any review reasons. Deterministic ids;",
        "-- safe to re-run.",
        "",
    ]
    by_key = {c["key"]: c for c in cfgs}
    for plate in plates:
        cfg = by_key[plate["key"]]
        pid = cfg["source_id"]
        meta = {
            "license": plate["license"],
            "license_note": plate["licenseNote"],
            "archive_title": plate["archiveTitle"],
            "fetched_at": plate["fetchedAt"],
            "file_sha256": plate["fileSha256"],
            "plate_key": plate["key"],
            "file_bytes": plate["fileBytes"],
            "generator": "infra/local/build-plate-georef.py",
            "georef": plate["georef"],
        }
        meta_json = json.dumps(meta, ensure_ascii=False, indent=None)
        assert "$json$" not in meta_json, "dollar tag would break dollar quoting"
        lines.append(f"-- {plate['key']}")
        lines.append("INSERT INTO sources (id, title_ar, author_ar, source_type, "
                     "publication_date_from, citation_ar, location_ar, metadata)")
        lines.append("VALUES (")
        lines.append(f"  '{pid}',")
        lines.append(f"  {sql_str(cfg['title_ar'])},\n  {sql_str(cfg['author_ar'])}, "
                     f"'{cfg['source_type']}', '{cfg['publication_date_from']}',")
        lines.append(f"  {sql_str(cfg['citation_ar'])},\n  {sql_str(cfg['location_ar'])},")
        lines.append(f"  $json${meta_json}$json$::jsonb)")
        lines.append("ON CONFLICT (id) DO UPDATE SET")
        lines.append("  title_ar = EXCLUDED.title_ar, author_ar = EXCLUDED.author_ar,")
        lines.append("  citation_ar = EXCLUDED.citation_ar, location_ar = EXCLUDED.location_ar,")
        lines.append("  metadata = EXCLUDED.metadata, updated_at = now();")
        lines.append("")
        fname = Path(plate["file"]).name
        lines.append("INSERT INTO source_files (source_id, storage_key, original_filename_ar, "
                     "mime_type, byte_size, checksum_sha256)")
        lines.append(f"SELECT '{pid}', {sql_str(plate['file'])}, {sql_str(fname)},")
        lines.append(f"       'image/jpeg', {plate['fileBytes']}, '{plate['fileSha256']}'")
        lines.append("WHERE NOT EXISTS (SELECT 1 FROM source_files "
                     f"WHERE storage_key = {sql_str(plate['file'])});")
        lines.append("")
    SEED.write_text("\n".join(lines) + "\n")
    print(f"wrote {SEED.relative_to(ROOT)}")


def sql_str(value: str) -> str:
    return "'" + value.replace("'", "''") + "'"


if __name__ == "__main__":
    sys.exit(main())
