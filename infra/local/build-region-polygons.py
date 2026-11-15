#!/usr/bin/env python3
"""Generate the seeded approximate historical-region polygons for the atlas.

The demo map's regions are approximate outlines, and this script is where they
are made: hand-set control points, smoothed into natural-looking rings, then
clipped against the same Natural Earth 50m landmass the atlas draws as its
coastline reference so no region spills into the sea. Each ring is stored
whole; the overlap report at the end is advisory, not a carve: the historical
frames overlap (Yemen contains both Hadhramaut and Tihamah on the coast;
Ahsa's governorate empties into the Empty Quarter), and a tiled layer would
silently change the borders the user is checking against Google.

Boundaries here are declared approximations, not historical claims: they must
never be read as documented frontiers, and nothing in the atlas presents them
as such. They use the classical fivefold division as the reference frame:
Tihāmah is checked against Britannica's Red Sea coastal-plain width (60–80 km
south of Medina and Mecca, Gulf of Aqaba to Bab el-Mandeb); Ḥijāz follows the
broad western-region envelope shown on Google Maps (Aqaba/Ḥaql to Muhayil,
coast through Jeddah and Al-Qunfudhah, inland to Tabuk, Al-Ula, Khaybar,
Medina, Taif and Al-Bahah); Najd follows Ibn Manẓūr and Fuʾād Ḥamzah
(central highland, west bounded by the Hijaz mountains, north by the Nafud,
east by the Dahna, south by the northern Rubʿ al Khali fringe to Wadi
al-Dawasir). Every ring is stored whole: an administrative
subregion shown inside Najd (e.g. Ha'il or Qassim) keeps its own ring, and
the historical envelope is the outer shape, not a hole-filled carve.

Run:  python3 infra/local/build-region-polygons.py
"""

import json
import os
import sys

try:
    from shapely.geometry import shape, Polygon
    from shapely.ops import unary_union
except ImportError:
    sys.exit("shapely is required: pip install --break-system-packages shapely")

ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
COASTLINE = os.path.join(ROOT, "apps/web/src/lib/coastline.geo.json")
SEED = os.path.join(ROOT, "db/seeds/001_demo.sql")
BEGIN = "-- BEGIN GENERATED REGION POLYGONS"
END = "-- END GENERATED REGION POLYGONS"
RESEARCHER = "00000000-0000-0000-0000-000000000001"

# Macro envelopes are deliberately not carved by the subregions placed
# before them: Google Maps' Hijaz envelope includes its coastal Tihamah
# strip, and Najd's envelope includes Ha'il and Qassim. The tiled
# subregions stay separate for place counting; the outer envelopes remain
# readable as reference frames even where they overlap.

# (name, normalized name, certainty, stable region id, control ring lon/lat).
# Build order is the carve order: deserts and sub-regions claim their ground
# first, so the macro regions (نجد، حجاز) end up carved rather than overlapping.
# ids for نجد/حجاز/الأحساء predate this generator and stay stable.
REGION_SOURCE = [
    ("الربع الخالي", "الربع الخالي", "approximate", 4, [
        (44.90, 20.50), (49.60, 26.50), (52.00, 25.00), (54.80, 24.20), (54.80, 22.00),
        (53.50, 20.00), (51.00, 18.50), (49.00, 17.10), (47.00, 17.10), (45.30, 15.80),
        (44.10, 17.50), (47.40, 17.50), (49.20, 19.00),
    ]),
    ("النفود", "النفود", "approximate", 5, [
        (37.60, 27.80), (38.80, 28.30), (40.20, 28.50), (41.50, 28.70), (42.80, 28.70),
        (43.60, 28.30), (43.20, 27.80), (42.20, 27.50), (41.00, 27.50), (39.00, 27.70),
        (37.80, 28.00),
    ]),
    ("القصيم", "القصيم", "approximate", 6, [
        (42.50, 28.25), (43.60, 28.10), (44.60, 27.60), (44.99, 26.80), (44.50, 25.60),
        (43.80, 24.95), (42.70, 24.48), (41.70, 25.00), (41.30, 26.50), (41.70, 27.50),
    ]),
    ("حائل", "حائل", "approximate", 7, [
        (39.30, 29.20), (41.00, 29.00), (42.80, 28.60), (44.30, 28.00), (44.37, 27.00),
        (43.50, 26.20), (42.50, 25.80), (41.00, 25.40), (39.80, 25.90), (39.11, 27.00),
    ]),
    ("عسير", "عسير", "approximate", 8, [
        (41.30, 20.30), (42.30, 20.40), (43.30, 20.20), (44.30, 19.80), (44.49, 19.30),
        (43.80, 18.70), (43.20, 18.00), (43.40, 17.50), (42.60, 17.37), (42.20, 17.80),
        (41.70, 18.50), (41.00, 19.70),
    ]),
    ("اليمن", "اليمن", "approximate", 9, [
        (42.50, 17.00), (44.50, 17.50), (47.50, 17.60), (49.20, 19.00), (52.00, 16.80),
        (54.50, 16.70), (52.30, 16.10), (50.00, 14.90), (48.00, 14.20),
        (45.00, 12.70), (43.40, 12.60), (43.80, 14.10), (43.20, 16.10), (42.60, 16.90),
    ]),
    ("حضرموت", "حضرموت", "approximate", 10, [
        (47.40, 17.50), (49.20, 19.00), (52.00, 16.80), (54.50, 16.20), (52.70, 15.90),
        (50.10, 14.60), (48.60, 15.90), (48.60, 16.00), (49.00, 16.40), (47.20, 17.50),
    ]),
    # Tihāmah: the Red Sea coastal plain from the Gulf of Aqaba to Bab el-Mandeb
    # (Britannica), never the African shore. The first leg follows the Arabian
    # coast (Ḥaql, Ḍubāʾ, Umluj, Yanbuʿ, Rabigh, Jeddah, Lith, Al-Qunfudhah,
    # Jizan, Midi, Hodeidah, Mokha, Bab el-Mandeb); the second leg returns a
    # few tens of kilometres inland and is capped north by the Aqaba corner.
    ("تهامة", "تهامة", "approximate", 11, [
        (34.95, 29.25), (36.70, 27.35), (37.25, 25.00), (38.05, 24.10), (39.00, 22.90),
        (39.20, 21.50), (40.30, 20.20), (41.10, 19.10), (42.00, 18.00), (42.55, 16.90),
        (42.90, 16.30), (43.00, 14.85), (43.15, 13.35), (43.35, 12.65), (43.65, 13.30),
        (43.45, 14.70), (43.25, 16.10), (42.95, 17.10), (42.20, 18.50), (41.80, 19.50),
        (41.00, 20.60), (39.95, 21.70), (39.50, 23.00), (38.70, 24.50), (37.90, 25.70),
        (37.00, 26.90), (36.40, 28.00), (36.10, 28.90), (35.85, 29.25),
    ]),
    ("الأحساء", "الاحساء", "approximate", 3, [
        (49.60, 26.60), (51.00, 25.50), (52.00, 23.50), (52.00, 21.00), (50.80, 19.70),
        (49.90, 19.30), (48.80, 19.70), (47.52, 21.00), (47.80, 23.50), (48.50, 25.50),
        (49.30, 26.50),
    ]),
    ("القطيف", "القطيف", "approximate", 12, [
        (49.75, 26.45), (49.85, 26.75), (50.00, 26.80), (50.09, 26.70), (50.00, 26.50),
        (50.09, 26.30), (49.85, 26.30), (49.75, 26.45),
    ]),
    ("عُمان", "عمان", "approximate", 13, [
        (56.30, 26.50), (58.60, 23.70), (59.60, 22.50), (59.20, 22.00), (58.10, 21.20),
        (57.30, 18.90), (54.10, 17.00), (52.30, 16.20), (52.00, 18.50), (54.00, 20.50),
        (55.70, 22.40), (55.80, 24.20), (56.00, 24.80), (56.20, 25.80),
    ]),
    # Ḥijāz: the broad western-Saudi envelope shown by the Google Maps
    # "Hijaz" place (Aqaba/Ḥaql in the north, the Red Sea coast through
    # Umluj, Yanbu, Rabigh, Jeddah, Lith and Al-Qunfudhah, and the southern
    # tip at Muhayil/Al-Namas). It deliberately includes both the coastal
    # Tihamah cities and the inland highlands (Tabuk, Al-Ula, Khaybar,
    # Medina, Taif and Al-Bahah), matching the place description that lists
    # Mecca, Medina, Jeddah, Tabuk, Yanbu, Taif and Al-Bahah. The eastern
    # leg keeps Hail, Buraydah, Afif and Bisha out, and the south ends at
    # northern Asir rather than Abha or Jazan.
    ("حجاز", "حجاز", "disputed", 2, [
        (34.95, 29.35), (36.25, 28.65), (36.70, 27.35), (37.25, 25.00), (38.05, 24.10),
        (38.90, 22.90), (39.05, 21.60), (40.30, 20.20), (41.10, 19.10), (42.45, 18.35),
        (42.75, 19.20), (42.45, 20.60), (42.20, 21.40), (41.70, 22.60), (41.00, 23.90),
        (40.75, 25.10), (40.60, 26.20), (40.90, 27.20), (40.60, 27.80), (39.80, 28.30),
        (38.60, 28.80), (37.40, 29.10), (36.20, 29.30),
    ]),
    # Najd: the Google-reference central highland (Aqaba-free central Saudi
    # Arabia): bounded west by the Hijaz mountains, north by the Nafud, east by
    # the Dahna, and south by the northern Rubʿ al Khali fringe to Wadi
    # al-Dawasir / Tathlith. The Google outline includes Hail, Buraydah,
    # Al-Majma'ah, Riyadh, Al-Dawadimi, Afif, Al-Bahah, Bisha and Wadi
    # al-Dawasir, while excluding Madinah, Mecca and the Red Sea coast.
    ("نجد", "نجد", "approximate", 1, [
        (41.80, 28.30), (42.80, 28.30), (43.20, 28.00), (44.70, 27.80), (45.70, 27.20),
        (46.60, 26.80), (47.40, 26.20), (48.80, 25.20), (48.90, 24.00), (48.50, 23.20),
        (47.50, 22.20), (46.20, 21.20), (45.00, 20.10), (43.80, 19.85), (42.65, 19.85),
        (41.60, 19.85), (41.40, 20.40), (41.30, 22.20), (41.20, 23.60), (41.50, 24.60),
        (41.70, 25.70), (41.70, 26.90), (41.40, 27.90),
    ]),
]


def smooth_ring(points, steps=5):
    """Closed centripetal Catmull-Rom through the control ring."""
    count = len(points)
    out = []
    for index in range(count):
        p0 = points[(index - 1) % count]
        p1 = points[index]
        p2 = points[(index + 1) % count]
        p3 = points[(index + 2) % count]
        for step in range(steps):
            t = step / steps
            t2, t3 = t * t, t * t * t
            x = 0.5 * ((2 * p1[0]) + (-p0[0] + p2[0]) * t + (2 * p0[0] - 5 * p1[0] + 4 * p2[0] - p3[0]) * t2 + (-p0[0] + 3 * p1[0] - 3 * p2[0] + p3[0]) * t3)
            y = 0.5 * ((2 * p1[1]) + (-p0[1] + p2[1]) * t + (2 * p0[1] - 5 * p1[1] + 4 * p2[1] - p3[1]) * t2 + (-p0[1] + 3 * p1[1] - 3 * p2[1] + p3[1]) * t3)
            out.append((x, y))
    return out


def wkt(polygon):
    ring = list(polygon.exterior.coords)
    return "POLYGON((" + ", ".join(f"{longitude:.4f} {latitude:.4f}" for longitude, latitude in ring) + "))"


def main():
    raw = json.load(open(COASTLINE))
    if raw.get("type") in ("Polygon", "MultiPolygon"):
        land = shape(raw)
    elif raw.get("type") == "FeatureCollection":
        land = unary_union(shape(feature["geometry"]) for feature in raw["features"])
    else:
        land = shape(raw["geometry"])

    placed = []
    built = []
    for name, normalized, certainty, region_id, control in REGION_SOURCE:
        polygon = Polygon(smooth_ring(control)).buffer(0)
        clipped = polygon.intersection(land)
        if clipped.is_empty:
            sys.exit(f"{name}: clipped away entirely — control ring never touches land")
        biggest = max(getattr(clipped, "geoms", [clipped]), key=lambda part: part.area)
        # Stored whole: the historical frames overlap (Yemen contains both
        # Hadhramaut and the Yemeni Tihamah; Hijaz contains its coastal strip;
        # Najd contains Ha'il and Qassim). The overlap report below is
        # advisory only — no area is silently carved away.
        if biggest.is_empty or biggest.geom_type != "Polygon" or biggest.area < 0.05:
            sys.exit(f"{name}: no usable land component")
        placed.append(biggest)
        bounds = biggest.bounds
        built.append((region_id, name, normalized, certainty, biggest))
        print(f"{name}  vertices {len(biggest.exterior.coords):3d}  area {biggest.area:6.2f} deg²  "
              f"bbox ({bounds[0]:.2f}, {bounds[1]:.2f}, {bounds[2]:.2f}, {bounds[3]:.2f})")

    print("\nremaining overlaps:")
    clean = True
    for first in built:
        for second in built:
            if first[4] is second[4]:
                continue
            overlap = first[4].intersection(second[4])
            if overlap.is_empty:
                continue
            clean = False
            print(f"  {first[1]} ∩ {second[1]}: {overlap.area:.4f} deg²")
    if clean:
        print("  none")

    rows = []
    for region_id, name, normalized, certainty, polygon in built:
        uid = f"40000000-0000-0000-0000-{region_id:012d}"
        rows.append(
            f"  ('{uid}', '{name}', '{normalized}',\n"
            f"   ST_GeomFromText('{wkt(polygon)}', 4326), '{certainty}', NULL, NULL, '{RESEARCHER}')"
        )
    block = (
        f"{BEGIN} — edit this script, not the block (infra/local/build-region-polygons.py).\n"
        "-- Approximate outlines: smooth interpolation of hand-set control points, clipped\n"
        "-- to the Natural Earth 50m landmass, carved by the regions placed before them.\n"
        "-- Not documented frontiers.\n"
        "INSERT INTO historical_regions (id, name_ar, normalized_name_ar, geometry, certainty, valid_from, valid_to, created_by)\n"
        "VALUES\n" + ",\n".join(rows) + "\nON CONFLICT (id) DO UPDATE SET\n"
        "  name_ar = EXCLUDED.name_ar, normalized_name_ar = EXCLUDED.normalized_name_ar,\n"
        "  geometry = EXCLUDED.geometry, certainty = EXCLUDED.certainty, updated_at = now();\n" + END
    )

    with open(SEED) as handle:
        text = handle.read()
    start = text.find(BEGIN)
    if start < 0:
        start = text.find("INSERT INTO historical_regions")
        if start < 0:
            sys.exit("no insertion site found in seed file")
        tail = "ON CONFLICT (id) DO NOTHING;"
        end = text.find(tail, start)
        if end < 0:
            sys.exit("unterminated historical_regions insert")
        end += len(tail)
        text = text[:start] + block + text[end:]
    else:
        end = text.find(END, start)
        if end < 0:
            sys.exit("unterminated generated block")
        text = text[:start] + block + text[end + len(END):]
    with open(SEED, "w") as handle:
        handle.write(text)
    print(f"\nwrote {len(rows)} regions into {os.path.relpath(SEED, ROOT)}")


if __name__ == "__main__":
    main()
