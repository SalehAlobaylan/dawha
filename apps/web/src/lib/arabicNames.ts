import commonNames from "../../../../services/core-api/internal/sourceprocessing/name_candidate_common.json";
// The names the review screen can suggest while a person types.
//
// This is not an authority and it is not a genealogy source. It is a spelling
// aid: a short list of names that are common in Arabic and in Kuwaiti family
// charts, plus the names the reviewer has already accepted in this file. The
// machine's own alternatives are kept separate and shown first when they exist.
//
// The matching is deliberately conservative. A prefix or a one- or two-letter
// typo is a useful guess; a fuzzy match that changes the name's length or its
// consonants is not, because "محمود" and "محمد" are different people.

export type NameCandidateSource = "reader" | "ai" | "file" | "common";

export interface NameCandidate {
  value: string;
  source: NameCandidateSource;
}

export interface CountedFileName { value: string; count: number; variants?: string[]; }

export interface NameCandidateGroups {
  /** The reader's chosen text and its alternative readings, best first. */
  reader?: string[];
  /** On-demand candidates from the vision model for this one crop. */
  ai?: string[];
  /** Names already accepted or corrected in this file. */
  file?: readonly (string | CountedFileName)[];
  /** The built-in spelling aid. */
  common?: readonly string[];
}

const SOURCE_RANK: Record<NameCandidateSource, number> = {
  reader: 0,
  ai: 1,
  file: 2,
  common: 3,
};

/** Common given names and family names, kept deliberately short. */
export const COMMON_ARABIC_NAMES: readonly string[] = commonNames;

// The versioned keys are evidence. A decision snapshot without them cannot be
// reproduced after the normalizer or the candidate pool changes.
export const ARABIC_NAME_NORMALIZER_VERSION = "ar-name-keys-v1";
export const ARABIC_NAME_POOL_VERSION = "ar-name-pool-v2";

export type NameCandidateMatchKind = "canonical" | "compact" | "particle" | "skeleton" | "listed";

/** All matching keys derived from one display value. None of them rewrites it. */
export interface ArabicNameKeys {
  display: string;
  canonical: string;
  compact: string;
  particle: string;
  skeleton: string;
}

/** A ranked candidate carries the evidence a decision snapshot needs. */
export interface RankedNameCandidate extends NameCandidate {
  key: string;
  keys: ArabicNameKeys;
  sources: NameCandidateSource[];
  ranks: Partial<Record<NameCandidateSource, number>>;
  score: number;
  fileCount: number;
  matchKind: NameCandidateMatchKind;
}

const ZERO_WIDTH = /[\u200B-\u200F\u202A-\u202E\u2060\uFEFF]/g;
const ARABIC_MARKS = /[\u0610-\u061A\u064B-\u065F\u0670\u06D6-\u06ED\u0640]/g;
const NAME_SPACES = /\s+/g;
const COMPACT_SEPARATORS = /[\s\-‐‑‒–—]/g;
const PARTICLE_TOKENS = new Set(["ال", "بن", "بنت", "ابو"]);

// A dotless shape key. It is deliberately weak: it can tell the pipeline that
// two readings have the same outline, but it must never be the only reason a
// name is treated as equal. The reader's own alternatives and the file's names
// still decide what reaches the list.
const SKELETON_MAP: Record<string, string> = {
  "ا": "ا", "أ": "ا", "إ": "ا", "آ": "ا", "ٱ": "ا",
  "ب": "ٮ", "ت": "ٮ", "ث": "ٮ", "ن": "ٮ", "ي": "ٮ", "ى": "ٮ", "ئ": "ٮ",
  "ج": "ح", "ح": "ح", "خ": "ح",
  "د": "د", "ذ": "د",
  "ر": "ر", "ز": "ر",
  "س": "س", "ش": "س",
  "ص": "ص", "ض": "ص",
  "ط": "ط", "ظ": "ط",
  "ع": "ع", "غ": "ع",
  "ف": "ف", "ق": "ف",
  "ك": "ك", "ل": "ل", "م": "م", "ه": "ه", "ة": "ه",
  "و": "و", "ؤ": "و", "ء": "ء",
};

/**
 * Normalizes a name for MATCHING only. The display value is never changed.
 *
 * This is the canonical key; `arabicNameKeys` exposes the additional keys used
 * for spacing, particles, and visual shape. Keeping this function stable matters:
 * the decision ledger stores its version beside every snapshot.
 */
export function normalizeArabicName(value: string): string {
  return arabicNameKeys(value).canonical;
}

/** Derives all matching keys from one name without changing its display. */
export function arabicNameKeys(value: string): ArabicNameKeys {
  const display = displayName(value);
  const canonical = display
    .normalize("NFKC")
    .replace(ZERO_WIDTH, "")
    .replace(ARABIC_MARKS, "")
    .replace(/[أإآٱ]/g, "ا")
    .replace(/ى/g, "ي")
    .replace(/ة/g, "ه")
    .replace(/[٠-٩]/g, (digit) => String(digit.charCodeAt(0) - 0x0660))
    .replace(/[۰-۹]/g, (digit) => String(digit.charCodeAt(0) - 0x06f0))
    .replace(NAME_SPACES, " ")
    .trim();
  const compact = canonical.replace(COMPACT_SEPARATORS, "");
  return {
    display,
    canonical,
    compact,
    particle: particleKey(canonical),
    skeleton: skeletonKey(canonical),
  };
}

/**
 * Suggests names for the editor. With an empty query it offers the reader's
 * alternatives, on-demand AI candidates, and names already used in this file; it
 * does not offer the alphabetically first common names, which would be noise.
 * With a query it ranks exact, prefix, and small-typo matches.
 *
 * This keeps the small public shape used by the review screen. Use
 * `rankArabicNames` when the evidence is needed for a decision snapshot.
 */
export function suggestArabicNames(
  query: string,
  groups: NameCandidateGroups,
  limit = 8,
): NameCandidate[] {
  return rankArabicNames(query, groups, limit).map(({ value, source }) => ({ value, source }));
}

/**
 * Ranks the candidate pool with the matching keys and keeps the evidence.
 *
 * The pool is bounded at the caller's limit. The truth has to be in the pool
 * before any judge can choose it; an empty list is a valid answer.
 */
export function rankArabicNames(
  query: string,
  groups: NameCandidateGroups,
  limit = 8,
): RankedNameCandidate[] {
  if (limit < 1) return [];
  const queryKeys = arabicNameKeys(query);
  const emptyQuery = queryKeys.canonical === "";
  type Input = { value: string; count: number; occurrence: string };
  const plain = (values: readonly string[]): Input[] => values.map((value) => ({ value, count: 0, occurrence: "" }));
  const file = (groups.file ?? []).flatMap((entry, index): Input[] => typeof entry === "string"
    ? [{ value: entry, count: 1, occurrence: String(index) }]
    : Array.from(new Set([entry.value, ...(entry.variants ?? [])])).map((value) => ({ value, count: Math.max(0, Math.floor(entry.count)), occurrence: String(index) })));
  const sources: Array<[NameCandidateSource, Input[]]> = [
    ["reader", plain(groups.reader ?? [])], ["ai", plain(groups.ai ?? [])],
    ["file", file], ["common", plain(groups.common ?? COMMON_ARABIC_NAMES)],
  ];
  type MutableCandidate = RankedNameCandidate & { order: number; fileOccurrences: Set<string> };
  const byKey = new Map<string, MutableCandidate>();
  let order = 0;

  for (const [source, values] of sources) {
    if (emptyQuery && source === "common") continue;
    for (let index = 0; index < values.length; index += 1) {
      const display = displayName(values[index]?.value ?? "");
      if (!display) continue;
      const keys = arabicNameKeys(display);
      if (!keys.canonical) continue;

      let score = 0;
      let matchKind: NameCandidateMatchKind = "listed";
      let dedupeKey = keys.canonical;
      if (!emptyQuery) {
        const match = matchName(queryKeys, keys);
        if (!match) continue;
        score = match.score;
        matchKind = match.kind;
        // Canonical and compact matches merge spellings that a reviewer sees as
        // the same name with different spacing. Particle matches merge article
        // variants separately. Skeleton is never an equality key.
        dedupeKey = match.kind === "particle"
          ? `particle:${keys.particle}`
          : match.kind === "skeleton"
            ? `canonical:${keys.canonical}`
            : `compact:${keys.compact}`;
      }

      const rank = index + 1;
      const existing = byKey.get(dedupeKey);
      if (existing) {
        const alreadyHasSource = existing.sources.includes(source);
        if (!alreadyHasSource) {
          existing.sources.push(source);
          existing.sources.sort((left, right) => SOURCE_RANK[left] - SOURCE_RANK[right]);
        }
        existing.ranks[source] = Math.min(existing.ranks[source] ?? rank, rank);
        const previousScore = existing.score;
        existing.score = Math.max(existing.score, score);
        if (score > 0 && (existing.matchKind === "listed" || score > previousScore)) {
          existing.matchKind = matchKind;
        }
        if (source === "file" && !existing.fileOccurrences.has(values[index].occurrence)) {
          existing.fileOccurrences.add(values[index].occurrence);
          existing.fileCount += values[index].count;
        }
        // The file's spelling is the spelling a reviewer already chose for this
        // chart, so it wins over the machine's or the lexicon's spelling when
        // the keys agree. The machine reading is still visible above the chips.
        if (source === "file" && !alreadyHasSource) {
          existing.source = "file";
          existing.value = display;
          existing.key = keys.canonical;
          existing.keys = keys;
        } else if (SOURCE_RANK[source] < SOURCE_RANK[existing.source]) {
          existing.source = source;
          existing.value = display;
          existing.key = keys.canonical;
          existing.keys = keys;
        }
        continue;
      }

      byKey.set(dedupeKey, {
        value: display,
        source,
        key: keys.canonical,
        keys,
        sources: [source],
        ranks: { [source]: rank },
        score,
        fileCount: source === "file" ? values[index].count : 0,
        fileOccurrences: new Set(source === "file" ? [values[index].occurrence] : []),
        matchKind,
        order: order++,
      });
    }
  }

  return Array.from(byKey.values())
    .sort((left, right) =>
      right.score - left.score ||
      SOURCE_RANK[left.source] - SOURCE_RANK[right.source] ||
      right.fileCount - left.fileCount ||
      left.key.length - right.key.length ||
      (left.key < right.key ? -1 : left.key > right.key ? 1 : 0) ||
      left.order - right.order,
    )
    .slice(0, limit)
    .map((candidate) => {
      const { order, fileOccurrences, ...ranked } = candidate;
      void fileOccurrences;
      void order; // the insertion order is a sort key, not part of the result
      return ranked;
    });
}

interface NameMatch {
  score: number;
  kind: Exclude<NameCandidateMatchKind, "listed">;
}

/** A small, conservative score. Null means "not a plausible completion". */
function matchName(query: ArabicNameKeys, candidate: ArabicNameKeys): NameMatch | null {
  if (query.canonical === candidate.canonical) {
    return { score: 100, kind: "canonical" };
  }
  if (query.compact && query.compact === candidate.compact) {
    return { score: 96, kind: "compact" };
  }
  if (query.particle && query.particle === candidate.particle) {
    return { score: 94, kind: "particle" };
  }
  if (candidate.canonical.startsWith(query.canonical)) {
    return { score: boundedScore(92, candidate.canonical.length - query.canonical.length), kind: "canonical" };
  }
  if (query.compact && candidate.compact.startsWith(query.compact)) {
    return { score: boundedScore(90, candidate.compact.length - query.compact.length), kind: "compact" };
  }
  if (query.particle && candidate.particle.startsWith(query.particle)) {
    return { score: boundedScore(88, candidate.particle.length - query.particle.length), kind: "particle" };
  }
  if (query.canonical && candidate.canonical.split(" ").some((token) => token.startsWith(query.canonical))) {
    return { score: boundedScore(85, candidate.canonical.length - query.canonical.length), kind: "canonical" };
  }
  if (query.canonical.length >= 2 && candidate.canonical.includes(query.canonical)) {
    return { score: boundedScore(72, candidate.canonical.indexOf(query.canonical)), kind: "canonical" };
  }
  if (query.compact.length >= 2 && candidate.compact.includes(query.compact)) {
    return { score: boundedScore(70, candidate.compact.indexOf(query.compact)), kind: "compact" };
  }
  if (query.particle.length >= 2 && candidate.particle.includes(query.particle)) {
    return { score: boundedScore(68, candidate.particle.indexOf(query.particle)), kind: "particle" };
  }
  if (query.canonical.length >= 3 && Math.abs(query.canonical.length - candidate.canonical.length) <= 2) {
    const distance = levenshtein(query.canonical, candidate.canonical);
    if (distance <= 2) {
      return { score: 66 - distance * 8, kind: "canonical" };
    }
  }
  if (query.compact.length >= 3 && Math.abs(query.compact.length - candidate.compact.length) <= 2) {
    const distance = levenshtein(query.compact, candidate.compact);
    if (distance <= 2) {
      return { score: 60 - distance * 8, kind: "compact" };
    }
  }
  if (query.canonical.length >= 3 && Math.abs(query.skeleton.length - candidate.skeleton.length) <= 1) {
    if (query.skeleton === candidate.skeleton) {
      return { score: 58, kind: "skeleton" };
    }
    const distance = levenshtein(query.skeleton, candidate.skeleton);
    if (distance <= 1) {
      return { score: 48, kind: "skeleton" };
    }
  }
  return null;
}

function boundedScore(base: number, delta: number): number {
  return base - Math.min(20, Math.max(0, delta));
}

function displayName(value: string): string {
  return value.normalize("NFKC").replace(ZERO_WIDTH, "").replace(NAME_SPACES, " ").trim();
}

function particleKey(canonical: string): string {
  const tokens = canonical.split(" ").filter(Boolean);
  const output: string[] = [];
  for (const token of tokens) {
    if (PARTICLE_TOKENS.has(token) && output.length < 2) continue;
    output.push(token);
  }
  if (output.length === 0) return canonical;
  output[0] = output[0].length > 2 && output[0].startsWith("ال") ? output[0].slice(2) : output[0];
  return output.join(" ");
}

function skeletonKey(canonical: string): string {
  const characters: string[] = [];
  for (const character of canonical) {
    if (/\s/.test(character)) continue;
    characters.push(SKELETON_MAP[character] ?? character);
  }
  return characters.join("");
}

function levenshtein(left: string, right: string): number {
  const leftRunes = Array.from(left);
  const rightRunes = Array.from(right);
  if (leftRunes.length === 0) return rightRunes.length;
  if (rightRunes.length === 0) return leftRunes.length;
  let previous = Array.from({ length: rightRunes.length + 1 }, (_, index) => index);
  for (let leftIndex = 1; leftIndex <= leftRunes.length; leftIndex += 1) {
    const current = [leftIndex];
    for (let rightIndex = 1; rightIndex <= rightRunes.length; rightIndex += 1) {
      const substitution = previous[rightIndex - 1] + (leftRunes[leftIndex - 1] === rightRunes[rightIndex - 1] ? 0 : 1);
      current[rightIndex] = Math.min(
        previous[rightIndex] + 1,
        current[rightIndex - 1] + 1,
        substitution,
      );
    }
    previous = current;
  }
  return previous[rightRunes.length];
}
