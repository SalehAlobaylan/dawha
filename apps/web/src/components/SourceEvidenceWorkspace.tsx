import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { BookOpen, FilePlus2, Link2, Plus, Quote, ShieldAlert } from "lucide-react";
import { FormEvent, useState } from "react";
import { addClaimEvidence, ApiError, createClaim, createSource, createSourcePassage, createSourceStatement, fetchSource, fetchSources } from "../lib/api";
import { SourceProcessingPanel } from "./SourceProcessingPanel";
import { SourceDependencyPanel } from "./SourceDependencyPanel";
import { SourceCharacterizationPanel } from "./SourceCharacterizationPanel";
import { StatusBadge } from "./StatusBadge";
import type { AddEvidenceInput, CreatePassageInput, CreateStatementInput, ResearchClaim, SourceDetail } from "../types";

const sourceTypeOptions = [
  ["manuscript", "مخطوط"],
  ["book", "كتاب"],
  ["family_document", "وثيقة عائلية"],
  ["archive_record", "سجل أرشيفي"],
  ["newspaper", "صحافة"],
  ["article", "مقال"],
  ["oral_testimony", "رواية شفوية"],
  ["website", "موقع"],
  ["other", "أخرى"],
] as const;

export function SourceEvidenceWorkspace() {
  const queryClient = useQueryClient();
  const [selectedSourceId, setSelectedSourceId] = useState("");
  const [sourceOpen, setSourceOpen] = useState(false);
  const [title, setTitle] = useState("");
  const [author, setAuthor] = useState("");
  const [sourceType, setSourceType] = useState("manuscript");
  const [sourceDate, setSourceDate] = useState("");
  const [citation, setCitation] = useState("");
  const [passageText, setPassageText] = useState("");
  const [passageLocator, setPassageLocator] = useState("");
  const [passagePage, setPassagePage] = useState("");
  const [statementText, setStatementText] = useState("");
  const [statementLocator, setStatementLocator] = useState("");
  const [statementPassageId, setStatementPassageId] = useState("");
  const [claimSubjectId, setClaimSubjectId] = useState("10000000-0000-0000-0000-000000000001");
  const [claimPredicate, setClaimPredicate] = useState("father_of");
  const [claimObjectId, setClaimObjectId] = useState("10000000-0000-0000-0000-000000000002");
  const [claimStatus, setClaimStatus] = useState("unresolved");
  const [claimNotes, setClaimNotes] = useState("");
  const [claim, setClaim] = useState<ResearchClaim | null>(null);
  const [evidenceTarget, setEvidenceTarget] = useState("");
  const [evidenceRelation, setEvidenceRelation] = useState<"supports" | "contextualizes" | "contradicts" | "refutes">("supports");
  const [evidenceNote, setEvidenceNote] = useState("");
  const [message, setMessage] = useState("");
  const sourcesQuery = useQuery({ queryKey: ["sources"], queryFn: fetchSources });
  const sourceQuery = useQuery({ queryKey: ["source", selectedSourceId], queryFn: () => fetchSource(selectedSourceId), enabled: Boolean(selectedSourceId) });
  const createSourceMutation = useMutation({
    mutationFn: () => createSource({ title_ar: title, author_ar: author || undefined, source_type: sourceType, publication_date_from: sourceDate || undefined, citation_ar: citation || undefined }),
    onSuccess: async (created) => {
      setSelectedSourceId(created.source.id);
      setSourceOpen(false);
      setTitle("");
      setAuthor("");
      setCitation("");
      setMessage("أُنشئ المصدر، وأصبح مواضعه قابلة للتسجيل.");
      await queryClient.invalidateQueries({ queryKey: ["sources"] });
      await queryClient.invalidateQueries({ queryKey: ["source-dependencies", created.source.id] });
    },
    onError: (error) => setMessage(errorMessage(error)),
  });
  const createPassageMutation = useMutation({
    mutationFn: (input: CreatePassageInput) => createSourcePassage(selectedSourceId, input),
    onSuccess: async (updated) => {
      setPassageText("");
      setPassageLocator("");
      setPassagePage("");
      setMessage("سُجل المقطع مع موضعه.");
      queryClient.setQueryData(["source", selectedSourceId], updated);
      await queryClient.invalidateQueries({ queryKey: ["sources"] });
      await queryClient.invalidateQueries({ queryKey: ["source-dependencies", selectedSourceId] });
      await queryClient.invalidateQueries({ queryKey: ["source-characterization", selectedSourceId] });
    },
    onError: (error) => setMessage(errorMessage(error)),
  });
  const createStatementMutation = useMutation({
    mutationFn: (input: CreateStatementInput) => createSourceStatement(selectedSourceId, input),
    onSuccess: async (updated) => {
      setStatementText("");
      setStatementLocator("");
      setStatementPassageId("");
      setMessage("سُجّلت عبارة المصدر دون تحويلها إلى حقيقة.");
      queryClient.setQueryData(["source", selectedSourceId], updated);
      await queryClient.invalidateQueries({ queryKey: ["sources"] });
      await queryClient.invalidateQueries({ queryKey: ["source-dependencies", selectedSourceId] });
      await queryClient.invalidateQueries({ queryKey: ["source-characterization", selectedSourceId] });
    },
    onError: (error) => setMessage(errorMessage(error)),
  });
  const createClaimMutation = useMutation({
    mutationFn: () => createClaim({ subject_type: "person", subject_id: claimSubjectId, predicate: claimPredicate, object_type: "person", object_id: claimObjectId, status: claimStatus, notes_ar: claimNotes || undefined }),
    onSuccess: (created) => {
      setClaim(created);
      setMessage("حُفظ الادعاء كطبقة بحثية قابلة للربط.");
    },
    onError: (error) => setMessage(errorMessage(error)),
  });
  const evidenceMutation = useMutation({
    mutationFn: (input: AddEvidenceInput) => addClaimEvidence(claim?.id ?? "", input),
    onSuccess: (updated) => {
      setClaim(updated);
      setEvidenceNote("");
      setEvidenceTarget("");
      setMessage("رُبط الدليل بالادعاء، مع إبقاء العلاقة قابلة للمراجعة.");
    },
    onError: (error) => setMessage(errorMessage(error)),
  });
  const selectedSource = sourceQuery.data;
  const sourceItems = sourcesQuery.data ?? [];
  const queryError = sourcesQuery.error ?? sourceQuery.error;

  const submitSource = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    createSourceMutation.mutate();
  };
  const submitPassage = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    createPassageMutation.mutate({ text_ar: passageText, locator_ar: passageLocator || undefined, page_number: passagePage ? Number(passagePage) : undefined });
  };
  const submitStatement = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    createStatementMutation.mutate({ statement_text_ar: statementText, source_passage_id: statementPassageId || undefined, locator_ar: statementLocator || undefined, review_status: "unreviewed" });
  };
  const submitClaim = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    createClaimMutation.mutate();
  };
  const submitEvidence = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const isStatement = evidenceTarget.startsWith("statement:");
    evidenceMutation.mutate({ [isStatement ? "source_statement_id" : "source_passage_id"]: evidenceTarget.split(":")[1], relation: evidenceRelation, evidence_note_ar: evidenceNote || undefined } as AddEvidenceInput);
  };

  return (
    <section className="evidence-workspace" id="evidence-workspace">
      <div className="evidence-workspace-head"><div><div className="eyebrow">مساحة المصدر والدليل</div><h2>ابدأ من النص، ثم اربط الادعاء</h2><p>المصدر يُحفظ كمواد قابلة للتتبع، والعبارة تبقى منفصلة عن الحكم الذي قد ينشأ عنها.</p></div><button className="primary-button" type="button" onClick={() => setSourceOpen((open) => !open)}><Plus size={15} /> مصدر جديد</button></div>
      {message ? <div className="evidence-workspace-message" role="status">{message}</div> : null}
      {queryError ? <div className="evidence-workspace-error" role="alert">{errorMessage(queryError)} {queryError instanceof ApiError && queryError.status === 503 ? <span>شغّل Core API وVITE_API_URL لتفعيل الإضافة.</span> : null}</div> : null}
      {sourceOpen ? <form className="evidence-source-form" onSubmit={submitSource}><div className="evidence-form-title"><FilePlus2 size={16} /><strong>بيانات المصدر</strong></div><div className="evidence-form-grid"><label className="composer-label">العنوان<input required value={title} onChange={(event) => setTitle(event.target.value)} placeholder="اسم المصدر بالعربية" /></label><label className="composer-label">المؤلف<input value={author} onChange={(event) => setAuthor(event.target.value)} placeholder="اختياري" /></label><label className="composer-label">النوع<select value={sourceType} onChange={(event) => setSourceType(event.target.value)}>{sourceTypeOptions.map(([value, label]) => <option key={value} value={value}>{label}</option>)}</select></label><label className="composer-label">سنة النشر<input type="date" value={sourceDate} onChange={(event) => setSourceDate(event.target.value)} /></label><label className="composer-label">موضع الاقتباس<input value={citation} onChange={(event) => setCitation(event.target.value)} placeholder="الجزء، الصفحة، أو الرمز" /></label></div><div className="evidence-form-actions"><button className="secondary-button" type="button" onClick={() => setSourceOpen(false)}>إلغاء</button><button className="primary-button" type="submit" disabled={createSourceMutation.isPending}>{createSourceMutation.isPending ? "جارٍ الحفظ…" : "احفظ المصدر"}</button></div></form> : null}
      <div className="evidence-workspace-grid">
        <div className="evidence-source-column"><div className="evidence-column-heading"><div><div className="detail-label">المكتبة المحفوظة</div><small>{sourceItems.length} مصادر</small></div><BookOpen size={16} /></div>{sourcesQuery.isPending ? <p className="evidence-empty">جارٍ تحميل المصادر…</p> : sourceItems.length > 0 ? <div className="evidence-source-list">{sourceItems.map((item) => <button className={`evidence-source-item${selectedSourceId === item.id ? " evidence-source-item-active" : ""}`} type="button" key={item.id} onClick={() => { setSelectedSourceId(item.id); setClaim(null); }}><span><strong>{item.titleAr}</strong><small>{sourceTypeLabel(item.sourceType)} · {item.statementCount} عبارات</small></span><span>{item.passageCount}</span></button>)}</div> : <p className="evidence-empty">لم تُحفظ مصادر بعد. أضف أول مصدر للبدء.</p>}</div>
        <div className="evidence-detail-column">{selectedSource ? <SourceDetailForms source={selectedSource} passagePending={createPassageMutation.isPending} statementPending={createStatementMutation.isPending} passageText={passageText} setPassageText={setPassageText} passageLocator={passageLocator} setPassageLocator={setPassageLocator} passagePage={passagePage} setPassagePage={setPassagePage} statementText={statementText} setStatementText={setStatementText} statementLocator={statementLocator} setStatementLocator={setStatementLocator} statementPassageId={statementPassageId} setStatementPassageId={setStatementPassageId} onPassage={submitPassage} onStatement={submitStatement} /> : sourceQuery.isPending ? <p className="evidence-empty">جارٍ فتح المصدر…</p> : <div className="evidence-empty"><Quote size={19} /><p>اختر مصدراً لإضافة مقطع أو عبارة مصدر.</p></div>}</div>
      </div>
      {selectedSource ? <SourceProcessingPanel sourceId={selectedSource.source.id} sourceTitle={selectedSource.source.titleAr} /> : null}
      {selectedSource ? <SourceDependencyPanel source={selectedSource.source} allSources={sourceItems} initialGraph={{ sourceId: selectedSource.source.id, items: selectedSource.dependencies, summary: selectedSource.dependencySummary, detectedCount: 0, scannedPassageCount: 0, truncated: false }} /> : null}
      {selectedSource ? <SourceCharacterizationPanel source={selectedSource.source} /> : null}
      <div className="evidence-claim-area"><div className="evidence-column-heading"><div><div className="detail-label">الادعاء والدليل</div><small>اكتب الفرضية، ثم اربط ما يدعمها أو يعارضها.</small></div><ShieldAlert size={16} /></div><div className="evidence-claim-grid"><form className="evidence-claim-form" onSubmit={submitClaim}><div className="evidence-form-title"><Link2 size={16} /><strong>ادعاء جديد</strong></div><div className="evidence-form-grid"><label className="composer-label">معرف الموضوع<input required value={claimSubjectId} onChange={(event) => setClaimSubjectId(event.target.value)} /></label><label className="composer-label">العلاقة<input required value={claimPredicate} onChange={(event) => setClaimPredicate(event.target.value)} placeholder="father_of" /></label><label className="composer-label">معرف الموضوع الآخر<input required value={claimObjectId} onChange={(event) => setClaimObjectId(event.target.value)} /></label><label className="composer-label">الحالة<select value={claimStatus} onChange={(event) => setClaimStatus(event.target.value)}><option value="unresolved">غير محسوم</option><option value="supported">مدعوم مبدئياً</option><option value="disputed">متنازع عليه</option></select></label><label className="composer-label">ملاحظة<textarea value={claimNotes} onChange={(event) => setClaimNotes(event.target.value)} rows={2} /></label></div><button className="primary-button" type="submit" disabled={createClaimMutation.isPending}>{createClaimMutation.isPending ? "جارٍ الحفظ…" : "احفظ الادعاء"}</button></form><div className="evidence-claim-result">{claim ? <><div className="evidence-claim-result-head"><StatusBadge tone="claim">{claim.status}</StatusBadge><span>{claim.predicate}</span></div><div className="evidence-evidence-list">{claim.evidence.length > 0 ? claim.evidence.map((item) => <div className="evidence-evidence-row" key={item.id}><DependencyWarning status={item.dependencyStatus} /><StatusBadge tone={item.relation === "supports" ? "source" : item.relation === "contradicts" || item.relation === "refutes" ? "disputed" : "claim"}>{evidenceRelationLabel(item.relation)}</StatusBadge><div><strong>{item.sourceTitleAr ?? "مصدر"}</strong><p>{item.statementTextAr ?? item.passageTextAr ?? "مقطع مصدر"}</p></div></div>) : <p className="evidence-empty">لم يُربط دليل بعد.</p>}</div><form className="evidence-link-form" onSubmit={submitEvidence}><label className="composer-label">الدليل<select required value={evidenceTarget} onChange={(event) => setEvidenceTarget(event.target.value)}><option value="">اختر عبارة أو مقطعاً</option>{selectedSource?.statements.map((item) => <option key={item.id} value={`statement:${item.id}`}>عبارة: {item.statementTextAr}</option>)}{selectedSource?.passages.map((item) => <option key={item.id} value={`passage:${item.id}`}>مقطع {item.sequenceNumber}: {item.textAr.slice(0, 55)}</option>)}</select></label><div className="evidence-link-fields"><label className="composer-label">العلاقة<select value={evidenceRelation} onChange={(event) => setEvidenceRelation(event.target.value as typeof evidenceRelation)}><option value="supports">يدعم</option><option value="contextualizes">يضع سياقاً</option><option value="contradicts">يعارض</option><option value="refutes">ينفي</option></select></label><label className="composer-label">ملاحظة<input value={evidenceNote} onChange={(event) => setEvidenceNote(event.target.value)} placeholder="لماذا هذا دليل؟" /></label></div><button className="secondary-button" type="submit" disabled={!evidenceTarget || evidenceMutation.isPending}>{evidenceMutation.isPending ? "جارٍ الربط…" : "اربط الدليل"}</button></form></> : <p className="evidence-empty">احفظ ادعاءً أولاً، ثم اختر عبارة من المصدر لربطها.</p>}</div></div></div>
    </section>
  );
}

function SourceDetailForms({ source, passagePending, statementPending, passageText, setPassageText, passageLocator, setPassageLocator, passagePage, setPassagePage, statementText, setStatementText, statementLocator, setStatementLocator, statementPassageId, setStatementPassageId, onPassage, onStatement }: { source: SourceDetail; passagePending: boolean; statementPending: boolean; passageText: string; setPassageText: (value: string) => void; passageLocator: string; setPassageLocator: (value: string) => void; passagePage: string; setPassagePage: (value: string) => void; statementText: string; setStatementText: (value: string) => void; statementLocator: string; setStatementLocator: (value: string) => void; statementPassageId: string; setStatementPassageId: (value: string) => void; onPassage: (event: FormEvent<HTMLFormElement>) => void; onStatement: (event: FormEvent<HTMLFormElement>) => void }) {
  return <div className="evidence-source-detail"><div className="evidence-source-detail-head"><div><div className="eyebrow">المصدر المحدد</div><h3>{source.source.titleAr}</h3><p>{source.source.citationAr || "دون موضع اقتباس"}</p></div><StatusBadge tone="source">{sourceTypeLabel(source.source.sourceType)}</StatusBadge></div><div className="evidence-passage-list">{source.passages.map((item) => <div className="evidence-passage-row" key={item.id}><span>{item.sequenceNumber}</span><div><small>{item.locatorAr || "دون صفحة"}</small><p>{item.textAr}</p></div></div>)}</div><form className="evidence-inline-form" onSubmit={onPassage}><div className="evidence-form-title"><Plus size={15} /><strong>أضف مقطعاً</strong></div><div className="evidence-inline-fields"><input required value={passageText} onChange={(event) => setPassageText(event.target.value)} placeholder="النص الأصلي للمقطع" /><input value={passageLocator} onChange={(event) => setPassageLocator(event.target.value)} placeholder="الموضع" /><input type="number" min="0" value={passagePage} onChange={(event) => setPassagePage(event.target.value)} placeholder="صفحة" /></div><button className="secondary-button" type="submit" disabled={passagePending}>{passagePending ? "جارٍ الحفظ…" : "حفظ المقطع"}</button></form><div className="evidence-statement-list">{source.statements.map((item) => <div className="evidence-statement-row" key={item.id}><Quote size={13} /><div><p>{item.statementTextAr}</p><small>{item.locatorAr || "دون موضع"} · {statementStatusLabel(item.reviewStatus)}</small></div></div>)}</div><form className="evidence-inline-form" onSubmit={onStatement}><div className="evidence-form-title"><Quote size={15} /><strong>سجّل عبارة المصدر</strong></div><div className="evidence-inline-fields"><input required value={statementText} onChange={(event) => setStatementText(event.target.value)} placeholder="ما الذي يقوله المصدر؟" /><input value={statementLocator} onChange={(event) => setStatementLocator(event.target.value)} placeholder="الموضع" /><select value={statementPassageId} onChange={(event) => setStatementPassageId(event.target.value)}><option value="">دون مقطع مرتبط</option>{source.passages.map((item) => <option key={item.id} value={item.id}>مقطع {item.sequenceNumber}</option>)}</select></div><button className="secondary-button" type="submit" disabled={statementPending}>{statementPending ? "جارٍ الحفظ…" : "حفظ العبارة"}</button></form></div>;
}

function DependencyWarning({ status }: { status?: string }) {
  if (status !== "derived" && status !== "likely_dependent") return null;
  return <span className="evidence-dependency-warning"><ShieldAlert size={11} /> {status === "derived" ? "مصدر يعتمد على مصدر آخر" : "ارتباط يحتاج مراجعة"}</span>;
}

function sourceTypeLabel(value: string): string {
  return sourceTypeOptions.find(([type]) => type === value)?.[1] ?? value;
}

function statementStatusLabel(value: string): string {
  if (value === "accepted") return "مقبولة";
  if (value === "needs_review") return "تحتاج مراجعة";
  if (value === "rejected") return "مرفوضة";
  return "غير مراجَعة";
}

function evidenceRelationLabel(value: string): string {
  if (value === "supports") return "يدعم";
  if (value === "contradicts") return "يعارض";
  if (value === "refutes") return "ينفي";
  return "سياق";
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : "تعذر إكمال عملية الدليل.";
}
