import type { GeoJSON } from "geojson";
import type { MapStatus } from "./map-vocabulary";

/**
 * The migration events' taxonomy (plan 033-D) — one vocabulary file beside the
 * atlas-periods artifact, with its own header provenance.
 *
 * Provenance / مرجع التصنيف:
 * - The classical repertoire (اليوم، الغزوة، السرية، النفير) is the Arabic
 *   historiographical genre set the sources themselves use: a ONE-DAY raid
 *   (ywm), a large raid (ghazwa), a small night raid (sariya), a general
 *   levy/call-out (nafir) — read against `docs/tribal-territory-solutions.md`
 *   §S4/v2, where the ACLED event taxonomy is adapted: the classical forms map
 *   to the ACLED shapes (battle/no-change, strategic development) but stay the
 *   sources' own words. The vocabulary here carries the SOURCE forms; the
 *   ACLED keys live in the solutions doc, not here.
 * - Relocation and famine are the movement-and-deprivation shapes the corpus
 *   records (migration-like rows whose duration is a season or a drought, not
 *   a day): relocation = نقل/إجابة the sources' own "transfer" verbs; famine =
 *   the movement a deprivation forces (سنة/قحط/majā'). `other` = the recorded
 *   movement the sources narrate without one of the classical names.
 * - An event whose sources narrate a movement WITHOUT classifying it carries
 *   NULL — the honest «حدث غير مُصنَّف بعد» row. This file never guesses.
 */

export const movementEventKinds = [
  "yawm",
  "ghazwa",
  "sariya",
  "nafir",
  "relocation",
  "famine",
  "other",
] as const;

export type MovementEventKind = (typeof movementEventKinds)[number];

export const movementEventLabels: Record<MovementEventKind, string> = {
  yawm: "يوم",
  ghazwa: "غزوة",
  sariya: "سرية",
  nafir: "نفير",
  relocation: "انتقال",
  famine: "سنة قحط",
  other: "أخرى",
};

/** The kind's caption for an event the sources left unclassified — the
 * vocabulary's honest NULL. */
export const movementEventUnclassifiedLabel = "حدث غير مُصنَّف بعد";

/** The chip label for one event's kind, or the unclassified caption for an
 * absent key — never a guess, never an invented form. */
export function movementEventKindLabel(kind: string | null | undefined): string {
  if (!kind) return movementEventUnclassifiedLabel;
  return (movementEventKinds as readonly string[]).includes(kind) ? movementEventLabels[kind as MovementEventKind] : movementEventUnclassifiedLabel;
}

/**
 * The movement modes a corridor stage names (-- plan 033-B): the transport
 * the sources themselves give (camel/horse/foot/sea). The corridor's own
 * day-counts quote the gazetteer's measured distances; the mode is the only
 * interpretive word a stage carries, and it is the SOURCE's word.
 */
export const movementModes = ["camel", "horse", "foot", "sea"] as const;

export type MovementMode = (typeof movementModes)[number];

export const movementModeLabels: Record<MovementMode, string> = {
  camel: "جمال",
  horse: "خيل",
  foot: "سير",
  sea: "بحر",
};

/**
 * The season windows a corridor carries (صيف/شتاء): the pastoral calendar's
 * own two movements (the dira's summer highland, the winter desert) — the
 * era grammar's seasons, not a climate model.
 */
export const movementSeasons = ["summer", "winter"] as const;

export type MovementSeason = (typeof movementSeasons)[number];

export const movementSeasonLabels: Record<MovementSeason, string> = {
  summer: "صيف",
  winter: "شتاء",
};

/** The corridor/event's certainty chip rides the MAP's own status vocabulary —
 * the certainty words are the atlas's, not this file's. */
export type { MapStatus };

/** The corridor's own polyline feature: the stops' ordered geometry (a
 * MultiPoint? no — a LineString through the readable stops, the leg's mode
 * and day count riding the line's own properties). */
export interface MovementCorridorInput {
  id: string;
  titleAr: string;
  season?: string;
  status: string;
  certainty: string;
  stops: Array<{
    sequence: number;
    placeId: string;
    placeName: string;
    longitude: number;
    latitude: number;
    dayCountFromPrev?: number;
    modeFromPrev: string;
  }>;
}

/** corridorCollection builds the corridor layer's polylines: one LineString
 * per corridor through its READABLE stops, dashed by the plan's own grammar
 * (a corridor is a reading, not a constancy), with the legs' mode + day
 * counts carried for the inspector's beats. */
export function corridorCollection(corridors: MovementCorridorInput[]): GeoJSON.FeatureCollection<GeoJSON.LineString> {
  const features: Array<GeoJSON.Feature<GeoJSON.LineString>> = [];
  for (const corridor of corridors) {
    if (corridor.stops.length < 2) continue;
    const stops = [...corridor.stops].sort((left, right) => left.sequence - right.sequence);
    const coordinates = stops.map((stop) => [stop.longitude, stop.latitude] as [number, number]);
    features.push({
      type: "Feature",
      id: corridor.id,
      properties: {
        id: corridor.id,
        titleAr: corridor.titleAr,
        season: corridor.season ?? "",
        status: corridor.status,
        certainty: corridor.certainty,
        firstMode: stops[1]?.modeFromPrev ?? "",
        totalDays: stops.reduce((sum, stop) => sum + (stop.dayCountFromPrev ?? 0), 0),
      },
      geometry: { type: "LineString", coordinates },
    });
  }
  return { type: "FeatureCollection", features };
}
