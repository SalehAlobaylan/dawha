import type { EpistemicTone } from "../types";

/**
 * The one map vocabulary.
 *
 * The atlas, the inspector, the legend and the canvas all read from here, so a
 * status cannot mean one thing in a pill and something else in a tooltip. The
 * five values are the backend's, not a demo vocabulary: `documented`,
 * `interpreted`, `platform_inferred`, `disputed`, `unresolved`.
 *
 * Each status carries a fill, a contour and a seal flag. That is deliberate:
 * status is never encoded by colour alone, so the distinction survives
 * grayscale, colour blindness and a printed page.
 */
export const mapStatuses = ["documented", "interpreted", "platform_inferred", "disputed", "unresolved"] as const;
export type MapStatus = (typeof mapStatuses)[number];

export interface MapStatusCopy {
  value: MapStatus;
  label: string;
  glyph: string;
  tone: EpistemicTone;
  fill: string;
  stroke: string;
  /** The contour is dashed, so the glyph reads as a reading rather than ink. */
  dashed: boolean;
  /** The platform's own inference is drawn as a stamp ring, never as an ink disc. */
  seal: boolean;
  /** The marker is hollow: the record exists, but its content is not settled. */
  hollow: boolean;
  description: string;
}

export const mapStatusVocabulary: Record<MapStatus, MapStatusCopy> = {
  documented: {
    value: "documented",
    label: "موثق",
    glyph: "●",
    tone: "source",
    fill: "#2d2520",
    stroke: "#2d2520",
    dashed: false,
    seal: false,
    hollow: false,
    description: "المصدر يذكر الموضع صراحةً.",
  },
  interpreted: {
    value: "interpreted",
    label: "مقروء",
    glyph: "◐",
    tone: "interpretation",
    fill: "#b66f4d",
    stroke: "#6e6257",
    dashed: true,
    seal: false,
    hollow: false,
    description: "قراءة باحث لموضع أو علاقة، وليست نصاً صريحاً.",
  },
  platform_inferred: {
    value: "platform_inferred",
    label: "استنتاجي",
    glyph: "▣",
    tone: "finding",
    fill: "#747153",
    stroke: "#747153",
    dashed: true,
    seal: true,
    hollow: false,
    description: "استنتاج المنصة، ويبقى قابلاً للمراجعة.",
  },
  disputed: {
    value: "disputed",
    label: "متنازع عليه",
    glyph: "◍",
    tone: "disputed",
    fill: "#994438",
    stroke: "#994438",
    dashed: false,
    seal: false,
    hollow: false,
    description: "الروايات لا تتفق على هذا الموضع أو هذه العلاقة.",
  },
  unresolved: {
    value: "unresolved",
    label: "غير محسوم",
    glyph: "○",
    tone: "question",
    fill: "rgba(45, 37, 32, 0.04)",
    stroke: "#8b8176",
    dashed: true,
    seal: false,
    hollow: true,
    description: "الإشارة مسجلة، لكن هويتها أو موضعها لم يُحسم.",
  },
};

export const mapCertainties = ["precise", "approximate", "uncertain"] as const;
export type MapCertainty = (typeof mapCertainties)[number];

/**
 * The undated region under an era window is the one mark that is not a status:
 * the record exists but never dated itself, and a window cannot prove presence.
 * It draws in the vocabulary's weakest voice it still must be seen to be a
 * region: half the fill ink and a dashed legible outline — a state, not an
 * absence. The legend names it and the inspector says so on the same data.
 */
export const mapUndatedRegionCopy = {
  label: "إطار بلا تواريخ مؤرَّخة",
  noteInsideWindow: "هذه المنطقة بلا تواريخ مؤرَّخة؛ ظهورها في نافذة العهد لا يعني تواجدها فيها.",
  legendGlyph: "◌",
};

export const mapCertaintyVocabulary: Record<MapCertainty, { label: string; dash: number[] }> = {
  precise: { label: "دقيق", dash: [] },
  approximate: { label: "تقريبي", dash: [3, 2] },
  uncertain: { label: "غير مؤكد", dash: [1, 2] },
};

export const mapRelations = ["documented_in", "lived_in", "originated_in", "associated_with", "mentioned_at", "migration"] as const;
export type MapRelation = (typeof mapRelations)[number];

export const mapRelationVocabulary: Record<MapRelation, string> = {
  documented_in: "مذكور في",
  lived_in: "أقام في",
  originated_in: "يُنسب إلى",
  associated_with: "مرتبط بـ",
  mentioned_at: "أُشير إليه في",
  migration: "مسار هجرة",
};

export const mapKinds = ["place", "association", "migration", "region"] as const;
export type MapKind = (typeof mapKinds)[number];

export const mapKindVocabulary: Record<MapKind, string> = {
  place: "موضع",
  association: "إشارة ارتباط",
  migration: "مسار هجرة",
  region: "منطقة تاريخية",
};

export function isMapStatus(value: string | undefined): value is MapStatus {
  return value !== undefined && (mapStatuses as readonly string[]).includes(value);
}

export function mapStatusCopy(value: string | undefined): MapStatusCopy {
  return isMapStatus(value) ? mapStatusVocabulary[value] : mapStatusVocabulary.unresolved;
}

export function mapStatusLabel(value: string | undefined): string {
  return mapStatusCopy(value).label;
}

export function mapCertaintyLabel(value: string | undefined): string {
  return value === "precise" || value === "approximate" || value === "uncertain"
    ? mapCertaintyVocabulary[value].label
    : "غير محدد";
}

const evidenceStatusVocabulary: Record<string, string> = {
  accepted: "مقبول",
  rejected: "مرفوض",
  unreviewed: "غير مراجع",
  needs_review: "يحتاج مراجعة",
};

export function mapEvidenceStatusLabel(value: string | undefined): string {
  if (!value) return "دون دليل مكتمل";
  return evidenceStatusVocabulary[value] ?? "حالة غير معروفة";
}

export function mapCertaintyDash(value: string | undefined): number[] {
  return value === "precise" || value === "approximate" || value === "uncertain"
    ? mapCertaintyVocabulary[value].dash
    : [2, 2];
}

export function mapRelationLabel(value: string | undefined): string {
  if (!value) return "إشارة جغرافية";
  return (mapRelationVocabulary as Record<string, string>)[value] ?? value;
}

export function mapKindLabel(value: string | undefined): string {
  return (mapKindVocabulary as Record<string, string>)[value ?? ""] ?? "إشارة";
}

/** Latin digits grouped for the monospace counts the product already uses. */
export function formatMapCount(value: number): string {
  return value.toLocaleString("en-US");
}
