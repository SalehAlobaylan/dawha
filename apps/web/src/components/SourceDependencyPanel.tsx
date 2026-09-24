import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, RefreshCw, ShieldAlert, X } from "lucide-react";
import { FormEvent, useState } from "react";
import { ApiError, createSourceDependency, detectSourceDependencies, fetchSourceDependencies, reviewSourceDependency } from "../lib/api";
import { StatusBadge } from "./StatusBadge";
import type { CreateSourceDependencyInput, SourceDependency, SourceDependencyGraph, SourceDependencyType, SourceMetadata } from "../types";

interface SourceDependencyPanelProps {
  source: SourceMetadata;
  allSources: SourceMetadata[];
  initialGraph?: SourceDependencyGraph;
}

export function SourceDependencyPanel({ source, allSources, initialGraph }: SourceDependencyPanelProps) {
  const queryClient = useQueryClient();
  const [targetSourceID, setTargetSourceID] = useState("");
  const [dependencyType, setDependencyType] = useState<SourceDependencyType>("cites");
  const [evidence, setEvidence] = useState("");
  const [reviewNotes, setReviewNotes] = useState<Record<string, string>>({});
  const [message, setMessage] = useState("");
  const graphQuery = useQuery({
    queryKey: ["source-dependencies", source.id],
    queryFn: () => fetchSourceDependencies(source.id),
    enabled: Boolean(source.id),
    initialData: initialGraph,
  });
  const refresh = async (graph: SourceDependencyGraph, successMessage: string) => {
    queryClient.setQueryData(["source-dependencies", source.id], graph);
    setMessage(successMessage);
    setEvidence("");
    setReviewNotes({});
    await queryClient.invalidateQueries({ queryKey: ["source", source.id] });
    await queryClient.invalidateQueries({ queryKey: ["sources"] });
  };
  const createMutation = useMutation({
    mutationFn: (input: CreateSourceDependencyInput) => createSourceDependency(source.id, input),
    onSuccess: (graph) => refresh(graph, "أُضيفت العلاقة كمرشحة، وتنتظر المراجعة."),
    onError: (error) => setMessage(errorMessage(error)),
  });
  const detectMutation = useMutation({
    mutationFn: () => detectSourceDependencies(source.id),
    onSuccess: (graph) => refresh(graph, `اكتمل الفحص: ${graph.detectedCount} مرشحة جديدة.`),
    onError: (error) => setMessage(errorMessage(error)),
  });
  const reviewMutation = useMutation({
    mutationFn: ({ dependencyID, decision }: { dependencyID: string; decision: "confirmed" | "rejected" }) => reviewSourceDependency(dependencyID, { decision, note_ar: reviewNotes[dependencyID] || undefined }),
    onSuccess: (graph) => refresh(graph, "حُفظ قرار المراجعة دون تغيير المصدر."),
    onError: (error) => setMessage(errorMessage(error)),
  });
  const graph = graphQuery.data ?? emptyGraph(source.id);
  const targets = allSources.filter((item) => item.id !== source.id);
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!targetSourceID) return;
    createMutation.mutate({ depends_on_source_id: targetSourceID, dependency_type: dependencyType, evidence_ar: evidence || undefined });
  };

  return (
    <section className="source-dependency-panel" aria-label="شبكة اعتماد المصادر">
      <div className="source-dependency-head">
        <div>
          <div className="eyebrow">تحليل الاعتماد</div>
          <h3>هل يعمل هذا المصدر مستقلاً؟</h3>
          <p>العلاقة إشارة تحتاج مراجعة، وليست حكماً على صحة المصدر.</p>
        </div>
        <button className="secondary-button" type="button" onClick={() => detectMutation.mutate()} disabled={detectMutation.isPending}>
          <RefreshCw size={14} /> {detectMutation.isPending ? "جارٍ الفحص…" : "فحص التشابه"}
        </button>
      </div>
      {message ? <div className="evidence-workspace-message" role="status">{message}</div> : null}
      {graphQuery.error ? <div className="evidence-workspace-error" role="alert">{errorMessage(graphQuery.error)}</div> : null}
      <div className="source-dependency-summary">
        <span><strong>{graph.summary.total}</strong> علاقة</span>
        <span><strong>{graph.summary.needsReview}</strong> تحتاج مراجعة</span>
        <span><strong>{graph.summary.confirmed}</strong> مؤكدة</span>
        <span><strong>{graph.summary.independentSourceCount}</strong> مصدر غير مرتبط مباشرة</span>
      </div>
      <form className="source-dependency-form" onSubmit={submit}>
        <label className="composer-label">المصدر المرتبط<select required value={targetSourceID} onChange={(event) => setTargetSourceID(event.target.value)}><option value="">اختر مصدراً</option>{targets.map((item) => <option value={item.id} key={item.id}>{item.titleAr}</option>)}</select></label>
        <label className="composer-label">نوع العلاقة<select value={dependencyType} onChange={(event) => setDependencyType(event.target.value as SourceDependencyType)}><option value="cites">يستشهد به</option><option value="derived_from">مشتق منه</option><option value="likely_paraphrase"> إعادة صياغة محتملة</option><option value="shared_origin">أصل مشترك</option><option value="unknown">غير محدد</option></select></label>
        <label className="composer-label source-dependency-evidence">الدليل<textarea value={evidence} onChange={(event) => setEvidence(event.target.value)} rows={2} placeholder="ما الذي يربط المصدرين؟" /></label>
        <button className="primary-button" type="submit" disabled={createMutation.isPending || !targets.length}>{createMutation.isPending ? "جارٍ الحفظ…" : "أضف كمرشحة"}</button>
      </form>
      <div className="source-dependency-list">
        {graph.items.length ? graph.items.map((dependency) => <DependencyRow dependency={dependency} reviewPending={reviewMutation.isPending} reviewNote={reviewNotes[dependency.id] || ""} setReviewNote={(value) => setReviewNotes((current) => ({ ...current, [dependency.id]: value }))} onReview={(dependencyID, decision) => reviewMutation.mutate({ dependencyID, decision })} key={dependency.id} />) : <p className="evidence-empty">لا توجد علاقات اعتماد محفوظة لهذا المصدر.</p>}
      </div>
    </section>
  );
}

function DependencyRow({ dependency, reviewPending, reviewNote, setReviewNote, onReview }: { dependency: SourceDependency; reviewPending: boolean; reviewNote: string; setReviewNote: (value: string) => void; onReview: (dependencyID: string, decision: "confirmed" | "rejected") => void }) {
  const signal = signalText(dependency);
  return (
    <article className="source-dependency-row">
      <div className="source-dependency-row-head"><div><strong>{dependency.dependsOnSourceTitleAr || "مصدر غير معروف"}</strong><small>{dependencyTypeLabel(dependency.dependencyType)}</small></div><StatusBadge tone={dependencyTone(dependency.status)}>{dependencyStatusLabel(dependency.status)}</StatusBadge></div>
      <p>{dependency.evidenceAr || "لا يوجد دليل نصي."}</p>
      {signal ? <small className="source-dependency-signal"><ShieldAlert size={12} /> {signal}</small> : null}
      <div className="source-dependency-meta"><span>{dependency.algorithmVersion || "إدخال يدوي"}</span>{dependency.reviewedAt ? <span>راجع {dependency.reviewedAt.slice(0, 10)}</span> : null}</div>
      {dependency.status === "needs_review" ? <div className="source-dependency-actions"><label className="composer-label">ملاحظة القرار<input value={reviewNote} onChange={(event) => setReviewNote(event.target.value)} placeholder="اختياري" /></label><button className="primary-button" type="button" disabled={reviewPending} onClick={() => onReview(dependency.id, "confirmed")}><Check size={13} /> تأكيد</button><button className="secondary-button" type="button" disabled={reviewPending} onClick={() => onReview(dependency.id, "rejected")}><X size={13} /> رفض</button></div> : null}
    </article>
  );
}

function emptyGraph(sourceID: string): SourceDependencyGraph {
  return { sourceId: sourceID, items: [], summary: { total: 0, needsReview: 0, confirmed: 0, rejected: 0, independentSourceCount: 0 }, detectedCount: 0, scannedPassageCount: 0, truncated: false };
}

function dependencyTypeLabel(value: SourceDependencyType): string {
  if (value === "cites") return "يستشهد به";
  if (value === "derived_from") return "مشتق منه";
  if (value === "likely_paraphrase") return "إعادة صياغة محتملة";
  if (value === "shared_origin") return "أصل مشترك";
  return "علاقة غير محددة";
}

function dependencyStatusLabel(value: SourceDependency["status"]): string {
  if (value === "confirmed") return "مؤكدة";
  if (value === "rejected") return "مرفوضة";
  return "تحتاج مراجعة";
}

function dependencyTone(value: SourceDependency["status"]): "source" | "finding" | "question" {
  if (value === "confirmed") return "source";
  if (value === "rejected") return "question";
  return "finding";
}

function signalText(dependency: SourceDependency): string {
  const signals = dependency.signalData.signals;
  if (!Array.isArray(signals)) return "";
  return signals.map((value) => {
    if (!value || typeof value !== "object") return "";
    const signal = value as Record<string, unknown>;
    if (signal.type === "repeated_wording") return `تشابه صياغة ${Math.round(Number(signal.similarity ?? 0) * 100)}%`;
    if (signal.type === "shared_claim_sequence") return "تسلسل ادعاءات مشترك";
    return "";
  }).filter(Boolean).join(" · ");
}

function errorMessage(error: unknown): string {
  if (error instanceof ApiError) return error.message;
  return error instanceof Error ? error.message : "تعذر إكمال تحليل الاعتماد.";
}
