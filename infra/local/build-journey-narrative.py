#!/usr/bin/env python3
"""Plan 033-F: the narrated journey export (رحلة مسردة).

Reads ONE tribe's documented movement rows + corridors — the plan's acceptance
word by word:

- DOCUMENTED ONLY: rows whose status = 'documented' AND certainty <> 'uncertain';
  a platform_inferred row NEVER enters the export (the acceptance: zero).
- Receipts per slide: the sourcing citation + the source id ride each slide.
- StoryMapJS-compatible shape: a slide per event/stop (title, text, the map's
  own lat/lon) inside the era window. The SHAREABLE artifact is the corpus's
  own export; nothing here writes a history the sources did not carry, and
  nothing renders a row the atlas reader could not have met on /places.

Generation is a generator, not a surface: it reads the committed database via
the documented container path (the plan-009 machine shape), writes the
artifact JSON + a sha256 sidecar (the plan's own checksummed-artifact rule),
and re-running it re-reads the same committed subset. The script fails loudly
when the window holds no documented row — it never emits an empty "journey"
that reads as narrated history.
"""

import argparse
import datetime
import hashlib
import json
import pathlib
import subprocess
import sys

ROOT = pathlib.Path(__file__).resolve().parents[2]
OUT = ROOT / "infra" / "local" / "journeys"
CONTAINER = "dawha-db-1"
FIELD_SEP = "\x1f"

HEADER = {
    "storymap_version": "0.1.0",
    "generator": "infra/local/build-journey-narrative.py",
    "contract": "documented rows only; receipts per slide; platform_inferred never exports",
}


def read_rows(sql: str) -> list[list[str]]:
    """One read against the documented local container: the generator's own
    bounded read — scripts, not screenshots."""
    psql = subprocess.run(
        ["docker", "exec", CONTAINER, "psql", "-U", "dawha", "-d", "dawha", "-t", "-A", "-F", FIELD_SEP, "-c", sql],
        capture_output=True, text=True, check=True,
    )
    rows = []
    for line in psql.stdout.splitlines():
        if line:
            rows.append(line.split(FIELD_SEP))
    return rows


def sha256_file(path: pathlib.Path) -> str:
    digest = hashlib.sha256()
    with open(path, "rb") as handle:
        for chunk in iter(lambda: handle.read(65536), b""):
            digest.update(chunk)
    return digest.hexdigest()


def name_literal(name: str) -> str:
    """One Arabic name into a quoted SQL literal — the generator's only
    string surgery, escaped to the grain."""
    return "'" + name.replace("'", "''") + "'"


def build(tribe_name: str, era_from: int, era_to: int) -> int:
    out_dir = OUT
    out_dir.mkdir(parents=True, exist_ok=True)
    slug = (tribe_name + "-" + str(era_from) + "-" + str(era_to)).replace(" ", "-")
    tribe = name_literal(tribe_name)

    events = read_rows(f"""
        SELECT m.id::text,
               m.time_from::text, m.time_to::text,
               pf.canonical_name_ar, ST_X(pf.geometry)::text, ST_Y(pf.geometry)::text,
               pt.canonical_name_ar, ST_X(pt.geometry)::text, ST_Y(pt.geometry)::text,
               COALESCE(src.citation_ar, ''), src.id::text
        FROM migration_events m
        JOIN places pf ON pf.id = m.from_place_id
        JOIN places pt ON pt.id = m.to_place_id
        JOIN sources src ON src.id = m.source_id
        JOIN tribes tr ON tr.id = m.subject_id
        WHERE m.subject_type = 'tribe'
          AND tr.canonical_name_ar = {tribe}
          AND m.status = 'documented'
          AND m.certainty <> 'uncertain'
          AND m.time_from >= '{era_from}-01-01'::date
          AND m.time_to <= '{era_to}-12-31'::date
        ORDER BY m.time_from NULLS LAST, m.id
    """)
    corridor_rows = read_rows(f"""
        SELECT c.id::text, c.title_ar, COALESCE(c.season, ''), st.sequence_number,
               p.canonical_name_ar, ST_X(p.geometry)::text, ST_Y(p.geometry)::text,
               COALESCE(st.day_count_from_prev::text, ''), COALESCE(st.mode_from_prev, ''),
               COALESCE(src.citation_ar, ''), src.id::text
        FROM movement_corridors c
        JOIN movement_corridor_stops st ON st.corridor_id = c.id
        JOIN places p ON p.id = st.place_id
        JOIN sources src ON src.id = c.source_id
        JOIN tribes tr ON tr.id = c.subject_id
        WHERE c.subject_type = 'tribe'
          AND tr.canonical_name_ar = {tribe}
          AND c.status = 'documented'
          AND c.certainty <> 'uncertain'
          AND (c.valid_from IS NULL OR c.valid_from <= {era_to})
          AND (c.valid_to IS NULL OR c.valid_to >= {era_from})
        ORDER BY c.id, st.sequence_number
    """)
    if not events and not corridor_rows:
        print(f"no documented movement rows for {tribe_name} inside {era_from}..{era_to}; refusing an empty narrative", file=sys.stderr)
        return 2

    slides: list[dict] = []
    for (event_id, time_from, time_to,
         from_name, _from_lon, _from_lat,
         to_name, to_lon, to_lat,
         citation, source_id) in events:
        slides.append({
            "slide_type": "event",
            "title_ar": f"{from_name} — {to_name}",
            "text_ar": f"حدث مسجَّل: {from_name} — {to_name} ({time_from} .. {time_to}).",
            "location": {"lat": float(to_lat), "lon": float(to_lon)},
            "era": [time_from, time_to],
            "receipts": {
                "event_id": event_id,
                "source_id": source_id,
                "citation_ar": citation,
            },
        })
    current_corridor = ""
    for (corridor_id, title_ar, season, sequence,
         place_name, lon, lat, days, mode,
         citation, source_id) in corridor_rows:
        if current_corridor != corridor_id:
            current_corridor = corridor_id
            slides.append({
                "slide_type": "corridor",
                "title_ar": title_ar,
                "text_ar": f"{title_ar} عن الموسم {season or 'غير محددة'}.",
                "location": {"lat": float(lat), "lon": float(lon)},
                "era": None,
                "receipts": {
                    "corridor_id": corridor_id,
                    "stop_sequence": int(sequence),
                    "place": place_name,
                    "day_count_from_prev": int(days) if days else None,
                    "mode": mode,
                    "source_id": source_id,
                    "citation_ar": citation,
                },
            })

    artifact = {
        "header": HEADER,
        "storymap": {
            "title": tribe_name,
            "era": [era_from, era_to],
            "slides": slides,
            "generated_at": datetime.date.today().isoformat(),
        },
    }
    out_path = out_dir / f"journey-{slug}.json"
    payload = json.dumps(artifact, ensure_ascii=False, indent=2) + "\n"
    out_path.write_text(payload, encoding="utf-8")
    print(f"wrote {out_path.relative_to(ROOT)} ({len(slides)} slides)")

    checksum = sha256_file(out_path)
    checksum_path = out_path.with_suffix(".json.sha256")
    checksum_path.write_text(checksum + "  " + out_path.name + "\n", encoding="utf-8")
    print(f"wrote {checksum_path.relative_to(ROOT)} ({checksum[:16]}…)")
    return 0


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="build a narrated journey export (documented rows only)")
    parser.add_argument("--tribe", required=True, help="the tribe's canonical Arabic name")
    parser.add_argument("--from", dest="era_from", type=int, required=True, help="the era window's from-year")
    parser.add_argument("--to", dest="era_to", type=int, required=True, help="the era window's to-year")
    args = parser.parse_args()
    sys.exit(build(args.tribe, args.era_from, args.era_to))
