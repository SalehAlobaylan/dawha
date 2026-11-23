#!/usr/bin/env python3
"""Fit an atlas registration to measured landmarks and check held-out landmarks."""

import argparse
import hashlib
import json
import math
from pathlib import Path

import numpy as np


def distance_km(lonlat, actual):
    lon, lat = map(math.radians, lonlat)
    ref_lon, ref_lat = map(math.radians, actual)
    term = math.sin((ref_lat - lat) / 2)**2 + (
        math.cos(lat) * math.cos(ref_lat) * math.sin((ref_lon - lon) / 2)**2
    )
    return 12_742 * math.asin(math.sqrt(max(0, min(term, 1))))


def fit(metadata, scan):
    left, top, right, bottom = map(float, metadata["neatline"])
    if not 0 <= left < right <= 1_000_000 or not 0 <= top < bottom <= 1_000_000:
        raise ValueError("Invalid neatline")
    budget = float(metadata["maxErrorKm"])
    if not math.isfinite(budget) or budget <= 0:
        raise ValueError("Choose a positive held-out error budget in kilometres")
    fit_points, checks = metadata["fit"], metadata["check"]
    if len(fit_points) < 4 or len(checks) < 3:
        raise ValueError("Use at least four fit points and three other check points")
    all_points = fit_points + checks
    for point in all_points:
        if not point.get("label") or not left <= point["x"] <= right or not top <= point["y"] <= bottom:
            raise ValueError("Each point needs a named landmark inside the neatline")
        if not -180 <= point["lon"] <= 180 or not -85 <= point["lat"] <= 85:
            raise ValueError("The reference coordinate is outside the atlas extent")
    if len({(p["x"], p["y"]) for p in all_points}) != len(all_points):
        raise ValueError("Fit and check locations must be distinct")
    if len({(p["lon"], p["lat"]) for p in all_points}) != len(all_points):
        raise ValueError("Reference locations must be distinct")
    coverage = {}
    for name, points in (("fit", fit_points), ("check", checks)):
        x_span = (max(p["x"] for p in points) - min(p["x"] for p in points)) / (right - left)
        y_span = (max(p["y"] for p in points) - min(p["y"] for p in points)) / (bottom - top)
        if np.linalg.matrix_rank([[p["x"], p["y"], 1] for p in points]) < 3 or min(x_span, y_span) < 0.4:
            raise ValueError(f"Spread the {name} landmarks over both dimensions of the sheet")
        coverage[name] = {"xSpanFraction": x_span, "ySpanFraction": y_span}

    matrix = np.asarray([[p["x"], p["y"], 1] for p in fit_points])
    actual = np.asarray([[p["lon"], p["lat"]] for p in fit_points])
    coefficients, _, _, _ = np.linalg.lstsq(matrix, actual, rcond=None)
    check_matrix = np.asarray([[p["x"], p["y"], 1] for p in checks])
    check_coordinates = np.asarray([[p["lon"], p["lat"]] for p in checks])
    predicted = check_matrix @ coefficients
    corners = np.asarray([[x, y, 1] for x in (left, right) for y in (top, bottom)]) @ coefficients
    if np.linalg.det(coefficients[:2]) >= -1e-14 or np.any(np.abs(corners) > [180, 85]):
        raise ValueError("The fit is mirrored, singular or extends beyond the atlas")
    error = np.abs(predicted - check_coordinates)
    km = [distance_km(a, b) for a, b in zip(predicted, check_coordinates)]
    good = max(km) <= budget
    with scan.open("rb") as handle:
        digest = hashlib.file_digest(handle, "sha256").hexdigest()
    return {
        "file_sha256": digest,
        "georef": {
            "method": "control_points" if good else "none",
            "reason": "" if good else "خطأ التحقق أكبر من الحد المحدد لهذه اللوحة.",
            "model": {"kind": "affine", "affine": coefficients.T.flatten().tolist()},
            "frame": {
                "outer_left": left, "inner_left": left, "outer_right": right, "inner_right": right,
                "outer_top": top, "inner_top": top, "outer_bottom": bottom, "inner_bottom": bottom,
            },
            "fit": {
                "lonRmsDeg": float(np.sqrt(np.mean(error[:, 0]**2))),
                "latRmsDeg": float(np.sqrt(np.mean(error[:, 1]**2))),
                "lonMaxDeg": float(np.max(error[:, 0])),
                "latMaxDeg": float(np.max(error[:, 1])),
                "heldOutCount": len(checks), "maxErrorKm": max(km), "maxAllowedKm": budget,
                "rmsKm": math.sqrt(sum(value**2 for value in km) / len(km)),
                "coverage": coverage, "fitPoints": fit_points,
                "checkPoints": [dict(p, errorKm=e) for p, e in zip(checks, km)],
                "intendedScale": metadata["intendedScale"],
            },
            "certainty": "approximate",
            "needsReview": True,
            "reviewReasons": ["landmark_based_registration"],
        },
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("scan", type=Path)
    parser.add_argument("controls", type=Path, help="JSON with neatline, fit, check, intendedScale and maxErrorKm")
    args = parser.parse_args()
    try:
        result = fit(json.loads(args.controls.read_text()), args.scan)
    except (ValueError, KeyError, TypeError, OSError) as error:
        parser.error(str(error))
    print(json.dumps(result, ensure_ascii=False, indent=2, allow_nan=False))


if __name__ == "__main__":
    main()
