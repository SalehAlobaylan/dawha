import type { NamePreparationContext, SourceTreeSuggestionCandidateSnapshot } from "../types";
import { arabicNameKeys, rankArabicNames, type RankedNameCandidate, type NameCandidateGroups } from "./arabicNames";

// The decision snapshot is what the reviewer actually saw, not the full internal
// candidate pool. Twelve entries is enough for the ranked chips plus the
// separately shown AI row, and it keeps the ledger query small.
export const MAX_CANDIDATE_SNAPSHOT = 12;

// Shadow comparison must start from the machine reading, before a reviewer
// types a correction. Otherwise typing the answer can put it into the candidate
// pool and make both recall and the judge comparison misleading.
export function buildShadowCandidateSnapshot(
  reading: string,
  alternatives: string[],
  fileNames: NameCandidateGroups["file"],
): SourceTreeSuggestionCandidateSnapshot[] {
  return buildCandidateSnapshot(rankArabicNames(reading, {
    reader: [reading, ...alternatives].filter((name) => name.trim() !== ""),
    file: fileNames,
  }, 8));
}

// A selected candidate must be the snapshot's display value, not the reviewer's
// raw input: the server stores `selectedCandidate` only when it is one of the
// values the reviewer saw. This resolves harmless differences such as
// zero-width characters or collapsed spaces, and returns nothing when the text
// cannot be attributed to a shown candidate.
export function matchSnapshotCandidate(
  candidates: SourceTreeSuggestionCandidateSnapshot[],
  value: string,
): SourceTreeSuggestionCandidateSnapshot | undefined {
  const trimmed = value.trim();
  if (!trimmed) return undefined;
  const exact = candidates.find((candidate) => candidate.value === trimmed);
  if (exact) return exact;
  const keys = arabicNameKeys(trimmed);
  if (!keys.canonical) return undefined;
  return candidates.find((candidate) => {
    const candidateKeys = arabicNameKeys(candidate.value);
    return candidateKeys.canonical === keys.canonical || candidateKeys.compact === keys.compact;
  });
}

export function buildCandidateSnapshot(
  candidates: RankedNameCandidate[],
  extraAICandidates: string[] = [],
): SourceTreeSuggestionCandidateSnapshot[] {
  const seen = new Set<string>();
  const out: SourceTreeSuggestionCandidateSnapshot[] = [];

  const remember = (keys: ReturnType<typeof arabicNameKeys>) => {
    for (const key of [keys.canonical, keys.compact, keys.particle]) {
      if (key) seen.add(key);
    }
  };
  const alreadySeen = (keys: ReturnType<typeof arabicNameKeys>) =>
    [keys.canonical, keys.compact, keys.particle].some((key) => key !== "" && seen.has(key));

  for (const candidate of candidates) {
    if (out.length >= MAX_CANDIDATE_SNAPSHOT) break;
    const keys = candidate.keys ?? arabicNameKeys(candidate.value);
    if (!keys.canonical || alreadySeen(keys)) continue;
    remember(keys);
    out.push({
      value: candidate.value,
      key: keys.canonical,
      compactKey: keys.compact,
      particleKey: keys.particle,
      skeletonKey: keys.skeleton,
      sources: [...candidate.sources],
      ranks: { ...candidate.ranks },
      score: candidate.score,
      fileCount: candidate.fileCount,
      shown: true,
      position: out.length + 1,
    });
  }

  for (let index = 0; index < extraAICandidates.length; index += 1) {
    if (out.length >= MAX_CANDIDATE_SNAPSHOT) break;
    const value = extraAICandidates[index]?.trim() ?? "";
    if (!value) continue;
    const keys = arabicNameKeys(value);
    if (!keys.canonical || alreadySeen(keys)) continue;
    remember(keys);
    out.push({
      value,
      key: keys.canonical,
      compactKey: keys.compact,
      particleKey: keys.particle,
      skeletonKey: keys.skeleton,
      sources: ["ai"],
      ranks: { ai: index + 1 },
      score: 0,
      fileCount: 0,
      shown: true,
      position: out.length + 1,
    });
  }

  return out;
}


// This function receives only the immutable server context. It cannot consume
// the editor draft. The ranker remains the same implementation used by chips.
export function buildPreparedCandidateSnapshot(context: NamePreparationContext): SourceTreeSuggestionCandidateSnapshot[] {
  const reader = context.readers.map((entry) => entry.value);
  const snapshot = buildCandidateSnapshot(rankArabicNames(context.reading, { reader, file: context.lexicon.entries }, 8));
  const seen = new Set(snapshot.map((item) => item.compactKey));
  for (let index = 0; index < reader.length && snapshot.length < MAX_CANDIDATE_SNAPSHOT; index++) {
    const keys = arabicNameKeys(reader[index]);
    if (!keys.canonical || seen.has(keys.compact)) continue;
    seen.add(keys.compact);
    snapshot.push({ value: keys.display, key: keys.canonical, compactKey: keys.compact, particleKey: keys.particle,
      skeletonKey: keys.skeleton, sources: ["reader"], ranks: { reader: index + 1 }, score: 0,
      fileCount: 0, shown: false, position: 0 });
  }
  return snapshot;
}
