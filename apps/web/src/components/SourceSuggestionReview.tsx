import {
  AlertTriangle,
  BookOpen,
  Check,
  ChevronLeft,
  ChevronRight,
  CircleHelp,
  CircleSlash,
  Crop,
  FileText,
  GitMerge,
  History,
  Keyboard,
  Loader2,
  Minus,
  Plus,
  RefreshCw,
  ScanText,
  Scissors,
  Sparkles,
  Undo2,
  X,
} from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ApiError } from "../lib/api";
import type { SuggestionCropGeometry } from "../lib/api";
import { SourceSpotMarker } from "./SourceSpotMarker";
import {
  decideSourceTreeSuggestion,
  fetchNameJudgeConfig,
 fetchNameLexicon, beginNameReview, prepareNameCandidates, sealNameReview,
  fetchReadingCapabilities,
  fetchLocalNameReadings,
  fetchLatestLocalRereadRun,
  fetchNameReadingRun,
  rereadSourceSuggestionLocally,
  fetchSourcePagePreview,
  fetchSourceSuggestionAccuracy,
  fetchSourceSuggestionCandidates,
  fetchSourceSuggestionCrop,
  fetchSourceTreeSuggestions,
  fetchNameReviewSummary,
  fetchSourceSuggestionHistory,
  judgeSourceTreeSuggestion,
  mergeSourceTreeSuggestions,
  previewSourceSuggestionRegion,
  reopenSourceSuggestion,
  reviseSourceSuggestionRegion,
  splitSourceTreeSuggestion,
} from "../lib/api";
import {
  ARABIC_NAME_NORMALIZER_VERSION,
  ARABIC_NAME_POOL_VERSION,
  normalizeArabicName,
  rankArabicNames,
  type NameCandidateSource,
 type CountedFileName,
} from "../lib/arabicNames";
import { buildCandidateSnapshot, buildPreparedCandidateSnapshot, matchSnapshotCandidate } from "../lib/nameReviewEvidence";
import {
  buildJudgeEvidence,
  judgeCandidateSignature,
  judgeCandidatesFromSnapshot,
  judgeChoiceIsInSnapshot,
  selectJudgeEvidenceCandidate,
  type JudgeResultSnapshot,
} from "../lib/nameJudge";
import type {
  NameJudgeConfig,
 NamePreparationContext, NameCandidatePreparation,
  ReadingCapabilities,
  NameReadingRun,
  LocalNameReading,
  NameRegion,
  NameReviewSummary,
  SourceSuggestionAccuracy,
  SourceSuggestionHistory,
  SourceSuggestionRegionPreview,
  SourceTreeSuggestion,
  SourceTreeSuggestionDecision,
  SourceTreeSuggestionDecisionEvidence,
  SourceTreeSuggestionInputMethod,
} from "../types";

interface SourceSuggestionReviewProps {
  fileId: string;
  sourceTitle?: string;
  onClose: () => void;
  onContinue?: () => void;
  initialQueueMode?: "pending" | "deferred" | "history";
}

type QueueFilter = "all" | "unreadable" | "uncertain" | "page";
type QueueMode = "pending" | "deferred" | "history";
type CropState = "idle" | "loading" | "ready" | "error";
type PercentRegion = { x: number; y: number; width: number; height: number };

interface SessionCounts {
  accepted: number;
  corrected: number;
  deferred: number;
  rejected: number;
}

const EMPTY_SESSION: SessionCounts = { accepted: 0, corrected: 0, deferred: 0, rejected: 0 };

const QUEUE_MODES: Array<{ value: QueueMode; label: string }> = [
  { value: "pending", label: "بانتظار المراجعة" },
  { value: "deferred", label: "مؤجلة" },
  { value: "history", label: "السجل" },
];

const DECISION_LABELS: Record<SourceTreeSuggestionDecision, string> = {
  accepted: "اعتماد",
  corrected: "تصحيح",
  deferred: "تأجيل",
  rejected: "رفض",
};

const EVENT_LABELS: Record<string, string> = {
  reopened: "إعادة فتح",
  decision: "قرار",
  region_revised: "تعديل حدود",
  split: "تقسيم",
  merge: "دمج",
  redetected: "تجديد مواضع الأسماء",
};

const QUEUE_LIMIT = 200;
const MAX_QUEUE_LIMIT = 200;
const ZOOM_STEPS = [.25, .5, .75, 1, 1.5, 2, 3, 4];
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
export function SourceSuggestionReview({ fileId, sourceTitle, onClose, onContinue, initialQueueMode = "pending" }: SourceSuggestionReviewProps) {
  const [suggestions, setSuggestions] = useState<SourceTreeSuggestion[]>([]);
  const [accuracy, setAccuracy] = useState<SourceSuggestionAccuracy | null>(null);
  const [activeID, setActiveID] = useState<string | null>(null);
  const [draft, setDraft] = useState("");
  const [filter, setFilter] = useState<QueueFilter>("all");
  const [queueMode, setQueueMode] = useState<QueueMode>(initialQueueMode);
  const [summary, setSummary] = useState<NameReviewSummary | null>(null);
  const [sessionCounts, setSessionCounts] = useState<SessionCounts>(EMPTY_SESSION);
  const [history, setHistory] = useState<SourceSuggestionHistory | null>(null);
  const [historyLoading, setHistoryLoading] = useState(false);
  const [historyOpen, setHistoryOpen] = useState(false);
  const [historyAttempt, setHistoryAttempt] = useState(0);
  const [regionDraft, setRegionDraft] = useState<PercentRegion | null>(null);
  const [regionPreview, setRegionPreview] = useState<SourceSuggestionRegionPreview | null>(null);
  const [regionBusy, setRegionBusy] = useState(false);
  const [regionError, setRegionError] = useState("");
  const [splitOpen, setSplitOpen] = useState(false);
  const [splitRegions, setSplitRegions] = useState<[PercentRegion, PercentRegion] | null>(null);
  const [splitBusy, setSplitBusy] = useState(false);
  const [splitError, setSplitError] = useState("");
  const [mergeOpen, setMergeOpen] = useState(false);
  const [mergeSelection, setMergeSelection] = useState<string[]>([]);
  const [mergeBusy, setMergeBusy] = useState(false);
  const [mergeError, setMergeError] = useState("");
  const [localReadings, setLocalReadings] = useState<LocalNameReading[]>([]);
  const [previewURL, setPreviewURL] = useState("");
  const [previewLoaded, setPreviewLoaded] = useState(false);
  const [tightURL, setTightURL] = useState("");
  const [contextURL, setContextURL] = useState("");
  const [cropGeometry, setCropGeometry] = useState<SuggestionCropGeometry>();
  const [previewSize, setPreviewSize] = useState({ width: 0, height: 0 });
  const [cropState, setCropState] = useState<CropState>("idle");
  const [cropError, setCropError] = useState("");
  const [cropAttempt, setCropAttempt] = useState(0);
  const [cropViewportSize, setCropViewportSize] = useState({ width: 0, height: 0 });
  const [cropNaturalSize, setCropNaturalSize] = useState({ width: 0, height: 0 });
  const [zoom, setZoom] = useState<number | null>(null);
  const [mapZoom, setMapZoom] = useState(1);
  const [mapLocateTick, setMapLocateTick] = useState(0);
  const mapDragRef = useRef<{ x: number; y: number } | null>(null);
  const [enhance, setEnhance] = useState(false);
  const [knownNames, setKnownNames] = useState<CountedFileName[]>([]);
 const [reviewPreparation,setReviewPreparation]=useState<{suggestionId:string;context:NamePreparationContext|null;preparation?:NameCandidatePreparation}|null>(null);
  const [readingCapabilities, setReadingCapabilities] = useState<ReadingCapabilities | null>(null);
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
 const activePostureRef=useRef(false);
  const judgeUsedResultRef = useRef<JudgeResultState | null>(null);
  // The judge result is valid for one active suggestion and one candidate set.
  // These refs let the debounced effect read the latest snapshot without making
  // every keystroke restart the request.
  const draftRef = useRef("");
  const judgeCandidatesRef = useRef<ReturnType<typeof judgeCandidatesFromSnapshot>>([]);
  const judgeSnapshotRef = useRef<ReturnType<typeof buildCandidateSnapshot>>([]);
  const cropFileRef = useRef(fileId);
  const cropCacheRef = useRef(new Map<string, Awaited<ReturnType<typeof fetchSourceSuggestionCrop>>>());
  const localRereadButtonRef = useRef<HTMLButtonElement>(null);
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

  const refreshSummary = useCallback(async () => {
    try {
      setSummary(await fetchNameReviewSummary(fileId));
    } catch {
      // Counts are assistance, not evidence. A failed count must not hide the
      // queue or make a saved decision look unsaved.
    }
  }, [fileId]);

  // The queue is deliberately loaded in pages. A wall chart can contain
  // thousands of names; the browser should not receive all of them to show one.
  // Pending, deferred, and history are separate operational reads over the same
  // append-only ledger.
  const loadInitial = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const [queue, measured, counted] = await Promise.all([
        fetchSourceTreeSuggestions(fileId, queueStatus(queueMode), QUEUE_LIMIT, 0),
        fetchSourceSuggestionAccuracy(fileId),
        fetchNameReviewSummary(fileId).catch(() => null),
      ]);
      setSuggestions(queue);
      setAccuracy(measured);
      setSummary(counted);
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
  }, [fileId, queueMode]);

  useEffect(() => {
    void loadInitial();
  }, [loadInitial]);

  const refreshReadingCapabilities = useCallback(async () => {
    try { setReadingCapabilities(await fetchReadingCapabilities(fileId)); }
    catch { setReadingCapabilities(null); }
  }, [fileId]);

  useEffect(() => {
    let live = true;
    setReadingCapabilities(null);
    void fetchReadingCapabilities(fileId).then((value) => { if (live) setReadingCapabilities(value); })
      .catch(() => { if (live) setReadingCapabilities(null); });
    return () => { live = false; };
  }, [fileId]);

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

  // Complete counted context is optional for interactive spelling assistance.
  // The initial preparation independently pins its exact server revision.
  useEffect(() => {
    const abort = new AbortController();
    void fetchNameLexicon(fileId, abort.signal).then((lexicon) => { if (!abort.signal.aborted) setKnownNames(lexicon.entries); }).catch(() => {});
    return () => abort.abort();
  }, [fileId]);

  const loadMore = useCallback(async () => {
    if (loadingMoreRef.current || !hasMore) return;
    loadingMoreRef.current = true;
    setLoadingMore(true);
    try {
      const next = await fetchSourceTreeSuggestions(fileId, queueStatus(queueMode), MAX_QUEUE_LIMIT, suggestions.length);
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
  }, [fileId, hasMore, queueMode, suggestions.length]);

  useEffect(() => {
    if (suggestions.length > 0 && suggestions.length <= 20 && hasMore && !loadingMore) {
      void loadMore();
    }
  }, [hasMore, loadMore, loadingMore, suggestions.length]);

  const reload = useCallback(async () => {
    setRefreshing(true);
    setError("");
    void refreshJudgeConfig();
    void refreshReadingCapabilities();
    try {
      const [queue, measured, counted] = await Promise.all([
        fetchSourceTreeSuggestions(fileId, queueStatus(queueMode), QUEUE_LIMIT, 0),
        fetchSourceSuggestionAccuracy(fileId),
        fetchNameReviewSummary(fileId).catch(() => null),
      ]);
      setSuggestions(queue);
      setAccuracy(measured);
      setSummary(counted);
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
  }, [fileId, queueMode, refreshJudgeConfig, refreshReadingCapabilities]);

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
    setCropGeometry(undefined);
    setZoom(null);
    setCropError("");
    if (cropFileRef.current !== fileId) { cropFileRef.current = fileId; cropCacheRef.current.clear(); setCropState("idle"); return; }
    if (!activeID) {
      setCropState("idle");
      return;
    }
    let live = true;
    const abort = new AbortController();
    let tightObjectURL = "";
    let contextObjectURL = "";
    setCropNaturalSize({ width: 0, height: 0 });
    setCropState("loading");
    const cached = cropCacheRef.current.get(activeID);
    void (cached ? Promise.resolve(cached) : fetchSourceSuggestionCrop(activeID, abort.signal)).then(({ tight, context, review, geometry }) => {
      tightObjectURL = URL.createObjectURL(review ?? tight);
      contextObjectURL = URL.createObjectURL(context);
      if (live) {
        setTightURL(tightObjectURL);
        setContextURL(contextObjectURL);
        setCropGeometry(geometry);
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
      abort.abort();
      if (tightObjectURL) URL.revokeObjectURL(tightObjectURL);
      if (contextObjectURL) URL.revokeObjectURL(contextObjectURL);
    };
  }, [activeID, cropAttempt, fileId]);

  // Warm one adjacent crop only after the active crop is ready. Cache at most
  // the active/next pair; aborting display work never cancels a durable run.
  useEffect(() => {
    if (cropState !== "ready" || !activeID) return;
    const index = suggestions.findIndex((item) => item.id === activeID);
    const next = suggestions[index + 1];
    for (const id of cropCacheRef.current.keys()) if (id !== activeID && id !== next?.id) cropCacheRef.current.delete(id);
    if (!next || cropCacheRef.current.has(next.id)) return;
    const abort = new AbortController();
    void fetchSourceSuggestionCrop(next.id, abort.signal).then((crop) => { if (!abort.signal.aborted) cropCacheRef.current.set(next.id, crop); }).catch(() => {});
    return () => abort.abort();
  }, [activeID, cropState, suggestions]);

  // Fit never enlarges source pixels. Explicit percentages are native-pixel
  // scale, and each new selection returns to fit rather than inheriting zoom.
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
    activePostureRef.current=false;
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
  const activeSuggestionID = current?.id ?? null;
  const readOnly = Boolean(current && ((current.status && current.status !== "suggested") || current.superseded));
  const wantsHistory = readOnly || historyOpen;
  useEffect(()=>{if(judgeConfig.mode==="active")activePostureRef.current=true;},[activeID,judgeConfig.mode]);
  const preparingReview = Boolean(current && !readOnly && reviewPreparation?.suggestionId !== current.id);

  // Reset geometry and history drafts when the reviewer moves to another name.
  // Draft text is intentionally not reset here: a recoverable crop error must
  // preserve what the reviewer typed.
  useEffect(() => {
    setRegionDraft(null);
    setRegionPreview(null);
    setRegionError("");
    setSplitOpen(false);
    setSplitRegions(null);
    setSplitError("");
    setMergeOpen(false);
    setMergeSelection([]);
    setMergeError("");
    setLocalReadings([]);
    setHistoryOpen(false);
    setHistory(null);
  }, [activeSuggestionID]);

  useEffect(() => {
    if (!activeSuggestionID || !wantsHistory) {
      setHistory(null);
      setHistoryLoading(false);
      return undefined;
    }
    const suggestionId = activeSuggestionID;
    const abort = new AbortController();
    setHistoryLoading(true);
    setHistory(null);
    void fetchSourceSuggestionHistory(suggestionId, abort.signal).then((value) => {
      if (!abort.signal.aborted) {
        setHistory(value);
        setHistoryLoading(false);
      }
    }).catch(() => {
      if (!abort.signal.aborted) setHistoryLoading(false);
    });
    return () => abort.abort();
  }, [activeSuggestionID, historyAttempt, wantsHistory]);

  useEffect(() => {
    if (!activeSuggestionID) return undefined;
    if (readOnly) {
      setReviewPreparation(null);
      return undefined;
    }
    const suggestionId=activeSuggestionID;
    const abort=new AbortController();
    void (async () => {
      try {
        const context=await beginNameReview(suggestionId,abort.signal);
        if(abort.signal.aborted)return;
        let preparation=context.preparation;
        if(!preparation && context.canPrepare) preparation=await prepareNameCandidates(suggestionId,context.exposureId,buildPreparedCandidateSnapshot(context),abort.signal);
        if(abort.signal.aborted)return;
        setKnownNames(context.lexicon.entries);
        setSuggestions((queue)=>queue.map((item)=>item.id===suggestionId?{...item,suggestedTextAr:context.reading,alternatives:context.alternatives}:item));
        setReviewPreparation({suggestionId,context,preparation});
        startedAtRef.current=typeof performance === "undefined" ? Date.now() : performance.now();
      } catch {
        if(abort.signal.aborted)return;
        // An evidence failure releases manual review and stays a genuine gap.
        // It never restarts preparation after editing has begun.
        void sealNameReview(suggestionId).catch(()=>{});
        setReviewPreparation({suggestionId,context:null});
      }
    })();
    return ()=>abort.abort();
  },[activeSuggestionID, readOnly]);


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
  }, [current?.id, mapZoom, mapLocateTick, previewLoaded, previewURL, previewSize]);

  const readerNames = useMemo(() => {
    if (!current) return [];
    const values: string[] = [];
    if (current.suggestedTextAr.trim()) values.push(current.suggestedTextAr.trim());
    for (const alternative of current.alternatives ?? []) {
      if (alternative.trim()) values.push(alternative.trim());
    }
    if (reviewPreparation?.suggestionId === current.id) for (const entry of reviewPreparation.context?.readers ?? []) values.push(entry.value);
    return uniqueNames(values);
  }, [current, reviewPreparation]);

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
  // Interaction evidence may respond to typing. Initial evaluation evidence is
  // the immutable preparation reference, in every review mode.
  const evidenceSnapshot = judgeSnapshot;
  const judgeCandidates = useMemo(() => judgeCandidatesFromSnapshot(evidenceSnapshot), [evidenceSnapshot]);
  const judgeSignature = useMemo(() => judgeCandidateSignature(judgeCandidates), [judgeCandidates]);
  draftRef.current = draft;
  judgeSnapshotRef.current = evidenceSnapshot;
  judgeCandidatesRef.current = judgeCandidates;

  useEffect(() => {
    const requestID = judgeRequestRef.current + 1;
    judgeRequestRef.current = requestID;
    setJudgeResult(null);
    setJudgeError(null);
    if (!activeSuggestionID || !judgeConfig.available || judgeConfig.mode !== "active" || preparingReview || judgeCandidates.length === 0) {
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
  }, [activeSuggestionID, judgeCandidates.length, judgeConfig, judgeSignature, preparingReview]);

  const rememberName = useCallback(() => {
    void fetchNameLexicon(fileId).then((lexicon)=>setKnownNames(lexicon.entries)).catch(()=>{});
  }, [fileId]);

  const requestAICandidates = useCallback(async () => {
    if (!current || preparingReview || aiLoading || !readingCapabilities?.hostedReader.ready) return;
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
  }, [aiLoading, current, readingCapabilities, preparingReview]);

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
    if (!current || readOnly || working || preparingReview) return;
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
    const candidateSnapshot = buildCandidateSnapshot(candidates, unmatchedAICandidates);
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
        judgeWasVisible: activePostureRef.current,
        evidenceSchemaVersion: "name-review-v2",
        preparationId: reviewPreparation?.suggestionId===current.id ? reviewPreparation.preparation?.id : undefined,
        preparationSignature: reviewPreparation?.suggestionId===current.id ? reviewPreparation.preparation?.signature : undefined,
        candidateSnapshot,
        selectedCandidate,
        inputMethod,
        normalizerVersion: ARABIC_NAME_NORMALIZER_VERSION,
        candidatePoolVersion: ARABIC_NAME_POOL_VERSION,
        durationMs,
        expectedRevision: current.reviewRevision,
        expectedGeometryRevision: current.geometryRevision,
      };
      if (judgeOutput) evidence.judgeOutput = judgeOutput;
      const saved = await decideSourceTreeSuggestion(current.id, actualDecision, textAr, "", evidence);
      if (actualDecision === "accepted" || actualDecision === "corrected") {
        rememberName();
      }
      setSessionCounts((counts) => ({ ...counts, [actualDecision]: counts[actualDecision] + 1 }));
      setSuggestions((queue) => queue.filter((suggestion) => suggestion.id !== current.id));
      setActiveID(nextID);
      void refreshSummary();
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
  }, [candidates, current, draft, filtered, judgeResult, refreshAccuracy, refreshSummary, reload, rememberName, readOnly, unmatchedAICandidates, working, preparingReview, reviewPreparation]);

  const step = useCallback((delta: number) => {
    if (!current || filtered.length === 0) return;
    const index = filtered.findIndex((suggestion) => suggestion.id === current.id);
    const next = filtered[Math.min(filtered.length - 1, Math.max(0, index + delta))];
    if (next) setActiveID(next.id);
  }, [current, filtered]);

  const reopenCurrent = useCallback(async () => {
    if (!current || working || !history?.canReopen) return;
    setWorking(true);
    setError("");
    setMessage("");
    try {
      await reopenSourceSuggestion(current.id, current.reviewRevision);
      setHistoryOpen(false);
      setMessage("أُعيد الموضع إلى قائمة الانتظار. القرار السابق محفوظ في السجل.");
      setQueueMode("pending");
      void refreshSummary();
    } catch (cause: unknown) {
      if (cause instanceof ApiError && cause.status === 409) {
        setError("تغيّر هذا الموضع في جلسة أخرى. حُدّث السجل؛ أعد المحاولة من النسخة الظاهرة.");
        void reload();
      } else {
        setError(errorMessage(cause));
      }
    } finally {
      setWorking(false);
    }
  }, [current, history?.canReopen, refreshSummary, reload, working]);

  const openRegionEditor = useCallback(() => {
    if (!current || readOnly) return;
    setRegionDraft(regionToPercent(current.region));
    setMapZoom(4);
    setMapLocateTick((value) => value + 1);
    setRegionPreview(null);
    setRegionError("");
    setSplitOpen(false);
    setMergeOpen(false);
  }, [current, readOnly]);

  const updateRegionField = useCallback((field: keyof PercentRegion, value: number) => {
    setRegionDraft((draft) => (draft ? { ...draft, [field]: value } : draft));
    setRegionPreview(null);
  }, []);

  const previewRegion = useCallback(async () => {
    if (!current || !regionDraft || regionBusy) return;
    const validation = validatePercentRegion(regionDraft);
    if (validation) {
      setRegionError(validation);
      return;
    }
    setRegionBusy(true);
    setRegionError("");
    try {
      const preview = await previewSourceSuggestionRegion(current.id, {
        expectedGeometryRevision: current.geometryRevision,
        region: percentToRegion(regionDraft),
      });
      setRegionPreview(preview);
    } catch (cause: unknown) {
      if (cause instanceof ApiError && cause.status === 409) {
        setRegionError("تغيّر موضع الاسم في جلسة أخرى. حُدّثت القائمة؛ راجع الموضع الحالي.");
        void reload();
      } else {
        setRegionError(errorMessage(cause));
      }
    } finally {
      setRegionBusy(false);
    }
  }, [current, regionBusy, regionDraft, reload]);

  const confirmRegion = useCallback(async () => {
    if (!current || !regionDraft || !regionPreview || regionBusy) return;
    if (!sameNameRegion(percentToRegion(regionDraft), regionPreview.region)) {
      setRegionError("غيّرت الحدود بعد المعاينة. أعد المعاينة قبل الحفظ.");
      return;
    }
    setRegionBusy(true);
    setRegionError("");
    try {
      const updated = await reviseSourceSuggestionRegion(current.id, {
        expectedGeometryRevision: current.geometryRevision,
        region: regionPreview.region,
        coordinateSpace: regionPreview.coordinateSpace,
        analysisDpi: regionPreview.analysisDpi,
        sourceChecksum: regionPreview.sourceChecksum,
        frameWidth: regionPreview.frameWidth,
        frameHeight: regionPreview.frameHeight,
      });
      setSuggestions((queue) => queue.map((item) => item.id === updated.id ? updated : item));
      cropCacheRef.current.delete(current.id);
      setCropAttempt((value) => value + 1);
      setRegionDraft(null);
      setRegionPreview(null);
      setMessage("حُدّثت حدود القراءة. القصاصة القديمة وقراءاتها محفوظة في السجل.");
      if (historyOpen) setHistoryAttempt((value) => value + 1);
      void refreshSummary();
    } catch (cause: unknown) {
      if (cause instanceof ApiError && cause.status === 409) {
        setRegionError("تغيّر هذا الموضع في جلسة أخرى. حُدّثت القائمة؛ أعد فتح محرر الحدود.");
        void reload();
      } else {
        setRegionError(errorMessage(cause));
      }
    } finally {
      setRegionBusy(false);
    }
  }, [current, historyOpen, refreshSummary, regionBusy, regionDraft, regionPreview, reload]);

  const openSplit = useCallback(() => {
    if (!current || readOnly) return;
    const parent = current.region;
    const half = { ...parent, width: parent.width / 2 };
    setSplitRegions([
      regionToPercent(half),
      regionToPercent({ ...parent, x: parent.x + parent.width / 2, width: parent.width / 2 }),
    ]);
    setSplitError("");
    setSplitOpen(true);
    setRegionDraft(null);
    setMergeOpen(false);
  }, [current, readOnly]);

  const updateSplitField = useCallback((index: 0 | 1, field: keyof PercentRegion, value: number) => {
    setSplitRegions((regions) => {
      if (!regions) return regions;
      const next: [PercentRegion, PercentRegion] = [{ ...regions[0] }, { ...regions[1] }];
      next[index][field] = value;
      return next;
    });
  }, []);

  const submitSplit = useCallback(async () => {
    if (!current || !splitRegions || splitBusy) return;
    const [first, second] = splitRegions;
    for (const region of [first, second]) {
      const validation = validatePercentRegion(region);
      if (validation) {
        setSplitError(validation);
        return;
      }
    }
    const parent = regionToPercent(current.region);
    if (!percentRegionContains(parent, first) || !percentRegionContains(parent, second)) {
      setSplitError("يجب أن يقع كل جزء داخل حدود الموضع الأصلي.");
      return;
    }
    if (percentRegionsOverlap(first, second)) {
      setSplitError("لا يجوز أن يتداخل الجزآن؛ فسيتكرر الاسم نفسه مرتين.");
      return;
    }
    setSplitBusy(true);
    setSplitError("");
    try {
      const result = await splitSourceTreeSuggestion(current.id, {
        expectedReviewRevision: current.reviewRevision,
        expectedGeometryRevision: current.geometryRevision,
        regions: [percentToRegion(first), percentToRegion(second)],
      });
      setSplitOpen(false);
      setSplitRegions(null);
      await loadInitial();
      setMessage(`قُسّم الموضع إلى ${result.children.length} موضعين. الموضع الأصلي محفوظ في السجل.`);
      void refreshSummary();
    } catch (cause: unknown) {
      if (cause instanceof ApiError && cause.status === 409) {
        setSplitError("تغيّر هذا الموضع في جلسة أخرى. حُدّثت القائمة؛ أعد المحاولة.");
        void reload();
      } else {
        setSplitError(errorMessage(cause));
      }
    } finally {
      setSplitBusy(false);
    }
  }, [current, loadInitial, refreshSummary, reload, splitBusy, splitRegions]);

  const mergeCandidates = useMemo(
    () => pageSuggestions.filter((suggestion) => suggestion.status === "suggested" && !suggestion.superseded),
    [pageSuggestions],
  );
  const mergeRegion = useMemo(() => {
    const selected = mergeCandidates.filter((suggestion) => mergeSelection.includes(suggestion.id));
    return selected.length > 0 ? unionNameRegions(selected.map((suggestion) => suggestion.region)) : null;
  }, [mergeCandidates, mergeSelection]);

  const openMerge = useCallback(() => {
    if (!current || readOnly || mergeCandidates.length < 2) return;
    setMergeSelection([current.id]);
    setMergeError("");
    setMergeOpen(true);
    setRegionDraft(null);
    setSplitOpen(false);
  }, [current, mergeCandidates.length, readOnly]);

  const toggleMerge = useCallback((id: string) => {
    setMergeSelection((selection) => selection.includes(id)
      ? selection.filter((value) => value !== id)
      : [...selection, id]);
  }, []);

  const submitMerge = useCallback(async () => {
    if (!current || mergeBusy) return;
    const selected = mergeCandidates.filter((suggestion) => mergeSelection.includes(suggestion.id));
    if (selected.length < 2) {
      setMergeError("اختر موضعين على الأقل من الصفحة نفسها.");
      return;
    }
    setMergeBusy(true);
    setMergeError("");
    try {
      const result = await mergeSourceTreeSuggestions(selected[0].id, {
        members: selected.map((suggestion) => ({
          id: suggestion.id,
          expectedReviewRevision: suggestion.reviewRevision,
          expectedGeometryRevision: suggestion.geometryRevision,
        })),
        region: unionNameRegions(selected.map((suggestion) => suggestion.region)),
      });
      setMergeOpen(false);
      setMergeSelection([]);
      await loadInitial();
      setMessage(`دُمجت ${result.superseded.length} مواضع في موضع واحد. المواضع الأصلية محفوظة في السجل.`);
      void refreshSummary();
    } catch (cause: unknown) {
      if (cause instanceof ApiError && cause.status === 409) {
        setMergeError("تغيّر أحد المواضع في جلسة أخرى. حُدّثت القائمة؛ أعد الاختيار.");
        void reload();
      } else {
        setMergeError(errorMessage(cause));
      }
    } finally {
      setMergeBusy(false);
    }
  }, [current, loadInitial, mergeBusy, mergeCandidates, mergeSelection, refreshSummary, reload]);

  const assistance = useMemo<ReaderAssistance | null>(() => {
    if (!current || readOnly) return null;
    if (cropGeometry?.fragment) return {
      tone: "caution",
      title: "موضع يحتاج تصحيحاً",
      detail: "الحدود تلتقط جزءاً صغيراً جداً. حدّد الاسم كاملاً على الخريطة أو ارفض الموضع إن كان رسماً.",
      action: "region",
      actionLabel: "عدّل الحدود",
    };
    return readerAssistance(current, cropState, readingCapabilities, localReadings);
  }, [current, readOnly, cropGeometry?.fragment, cropState, readingCapabilities, localReadings]);
  const runAssistance = useCallback(() => {
    if (!assistance) return;
    if (assistance.action === "region") {
      openRegionEditor();
    } else if (assistance.action === "reread") {
      localRereadButtonRef.current?.click();
    } else {
      inputRef.current?.focus();
    }
  }, [assistance, openRegionEditor]);

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
      if (preparingReview && !["j","k","ArrowDown","ArrowUp"].includes(event.key)) return;
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
        if (!readOnly && current?.suggestedTextAr.trim()) {
          draftOriginRef.current = "reader";
          void submit("save", current.suggestedTextAr);
        } else if (!readOnly) setError("لا توجد قراءة آلية لاعتمادها هنا. اكتب الاسم من الصورة.");
      } else if (event.key === "r") {
        event.preventDefault();
        if (!readOnly) void submit("rejected");
      } else if (event.key === "d") {
        event.preventDefault();
        if (!readOnly) void submit("deferred");
      } else if (/^[1-8]$/.test(event.key)) {
        const candidate = candidates[Number(event.key) - 1];
        if (candidate && !readOnly) {
          event.preventDefault();
          draftOriginRef.current = "candidate";
          setDraft(candidate.value);
          inputRef.current?.focus();
        }
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [candidates, current, helpOpen, onClose, readOnly, step, submit, preparingReview]);

  // A percentage from a handful of decisions would be an anecdote wearing a
  // measurement's clothes. The API marks the sample; the header only shows a rate
  // when that marker says the sample can carry one.
  const unchangedRate = accuracy && accuracy.measurable ? Math.round(accuracy.unchangedRate * 100) : null;
  const smallAccuracySample = accuracy !== null && accuracy.total > 0 && !accuracy.measurable;
  const allUnread = suggestions.length > 0 && suggestions.every((suggestion) => !suggestion.suggestedTextAr.trim());
  const pendingRemaining = summary?.pending ?? (queueMode === "pending" ? suggestions.length : null);
  const modeCount = (mode: QueueMode): number | null => {
    if (mode === "pending") return summary?.pending ?? (queueMode === "pending" ? suggestions.length : null);
    if (mode === "deferred") return summary?.deferred ?? (queueMode === "deferred" ? suggestions.length : null);
    return summary ? Math.max(0, summary.total - summary.pending) : (queueMode === "history" ? suggestions.length : null);
  };
  const previewURLForFallback = preparingReview ? "" : previewURL;
  const confidencePercent = Math.round((current?.confidence ?? 0) * 100);
  const confidenceLevel = confidencePercent >= 80 ? "high" : confidencePercent >= 45 ? "medium" : "low";
  const pagePosition = current ? Math.max(1, pages.indexOf(current.pageNumber) + 1) : 0;
  const cropFitScale = cropNaturalSize.width > 0 && cropNaturalSize.height > 0 && cropViewportSize.width > 0 && cropViewportSize.height > 0
    ? Math.min(1, cropViewportSize.width / cropNaturalSize.width, cropViewportSize.height / cropNaturalSize.height)
    : 1;
  const cropScale = zoom ?? cropFitScale;
  const cropDisplayWidth = cropNaturalSize.width > 0
    ? Math.max(1, Math.round(cropNaturalSize.width * cropScale))
    : 0;
  const cropFitsViewport = cropNaturalSize.width > 0
    && cropNaturalSize.width * cropScale <= cropViewportSize.width
    && cropNaturalSize.height * cropScale <= cropViewportSize.height;

  useEffect(() => {
    const viewport = cropViewportRef.current;
    const frame = viewport?.querySelector(".name-bench-crop-frame");
    if (!viewport || !frame || !cropNaturalSize.width) return;
    if (cropFitsViewport) { viewport.scrollLeft = 0; viewport.scrollTop = 0; return; }
    const region = cropGeometry?.reviewRegion ?? cropGeometry?.tightRegion;
    const x = region ? region.x + region.width / 2 : .5;
    const y = region ? region.y + region.height / 2 : .5;
    const bounds = frame.getBoundingClientRect();
    const view = viewport.getBoundingClientRect();
    viewport.scrollLeft += bounds.left + x * bounds.width - view.left - viewport.clientWidth / 2;
    viewport.scrollTop += bounds.top + y * bounds.height - view.top - viewport.clientHeight / 2;
  }, [cropScale, cropFitsViewport, cropGeometry, cropNaturalSize, cropViewportSize]);

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
            {onContinue ? <button className="secondary-button" type="button" onClick={onContinue}>الوصلات والشجرة</button> : null}
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
          <div className="name-bench-queue-modes" role="tablist" aria-label="حالة قائمة الأسماء">
            {QUEUE_MODES.map(({ value, label }) => (
              <button
                key={value}
                type="button"
                role="tab"
                aria-selected={queueMode === value}
                className={queueMode === value ? "name-bench-queue-mode is-active" : "name-bench-queue-mode"}
                onClick={() => {
                  setQueueMode(value);
                  setFilter("all");
                  setActiveID(null);
                  setMessage("");
                }}
              >
                {label}
                <span>{modeCount(value) ?? "—"}</span>
              </button>
            ))}
          </div>
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

        <div className="name-bench-session" role="status" aria-label="ملخص الجلسة">
          <span className="name-bench-session-label">هذه الجلسة</span>
          <span>اعتماد <strong>{sessionCounts.accepted}</strong></span>
          <span>تصحيح <strong>{sessionCounts.corrected}</strong></span>
          <span>تأجيل <strong>{sessionCounts.deferred}</strong></span>
          <span>رفض <strong>{sessionCounts.rejected}</strong></span>
          <span className="name-bench-session-remaining">المتبقي <strong>{pendingRemaining ?? "—"}</strong></span>
        {hasMore ? <div className="name-bench-load-more"><span>المعروض {suggestions.length.toLocaleString("ar-SA")} من {modeCount(queueMode)?.toLocaleString("ar-SA") ?? "المزيد من"} الأسماء. التصفية تخص الأسماء المحمّلة.</span><button className="secondary-button" type="button" disabled={loadingMore || loading} onClick={() => void loadMore()}>{loadingMore ? "جارٍ تحميل المزيد…" : "تحميل المزيد من الأسماء"}</button></div> : null}
        </div>



        {allUnread && !loading && queueMode === "pending" ? (
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
              <h3>{queueMode === "deferred" ? "لا توجد مواضع مؤجلة" : queueMode === "history" ? "لا يوجد سجل بعد" : "لا توجد أسماء بانتظار المراجعة"}</h3>
              <p>
                {queueMode === "deferred"
                  ? "كل موضع أجّلته سيظهر هنا بعد إعادة التحميل. يمكنك إعادة فتحه لاحقاً."
                  : queueMode === "history"
                    ? "ستظهر هنا القرارات السابقة، والمواضع التي أُعيد فتحها أو استُبدلت بتقسيم أو دمج."
                    : accuracy && accuracy.total > 0
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
                      onClick={() => setZoom([...ZOOM_STEPS].reverse().find((value) => value < cropScale) ?? ZOOM_STEPS[0])}
                      disabled={cropScale <= ZOOM_STEPS[0]}
                      aria-label="تصغير القصاصة"
                    >
                      <Minus size={14} />
                    </button>
                    <span className="name-bench-zoom" aria-live="polite">{Math.round(cropScale * 100)}٪</span>
                    <button
                      type="button"
                      className="name-bench-tool"
                      onClick={() => setZoom(ZOOM_STEPS.find((value) => value > cropScale) ?? ZOOM_STEPS[ZOOM_STEPS.length - 1])}
                      disabled={cropScale >= ZOOM_STEPS[ZOOM_STEPS.length - 1]}
                      aria-label="تكبير القصاصة"
                    >
                      <Plus size={14} />
                    </button>
                    <button type="button" className="name-bench-tool" onClick={() => setZoom(null)} aria-pressed={zoom === null}>ملاءمة</button>
                  </div>
                </div>

                <div className="name-bench-source-row">
                  <figure className={cropState === "ready" ? "name-bench-crop" : "name-bench-crop is-placeholder"}>
                    {!preparingReview && cropState === "ready" && tightURL ? (
                      <div className={cropDisplayWidth > 0 && !cropFitsViewport ? "name-bench-crop-viewport is-scrollable" : "name-bench-crop-viewport"} ref={cropViewportRef}>
                        <div className="name-bench-crop-frame" style={cropDisplayWidth > 0 ? { width: `${cropDisplayWidth}px` } : undefined}><img
                          src={tightURL}
                          alt={`قصاصة الاسم من صفحة ${current.pageNumber}`}
                          className={enhance ? "name-bench-crop-image is-enhanced" : "name-bench-crop-image"}
                          style={{ width: "100%" }}
                          onLoad={(event) => setCropNaturalSize({
                            width: event.currentTarget.naturalWidth,
                            height: event.currentTarget.naturalHeight,
                          })}
                          draggable={false}
                        />
                        {cropGeometry ? <SourceSpotMarker region={cropGeometry.reviewRegion ?? cropGeometry.tightRegion} /> : null}
                        </div>
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
                      <span>{cropGeometry?.fragment ? "جوار الموضع · الحدود الأصلية بالبرتقالي" : "القصاصة من المصدر"}</span>
                      <span>{cropGeometry ? `${Math.round(current.region.width * cropGeometry.pageWidth)} × ${Math.round(current.region.height * cropGeometry.pageHeight)} بكسل` : "التكبير لا يضيف تفاصيل"}</span>
                    </figcaption>
                  </figure>

                  <div className="name-bench-source-side">
                    <figure className="name-bench-context">
                      {!preparingReview && contextURL ? (
                        <div className="name-bench-context-frame"><img src={contextURL} alt={`سياق الاسم في صفحة ${current.pageNumber}`} />
                          {cropGeometry ? <SourceSpotMarker region={cropGeometry.contextRegion} target /> : null}
                        </div>
                      ) : (
                        <div className="name-bench-context-placeholder">السياق حول الاسم</div>
                      )}
                      <figcaption>الموضع المحدد داخل السياق · الإطار البرتقالي</figcaption>
                    </figure>
                    <div className="name-bench-reading">
                      <span className="name-bench-kicker">ما قرأته الآلة</span>
                      <strong>{preparingReview ? "نجهّز مساحة القراءة…" : current.suggestedTextAr || "لم تقرأ اسماً هنا"}</strong>
                      {current.readingMethod?.includes("tesseract") ? <small>{current.viewCount} {current.viewCount === 1 ? "قراءة محلية تجريبية" : "قراءات محلية تجريبية"} · تحتاج مراجعتك من المصدر</small> : <div className="name-bench-confidence">
                        <div className="name-bench-confidence-track">
                          <span className={`is-${confidenceLevel}`} style={{ width: `${Math.max(2, confidencePercent)}%` }} />
                        </div>
                        <span>{confidencePercent}٪ · {current.viewCount} {current.viewCount === 1 ? "قراءة" : "قراءات"}</span>
                      </div>}
                      <p>
                        {current.suggestedTextAr
                          ? "هذه فرضية قراءة، لا حكم على الاسم. صحّحها إن كانت قريبة."
                          : "افحص الموضع في السياق أولاً. اكتب الاسم فقط إن ظهر كاملاً، أو عدّل الحدود أو ارفض الموضع."}
                      </p>
                      {current.alternatives && current.alternatives.length > 0 ? (
                        <div className="name-bench-alternatives">
                          <span>بدائل القارئ:</span>
                          {current.alternatives.map((alternative) => (
                            <button key={alternative} type="button" disabled={preparingReview} onClick={() => { draftOriginRef.current = "candidate"; setDraft(alternative); inputRef.current?.focus(); }}>
                              {alternative}
                            </button>
                          ))}
                        </div>
                      ) : null}
                    </div>
                  </div>
                </div>

                {cropGeometry?.fragment ? <div className="name-bench-fragment-warning" role="status">
                  <AlertTriangle size={18} />
                  <span>الموضع صغير جداً؛ قد يكون جزءاً من رسم أو حرف. التكبير لن يُظهر اسماً كاملاً. وسّع الحدود أو ارفض الموضع.</span>
                  {!readOnly ? <button type="button" className="name-bench-secondary" onClick={openRegionEditor}><Crop size={14} /> تعديل الحدود</button> : null}
                </div> : null}

                <div className="name-bench-map">
                  <div className="name-bench-map-head">
                    <div>
                      <span className="name-bench-kicker">خريطة الصفحة</span>
                      <h4>أين يقع هذا الاسم؟</h4>
                    </div>
                    <div className="name-bench-map-tools">
                      <button type="button" className="name-bench-tool" onClick={() => { setMapZoom(4); setMapLocateTick((value) => value + 1); }}>إظهار الموضع</button>
                      <button type="button" className="name-bench-tool" onClick={() => setMapZoom(1)}>الصفحة كاملة</button>
                      <button type="button" className="name-bench-tool" onClick={() => setMapZoom((value) => Math.max(1, value - 0.5))} disabled={mapZoom <= 1} aria-label="تصغير الخريطة"><Minus size={14} /></button>
                      <span>{Math.round(mapZoom * 100)}٪</span>
                      <button type="button" className="name-bench-tool" onClick={() => setMapZoom((value) => Math.min(6, value + 0.5))} disabled={mapZoom >= 6} aria-label="تكبير الخريطة"><Plus size={14} /></button>
                    </div>
                  </div>
                  <div className="name-bench-map-viewport" ref={mapViewportRef}>
                    {!preparingReview && previewURL ? (
                      <div className={`name-bench-map-frame${regionDraft ? " is-drawing" : ""}`} style={{ width: previewSize.height > 0 ? `calc(var(--name-map-fit-height, 260px) * ${previewSize.width / previewSize.height * mapZoom})` : "100%", maxWidth: mapZoom === 1 ? "100%" : undefined }}
                        onPointerDown={(event) => {
                          if (!regionDraft || regionBusy || readOnly || event.button !== 0) return;
                          const bounds = event.currentTarget.getBoundingClientRect();
                          mapDragRef.current = { x: (event.clientX - bounds.left) / bounds.width, y: (event.clientY - bounds.top) / bounds.height };
                          event.currentTarget.setPointerCapture(event.pointerId);
                        }}
                        onPointerMove={(event) => {
                          const start = mapDragRef.current;
                          if (!start || !regionDraft) return;
                          const bounds = event.currentTarget.getBoundingClientRect();
                          const x = Math.max(0, Math.min(1, (event.clientX - bounds.left) / bounds.width));
                          const y = Math.max(0, Math.min(1, (event.clientY - bounds.top) / bounds.height));
                          setRegionDraft({ x: Math.min(start.x, x) * 100, y: Math.min(start.y, y) * 100, width: Math.abs(x - start.x) * 100, height: Math.abs(y - start.y) * 100 });
                          setRegionPreview(null);
                        }}
                        onPointerUp={() => { mapDragRef.current = null; }}
                        onPointerCancel={() => { mapDragRef.current = null; }}>
                        <img src={previewURL} alt={`معاينة صفحة ${current.pageNumber}`} draggable={false} onLoad={(event) => { setPreviewSize({ width: event.currentTarget.naturalWidth, height: event.currentTarget.naturalHeight }); setPreviewLoaded(true); }} />
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
                        <SourceSpotMarker region={regionDraft ? percentToRegion(regionDraft) : current.region} target />
                      </div>
                    ) : (
                      <div className="name-bench-map-empty">لم تُحمّل معاينة الصفحة بعد.</div>
                    )}
                  </div>
                  <p>{regionDraft ? "اسحب حول الاسم على الخريطة لتغيير الحدود، ثم اضغط «معاينة» قبل الحفظ." : "الإطار والهدف البرتقالي يحددان الموضع الحالي. انقر مربعاً آخر للانتقال إليه."}</p>
                {regionDraft ? (
                  <div className="name-bench-repair" role="group" aria-label="تعديل حدود القراءة">
                    <div className="name-bench-section-label">
                      <span><Crop size={15} /> حدود القراءة</span>
                      <small>اسحب حول الاسم على الخريطة أو أدخل النسب</small>
                    </div>
                    <div className="name-bench-region-grid">
                      {(["x", "y", "width", "height"] as const).map((field) => (
                        <label key={field}>
                          <span>{REGION_FIELD_LABELS[field]}</span>
                          <input
                            type="number"
                            min={0}
                            max={100}
                            step={0.1}
                            value={Number(regionDraft[field].toFixed(2))}
                            onChange={(event) => updateRegionField(field, Number(event.target.value))}
                          />
                        </label>
                      ))}
                    </div>
                    {regionPreview ? (
                      <div className="name-bench-region-preview">
                        <figure>
                          <img src={`data:image/png;base64,${regionPreview.tightPng}`} alt="معاينة الحد الضيق" />
                          <figcaption>الحد الضيق</figcaption>
                        </figure>
                        <figure>
                          <img src={`data:image/png;base64,${regionPreview.contextPng}`} alt="معاينة السياق" />
                          <figcaption>السياق</figcaption>
                        </figure>
                        <p>
                          ارتفاع الاسم {regionPreview.quality.nameHeightPx} بكسل
                          {regionPreview.quality.clipped ? " · القصاصة تلامس حافة المصدر" : ""}
                        </p>
                      </div>
                    ) : null}
                    {regionError ? <p className="name-bench-error" role="alert">{regionError}</p> : null}
                    <div className="name-bench-repair-actions">
                      <button type="button" className="name-bench-secondary" onClick={() => void previewRegion()} disabled={regionBusy}>
                        {regionBusy ? <Loader2 size={14} className="name-bench-spin" /> : <ScanText size={14} />} معاينة
                      </button>
                      <button type="button" className="name-bench-primary" onClick={() => void confirmRegion()} disabled={!regionPreview || regionBusy}>
                        <Check size={14} /> حفظ الحدود
                      </button>
                      <button type="button" className="name-bench-text-button" onClick={() => { setRegionDraft(null); setRegionPreview(null); setRegionError(""); }} disabled={regionBusy}>
                        إلغاء
                      </button>
                    </div>
                  </div>
                ) : null}

                </div>
              </section>

              <aside className="name-bench-editor" aria-label="إدخال الاسم">
                <div className="name-bench-editor-head">
                  <div>
                    <span className="name-bench-kicker">الاسم {Math.max(1, activeIndex + 1)} من {filtered.length}</span>
                    <h3>{readOnly ? "سجل هذا الموضع" : current.suggestedTextAr ? "راجع القراءة أو صحّحها" : "اكتب الاسم من الصورة"}</h3>
                  </div>
                  <div className="name-bench-editor-head-tools">
                    <span className="name-bench-position">صفحة {current.pageNumber}</span>
                    {!readOnly ? (
                      <button type="button" className="name-bench-text-button" onClick={() => setHistoryOpen((open) => !open)} aria-pressed={historyOpen}>
                        <History size={14} /> السجل
                      </button>
                    ) : null}
                  </div>
                </div>

                {readOnly ? (
                  <>
                    <ReviewHistoryPanel
                      history={history}
                      loading={historyLoading}
                      onReopen={() => void reopenCurrent()}
                      reopening={working}
                    />
                    <div className="name-bench-nav">
                      <button type="button" onClick={() => step(-1)} disabled={activeIndex <= 0} title="k">
                        <ChevronRight size={15} /> السابق
                      </button>
                      <button type="button" onClick={() => step(1)} disabled={activeIndex < 0 || activeIndex >= filtered.length - 1} title="j">
                        التالي <ChevronLeft size={15} />
                      </button>
                    </div>
                  </>
                ) : (
                  <>
                <label className="name-bench-field">
                  <span>الاسم كما تراه</span>
                  <input
                    ref={inputRef}
                    disabled={working || preparingReview}
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
                      disabled={preparingReview || aiLoading || !readingCapabilities?.hostedReader.ready}
                      aria-busy={aiLoading}
                      title={readingCapabilities?.hostedReader.ready ? "يرسل قصاصة الاسم وحدها إلى النموذج البصري" : readerReason(readingCapabilities?.hostedReader.reason)}
                    >
                      {aiLoading ? <Loader2 size={14} className="name-bench-spin" /> : <Sparkles size={14} />}
                      {aiCandidates.length > 0 ? "اقترح مرة أخرى" : "اقترح بالذكاء الاصطناعي"}
                    </button>
                    <small>{readingCapabilities?.hostedReader.ready ? "يقرأ القصاصة نفسها، ويعيد احتمالات فقط." : readerReason(readingCapabilities?.hostedReader.reason)}</small>
                  </div>
                  <LocalNameReadingTool key={current.id} suggestionId={current.id} available={readingCapabilities?.localReader.ready === true} unavailableReason={readerReason(readingCapabilities?.localReader.reason, true)} onReadings={setLocalReadings} buttonRef={localRereadButtonRef} />
                  {aiError ? <p className="name-bench-ai-error">{aiError}</p> : null}
                  {candidates.length > 0 ? (
                    <div className="name-bench-chips">
                      {candidates.map((candidate, index) => (
                        <button
                          key={`${candidate.source}-${candidate.value}`}
                          type="button"
                          className={`name-bench-chip is-${candidate.source}`}
                          disabled={preparingReview} onClick={() => { draftOriginRef.current = "candidate"; setDraft(candidate.value); inputRef.current?.focus(); }}
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
                        <button key={name} type="button" disabled={preparingReview} onClick={() => { draftOriginRef.current = "candidate"; setDraft(name); inputRef.current?.focus(); }} title="اقتراح من النموذج للقصاصة نفسها">
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

                {assistance ? (
                  <div className={`name-bench-assistance is-${assistance.tone}`} role="status">
                    <ScanText size={17} />
                    <div>
                      <strong>{assistance.title}</strong>
                      <p>{assistance.detail}</p>
                    </div>
                    {assistance.actionLabel ? (
                      <button type="button" className="name-bench-text-button" onClick={runAssistance}>
                        {assistance.actionLabel}
                      </button>
                    ) : null}
                  </div>
                ) : null}

                {splitOpen && splitRegions ? (
                  <div className="name-bench-repair" role="group" aria-label="تقسيم الموضع">
                    <div className="name-bench-section-label">
                      <span><Scissors size={15} /> تقسيم الموضع</span>
                      <small>جزءان لا يتداخلان داخل الموضع الأصلي</small>
                    </div>
                    <div className="name-bench-split-grid">
                      {(["first", "second"] as const).map((label, index) => {
                        const region = splitRegions[index as 0 | 1];
                        return (
                          <fieldset key={label}>
                            <legend>{index === 0 ? "الجزء الأول" : "الجزء الثاني"}</legend>
                            {(["x", "y", "width", "height"] as const).map((field) => (
                              <label key={field}>
                                <span>{REGION_FIELD_LABELS[field]}</span>
                                <input
                                  type="number"
                                  min={0}
                                  max={100}
                                  step={0.1}
                                  value={Number(region[field].toFixed(2))}
                                  onChange={(event) => updateSplitField(index as 0 | 1, field, Number(event.target.value))}
                                />
                              </label>
                            ))}
                          </fieldset>
                        );
                      })}
                    </div>
                    {splitError ? <p className="name-bench-error" role="alert">{splitError}</p> : null}
                    <div className="name-bench-repair-actions">
                      <button type="button" className="name-bench-primary" onClick={() => void submitSplit()} disabled={splitBusy}>
                        {splitBusy ? <Loader2 size={14} className="name-bench-spin" /> : <Scissors size={14} />} قسّم واحفظ
                      </button>
                      <button type="button" className="name-bench-text-button" onClick={() => { setSplitOpen(false); setSplitRegions(null); setSplitError(""); }} disabled={splitBusy}>
                        إلغاء
                      </button>
                    </div>
                  </div>
                ) : null}

                {mergeOpen ? (
                  <div className="name-bench-repair" role="group" aria-label="دمج المواضع">
                    <div className="name-bench-section-label">
                      <span><GitMerge size={15} /> دمج المواضع</span>
                      <small>المواضع المختارة على الصفحة نفسها</small>
                    </div>
                    <div className="name-bench-merge-list">
                      {mergeCandidates.map((suggestion) => (
                        <label key={suggestion.id}>
                          <input
                            type="checkbox"
                            checked={mergeSelection.includes(suggestion.id)}
                            onChange={() => toggleMerge(suggestion.id)}
                          />
                          <span dir="rtl">{suggestion.suggestedTextAr || "بلا قراءة"}</span>
                          <small>{Math.round(suggestion.region.x * 100)}٪، {Math.round(suggestion.region.y * 100)}٪</small>
                        </label>
                      ))}
                    </div>
                    {mergeRegion ? (
                      <p className="name-bench-merge-region">
                        الحد الناتج: {Math.round(mergeRegion.x * 100)}٪، {Math.round(mergeRegion.y * 100)}٪ · {Math.round(mergeRegion.width * 100)}٪ × {Math.round(mergeRegion.height * 100)}٪
                      </p>
                    ) : null}
                    {mergeError ? <p className="name-bench-error" role="alert">{mergeError}</p> : null}
                    <div className="name-bench-repair-actions">
                      <button type="button" className="name-bench-primary" onClick={() => void submitMerge()} disabled={mergeBusy || mergeSelection.length < 2}>
                        {mergeBusy ? <Loader2 size={14} className="name-bench-spin" /> : <GitMerge size={14} />} ادمج واحفظ
                      </button>
                      <button type="button" className="name-bench-text-button" onClick={() => { setMergeOpen(false); setMergeSelection([]); setMergeError(""); }} disabled={mergeBusy}>
                        إلغاء
                      </button>
                    </div>
                  </div>
                ) : null}

                {message ? <p className="name-bench-message" role="status">{message}</p> : null}
                {error ? <p className="name-bench-error" role="alert">{error}</p> : null}

                <div className="name-bench-actions">
                  <button
                    type="button"
                    className="name-bench-primary"
                    onClick={() => void submit("save")}
                    disabled={working || preparingReview || !draft.trim()}
                    title="Enter"
                  >
                    {working ? <Loader2 size={17} className="name-bench-spin" /> : <Check size={17} />}
                    {draft.trim() && draft.trim() === current.suggestedTextAr.trim() ? "اعتمد القراءة" : "احفظ الاسم"}
                  </button>
                  <button type="button" className="name-bench-secondary" onClick={() => void submit("deferred")} disabled={working || preparingReview} title="d (يخرج من القائمة الحالية)">
                    تأجيل
                  </button>
                  <button type="button" className="name-bench-danger" onClick={() => void submit("rejected")} disabled={working || preparingReview} title="r (هذا الموضع ليس اسماً)">
                    <CircleSlash size={15} /> رفض الموضع
                  </button>
                </div>

                <div className="name-bench-repair-actions">
                  <button type="button" className="name-bench-text-button" onClick={openRegionEditor} disabled={working || preparingReview}>
                    <Crop size={14} /> تعديل الحدود
                  </button>
                  <button type="button" className="name-bench-text-button" onClick={openSplit} disabled={working || preparingReview}>
                    <Scissors size={14} /> تقسيم
                  </button>
                  <button
                    type="button"
                    className="name-bench-text-button"
                    onClick={openMerge}
                    disabled={working || preparingReview || mergeCandidates.length < 2}
                    title={mergeCandidates.length < 2 ? "تحتاج موضعين على الصفحة نفسها للدمج" : "دمج مواضع الصفحة نفسها"}
                  >
                    <GitMerge size={14} /> دمج
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
                {historyOpen ? (
                  <ReviewHistoryPanel
                    history={history}
                    loading={historyLoading}
                    onReopen={() => void reopenCurrent()}
                    reopening={working}
                  />
                ) : null}
                  </>
                )}
              </aside>
            </>
          ) : null}
        </div>
      </section>
    </div>
  );
}

const REGION_FIELD_LABELS: Record<keyof PercentRegion, string> = {
  x: "س",
  y: "ص",
  width: "العرض",
  height: "الارتفاع",
};

function queueStatus(mode: QueueMode): string {
  return mode === "pending" ? "suggested" : mode;
}

function regionToPercent(region: NameRegion): PercentRegion {
  return {
    x: region.x * 100,
    y: region.y * 100,
    width: region.width * 100,
    height: region.height * 100,
  };
}

function percentToRegion(region: PercentRegion): NameRegion {
  return {
    x: region.x / 100,
    y: region.y / 100,
    width: region.width / 100,
    height: region.height / 100,
  };
}

function validatePercentRegion(region: PercentRegion): string | null {
  for (const value of [region.x, region.y, region.width, region.height]) {
    if (!Number.isFinite(value)) return "أدخل أرقاماً صالحة لكل حقل.";
  }
  if (region.width <= 0 || region.height <= 0) return "يجب أن يكون عرض الحد وارتفاعه أكبر من صفر.";
  if (region.x < 0 || region.y < 0) return "لا يمكن أن يبدأ الحد خارج الصفحة.";
  if (region.x + region.width > 100.0001 || region.y + region.height > 100.0001) {
    return "يجب أن يبقى الحد داخل صفحة المصدر.";
  }
  return null;
}

function sameNameRegion(left: NameRegion, right: NameRegion): boolean {
  const epsilon = 1e-9;
  return Math.abs(left.x - right.x) <= epsilon
    && Math.abs(left.y - right.y) <= epsilon
    && Math.abs(left.width - right.width) <= epsilon
    && Math.abs(left.height - right.height) <= epsilon;
}

function percentRegionContains(outer: PercentRegion, inner: PercentRegion): boolean {
  const epsilon = 1e-6;
  return inner.x + epsilon >= outer.x
    && inner.y + epsilon >= outer.y
    && inner.x + inner.width <= outer.x + outer.width + epsilon
    && inner.y + inner.height <= outer.y + outer.height + epsilon;
}

function percentRegionsOverlap(left: PercentRegion, right: PercentRegion): boolean {
  const x = Math.max(left.x, right.x);
  const y = Math.max(left.y, right.y);
  const rightEdge = Math.min(left.x + left.width, right.x + right.width);
  const bottom = Math.min(left.y + left.height, right.y + right.height);
  return rightEdge - x > 1e-6 && bottom - y > 1e-6;
}

function unionNameRegions(regions: NameRegion[]): NameRegion {
  if (regions.length === 0) return { x: 0, y: 0, width: 0, height: 0 };
  let left = regions[0].x;
  let top = regions[0].y;
  let right = regions[0].x + regions[0].width;
  let bottom = regions[0].y + regions[0].height;
  for (const region of regions.slice(1)) {
    left = Math.min(left, region.x);
    top = Math.min(top, region.y);
    right = Math.max(right, region.x + region.width);
    bottom = Math.max(bottom, region.y + region.height);
  }
  return { x: left, y: top, width: right - left, height: bottom - top };
}

interface ReaderAssistance {
  tone: "ready" | "caution" | "blocked";
  title: string;
  detail: string;
  action: "region" | "reread" | "input";
  actionLabel: string;
}

// Turn a reader-stage outcome into the one useful next action, without turning
// a machine score into a correctness claim. The reviewer still writes the name.
function readerAssistance(
  suggestion: SourceTreeSuggestion,
  cropState: CropState,
  capabilities: ReadingCapabilities | null,
  readings: LocalNameReading[],
): ReaderAssistance {
  if (cropState === "error") {
    return {
      tone: "blocked",
      title: "تعذّرت قراءة القصاصة من المصدر",
      detail: "عدّل الحدود لالتقاط الاسم كاملاً، أو اكتبه يدوياً إن كان واضحاً في السياق.",
      action: "region",
      actionLabel: "عدّل الحدود",
    };
  }
  const latest = readings[0];
  if (!latest) {
    if (capabilities && !capabilities.localReader.ready) {
      return {
        tone: suggestion.suggestedTextAr.trim() ? "ready" : "caution",
        title: suggestion.suggestedTextAr.trim() ? "جاهز للمراجعة من المصدر" : "القارئ المحلي غير متاح",
        detail: suggestion.suggestedTextAr.trim()
          ? "اقرأ الاسم من القصاصة، ثم اعتمد القراءة أو صحّحها."
          : "يمكنك كتابة الاسم من الصورة؛ لا حاجة لأي مزوّد خارجي.",
        action: "input",
        actionLabel: "أدخل الاسم",
      };
    }
    return {
      tone: "ready",
      title: "جاهز للمراجعة",
      detail: "اقرأ الاسم من القصاصة، أو اطلب قراءة محلية تجريبية للمقارنة.",
      action: capabilities?.localReader.ready ? "reread" : "input",
      actionLabel: capabilities?.localReader.ready ? "أعد القراءة محلياً" : "راجع النص",
    };
  }
  if (latest.quality.clipped) {
    return {
      tone: "caution",
      title: "القصاصة تلامس حافة المصدر",
      detail: "قد يكون جزء من الاسم خارج الحد. عدّل الحدود قبل الاعتماد.",
      action: "region",
      actionLabel: "عدّل الحدود",
    };
  }
  if (latest.outcome === "reading_unavailable") {
    return {
      tone: "blocked",
      title: "تعذّرت القراءة المحلية",
      detail: "أعد طلب القراءة المحلية بعد تعديل الحدود، أو اكتب الاسم من الصورة.",
      action: capabilities?.localReader.ready ? "reread" : "input",
      actionLabel: capabilities?.localReader.ready ? "أعد القراءة محلياً" : "أدخل الاسم",
    };
  }
  const variants = uniqueNames(latest.variants.map((variant) => variant.rawText));
  if (variants.length > 1) {
    return {
      tone: "caution",
      title: "قراءات محلية متعارضة",
      detail: "الاختلاف بين القراءات لا يثبت صحة اسم. راجع البدائل واختر ما تراه في الأصل.",
      action: "input",
      actionLabel: "راجع البدائل",
    };
  }
  if (latest.outcome === "unreadable") {
    return {
      tone: "caution",
      title: "لم يُستخرج نص واضح",
      detail: "القارئ المحلي لم يجد نصاً موثوقاً. اكتب الاسم من الصورة.",
      action: "input",
      actionLabel: "أدخل الاسم",
    };
  }
  return {
    tone: "ready",
    title: "قراءة محلية جاهزة للمقارنة",
    detail: "النص الآلي احتمال، وليس اعتماداً. راجع الأصل ثم اكتب الاسم أو اعتمده.",
    action: "input",
    actionLabel: "راجع النص",
  };
}

function formatReviewTime(value: string): string {
  if (!value) return "";
  const parsed = new Date(value);
  return Number.isNaN(parsed.getTime()) ? value : parsed.toLocaleString("ar", { dateStyle: "short", timeStyle: "short" });
}

function ReviewHistoryPanel({ history, loading, onReopen, reopening }: {
  history: SourceSuggestionHistory | null;
  loading: boolean;
  onReopen: () => void;
  reopening: boolean;
}) {
  if (loading) {
    return <div className="name-bench-history is-loading"><Loader2 size={18} className="name-bench-spin" /><span>نجهّز السجل…</span></div>;
  }
  if (!history) {
    return <div className="name-bench-history"><p>تعذّر تحميل السجل الآن. أعد المحاولة من زر التحديث.</p></div>;
  }
  const originalReading = history.decisions[0]?.suggestedTextAr || history.suggestion.suggestedTextAr;
  return (
    <div className="name-bench-history">
      <div className="name-bench-section-label">
        <span><History size={15} /> سجل المراجعة</span>
        <small>{history.suggestion.superseded ? "موضع مُستبدل" : history.suggestion.status === "suggested" ? "بانتظار المراجعة" : "مقرر"}</small>
      </div>
      {history.suggestion.superseded ? (
        <p className="name-bench-history-note">استُبدل هذا الموضع بعد تجديد الكشف أو تعديل المواضع. قراءته وقراراته السابقة باقية للقراءة فقط.</p>
      ) : null}
      <div className="name-bench-history-original">
        <span>القراءة الأصلية</span>
        <strong dir="rtl">{originalReading || "لم تقرأ الآلة اسماً"}</strong>
      </div>
      {history.decisions.length > 0 ? (
        <ol className="name-bench-history-decisions">
          {history.decisions.map((decision) => (
            <li key={decision.id}>
              <div>
                <strong>المراجعة {decision.revision}</strong>
                <em>{DECISION_LABELS[decision.decision]}</em>
                <small>{formatReviewTime(decision.reviewedAt)}</small>
              </div>
              {decision.decidedTextAr ? <p dir="rtl">«{decision.decidedTextAr}»</p> : null}
              {decision.noteAr ? <p className="name-bench-history-meta">{decision.noteAr}</p> : null}
              {decision.reviewerLabelAr ? <p className="name-bench-history-meta">بواسطة {decision.reviewerLabelAr}</p> : null}
            </li>
          ))}
        </ol>
      ) : (
        <p className="name-bench-history-note">لا قرار بشري بعد.</p>
      )}
      {history.regionRevisions.length > 0 ? (
        <div className="name-bench-history-block">
          <span>حدود القراءة المعدّلة</span>
          {history.regionRevisions.map((revision) => (
            <p key={revision.id}>
              المراجعة {revision.revision}: {Math.round(revision.previousRegion.x * 100)}٪، {Math.round(revision.previousRegion.y * 100)}٪
              {" → "}
              {Math.round(revision.region.x * 100)}٪، {Math.round(revision.region.y * 100)}٪
            </p>
          ))}
        </div>
      ) : null}
      {history.lineage.length > 0 ? (
        <div className="name-bench-history-block">
          <span>الارتباط الهندسي</span>
          {history.lineage.map((edge) => (
            <p key={`${edge.kind}-${edge.parentId}-${edge.childId}`}>
              {EVENT_LABELS[edge.kind] ?? edge.kind}: {edge.parentId.slice(0, 8)} ← {edge.childId.slice(0, 8)}
            </p>
          ))}
        </div>
      ) : null}
      {history.events.filter((event) => event.type !== "decision").length > 0 ? (
        <div className="name-bench-history-block">
          <span>الأحداث</span>
          {history.events.filter((event) => event.type !== "decision").map((event) => (
            <p key={event.id}>
              {EVENT_LABELS[event.type] ?? event.type} · المراجعة {event.revision} · {formatReviewTime(event.createdAt)}
              {typeof event.metadata?.note === "string" && event.metadata.note ? ` — ${event.metadata.note}` : ""}
            </p>
          ))}
        </div>
      ) : null}
      {history.canReopen ? (
        <button type="button" className="name-bench-secondary name-bench-reopen" onClick={onReopen} disabled={reopening}>
          {reopening ? <Loader2 size={15} className="name-bench-spin" /> : <Undo2 size={15} />}
          إعادة فتح الموضع
        </button>
      ) : null}
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

function readerReason(reason?: string, local = false): string {
  switch (reason) {
    case "disabled_by_policy": return local ? "القراءة المحلية معطّلة في الإعدادات الحالية." : "القراءة البصرية الخارجية معطّلة لهذا المصدر. يمكنك تصحيح الاسم يدوياً.";
    case "not_configured": return "القارئ البصري غير مهيأ. يمكنك تصحيح الاسم يدوياً.";
    case "local_reader_not_implemented": return "إعادة قراءة القصاصة محلياً غير متاحة في هذه المرحلة.";
    case "tool_missing": return "تحتاج القراءة المحلية إلى أدوات غير مثبتة.";
    case "language_missing": return "بيانات اللغة العربية للقراءة المحلية غير مثبتة.";
    default: return "خدمة القراءة غير متاحة مؤقتاً. يمكنك التصحيح يدوياً أو تحديث الحالة.";
  }
}

function candidateErrorMessage(cause: unknown): string {
  if (cause instanceof ApiError) {
    if (cause.code === "disabled_by_policy" || cause.code === "not_configured" || cause.code === "temporarily_unavailable") return readerReason(cause.code);
    if (cause.code === "unreadable_crop") return "لم يتمكن القارئ من قراءة هذه القصاصة. يمكنك كتابة الاسم يدوياً.";
    if (cause.status === 401 || cause.status === 403) return "انتهت الجلسة أو لا تملك صلاحية طلب الاقتراحات. أعد تسجيل الدخول.";
    if (cause.status === 404) return "لم يعد موضع الاسم موجوداً في هذا الملف.";
    if (cause.status === 429) return "طلبت اقتراحات كثيرة بسرعة. انتظر قليلاً ثم أعد المحاولة.";
    if (cause.status === 503) return readerReason("temporarily_unavailable");
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

function LocalNameReadingTool({ suggestionId, available, unavailableReason, onReadings, buttonRef }: { suggestionId: string; available: boolean; unavailableReason: string; onReadings?: (readings: LocalNameReading[]) => void; buttonRef?: { current: HTMLButtonElement | null } }) {
 const [run, setRun] = useState<NameReadingRun | null>(null);
 const [readings, setReadings] = useState<LocalNameReading[]>([]);
 const [busy, setBusy] = useState(false);
 const [error, setError] = useState("");
 const request = useRef<AbortController | null>(null);
 const active = run?.status === "queued" || run?.status === "running";
 useEffect(() => { onReadings?.(readings); }, [onReadings, readings]);
 useEffect(() => {
  const abort = new AbortController();
  void fetchLocalNameReadings(suggestionId, abort.signal).then((values) => { if (!abort.signal.aborted) setReadings(values); }).catch(() => {});
  void fetchLatestLocalRereadRun(suggestionId, abort.signal).then((value) => { if (!abort.signal.aborted) setRun(value); }).catch(() => {});
  return () => { abort.abort(); request.current?.abort(); };
 }, [suggestionId]);
 useEffect(() => {
  if (!run || !active) return;
  const abort = new AbortController();
  const timer = window.setTimeout(() => {
   void fetchNameReadingRun(run.id, abort.signal).then(async (value) => {
    if (abort.signal.aborted) return;
    setRun(value);
    if (value.status !== "queued" && value.status !== "running") {
     const values = await fetchLocalNameReadings(suggestionId, abort.signal);
     if (!abort.signal.aborted) setReadings(values);
    }
   }).catch(() => { if (!abort.signal.aborted) { setError("تعذّر تحديث الحالة. يمكنك متابعة التصحيح أو طلب الحالة مجدداً؛ الإعداد المحلي مستمر في الخلفية."); setRun(null); } });
  }, 1500);
  return () => { window.clearTimeout(timer); abort.abort(); };
 }, [run, active, suggestionId]);
 const reread = async () => {
  if (!available || busy || active) return;
  request.current?.abort(); const abort = new AbortController(); request.current = abort;
  setBusy(true); setError("");
  try {
   const value = await rereadSourceSuggestionLocally(suggestionId, abort.signal);
   if (abort.signal.aborted) return;
   setRun(value);
   if (value.status !== "queued" && value.status !== "running") setReadings(await fetchLocalNameReadings(suggestionId, abort.signal));
  } catch { if (!abort.signal.aborted) setError("تعذّر طلب القراءة المحلية. يمكنك كتابة الاسم يدوياً."); }
  finally { if (!abort.signal.aborted) setBusy(false); }
 };
 return <>
  <div className="name-bench-ai-row">
   <button ref={buttonRef} type="button" className="name-bench-ai-button" disabled={!available || busy || active} aria-busy={busy || active} title={available ? "تجهيز قصاصة الاسم بالأدوات المحلية؛ لا تُرسل الصورة إلى نموذج خارجي" : unavailableReason} onClick={() => void reread()}><ScanText size={14} /> إعادة القراءة محلياً</button>
   <small>{!available ? unavailableReason : active ? "جارٍ الإعداد محلياً؛ يمكنك متابعة التصحيح." : "قراءات تجريبية محلية، ولا يُحفظ اسم دون قرارك."}</small>
  </div>
  {error ? <p className="name-bench-ai-error">{error}</p> : null}
  {readings.length > 0 ? <details className="name-bench-suggestions"><summary>مقارنة القراءات المحلية المحفوظة</summary>
   <small>اختلاف القراءات ودرجات الأداة لا يثبت صحة اسم.</small>
   {readings.slice(0, 3).map((reading, index) => <div key={index}>
    <small>{reading.readerVersion} · {reading.outcome === "reading_unavailable" ? "القارئ غير متاح" : reading.outcome === "manual_review" ? "يحتاج فحص المصدر يدوياً" : reading.outcome === "unreadable" ? "لم يُستخرج نص" : "نص آلي يحتاج مراجعتك"}{reading.quality.clipped ? " · القصاصة تلامس حافة المصدر" : ""}</small>
    <small>الجودة: ارتفاع الاسم {reading.quality.nameHeightPx} بكسل · تباين {(reading.quality.contrast ?? 0).toFixed(2)}{reading.quality.reason ? ` · ${reading.quality.reason}` : ""}</small>
    {reading.variants.map((variant) => <p key={variant.variant}><span>{variant.variant}</span> · <span dir="rtl">{variant.rawText || "لا نص"}</span>{variant.engineScore !== undefined ? <small> · درجة الأداة الخام {variant.engineScore.toFixed(3)}</small> : null}</p>)}
    {reading.alternatives?.length ? <p><small>بدائل مقترحة: <span dir="rtl">{reading.alternatives.join("، ")}</span></small></p> : null}
   </div>)}
  </details> : null}
 </>;
}
