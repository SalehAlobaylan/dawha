/**
 * Reference captions for the atlas canvas.
 *
 * These are geographic facts drawn in reference grammar: sea and gulf names,
 * mountain ranges, and the well-known modern city names, set small and muted so
 * nobody can mistake them for archive records. They never carry a claim, a date,
 * or a source citation, because they are not the archive's business — they are
 * north up, and the water is west.
 *
 * GENERATED CONTENT: entries come from apps/web/src/assets/reference-places.json
 * (Wikidata, CC0), produced by infra/local/build-reference-layer.py. Edit the
 * curated list in the generator and regenerate; do not hand-edit the artifact.
 * 'placement: editorial' marks a caption whose drawn coordinate is a placement
 * inside the visible frame (its item's centroid lies outside it), while the
 * qid stays recorded so the reference remains re-derivable.
 */
import referencePlaces from "../assets/reference-places.json";

export type MapReferenceKind = "sea" | "mountain" | "city";

export interface MapReferenceCaption {
  label: string;
  longitude: number;
  latitude: number;
  kind: MapReferenceKind;
  /** The Wikidata item the caption was derived from (CC0 structured data). */
  qid: string;
  /** How the drawn coordinate was chosen; honest provenance for a placement. */
  placement: "wikidata" | "editorial";
}

interface RawReferenceEntry {
  label: string;
  kind: string;
  qid: string;
  longitude: number;
  latitude: number;
  placement: string;
}

const KINDS: MapReferenceKind[] = ["sea", "mountain", "city"];

function kindOf(raw: string): MapReferenceKind {
  if ((KINDS as string[]).includes(raw)) return raw as MapReferenceKind;
  throw new Error(`reference artifact carries an unknown kind: ${raw}`);
}

export const mapReferenceCaptions: MapReferenceCaption[] = (
  (referencePlaces as { entries: RawReferenceEntry[] }).entries ?? []
).map((entry) => ({
  label: entry.label,
  longitude: entry.longitude,
  latitude: entry.latitude,
  kind: kindOf(entry.kind),
  qid: entry.qid,
  placement: entry.placement === "editorial" ? "editorial" : "wikidata",
}));
