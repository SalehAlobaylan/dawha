import type { FeatureCollection, Polygon } from "geojson";
import type { AtlasPlateRegisterRow } from "./api";
import { additionalPlateLabels } from "./additional-plate-labels";
import { admiraltyTribeLabels } from "./admiralty-tribe-labels";
import { dicksonTribeLabels } from "./dickson-tribe-labels";
import { dicksonTribeTerritories } from "./dickson-tribe-territories";
import { ottomanHarita1910Territories, type OttomanHaritaTerritory } from "./ottoman-harita-1910-territories";
import type { AtlasPlateLayer, AtlasPlateTribeLabel, AtlasPlateTribeTerritory } from "./atlas";
import { plateForwardLonLat, plateNeatlineRing, plateSourceBounds } from "./atlas";

/**
 * The plate lane's bridge (plan 032-A/B): the atlas register read into the
 * map's layer specs. Every number is one the server measured and validated;
 * this module only narrows and orders — it never re-fits, never invents a
 * coordinate. The overlay is a layer of IMAGES (stipulation 3): the canvas
 * draws the server's warped tiles plus the dashed neatline outline, never a
 * dira shape derived here.
 */

/** The overlay's default opacity in its URL absence (percent, 5..100). */
export const plateHttpDefaultOpacity = 65;

function fallbackTribeLabels(row: AtlasPlateRegisterRow) {
  if (row.plateKey === "admiralty-1916-tribal" && row.sourceId === "b0000000-0000-0000-0000-00000000e004") {
    return admiraltyTribeLabels;
  }
  if (row.plateKey === "dickson-1914-tribal" && row.sourceId === "b0000000-0000-0000-0000-00000000e005") {
    return dicksonTribeLabels;
  }
  return additionalPlateLabels[row.plateKey] ?? null;
}

export function hasPlateTribeLabels(row: AtlasPlateRegisterRow): boolean {
  return Boolean(row.captions?.length) || Boolean(fallbackTribeLabels(row));
}

/**
 * The renderable layer list: register rows whose stored transform validated,
 * narrowed to the URL's ON keys, in the keys' order. A plate without a
 * usable transform is the panel's captioned refusal — never a spec, never an
 * approximation.
 */
export function plateLayerSpecs(
  register: { plates: AtlasPlateRegisterRow[] } | null | undefined,
  keys: string[],
  opacityPercent: number,
): AtlasPlateLayer[] {
  const opacity = Math.min(1, Math.max(0.05, opacityPercent / 100));
  if (!register || keys.length === 0) return [];
  const rows = new Map(register.plates.map((row) => [row.plateKey, row]));
  const specs: AtlasPlateLayer[] = [];
  for (const plateKey of keys) {
    const row = rows.get(plateKey);
    if (!row?.transform) continue;
    const { transform, frame } = row.transform;
    const fallback = fallbackTribeLabels(row);
    const captions = row.captions?.length ? row.captions : (fallback
      ? fallback.map(([printedName, nameAr, x, y]) => ({ id: undefined, tribeId: undefined, printedName, nameAr, x, y }))
      : []);
    const tribeLabels = captions.flatMap(({ id: catalogId, tribeId, printedName, nameAr, x, y }) => {
      if (x < frame.innerLeft || x >= frame.innerRight || y < frame.innerTop || y >= frame.innerBottom) return [];
      return [{ id: `${plateKey}:${x}-${y}`, plateKey, catalogId, tribeId, printedName, nameAr, coordinates: plateForwardLonLat(transform, x, y) }];
    });
    let tribeTerritories: AtlasPlateTribeTerritory[] | undefined;
    if (row.plateKey === "dickson-1914-tribal") {
      tribeTerritories = dicksonTribeTerritories.map((territory, index) => ({
        id: `${plateKey}:territory:${index}:${territory.captionX}-${territory.captionY}`,
        captionId: `${plateKey}:${territory.captionX}-${territory.captionY}`,
        plateKey,
        printedName: territory.printedName,
        nameAr: territory.nameAr,
        spanAr: territory.spanAr,
        rings: territory.rings.map((ring) => ring.map(([px, py]) => plateForwardLonLat(transform, px, py))),
      }));
    }
    specs.push({
      key: row.plateKey,
      title: row.titleAr || row.plateKey,
      needsReview: row.transform.needsReview,
      opacity,
      transform,
      frame,
      tribeLabels,
      tribeTerritories,
    });
  }
  return specs;
}

/**
 * Projects digitized historical-plate tribal boundary polygons into a GeoJSON
 * collection rendered in the plate's own fine-dotted visual register.
 * Deduplicates shared rings (e.g. Harb Hejaz + Najd anchors sharing one full
 * territory polygon) while preserving lookup by captionId or tribe name.
 */
export function plateTerritoryCollection(
  plateLayers: readonly AtlasPlateLayer[],
): FeatureCollection<Polygon, { id: string; captionId: string; plateKey: string; nameAr: string; printedName: string; spanAr: string }> {
  const seen = new Set<string>();
  const features: FeatureCollection<Polygon, { id: string; captionId: string; plateKey: string; nameAr: string; printedName: string; spanAr: string }>["features"] = [];
  for (const plate of plateLayers) {
    for (const territory of plate.tribeTerritories ?? []) {
      const signature = `${plate.key}:${territory.nameAr}:${territory.rings[0]?.[0]?.[0].toFixed(3)},${territory.rings[0]?.[0]?.[1].toFixed(3)}`;
      if (seen.has(signature)) continue;
      seen.add(signature);
      for (const ring of territory.rings) {
        if (ring.length < 4) continue;
        features.push({
          type: "Feature",
          geometry: { type: "Polygon", coordinates: [ring] },
          properties: {
            id: territory.id,
            captionId: territory.captionId,
            plateKey: territory.plateKey,
            nameAr: territory.nameAr,
            printedName: territory.printedName,
            spanAr: territory.spanAr ?? "",
          },
        });
      }
    }
  }
  return { type: "FeatureCollection", features };
}

type LabelBox = { left: number; top: number; right: number; bottom: number };

export function mapLabelBox(name: string, x: number, y: number): LabelBox {
  // Leave room for Arabic glyphs, the label's padding, and a small gap.
  const halfWidth = (Array.from(name).length * 11 + 20) / 2;
  return { left: x - halfWidth, right: x + halfWidth, top: y - 13, bottom: y + 13 };
}

export function mapLabelsOverlap(a: LabelBox, b: LabelBox): boolean {
  return a.left < b.right && a.right > b.left && a.top < b.bottom && a.bottom > b.top;
}

export function placePlateTribeLabels(
  labels: readonly AtlasPlateTribeLabel[],
  project: (coordinates: AtlasPlateTribeLabel["coordinates"]) => { x: number; y: number },
  width: number,
  height: number,
  markerBoxes: readonly LabelBox[] = [],
) {
  const placed: Array<LabelBox & { label: AtlasPlateTribeLabel; x: number; y: number }> = [];
  for (const label of labels) {
    const { x, y } = project(label.coordinates);
    const box = mapLabelBox(label.nameAr, x, y);
    if (box.left < 0 || box.right > width || box.top < 0 || box.bottom > height) continue;
    if (placed.some((other) => mapLabelsOverlap(box, other)) || markerBoxes.some((marker) => mapLabelsOverlap(box, marker))) continue;
    placed.push({ label, x, y, ...box });
  }
  return placed;
}

export function ottomanHaritaCollection(): FeatureCollection<Polygon, {
  id: string;
  nameAr: string;
  ottomanName: string;
  summaryAr: string;
  locationAr: string;
}> {
  const features: FeatureCollection<Polygon, {
    id: string;
    nameAr: string;
    ottomanName: string;
    summaryAr: string;
    locationAr: string;
  }>["features"] = [];
  for (const territory of ottomanHarita1910Territories) {
    for (const ring of territory.rings) {
      if (ring.length < 4) continue;
      features.push({
        type: "Feature",
        geometry: {
          type: "Polygon",
          coordinates: [ring.map(([lon, lat]) => [lon, lat])],
        },
        properties: {
          id: territory.id,
          nameAr: territory.nameAr,
          ottomanName: territory.ottomanName,
          summaryAr: territory.summaryAr,
          locationAr: territory.locationAr ?? "",
        },
      });
    }
  }
  return { type: "FeatureCollection", features };
}

export { ottomanHarita1910Territories, type OttomanHaritaTerritory };
export { plateForwardLonLat, plateNeatlineRing, plateSourceBounds };
