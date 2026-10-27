import { describe, expect, it } from "vitest";

import {
  buildJudgeEvidence,
  judgeCandidateSignature,
  judgeCandidatesFromSnapshot,
  judgeChoiceIsInSnapshot,
  sampledForShadow,
  selectJudgeEvidenceCandidate,
} from "./nameJudge";
import type { NameJudgeResult, SourceTreeSuggestionCandidateSnapshot } from "../types";

function snapshot(value: string, sources: SourceTreeSuggestionCandidateSnapshot["sources"] = ["reader"]): SourceTreeSuggestionCandidateSnapshot {
  return {
    value,
    key: value,
    sources,
    ranks: { [sources[0]]: 1 },
    score: 1,
    shown: true,
    position: 1,
  };
}

const result: NameJudgeResult = {
  choice: "محمد",
  abstained: false,
  probabilities: { "محمد": 0.7, "أحمد": 0.2, unclear: 0.1 },
  confidence: 0.7,
  readable: null,
  provider: "jev",
  model: "jev-pinned",
  promptVersion: "name-judge-v1",
  cacheHit: false,
};

describe("shadow sampling", () => {
  it("is exact at the endpoints and stable for the same suggestion", () => {
    expect(sampledForShadow("suggestion-1", 0)).toBe(false);
    expect(sampledForShadow("suggestion-1", 1)).toBe(true);
    expect(sampledForShadow("suggestion-1", 0.2)).toBe(sampledForShadow("suggestion-1", 0.2));
  });

  it("spreads a fifth of a large deterministic set into the sample", () => {
    let included = 0;
    for (let index = 0; index < 5000; index += 1) {
      if (sampledForShadow(`suggestion-${index}`, 0.2)) included += 1;
    }
    expect(included).toBeGreaterThan(700);
    expect(included).toBeLessThan(1300);
  });
});

describe("judge candidates", () => {
  it("uses the snapshot values, primary source, and contiguous ranks, capped at twelve", () => {
    const values = Array.from({ length: 15 }, (_, index) => snapshot(`اسم${index}`, index % 2 === 0 ? ["reader", "file"] : ["common"]));
    const candidates = judgeCandidatesFromSnapshot(values);
    expect(candidates).toHaveLength(12);
    expect(candidates[0]).toEqual({ value: "اسم0", source: "reader", rank: 1 });
    expect(candidates[1]).toEqual({ value: "اسم1", source: "common", rank: 2 });
    expect(candidates[11].rank).toBe(12);
    expect(judgeCandidateSignature(candidates)).toContain("اسم0");
  });

  it("only accepts a choice that is in the shown snapshot, and always accepts abstention", () => {
    expect(judgeChoiceIsInSnapshot(result, [snapshot("محمد")])).toBe(true);
    expect(judgeChoiceIsInSnapshot(result, [snapshot("أحمد")])).toBe(false);
    expect(judgeChoiceIsInSnapshot({ ...result, choice: null, abstained: true }, [])).toBe(true);
  });
});

describe("judge evidence selection", () => {
  const shown = [snapshot("محمد"), snapshot("أحمد")];
  const shadow = {
    suggestionId: "suggestion-1",
    signature: "sig-1",
    mode: "shadow" as const,
    result,
    latencyMs: 10,
    draftAtResult: "محمد",
  };

  it("keeps a shadow label only when the final candidate set is the one the judge saw", () => {
    const selected = selectJudgeEvidenceCandidate({
      suggestionId: "suggestion-1",
      snapshotSignature: "sig-1",
      draft: "محمد",
      current: shadow,
      used: null,
      usedFlag: false,
      snapshot: shown,
    });
    expect(selected?.result.mode).toBe("shadow");
    expect(selected?.used).toBe(false);
    expect(selectJudgeEvidenceCandidate({
      suggestionId: "suggestion-1",
      snapshotSignature: "sig-2",
      draft: "محمد",
      current: shadow,
      used: null,
      usedFlag: false,
      snapshot: shown,
    })).toBeUndefined();
  });

  it("hides an active row after the draft changes, unless the reviewer chose it", () => {
    const active = { ...shadow, mode: "active" as const };
    expect(selectJudgeEvidenceCandidate({
      suggestionId: "suggestion-1",
      snapshotSignature: "sig-1",
      draft: "أحمد",
      current: active,
      used: null,
      usedFlag: false,
      snapshot: shown,
    })).toBeUndefined();
    expect(selectJudgeEvidenceCandidate({
      suggestionId: "suggestion-1",
      snapshotSignature: "sig-1",
      draft: "أحمد",
      current: active,
      used: active,
      usedFlag: true,
      snapshot: shown,
    })?.used).toBe(true);
  });

  it("records a used result even when selecting it changed the candidate set", () => {
    const active = { ...shadow, mode: "active" as const, signature: "wider-set" };
    const selected = selectJudgeEvidenceCandidate({
      suggestionId: "suggestion-1",
      snapshotSignature: "narrower-set",
      draft: "محمد",
      current: null,
      used: active,
      usedFlag: true,
      snapshot: shown,
    });
    expect(selected?.result).toBe(active);
    expect(selected?.used).toBe(true);
  });

  it("does not record a judge choice absent from the final snapshot", () => {
    expect(selectJudgeEvidenceCandidate({
      suggestionId: "suggestion-1",
      snapshotSignature: "sig-1",
      draft: "محمد",
      current: shadow,
      used: null,
      usedFlag: false,
      snapshot: [snapshot("خالد")],
    })).toBeUndefined();
  });
});

describe("judge evidence", () => {
  it("never marks shadow mode as shown or used", () => {
    const evidence = buildJudgeEvidence({ mode: "shadow", result, shown: true, used: true, latencyMs: 42 });
    expect(evidence).toMatchObject({
      mode: "shadow",
      provider: "jev",
      model: "jev-pinned",
      promptVersion: "name-judge-v1",
      choice: "محمد",
      confidence: 0.7,
      abstained: false,
      shown: false,
      used: false,
      latencyMs: 42,
    });
  });

  it("records the local Liquid provider without rewriting it", () => {
    const evidence = buildJudgeEvidence({
      mode: "shadow",
      result: {
        ...result,
        provider: "liquid",
        model: "d1:3b",
        promptVersion: "name-judge-liquid-v1",
      },
      shown: false,
      used: false,
      latencyMs: 42,
    });
    expect(evidence).toMatchObject({
      provider: "liquid",
      model: "d1:3b",
      promptVersion: "name-judge-liquid-v1",
    });
  });

  it("drops an unknown provider rather than inventing one", () => {
    expect(buildJudgeEvidence({
      mode: "shadow",
      result: { ...result, provider: "other" },
      shown: false,
      used: false,
      latencyMs: 42,
    })).toBeUndefined();
  });

  it("marks an active row only when shown, and records explicit use", () => {
    const evidence = buildJudgeEvidence({ mode: "active", result, shown: true, used: true, latencyMs: 99 });
    expect(evidence?.shown).toBe(true);
    expect(evidence?.used).toBe(true);
    expect(buildJudgeEvidence({ mode: "active", result, shown: false, used: true, latencyMs: 99 })).toBeUndefined();
  });

  it("maps abstention to the explicit unclear evidence and clamps latency", () => {
    const evidence = buildJudgeEvidence({
      mode: "shadow",
      result: { ...result, choice: null, abstained: true },
      shown: false,
      used: false,
      latencyMs: 999_999,
    });
    expect(evidence?.choice).toBe("unclear");
    expect(evidence?.abstained).toBe(true);
    expect(evidence?.latencyMs).toBe(120_000);
  });
});
