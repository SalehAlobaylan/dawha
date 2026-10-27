import {
  AlertTriangle,
  BookOpen,
  Check,
  ChevronLeft,
  ChevronRight,
  CircleHelp,
  CircleSlash,
  FileText,
  Keyboard,
  Loader2,
  Minus,
  Plus,
  RefreshCw,
  ScanText,
  Sparkles,
  X,
} from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ApiError } from "../lib/api";
import {
  decideSourceTreeSuggestion,
  fetchNameJudgeConfig,
  fetchSourcePagePreview,
  fetchSourceSuggestionAccuracy,
  fetchSourceSuggestionCandidates,
  fetchSourceSuggestionCrop,
  fetchSourceTreeSuggestions,
  judgeSourceTreeSuggestion,
} from "../lib/api";
import {
  ARABIC_NAME_NORMALIZER_VERSION,
  ARABIC_NAME_POOL_VERSION,
  normalizeArabicName,
  rankArabicNames,
  type NameCandidateSource,
} from "../lib/arabicNames";
import { buildCandidateSnapshot, buildShadowCandidateSnapshot, matchSnapshotCandidate } from "../lib/nameReviewEvidence";
import {
  buildJudgeEvidence,
  judgeCandidateSignature,
  judgeCandidatesFromSnapshot,
  judgeChoiceIsInSnapshot,
  sampledForShadow,
  selectJudgeEvidenceCandidate,
  type JudgeResultSnapshot,
} from "../lib/nameJudge";
import type {
  NameJudgeConfig,
  SourceSuggestionAccuracy,
  SourceTreeSuggestion,
  SourceTreeSuggestionDecision,
  SourceTreeSuggestionDecisionEvidence,
  SourceTreeSuggestionInputMethod,
} from "../types";

interface SourceSuggestionReviewProps {
  fileId: string;
  sourceTitle?: string;
  onClose: () => void;
}

type QueueFilter = "all" | "unreadable" | "uncertain" | "page";
type CropState = "idle" | "loading" | "ready" | "error";

const QUEUE_LIMIT = 200;
const MAX_QUEUE_LIMIT = 200;
const ZOOM_STEPS = [1, 1.5, 2, 3, 4];
const JUDGE_DEBOUNCE_MS = 600;
const OFF_JUDGE_CONFIG: NameJudgeConfig = { mode: "off", imageEnabled: false, available: false, sample: 0 };

type JudgeResultState = JudgeResultSnapshot;

interface JudgeErrorState {
  suggestionId: string;
  signature: string;
  draftAtError: string;
}

const FILTER_LABELS: Array<{ value: QueueFilter; label: string }> = [
  { value: "all", label: "الكل" },
  { value: "unreadable", label: "بلا قراءة" },
  { value: "uncertain", label: "ثقة منخفضة" },
  { value: "page", label: "هذه الصفحة" },
];

const SOURCE_LABELS: Record<NameCandidateSource, string> = {
  reader: "من القراءة",
  ai: "من النموذج",
  file: "من الملف",
  common: "اسم شائع",
};

// The review bench.
//
// A suggestion is a place on the page with ink the reader thinks is a name. The
// bench shows that place at source resolution, beside the reader's guess and its
// alternatives, and records the reviewer's decision. The machine reading is never
// replaced: it stays in the decision record so the two can be compared later.
// That comparison is the accuracy measurement, not a claim made by this screen.
export function SourceSuggestionReview({ fileId, sourceTitle, onClose }: SourceSuggestionReviewProps) {
  const [suggestions, setSuggestions] = useState<SourceTreeSuggestion[]>([]);
  const [accuracy, setAccuracy] = useState<SourceSuggestionAccuracy | null>(null);
  const [activeID, setActiveID] = useState<string | null>(null);
  const [draft, setDraft] = useState("");
  const [filter, setFilter] = useState<QueueFilter>("all");
  const [previewURL, setPreviewURL] = useState("");
  const [previewLoaded, setPreviewLoaded] = useState(false);
  const [tightURL, setTightURL] = useState("");
  const [contextURL, setContextURL] = useState("");
  const [cropState, setCropState] = useState<CropState>("idle");
  const [cropError, setCropError] = useState("");
  const [cropAttempt, setCropAttempt] = useState(0);
  const [cropViewportSize, setCropViewportSize] = useState({ width: 0, height: 0 });
  const [cropNaturalSize, setCropNaturalSize] = useState({ width: 0, height: 0 });
  const [zoom, setZoom] = useState(1);
  const [mapZoom, setMapZoom] = useState(1);
  const [enhance, setEnhance] = useState(false);
  const [knownNames, setKnownNames] = useState<string[]>([]);
  const [aiCandidates, setAICandidates] = useState<string[]>([]);
  const [aiLoading, setAILoading] = useState(false);
  const [aiError, setAIError] = useState("");
  const [judgeConfig, setJudgeConfig] = useState<NameJudgeConfig>(OFF_JUDGE_CONFIG);
  const [judgeResult, setJudgeResult] = useState<JudgeResultState | null>(null);
  const [judgeError, setJudgeError] = useState<JudgeErrorState | null>(null);
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [refreshing, setRefreshing] = useState(false);
  const [hasMore, setHasMore] = useState(false);
  const [working, setWorking] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const [helpOpen, setHelpOpen] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);
  const dialogRef = useRef<HTMLElement>(null);
  const cropViewportRef = useRef<HTMLDivElement>(null);
  const mapViewportRef = useRef<HTMLDivElement>(null);
  const activeMapBoxRef = useRef<SVGRectElement>(null);
  const loadingMoreRef = useRef(false);
  const aiRequestRef = useRef(0);
  const judgeRequestRef = useRef(0);
  const judgeUsedRef = useRef(false);
  const judgeUsedResultRef = useRef<JudgeResultState | null>(null);
  // The judge result is valid for one active suggestion and one candidate set.
  // These refs let the debounced effect read the latest snapshot without making
  // every keystroke restart the request.
  const draftRef = useRef("");
  const judgeCandidatesRef = useRef<ReturnType<typeof judgeCandidatesFromSnapshot>>([]);
  const judgeSnapshotRef = useRef<ReturnType<typeof buildCandidateSnapshot>>([]);
  const shadowSnapshotsRef = useRef(new Map<string, ReturnType<typeof buildCandidateSnapshot>>());
  // The origin is what the reviewer did, not what the final text happens to
  // equal. Typing a candidate's spelling is still manual entry; clicking it is
  // a candidate choice. That distinction is the measurement.
  const draftOriginRef = useRef<"reader" | "candidate" | "typed">("reader");
  const startedAtRef = useRef(0);

  const active = useMemo(
    () => suggestions.find((suggestion) => suggestion.id === activeID) ?? null,
    [activeID, suggestions],
  );
  const activePage = active?.pageNumber ?? null;

  // The queue is deliberately loaded in pages. A wall chart can contain
  // thousands of names; the browser should not receive all of them to show one.
  const loadInitial = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const [queue, measured] = await Promise.all([
        fetchSourceTreeSuggestions(fileId, "suggested", QUEUE_LIMIT, 0),
        fetchSourceSuggestionAccuracy(fileId),
      ]);
      setSuggestions(queue);
      setAccuracy(measured);
      setHasMore(queue.length === QUEUE_LIMIT);
      setActiveID((current) => {
        if (current && queue.some((suggestion) => suggestion.id === current)) return current;
        return queue[0]?.id ?? null;
      });
    } catch (cause: unknown) {
      setError(errorMessage(cause));
    } finally {
      setLoading(false);
    }
  }, [fileId]);

  useEffect(() => {
    void loadInitial();
  }, [loadInitial]);

  // The judge posture is optional. A failed config request means "no judge",
  // not a failed review, so it never touches the page-level error.
  const refreshJudgeConfig = useCallback(async () => {
    try {
      setJudgeConfig(await fetchNameJudgeConfig());
    } catch {
      setJudgeConfig(OFF_JUDGE_CONFIG);
    }
  }, []);

  useEffect(() => {
    void refreshJudgeConfig();
  }, [refreshJudgeConfig]);

  useEffect(() => {
    if (judgeConfig.mode === "off" || !judgeConfig.available) {
      judgeUsedRef.current = false;
      judgeUsedResultRef.current = null;
    }
  }, [judgeConfig]);

  // The workbench is modal. Lock the page behind it, and put focus inside the
  // dialog when a phone has not focused the source editor yet.
  useEffect(() => {
    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    if (!window.matchMedia("(min-width: 981px)").matches) {
      dialogRef.current?.focus();
    }
    return () => {
      document.body.style.overflow = previousOverflow;
    };
  }, []);

  // Names already decided in this file are the most useful spelling aid for the
  // next name: the same chart repeats family names, and a reviewer's own
  // spelling is better than a generic lexicon.
  useEffect(() => {
    let live = true;
    void Promise.all([
      fetchSourceTreeSuggestions(fileId, "accepted", MAX_QUEUE_LIMIT, 0),
      fetchSourceTreeSuggestions(fileId, "corrected", MAX_QUEUE_LIMIT, 0),
    ]).then(([accepted, corrected]) => {
      if (!live) return;
      const names = [...accepted, ...corrected]
        .map((suggestion) => suggestion.decidedTextAr || suggestion.suggestedTextAr)
        .filter(Boolean);
      setKnownNames(uniqueNames(names));
    }).catch(() => {
      // A spelling aid is optional. The main queue and the source crop are not.
    });
    return () => { live = false; };
  }, [fileId]);

  const loadMore = useCallback(async () => {
    if (loadingMoreRef.current || !hasMore) return;
    loadingMoreRef.current = true;
    setLoadingMore(true);
    try {
      const next = await fetchSourceTreeSuggestions(fileId, "suggested", MAX_QUEUE_LIMIT, suggestions.length);
      setSuggestions((current) => {
        const seen = new Set(current.map((suggestion) => suggestion.id));
        return [...current, ...next.filter((suggestion) => !seen.has(suggestion.id))];
      });
      setHasMore(next.length === MAX_QUEUE_LIMIT);
    } catch {
      // Do not throw away the queue because a prefetch failed. Stop the automatic
      // retry loop; the refresh button is the deliberate way to try again.
      setHasMore(false);
      setError("تعذّر تحميل المزيد من الأسماء. يمكنك متابعة الأسماء الظاهرة أو استخدام زر التحديث.");
    } finally {
      loadingMoreRef.current = false;
      setLoadingMore(false);
    }
  }, [fileId, hasMore, suggestions.length]);

  useEffect(() => {
    if (suggestions.length > 0 && suggestions.length <= 20 && hasMore && !loadingMore) {
      void loadMore();
    }
  }, [hasMore, loadMore, loadingMore, suggestions.length]);

  const reload = useCallback(async () => {
    setRefreshing(true);
    setError("");
    void refreshJudgeConfig();
    try {
      const [queue, measured] = await Promise.all([
        fetchSourceTreeSuggestions(fileId, "suggested", QUEUE_LIMIT, 0),
        fetchSourceSuggestionAccuracy(fileId),
      ]);
      setSuggestions(queue);
      setAccuracy(measured);
      setHasMore(queue.length === QUEUE_LIMIT);
      setActiveID((current) => {
        if (current && queue.some((suggestion) => suggestion.id === current)) return current;
        return queue[0]?.id ?? null;
      });
      setMessage("حُدّثت القائمة.");
    } catch (cause: unknown) {
      setError(errorMessage(cause));
    } finally {
      setRefreshing(false);
    }
  }, [fileId, refreshJudgeConfig]);

  useEffect(() => {
    if (!activePage) {
      setPreviewURL("");
      setPreviewLoaded(false);
      return;
    }
    let live = true;
    let objectURL = "";
    setPreviewURL("");
    setPreviewLoaded(false);
    void fetchSourcePagePreview(fileId, activePage).then((blob) => {
      objectURL = URL.createObjectURL(blob);
      if (live) setPreviewURL(objectURL);
      else URL.revokeObjectURL(objectURL);
    }).catch((cause: unknown) => {
      if (live) setError(errorMessage(cause));
    });
    return () => {
      live = false;
      if (objectURL) URL.revokeObjectURL(objectURL);
    };
  }, [activePage, fileId]);

  // The crop is the part that makes this a review of the SOURCE rather than of a
  // thumbnail. It is fetched per active suggestion, at source resolution, and
  // revoked as soon as the reviewer moves on.
  useEffect(() => {
    setTightURL("");
    setContextURL("");
    setCropError("");
    if (!activeID) {
      setCropState("idle");
      return;
    }
    let live = true;
    let tightObjectURL = "";
    let contextObjectURL = "";
    setCropNaturalSize({ width: 0, height: 0 });
    setCropState("loading");
    void fetchSourceSuggestionCrop(activeID).then(({ tight, context }) => {
      tightObjectURL = URL.createObjectURL(tight);
      contextObjectURL = URL.createObjectURL(context);
      if (live) {
        setTightURL(tightObjectURL);
        setContextURL(contextObjectURL);
        setCropState("ready");
      } else {
        URL.revokeObjectURL(tightObjectURL);
        URL.revokeObjectURL(contextObjectURL);
      }
    }).catch((cause: unknown) => {
      if (live) {
        setCropState("error");
        setCropError(cropErrorMessage(cause));
      }
    });
    return () => {
      live = false;
      if (tightObjectURL) URL.revokeObjectURL(tightObjectURL);
      if (contextObjectURL) URL.revokeObjectURL(contextObjectURL);
    };
  }, [activeID, cropAttempt]);

  // The crop's "100%" is a fit-to-stage view, capped at twice the source pixels so
  // a tiny name is not blown up into a blur. Zoom then grows it from that fitted
  // size, and the stage scrolls instead of stretching the image to the column.
  useEffect(() => {
    if (cropState !== "ready") return undefined;
    const node = cropViewportRef.current;
    if (!node || typeof ResizeObserver === "undefined") return undefined;
    const measure = () => {
      setCropViewportSize({
        width: Math.max(1, node.clientWidth - 24),
        height: Math.max(1, node.clientHeight - 24),
      });
    };
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(node);
    return () => observer.disconnect();
  }, [cropState, tightURL]);

  const activeSuggestedText = active?.suggestedTextAr ?? "";
  useEffect(() => {
    setDraft(activeSuggestedText);
    draftOriginRef.current = "reader";
    startedAtRef.current = typeof performance === "undefined" ? Date.now() : performance.now();
    setError("");
    // On a phone the editor sits below the source; focusing it immediately
    // scrolls the crop the reviewer is supposed to read out of view. Keep the
    // opening view on the source and let the reviewer tap the field.
    if (activeID && window.matchMedia("(min-width: 981px)").matches) {
      const frame = requestAnimationFrame(() => inputRef.current?.focus());
      return () => cancelAnimationFrame(frame);
    }
    return undefined;
  }, [activeID, activeSuggestedText]);

  useEffect(() => {
    // A new name means the old crop's model candidates are no longer about the
    // thing on screen. Ignore any request still in flight for the previous name.
    aiRequestRef.current += 1;
    setAICandidates([]);
    setAIError("");
    setAILoading(false);
    judgeUsedRef.current = false;
    judgeUsedResultRef.current = null;
  }, [activeID]);

  const filtered = useMemo(() => {
    switch (filter) {
      case "unreadable":
        return suggestions.filter((suggestion) => !suggestion.suggestedTextAr.trim());
      case "uncertain":
        return suggestions.filter((suggestion) => suggestion.confidence < 0.5);
      case "page":
        return activePage === null
          ? suggestions
          : suggestions.filter((suggestion) => suggestion.pageNumber === activePage);
      default:
        return suggestions;
    }
  }, [activePage, filter, suggestions]);

  // A filter can hide the name the reviewer was on. Move to the first name the
  // filter actually shows rather than leaving an empty editor over a full queue.
  useEffect(() => {
    if (filtered.length === 0) return;
    if (!filtered.some((suggestion) => suggestion.id === activeID)) {
      setActiveID(filtered[0].id);
    }
  }, [activeID, filtered]);

  const activeIndex = active ? filtered.findIndex((suggestion) => suggestion.id === active.id) : -1;
  const current = activeIndex >= 0 ? active : filtered[0] ?? null;

  const pages = useMemo(
    () => Array.from(new Set(filtered.map((suggestion) => suggestion.pageNumber))).sort((left, right) => left - right),
    [filtered],
  );
  const pageSuggestions = useMemo(
    () => (current ? suggestions.filter((suggestion) => suggestion.pageNumber === current.pageNumber) : []),
    [current, suggestions],
  );

  useEffect(() => {
    if (!previewURL || !previewLoaded) return;
    const viewport = mapViewportRef.current;
    const box = activeMapBoxRef.current;
    if (!viewport || !box) return;
    const viewportRect = viewport.getBoundingClientRect();
    const boxRect = box.getBoundingClientRect();
    // Scroll only the map viewport. scrollIntoView() on the box also scrolls the
    // viewer, which pushes the source crop out of sight just after it loads.
    if (boxRect.top < viewportRect.top || boxRect.bottom > viewportRect.bottom) {
      viewport.scrollTop += boxRect.top - viewportRect.top - (viewport.clientHeight - boxRect.height) / 2;
    }
    if (boxRect.left < viewportRect.left || boxRect.right > viewportRect.right) {
      viewport.scrollLeft += boxRect.left - viewportRect.left - (viewport.clientWidth - boxRect.width) / 2;
    }
  }, [current?.id, mapZoom, previewLoaded, previewURL]);

  const readerNames = useMemo(() => {
    if (!current) return [];
    const values: string[] = [];
    if (current.suggestedTextAr.trim()) values.push(current.suggestedTextAr.trim());
    for (const alternative of current.alternatives ?? []) {
      if (alternative.trim()) values.push(alternative.trim());
    }
    return uniqueNames(values);
  }, [current]);

  const candidates = useMemo(
    () => rankArabicNames(draft, { reader: readerNames, ai: aiCandidates, file: knownNames }, 8),
    [aiCandidates, draft, knownNames, readerNames],
  );

  // A candidate matching the query is already in the ranked chips. An AI
  // candidate that does not match the current draft must still be visible: the
  // model may read a different name, and the reviewer is the one who decides
  // whether that hypothesis is useful.
  const unmatchedAICandidates = useMemo(() => {
    const shown = new Set(candidates.map((candidate) => normalizeArabicName(candidate.value)));
    return uniqueNames(aiCandidates).filter((name) => !shown.has(normalizeArabicName(name)));
  }, [aiCandidates, candidates]);

  // The judge sees exactly the bounded snapshot the reviewer can see, not the
  // full candidate pool. It is called once per stable candidate set and never on
  // every keystroke.
  const judgeSnapshot = useMemo(
    () => buildCandidateSnapshot(candidates, unmatchedAICandidates),
    [candidates, unmatchedAICandidates],
  );
  // Capture the initial candidate pool per suggestion. The interactive chips
  // can continue responding to edits, but those edits cannot become evidence
  // for the shadow comparison or leak the human's answer to the judge.
  const evidenceSnapshot = useMemo(() => {
    if (judgeConfig.mode !== "shadow" || !current) return judgeSnapshot;
    const cached = shadowSnapshotsRef.current.get(current.id);
    if (cached) return cached;
    const initial = buildShadowCandidateSnapshot(current.suggestedTextAr, current.alternatives ?? [], knownNames);
    shadowSnapshotsRef.current.set(current.id, initial);
    return initial;
  }, [current, judgeConfig.mode, judgeSnapshot, knownNames]);
  const judgeCandidates = useMemo(() => judgeCandidatesFromSnapshot(evidenceSnapshot), [evidenceSnapshot]);
  const judgeSignature = useMemo(() => judgeCandidateSignature(judgeCandidates), [judgeCandidates]);
  const activeSuggestionID = current?.id ?? null;
  draftRef.current = draft;
  judgeSnapshotRef.current = evidenceSnapshot;
  judgeCandidatesRef.current = judgeCandidates;

  useEffect(() => {
    const requestID = judgeRequestRef.current + 1;
    judgeRequestRef.current = requestID;
    setJudgeResult(null);
    setJudgeError(null);
    if (!activeSuggestionID || !judgeConfig.available || judgeConfig.mode === "off" || judgeCandidates.length === 0) {
      return undefined;
    }
    if (judgeConfig.mode === "shadow" && !sampledForShadow(activeSuggestionID, judgeConfig.sample)) {
      return undefined;
    }
    const mode = judgeConfig.mode;
    const suggestionId = activeSuggestionID;
    const draftAtRequest = draftRef.current;
    const startedAt = typeof performance === "undefined" ? Date.now() : performance.now();
    const timer = window.setTimeout(() => {
      void judgeSourceTreeSuggestion(suggestionId, judgeCandidatesRef.current).then((result) => {
        if (judgeRequestRef.current !== requestID) return;
        if (!judgeChoiceIsInSnapshot(result, judgeSnapshotRef.current)) return;
        const elapsed = typeof performance === "undefined" ? Date.now() - startedAt : performance.now() - startedAt;
        setJudgeResult({
          suggestionId,
          signature: judgeSignature,
          mode,
          result,
          latencyMs: Math.max(0, Math.min(120_000, Math.round(elapsed))),
          draftAtResult: draftAtRequest,
        });
      }).catch(() => {
        if (judgeRequestRef.current !== requestID) return;
        // Shadow failures stay invisible. Active failures are a small, quiet note
        // beside the editor, never a failed review.
        if (mode === "active") {
          setJudgeError({ suggestionId, signature: judgeSignature, draftAtError: draftAtRequest });
        }
      });
    }, JUDGE_DEBOUNCE_MS);
    return () => {
      window.clearTimeout(timer);
      if (judgeRequestRef.current === requestID) judgeRequestRef.current += 1;
    };
  }, [activeSuggestionID, judgeCandidates.length, judgeConfig, judgeSignature]);

  const rememberName = useCallback((name: string) => {
    const trimmed = name.trim();
    if (!trimmed) return;
    setKnownNames((currentNames) => uniqueNames([trimmed, ...currentNames]).slice(0, 500));
  }, []);

  const requestAICandidates = useCallback(async () => {
    if (!current || aiLoading) return;
    const requestID = aiRequestRef.current + 1;
    aiRequestRef.current = requestID;
    setAILoading(true);
    setAIError("");
    try {
      const names = await fetchSourceSuggestionCandidates(current.id);
      if (aiRequestRef.current !== requestID) return;
      setAICandidates(uniqueNames(names));
      if (names.length === 0) {
        setAIError("لم يقترح النموذج أسماء لهذه القصاصة. اكتب الاسم من الصورة أو جرّب اسماً شائعاً.");
      }
    } catch (cause: unknown) {
      if (aiRequestRef.current !== requestID) return;
      setAIError(candidateErrorMessage(cause));
    } finally {
      if (aiRequestRef.current === requestID) setAILoading(false);
    }
  }, [aiLoading, current]);

  const refreshAccuracy = useCallback(() => {
    void fetchSourceSuggestionAccuracy(fileId).then(setAccuracy).catch(() => {
      // The decisions are already recorded; a failed measurement must not look
      // like a failed save.
    });
  }, [fileId]);

  const submit = useCallback(async (
    decision: "save" | "rejected" | "deferred",
    textOverride?: string,
  ) => {
    if (!current || working) return;
    let text = (textOverride ?? draft).trim();
    let actualDecision: SourceTreeSuggestionDecision;
    let textAr = "";
    if (decision === "save") {
      if (!text) {
        setError("اكتب الاسم أولاً، أو استخدم «تأجيل» إذا لم تتبيّنه.");
        inputRef.current?.focus();
        return;
      }
      const machine = current.suggestedTextAr.trim();
      actualDecision = text === machine ? "accepted" : "corrected";
      textAr = actualDecision === "corrected" ? text : "";
    } else {
      // Rejection means this place is not a name; a deferral means it is one but
      // the reviewer is not ready to decide it. Neither carries a name.
      text = "";
      actualDecision = decision;
    }
    const nextID = nextSuggestionID(filtered, current.id);
    const candidateSnapshot = judgeConfig.mode === "shadow"
      ? evidenceSnapshot
      : buildCandidateSnapshot(candidates, unmatchedAICandidates);
    const selected = matchSnapshotCandidate(candidateSnapshot, text);
    const snapshotSignature = judgeCandidateSignature(judgeCandidatesFromSnapshot(candidateSnapshot));
    let judgeOutput: SourceTreeSuggestionDecisionEvidence["judgeOutput"];
    const selectedJudge = selectJudgeEvidenceCandidate({
      suggestionId: current.id,
      snapshotSignature,
      draft,
      current: judgeResult,
      used: judgeUsedResultRef.current,
      usedFlag: judgeUsedRef.current,
      snapshot: candidateSnapshot,
    });
    if (selectedJudge) {
      judgeOutput = buildJudgeEvidence({
        mode: selectedJudge.result.mode,
        result: selectedJudge.result.result,
        shown: selectedJudge.result.mode === "active",
        used: selectedJudge.used,
        latencyMs: selectedJudge.result.latencyMs,
      });
    }
    let inputMethod: SourceTreeSuggestionInputMethod = "none";
    let selectedCandidate = "";
    if (actualDecision === "accepted" || actualDecision === "corrected") {
      const origin = draftOriginRef.current;
      if (origin === "reader" && text === current.suggestedTextAr.trim() && selected) {
        inputMethod = "reader";
        selectedCandidate = selected.value;
      } else if (origin === "candidate" && selected) {
        inputMethod = "candidate";
        selectedCandidate = selected.value;
      } else if (origin === "typed") {
        inputMethod = "typed";
      }
      // If the origin cannot be matched to a shown candidate, the method stays
      // "none". That is a measurement gap, not a fabricated candidate choice.
    }
    const elapsed = typeof performance === "undefined" ? Date.now() - startedAtRef.current : performance.now() - startedAtRef.current;
    const durationMs = startedAtRef.current > 0
      ? Math.min(3_600_000, Math.max(0, Math.round(elapsed)))
      : undefined;
    setWorking(true);
    setError("");
    setMessage("");
    try {
      const evidence: SourceTreeSuggestionDecisionEvidence = {
        candidateSnapshot,
        selectedCandidate,
        inputMethod,
        normalizerVersion: ARABIC_NAME_NORMALIZER_VERSION,
        candidatePoolVersion: ARABIC_NAME_POOL_VERSION,
        durationMs,
      };
      if (judgeOutput) evidence.judgeOutput = judgeOutput;
      const saved = await decideSourceTreeSuggestion(current.id, actualDecision, textAr, "", evidence);
      if (actualDecision === "accepted" || actualDecision === "corrected") {
        rememberName(actualDecision === "accepted" ? current.suggestedTextAr : textAr);
      }
      setSuggestions((queue) => queue.filter((suggestion) => suggestion.id !== current.id));
      setActiveID(nextID);
      const decidedMessage = describeDecision(actualDecision, actualDecision === "accepted" ? current.suggestedTextAr : textAr);
      setMessage(saved.evidenceRecorded
        ? decidedMessage
        : `${decidedMessage} حُفظ القرار، لكن لم تُحفظ أدلة المرشحات؛ سيُحتسب ضمن القياس الناقص.`);
      refreshAccuracy();
    } catch (cause: unknown) {
      if (cause instanceof ApiError && cause.status === 409) {
        setError("سبق أن قرّر مراجع آخر هذا الاسم. حُدّثت القائمة؛ راجع الاسم التالي.");
        void reload();
      } else {
        setError(errorMessage(cause));
      }
    } finally {
      setWorking(false);
    }
  }, [candidates, current, draft, evidenceSnapshot, filtered, judgeConfig.mode, judgeResult, refreshAccuracy, reload, rememberName, unmatchedAICandidates, working]);

  const step = useCallback((delta: number) => {
    if (!current || filtered.length === 0) return;
    const index = filtered.findIndex((suggestion) => suggestion.id === current.id);
    const next = filtered[Math.min(filtered.length - 1, Math.max(0, index + delta))];
    if (next) setActiveID(next.id);
  }, [current, filtered]);

  const goToPage = useCallback((delta: number) => {
    if (!current || pages.length === 0) return;
    const index = pages.indexOf(current.pageNumber);
    const targetPage = pages[Math.min(pages.length - 1, Math.max(0, (index < 0 ? 0 : index) + delta))];
    const first = filtered.find((suggestion) => suggestion.pageNumber === targetPage);
    if (first) setActiveID(first.id);
  }, [current, filtered, pages]);

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.metaKey || event.ctrlKey || event.altKey) return;
      if (event.key === "Escape") {
        if (helpOpen) {
          setHelpOpen(false);
          return;
        }
        const target = event.target as HTMLElement | null;
        if (target && (target.tagName === "INPUT" || target.tagName === "TEXTAREA")) {
          target.blur();
          return;
        }
        onClose();
        return;
      }
      const target = event.target as HTMLElement | null;
      if (target && (target.tagName === "INPUT" || target.tagName === "TEXTAREA" || target.tagName === "SELECT" || target.isContentEditable)) {
        return;
      }
      if (event.key === "j" || event.key === "ArrowDown") {
        event.preventDefault();
        step(1);
      } else if (event.key === "k" || event.key === "ArrowUp") {
        event.preventDefault();
        step(-1);
      } else if (event.key === "/") {
        event.preventDefault();
        inputRef.current?.focus();
      } else if (event.key === "?") {
        event.preventDefault();
        setHelpOpen((open) => !open);
      } else if (event.key === "a") {
        event.preventDefault();
        if (current?.suggestedTextAr.trim()) {
          draftOriginRef.current = "reader";
          void submit("save", current.suggestedTextAr);
        } else setError("لا توجد قراءة آلية لاعتمادها هنا. اكتب الاسم من الصورة.");
      } else if (event.key === "r") {
        event.preventDefault();
        void submit("rejected");
      } else if (event.key === "d") {
        event.preventDefault();
        void submit("deferred");
      } else if (/^[1-8]$/.test(event.key)) {
        const candidate = candidates[Number(event.key) - 1];
        if (candidate) {
          event.preventDefault();
          draftOriginRef.current = "candidate";
          setDraft(candidate.value);
          inputRef.current?.focus();
        }
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [candidates, current, helpOpen, onClose, step, submit]);

  // A percentage from a handful of decisions would be an anecdote wearing a
  // measurement's clothes. The API marks the sample; the header only shows a rate
  // when that marker says the sample can carry one.
  const unchangedRate = accuracy && accuracy.measurable ? Math.round(accuracy.unchangedRate * 100) : null;
  const smallAccuracySample = accuracy !== null && accuracy.total > 0 && !accuracy.measurable;
  const allUnread = suggestions.length > 0 && suggestions.every((suggestion) => !suggestion.suggestedTextAr.trim());
  const previewURLForFallback = previewURL;
  const confidencePercent = Math.round((current?.confidence ?? 0) * 100);
  const confidenceLevel = confidencePercent >= 80 ? "high" : confidencePercent >= 45 ? "medium" : "low";
  const pagePosition = current ? Math.max(1, pages.indexOf(current.pageNumber) + 1) : 0;
  const cropFitScale = cropNaturalSize.width > 0 && cropNaturalSize.height > 0 && cropViewportSize.width > 0 && cropViewportSize.height > 0
    ? Math.min(2, cropViewportSize.width / cropNaturalSize.width, cropViewportSize.height / cropNaturalSize.height)
    : 1;
  const cropDisplayWidth = cropNaturalSize.width > 0
    ? Math.max(1, Math.round(cropNaturalSize.width * cropFitScale * zoom))
    : 0;
  const cropFitsViewport = cropNaturalSize.width > 0
    && cropNaturalSize.width * cropFitScale * zoom <= cropViewportSize.width
    && cropNaturalSize.height * cropFitScale * zoom <= cropViewportSize.height;

  return (
    <div className="name-bench-backdrop" role="presentation" onClick={onClose}>
      <section
        className="name-bench"
        role="dialog"
        aria-modal="true"
        aria-labelledby="name-bench-title"
        tabIndex={-1}
        ref={dialogRef}
        onClick={(event) => event.stopPropagation()}
      >
        <header className="name-bench-head">
          <div className="name-bench-title">
            <span className="name-bench-eyebrow">ورشة الأسماء</span>
            <h2 id="name-bench-title" title={sourceTitle ? `أسماء «${sourceTitle}»` : "مراجعة أسماء الشجرة"}>{sourceTitle ? `أسماء «${sourceTitle}»` : "مراجعة أسماء الشجرة"}</h2>
            <p>اقرأ الاسم من الأصل، واختر أو اكتب. قرارك هو السجل، وقراءة الآلة تبقى بجانبه.</p>
          </div>
          <div className="name-bench-head-side">
            <div className="name-bench-stats" aria-label="حالة المراجعة">
              <div>
                <strong>{accuracy?.total ?? 0}</strong>
                <span>قرار</span>
              </div>
              <div>
                <strong>{suggestions.length}{hasMore ? "+" : ""}</strong>
                <span>في القائمة</span>
              </div>
              <div title={smallAccuracySample ? "لا نعرض نسبة قبل 100 قرار؛ القرارات نفسها محفوظة." : undefined}>
                <strong>{unchangedRate === null ? "—" : `${unchangedRate}٪`}</strong>
                <span>دون تصحيح</span>
                {smallAccuracySample ? <em>لا نسبة بعد</em> : null}
              </div>
            </div>
            <button
              type="button"
              className="name-bench-icon-button"
              onClick={() => setHelpOpen((open) => !open)}
              aria-label="اختصارات لوحة المفاتيح"
              title="اختصارات لوحة المفاتيح"
            >
              <Keyboard size={17} />
            </button>
            <button
              type="button"
              className="name-bench-icon-button"
              onClick={() => void reload()}
              disabled={refreshing}
              aria-label="تحديث القائمة"
              title="تحديث القائمة"
            >
              <RefreshCw size={17} className={refreshing ? "name-bench-spin" : undefined} />
            </button>
            <button type="button" className="name-bench-close" onClick={onClose} aria-label="إغلاق ورشة الأسماء">
              <X size={19} />
            </button>
          </div>
        </header>

        <div className="name-bench-toolbar">
          <div className="name-bench-filters" role="tablist" aria-label="تصفية قائمة الأسماء">
            {FILTER_LABELS.map(({ value, label }) => {
              const count = countForFilter(suggestions, value, activePage);
              return (
                <button
                  key={value}
                  type="button"
                  role="tab"
                  aria-selected={filter === value}
                  className={filter === value ? "name-bench-filter is-active" : "name-bench-filter"}
                  onClick={() => setFilter(value)}
                >
                  {label}
                  <span>{count}{value === "all" && hasMore ? "+" : ""}</span>
                </button>
              );
            })}
          </div>
          <div className="name-bench-page-nav">
            <button
              type="button"
              className="name-bench-text-button"
              onClick={() => goToPage(-1)}
              disabled={pagePosition <= 1 || pages.length === 0}
              aria-label="الصفحة السابقة"
            >
              <ChevronRight size={15} /> صفحة سابقة
            </button>
            <span>{current ? `صفحة ${current.pageNumber} من ${pages.length || 1}` : "لا صفحة"}</span>
            <button
              type="button"
              className="name-bench-text-button"
              onClick={() => goToPage(1)}
              disabled={pagePosition >= pages.length || pages.length === 0}
              aria-label="الصفحة التالية"
            >
              صفحة تالية <ChevronLeft size={15} />
            </button>
          </div>
        </div>

        {allUnread && !loading ? (
          <div className="name-bench-banner" role="status">
            <ScanText size={18} />
            <p>
              لم تُنتج القراءة الآلية اسماً واضحاً في هذه القائمة. اقرأ الاسم من القصاصة واكتبه؛
              قد يكون المصدر صغيراً أو الخط زخرفياً. لا نضيف تفاصيل غير موجودة في الأصل.
            </p>
          </div>
        ) : null}

        <div className="name-bench-body">
          {loading ? (
            <div className="name-bench-state name-bench-state-wide">
              <Loader2 size={26} className="name-bench-spin" />
              <p>نجهّز قائمة الأسماء والصور…</p>
            </div>
          ) : error && suggestions.length === 0 ? (
            <div className="name-bench-state name-bench-state-wide">
              <AlertTriangle size={24} />
              <p>{error}</p>
              <button type="button" className="name-bench-primary" onClick={() => void loadInitial()}>أعد المحاولة</button>
            </div>
          ) : suggestions.length === 0 ? (
            <div className="name-bench-state name-bench-state-wide">
              <Check size={28} />
              <h3>لا توجد أسماء بانتظار المراجعة</h3>
              <p>
                {accuracy && accuracy.total > 0
                  ? `سُجّل ${accuracy.total} قراراً في هذا الملف. يمكنك العودة إلى لوحة المصدر أو بدء قراءة جديدة.`
                  : "لم يصل هذا الملف إلى ورشة الأسماء بعد، أو لم يجد القارئ مواضع أسماء فيه. ابدأ القراءة من لوحة المصدر."}
              </p>
              <button type="button" className="name-bench-primary" onClick={() => void reload()}>
                <RefreshCw size={15} /> حدّث القائمة
              </button>
            </div>
          ) : filtered.length === 0 ? (
            <div className="name-bench-state name-bench-state-wide">
              <CircleSlash size={24} />
              <p>لا توجد أسماء في هذا التصفية. جرّب «الكل» أو صفحة أخرى.</p>
            </div>
          ) : current ? (
            <>
              <section className="name-bench-viewer" aria-label="صورة الاسم من الأصل">
                <div className="name-bench-viewer-head">
                  <div>
                    <span className="name-bench-kicker">الصورة الأصلية</span>
                    <h3>الاسم في موضعه</h3>
                  </div>
                  <div className="name-bench-image-tools">
                    <button
                      type="button"
                      className={enhance ? "name-bench-tool is-active" : "name-bench-tool"}
                      onClick={() => setEnhance((value) => !value)}
                      aria-pressed={enhance}
                      title="زيادة التباين للمساعدة على الرؤية"
                    >
                      وضوح
                    </button>
                    <span className="name-bench-tool-separator" />
                    <button
                      type="button"
                      className="name-bench-tool"
                      onClick={() => setZoom((value) => ZOOM_STEPS[Math.max(0, ZOOM_STEPS.indexOf(value) - 1)])}
                      disabled={zoom <= ZOOM_STEPS[0]}
                      aria-label="تصغير القصاصة"
                    >
                      <Minus size={14} />
                    </button>
                    <span className="name-bench-zoom">{Math.round(zoom * 100)}٪</span>
                    <button
                      type="button"
                      className="name-bench-tool"
                      onClick={() => setZoom((value) => ZOOM_STEPS[Math.min(ZOOM_STEPS.length - 1, ZOOM_STEPS.indexOf(value) + 1)])}
                      disabled={zoom >= ZOOM_STEPS[ZOOM_STEPS.length - 1]}
                      aria-label="تكبير القصاصة"
                    >
                      <Plus size={14} />
                    </button>
                  </div>
                </div>

                <div className="name-bench-source-row">
                  <figure className={cropState === "ready" ? "name-bench-crop" : "name-bench-crop is-placeholder"}>
                    {cropState === "ready" && tightURL ? (
                      <div className={cropDisplayWidth > 0 && !cropFitsViewport ? "name-bench-crop-viewport is-scrollable" : "name-bench-crop-viewport"} ref={cropViewportRef}>
                        <img
                          src={tightURL}
                          alt={`قصاصة الاسم من صفحة ${current.pageNumber}`}
                          className={enhance ? "name-bench-crop-image is-enhanced" : "name-bench-crop-image"}
                          style={cropDisplayWidth > 0 ? { width: `${cropDisplayWidth}px` } : undefined}
                          onLoad={(event) => setCropNaturalSize({
                            width: event.currentTarget.naturalWidth,
                            height: event.currentTarget.naturalHeight,
                          })}
                          draggable={false}
                        />
                      </div>
                    ) : cropState === "error" ? (
                      <div className="name-bench-crop-fallback">
                        {previewURLForFallback ? (
                          <div className="name-bench-fallback-frame">
                            <img src={previewURLForFallback} alt={`صفحة ${current.pageNumber}`} />
                            <svg viewBox="0 0 1 1" preserveAspectRatio="none" aria-hidden="true">
                              <rect
                                x={current.region.x}
                                y={current.region.y}
                                width={current.region.width}
                                height={current.region.height}
                                className="name-bench-fallback-box"
                              />
                            </svg>
                          </div>
                        ) : null}
                        <p><AlertTriangle size={15} /> {cropError || "تعذّر تجهيز القصاصة من المصدر."}</p>
                        <button type="button" className="name-bench-text-button" onClick={() => setCropAttempt((value) => value + 1)}>
                          <RefreshCw size={14} /> أعد المحاولة
                        </button>
                      </div>
                    ) : (
                      <div className="name-bench-crop-loading">
                        <Loader2 size={22} className="name-bench-spin" />
                        <span>نجهّز قصاصة من الأصل…</span>
                      </div>
                    )}
                    <figcaption>
                      <span>القصاصة المكبرة من المصدر</span>
                      <span>لا نضيف تفاصيل غير موجودة</span>
                    </figcaption>
                  </figure>

                  <div className="name-bench-source-side">
                    <figure className="name-bench-context">
                      {contextURL ? (
                        <img src={contextURL} alt={`سياق الاسم في صفحة ${current.pageNumber}`} />
                      ) : (
                        <div className="name-bench-context-placeholder">السياق حول الاسم</div>
                      )}
                      <figcaption>السياق: الأسماء المجاورة تساعد على التمييز</figcaption>
                    </figure>
                    <div className="name-bench-reading">
                      <span className="name-bench-kicker">ما قرأته الآلة</span>
                      <strong>{current.suggestedTextAr || "لم تقرأ اسماً هنا"}</strong>
                      <div className="name-bench-confidence">
                        <div className="name-bench-confidence-track">
                          <span className={`is-${confidenceLevel}`} style={{ width: `${Math.max(2, confidencePercent)}%` }} />
                        </div>
                        <span>{confidencePercent}٪ · {current.viewCount} {current.viewCount === 1 ? "قراءة" : "قراءات"}</span>
                      </div>
                      <p>
                        {current.suggestedTextAr
                          ? "هذه فرضية قراءة، لا حكم على الاسم. صحّحها إن كانت قريبة."
                          : "لم تتبيّن الآلة الاسم. اكتبه من الصورة، أو اختر اسماً شائعاً."}
                      </p>
                      {current.alternatives && current.alternatives.length > 0 ? (
                        <div className="name-bench-alternatives">
                          <span>بدائل القارئ:</span>
                          {current.alternatives.map((alternative) => (
                            <button key={alternative} type="button" onClick={() => { draftOriginRef.current = "candidate"; setDraft(alternative); inputRef.current?.focus(); }}>
                              {alternative}
                            </button>
                          ))}
                        </div>
                      ) : null}
                    </div>
                  </div>
                </div>

                <div className="name-bench-map">
                  <div className="name-bench-map-head">
                    <div>
                      <span className="name-bench-kicker">خريطة الصفحة</span>
                      <h4>أين يقع هذا الاسم؟</h4>
                    </div>
                    <div className="name-bench-map-tools">
                      <button type="button" className="name-bench-tool" onClick={() => setMapZoom((value) => Math.max(1, value - 0.5))} disabled={mapZoom <= 1} aria-label="تصغير الخريطة"><Minus size={14} /></button>
                      <span>{Math.round(mapZoom * 100)}٪</span>
                      <button type="button" className="name-bench-tool" onClick={() => setMapZoom((value) => Math.min(3, value + 0.5))} disabled={mapZoom >= 3} aria-label="تكبير الخريطة"><Plus size={14} /></button>
                    </div>
                  </div>
                  <div className="name-bench-map-viewport" ref={mapViewportRef}>
                    {previewURL ? (
                      <div className="name-bench-map-frame" style={{ width: `${mapZoom * 100}%` }}>
                        <img src={previewURL} alt={`معاينة صفحة ${current.pageNumber}`} onLoad={() => setPreviewLoaded(true)} />
                        <svg viewBox="0 0 1 1" preserveAspectRatio="none" aria-hidden="true">
                          {pageSuggestions.map((suggestion) => (
                            <rect
                              key={suggestion.id}
                              ref={suggestion.id === current.id ? activeMapBoxRef : undefined}
                              x={suggestion.region.x}
                              y={suggestion.region.y}
                              width={suggestion.region.width}
                              height={suggestion.region.height}
                              className={suggestion.id === current.id ? "name-bench-map-box is-active" : "name-bench-map-box"}
                              onClick={() => { setFilter("all"); setActiveID(suggestion.id); }}
                            />
                          ))}
                        </svg>
                      </div>
                    ) : (
                      <div className="name-bench-map-empty">لم تُحمّل معاينة الصفحة بعد.</div>
                    )}
                  </div>
                  <p>انقر على مربع للانتقال إلى اسم آخر في الصفحة. المربع البرتقالي هو الاسم الحالي.</p>
                </div>
              </section>

              <aside className="name-bench-editor" aria-label="إدخال الاسم">
                <div className="name-bench-editor-head">
                  <div>
                    <span className="name-bench-kicker">الاسم {Math.max(1, activeIndex + 1)} من {filtered.length}</span>
                    <h3>{current.suggestedTextAr ? "راجع القراءة أو صحّحها" : "اكتب الاسم من الصورة"}</h3>
                  </div>
                  <span className="name-bench-position">صفحة {current.pageNumber}</span>
                </div>

                <label className="name-bench-field">
                  <span>الاسم كما تراه</span>
                  <input
                    ref={inputRef}
                    value={draft}
                    onChange={(event) => { draftOriginRef.current = "typed"; setDraft(event.target.value); if (error) setError(""); if (message) setMessage(""); }}
                    onKeyDown={(event) => {
                      if (event.key === "Enter") {
                        event.preventDefault();
                        void submit("save");
                      } else if (event.key === "Escape") {
                        event.preventDefault();
                        event.stopPropagation();
                        onClose();
                      }
                    }}
                    placeholder="اكتب الاسم هنا…"
                    dir="rtl"
                    lang="ar"
                    autoComplete="off"
                    autoCapitalize="off"
                    spellCheck={false}
                  />
                  <small>Enter يحفظ وينتقل. اختصارات الورشة تعمل عندما لا يكون حقل الاسم محدداً.</small>
                </label>

                <div className="name-bench-suggestions">
                  <div className="name-bench-section-label">
                    <span><Sparkles size={15} /> اقتراحات للمساعدة</span>
                    <small>احتمالات، ليست قراءة مؤكدة</small>
                  </div>
                  <div className="name-bench-ai-row">
                    <button
                      type="button"
                      className="name-bench-ai-button"
                      onClick={() => void requestAICandidates()}
                      disabled={aiLoading}
                      title="يرسل قصاصة الاسم وحدها إلى النموذج البصري"
                    >
                      {aiLoading ? <Loader2 size={14} className="name-bench-spin" /> : <Sparkles size={14} />}
                      {aiCandidates.length > 0 ? "اقترح مرة أخرى" : "اقترح بالذكاء الاصطناعي"}
                    </button>
                    <small>يقرأ القصاصة نفسها، ويعيد احتمالات فقط.</small>
                  </div>
                  {aiError ? <p className="name-bench-ai-error">{aiError}</p> : null}
                  {candidates.length > 0 ? (
                    <div className="name-bench-chips">
                      {candidates.map((candidate, index) => (
                        <button
                          key={`${candidate.source}-${candidate.value}`}
                          type="button"
                          className={`name-bench-chip is-${candidate.source}`}
                          onClick={() => { draftOriginRef.current = "candidate"; setDraft(candidate.value); inputRef.current?.focus(); }}
                          title={`${SOURCE_LABELS[candidate.source]} · اضغط ${index + 1}`}
                        >
                          {candidate.source === "reader" ? <ScanText size={13} /> : candidate.source === "ai" ? <Sparkles size={13} /> : candidate.source === "file" ? <FileText size={13} /> : <BookOpen size={13} />}
                          <span>{candidate.value}</span>
                          <em>{SOURCE_LABELS[candidate.source]}</em>
                        </button>
                      ))}
                    </div>
                  ) : (
                    <p className="name-bench-no-suggestion">
                      {draft.trim() ? "لا اقتراح مطابق. اكتب الاسم كما تراه، أو جرّب بداية أخرى." : "لا اقتراحات بعد. ابدأ بالكتابة أو انظر إلى بدائل القارئ."}
                    </p>
                  )}
                  {unmatchedAICandidates.length > 0 ? (
                    <div className="name-bench-alternatives name-bench-ai-suggestions">
                      <span><Sparkles size={13} /> من النموذج:</span>
                      {unmatchedAICandidates.map((name) => (
                        <button key={name} type="button" onClick={() => { draftOriginRef.current = "candidate"; setDraft(name); inputRef.current?.focus(); }} title="اقتراح من النموذج للقصاصة نفسها">
                          {name}
                        </button>
                      ))}
                    </div>
                  ) : null}
                  {judgeConfig.mode === "active" && judgeConfig.available && activeSuggestionID ? (
                    judgeResult &&
                    judgeResult.suggestionId === current.id &&
                    judgeResult.signature === judgeSignature &&
                    judgeResult.draftAtResult === draft ? (
                      <div className="name-bench-judge" role="note" aria-label="رأي النموذج">
                        <span className="name-bench-judge-label">
                          <Sparkles size={13} /> رأي النموذج
                          <small>اختيار من المرشحين، ليس اعتماداً</small>
                        </span>
                        {judgeResult.result.abstained || !judgeResult.result.choice ? (
                          <button
                            type="button"
                            className="name-bench-judge-choice is-unclear"
                            onClick={() => inputRef.current?.focus()}
                          >
                            لا مرشح واضح — اكتب الاسم من الصورة
                          </button>
                        ) : (
                          <button
                            type="button"
                            className="name-bench-judge-choice"
                            title="اختيار من بين المرشحات المعروضة فقط؛ لا يغيّر ترتيبها ولا يعتمد الاسم."
                            onClick={() => {
                              judgeUsedRef.current = true;
                              judgeUsedResultRef.current = judgeResult;
                              draftOriginRef.current = "candidate";
                              setDraft(judgeResult.result.choice ?? "");
                              inputRef.current?.focus();
                            }}
                          >
                            <span>{judgeResult.result.choice}</span>
                            <em>اختيار من القائمة</em>
                          </button>
                        )}
                      </div>
                    ) : judgeError &&
                      judgeError.suggestionId === current.id &&
                      judgeError.signature === judgeSignature &&
                      judgeError.draftAtError === draft ? (
                      <div className="name-bench-judge is-unavailable" role="note">
                        <AlertTriangle size={13} />
                        تعذّر الحصول على رأي النموذج الآن. المراجعة تعمل بدونه.
                      </div>
                    ) : null
                  ) : null}
                </div>

                <div className="name-bench-source-note">
                  <ScanText size={16} />
                  <p>
                    {knownNames.length > 0
                      ? `تتذكّر الورشة ${knownNames.length} اسماً من هذا الملف؛ تظهر أسماؤها في الاقتراحات.`
                      : "كل اسم تقبله أو تصححه يوسّع اقتراحات هذا الملف."}
                  </p>
                </div>

                {message ? <p className="name-bench-message" role="status">{message}</p> : null}
                {error ? <p className="name-bench-error" role="alert">{error}</p> : null}

                <div className="name-bench-actions">
                  <button
                    type="button"
                    className="name-bench-primary"
                    onClick={() => void submit("save")}
                    disabled={working || !draft.trim()}
                    title="Enter"
                  >
                    {working ? <Loader2 size={17} className="name-bench-spin" /> : <Check size={17} />}
                    {draft.trim() && draft.trim() === current.suggestedTextAr.trim() ? "اعتمد القراءة" : "احفظ الاسم"}
                  </button>
                  <button type="button" className="name-bench-secondary" onClick={() => void submit("deferred")} disabled={working} title="d (يخرج من القائمة الحالية)">
                    تأجيل
                  </button>
                  <button type="button" className="name-bench-danger" onClick={() => void submit("rejected")} disabled={working} title="r (هذا الموضع ليس اسماً)">
                    <CircleSlash size={15} /> رفض الموضع
                  </button>
                </div>

                <div className="name-bench-nav">
                  <button type="button" onClick={() => step(-1)} disabled={activeIndex <= 0} title="k">
                    <ChevronRight size={15} /> السابق
                  </button>
                  <button type="button" onClick={() => step(1)} disabled={activeIndex < 0 || activeIndex >= filtered.length - 1} title="j">
                    التالي <ChevronLeft size={15} />
                  </button>
                </div>

                {helpOpen ? (
                  <div className="name-bench-help">
                    <div className="name-bench-section-label">
                      <span><Keyboard size={15} /> اختصارات</span>
                      <button type="button" className="name-bench-text-button" onClick={() => setHelpOpen(false)}>إغلاق</button>
                    </div>
                    <dl>
                      <div><dt><kbd>Enter</kbd></dt><dd>حفظ الاسم والانتقال</dd></div>
                      <div><dt><kbd>j</kbd> / <kbd>k</kbd></dt><dd>الاسم التالي / السابق دون قرار</dd></div>
                      <div><dt><kbd>a</kbd></dt><dd>اعتماد قراءة الآلة كما هي</dd></div>
                      <div><dt><kbd>1</kbd>–<kbd>8</kbd></dt><dd>اختيار اقتراح</dd></div>
                      <div><dt><kbd>r</kbd> / <kbd>d</kbd></dt><dd>رفض الموضع / تأجيله</dd></div>
                      <div><dt><kbd>/</kbd></dt><dd>الانتقال إلى حقل الاسم</dd></div>
                      <div><dt><kbd>Esc</kbd></dt><dd>إغلاق الورشة</dd></div>
                    </dl>
                    <p>كل قرار نهائي. قراءة الآلة تبقى محفوظة بجانب قرارك للمقارنة.</p>
                  </div>
                ) : (
                  <p className="name-bench-shortcuts" onClick={() => setHelpOpen(true)} role="button" tabIndex={0} onKeyDown={(event) => { if (event.key === "Enter" || event.key === " ") setHelpOpen(true); }}>
                    <CircleHelp size={14} /> اختصارات: Enter حفظ · j/k تنقّل · 1–8 اقتراحات
                  </p>
                )}
              </aside>
            </>
          ) : null}
        </div>
      </section>
    </div>
  );
}

function nextSuggestionID(suggestions: SourceTreeSuggestion[], currentID: string): string | null {
  const index = suggestions.findIndex((suggestion) => suggestion.id === currentID);
  if (index < 0) return suggestions[0]?.id ?? null;
  return suggestions[index + 1]?.id ?? suggestions[index - 1]?.id ?? null;
}

function countForFilter(suggestions: SourceTreeSuggestion[], filter: QueueFilter, activePage: number | null): number {
  switch (filter) {
    case "unreadable": return suggestions.filter((suggestion) => !suggestion.suggestedTextAr.trim()).length;
    case "uncertain": return suggestions.filter((suggestion) => suggestion.confidence < 0.5).length;
    case "page": return activePage === null ? suggestions.length : suggestions.filter((suggestion) => suggestion.pageNumber === activePage).length;
    default: return suggestions.length;
  }
}

function uniqueNames(values: string[]): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const value of values) {
    const trimmed = value.trim();
    if (!trimmed) continue;
    const key = normalizeArabicName(trimmed);
    if (!key || seen.has(key)) continue;
    seen.add(key);
    out.push(trimmed);
  }
  return out;
}

function describeDecision(decision: SourceTreeSuggestionDecision, text: string): string {
  switch (decision) {
    case "accepted": return `حُفظت قراءة «${text}» كما هي.`;
    case "corrected": return `حُفظ التصحيح «${text}».`;
    case "rejected": return "رُفض الموضع؛ لم يُنشأ منه اسم.";
    default: return "أُجّل الموضع إلى مراجعة لاحقة.";
  }
}

function cropErrorMessage(cause: unknown): string {
  if (cause instanceof ApiError) {
    if (cause.status === 401 || cause.status === 403) return "لم نعد نملك صلاحية فتح صورة هذا المصدر. أعد تسجيل الدخول ثم حاول مجدداً.";
    if (cause.status === 404) return "لم يعد موضع الاسم موجوداً في هذا المصدر.";
    if (cause.status === 422) return "موضع الاسم غير صالح في المصدر؛ استخدم السياق الظاهر أو راجع الصفحة.";
    if (cause.status === 429) return "طلبت صوراً كثيرة بسرعة. انتظر قليلاً ثم أعد المحاولة.";
    if (cause.status >= 500) return "تعذّر فتح صورة المصدر الآن. أعد المحاولة بعد قليل.";
  }
  return "تعذّر تجهيز القصاصة من المصدر. أعد المحاولة.";
}

function candidateErrorMessage(cause: unknown): string {
  if (cause instanceof ApiError) {
    if (cause.status === 401 || cause.status === 403) return "انتهت الجلسة أو لا تملك صلاحية طلب الاقتراحات. أعد تسجيل الدخول.";
    if (cause.status === 404) return "لم يعد موضع الاسم موجوداً في هذا الملف.";
    if (cause.status === 429) return "طلبت اقتراحات كثيرة بسرعة. انتظر قليلاً ثم أعد المحاولة.";
    if (cause.status === 503) return "خدمة الاقتراح الذكي غير مهيأة الآن. يمكنك الاعتماد على القراءة والاقتراحات الأخرى.";
    if (cause.status === 422) return "لم يتمكن النموذج من اقتراح أسماء لهذه القصاصة. جرّب كتابة الاسم يدوياً.";
    return "تعذّر طلب اقتراحات النموذج. جرّب لاحقاً أو اكتب الاسم يدوياً.";
  }
  return "تعذّر طلب اقتراحات النموذج. جرّب لاحقاً أو اكتب الاسم يدوياً.";
}

function errorMessage(cause: unknown): string {
  if (cause instanceof ApiError) return cause.message;
  if (cause instanceof Error) return cause.message;
  return "تعذّر إكمال الطلب.";
}
