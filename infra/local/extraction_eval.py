#!/usr/bin/env python3
"""Plan 030-G: measure the extraction factory from its own tables.

What this measures, and what it refuses to claim
------------------------------------------------
The evaluation asks four questions of the tribal lanes' own records:

  * throughput — candidate rows created per week, by lane;
  * acceptance — the decision mix per source (accepted / corrected / rejected
    / deferred / still suggested), the acceptance rate over DECIDED rows only;
  * projection quality — the distribution of `transform_rms_km` the plate
    proposals carry (the 030-D held-out RMS as the floor, per plate);
  * dira coverage — how much area the accepted board actually underwrites.

The last of these is honest about being small today: no candidate in the
current lanes carries a traced `region_geometry` (every first-lane row is a
label box), so dira coverage is reported as the number of accepted rows with
geometry and NOT as a kilometre figure invented from label centres. A label's
projected centre is not an area, and no aggregation here pretends otherwise.

Read-only: the script opens no connection of its own — it runs the queries
through the same documented path as every local ops command
(`docker compose exec -T db psql …`), prints the human summary, and writes
`docs/benchmarks/extraction-eval.json` with every query recorded beside its
numbers. It never writes to the database.

Run:    make extraction-eval
"""

from __future__ import annotations

import csv
import json
import subprocess
import sys
from datetime import date, datetime, timezone
from pathlib import Path

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[1]
ARTIFACT = ROOT / "docs" / "benchmarks" / "extraction-eval.json"

QUERIES = {
    # Throughput: candidate rows per ISO week, per lane. The lanes are distinct
    # tables (text toponyms vs plate labels), counted separately, never summed
    # into one number that would hide which lane is producing.
    "toponym_lane_weekly": """
        SELECT to_char(date_trunc('week', created_at), 'YYYY-MM-DD') AS week, count(*)
        FROM external_place_candidates
        GROUP BY date_trunc('week', created_at) ORDER BY week""",
    "plate_label_lane_weekly": """
        SELECT to_char(date_trunc('week', created_at), 'YYYY-MM-DD') AS week, count(*)
        FROM external_region_candidates
        GROUP BY date_trunc('week', created_at) ORDER BY week""",
    # Acceptance: the decision mix per source; the rate is derived over
    # decided rows only. 'suggested' rows are work remaining, neither accepted
    # nor refused.
    "candidate_decisions_by_source": """
        SELECT c.source_id::text, COALESCE(src.title_ar, '') AS source_title,
               count(*) AS total,
               count(*) FILTER (WHERE c.status = 'suggested')  AS suggested,
               count(*) FILTER (WHERE c.status = 'accepted')   AS accepted,
               count(*) FILTER (WHERE c.status = 'corrected')  AS corrected,
               count(*) FILTER (WHERE c.status = 'rejected')   AS rejected,
               count(*) FILTER (WHERE c.status = 'deferred')   AS deferred
        FROM external_region_candidates c
        LEFT JOIN sources src ON src.id = c.source_id
        GROUP BY c.source_id, src.title_ar
        ORDER BY total DESC""",
    "pairing_decisions_by_source": """
        SELECT c.source_id::text, COALESCE(src.title_ar, '') AS source_title,
               count(*) AS total,
               count(*) FILTER (WHERE c.status IN ('accepted', 'alias_only')) AS accepted,
               count(*) FILTER (WHERE c.status = 'rejected')                  AS rejected,
               count(*) FILTER (WHERE c.status = 'deferred')                  AS deferred,
               count(*) FILTER (WHERE c.status = 'suggested')                 AS suggested
        FROM external_place_candidates c
        LEFT JOIN sources src ON src.id = c.source_id
        GROUP BY c.source_id, src.title_ar
        ORDER BY total DESC""",
    # Projection quality: the per-plate RMS floor the proposals carry. The
    # distribution is over that recorded floor (the transform's held-out RMS),
    # not over per-candidate errors — no per-candidate error is known yet.
    "transform_rms_distribution": """
        SELECT c.plate_key, c.transform_rms_km,
               count(*) AS proposals,
               min(c.label_confidence) AS min_label_confidence,
               max(c.label_confidence) AS max_label_confidence
        FROM external_region_candidates c
        WHERE c.transform_rms_km IS NOT NULL
        GROUP BY c.plate_key, c.transform_rms_km
        ORDER BY c.plate_key""",
    # Dira coverage: acceptance WITH geometry only. Zero rows with geometry
    # means zero km², stated as zero — never estimated from label centres.
    "dira_coverage": """
        SELECT count(*) FILTER (WHERE c.status IN ('accepted', 'corrected')) AS accepted_rows,
               count(*) FILTER (WHERE c.status IN ('accepted', 'corrected')
                                AND c.region_geometry IS NOT NULL)   AS accepted_with_geometry,
               COALESCE(sum(ST_Area(c.region_geometry::geography)) FILTER (
                 WHERE c.status IN ('accepted', 'corrected') AND c.region_geometry IS NOT NULL), 0) AS coverage_km2
        FROM external_region_candidates c""",
    # The workbench: tiles by state; retirement is the two-person arithmetic's
    # own record.
    "tile_states": """
        SELECT status, count(*) FROM region_tile_tasks GROUP BY status ORDER BY status""",
}

UNIT = "\x1f"  # unit separator: a field delimiter no Arabic source title contains


def main() -> int:
    sections: dict[str, object] = {}
    for name, query in QUERIES.items():
        sections[name] = read_rows(query)

    # The acceptance rates are derived here, and the derivation is recorded
    # beside every rate: accepted + corrected over decided rows.
    for name, accepted_columns in (
        ("candidate_decisions_by_source", ("accepted", "corrected")),
        ("pairing_decisions_by_source", ("accepted",)),
    ):
        for row in sections[name]:  # type: ignore[index]
            decided = sum(row[column] for column in accepted_columns) + row["rejected"]
            row["decided"] = decided
            row["acceptance_rate_among_decided"] = (
                round(sum(row[column] for column in accepted_columns) / decided, 4)
                if decided else None
            )

    coverage = sections["dira_coverage"][0]  # type: ignore[index]
    if not coverage["accepted_with_geometry"]:
        coverage["note"] = (
            "لا توجد مقترحات مقبولة تحمل شكل حدود بعد؛ كل صفوف المسار الأول صناديق ملصقات. "
            "التغطية صفر كم² منطوقة، ولا رقم يُصنع من مراكز الملصقات."
        )

    artifact = {
        "meta": {
            "generator": "infra/local/extraction_eval.py",
            "generatedAt": date.today().isoformat(),
            "generatedAtUtc": datetime.now(timezone.utc).replace(microsecond=0).isoformat(),
            "queries": {name: " ".join(query.split()) for name, query in QUERIES.items()},
            "definitions": {
                "acceptanceRate": (
                    "(accepted + corrected) / decided rows; suggested rows are work "
                    "remaining and sit in no rate"
                ),
                "diraCoverage": (
                    "the area of accepted candidates' traced region_geometry only; a "
                    "label box is not an area, and no km² is derived from its centre"
                ),
                "rms": (
                    "the plate's recorded transform held-out RMS (030-D), the floor "
                    "under every position the lane projects"
                ),
            },
        },
        **sections,
    }
    ARTIFACT.write_text(json.dumps(artifact, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(f"wrote {ARTIFACT.relative_to(ROOT)}")

    for name in ("tile_states", "candidate_decisions_by_source", "dira_coverage"):
        print(f"\n— {name}")
        for row in sections[name]:  # type: ignore[index]
            print("  " + json.dumps(row, ensure_ascii=False))
    return 0


def read_rows(query: str) -> list[dict]:
    """One read-only query through the documented container path, as CSV so a
    source title's own commas cannot misalign a row. A NULL arrives as an
    empty field and is read as None for numeric columns, so an absent
    measurement never impersonates a zero."""
    command = [
        "docker", "compose", "exec", "-T", "db",
        "psql", "-U", "dawha", "-d", "dawha", "-v", "ON_ERROR_STOP=1",
        "-c", " ".join(query.split()), "--csv",
    ]
    completed = subprocess.run(command, capture_output=True, text=True)
    if completed.returncode != 0:
        print(f"extraction-eval: query failed: {completed.stderr.strip()}", file=sys.stderr)
        raise SystemExit(1)
    lines = completed.stdout.splitlines()
    if len(lines) < 1:
        return []
    columns = next(csv.reader([lines[0]]))
    rows = []
    for line in lines[1:]:
        if not line:
            continue
        fields = next(csv.reader([line]))
        row: dict = {}
        for index, column in enumerate(columns):
            value = fields[index] if index < len(fields) else ""
            row[column] = None if value == "" else typed(value)
        rows.append(row)
    return rows


def typed(value: str):
    """Reads a psql text field into the number it is, so the artifact's rates
    compute in Python without a second unquoting layer."""
    try:
        return int(value)
    except ValueError:
        pass
    try:
        return float(value)
    except ValueError:
        return value


if __name__ == "__main__":
    sys.exit(main())
