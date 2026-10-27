import type {
  NameJudgeCandidateInput,
  NameJudgeConfig,
  NameJudgeResult,
  SourceTreeSuggestionCandidateSnapshot,
  SourceTreeSuggestionCandidateSource,
  SourceTreeSuggestionJudgeOutput,
} from "../types";

// The judge is a measurement and, in active mode, a separate opinion row. This
// helper does not rank, reorder, accept, or fill anything. It only turns the
// snapshot the reviewer already saw into the bounded payload the server accepts.

export const MAX_JUDGE_CANDIDATES = 12;

// Shadow sampling is deterministic per suggestion: the same suggestion is in or
// out on every reload, so an accidental double request cannot bias the sample.
// A sample of 0 or 1 is exact; values between are a hash threshold.
export function sampledForShadow(suggestionId: string, sample: number): boolean {
  if (!Number.isFinite(sample) || sample <= 0) return false;
  if (sample >= 1) return true;
  let hash = 0x811c9dc5;
  for (let index = 0; index < suggestionId.length; index += 1) {
    hash ^= suggestionId.charCodeAt(index);
    hash = Math.imul(hash, 0x01000193);
  }
  return (hash >>> 0) / 0x100000000 < sample;
}

function primarySource(candidate: SourceTreeSuggestionCandidateSnapshot): SourceTreeSuggestionCandidateSource {
  return candidate.sources[0] ?? "common";
}

export function judgeCandidatesFromSnapshot(
  snapshot: SourceTreeSuggestionCandidateSnapshot[],
): NameJudgeCandidateInput[] {
  return snapshot.slice(0, MAX_JUDGE_CANDIDATES).map((candidate, index) => ({
    value: candidate.value,
    source: primarySource(candidate),
    rank: index + 1,
  }));
}

export function judgeCandidateSignature(candidates: NameJudgeCandidateInput[]): string {
  return candidates
    .map((candidate) => `${candidate.rank}\u0001${candidate.source}\u0001${candidate.value}`)
    .join("\u0002");
}

export function judgeChoiceIsInSnapshot(
  result: NameJudgeResult,
  snapshot: SourceTreeSuggestionCandidateSnapshot[],
): boolean {
  if (result.abstained) return true;
  if (!result.choice) return false;
  return snapshot.some((candidate) => candidate.value === result.choice);
}

export interface JudgeResultSnapshot {
  suggestionId: string;
  signature: string;
  mode: "shadow" | "active";
  result: NameJudgeResult;
  latencyMs: number;
  draftAtResult: string;
}

export interface JudgeEvidenceSelection {
  result: JudgeResultSnapshot;
  used: boolean;
}

// Decide which judge result, if any, belongs beside the final decision. Shadow
// results are labels for measurement and must match the final candidate set.
// Active results are evidence only if the separate row was visible at decision
// time or the reviewer chose it; a used result may have been built from a wider
// option set, which the server keeps as bounded evidence.
export function selectJudgeEvidenceCandidate(options: {
  suggestionId: string;
  snapshotSignature: string;
  draft: string;
  current: JudgeResultSnapshot | null;
  used: JudgeResultSnapshot | null;
  usedFlag: boolean;
  snapshot: SourceTreeSuggestionCandidateSnapshot[];
}): JudgeEvidenceSelection | undefined {
  const ordered = options.usedFlag && options.used
    ? [options.used, options.current]
    : [options.current, options.used];
  for (const candidate of ordered) {
    if (!candidate || candidate.suggestionId !== options.suggestionId) continue;
    const sameSignature = candidate.signature === options.snapshotSignature;
    const wasUsed = options.usedFlag && candidate === options.used;
    const activeVisible = candidate.mode === "active" &&
      ((sameSignature && candidate.draftAtResult === options.draft) || wasUsed);
    if (candidate.mode === "shadow" && !sameSignature) continue;
    if (candidate.mode === "active" && !activeVisible) continue;
    if (!judgeChoiceIsInSnapshot(candidate.result, options.snapshot)) continue;
    return { result: candidate, used: wasUsed };
  }
  return undefined;
}

export function buildJudgeEvidence(options: {
  mode: NameJudgeConfig["mode"];
  result: NameJudgeResult;
  shown: boolean;
  used: boolean;
  latencyMs: number;
}): SourceTreeSuggestionJudgeOutput | undefined {
  if (options.mode !== "shadow" && options.mode !== "active") return undefined;
  if (options.mode === "active" && !options.shown) return undefined;
  if (options.result.provider !== "jev" && options.result.provider !== "liquid") return undefined;
  const shown = options.mode === "active";
  return {
    mode: options.mode,
    provider: options.result.provider,
    model: options.result.model,
    promptVersion: options.result.promptVersion,
    choice: options.result.abstained ? "unclear" : options.result.choice ?? "unclear",
    probabilities: options.result.probabilities,
    confidence: options.result.confidence,
    abstained: options.result.abstained,
    readable: options.result.readable,
    shown,
    used: shown && options.used,
    latencyMs: Math.max(0, Math.min(120_000, Math.round(options.latencyMs))),
  };
}
