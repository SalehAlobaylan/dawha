#!/usr/bin/env python3
"""Score document-level Arabic OCR and chart extraction against reviewed gold.

Chart name/relationship metrics require blinded occurrence alignment. Strings
never establish identity. A chart name counts only when its occurrence mapping,
raw Arabic transcription, and page-relative location are all correct.
"""

from __future__ import annotations

import argparse
import json
import math
import random
import sys
import unicodedata
from collections import Counter, defaultdict
from pathlib import Path
from typing import Any, Iterable

IOU_THRESHOLD = 0.5
CONNECTOR_ENDPOINT_TOLERANCE = 0.025
CONNECTOR_PATH_TOLERANCE = 0.025
PRECISION_TARGET = 0.99
RECALL_TARGET = 0.95
MIN_HELD_OUT_DOCUMENTS = 3
METRIC_KEYS = (
    "character_error_rate_raw",
    "character_error_rate_search_normalized",
    "exact_name_precision",
    "exact_name_recall",
    "search_normalized_name_precision",
    "search_normalized_name_recall",
    "name_detection_precision",
    "name_detection_recall",
    "unreadable_label_detection_recall",
    "partial_label_detection_recall",
    "branch_precision",
    "branch_recall",
    "connector_precision",
    "connector_recall",
    "relationship_structure_precision",
    "relationship_structure_recall",
    "relationship_precision",
    "relationship_recall",
    "assertion_precision",
    "assertion_recall",
    "page_coverage",
    "tile_coverage",
)


def read_jsonl(path: Path) -> dict[str, dict[str, Any]]:
    records: dict[str, dict[str, Any]] = {}
    with path.open(encoding="utf-8") as stream:
        for line_number, line in enumerate(stream, start=1):
            if not line.strip():
                continue
            try:
                value = json.loads(line)
            except json.JSONDecodeError as error:
                raise ValueError(f"{path}:{line_number}: invalid JSON: {error}") from error
            if not isinstance(value, dict) or not isinstance(value.get("document_id"), str):
                raise ValueError(f"{path}:{line_number}: each row needs a string document_id")
            document_id = value["document_id"]
            if document_id in records:
                raise ValueError(f"{path}:{line_number}: duplicate document_id {document_id!r}")
            if value.get("source_class") not in {"chart", "book", "mixed"}:
                raise ValueError(f"{path}:{line_number}: source_class must be chart, book, or mixed")
            for key in ("pages", "nodes", "branches", "connectors", "tiles", "mentions", "edges", "assertions"):
                if key in value and not isinstance(value[key], list):
                    raise ValueError(f"{path}:{line_number}: {key} must be an array")
            if value.get("split", "unspecified") not in {"development", "held_out", "unspecified"}:
                raise ValueError(f"{path}:{line_number}: split must be development or held_out")
            records[document_id] = value
    return records


def arabic_search_normalize(text: str) -> str:
    """Search-only diagnostic: remove marks/tatweel and fold alef/ya forms."""
    output: list[str] = []
    for char in unicodedata.normalize("NFC", text):
        if char == "ـ" or unicodedata.category(char) in {"Mn", "Me"}:
            continue
        if char in "أإآٱ":
            char = "ا"
        elif char == "ى":
            char = "ي"
        output.append(char)
    return "".join(output)


def exact_name_normalize(text: str) -> str:
    """Canonical Unicode composition only; preserve spaces and Arabic spelling."""
    return unicodedata.normalize("NFC", text)


def edit_distance(left: str, right: str) -> int:
    if len(left) < len(right):
        left, right = right, left
    if not right:
        return len(left)
    previous = list(range(len(right) + 1))
    for row, left_char in enumerate(left, start=1):
        current = [row]
        for column, right_char in enumerate(right, start=1):
            current.append(min(current[-1] + 1, previous[column] + 1, previous[column - 1] + (left_char != right_char)))
        previous = current
    return previous[-1]


def ratio(numerator: float, denominator: float) -> float | None:
    return numerator / denominator if denominator else None


def metric(numerator: float, denominator: float) -> dict[str, float | int | None]:
    return {"numerator": numerator, "denominator": denominator, "value": ratio(numerator, denominator)}


def _unique_index(items: Iterable[dict[str, Any]], id_key: str, label: str) -> dict[str, dict[str, Any]]:
    indexed: dict[str, dict[str, Any]] = {}
    for item in items:
        identifier = item.get(id_key)
        if not isinstance(identifier, str) or not identifier:
            raise ValueError(f"{label} occurrence is missing string {id_key}")
        if identifier in indexed:
            raise ValueError(f"duplicate {label} occurrence id {identifier!r}")
        indexed[identifier] = item
    return indexed


def _box(item: dict[str, Any]) -> tuple[float, float, float, float] | None:
    value = item.get("region", item.get("box"))
    if not isinstance(value, dict):
        polygon = item.get("polygon")
        if isinstance(polygon, dict):
            polygon = polygon.get("points")
        if isinstance(polygon, list) and len(polygon) >= 3:
            try:
                xs = [float(point[0]) for point in polygon]
                ys = [float(point[1]) for point in polygon]
                value = {"x": min(xs), "y": min(ys), "width": max(xs) - min(xs), "height": max(ys) - min(ys)}
            except (TypeError, ValueError, IndexError):
                return None
        else:
            return None
    try:
        x, y, width, height = (float(value[key]) for key in ("x", "y", "width", "height"))
    except (KeyError, TypeError, ValueError):
        return None
    if not all(math.isfinite(number) for number in (x, y, width, height)) or x < 0 or y < 0 or width <= 0 or height <= 0 or x + width > 1.000001 or y + height > 1.000001:
        return None
    return x, y, width, height


def _box_iou(left: tuple[float, float, float, float] | None, right: tuple[float, float, float, float] | None) -> float:
    if left is None or right is None:
        return 0.0
    lx, ly, lw, lh = left
    rx, ry, rw, rh = right
    intersection = max(0.0, min(lx + lw, rx + rw) - max(lx, rx)) * max(0.0, min(ly + lh, ry + rh) - max(ly, ry))
    union = lw * lh + rw * rh - intersection
    return intersection / union if union else 0.0


def _shape(item: dict[str, Any]) -> list[tuple[float, float]]:
    polygon = item.get("polygon")
    if isinstance(polygon, dict):
        polygon = polygon.get("points")
    if isinstance(polygon, list) and len(polygon) >= 3:
        points: list[tuple[float, float]] = []
        for point in polygon:
            if not isinstance(point, (list, tuple)) or len(point) != 2:
                return []
            try:
                x, y = float(point[0]), float(point[1])
            except (TypeError, ValueError):
                return []
            if not math.isfinite(x) or not math.isfinite(y) or not 0 <= x <= 1 or not 0 <= y <= 1:
                return []
            points.append((x, y))
        return points
    bounds = _box(item)
    if bounds is None:
        return []
    x, y, width, height = bounds
    return [(x, y), (x + width, y), (x + width, y + height), (x, y + height)]


def _point_in_polygon(point: tuple[float, float], polygon: list[tuple[float, float]]) -> bool:
    x, y = point
    inside = False
    previous = polygon[-1]
    for current in polygon:
        x1, y1 = previous
        x2, y2 = current
        if (y1 > y) != (y2 > y) and x < (x2 - x1) * (y - y1) / (y2 - y1) + x1:
            inside = not inside
        previous = current
    return inside


def _shape_iou(left: dict[str, Any], right: dict[str, Any], samples: int = 64) -> float:
    left_shape, right_shape = _shape(left), _shape(right)
    if not left_shape or not right_shape:
        return 0.0
    all_points = left_shape + right_shape
    x0, x1 = min(point[0] for point in all_points), max(point[0] for point in all_points)
    y0, y1 = min(point[1] for point in all_points), max(point[1] for point in all_points)
    if x1 <= x0 or y1 <= y0:
        return 0.0
    intersection = union = 0
    for row in range(samples):
        y = y0 + (row + 0.5) * (y1 - y0) / samples
        for column in range(samples):
            x = x0 + (column + 0.5) * (x1 - x0) / samples
            in_left = _point_in_polygon((x, y), left_shape)
            in_right = _point_in_polygon((x, y), right_shape)
            intersection += in_left and in_right
            union += in_left or in_right
    return intersection / union if union else 0.0


def _path(item: dict[str, Any]) -> list[tuple[float, float]]:
    raw = item.get("path", item.get("points", []))
    if not isinstance(raw, list):
        return []
    points: list[tuple[float, float]] = []
    for point in raw:
        if not isinstance(point, (list, tuple)) or len(point) != 2:
            return []
        try:
            x, y = float(point[0]), float(point[1])
        except (TypeError, ValueError):
            return []
        if not math.isfinite(x) or not math.isfinite(y) or not 0 <= x <= 1 or not 0 <= y <= 1:
            return []
        points.append((x, y))
    return points


def _path_matches(left: list[tuple[float, float]], right: list[tuple[float, float]]) -> bool:
    if len(left) < 2 or len(right) < 2:
        return False
    direct = max(math.dist(left[0], right[0]), math.dist(left[-1], right[-1]))
    reverse = max(math.dist(left[0], right[-1]), math.dist(left[-1], right[0]))
    if min(direct, reverse) > CONNECTOR_ENDPOINT_TOLERANCE:
        return False
    return min(_discrete_frechet(left, right), _discrete_frechet(left, list(reversed(right)))) <= CONNECTOR_PATH_TOLERANCE


def _resample_path(path: list[tuple[float, float]], count: int = 48) -> list[tuple[float, float]]:
    if len(path) < 2:
        return path
    distances = [0.0]
    for index in range(1, len(path)):
        distances.append(distances[-1] + math.dist(path[index - 1], path[index]))
    total = distances[-1]
    if total == 0:
        return [path[0]] * count
    output = []
    segment = 0
    for index in range(count):
        target = total * index / (count - 1)
        while segment < len(distances) - 2 and distances[segment + 1] < target:
            segment += 1
        length = distances[segment + 1] - distances[segment]
        fraction = (target - distances[segment]) / length if length else 0
        left, right = path[segment], path[segment + 1]
        output.append((left[0] + (right[0] - left[0]) * fraction, left[1] + (right[1] - left[1]) * fraction))
    return output


def _discrete_frechet(left: list[tuple[float, float]], right: list[tuple[float, float]]) -> float:
    a, b = _resample_path(left), _resample_path(right)
    previous = [math.inf] * len(b)
    for row, point_a in enumerate(a):
        current = [math.inf] * len(b)
        for column, point_b in enumerate(b):
            distance = math.dist(point_a, point_b)
            if row == 0 and column == 0:
                current[column] = distance
            else:
                prior = min(previous[column] if row else math.inf, current[column - 1] if column else math.inf, previous[column - 1] if row and column else math.inf)
                current[column] = max(distance, prior)
        previous = current
    return previous[-1]


def _alignment(item: dict[str, Any], gold: dict[str, dict[str, Any]]) -> str | None:
    reference_id = item.get("reference_id")
    return reference_id if isinstance(reference_id, str) and reference_id in gold else None


def document_counts(gold: dict[str, Any], predicted: dict[str, Any]) -> dict[str, dict[str, float]]:
    gold_pages = _unique_index(gold.get("pages", []), "page_id", "gold page")
    predicted_pages = _unique_index(predicted.get("pages", []), "page_id", "predicted page")
    extra_pages = sorted(set(predicted_pages) - set(gold_pages))
    if extra_pages:
        raise ValueError(f"prediction contains pages absent from reference: {extra_pages}")
    raw_errors = normalized_errors = scored_characters = normalized_characters = page_coverage = 0
    for page_id, gold_page in gold_pages.items():
        predicted_page = predicted_pages.get(page_id, {})
        if predicted_page.get("processed") is True:
            page_coverage += 1
        if gold_page.get("text_status", "readable") == "unreadable":
            continue
        reference = str(gold_page.get("text_ar", ""))
        hypothesis = str(predicted_page.get("text_ar", ""))
        raw_errors += edit_distance(reference, hypothesis)
        scored_characters += len(reference)
        normalized_reference = arabic_search_normalize(reference)
        normalized_hypothesis = arabic_search_normalize(hypothesis)
        normalized_errors += edit_distance(normalized_reference, normalized_hypothesis)
        normalized_characters += len(normalized_reference)

    gold_nodes = _unique_index(gold.get("nodes", []), "id", "gold name")
    predicted_nodes = _unique_index(predicted.get("nodes", []), "id", "predicted name")
    localized_by_gold: dict[str, list[str]] = defaultdict(list)
    exact_by_gold: dict[str, list[str]] = defaultdict(list)
    search_by_gold: dict[str, list[str]] = defaultdict(list)
    detection_mapping: dict[str, str] = {}
    exact_mapping: dict[str, str] = {}
    name_status = {identifier: node.get("name_status", node.get("status", "readable")) for identifier, node in gold_nodes.items()}
    unreadable_gold = {identifier for identifier, status in name_status.items() if status == "unreadable"}
    partial_gold = {identifier for identifier, status in name_status.items() if status == "partial"}
    readable_gold = {identifier for identifier, status in name_status.items() if status == "readable"}
    for local_id, node in predicted_nodes.items():
        reference_id = _alignment(node, gold_nodes)
        if reference_id is None:
            continue
        reference = gold_nodes[reference_id]
        if str(node.get("page_id", reference.get("page_id", ""))) != str(reference.get("page_id", "")):
            continue
        if _box_iou(_box(node), _box(reference)) < IOU_THRESHOLD:
            continue
        localized_by_gold[reference_id].append(local_id)
        detection_mapping[local_id] = reference_id
        predicted_name = str(node.get("name_ar", ""))
        reference_name = str(reference.get("name_ar", ""))
        if reference_id in readable_gold and node.get("status", "readable") not in {"unreadable", "abstained"} and predicted_name.strip():
            if exact_name_normalize(predicted_name) == exact_name_normalize(reference_name):
                exact_by_gold[reference_id].append(local_id)
                exact_mapping[local_id] = reference_id
            if arabic_search_normalize(predicted_name) == arabic_search_normalize(reference_name):
                search_by_gold[reference_id].append(local_id)

    detected_ids = set(localized_by_gold)
    exact_ids = set(exact_by_gold)
    search_ids = set(search_by_gold)
    detection_correct = len(detected_ids)
    exact_correct = len(exact_ids)
    abstentions = sum(
        1 for reference_id in readable_gold & detected_ids
        if any(predicted_nodes[local_id].get("status") in {"unreadable", "abstained"} or not str(predicted_nodes[local_id].get("name_ar", "")).strip() for local_id in localized_by_gold[reference_id])
    )
    omissions = len(readable_gold - detected_ids)
    duplicate_proposals = sum(max(0, len(local_ids) - 1) for local_ids in localized_by_gold.values())
    matched_locations = [(gold_nodes[reference_id], reference_id) for reference_id in detected_ids]
    for local_id, node in predicted_nodes.items():
        if local_id in detection_mapping:
            continue
        if any(
            str(node.get("page_id", gold_node.get("page_id", ""))) == str(gold_node.get("page_id", ""))
            and _box_iou(_box(node), _box(gold_node)) >= IOU_THRESHOLD
            for gold_node, _ in matched_locations
        ):
            duplicate_proposals += 1

    gold_branches = _unique_index(gold.get("branches", []), "id", "gold branch")
    predicted_branches = _unique_index(predicted.get("branches", []), "id", "predicted branch")
    branch_detected: set[str] = set()
    for branch in predicted_branches.values():
        reference_id = _alignment(branch, gold_branches)
        if reference_id and str(branch.get("page_id", gold_branches[reference_id].get("page_id", ""))) == str(gold_branches[reference_id].get("page_id", "")):
            if _shape_iou(branch, gold_branches[reference_id]) >= IOU_THRESHOLD:
                branch_detected.add(reference_id)

    gold_connectors = _unique_index(gold.get("connectors", []), "id", "gold connector")
    predicted_connectors = _unique_index(predicted.get("connectors", []), "id", "predicted connector")
    connector_detected: set[str] = set()
    for connector in predicted_connectors.values():
        reference_id = _alignment(connector, gold_connectors)
        if reference_id and _path_matches(_path(connector), _path(gold_connectors[reference_id])):
            connector_detected.add(reference_id)

    expected_edges = Counter(_gold_edge(edge) for edge in gold.get("edges", []))
    structure_mapping = detection_mapping
    actual_structure = Counter(_predicted_edge(edge, structure_mapping) for edge in predicted.get("edges", []))
    actual_end_to_end = Counter(_predicted_edge(edge, exact_mapping) for edge in predicted.get("edges", []))
    correct_structure = sum((expected_edges & actual_structure).values())
    correct_end_to_end = sum((expected_edges & actual_end_to_end).values())

    gold_tiles = _unique_index(gold.get("tiles", []), "id", "gold tile")
    predicted_tiles = _unique_index(predicted.get("tiles", []), "id", "predicted tile")
    covered_tiles = {
        reference_id for tile in predicted_tiles.values()
        if (reference_id := _alignment(tile, gold_tiles)) is not None and tile.get("status", "succeeded") == "succeeded"
    }

    gold_mentions = _unique_index(gold.get("mentions", []), "id", "gold mention")
    predicted_mentions = _unique_index(predicted.get("mentions", []), "id", "predicted mention")
    mention_mapping = {local_id: reference_id for local_id, item in predicted_mentions.items() if (reference_id := _alignment(item, gold_mentions)) is not None}
    expected_assertions = Counter(_gold_assertion(item) for item in gold.get("assertions", []))
    actual_assertions = Counter(_predicted_assertion(item, mention_mapping) for item in predicted.get("assertions", []))
    correct_assertions = sum((expected_assertions & actual_assertions).values())

    return {
        "character_error_rate_raw": {"errors": raw_errors, "total": scored_characters},
        "character_error_rate_search_normalized": {"errors": normalized_errors, "total": normalized_characters},
        "exact_name_precision": {"correct": exact_correct, "total": len(predicted_nodes)},
        "exact_name_recall": {"correct": exact_correct, "total": len(readable_gold)},
        "search_normalized_name_precision": {"correct": len(search_ids), "total": len(predicted_nodes)},
        "search_normalized_name_recall": {"correct": len(search_ids), "total": len(readable_gold)},
        "name_detection_precision": {"correct": detection_correct, "total": len(predicted_nodes)},
        "name_detection_recall": {"correct": detection_correct, "total": len(gold_nodes)},
        "unreadable_label_detection_recall": {"correct": len(detected_ids & unreadable_gold), "total": len(unreadable_gold)},
        "partial_label_detection_recall": {"correct": len(detected_ids & partial_gold), "total": len(partial_gold)},
        "branch_precision": {"correct": len(branch_detected), "total": len(predicted_branches)},
        "branch_recall": {"correct": len(branch_detected), "total": len(gold_branches)},
        "connector_precision": {"correct": len(connector_detected), "total": len(predicted_connectors)},
        "connector_recall": {"correct": len(connector_detected), "total": len(gold_connectors)},
        "relationship_structure_precision": {"correct": correct_structure, "total": sum(actual_structure.values())},
        "relationship_structure_recall": {"correct": correct_structure, "total": sum(expected_edges.values())},
        "relationship_precision": {"correct": correct_end_to_end, "total": sum(actual_end_to_end.values())},
        "relationship_recall": {"correct": correct_end_to_end, "total": sum(expected_edges.values())},
        "assertion_precision": {"correct": correct_assertions, "total": sum(actual_assertions.values())},
        "assertion_recall": {"correct": correct_assertions, "total": sum(expected_assertions.values())},
        "page_coverage": {"correct": page_coverage, "total": len(gold_pages)},
        "tile_coverage": {"correct": len(covered_tiles), "total": len(gold_tiles)},
        "diagnostics": {
            "readable_name_omissions": omissions,
            "duplicate_name_proposals": duplicate_proposals,
            "name_abstentions": abstentions,
            "unreadable_gold_name_occurrences": len(unreadable_gold),
            "partial_gold_name_occurrences": len(partial_gold),
            "proposed_name_occurrences": len(predicted_nodes),
            "processing_seconds": _nonnegative_number(predicted.get("processing_seconds")),
            "review_seconds": _nonnegative_number(predicted.get("review_seconds")),
            "cost_usd": _nonnegative_number(predicted.get("cost_usd")),
            "provider_call_count": _nonnegative_number(predicted.get("provider_call_count")),
            "ocr_operation_count": _nonnegative_number(predicted.get("ocr_operation_count")),
            "input_image_bytes": _nonnegative_number(predicted.get("input_image_bytes")),
        },
    }


def _nonnegative_number(value: Any) -> float:
    try:
        number = float(value or 0)
    except (TypeError, ValueError):
        return 0.0
    return number if math.isfinite(number) and number >= 0 else 0.0


def _gold_edge(edge: dict[str, Any]) -> tuple[str, str, str]:
    return (str(edge.get("subject_id", "")), str(edge.get("predicate", "")), str(edge.get("object_id", "")))


def _predicted_edge(edge: dict[str, Any], mapping: dict[str, str]) -> tuple[str, str, str]:
    subject = str(edge.get("subject_node_id", ""))
    object_id = str(edge.get("object_node_id", ""))
    return (mapping.get(subject, "unmatched:" + subject), str(edge.get("predicate", "")), mapping.get(object_id, "unmatched:" + object_id))


def _gold_assertion(assertion: dict[str, Any]) -> tuple[str, ...]:
    return tuple(str(assertion.get(key, "")) for key in ("subject_id", "predicate", "object_id", "quote_ar", "attribution_ar", "polarity", "certainty"))


def _predicted_assertion(assertion: dict[str, Any], mapping: dict[str, str]) -> tuple[str, ...]:
    subject = str(assertion.get("subject_mention_id", ""))
    object_id = str(assertion.get("object_mention_id", ""))
    return (
        mapping.get(subject, "unmatched:" + subject), str(assertion.get("predicate", "")),
        mapping.get(object_id, "unmatched:" + object_id), str(assertion.get("quote_ar", "")),
        str(assertion.get("attribution_ar", "")), str(assertion.get("polarity", "")), str(assertion.get("certainty", "")),
    )


def _sum_diagnostics(counts: list[dict[str, Any]]) -> dict[str, float]:
    keys = {key for count in counts for key in count["diagnostics"]}
    return {key: sum(count["diagnostics"].get(key, 0) for count in counts) for key in sorted(keys)}


def summarize_counts(counts: list[dict[str, Any]], bootstrap_samples: int, seed: int) -> dict[str, Any]:
    result: dict[str, Any] = {}
    rng = random.Random(seed)
    for key in METRIC_KEYS:
        numerator_key = "errors" if key.startswith("character_error") else "correct"
        values = [item[key] for item in counts]
        numerator = sum(value[numerator_key] for value in values)
        denominator = sum(value["total"] for value in values)
        metric_result = metric(numerator, denominator)
        if len(values) > 1 and bootstrap_samples > 0:
            samples: list[float] = []
            for _ in range(bootstrap_samples):
                draw = [values[rng.randrange(len(values))] for _ in values]
                draw_numerator = sum(value[numerator_key] for value in draw)
                draw_denominator = sum(value["total"] for value in draw)
                value = ratio(draw_numerator, draw_denominator)
                if value is not None:
                    samples.append(value)
            samples.sort()
            if samples:
                metric_result["document_bootstrap_95_ci"] = [samples[int(.025 * (len(samples) - 1))], samples[int(.975 * (len(samples) - 1))]]
        result[key] = metric_result
    return result


def evaluate(gold_records: dict[str, dict[str, Any]], prediction_records: dict[str, dict[str, Any]], bootstrap_samples: int = 2000, seed: int = 1) -> dict[str, Any]:
    if set(gold_records) != set(prediction_records):
        missing = sorted(set(gold_records) - set(prediction_records))
        extra = sorted(set(prediction_records) - set(gold_records))
        raise ValueError(f"document sets differ; missing predictions={missing}, unexpected predictions={extra}")
    by_class: dict[str, list[dict[str, Any]]] = defaultdict(list)
    by_split: dict[str, list[dict[str, Any]]] = defaultdict(list)
    per_document: dict[str, Any] = {}
    all_counts: list[dict[str, Any]] = []
    for document_id in sorted(gold_records):
        gold, prediction = gold_records[document_id], prediction_records[document_id]
        if gold.get("source_class") != prediction.get("source_class"):
            raise ValueError(f"source_class differs for document {document_id!r}")
        counts = document_counts(gold, prediction)
        all_counts.append(counts)
        by_class[str(gold["source_class"])].append(counts)
        by_split[str(gold.get("split", "unspecified"))].append(counts)
        per_document[document_id] = {"metrics": summarize_counts([counts], 0, seed), "diagnostics": counts["diagnostics"], "split": gold.get("split", "unspecified")}
    held_out = by_split.get("held_out", [])
    held_metrics = summarize_counts(held_out, bootstrap_samples, seed)
    precision = held_metrics["exact_name_precision"]["value"]
    recall = held_metrics["exact_name_recall"]["value"]
    meets_targets = precision is not None and recall is not None and precision >= PRECISION_TARGET and recall >= RECALL_TARGET
    return {
        "document_count": len(all_counts),
        "metrics": summarize_counts(all_counts, bootstrap_samples, seed),
        "by_source_class": {source_class: {"document_count": len(values), "metrics": summarize_counts(values, bootstrap_samples, seed), "diagnostics": _sum_diagnostics(values)} for source_class, values in sorted(by_class.items())},
        "by_split": {split: {"document_count": len(values), "metrics": summarize_counts(values, bootstrap_samples, seed), "diagnostics": _sum_diagnostics(values)} for split, values in sorted(by_split.items())},
        "held_out_evidence": {
            "document_count": len(held_out),
            "metrics": held_metrics,
            "targets": {"exact_name_precision": PRECISION_TARGET, "exact_name_recall": RECALL_TARGET},
            "meets_point_targets": meets_targets,
            "status": "pending_independent_review",
            "qualification_reasons": [
                "two_independent_readers_and_adjudication_are_not_attested_by_this_evaluator",
                "reviewer_approval_and_supported_chart_class_are_external decisions",
                *(["fewer_than_three_held_out_documents"] if len(held_out) < MIN_HELD_OUT_DOCUMENTS else []),
                *(["held_out_point_targets_not_met"] if not meets_targets else []),
            ],
            "uncertainty_note": "Document bootstrap intervals are descriptive. A small corpus or zero-error interval does not establish general reliability.",
        },
        "per_document": per_document,
    }


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--gold", required=True, type=Path, help="Adjudicated document-level JSONL")
    parser.add_argument("--predictions", required=True, action="append", metavar="LABEL=PATH", help="Provider/pipeline JSONL; repeat to compare systems")
    parser.add_argument("--output", type=Path, help="Write a JSON comparison report; default is stdout")
    parser.add_argument("--bootstrap-samples", type=int, default=2000)
    parser.add_argument("--seed", type=int, default=1)
    args = parser.parse_args()
    if args.bootstrap_samples < 0:
        parser.error("--bootstrap-samples must be nonnegative")
    try:
        gold = read_jsonl(args.gold)
        comparisons = {}
        for item in args.predictions:
            if "=" not in item:
                raise ValueError(f"prediction must be LABEL=PATH, got {item!r}")
            label, filename = item.split("=", 1)
            if not label or label in comparisons:
                raise ValueError(f"empty or duplicate pipeline label {label!r}")
            comparisons[label] = evaluate(gold, read_jsonl(Path(filename)), args.bootstrap_samples, args.seed)
        output = json.dumps({"schema_version": 2, "pipelines": comparisons}, ensure_ascii=False, indent=2) + "\n"
        if args.output:
            args.output.write_text(output, encoding="utf-8")
        else:
            sys.stdout.write(output)
    except (OSError, ValueError) as error:
        print(f"ocr-eval: {error}", file=sys.stderr)
        return 2
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
