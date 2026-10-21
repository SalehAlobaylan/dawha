import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, Check, ChevronDown, Eye, GitBranch, Maximize2, Minus, Plus, RotateCcw, Save, ScanSearch, X } from "lucide-react";
import { useEffect, useMemo, useRef, useState, type PointerEvent as ReactPointerEvent } from "react";
import { ApiError, fetchSourceChartAnalysis, fetchSourcePagePreview, retrySourceChartAnalysis, saveSourceChartAnalysisReview } from "../lib/api";
import type { ChartAnalysisReviewPayload, ChartAnalysisView, ChartBranchRegion, ChartConnectorHypothesis, ChartNameOccurrence, ChartPointPair } from "../types";

interface SourceChartAnalysisDialogProps {
  fileId: string;
  titleAr: string;
  initial: ChartAnalysisView;
  onClose: () => void;
  onManualDraft: (fileId: string) => void;
}

type DrawMode = "branch" | "name" | "crop" | "connector" | null;
type Point = { x: number; y: number };

export function SourceChartAnalysisDialog({ fileId, titleAr, initial, onClose, onManualDraft }: SourceChartAnalysisDialogProps) {
  const queryClient = useQueryClient();
  const [branches, setBranches] = useState<ChartBranchRegion[]>(initial.review?.branches ?? initial.branches);
  const [names, setNames] = useState<ChartNameOccurrence[]>(initial.review?.names ?? initial.names);
  const [connectors, setConnectors] = useState<ChartConnectorHypothesis[]>(initial.review?.connectors ?? initial.connectors);
  const [reviewedTileIds, setReviewedTileIds] = useState<string[]>(initial.review?.reviewedTileIds ?? []);
  const [residualReviewed, setResidualReviewed] = useState(initial.review?.residualRegionsReviewed ?? false);
  const [coverageConfirmed, setCoverageConfirmed] = useState(initial.review?.coverageConfirmed ?? false);
  const [selectedNameId, setSelectedNameId] = useState(initial.names[0]?.id ?? "");
  const [selectedBranchId, setSelectedBranchId] = useState("");
  const [branchFilter, setBranchFilter] = useState("");
  const [nameFilter, setNameFilter] = useState("all");
  const [drawMode, setDrawMode] = useState<DrawMode>(null);
  const [drawing, setDrawing] = useState<Point[]>([]);
  const [previewUrl, setPreviewUrl] = useState("");
  const [zoom, setZoom] = useState(1);
  const [pan, setPan] = useState<Point>({ x: 0, y: 0 });
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const [dirty, setDirty] = useState(false);
  const [layers, setLayers] = useState({ branches: true, names: true, connectors: false, residual: true });
  const svgRef = useRef<SVGSVGElement>(null);
  const panOrigin = useRef<Point | null>(null);
  const syncedRevision = useRef(`${initial.id}:${initial.reviewRevision}:${initial.status}`);

  const analysisQuery = useQuery({
    queryKey: ["source-chart-analysis", fileId],
    queryFn: () => fetchSourceChartAnalysis(fileId),
    initialData: initial,
    refetchInterval: (query) => {
      const current = query.state.data as ChartAnalysisView | undefined;
      return current?.status === "queued" || current?.status === "running" ? 1800 : false;
    },
  });
  const analysis = analysisQuery.data;
  const busy = analysis?.status === "queued" || analysis?.status === "running";
  const saveMutation = useMutation({
    mutationFn: (input: ChartAnalysisReviewPayload) => saveSourceChartAnalysisReview(initial.id, input),
    onSuccess: (updated) => {
      queryClient.setQueryData(["source-chart-analysis", fileId], updated);
      syncReview(updated);
      setDirty(false);
      setMessage(`حُفظت المراجعة ${updated.reviewRevision}. قراءات هذه المساحة تجريبية ولا تدخل الاستيراد.`);
      setError("");
    },
    onError: (cause) => {
      setError(cause instanceof ApiError && cause.status === 409 ? "تغيرت المراجعة في جلسة أخرى. حدّث الصفحة قبل حفظ تعديلاتك." : errorMessage(cause));
    },
  });
  const retryMutation = useMutation({
    mutationFn: () => retrySourceChartAnalysis(initial.id),
    onSuccess: (updated) => {
      queryClient.setQueryData(["source-chart-analysis", fileId], updated);
      setError(""); setMessage("أُعيدت جدولة الوحدات المتعثرة؛ ستُستأنف الوحدات المحفوظة دون تكرارها.");
    },
    onError: (cause) => setError(errorMessage(cause)),
  });

  function syncReview(updated: ChartAnalysisView) {
    const review = updated.review;
    setBranches(review?.branches ?? updated.branches);
    setNames(review?.names ?? updated.names);
    setConnectors(review?.connectors ?? updated.connectors);
    setReviewedTileIds(review?.reviewedTileIds ?? []);
    setResidualReviewed(review?.residualRegionsReviewed ?? false);
    setCoverageConfirmed(review?.coverageConfirmed ?? false);
    setSelectedNameId((current) => (review?.names ?? updated.names).some((name) => name.id === current) ? current : (review?.names ?? updated.names)[0]?.id ?? "");
    setDirty(false);
  }

  useEffect(() => {
    if (!analysis) return;
    const revisionKey = `${analysis.id}:${analysis.reviewRevision}:${analysis.status}`;
    if (revisionKey === syncedRevision.current) return;
    syncedRevision.current = revisionKey;
    if (!dirty) syncReview(analysis);
    else setError("وصلت مراجعة أحدث من الخادم. احفظ تعديلاتك بعد تحديث الصفحة لتجنب الكتابة فوقها.");
  }, [analysis, dirty]);

  useEffect(() => {
    let active = true;
    let objectUrl = "";
    void fetchSourcePagePreview(fileId, 1).then((blob) => {
      objectUrl = URL.createObjectURL(blob);
      if (active) setPreviewUrl(objectUrl);
      else URL.revokeObjectURL(objectUrl);
    }).catch((cause: unknown) => { if (active) setError(errorMessage(cause)); });
    return () => { active = false; if (objectUrl) URL.revokeObjectURL(objectUrl); };
  }, [fileId]);

  const selectedName = names.find((name) => name.id === selectedNameId);
  const selectedBranch = branches.find((branch) => branch.id === selectedBranchId);
  const visibleNames = useMemo(() => names.filter((name) => {
    if (branchFilter && name.branchId !== branchFilter) return false;
    if (nameFilter === "needs_review" && name.reviewStatus !== "needs_review") return false;
    if (nameFilter === "verified" && name.reviewStatus !== "verified") return false;
    if (nameFilter === "unreadable" && name.readingStatus !== "unreadable") return false;
    return true;
  }), [names, branchFilter, nameFilter]);
  const allTilesReviewed = Boolean(analysis && analysis.tiles.length > 0 && analysis.tiles.every((tile) => tile.status === "succeeded" && reviewedTileIds.includes(tile.id)));
  const hasFailedUnits = Boolean(analysis && (analysis.quality.failedUnitCount > 0 || analysis.quality.pendingUnitCount > 0));

  useEffect(() => {
    if (!analysis || !coverageConfirmed) return;
    if (!allTilesReviewed || hasFailedUnits || analysis.quality.failedTileCount > 0) {
      setCoverageConfirmed(false);
      setResidualReviewed(false);
    }
  }, [analysis, allTilesReviewed, coverageConfirmed, hasFailedUnits]);

  const setNamePatch = (id: string, patch: Partial<ChartNameOccurrence>) => {
    setNames((current) => current.map((name) => name.id === id ? { ...name, ...patch } : name));
    setConnectors((current) => current.map((connector) => connector.startNameId === id || connector.endNameId === id ? { ...connector, status: "needs_review" } : connector));
    setDirty(true);
  };
  const markTileReviewed = (tileId: string) => {
    setReviewedTileIds((current) => current.includes(tileId) ? current.filter((id) => id !== tileId) : [...current, tileId]);
    setCoverageConfirmed(false); setResidualReviewed(false); setDirty(true);
  };
  const pointFromEvent = (event: ReactPointerEvent<SVGSVGElement>): Point => {
    const rect = event.currentTarget.getBoundingClientRect();
    if (rect.width <= 0 || rect.height <= 0) return { x: 0, y: 0 };
    return { x: clampUnit((event.clientX - rect.left) / rect.width), y: clampUnit((event.clientY - rect.top) / rect.height) };
  };
  const pointerDown = (event: ReactPointerEvent<SVGSVGElement>) => {
    if (drawMode) {
      event.preventDefault(); event.currentTarget.setPointerCapture(event.pointerId); setDrawing([pointFromEvent(event)]); return;
    }
    panOrigin.current = { x: event.clientX - pan.x, y: event.clientY - pan.y };
    event.currentTarget.setPointerCapture(event.pointerId);
  };
  const pointerMove = (event: ReactPointerEvent<SVGSVGElement>) => {
    if (drawing.length > 0) {
      const point = pointFromEvent(event);
      setDrawing((current) => current.length >= 300 ? current : [...current, point]);
    } else if (panOrigin.current) {
      setPan({ x: event.clientX - panOrigin.current.x, y: event.clientY - panOrigin.current.y });
    }
  };
  const pointerUp = (event: ReactPointerEvent<SVGSVGElement>) => {
    if (drawing.length === 0 || !analysis) { panOrigin.current = null; return; }
    const points = [...drawing, pointFromEvent(event)];
    const first = points[0]; const last = points[points.length - 1];
    const region = normalizedRect(first, last);
    if (region.width < 0.003 || region.height < 0.003) { setDrawing([]); setDrawMode(null); return; }
    if (drawMode === "name") {
      const id = `manual_${crypto.randomUUID()}`;
      const name: ChartNameOccurrence = { id, sourceOccurrenceIds: [], pageNumber: 1, region, legibility: "uncertain", readingStatus: "unreadable", textAr: "", alternatives: [], uncertainSpans: [], reviewStatus: "needs_review" };
      setNames((current) => [...current, name]); setSelectedNameId(id); setCoverageConfirmed(false); setResidualReviewed(false); setDirty(true); setMessage("أُضيف موضع اسم يدوي. انسخه من الأصل ثم أكد مراجعته.");
    } else if (drawMode === "branch") {
      const polygon: ChartPointPair[] = [[region.x, region.y], [region.x + region.width, region.y], [region.x + region.width, region.y + region.height], [region.x, region.y + region.height]];
      if (selectedBranch) setBranches((current) => current.map((branch) => branch.id === selectedBranch.id ? { ...branch, polygon, reviewerEdited: true, reviewStatus: "needs_review" } : branch));
      else {
        const id = `manual_${crypto.randomUUID()}`;
        setBranches((current) => [...current, { id, pageNumber: 1, polygon, label: "فرع أضافه المراجع", reviewerEdited: true, reviewStatus: "needs_review" }]);
        setSelectedBranchId(id);
      }
      setCoverageConfirmed(false); setResidualReviewed(false);
      setDirty(true); setMessage("حُدّث حد الفرع يدوياً؛ راجع ارتباط الأسماء بهذا الفرع.");
    } else if (drawMode === "crop" && selectedName) {
      setNamePatch(selectedName.id, { region, textAr: "", alternatives: [], uncertainSpans: [], readingStatus: "unreadable", reviewStatus: "needs_review", reviewerNoteAr: "" });
      setMessage("تغير موضع الاسم؛ أُبطلت القراءة السابقة ويجب نسخ الاسم من القصاصة الجديدة.");
    } else if (drawMode === "connector" && points.length >= 3) {
      const connector: ChartConnectorHypothesis = { id: `manual_${crypto.randomUUID()}`, pageNumber: 1, points: points.map(({ x, y }) => [x, y]), kind: "line", status: "needs_review" };
      setConnectors((current) => [...current, connector]); setResidualReviewed(false); setDirty(true); setMessage("رُسم مسار اتصال مرشح. اربط طرفيه باسميْن، ولا يعد المسار وحده دليلاً على النسب.");
    }
    setDrawing([]); setDrawMode(null); panOrigin.current = null;
  };
  const save = () => {
    if (!analysis) return;
    const payload: ChartAnalysisReviewPayload = {
      expectedRevision: analysis.review?.expectedRevision ?? analysis.reviewRevision,
      branches, names, connectors, reviewedTileIds, residualRegionsReviewed: residualReviewed, coverageConfirmed,
    };
    saveMutation.mutate(payload);
  };
  const setConnectorPatch = (id: string, patch: Partial<ChartConnectorHypothesis>) => {
    setConnectors((current) => current.map((connector) => connector.id === id ? { ...connector, ...patch, ...(patch.startNameId !== undefined || patch.endNameId !== undefined ? { status: "needs_review" as const } : {}) } : connector)); setDirty(true);
  };
  const setDrawTool = (mode: DrawMode) => { setDrawMode((current) => current === mode ? null : mode); setDrawing([]); };
  const pageAspect = analysis?.pageWidth && analysis.pageHeight ? `${analysis.pageWidth} / ${analysis.pageHeight}` : "1 / 1";
  return (
    <div className="source-chart-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget && !dirty) onClose(); }}>
      <section className="source-chart-dialog" role="dialog" aria-modal="true" aria-labelledby="source-chart-title" dir="rtl">
        <header className="source-chart-head">
          <div><div className="eyebrow"><ScanSearch size={14} /> مساحة التحليل البصري التجريبية</div><h2 id="source-chart-title">{titleAr}</h2>
            <p>قراءة الاقتراحات على الصورة الأصلية. لا تُستورد هذه النتائج إلى الشجرة؛ الاستيراد يتطلب إدخالاً ومراجعة مستقلين.</p></div>
          <button className="source-chart-close" type="button" aria-label="إغلاق" onClick={() => { if (dirty && !window.confirm("توجد تعديلات غير محفوظة. أغلق دون حفظ؟")) return; onClose(); }}><X size={18} /></button>
        </header>
        {analysis ? <>
          <div className="source-chart-quality" role="status">
            <AlertTriangle size={17} />
            <div><strong>{qualityLabel(analysis.qualityStatus)} · غير مؤهل للاستيراد الآلي</strong><span>هدف التقييم: دقة أسماء 99% واستدعاء 95%. لا يوجد حتى الآن معيار ذهبي محكّم أو دليل على وثائق مستقلة محجوبة.</span></div>
            <span className={`source-chart-run-state is-${analysis.status}`}>{runStatusLabel(analysis.status)}</span>
          </div>
          {analysis.errorCode ? <div className="source-chart-error" role="alert">تعذر التحليل: {errorCodeLabel(analysis.errorCode)}.</div> : null}
          <div className="source-chart-metrics">
            <Metric label="مناطق أسماء مقترحة" value={analysis.quality.nameOccurrenceCount} />
            <Metric label="غير مقروءة" value={analysis.quality.unreadableNameCount} />
            <Metric label="تحتاج مراجعة" value={analysis.quality.unreviewedNameCount} />
            <Metric label="حدود فروع غير محسومة" value={analysis.quality.unreviewedBranchCount} />
            <Metric label="الوحدات المتعثرة" value={analysis.quality.failedTileCount} />
            <Metric label="قراءات متعثرة/معلقة" value={analysis.quality.failedUnitCount + analysis.quality.pendingUnitCount} />
            <Metric label="طلبات الرؤية" value={analysis.quality.providerCallCount} />
            <Metric label="حجم الصور" value={formatBytes(analysis.quality.inputImageBytes)} />
          </div>
          {busy ? <div className="source-chart-running"><span className="source-chart-spinner" /> جارٍ فحص المناطق وتسجيل كل وحدة؛ قد تستغرق اللوحة وقتاً.</div> : null}
          <div className="source-chart-workspace">
            <div className="source-chart-visual-column">
              <div className="source-chart-tools">
                <div className="source-chart-tool-group">
                  <button type="button" className={drawMode === "name" ? "is-active" : ""} disabled={busy} onClick={() => setDrawTool("name")}><Plus size={14} /> اسم مفقود</button>
                  <button type="button" className={drawMode === "crop" ? "is-active" : ""} disabled={busy || !selectedName} onClick={() => setDrawTool("crop")}><ScanSearch size={14} /> تصحيح قص الاسم</button>
                  <button type="button" className={drawMode === "branch" ? "is-active" : ""} disabled={busy} onClick={() => setDrawTool("branch")}><GitBranch size={14} /> {selectedBranch ? "تصحيح حدود الفرع" : "إضافة فرع"}</button>
                  <button type="button" className={drawMode === "connector" ? "is-active" : ""} disabled={busy} onClick={() => setDrawTool("connector")}><GitBranch size={14} /> رسم اتصال</button>
                </div>
                <div className="source-chart-zoom-tools">
                  <button type="button" aria-label="تصغير" onClick={() => setZoom((value) => Math.max(1, value - .25))}><Minus size={14} /></button>
                  <span>{Math.round(zoom * 100)}%</span>
                  <button type="button" aria-label="تكبير" onClick={() => setZoom((value) => Math.min(4, value + .25))}><Plus size={14} /></button>
                  <button type="button" aria-label="إعادة ضبط العرض" onClick={() => { setZoom(1); setPan({ x: 0, y: 0 }); }}><Maximize2 size={14} /></button>
                </div>
              </div>
              {drawMode ? <div className="source-chart-draw-hint">{drawHint(drawMode)} · اسحب على الصورة لتحديد المنطقة. اسحب الصورة للتحريك عند إغلاق أداة الرسم.</div> : null}
              <div className="source-chart-canvas" style={{ aspectRatio: pageAspect }}>
                <div className="source-chart-canvas-content" style={{ transform: `translate(${pan.x}px, ${pan.y}px) scale(${zoom})` }}>
                  {previewUrl ? <img src={previewUrl} alt="الصورة الأصلية للوحة" draggable={false} /> : <div className="source-chart-preview-loading">{analysisQuery.isPending ? "جارٍ تحميل التحليل…" : "جارٍ تحميل معاينة المصدر…"}</div>}
                  <svg ref={svgRef} viewBox="0 0 1 1" preserveAspectRatio="none" className={drawMode ? "is-drawing" : ""} onPointerDown={pointerDown} onPointerMove={pointerMove} onPointerUp={pointerUp} onPointerCancel={() => { setDrawing([]); panOrigin.current = null; }} onKeyDown={(event) => { if (event.key === "Escape") { setDrawMode(null); setDrawing([]); } }} tabIndex={0} aria-label="صورة اللوحة وطبقات المراجعة">
                    {layers.branches ? branches.map((branch) => <polygon key={branch.id} className={`source-chart-branch${branch.id === selectedBranchId ? " is-selected" : ""}`} points={branch.polygon.map(([x, y]) => `${x},${y}`).join(" ")} />) : null}
                    {layers.residual ? analysis.residualRegions.map((region, index) => <rect key={`residual-${index}`} className="source-chart-residual" x={region.x} y={region.y} width={region.width} height={region.height} />) : null}
                    {layers.connectors ? connectors.map((connector) => <polyline key={connector.id} className={`source-chart-connector is-${connector.status}`} points={connector.points.map(([x, y]) => `${x},${y}`).join(" ")} />) : null}
                    {layers.names ? names.map((name) => <g key={name.id} className={`source-chart-name is-${name.reviewStatus}`} onClick={(event) => { if (!drawMode) { event.stopPropagation(); setSelectedNameId(name.id); } }}>
                      <rect x={name.region.x} y={name.region.y} width={name.region.width} height={name.region.height} />
                    </g>) : null}
                    {drawing.length > 1 ? drawMode === "connector" ? <polyline className="source-chart-live-connector" points={drawing.map(({ x, y }) => `${x},${y}`).join(" ")} /> : <rect className="source-chart-live-box" x={Math.min(drawing[0].x, drawing[drawing.length - 1].x)} y={Math.min(drawing[0].y, drawing[drawing.length - 1].y)} width={Math.abs(drawing[0].x - drawing[drawing.length - 1].x)} height={Math.abs(drawing[0].y - drawing[drawing.length - 1].y)} /> : null}
                  </svg>
                </div>
              </div>
              <div className="source-chart-map-row">
                <div className="source-chart-mini-map" style={{ aspectRatio: pageAspect }} aria-label="خريطة التغطية المصغرة">
                  {previewUrl ? <img src={previewUrl} alt="" /> : null}
                  {analysis.tiles.map((tile) => <button key={tile.id} type="button" title={`${tile.id}: ${tile.incomplete ? "dense, needs human completion" : tile.subdivisionReason ? subdivisionReasonLabel(tile.subdivisionReason) : tile.status}`} className={`source-chart-mini-tile is-${tile.status}${tile.incomplete ? " is-incomplete" : ""}${reviewedTileIds.includes(tile.id) ? " is-reviewed" : ""}`} style={{ left: `${tile.bounds.x / Math.max(analysis.pageWidth, 1) * 100}%`, top: `${tile.bounds.y / Math.max(analysis.pageHeight, 1) * 100}%`, width: `${tile.bounds.width / Math.max(analysis.pageWidth, 1) * 100}%`, height: `${tile.bounds.height / Math.max(analysis.pageHeight, 1) * 100}%` }} onClick={() => markTileReviewed(tile.id)} />)}
                </div>
                <div className="source-chart-tile-ledger"><strong>سجل تغطية المناطق · {reviewedTileIds.length}/{analysis.tiles.length} راجعها إنسانياً</strong>
                  <div>{analysis.tiles.map((tile) => <button key={tile.id} type="button" disabled={tile.status !== "succeeded"} className={`source-chart-tile-row is-${tile.status}${tile.incomplete ? " is-incomplete" : ""}${reviewedTileIds.includes(tile.id) ? " is-reviewed" : ""}`} onClick={() => markTileReviewed(tile.id)}><span>{tile.id.replace("p1_", "منطقة ").replaceAll("_", " · ")}</span><b>{reviewedTileIds.includes(tile.id) ? "مراجعة" : tile.incomplete ? "كثيفة · أكملها يدوياً" : tileStatusLabel(tile.status)}{tile.subdivisionReason ? <small> · {subdivisionReasonLabel(tile.subdivisionReason)}</small> : null}</b></button>)}</div>
                </div>
              </div>
              <div className="source-chart-layer-toggles">
                {Object.entries({ branches: "الفروع", names: "الأسماء", connectors: "المسارات", residual: "المناطق المتبقية" }).map(([key, label]) => <label key={key}><input type="checkbox" checked={layers[key as keyof typeof layers]} onChange={(event) => setLayers((value) => ({ ...value, [key]: event.target.checked }))} />{label}</label>)}
              </div>
              {selectedName && previewUrl ? <div className="source-chart-crop-comparison"><strong><Eye size={14} /> مقارنة منطقة الاسم بالسياق</strong><div className="source-chart-crop-window" style={{ aspectRatio: Math.max(selectedName.region.width, .02) / Math.max(selectedName.region.height, .02) }}><img src={previewUrl} alt="قصاصة سياق للاسم المحدد" style={{ width: `${100 / Math.max(selectedName.region.width, .02)}%`, left: `${-selectedName.region.x / Math.max(selectedName.region.width, .02) * 100}%`, top: `${-selectedName.region.y / Math.max(selectedName.region.height, .02) * 100}%` }} /></div><small>المعاينة مشتقة محلياً من صورة الصفحة؛ المقارنة لا تغيّر دقة التحليل أو مصدر الصورة.</small></div> : null}
            </div>
            <aside className="source-chart-review-column">
              <div className="source-chart-review-section">
                <div className="source-chart-section-title"><strong>الفروع المقترحة</strong><span>{branches.length}</span></div>
                <label className="source-chart-field">الفرع النشط<select value={selectedBranchId} onChange={(event) => setSelectedBranchId(event.target.value)}><option value="">لا يوجد فرع محدد</option>{branches.map((branch, index) => <option key={branch.id} value={branch.id}>{branch.label || `فرع ${index + 1}`}{branch.reviewerEdited ? " · مُصحّح" : ""}</option>)}</select></label>
                {selectedBranch ? <>
                  <label className="source-chart-field">اسم الفرع<input value={selectedBranch.label ?? ""} disabled={busy} onChange={(event) => { setBranches((current) => current.map((branch) => branch.id === selectedBranch.id ? { ...branch, label: event.target.value, reviewStatus: "needs_review" } : branch)); setDirty(true); }} /></label>
                  <div className="source-chart-review-actions"><button type="button" disabled={busy} onClick={() => { setBranches((current) => current.map((branch) => branch.id === selectedBranch.id ? { ...branch, reviewStatus: "confirmed" } : branch)); setDirty(true); }}><Check size={14} /> حد صحيح</button><button type="button" disabled={busy} onClick={() => { setBranches((current) => current.map((branch) => branch.id === selectedBranch.id ? { ...branch, reviewStatus: "rejected" } : branch)); setDirty(true); }}>ليس فرعاً</button></div>
                </> : null}
                <small>اختر فرعاً ثم استخدم أداة تصحيح الحدود وارسم مستطيلاً يغطيه. الأسماء المرئية خارج الفروع يمكن إضافتها يدوياً.</small>
              </div>
              <div className="source-chart-review-section">
                <div className="source-chart-section-title"><strong>الأسماء · {visibleNames.length} من {names.length}</strong><span>{analysis.quality.unreviewedNameCount} معلقة</span></div>
                <div className="source-chart-filters"><select aria-label="تصفية الأسماء" value={nameFilter} onChange={(event) => setNameFilter(event.target.value)}><option value="all">كل المواضع</option><option value="needs_review">تحتاج مراجعة</option><option value="verified">مؤكدة بصرياً</option><option value="unreadable">غير مقروءة</option></select><select aria-label="تصفية الفرع" value={branchFilter} onChange={(event) => setBranchFilter(event.target.value)}><option value="">كل الفروع</option>{branches.map((branch, index) => <option key={branch.id} value={branch.id}>{branch.label || `فرع ${index + 1}`}</option>)}</select></div>
                <div className="source-chart-name-list">{visibleNames.length === 0 ? <p className="source-chart-empty">لا توجد مواضع في هذا الاختيار. راجع المناطق المتبقية وأضف أي اسم فات الكشف.</p> : visibleNames.map((name, index) => <button type="button" key={name.id} className={`source-chart-name-row${selectedNameId === name.id ? " is-selected" : ""}`} onClick={() => setSelectedNameId(name.id)}><span>{name.textAr || `موضع اسم ${index + 1}`}</span><small>{name.reviewStatus === "verified" ? "مؤكد" : name.reviewStatus === "rejected" ? "مرفوض" : name.readingStatus === "unreadable" ? "غير مقروء" : "مراجعة"}</small></button>)}</div>
                {selectedName ? <div className="source-chart-name-editor">
                  <div className="source-chart-section-title"><strong>النص المرئي كما هو</strong><span>{selectedName.id.startsWith("manual_") ? "أُضيف يدوياً" : "اقتراح آلي"}</span></div>
                  <div className="source-chart-alternatives">{selectedName.alternatives?.length ? <>بدائل الرؤية: {selectedName.alternatives.join(" · ")}</> : "لا توجد قراءة بديلة."}{selectedName.uncertainSpans?.length ? <p>مقاطع غير مؤكدة: {selectedName.uncertainSpans.join("، ")}</p> : null}</div>
                  <textarea aria-label="نسخ الاسم العربي من الصورة" dir="rtl" value={selectedName.textAr} disabled={busy} onChange={(event) => setNamePatch(selectedName.id, { textAr: event.target.value, reviewStatus: "needs_review" })} rows={2} placeholder="انسخ الحروف الظاهرة فقط؛ لا تكمل النسب من السياق" />
                  <label className="source-chart-field">الفرع<select value={selectedName.branchId ?? ""} onChange={(event) => setNamePatch(selectedName.id, { branchId: event.target.value || undefined })}><option value="">غير محدد</option>{branches.map((branch, index) => <option key={branch.id} value={branch.id}>{branch.label || `فرع ${index + 1}`}</option>)}</select></label>
                  <div className="source-chart-review-actions"><button type="button" disabled={busy || !selectedName.textAr.trim()} onClick={() => setNamePatch(selectedName.id, { reviewStatus: "verified", readingStatus: "readable" })}><Check size={14} /> تحقق من الصورة</button><button type="button" disabled={busy} onClick={() => setNamePatch(selectedName.id, { reviewStatus: "unreadable", readingStatus: "unreadable" })}>غير مقروء</button><button type="button" disabled={busy} onClick={() => setNamePatch(selectedName.id, { reviewStatus: "rejected" })}>ليس اسماً</button></div>
                  {selectedName.sourceOccurrenceIds?.length ? <small>مصادر موضع الكشف: {selectedName.sourceOccurrenceIds.length} بلاطة/تكرار متداخل؛ لا تُدمج التكرارات في مواقع مختلفة.</small> : null}
                </div> : null}
              </div>
              <div className="source-chart-review-section">
                <div className="source-chart-section-title"><strong>مسارات الاتصال</strong><span>{connectors.length}</span></div>
                {connectors.length === 0 ? <p className="source-chart-empty">لم تُرصد مسارات حتى الآن. يمكن رسمها يدوياً على الصورة.</p> : <div className="source-chart-connector-list">{connectors.map((connector, index) => <div className="source-chart-connector-card" key={connector.id}>
                  <strong>مسار {index + 1} · {connector.status === "reviewed" ? "مُتتبّع" : connector.status === "uncertain" ? "غير واضح" : "بانتظار التحقق"}</strong>
                  <div className="source-chart-endpoints"><select aria-label={`بداية المسار ${index + 1}`} value={connector.startNameId ?? ""} onChange={(event) => setConnectorPatch(connector.id, { startNameId: event.target.value || undefined })}><option value="">طرف أول</option>{names.map((name, n) => <option key={name.id} value={name.id}>{name.textAr || `موضع ${n + 1}`}</option>)}</select><ChevronDown size={13} /><select aria-label={`نهاية المسار ${index + 1}`} value={connector.endNameId ?? ""} onChange={(event) => setConnectorPatch(connector.id, { endNameId: event.target.value || undefined })}><option value="">طرف ثانٍ</option>{names.map((name, n) => <option key={name.id} value={name.id}>{name.textAr || `موضع ${n + 1}`}</option>)}</select></div>
                  <div className="source-chart-review-actions"><button type="button" disabled={!connector.startNameId || !connector.endNameId || connector.startNameId === connector.endNameId || names.find((name) => name.id === connector.startNameId)?.reviewStatus !== "verified" || names.find((name) => name.id === connector.endNameId)?.reviewStatus !== "verified"} onClick={() => setConnectorPatch(connector.id, { status: "reviewed" })}><Check size={14} /> تحقق من المسار</button><button type="button" disabled={busy} onClick={() => setConnectorPatch(connector.id, { status: "uncertain" })}>غير واضح</button></div>
                  <small>توثيق خط بين نقطتين لا يفسر العلاقة ولا يثبت الأبوة؛ تفسيرها ومراجعتها يتمان لاحقاً يدوياً.</small>
                </div>)}</div>}
              </div>
              <div className="source-chart-review-section source-chart-coverage-checks">
                <strong>إغلاق مراجعة الصفحة</strong>
                <label><input type="checkbox" checked={coverageConfirmed} disabled={!allTilesReviewed || hasFailedUnits || busy} onChange={(event) => { setCoverageConfirmed(event.target.checked); setDirty(true); }} /> راجعت كل البلاطات والصفحة كاملة بحثاً عن أسماء وفروع فائتة.</label>
                <label><input type="checkbox" checked={residualReviewed} disabled={!coverageConfirmed || busy} onChange={(event) => { setResidualReviewed(event.target.checked); setDirty(true); }} /> راجعت المناطق المتبقية، والتعارضات، والمواضع غير المقروءة، والوصلات عند الحواف.</label>
                {!allTilesReviewed ? <small>علّم كل بلاطة ناجحة في سجل التغطية بعد فحصها بصرياً. اكتمال المعالجة وحده لا يؤكد اكتمال الكشف.</small> : null}
              </div>
              {analysis.quality.reasons.length ? <div className="source-chart-reasons"><strong>ما يمنع التأهيل</strong>{analysis.quality.reasons.map((reason) => <span key={reason}>• {reasonLabel(reason)}</span>)}</div> : null}
            </aside>
          </div>
          {message ? <div className="source-chart-message" role="status">{message}</div> : null}
          {error ? <div className="source-chart-error" role="alert">{error}</div> : null}
          <footer className="source-chart-footer">
            <div><span>الطراز: {analysis.modelVersion || "غير مهيأ"}</span><span>التشغيل: {runStatusLabel(analysis.status)}</span><span>مراجعة بشرية: {analysis.reviewRevision}</span></div>
            <div className="source-chart-footer-actions">
              {analysis.status === "partial" || analysis.status === "failed" ? <button type="button" disabled={retryMutation.isPending} onClick={() => retryMutation.mutate()}><RotateCcw size={14} /> {retryMutation.isPending ? "جارٍ إعادة المحاولة…" : "استئناف الوحدات المتعثرة"}</button> : null}
              <button type="button" className="source-chart-manual" onClick={() => onManualDraft(fileId)}><GitBranch size={14} /> متابعة بالإدخال اليدوي</button>
              <button type="button" className="source-chart-save" disabled={!dirty || saveMutation.isPending || busy} onClick={save}><Save size={14} /> {saveMutation.isPending ? "جارٍ الحفظ…" : "حفظ مراجعة التحليل"}</button>
            </div>
          </footer>
        </> : <p className="source-chart-running">جارٍ تحميل سجل التغطية…</p>}
      </section>
    </div>
  );
}

function Metric({ label, value }: { label: string; value: string | number }) { return <div><span>{label}</span><strong>{value}</strong></div>; }
function normalizedRect(first: Point, last: Point) { return { x: Math.min(first.x, last.x), y: Math.min(first.y, last.y), width: Math.abs(first.x - last.x), height: Math.abs(first.y - last.y) }; }
function clampUnit(value: number) { return Math.max(0, Math.min(1, value)); }
function formatBytes(value: number) { return value < 1024 ? `${value} ب` : `${(value / 1024 / 1024).toFixed(1)} م.ب`; }
function runStatusLabel(value: string) { return ({ queued: "في الطابور", running: "قيد التحليل", succeeded: "اكتمل التحليل", partial: "اكتمل جزئياً", failed: "تعذر التحليل" } as Record<string, string>)[value] ?? value; }
function qualityLabel(value: string) { return value === "manual_required" ? "يتطلب إدخالاً يدوياً" : value === "qualified_for_assisted_review" ? "مؤهل لمراجعة مساعدة" : "بانتظار الدليل"; }
function tileStatusLabel(value: string) { return ({ pending: "بانتظار الفحص", running: "قيد الفحص", succeeded: "معالجة فقط", failed: "متعذرة" } as Record<string, string>)[value] ?? value; }
function drawHint(value: DrawMode) { return value === "name" ? "تحديد اسم فات الكشف" : value === "crop" ? "تصحيح حدود القصاصة؛ ستُبطل القراءة الحالية" : value === "branch" ? "تحديد حدود الفرع" : value === "connector" ? "تتبع مسار الاتصال الظاهر" : ""; }
function reasonLabel(value: string) { return ({ benchmark_not_qualified: "المقارنة لم تحقق معياراً ذهبياً مستقلاً.", manual_source_review_required: "كل اسم واتصال يحتاج إلى تحقق بشري من الأصل.", page_coverage_incomplete: "لم تُعالج كل البلاطات بنجاح.", analysis_units_failed: "وحدات من التحليل أو قراءة القصاصات تعثرت.", analysis_units_pending: "بعض وحدات التحليل لم تكتمل.", dense_regions_need_manual_subdivision: "منطقة كثيفة بلغت حد الاكتشاف؛ أكمل مراجعتها وأضف أي أسماء فائتة يدوياً.", full_page_coverage_not_human_confirmed: "لم يؤكد مراجع بشري اكتمال البحث في كامل الصفحة.", residual_regions_not_reviewed: "لم تُغلق مراجعة المناطق المتبقية.", reviewer_names_pending: "توجد أسماء لم تُراجع.", reviewer_branches_pending: "توجد حدود فروع لم تُراجع.", connector_paths_pending: "توجد مسارات لم تُتبع أو تراجع.", chart_style_not_qualified: "هذا النمط من الرسوم غير مؤهل." } as Record<string, string>)[value] ?? value.replaceAll("_", " "); }
function subdivisionReasonLabel(value: string) { return ({ provider_output_truncated: "قُسم بعد بلوغ حد الإخراج.", label_output_limit: "قُسم بعد بلوغ حد عدد الأسماء.", reader_reported_dense_or_truncated: "قُسمت المنطقة لكثافتها أو نقص مخرجاتها." } as Record<string, string>)[value] ?? value.replaceAll("_", " "); }
function errorCodeLabel(value: string) {
  const labels: Record<string, string> = {
    provider_request_failed: "تعذر الاتصال بمزود الرؤية بعد المحاولات المحدودة",
    provider_request_rejected: "رفض مزود الرؤية الطلب؛ راجع الإعدادات قبل إعادة المحاولة",
    hosted_vision_setup_required: "يلزم إعداد مفتاح DeepSeek؛ لم تُرسل صورة",
    request_budget_exhausted: "انتهت ميزانية الطلبات؛ يمكن استئناف الوحدات المتبقية",
    request_image_too_large: "قصاصة الصورة أكبر من حد الطلب",
    model_output_invalid: "خرج النموذج عن بنية القراءة المقبولة؛ لم تُحفظ القراءة",
    provider_output_invalid_geometry: "إحداثيات أو أشكال النموذج لم تجتز التحقق؛ لم تُحفظ القراءة",
    provider_output_invalid_branch_reference: "أشار النموذج إلى فرع غير موجود؛ لم تُحفظ القراءة",
    provider_output_invalid_reading: "بنية قراءة الاسم لم تجتز التحقق؛ لم يُحفظ النص",
    provider_output_invalid_schema: "خرج النموذج عن البنية المقبولة؛ لم تُحفظ القراءة",
    provider_output_truncated_after_subdivision: "ظل الإخراج مبتوراً بعد تقسيم المنطقة",
    pipeline_configuration_changed: "تغير إصدار مسار التحليل؛ أُوقف الاستئناف لحماية اتساق النتائج",
    unsupported_page_count: "يدعم النموذج التجريبي صفحة واحدة فقط",
    hosted_vision_unavailable: "خدمة الرؤية أو إعداداتها غير متاحة",
    source_checksum_changed: "تغير ملف المصدر",
  };
  return labels[value] ?? value.replaceAll("_", " ");
}
function errorMessage(error: unknown) { return error instanceof Error ? error.message : "حدث خطأ غير متوقع."; }
