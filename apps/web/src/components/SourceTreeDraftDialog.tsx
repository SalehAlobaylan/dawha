import { Link } from "@tanstack/react-router";
import { AlertTriangle, Check, GitBranch, Minus, Plus, RotateCcw, Save, X } from "lucide-react";
import { useEffect, useMemo, useRef, useState, type PointerEvent as ReactPointerEvent } from "react";
import { fetchSourcePagePreview, importSourceTreeDraft, previewSourceTreeDraftImport, retractSourceTreeDraft, saveSourceTreeDraft } from "../lib/api";
import { ApiError } from "../lib/api";
import type { SourceTreeDraft, SourceTreeDraftEdge, SourceTreeDraftImportPreview, SourceTreeDraftNode, SourceTreeDraftPoint } from "../types";

interface SourceTreeDraftDialogProps {
  initialDraft: SourceTreeDraft;
  onClose: () => void;
  onChanged?: (draft: SourceTreeDraft) => void;
}

type DrawMode = "node" | "edge" | null;

export function SourceTreeDraftDialog({ initialDraft, onClose, onChanged }: SourceTreeDraftDialogProps) {
  const [draft, setDraft] = useState(initialDraft);
  const [title, setTitle] = useState(initialDraft.titleAr);
  const [nodes, setNodes] = useState(initialDraft.nodes);
  const [edges, setEdges] = useState(initialDraft.edges);
  const [activePageID, setActivePageID] = useState(initialDraft.pages[0]?.pageEvidenceId ?? "");
  const [subjectID, setSubjectID] = useState("");
  const [objectID, setObjectID] = useState("");
  const [predicate, setPredicate] = useState<SourceTreeDraftEdge["predicate"]>("parent_of");
  const [drawMode, setDrawMode] = useState<DrawMode>(null);
  const [activeTraceEdgeID, setActiveTraceEdgeID] = useState("");
  const [drawing, setDrawing] = useState<SourceTreeDraftPoint[]>([]);
  const [dirty, setDirty] = useState(false);
  const [closeRequested, setCloseRequested] = useState(false);
  const [zoom, setZoom] = useState(1);
  const [previewURL, setPreviewURL] = useState("");
  const [working, setWorking] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const svgRef = useRef<SVGSVGElement>(null);
  const dialogRef = useRef<HTMLElement>(null);
  useEffect(() => {
    const previous = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    const overflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    dialogRef.current?.focus();
    return () => { document.body.style.overflow = overflow; previous?.focus(); };
  }, []);
  const activePage = initialDraft.pages.find((page) => page.pageEvidenceId === activePageID) ?? initialDraft.pages[0];
  const activePageNumber = activePage?.pageNumber;
  const activePageEvidenceID = activePage?.pageEvidenceId;
  const includedNodes = useMemo(() => nodes.filter((node) => node.included), [nodes]);
  const graphErrors = useMemo(() => {
    const reasons: string[] = [];
    if (includedNodes.length === 0) reasons.push("اختر اسماً واحداً على الأقل لاستيراده.");
    if (includedNodes.some((node) => !node.reviewed || node.status !== "reviewed")) reasons.push("راجع كل اسم اخترته قبل الاستيراد.");
    if (edges.some((edge) => edge.status === "proposed")) reasons.push("تتبّع كل وصلة أضفتها على الصورة أو احذفها قبل المتابعة.");
    if (edges.some((edge) => edge.status === "reviewed" && edge.predicate === "parent_of") && hasParentCycle(nodes, edges)) reasons.push("توجد حلقة في اتجاهات الأبوة؛ راجع الوصلات قبل الاستيراد.");
    if (edges.some((edge) => edge.status === "reviewed" && edge.secondReviewRequired && edge.reviewerCount < 2)) reasons.push("تحتاج بعض الوصلات غير الواضحة إلى مراجع مستقل ثانٍ.");
    return reasons;
  }, [edges, includedNodes, nodes]);
  const fileWarnings = draft.warnings.filter((warning) => !["No people are selected", "Every included name needs", "Reviewed parent relationships contain"].some((prefix) => warning.startsWith(prefix))).map(warningLabel);
  useEffect(() => {
    if (!dirty) return;
    const warnOnLeave = (event: BeforeUnloadEvent) => { event.preventDefault(); event.returnValue = ""; };
    window.addEventListener("beforeunload", warnOnLeave);
    return () => window.removeEventListener("beforeunload", warnOnLeave);
  }, [dirty]);

  useEffect(() => {
    if (activePageNumber === undefined) return;
    let active = true;
    let objectURL = "";
    setPreviewURL("");
    void fetchSourcePagePreview(initialDraft.sourceFileId, activePageNumber).then((blob) => {
      objectURL = URL.createObjectURL(blob);
      if (active) setPreviewURL(objectURL);
      else URL.revokeObjectURL(objectURL);
    }).catch((cause: unknown) => {
      if (active) setError(errorMessage(cause));
    });
    return () => {
      active = false;
      if (objectURL) URL.revokeObjectURL(objectURL);
    };
  }, [activePageNumber, activePageEvidenceID, initialDraft.sourceFileId]);

  const pointFromEvent = (event: ReactPointerEvent<SVGSVGElement>): SourceTreeDraftPoint => {
    const bounds = event.currentTarget.getBoundingClientRect();
    if (bounds.width <= 0 || bounds.height <= 0) return { x: 0, y: 0 };
    return { x: clampUnit((event.clientX - bounds.left) / bounds.width), y: clampUnit((event.clientY - bounds.top) / bounds.height) };
  };
  const pointerDown = (event: ReactPointerEvent<SVGSVGElement>) => {
    if (working || !drawMode || !activePage) return;
    event.preventDefault();
    event.currentTarget.setPointerCapture(event.pointerId);
    setDrawing([pointFromEvent(event)]);
  };
  const pointerMove = (event: ReactPointerEvent<SVGSVGElement>) => {
    if (drawing.length === 0) return;
    const point = pointFromEvent(event);
    setDrawing((current) => current.length >= 300 ? current : [...current, point]);
  };
  const pointerUp = (event: ReactPointerEvent<SVGSVGElement>) => {
    if (drawing.length === 0 || !activePage) return;
    const points = [...drawing, pointFromEvent(event)];
    if (drawMode === "node") {
      const first = points[0];
      const last = points[points.length - 1];
      const x = Math.min(first.x, last.x);
      const y = Math.min(first.y, last.y);
      const width = Math.abs(first.x - last.x);
      const height = Math.abs(first.y - last.y);
      if (width > 0.008 && height > 0.004) {
        const node: SourceTreeDraftNode = {
          id: crypto.randomUUID(), nameAr: "اسم يحتاج إلى نسخ", pageEvidenceId: activePage.pageEvidenceId,
          region: { kind: "manual_name", text: "", x, y, width, height }, included: false, reviewed: false, status: "proposed",
        };
        setNodes((current) => [...current, node]);
        setDirty(true);
        setMessage("أُضيف موضع اسم يدوي. اكتب الاسم كما يظهر في الأصل ثم علّمه كمراجع.");
      }
    } else if (drawMode === "edge" && activeTraceEdgeID && points.length >= 2) {
      setEdges((current) => current.map((edge) => edge.id === activeTraceEdgeID ? { ...edge, pageEvidenceId: activePage.pageEvidenceId, path: points, status: "reviewed" } : edge));
      setDirty(true);
      setMessage("حُفظ مسار الوصلة كمراجَع. تأكد أن طرفيه يصلان إلى الاسمين الصحيحين.");
    }
    setDrawing([]);
    setDrawMode(null);
    setActiveTraceEdgeID("");
  };

  const updateNode = (nodeID: string, patch: Partial<SourceTreeDraftNode>) => {
    setNodes((current) => current.map((node) => node.id === nodeID ? { ...node, ...patch } : node));
    setDirty(true);
  };
  const addRelation = () => {
    if (working || !subjectID || !objectID || subjectID === objectID || !activePage) return;
    const edge: SourceTreeDraftEdge = {
      id: crypto.randomUUID(), subjectNodeId: subjectID, objectNodeId: objectID,
      predicate, pageEvidenceId: activePage.pageEvidenceId, path: [], status: "proposed",
      secondReviewRequired: false, reviewerCount: 0, reviewedByMe: false,
    };
    setEdges((current) => [...current, edge]);
    setDirty(true);
  };
  const save = async () => {
    setWorking(true); setError(""); setMessage("");
    try {
      const result = await saveSourceTreeDraft(draft.id, { expectedRevision: draft.revision, titleAr: title, nodes, edges });
      setDraft(result); setNodes(result.nodes); setEdges(result.edges); setTitle(result.titleAr); setDirty(false);
      setMessage(`حُفظت المراجعة رقم ${result.revision}.`); onChanged?.(result);
      return result;
    } catch (cause) {
      setError(cause instanceof ApiError && cause.status === 409 ? "تغيرت المسودة في جلسة أخرى. أعد فتحها لتحميل أحدث مراجعة." : errorMessage(cause));
    } finally { setWorking(false); }
  };
  const [preview, setPreview] = useState<SourceTreeDraftImportPreview | null>(null);
  const requestClose = () => {
    if (working) return;
    if (dirty) setCloseRequested(true);
    else onClose();
  };
  const runImport = async (pending: SourceTreeDraftImportPreview) => {
    setWorking(true); setError("");
    try {
      const result = await importSourceTreeDraft(draft.id, {
        expectedRevision: draft.revision,
        acknowledgePreview: true,
        sourceFileChecksum: pending.sourceFileChecksum,
        expectedNewPeople: pending.newPeople,
        expectedLinkedPeople: pending.linkedExisting,
        expectedCollisions: pending.collisions.length,
        expectedRelations: pending.relations,
        expectedStatements: pending.statements,
        expectedAliases: pending.aliases,
      });
      setPreview(null);
      const updated: SourceTreeDraft = { ...draft, state: "imported", treeId: result.treeId, treeVersionId: result.treeVersionId };
      setDraft(updated); onChanged?.(updated);
      const linkedNote = result.linkedExistingPeople > 0 ? ` ورُبط ${result.linkedExistingPeople} اسم بالموجودين بدل تكرارهم` : "";
      setMessage(`أُنشئت مسودة خاصة تضم ${result.importedPeople} أسماء${linkedNote} و${result.importedRelations} علاقات غير محسومة، ومُثّل وصل الشجرة بمشاهد بشواهد مستوردة. لم تُنشر الشجرة، ولم يُدمج شيء إلا بقرار مراجع لاحق.`);
    } catch (cause) {
      setPreview(null);
      setError(cause instanceof ApiError && cause.status === 409 ? "رفض الاستيراد لأن تقرير الفحص لم يعد مطابقاً أو بقي تعارض بنيوي. أعد الفحص المبدئي ثم أكّد." : errorMessage(cause));
    } finally { setWorking(false); }
  };
  const applyImport = async () => {
    const saved = dirty ? await save() : draft;
    if (!saved) return;
    setWorking(true); setError(""); setMessage("");
    try {
      const report = await previewSourceTreeDraftImport(saved.id, saved.revision);
      setPreview(report);
      setMessage("");
    } catch (cause) {
      setError(errorMessage(cause));
    } finally { setWorking(false); }
  };
  const retractImport = async () => {
    setWorking(true); setError(""); setMessage("");
    try {
      const result = await retractSourceTreeDraft(draft.id);
      const updated: SourceTreeDraft = { ...draft, state: "retracted", treeVersionId: result.supersedingVersionId };
      setDraft(updated); onChanged?.(updated);
      setMessage("تراجع الاستيراد بإنشاء نسخة مسودة جديدة تستبعد عناصره، مع الاحتفاظ بسجل النسخة السابقة.");
    } catch (cause) {
      setError(cause instanceof ApiError && cause.status === 409 ? "لا يمكن التراجع بأمان لأن الشجرة تغيّرت أو تعتمد أعمال لاحقة على هذه الأسماء. راجع النسخة يدوياً." : errorMessage(cause));
    } finally { setWorking(false); }
  };

  const isImported = draft.state === "imported";
  const currentPreview = preview && !dirty && preview.expectedRevision === draft.revision ? preview : null;
  const pageNodes = activePage ? nodes.filter((node) => node.pageEvidenceId === activePage.pageEvidenceId) : [];
  const pageEdges = activePage ? edges.filter((edge) => edge.pageEvidenceId === activePage.pageEvidenceId && edge.path.length > 1) : [];
  const drawPrompt = drawMode === "node" ? "اسحب حول الاسم الذي لم يلتقطه OCR" : drawMode === "edge" ? "تتبّع الوصلة من الاسم الأول حتى الاسم الثاني" : "";

  return (
    <div className="source-tree-draft-backdrop" role="presentation" onClick={requestClose}>
      <section ref={dialogRef} tabIndex={-1} className="source-tree-draft-dialog" role="dialog" aria-modal="true" aria-labelledby="source-tree-draft-title" onClick={(event) => event.stopPropagation()} onKeyDown={(event) => {
        if (event.key === "Escape") {
          event.stopPropagation();
          if (drawMode) { setDrawMode(null); setDrawing([]); setActiveTraceEdgeID(""); }
          else requestClose();
        }
        if (event.key === "Tab") {
          const controls = [...event.currentTarget.querySelectorAll<HTMLElement>('button:not(:disabled), a[href], input:not(:disabled), select:not(:disabled), textarea:not(:disabled), [tabindex="0"]')].filter((item) => item.getClientRects().length > 0);
          const first = controls[0]; const last = controls.at(-1);
          if (event.shiftKey && (document.activeElement === first || document.activeElement === event.currentTarget)) { event.preventDefault(); last?.focus(); }
          else if (!event.shiftKey && (document.activeElement === last || document.activeElement === event.currentTarget)) { event.preventDefault(); first?.focus(); }
        }
      }}>
        <header className="source-tree-draft-head">
          <div><div className="eyebrow">استيراد مشجر / المراجعة</div><h2 id="source-tree-draft-title">راجع الأسماء والوصلات</h2><p>١. تحقّق من الأسماء واختر ما تضيفه. ٢. سجّل الوصلات الظاهرة في الصورة. ٣. راجع الملخص وأنشئ الشجرة الخاصة.</p></div>
          <button className="secondary-button" type="button" onClick={requestClose} disabled={working} aria-label="إغلاق"><X size={16} /></button>
        </header>
        {closeRequested ? <div className="source-tree-close-prompt" role="alert"><span>لديك تغييرات لم تُحفظ.</span><button className="primary-button" type="button" disabled={working} onClick={() => void save().then((saved) => { if (saved) onClose(); })}>حفظ وإغلاق</button><button className="secondary-button" type="button" onClick={() => setCloseRequested(false)}>متابعة المراجعة</button><button className="inline-link" type="button" onClick={onClose}>إغلاق دون حفظ</button></div> : null}
        <div className="source-tree-draft-body">
          <div className="source-tree-draft-review">
            <div className="source-tree-page-tools">
              <label className="composer-label">الصفحة الأصلية
                <select disabled={working} value={activePage?.pageEvidenceId ?? ""} onChange={(event) => setActivePageID(event.target.value)}>
                  {draft.pages.map((page) => <option key={page.pageEvidenceId} value={page.pageEvidenceId}>صفحة {page.pageNumber}</option>)}
                </select>
              </label>
              <button className={`secondary-button${drawMode === "node" ? " is-active" : ""}`} type="button" disabled={working || isImported || !activePage} onClick={() => { setDrawMode(drawMode === "node" ? null : "node"); setDrawing([]); setActiveTraceEdgeID(""); }}><Plus size={14} /> تحديد اسم يدوياً</button>
              <div className="source-tree-zoom"><button className="secondary-button" type="button" aria-label="تصغير المشجر" disabled={zoom <= .5} onClick={() => setZoom((value) => Math.max(.5, value - .5))}><Minus size={15} /></button><button className="secondary-button" type="button" onClick={() => setZoom(1)}>ملاءمة الصفحة</button><button className="secondary-button" type="button" aria-label="تكبير المشجر" disabled={zoom >= 4} onClick={() => setZoom((value) => Math.min(4, value + .5))}><Plus size={15} /></button></div>
            </div>
            {drawPrompt ? <div className="source-tree-draw-hint" role="status">{drawPrompt} · اضغط Esc للإلغاء</div> : null}
            <div className="source-tree-preview-viewport"><div className={`source-tree-preview${drawMode ? " is-drawing" : ""}`} style={{ width: `min(${zoom * 100}%, ${52 * zoom * (activePage && activePage.width > 0 && activePage.height > 0 ? activePage.width / activePage.height : 1)}dvh)`, aspectRatio: activePage && activePage.width > 0 && activePage.height > 0 ? `${activePage.width} / ${activePage.height}` : "1 / 1" }}>
              {previewURL ? <img src={previewURL} alt={`صورة الصفحة ${activePage?.pageNumber ?? ""}`} /> : <div className="source-tree-preview-loading">جارٍ تجهيز معاينة محلية للصفحة…</div>}
              {activePage ? <svg ref={svgRef} viewBox="0 0 1000 1000" preserveAspectRatio="none" aria-label="مربعات الأسماء ومسارات الوصلات" onPointerDown={pointerDown} onPointerMove={pointerMove} onPointerUp={pointerUp} onPointerCancel={() => setDrawing([])} onKeyDown={(event) => { if (event.key === "Escape") { setDrawMode(null); setDrawing([]); setActiveTraceEdgeID(""); } }} tabIndex={drawMode ? 0 : -1}>
                {activePage.lineCandidates.map((region, index) => <rect key={`line-${index}`} className="source-tree-line-candidate" x={region.x * 1000} y={region.y * 1000} width={region.width * 1000} height={Math.max(region.height * 1000, 1)} />)}
                {pageEdges.map((edge) => <polyline key={edge.id} className="source-tree-traced-edge" points={edge.path.map((point) => `${point.x * 1000},${point.y * 1000}`).join(" ")} />)}
                {pageNodes.map((node) => <g key={node.id} className={`source-tree-node-overlay${node.included && node.reviewed ? " is-included" : node.status === "rejected" ? " is-rejected" : ""}`}>
                  <rect x={node.region.x * 1000} y={node.region.y * 1000} width={node.region.width * 1000} height={node.region.height * 1000} />
                  <text x={(node.region.x + node.region.width) * 1000} y={Math.max(node.region.y * 1000 - 3, 12)} textAnchor="end" direction="rtl">{node.nameAr}</text>
                </g>)}
                {drawing.length > 1 ? <polyline className={drawMode === "edge" ? "source-tree-live-edge" : "source-tree-live-box"} points={drawing.map((point) => `${point.x * 1000},${point.y * 1000}`).join(" ")} /> : null}
              </svg> : null}
            </div></div>
            <p className="source-tree-preview-caption">الأصفر خطوط محتملة من كاشف محلي بسيط، وقد تكون حدود مربعات أو حروفاً. تحقّق من الخط في الصورة قبل تسجيل علاقة.</p>
            {activePage?.lineCandidatesTruncated ? <div className="source-tree-warnings" role="status"><p><AlertTriangle size={14} /> بلغ كاشف الخطوط حد الاقتراحات في هذه الصفحة؛ قد تكون بعض الوصلات غير ظاهرة هنا. تتبّع العلاقات يدوياً من الصورة الأصلية.</p></div> : null}
            <div className="source-tree-relation-builder">
              <strong>إضافة علاقة من الرسم</strong>
              <div className="source-tree-relation-fields">
                <label className="composer-label">من<select disabled={working || isImported} value={subjectID} onChange={(event) => setSubjectID(event.target.value)}><option value="">اختر اسماً</option>{nodes.filter((node) => node.included && node.reviewed).map((node) => <option key={node.id} value={node.id}>{node.nameAr}</option>)}</select></label>
                <label className="composer-label">العلاقة<select disabled={working || isImported} value={predicate} onChange={(event) => setPredicate(event.target.value as SourceTreeDraftEdge["predicate"])}><option value="parent_of">أبٌ لـ</option><option value="spouse_of">زوج/زوجة</option><option value="sibling_of">أخ/أخت</option><option value="descendant_of">من ذرية</option><option value="unknown">غير محددة</option></select></label>
                <label className="composer-label">إلى<select disabled={working || isImported} value={objectID} onChange={(event) => setObjectID(event.target.value)}><option value="">اختر اسماً</option>{nodes.filter((node) => node.included && node.reviewed).map((node) => <option key={node.id} value={node.id}>{node.nameAr}</option>)}</select></label>
                <button className="secondary-button" type="button" disabled={working || isImported || !subjectID || !objectID || subjectID === objectID} onClick={addRelation}><Plus size={14} /> إضافة</button>
              </div>
              {edges.filter((edge) => edge.status !== "rejected").map((edge) => <div className="source-tree-edge-row" key={edge.id}>
                <span>{nodeName(nodes, edge.subjectNodeId)} <b>{predicateLabel(edge.predicate)}</b> {nodeName(nodes, edge.objectNodeId)}</span>
                <span className={`source-tree-edge-status${edge.status === "reviewed" ? " is-reviewed" : ""}`}>{edge.status === "reviewed" ? "راجعتها" : "تحتاج إلى مراجعة"}{edge.secondReviewRequired ? ` · مراجعون ${edge.reviewerCount}/2` : ""}</span>
                <button className="source-tree-trace-button" type="button" disabled={working || isImported || !activePage || nodes.find((node) => node.id === edge.subjectNodeId)?.pageEvidenceId !== activePage.pageEvidenceId || nodes.find((node) => node.id === edge.objectNodeId)?.pageEvidenceId !== activePage.pageEvidenceId} onClick={() => { setActiveTraceEdgeID(edge.id); setDrawMode("edge"); setDrawing([]); }}>
                  {edge.status === "reviewed" ? "إعادة تتبع الوصلة" : "تتبّع الوصلة في الصفحة"}
                </button>
                {edge.status === "reviewed" ? <button className="source-tree-trace-button" type="button" disabled={working || isImported || edge.reviewerCount >= 2 || edge.reviewedByMe} onClick={() => { setEdges((current) => current.map((item) => item.id === edge.id ? { ...item, secondReviewRequired: true } : item)); setDirty(true); setMessage(edge.reviewedByMe ? "هذه الوصلة تحتاج إلى مراجع مختلف؛ افتح المسودة بحساب مراجع آخر." : "احفظ قرارك لتسجيل مراجعة مستقلة لهذه الوصلة."); }}>{edge.reviewerCount >= 2 ? "اكتملت المراجعتان" : edge.reviewedByMe ? "بانتظار مراجع مختلف" : edge.secondReviewRequired ? "سجّل مراجعتي" : "طلب مراجعة ثانية"}</button> : null}
                <button className="source-tree-remove" type="button" disabled={working || isImported} onClick={() => { setEdges((current) => current.map((item) => item.id === edge.id ? { ...item, status: "rejected" } : item)); setDirty(true); }}>حذف</button>
              </div>)}
              {edges.some((edge) => edge.status !== "rejected" && (nodes.find((node) => node.id === edge.subjectNodeId)?.pageEvidenceId !== nodes.find((node) => node.id === edge.objectNodeId)?.pageEvidenceId)) ? <small>العلاقة بين صفحتين تبقى معلّقة؛ لا يمكن إثبات مسار مباشر على صفحة واحدة.</small> : null}
            </div>
          </div>
          <aside className="source-tree-draft-sidebar">
            <label className="composer-label">اسم مسودة الشجرة<input dir="rtl" value={title} maxLength={200} disabled={working || isImported} onChange={(event) => { setTitle(event.target.value); setDirty(true); }} /></label>
            <div className="source-tree-node-list-head"><strong>الأسماء المرشحة</strong><span>{includedNodes.length} مختار · {nodes.length} مقترح</span></div>
            {nodes.length === 0 ? <div className="source-tree-no-nodes"><Plus size={16} /><span>ابدأ بـ«تحديد اسم يدوياً»، ثم اسحب حول الاسم على الصورة واكتبه هنا.</span></div> : null}
            <div className="source-tree-node-list">
              {nodes.map((node, index) => <article className={`source-tree-node-card${node.included ? " is-included" : ""}`} key={node.id}>
                <div className="source-tree-node-card-head"><span>اسم {index + 1} · صفحة {draft.pages.find((page) => page.pageEvidenceId === node.pageEvidenceId)?.pageNumber ?? "؟"}</span><button className="source-tree-remove" type="button" disabled={working || isImported} onClick={() => updateNode(node.id, { status: node.status === "rejected" ? "proposed" : "rejected", reviewed: false, included: false })}>{node.status === "rejected" ? "إعادة للمراجعة" : "استبعاد"}</button></div>
                <input dir="rtl" aria-label={`النص المقروء للاسم ${index + 1}`} value={node.nameAr} maxLength={200} disabled={working || isImported || node.status === "rejected"} onChange={(event) => updateNode(node.id, { nameAr: event.target.value, status: "proposed", reviewed: false, included: false })} />
                <label className="source-tree-node-check"><input type="checkbox" checked={node.reviewed && node.included} disabled={working || isImported || node.status === "rejected"} onChange={(event) => updateNode(node.id, { reviewed: event.target.checked, included: event.target.checked, status: event.target.checked ? "reviewed" : "proposed" })} /><span>تحققت من الاسم وأضفته للشجرة</span></label>
              </article>)}
            </div>
            {fileWarnings.length > 0 || graphErrors.length > 0 ? <div className="source-tree-warnings" role="status">
              {[...new Set([...graphErrors, ...fileWarnings])].map((warning) => <p key={warning}><AlertTriangle size={14} /> {warning}</p>)}
            </div> : null}
            {error ? <div className="evidence-workspace-error" role="alert">{error}</div> : null}
            {message ? <div className="evidence-workspace-message" role="status">{message}</div> : null}
            {preview && currentPreview && draft.state === "draft" ? <div className="source-tree-import-preview" role="group" aria-label="تقرير الفحص المبدئي للاستيراد">
              <strong>ملخص الشجرة قبل الإنشاء</strong>
              <ul>
                <li>سيُنشئ {preview.newPeople} اسماً جديداً و{preview.relations} علاقات و{preview.statements} شواهد مُستوردة.</li>
                <li>سيُدوّن {preview.aliases} تهجئة مصدر بجانب الأسماء بما كتبته الصفحة، فلا يضيع نصّها أبداً.</li>
                {preview.linkedExisting > 0 ? <li>{preview.linkedExisting} من الأسماء تطابق أشخاصاً محفوظين؛ سيُربطون بالأصل ولن يُكرَّروا.</li> : null}
                {preview.candidates > 0 ? <li>{preview.candidates} اقتراح ربط محتمل سيُوضع في واجهة المراجعة؛ لا يُدمج شيء تلقائياً.</li> : null}
                {preview.stagedRelations > 0 ? <li>{preview.stagedRelations} علاقة لا يستطيع نموذج الشجرة تمثيلها تبقى في المسودة.</li> : null}
                {preview.linkedExisting === 0 && preview.candidates === 0 && preview.stagedRelations === 0 ? <li>لا تعارضات أو توسيمات تخص الأشخاص الموجودين.</li> : null}
              </ul>
              {preview.collisions.length > 0 ? <ul className="source-tree-import-collisions">
                {preview.collisions.map((collision) => (
                  <li key={collision.draftNodeId}>«{collision.draftNameAr}» ← <b>{collision.existingNameAr}</b> ({collision.basis === "alias" ? "مطابقة تهجئة سابقة" : "مطابقة الاسم"})</li>
                ))}
              </ul> : null}
              {preview.warnings.length > 0 ? <ul>{preview.warnings.map((warning) => <li key={warning}>{warning}</li>)}</ul> : null}
              <div className="source-tree-draft-actions">
                <button className="primary-button" type="button" disabled={working || dirty} onClick={() => void runImport(preview)}>{working ? "جارٍ إنشاء الشجرة…" : "تأكيد وإنشاء الشجرة الخاصة"}</button>
                <button className="secondary-button" type="button" disabled={working} onClick={() => setPreview(null)}>إلغاء</button>
              </div>
            </div> : null}
            <div className="source-tree-draft-actions">
              {draft.state === "draft" ? <>
                <button className="secondary-button" type="button" disabled={working || !dirty} onClick={() => void save()}><Save size={14} /> {working ? "جارٍ الحفظ…" : "حفظ المراجعة"}</button>
                {!currentPreview ? <button className="primary-button" type="button" disabled={working || graphErrors.length > 0} onClick={() => void applyImport()}><GitBranch size={15} /> {working ? "جارٍ تجهيز الملخص…" : "متابعة إلى ملخص الشجرة"}</button> : null}
                {dirty ? <small>ستُحفظ مراجعتك عند المتابعة.</small> : null}
              </> : null}
              {isImported ? <>
                {draft.treeId ? <Link className="primary-button" to="/tree/$treeId" params={{ treeId: draft.treeId }}><GitBranch size={15} /> فتح الشجرة الخاصة</Link> : null}
                <button className="secondary-button" type="button" disabled={working} onClick={() => void retractImport()}><RotateCcw size={14} /> {working ? "جارٍ التراجع…" : "التراجع عن الاستيراد"}</button>
              </> : null}
              {draft.state === "retracted" ? <p className="source-tree-retracted"><Check size={15} /> سُجل التراجع. النسخة السابقة محفوظة للتدقيق.</p> : null}
            </div>
          </aside>
        </div>
      </section>
    </div>
  );
}

function clampUnit(value: number): number { return Math.max(0, Math.min(1, value)); }
function nodeName(nodes: SourceTreeDraftNode[], id: string): string { return nodes.find((node) => node.id === id)?.nameAr ?? "اسم محذوف"; }
function predicateLabel(value: SourceTreeDraftEdge["predicate"]): string {
  if (value === "parent_of") return "أبٌ لـ";
  if (value === "spouse_of") return "زوج/زوجة";
  if (value === "sibling_of") return "أخ/أخت";
  if (value === "descendant_of") return "من ذرية";
  return "علاقة غير محددة مع";
}
function warningLabel(value: string): string {
  if (value.startsWith("Family chart machine readings")) return "تحتاج الأسماء في هذا المشجر إلى نسخ وتحقّق يدوي على الصورة.";
  if (value.startsWith("Reviewed parent relationships contain a cycle")) return "توجد حلقة في علاقات الأبوة؛ صححها قبل الاستيراد.";
  if (value.startsWith("Some reviewed relationship types remain staged")) return "ستبقى بعض أنواع العلاقات في المسودة لأنها غير مدعومة في بنية الشجرة الحالية.";
  if (value.startsWith("Every included name needs")) return "يحتاج كل اسم مختار إلى مراجعة بشرية.";
  if (value.startsWith("Local line detection reached its per-page candidate cap")) return "بلغ كاشف الخطوط حد الاقتراحات في صفحة واحدة؛ قد تكون بعض الوصلات غير ممثلة.";
  return "راجع الأسماء والوصلات على الصورة الأصلية قبل المتابعة.";
}
function hasParentCycle(nodes: SourceTreeDraftNode[], edges: SourceTreeDraftEdge[]): boolean {
  const graph = new Map<string, string[]>();
  for (const edge of edges) if (edge.status === "reviewed" && edge.predicate === "parent_of") graph.set(edge.subjectNodeId, [...(graph.get(edge.subjectNodeId) ?? []), edge.objectNodeId]);
  const state = new Map<string, number>();
  const visit = (id: string): boolean => {
    if (state.get(id) === 1) return true;
    if (state.get(id) === 2) return false;
    state.set(id, 1);
    for (const next of graph.get(id) ?? []) if (visit(next)) return true;
    state.set(id, 2);
    return false;
  };
  return nodes.some((node) => visit(node.id));
}
function errorMessage(error: unknown): string {
  if (error instanceof ApiError) return error.message;
  return error instanceof Error ? error.message : "تعذرت معالجة مسودة الشجرة.";
}
