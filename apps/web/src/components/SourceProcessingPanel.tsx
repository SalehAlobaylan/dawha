import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { Check, Eye, FileText, GitBranch, Quote, Save, ScanSearch, Upload, X } from "lucide-react";
import { ChangeEvent, FormEvent, useRef, useState } from "react";
import { createSourceFileDownloadLink, createSourceTreeDraft, fetchSourceChartAnalysis, fetchSourcePageEvidence, fetchSourcePageEvidenceIndex, fetchSourceProcessing, fetchLatestNameReadingRun, controlNameReadingRun, readSourceTreeSuggestions, reextractReviewedPage as reextractReviewedPageAPI, reprocessSourceFile, reviewSourceCandidate, reviewSourcePage, startSourceChartAnalysis, uploadSourceFile } from "../lib/api";
import { ApiError } from "../lib/api";
import { suggestBookQuoteSpans } from "../lib/book-quote-suggestions";
import { StatusBadge } from "./StatusBadge";
import type { NameReadingRun, ChartAnalysisView, SourceCandidate, SourcePageEvidence, SourcePageEvidenceIndex, SourcePageReviewInput, SourceProcessing, SourceProcessingRun, SourceTreeDraft } from "../types";
import { SourceSuggestionReview } from "./SourceSuggestionReview";
import { SourceTreeDraftDialog } from "./SourceTreeDraftDialog";
import { SourceBookAssertionDialog } from "./SourceBookAssertionDialog";
import { SourceChartAnalysisDialog } from "./SourceChartAnalysisDialog";

const JASSER_CHART_CHECKSUM = "5735335ee7b21c1c285882ba2411cb77d2615937e6e2188a8c35cbe8705307fd";

interface SourceProcessingPanelProps {
  sourceId: string;
  sourceTitle: string;
  sourceType: string;
}

// The same format matrix the API enforces. This copy is the pre-upload version
// of the answer the caller would otherwise get after a failed upload.
const SUPPORTED_UPLOAD_MEDIA_TYPES = ["text/*", "application/json", "application/xml", "application/pdf", "image/jpeg", "image/png", "image/gif"];
const SUPPORTED_UPLOAD_ACCEPT = "text/*,.txt,.md,.json,.xml,.pdf,application/json,application/xml,application/pdf,image/jpeg,image/png,image/gif,.jpg,.jpeg,.png,.gif";
const SUPPORTED_UPLOAD_HINT = "الصيغ المدعومة: نص و JSON و XML بترميز UTF-8، وملفات PDF، والصور JPG وPNG وGIF، حتى 128 ميغابايت لكل ملف. يُجرى استخراج PDF و OCR للصور محلياً، ثم يُمرر نص الصفحات إلى خدمة الذكاء الاصطناعي المهيأة لاستخراج المرشحين؛ وتبقى النتائج بحاجة للمراجعة.";

// A visual source is a page a reviewer can open: a PDF page or a raster image.
// They share the review surfaces even though only a PDF has many pages and a
// native text layer.
const isVisualSource = (mimeType: string | null | undefined) => Boolean(mimeType && (mimeType === "application/pdf" || mimeType.startsWith("image/")));
const UNDECLARED_UPLOAD_MEDIA_TYPE = "application/octet-stream";

function isSupportedUploadType(value: string): boolean {
  const normalized = value.split(";")[0].trim().toLowerCase();
  if (normalized === "" || normalized === UNDECLARED_UPLOAD_MEDIA_TYPE) {
    return true;
  }
  return SUPPORTED_UPLOAD_MEDIA_TYPES.some((entry) => (entry.endsWith("/*") ? normalized.startsWith(entry.slice(0, -1)) : normalized === entry));
}

export function SourceProcessingPanel({ sourceId, sourceTitle, sourceType }: SourceProcessingPanelProps) {
  const queryClient = useQueryClient();
  const [selectedFile, setSelectedFile] = useState<File | null>(null);
  const [reviewNote, setReviewNote] = useState("");
  const [message, setMessage] = useState("");
  const [pageReview, setPageReview] = useState<{ evidence?: SourcePageEvidence; fileUrl: string } | null>(null);
  const [pageReviewLoading, setPageReviewLoading] = useState(false);
  const [treeDraft, setTreeDraft] = useState<SourceTreeDraft | null>(null);
  const [suggestionFileID, setSuggestionFileID] = useState<string | null>(null);
  const [treeDraftLoading, setTreeDraftLoading] = useState(false);
  const [chartAnalysis, setChartAnalysis] = useState<{ fileId: string; initial: ChartAnalysisView } | null>(null);
  const processingQuery = useQuery({
    queryKey: ["source-processing", sourceId],
    queryFn: () => fetchSourceProcessing(sourceId),
    enabled: Boolean(sourceId),
    refetchInterval: (query) => {
      const current = query.state.data as SourceProcessing | undefined;
      if (!current || current.runs.length === 0) return false;
      const active = current.runs.some((run) => run.status === "queued" || run.status === "running");
      if (active) return 2000;
      const latest = current.runs[0];
      return latest.status === "failed" && Date.now() - Date.parse(latest.updatedAt) < 120000 ? 2000 : false;
    },
  });
  const uploadMutation = useMutation({
    mutationFn: () => {
      if (!selectedFile) {
        throw new Error("اختر ملفاً أولاً.");
      }
      return uploadSourceFile(sourceId, selectedFile);
    },
    onSuccess: async () => {
      setSelectedFile(null);
      setMessage("تم رفع الملف، وأُضيفت مهمة المعالجة إلى الطابور.");
      await queryClient.invalidateQueries({ queryKey: ["source-processing", sourceId] });
      await queryClient.invalidateQueries({ queryKey: ["source", sourceId] });
      await queryClient.invalidateQueries({ queryKey: ["sources"] });
      await queryClient.invalidateQueries({ queryKey: ["source-dependencies", sourceId] });
      await queryClient.invalidateQueries({ queryKey: ["source-characterization", sourceId] });
    },
    onError: (error) => setMessage(errorMessage(error)),
  });
  const suggestionReadMutation = useMutation({
    mutationFn: async (fileId: string) => {
      const result = await readSourceTreeSuggestions(fileId);
      return result;
    },
    onSuccess: async (result) => {
      setMessage("أُضيف إعداد الأسماء إلى قائمة العمل المحلية. يمكنك مراجعة المواضع المحفوظة أثناء استمرار القراءة.");
      await queryClient.invalidateQueries({ queryKey: ["name-reading-run", result.sourceFileId] });
    },
    onError: (error) => {
      setMessage(error instanceof ApiError ? error.message : "تعذّرت قراءة الملف.");
    },
  });

  const chartAnalysisMutation = useMutation({
    mutationFn: async (fileId: string) => {
      try {
        return await fetchSourceChartAnalysis(fileId);
      } catch (error) {
        if (error instanceof ApiError && error.status === 404) return startSourceChartAnalysis(fileId);
        throw error;
      }
    },
    onSuccess: async (initial, fileId) => {
      setChartAnalysis({ fileId, initial });
      setMessage("فُتحت مساحة التحليل البصري. النتائج تجريبية ولا تُضاف إلى الاستيراد.");
      await queryClient.invalidateQueries({ queryKey: ["source-processing", sourceId] });
    },
    onError: (cause) => setMessage(errorMessage(cause)),
  });
  const reviewMutation = useMutation({
    mutationFn: ({ candidateId, decision, note }: { candidateId: string; decision: "accepted" | "rejected"; note: string }) => reviewSourceCandidate(candidateId, decision, note || undefined),
    onSuccess: async () => {
      setReviewNote("");
      setMessage("تم حفظ قرار المراجعة في سجل المرشحات.");
      await queryClient.invalidateQueries({ queryKey: ["source-processing", sourceId] });
      await queryClient.invalidateQueries({ queryKey: ["source", sourceId] });
      await queryClient.invalidateQueries({ queryKey: ["sources"] });
      await queryClient.invalidateQueries({ queryKey: ["source-dependencies", sourceId] });
      await queryClient.invalidateQueries({ queryKey: ["source-characterization", sourceId] });
    },
    onError: (error) => setMessage(errorMessage(error)),
  });
  const reprocessMutation = useMutation({
    mutationFn: (fileId: string) => reprocessSourceFile(fileId, 400),
    onSuccess: async (run) => {
      setMessage("بدأ تشغيل OCR جديداً بدقة 400 نقطة/بوصة. ستبقى القراءات والقرارات السابقة محفوظة للمقارنة.");
      await queryClient.invalidateQueries({ queryKey: ["source-processing", sourceId] });
      await queryClient.invalidateQueries({ queryKey: ["source-page-evidence-index", run.sourceFileId] });
      await queryClient.invalidateQueries({ queryKey: ["source", sourceId] });
    },
    onError: (error) => setMessage(errorMessage(error)),
  });
  const pageReextractMutation = useMutation({
    mutationFn: (evidence: SourcePageEvidence) => {
      const review = evidence.currentReview;
      if (!review || review.sourcePageEvidenceId !== evidence.pageEvidenceId || (review.status !== "transcribed" && review.status !== "partial")) {
        throw new Error("احفظ مراجعة بشرية حديثة لهذه الصفحة قبل إعادة استخراج المرشحين.");
      }
      return reextractReviewedPageAPI(evidence.sourceFileId, evidence.pageNumber, {
        expectedPageEvidenceId: evidence.pageEvidenceId,
        pageReviewRevisionId: review.id,
        expectedRevision: review.revisionNumber,
      });
    },
    onSuccess: async (run) => {
      setMessage("بدأ استخراج مرشحين من النص البشري المحفوظ. سيظل نص النسخة الأصلية والقرارات السابقة محفوظة.");
      await queryClient.invalidateQueries({ queryKey: ["source-processing", sourceId] });
      await queryClient.invalidateQueries({ queryKey: ["source-page-evidence-index", run.sourceFileId] });
    },
    onError: (error) => setMessage(errorMessage(error)),
  });
  const processing = processingQuery.data;
  const error = processingQuery.error;
  const viewSourcePage = async (fileId: string, pageNumber: number) => {
    setPageReviewLoading(true);
    setMessage("");
    try {
      const [evidence, download] = await Promise.all([
        fetchSourcePageEvidence(fileId, pageNumber),
        createSourceFileDownloadLink(fileId),
      ]);
      setPageReview({ evidence, fileUrl: `${download.url}#page=${pageNumber}&toolbar=0&navpanes=0` });
    } catch (pageError) {
      setMessage(errorMessage(pageError));
    } finally {
      setPageReviewLoading(false);
    }
  };
  const viewOriginalPDF = async (fileId: string) => {
    setPageReviewLoading(true);
    setMessage("");
    try {
      const download = await createSourceFileDownloadLink(fileId);
      setPageReview({ fileUrl: download.url });
    } catch (pageError) {
      setMessage(errorMessage(pageError));
    } finally {
      setPageReviewLoading(false);
    }
  };
  const startTreeDraft = async (fileId: string) => {
    setTreeDraftLoading(true);
    setMessage("");
    try {
      const draft = await createSourceTreeDraft(fileId, sourceTitle);
      setTreeDraft(draft);
      setMessage(`أُنشئت مسودة مراجعة من ${draft.nodes.length} اقتراحات أسماء على ${draft.pages.length} صفحات. لم تُضف أي علاقة تلقائياً.`);
    } catch (draftError) {
      setMessage(errorMessage(draftError));
    } finally {
      setTreeDraftLoading(false);
    }
  };
  const savePageReview = async (input: SourcePageReviewInput) => {
    const current = pageReview?.evidence;
    if (!current) throw new Error("افتح صفحة مستخرجة أولاً لحفظ مراجعتها.");
    const revision = await reviewSourcePage(current.sourceFileId, current.pageNumber, input);
    setPageReview((value) => value?.evidence ? {
      ...value,
      evidence: { ...value.evidence, currentReview: revision, reviewCount: revision.revisionNumber },
    } : value);
    await queryClient.invalidateQueries({ queryKey: ["source-page-evidence-index", current.sourceFileId] });
    return revision;
  };
  const chooseFile = (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0] ?? null;
    if (file && !isSupportedUploadType(file.type)) {
      setSelectedFile(null);
      event.target.value = "";
      setMessage(`صيغة الملف ${file.type} غير مدعومة. ${SUPPORTED_UPLOAD_HINT}`);
      return;
    }
    setSelectedFile(file);
  };

  return (
    <section className="source-processing-panel" aria-label="معالجة المصدر">
      <div className="source-processing-head">
        <div>
          <div className="eyebrow">معالجة المصدر</div>
          <h3>{sourceTitle}</h3>
          <p>{sourceType === "family_document" ? "تابع استيراد المشجر لمراجعة الأسماء والوصلات وإنشاء شجرة خاصة." : "ارفع نصاً أو PDF أو صورة، ثم راجع النتائج على الصفحة الأصلية."}</p>
        </div>
        <FileText size={18} />
      </div>
      {sourceType === "book" || sourceType === "manuscript" ? <ol className="source-processing-steps" dir="rtl">
        <li><strong>ارفع الكتاب أو المخطوط</strong><span>تُحفظ النسخة وتُفهرس قراءة كل صفحة.</span></li>
        <li><strong>راجع النسخ مع الصفحة</strong><span>صحح النص يدوياً، واحفظ كل مراجعة كنسخة مستقلة.</span></li>
        <li><strong>سجّل عبارة موثقة</strong><span>اختر اقتباساً، ثم أدخل الأسماء والعلاقة كما يذكرها النص.</span></li>
      </ol> : null}
      <details className="source-upload-disclosure" open={sourceType !== "family_document" || !processing?.files.length || undefined}><summary>إضافة ملف إلى المصدر</summary><div className="source-processing-upload">
        <label className="composer-label source-processing-file">
          ملف المصدر
          <input type="file" accept={SUPPORTED_UPLOAD_ACCEPT} onChange={chooseFile} />
          <small>{SUPPORTED_UPLOAD_HINT}</small>
        </label>
        <button className="secondary-button" type="button" disabled={!selectedFile || uploadMutation.isPending} onClick={() => uploadMutation.mutate()}>
          <Upload size={15} /> {uploadMutation.isPending ? "جارٍ الرفع…" : "رفع ومعالجة"}
        </button>
      </div>
      </details>
      {message ? <div className="evidence-workspace-message" role="status">{message}</div> : null}
      {error ? <div className="evidence-workspace-error" role="alert">{errorMessage(error)}</div> : null}
      {processingQuery.isPending ? <p className="evidence-empty">جارٍ تحميل حالة المعالجة…</p> : null}
      {pageReviewLoading ? <div className="evidence-workspace-message" role="status">جارٍ تحميل الملف الأصلي وقراءات الصفحات…</div> : null}
      {treeDraftLoading ? <div className="evidence-workspace-message" role="status">جارٍ تجهيز مسودة الشجرة ومربعات الأسماء…</div> : null}
      {processing ? <ProcessingSummary processing={processing} sourceType={sourceType} reviewPending={reviewMutation.isPending} reviewNote={reviewNote} setReviewNote={setReviewNote} onReview={(candidateId, decision) => reviewMutation.mutate({ candidateId, decision, note: reviewNote })} onViewPage={viewSourcePage} onViewOriginalPDF={viewOriginalPDF} onStartTreeDraft={(fileId) => void startTreeDraft(fileId)} treeDraftLoading={treeDraftLoading} reprocessPending={reprocessMutation.isPending} onReprocess={(fileId) => reprocessMutation.mutate(fileId)} chartAnalysisPending={chartAnalysisMutation.isPending} onStartChartAnalysis={(fileId) => chartAnalysisMutation.mutate(fileId)} onReviewSuggestions={(fileId) => setSuggestionFileID(fileId)} suggestionReadPending={suggestionReadMutation.isPending} onReadSuggestions={(fileId) => suggestionReadMutation.mutate(fileId)} /> : null}
      {pageReview ? <SourcePageReviewDialog review={pageReview} sourceType={sourceType} onClose={() => setPageReview(null)} onSaveReview={savePageReview} onBookAssertionAccepted={async () => {
        await queryClient.invalidateQueries({ queryKey: ["source", sourceId] });
        await queryClient.invalidateQueries({ queryKey: ["sources"] });
      }} reextractPending={pageReextractMutation.isPending} onReextractReviewedPage={(evidence) => pageReextractMutation.mutateAsync(evidence)} /> : null}
      {treeDraft ? <SourceTreeDraftDialog initialDraft={treeDraft} onClose={() => setTreeDraft(null)} onChanged={setTreeDraft} /> : null}
      {suggestionFileID ? <SourceSuggestionReview key={suggestionFileID} fileId={suggestionFileID} sourceTitle={sourceTitle} onClose={() => setSuggestionFileID(null)} /> : null}
      {chartAnalysis ? <SourceChartAnalysisDialog fileId={chartAnalysis.fileId} titleAr={sourceTitle} initial={chartAnalysis.initial} onClose={() => setChartAnalysis(null)} onManualDraft={(fileId) => { setChartAnalysis(null); void startTreeDraft(fileId); }} /> : null}
    </section>
  );
}

function ProcessingSummary({ processing, sourceType, reviewPending, reviewNote, setReviewNote, onReview, onViewPage, onViewOriginalPDF, onStartTreeDraft, treeDraftLoading, reprocessPending, onReprocess, chartAnalysisPending, onStartChartAnalysis, onReviewSuggestions, suggestionReadPending, onReadSuggestions }: { processing: SourceProcessing; sourceType: string; reviewPending: boolean; reviewNote: string; setReviewNote: (value: string) => void; onReview: (candidateId: string, decision: "accepted" | "rejected") => void; onViewPage: (fileId: string, pageNumber: number) => void; onViewOriginalPDF: (fileId: string) => void; onStartTreeDraft: (fileId: string) => void; treeDraftLoading: boolean; reprocessPending: boolean; onReprocess: (fileId: string) => void; chartAnalysisPending: boolean; onStartChartAnalysis: (fileId: string) => void; onReviewSuggestions: (fileId: string) => void; suggestionReadPending: boolean; onReadSuggestions: (fileId: string) => void }) {
  const latestRun = processing.runs[0];
  const recentRuns = processing.runs.slice(0, 8);
  const runSummary = <>
      <div className="source-processing-run">
        <div><strong>أحدث تشغيل</strong><span>{latestRun ? `${runKindLabel(latestRun)} · ${runStatusLabel(latestRun.status)}` : "لم تبدأ"}</span></div>
        {latestRun ? <small>{latestRun.pageCount} صفحة · {latestRun.passageCount} مقطع · {latestRun.candidateCount} مرشح</small> : null}
        {latestRun?.error ? <p className="source-processing-error">{latestRun.error}</p> : null}
      </div>
      {recentRuns.length > 1 ? <details className="source-processing-history"><summary>سجل التشغيلات · {processing.runs.length}</summary>{recentRuns.map((run) => <div className="source-processing-history-row" key={run.id}><span>{runKindLabel(run)}</span><span>{runStatusLabel(run.status)} · {run.pageCount} صفحة · {run.candidateCount} مرشح</span><small>{new Date(run.createdAt).toLocaleString("ar-SA")}</small></div>)}</details> : null}
      {processing.candidates.length > 0 ? <label className="composer-label source-review-note">ملاحظة القرار<textarea value={reviewNote} onChange={(event) => setReviewNote(event.target.value)} rows={2} placeholder="اختياري: سبب القبول أو الرفض" /></label> : null}
  </>;
  return (
    <div className="source-processing-summary">
      {processing.files.length > 0 ? <div className="source-processing-files">{processing.files.map((file) => {
        const active = processing.runs.some((run) => run.sourceFileId === file.id && (run.status === "queued" || run.status === "running"));
        return <div className="source-processing-file" key={file.id}>
        <div className="source-processing-file-detail">
          <span>{file.originalFilenameAr}</span>
          {file.processingError ? <p className="source-processing-error">{file.processingError}</p> : null}
          {sourceType === "family_document" && isVisualSource(file.mimeType) ? <div className="tree-library-actions"><Link className="primary-button" to="/tree/charts/$fileId/names" params={{ fileId: file.id }}><ScanSearch size={15} /> مراجعة اقتراحات الأسماء</Link><Link className="secondary-button" to="/import" search={{ sourceId: processing.sourceId, fileId: file.id }}><GitBranch size={15} /> الوصلات والشجرة</Link></div> : null}
          {isVisualSource(file.mimeType) ? <>
            <details className="source-file-tools"><summary>أدوات الملف والقراءة المتقدمة</summary>
            <button className="source-pdf-page-index-toggle" type="button" disabled={active || reprocessPending || (file.processingStatus !== "succeeded" && file.processingStatus !== "failed")} onClick={() => onReprocess(file.id)}>{reprocessPending ? "جارٍ بدء إعادة القراءة…" : file.mimeType === "application/pdf" ? "إعادة OCR بدقة أعلى (400 نقطة/بوصة)" : "إعادة قراءة الصورة"}</button>
            {file.mimeType === "application/pdf" ? <>
              <button className="source-pdf-page-index-toggle" type="button" onClick={() => void onViewOriginalPDF(file.id)}><Eye size={14} /> فتح ملف PDF الأصلي</button>
              <SourcePDFPagePicker fileId={file.id} onViewPage={onViewPage} />
              {sourceType === "family_document" && file.checksumSha256.toLowerCase() === JASSER_CHART_CHECKSUM ? <><button className="source-chart-analysis-start" type="button" disabled={chartAnalysisPending} onClick={() => onStartChartAnalysis(file.id)}><ScanSearch size={15} /> {chartAnalysisPending ? "جارٍ فتح تحليل الرؤية…" : "فتح التحليل البصري التجريبي"}</button><small className="source-chart-analysis-hint">متاح لهذا الملف الخاص فقط. اقتراحات الرؤية معزولة عن الاستيراد حتى تأهيلها معيارياً.</small></> : null}
            </> : <button className="source-pdf-page-index-toggle" type="button" onClick={() => onViewPage(file.id, 1)}><Eye size={14} /> عرض الصفحة الأصلية</button>}
            {sourceType === "family_document" ? <><button className="source-tree-draft-start" type="button" disabled={treeDraftLoading || file.processingStatus !== "succeeded"} onClick={() => onStartTreeDraft(file.id)}><GitBranch size={15} /> إعداد مسودة شجرة من هذا الملف</button><button className="source-tree-draft-start" type="button" disabled={suggestionReadPending} onClick={() => onReadSuggestions(file.id)}><ScanSearch size={15} /> قراءة الملف واستخراج الأسماء</button><button className="source-tree-draft-start" type="button" onClick={() => onReviewSuggestions(file.id)}><Quote size={15} /> مراجعة اقتراحات القراءة</button><small className="source-tree-draft-start-hint">ستراجع الأسماء على صورة كل صفحة؛ تبقى الوصلات لمرحلة مسودة الشجرة.</small><NameReadingRunStatus fileId={file.id} /></> : null}
            </details>
          </> : null}
        </div>
        <StatusBadge tone={file.processingStatus === "failed" ? "disputed" : "source"}>{file.processingStatus === "succeeded" ? "اكتملت" : file.processingStatus === "failed" ? "فشلت" : "قيد المعالجة"}</StatusBadge>
      </div>;
      })}</div> : null}
      {sourceType === "family_document" ? <details className="source-file-tools"><summary>حالة المعالجة وسجل التشغيلات</summary>{runSummary}</details> : runSummary}
      {processing.candidates.length > 0 ? <div className="source-candidate-list">{processing.candidates.map((candidate) => <CandidateRow candidate={candidate} key={candidate.id} reviewPending={reviewPending} isVisual={isVisualSource(processing.files.find((file) => file.id === candidate.sourceFileId)?.mimeType)} onReview={onReview} onViewPage={onViewPage} />)}</div> : sourceType !== "family_document" && latestRun?.status === "succeeded" ? <p className="evidence-empty">اكتملت المعالجة دون استخراج مرشحين.</p> : null}
    </div>
  );
}

function SourcePDFPagePicker({ fileId, onViewPage }: { fileId: string; onViewPage: (fileId: string, pageNumber: number) => void }) {
  const [expanded, setExpanded] = useState(false);
  const pagesQuery = useQuery<SourcePageEvidenceIndex>({
    queryKey: ["source-page-evidence-index", fileId],
    queryFn: () => fetchSourcePageEvidenceIndex(fileId),
    enabled: expanded,
    staleTime: 60_000,
  });
  const pages = pagesQuery.data?.pages ?? [];
  return (
    <div className="source-pdf-page-index">
      <button className="source-pdf-page-index-toggle" type="button" aria-expanded={expanded} onClick={() => setExpanded((value) => !value)}>
        <Eye size={14} /> {expanded ? "إخفاء صفحات PDF" : "عرض جميع صفحات PDF"}
      </button>
      {expanded ? <div className="source-pdf-page-index-list">
        {pagesQuery.isPending ? <small>جارٍ تحميل فهرس الصفحات…</small> : null}
        {pagesQuery.error ? <small className="source-processing-error">{errorMessage(pagesQuery.error)}</small> : null}
        {!pagesQuery.isPending && !pagesQuery.error && pages.length === 0 ? <small>لم تُحفظ بيانات صفحات لهذا الملف بعد.</small> : null}
        {pages.map((page) => <button className="source-pdf-page-index-item" key={page.pageNumber} type="button" onClick={() => onViewPage(fileId, page.pageNumber)}>
          صفحة {page.pageNumber} · {page.selectedMethod === "not_processed" ? "لم تُعالج" : page.status === "unreadable" ? "غير مقروءة" : readingMethodLabel(page.selectedMethod)}{page.reviewCount ? ` · مراجعات بشرية ${page.reviewCount}` : ""}
        </button>)}
      </div> : null}
    </div>
  );
}

function CandidateRow({ candidate, reviewPending, isVisual, onReview, onViewPage }: { candidate: SourceCandidate; reviewPending: boolean; isVisual: boolean; onReview: (candidateId: string, decision: "accepted" | "rejected") => void; onViewPage: (fileId: string, pageNumber: number) => void }) {
  const canReview = candidate.status === "unreviewed" || candidate.status === "needs_review";
  return (
    <article className="source-candidate-row">
      <div className="source-candidate-row-head"><StatusBadge tone={candidate.candidateType === "claim" ? "claim" : "source"}>{candidate.candidateType === "claim" ? "علاقة" : "كيان"}</StatusBadge><span>{Math.round(candidate.confidence * 100)}%</span><small>{candidate.pageNumber ? `صفحة ${candidate.pageNumber}` : candidate.locatorAr || "دون موضع"}</small></div>
      <p className="source-candidate-raw">{candidate.rawTextAr}</p>
      <p className="source-candidate-passage">{candidate.passageTextAr}</p>
      <div className="source-candidate-meta"><span>{candidate.candidateType === "claim" ? relationLabel(candidate) : candidate.proposedEntityNameAr ? `مطابقة مقترحة: ${candidate.proposedEntityNameAr}` : "لا توجد مطابقة كيان مقترحة"}</span>{candidate.proposedMatchScore !== undefined ? <span>درجة المطابقة: {Math.round(candidate.proposedMatchScore * 100)}%</span> : null}<span>{candidate.rationaleAr}</span></div>
      <div className="source-candidate-actions">
        {isVisual && candidate.pageNumber ? <button className="secondary-button" type="button" onClick={() => onViewPage(candidate.sourceFileId, candidate.pageNumber!)}><Eye size={14} /> الصفحة الأصلية ومقارنة القراءات</button> : null}
        {canReview ? <><button className="primary-button" type="button" disabled={reviewPending} onClick={() => onReview(candidate.id, "accepted")}><Check size={14} /> قبول</button><button className="secondary-button" type="button" disabled={reviewPending} onClick={() => onReview(candidate.id, "rejected")}><X size={14} /> رفض</button></> : <span className="source-candidate-reviewed">{candidate.status === "accepted" ? "مقبول" : candidate.status === "rejected" ? "مرفوض" : candidate.status === "superseded" ? "استُبدل بتشغيل أحدث" : "راجعته سابقاً"}{candidate.reviews.length ? ` · ${candidate.reviews.length} قرار` : ""}</span>}
      </div>
    </article>
  );
}

function SourcePageReviewDialog({ review, sourceType, onClose, onSaveReview, onBookAssertionAccepted, reextractPending, onReextractReviewedPage }: { review: { evidence?: SourcePageEvidence; fileUrl: string }; sourceType: string; onClose: () => void; onSaveReview: (input: SourcePageReviewInput) => Promise<unknown>; onBookAssertionAccepted: () => Promise<void>; reextractPending: boolean; onReextractReviewedPage: (evidence: SourcePageEvidence) => Promise<SourceProcessingRun> }) {
  const evidence = review.evidence;
  const transcriptionRef = useRef<HTMLTextAreaElement>(null);
  const [transcription, setTranscription] = useState(evidence?.currentReview?.textAr ?? evidence?.text ?? "");
  const [status, setStatus] = useState<SourcePageReviewInput["status"]>(evidence?.currentReview?.status ?? (evidence?.status === "unreadable" && !evidence.text ? "unreadable" : "transcribed"));
  const [note, setNote] = useState(evidence?.currentReview?.noteAr ?? "");
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState("");
  const [saveMessage, setSaveMessage] = useState("");
  const [bookQuote, setBookQuote] = useState<{ start: number; end: number; text: string } | null>(null);
  const [bookDialogOpen, setBookDialogOpen] = useState(false);
  const bookReviewMatches = Boolean(evidence?.currentReview && evidence.currentReview.sourcePageEvidenceId === evidence.pageEvidenceId && evidence.currentReview.textAr === transcription && evidence.currentReview.status === status && status !== "unreadable");
  const bookQuoteSuggestions = bookReviewMatches ? suggestBookQuoteSpans(transcription) : [];
  const reextract = async () => {
    if (!evidence) return;
    setSaveError("");
    setSaveMessage("");
    try {
      const run = await onReextractReviewedPage(evidence);
      setSaveMessage(`بدأ التشغيل ${run.id.slice(0, 8)} من النسخة البشرية رقم ${evidence.currentReview?.revisionNumber}.`);
    } catch (error) {
      setSaveError(errorMessage(error));
    }
  };
  const save = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!evidence) return;
    setSaving(true);
    setSaveError("");
    setSaveMessage("");
    try {
      await onSaveReview({
        expected_page_evidence_id: evidence.pageEvidenceId,
        expected_revision: evidence.reviewCount,
        status,
        text_ar: status === "unreadable" ? "" : transcription,
        note_ar: note,
      });
      setTranscription(status === "unreadable" ? "" : transcription);
      setSaveMessage("حُفظت المراجعة البشرية كنسخة مستقلة. شغّل استخراج المرشحين منها أدناه عند الحاجة.");
    } catch (error) {
      setSaveError(error instanceof ApiError && error.status === 409 ? "تغيرت قراءة الصفحة أو حُفظت مراجعة أحدث. أغلق الصفحة وأعد فتحها لتحميل أحدث نسخة." : errorMessage(error));
    } finally {
      setSaving(false);
    }
  };
  const beginBookAssertion = (suggestion?: { start: number; end: number; text: string }) => {
    if (suggestion) {
      setSaveError("");
      setBookQuote(suggestion);
      setBookDialogOpen(true);
      return;
    }
    const field = transcriptionRef.current;
    if (!field || !evidence?.currentReview || evidence.currentReview.sourcePageEvidenceId !== evidence.pageEvidenceId || evidence.currentReview.textAr !== transcription || evidence.currentReview.status !== status || status === "unreadable") return;
    const start = field.selectionStart;
    const end = field.selectionEnd;
    const text = field.value.slice(start, end);
    if (!text.trim()) {
      setSaveError("حدد العبارة المطلوبة من النص المنسوخ أولاً.");
      return;
    }
    setSaveError("");
    setBookQuote({ start: Array.from(field.value.slice(0, start)).length, end: Array.from(field.value.slice(0, end)).length, text });
    setBookDialogOpen(true);
  };
  return (
    <div className="source-page-review-backdrop" role="presentation" onClick={onClose}>
      <section className="source-page-review" role="dialog" aria-modal="true" aria-labelledby="source-page-review-title" onClick={(event) => event.stopPropagation()} onKeyDown={(event) => {
        if (event.key === "Escape") {
          event.preventDefault();
          onClose();
          return;
        }
        if (event.key === "Tab") {
          const focusable = Array.from(event.currentTarget.querySelectorAll<HTMLElement>("button:not(:disabled), select:not(:disabled), textarea:not(:disabled), iframe"));
          const first = focusable[0];
          const last = focusable[focusable.length - 1];
          if (event.shiftKey && event.currentTarget.ownerDocument.activeElement === first) {
            event.preventDefault();
            last?.focus();
          } else if (!event.shiftKey && event.currentTarget.ownerDocument.activeElement === last) {
            event.preventDefault();
            first?.focus();
          }
        }
      }}>
        <header className="source-page-review-head">
          <div><div className="eyebrow">دليل المصدر</div><h3 id="source-page-review-title">{evidence ? `الصفحة ${evidence.pageNumber}` : "الملف الأصلي"}</h3><p>{evidence ? evidence.selectedMethod === "not_processed" ? "توقف الاستخراج قبل معالجة هذه الصفحة. افتح موضعها في الملف الأصلي واكتب ما يمكن قراءته دون تخمين." : "قارن النص المقروء بالنسخة الأصلية. درجات OCR خام وليست احتمالاً معايراً للصحة." : "تصفح النسخة الأصلية حتى عند تعذر حفظ قراءة آلية لهذه الصفحات."}</p></div>
          <button className="secondary-button" type="button" autoFocus onClick={onClose} aria-label="إغلاق"><X size={16} /></button>
        </header>
        <div className="source-page-review-grid">
          <iframe className="source-page-review-pdf" title={evidence ? `الصفحة الأصلية رقم ${evidence.pageNumber}` : "الملف الأصلي"} src={review.fileUrl} referrerPolicy="no-referrer" />
          {evidence ? <div className="source-page-review-readings">
            <div className="source-page-review-meta">{evidence.selectedMethod === "not_processed" ? "حالة الصفحة: لم تُعالج" : `القراءة المختارة: ${readingMethodLabel(evidence.selectedMethod)} · ${evidence.regions.length} منطقة نصية · درجات OCR خام وليست احتمالاً للصحة`}</div>
            {evidence.readings.map((reading) => <article className="source-page-reading" key={`${reading.method}:${reading.model}`}>
              <div><strong>{readingMethodLabel(reading.method)}</strong><small>{reading.model}{reading.confidence !== undefined ? ` · متوسط الدرجة الخام ${Math.round(reading.confidence * 100)}%` : ""}</small></div>
              <pre dir="rtl">{reading.text || "لم تُقرأ كلمات في هذه الصفحة."}</pre>
            </article>)}
            {evidence.status === "unreadable" ? <p className="source-processing-error">{evidence.selectedMethod === "not_processed" ? "لم تُعالج هذه الصفحة آلياً؛ هذا لا يعني أنها خالية من معلومات النسب." : "لم تُستخرج قراءة نصية لهذه الصفحة. قد تحتاج إلى كتابة يدوية أو نسخة أوضح."}</p> : null}
            <form className="source-page-review-form" onSubmit={(event) => void save(event)}>
              <div>
                <strong>نسخ المراجع</strong>
                <small>{evidence.currentReview ? evidence.currentReview.sourcePageEvidenceId === evidence.pageEvidenceId
                  ? `المراجعة البشرية رقم ${evidence.currentReview.revisionNumber} محفوظة لهذه القراءة؛ التعديل ينشئ نسخة جديدة.`
                  : "توجد مراجعة بشرية محفوظة لقراءة آلية سابقة لهذه الصفحة. النص أدناه نقطة بداية؛ تحقق منه مقابل النسخة الأصلية قبل حفظ مراجعة جديدة."
                  : "النص المقترح أدناه غير معتمد حتى تحفظ مراجعتك."}</small>
              </div>
              <label className="composer-label">حالة الصفحة
                <select value={status} onChange={(event) => setStatus(event.target.value as SourcePageReviewInput["status"])}>
                  <option value="transcribed">تم نسخ النص المتاح</option>
                  <option value="partial">نسخ جزئي؛ بقي نص غير مقروء</option>
                  <option value="unreadable">تعذر قراءة النص</option>
                </select>
              </label>
              <label className="composer-label">النص العربي بعد المراجعة
                <textarea ref={transcriptionRef} dir="rtl" rows={8} maxLength={200000} value={transcription} disabled={status === "unreadable"} onChange={(event) => setTranscription(event.target.value)} placeholder="اكتب النص كما يظهر في الصفحة، واترك الأجزاء غير المقروءة دون تخمين." />
              </label>
              {(sourceType === "book" || sourceType === "manuscript") ? <div className="source-book-assertion-entry">
                <div><strong>استخراج عبارة من الكتاب</strong><small>{evidence.currentReview?.sourcePageEvidenceId === evidence.pageEvidenceId && evidence.currentReview.textAr === transcription && evidence.currentReview.status === status ? "حدد اقتباساً من النص المحفوظ لربطه بادعاء قابل للمراجعة." : "احفظ مراجعة النص أولاً؛ الاقتباس يجب أن يطابق نسخة محفوظة بعينها."}</small></div>
                <button className="secondary-button" type="button" disabled={!evidence.currentReview || evidence.currentReview.sourcePageEvidenceId !== evidence.pageEvidenceId || evidence.currentReview.textAr !== transcription || evidence.currentReview.status !== status || status === "unreadable"} onClick={() => beginBookAssertion()}><Quote size={14} /> ترميز الاقتباس المحدد</button>
              </div> : null}
              {(sourceType === "book" || sourceType === "manuscript") && bookReviewMatches ? <div className="source-book-suggestions">
                <div><strong>عبارات محتملة للمراجعة</strong><small>اقتراحات محلية من ألفاظ نسب ظاهرة في النص؛ لا تستنتج أطراف العلاقة أو نوعها.</small></div>
                {bookQuoteSuggestions.length === 0 ? <small>لم تظهر عبارة مطابقة للقواعد البسيطة؛ يمكنك تحديد أي اقتباس يدوياً.</small> : bookQuoteSuggestions.map((suggestion, index) => <button type="button" className="source-book-suggestion" key={`${suggestion.start}:${suggestion.end}`} onClick={() => beginBookAssertion(suggestion)}><span>عبارة {index + 1}</span><q>{suggestion.text}</q><small>فتح للمراجعة اليدوية</small></button>)}
              </div> : null}
              <label className="composer-label">ملاحظة المراجع<textarea dir="rtl" rows={2} maxLength={2000} value={note} onChange={(event) => setNote(event.target.value)} placeholder="اختياري: موضع التباس أو سبب اعتبار القراءة جزئية" /></label>
              {saveError ? <p className="source-processing-error" role="alert">{saveError}</p> : null}
              {saveMessage ? <p className="source-page-review-saved" role="status">{saveMessage}</p> : null}
              <button className="primary-button" type="submit" disabled={saving || (status !== "unreadable" && !transcription.trim())}>
                <Save size={14} /> {saving ? "جارٍ حفظ المراجعة…" : "حفظ مراجعة بشرية جديدة"}
              </button>
              {evidence.currentReview && evidence.currentReview.sourcePageEvidenceId === evidence.pageEvidenceId && evidence.currentReview.textAr === transcription && evidence.currentReview.status === status && (status === "transcribed" || status === "partial") ? <div className="source-page-reextract">
                <p>يستخدم التشغيل هذه النسخة البشرية لتكوين مرشحين جدد ويحفظ صلتها بالمراجعة ورقم الصفحة، مع إبقاء قراءة OCR الأصلية.</p>
                <button className="secondary-button" type="button" disabled={reextractPending || saving} onClick={() => void reextract()}>{reextractPending ? "جارٍ بدء الاستخراج…" : "إعادة استخراج المرشحين من النسخة البشرية"}</button>
              </div> : null}
            </form>
          </div> : <div className="source-page-review-readings"><p>لا تتوفر قراءة نصية محفوظة. افحص الصورة الأصلية لتحديد ما إذا كانت الصفحة تحتاج إلى نسخ يدوي أو إلى نسخة أوضح.</p></div>}
        </div>
      </section>
      {bookDialogOpen && evidence && bookQuote ? <SourceBookAssertionDialog fileId={evidence.sourceFileId} pageEvidenceId={evidence.pageEvidenceId} pageNumber={evidence.pageNumber} pageReviewRevisionId={evidence.currentReview!.id} expectedReviewRevision={evidence.currentReview!.revisionNumber} quote={bookQuote} onClose={() => setBookDialogOpen(false)} onReviewed={async () => { await onBookAssertionAccepted(); setBookDialogOpen(false); }} /> : null}
    </div>
  );
}

function readingMethodLabel(value: string): string {
  if (value === "pdf_text") return "النص المضمّن في PDF";
  if (value === "tesseract_ocr") return "OCR عربي محلي";
  if (value === "tesseract_unavailable") return "تعذر تنفيذ OCR عربي محلي";
  if (value === "not_processed") return "لم تُعالج";
  if (value === "unreadable") return "غير مقروءة";
  return value;
}

function relationLabel(candidate: SourceCandidate): string {
  const subject = candidate.subjectTextAr || "موضوع غير محدد";
  const predicate = candidate.predicateAr || "علاقة";
  const object = candidate.objectTextAr ? ` ${candidate.objectTextAr}` : "";
  return `${subject} — ${predicate}${object}`;
}

function runStatusLabel(value: string): string {
  if (value === "succeeded") return "اكتملت";
  if (value === "failed") return "فشلت";
  if (value === "running") return "تعمل الآن";
  return "في الطابور";
}

function runKindLabel(run: SourceProcessingRun): string {
  if (run.runKind === "reviewed_page") return `استخراج من مراجعة بشرية · صفحة ${Number(run.pipelineConfig.pageNumber) || "؟"}`;
  const dpi = Number(run.pipelineConfig.renderDpi);
  return dpi > 0 ? `OCR PDF · ${dpi} نقطة/بوصة` : "معالجة PDF";
}

function errorMessage(error: unknown): string {
  if (error instanceof ApiError) return error.message;
  return error instanceof Error ? error.message : "تعذر إكمال عملية المعالجة.";
}

const NAME_RUN_LABELS: Record<NameReadingRun["status"], string> = { queued: "بانتظار العامل المحلي", running: "جارٍ الإعداد محلياً", partial: "إعداد جزئي؛ المواضع المحفوظة قابلة للمراجعة", succeeded: "اكتملت مراحل الإعداد؛ القراءات ما زالت تحتاج مراجعتك", failed: "تعذّر الإعداد", cancelled: "أُوقف الإعداد مع الاحتفاظ بالعمل المكتمل" };
const NAME_STAGE_LABELS: Record<NameReadingRun["stage"], string> = { queued: "في قائمة العمل", preparing: "تجهيز المصدر", detecting: "تحديد مواضع الأسماء", reading: "قراءة القصاصات محلياً", complete: "انتهت المحاولة" };

export function NameReadingRunStatus({ fileId }: { fileId: string }) {
 const client = useQueryClient();
 const query = useQuery({ queryKey: ["name-reading-run", fileId], queryFn: ({ signal }) => fetchLatestNameReadingRun(fileId, signal), refetchInterval: (query) => {
  const run = query.state.data as NameReadingRun | null | undefined;
  return run && (run.status === "queued" || run.status === "running") ? 1500 : false;
 } });
 const action = useMutation({ mutationFn: ({ id, action }: { id: string; action: "resume" | "cancel" }) => controlNameReadingRun(id, action), onSuccess: (run) => client.setQueryData(["name-reading-run", fileId], run) });
 const run = query.data;
 if (!run) return query.isError ? <p role="status">تعذّر تحميل حالة الإعداد المحلي. أعد المحاولة من المصدر.</p> : null;
 const active = run.status === "queued" || run.status === "running";
 return <div className="evidence-workspace-message" role="status">
  <strong>{NAME_RUN_LABELS[run.status]}</strong>
  <p>{NAME_STAGE_LABELS[run.stage]} · {run.detected} موضعاً محفوظاً · {run.named} قراءة نصية · {run.unreadable} بلا نص مقروء · {run.unavailable} قراءة غير متاحة</p>
  <small>{run.completedUnits} مرحلة محفوظة من {run.totalUnits} · {run.cacheHits} استخداماً للعمل المحفوظ · {run.manualReview} موضعاً يحتاج مراجعة المصدر يدوياً · {run.sourcePixelWarnings} موضعاً بحروف صغيرة في الأصل</small>
  {run.errorCode ? <p>توقفت مرحلة إعداد محلية. العمل المكتمل محفوظ ويمكن إعادة محاولة المراحل المتبقية.</p> : null}
  {active ? <button type="button" className="secondary-button" disabled={action.isPending} onClick={() => action.mutate({ id: run.id, action: "cancel" })}>إيقاف الإعداد</button> : run.status !== "succeeded" ? <button type="button" className="secondary-button" disabled={action.isPending} onClick={() => action.mutate({ id: run.id, action: "resume" })}>استئناف المراحل المتبقية</button> : null}
  {action.isError ? <p>تعذّر تنفيذ الطلب. إذا تغيرت أدوات القراءة فابدأ إعداداً جديداً.</p> : null}
 </div>;
}
