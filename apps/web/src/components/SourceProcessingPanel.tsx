import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, FileText, Upload, X } from "lucide-react";
import { ChangeEvent, useState } from "react";
import { fetchSourceProcessing, reviewSourceCandidate, uploadSourceFile } from "../lib/api";
import { ApiError } from "../lib/api";
import { StatusBadge } from "./StatusBadge";
import type { SourceCandidate, SourceProcessing } from "../types";

interface SourceProcessingPanelProps {
  sourceId: string;
  sourceTitle: string;
}

// The same format matrix the API enforces: text plus JSON and XML. The API
// refuses anything else with the same list, so this copy is the pre-upload
// version of the answer the caller would otherwise get after a failed upload.
const SUPPORTED_UPLOAD_MEDIA_TYPES = ["text/*", "application/json", "application/xml"];
const SUPPORTED_UPLOAD_ACCEPT = "text/*,.txt,.md,.json,.xml,application/json,application/xml";
const SUPPORTED_UPLOAD_HINT = "الصيغ المدعومة: نص (text/*) وملفات JSON و XML بترميز UTF-8. صيغ PDF والصور والصيغ الثنائية غير مدعومة في هذه النسخة.";
const UNDECLARED_UPLOAD_MEDIA_TYPE = "application/octet-stream";

function isSupportedUploadType(value: string): boolean {
  const normalized = value.split(";")[0].trim().toLowerCase();
  if (normalized === "" || normalized === UNDECLARED_UPLOAD_MEDIA_TYPE) {
    return true;
  }
  return SUPPORTED_UPLOAD_MEDIA_TYPES.some((entry) => (entry.endsWith("/*") ? normalized.startsWith(entry.slice(0, -1)) : normalized === entry));
}

export function SourceProcessingPanel({ sourceId, sourceTitle }: SourceProcessingPanelProps) {
  const queryClient = useQueryClient();
  const [selectedFile, setSelectedFile] = useState<File | null>(null);
  const [reviewNote, setReviewNote] = useState("");
  const [message, setMessage] = useState("");
  const processingQuery = useQuery({
    queryKey: ["source-processing", sourceId],
    queryFn: () => fetchSourceProcessing(sourceId),
    enabled: Boolean(sourceId),
    refetchInterval: (query) => {
      const current = query.state.data as SourceProcessing | undefined;
      if (!current || current.runs.length === 0) return false;
      const latest = current.runs[0];
      if (latest.status === "queued" || latest.status === "running") return 2000;
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
  const processing = processingQuery.data;
  const error = processingQuery.error;
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
          <p>ارفع ملفاً نصياً، ثم تبقى نتائج الاستخراج مرشحات تحتاج قراراً صريحاً.</p>
        </div>
        <FileText size={18} />
      </div>
      <div className="source-processing-upload">
        <label className="composer-label source-processing-file">
          ملف المصدر
          <input type="file" accept={SUPPORTED_UPLOAD_ACCEPT} onChange={chooseFile} />
          <small>{SUPPORTED_UPLOAD_HINT}</small>
        </label>
        <button className="secondary-button" type="button" disabled={!selectedFile || uploadMutation.isPending} onClick={() => uploadMutation.mutate()}>
          <Upload size={15} /> {uploadMutation.isPending ? "جارٍ الرفع…" : "رفع ومعالجة"}
        </button>
      </div>
      {message ? <div className="evidence-workspace-message" role="status">{message}</div> : null}
      {error ? <div className="evidence-workspace-error" role="alert">{errorMessage(error)}</div> : null}
      {processingQuery.isPending ? <p className="evidence-empty">جارٍ تحميل حالة المعالجة…</p> : null}
      {processing ? <ProcessingSummary processing={processing} reviewPending={reviewMutation.isPending} reviewNote={reviewNote} setReviewNote={setReviewNote} onReview={(candidateId, decision) => reviewMutation.mutate({ candidateId, decision, note: reviewNote })} /> : null}
    </section>
  );
}

function ProcessingSummary({ processing, reviewPending, reviewNote, setReviewNote, onReview }: { processing: SourceProcessing; reviewPending: boolean; reviewNote: string; setReviewNote: (value: string) => void; onReview: (candidateId: string, decision: "accepted" | "rejected") => void }) {
  const latestRun = processing.runs[0];
  return (
    <div className="source-processing-summary">
      <div className="source-processing-run">
        <div><strong>حالة التشغيل</strong><span>{latestRun ? runStatusLabel(latestRun.status) : "لم تبدأ"}</span></div>
        {latestRun ? <small>{latestRun.pageCount} صفحة · {latestRun.passageCount} مقطع · {latestRun.candidateCount} مرشح</small> : null}
        {latestRun?.error ? <p className="source-processing-error">{latestRun.error}</p> : null}
      </div>
      <label className="composer-label source-review-note">ملاحظة القرار<textarea value={reviewNote} onChange={(event) => setReviewNote(event.target.value)} rows={2} placeholder="اختياري: سبب القبول أو الرفض" /></label>
      {processing.files.length > 0 ? <div className="source-processing-files">{processing.files.map((file) => <div className="source-processing-file" key={file.id}><div><span>{file.originalFilenameAr}</span>{file.processingError ? <p className="source-processing-error">{file.processingError}</p> : null}</div><StatusBadge tone={file.processingStatus === "failed" ? "disputed" : "source"}>{file.processingStatus === "succeeded" ? "اكتملت" : file.processingStatus === "failed" ? "فشلت" : "قيد المعالجة"}</StatusBadge></div>)}</div> : null}
      {processing.candidates.length > 0 ? <div className="source-candidate-list">{processing.candidates.map((candidate) => <CandidateRow candidate={candidate} key={candidate.id} reviewPending={reviewPending} onReview={onReview} />)}</div> : latestRun?.status === "succeeded" ? <p className="evidence-empty">اكتملت المعالجة دون استخراج مرشحين.</p> : null}
    </div>
  );
}

function CandidateRow({ candidate, reviewPending, onReview }: { candidate: SourceCandidate; reviewPending: boolean; onReview: (candidateId: string, decision: "accepted" | "rejected") => void }) {
  const canReview = candidate.status === "unreviewed" || candidate.status === "needs_review";
  return (
    <article className="source-candidate-row">
      <div className="source-candidate-row-head"><StatusBadge tone={candidate.candidateType === "claim" ? "claim" : "source"}>{candidate.candidateType === "claim" ? "علاقة" : "كيان"}</StatusBadge><span>{Math.round(candidate.confidence * 100)}%</span><small>{candidate.pageNumber ? `صفحة ${candidate.pageNumber}` : candidate.locatorAr || "دون موضع"}</small></div>
      <p className="source-candidate-raw">{candidate.rawTextAr}</p>
      <p className="source-candidate-passage">{candidate.passageTextAr}</p>
      <div className="source-candidate-meta"><span>{candidate.candidateType === "claim" ? relationLabel(candidate) : candidate.proposedEntityNameAr ? `مطابقة مقترحة: ${candidate.proposedEntityNameAr}` : "لا توجد مطابقة كيان مقترحة"}</span>{candidate.proposedMatchScore !== undefined ? <span>درجة المطابقة: {Math.round(candidate.proposedMatchScore * 100)}%</span> : null}<span>{candidate.rationaleAr}</span></div>
      {canReview ? <div className="source-candidate-actions"><button className="primary-button" type="button" disabled={reviewPending} onClick={() => onReview(candidate.id, "accepted")}><Check size={14} /> قبول</button><button className="secondary-button" type="button" disabled={reviewPending} onClick={() => onReview(candidate.id, "rejected")}><X size={14} /> رفض</button></div> : <div className="source-candidate-reviewed">{candidate.status === "accepted" ? "مقبول" : "مرفوض"} · {candidate.reviews.length} قرار</div>}
    </article>
  );
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

function errorMessage(error: unknown): string {
  if (error instanceof ApiError) return error.message;
  return error instanceof Error ? error.message : "تعذر إكمال عملية المعالجة.";
}
