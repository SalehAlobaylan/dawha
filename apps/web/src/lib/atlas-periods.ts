/**
 * The atlas era vocabulary: curated CE windows the chips set on the year
 * scrubber. One control, not two — a chip is a named window, and moving the
 * scrubber leaves the named era when the numbers stop matching.
 *
 * PROVENANCE AND HONEST DELAY: the plan (028-B) intended these windows to be
 * derived from a reviewed PeriodO snapshot (public-domain era datasets).
 * That derivation is deliberately not done yet — it needs its own review pass
 * over the PeriodO hierarchy; committing a machine-fetched mapping nobody
 * reviewed is exactly the provenance this archive refuses. Until that pass
 * happens, the windows below are a curated editorial mapping from the common
 * Arabic periodization of Najd and the Arabian Peninsula, and this header says
 * so. They are CE year integers on purpose, matching the map's `date` columns
 * and predicates exactly; Hijri display belongs to a later presentation layer
 * and needs no change here.
 *
 * The eras do not tile the calendar: the gaps (1258–1727, 1891–1902, 1953–now)
 * are left to the free scrubber, and a chip whose window holds no dated evidence
 * is a truthful empty read, stated as such by the inspector — never faked by
 * stretching a window to cover data.
 */
export interface AtlasEra {
  id: string;
  label: string;
  from: number;
  to: number;
}

export const atlasEras: AtlasEra[] = [
  { id: "prophetic", label: "عصر الرسالة", from: 622, to: 632 },
  { id: "rashidun", label: "الراشدون", from: 632, to: 661 },
  { id: "umayyad", label: "الأمويون", from: 661, to: 750 },
  { id: "abbasid", label: "العباسيون", from: 750, to: 1258 },
  { id: "saudi-first", label: "الدولة السعودية الأولى", from: 1727, to: 1818 },
  { id: "saudi-second", label: "الدولة السعودية الثانية", from: 1824, to: 1891 },
  { id: "unification", label: "توحيد الملك عبدالعزيز", from: 1902, to: 1953 },
];

/**
 * The era the current window names, if any. Matching is exact on both edges:
 * a hand-nudged window stops being a named era and the chip says so by
 * going dark, which is what keeps the two controls from disagreeing.
 */
export function eraOfWindow(fromYear: number, toYear: number): AtlasEra | undefined {
  return atlasEras.find((era) => era.from === fromYear && era.to === toYear);
}
