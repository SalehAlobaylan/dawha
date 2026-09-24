import { Link, useNavigate, useParams, useSearch } from "@tanstack/react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, ArrowLeft, BookOpen, Check, CircleHelp, FileSearch, GitBranch, Link2, Map, MessageCircleQuestion, Plus, RefreshCw, ShieldQuestion, Sparkles, UserRound, X } from "lucide-react";
import { useState } from "react";
import { demoDashboard, sources } from "../data/demo";
import { ApiError, addClaimEvidence, createClaim, createDispute, createQuestion, fetchResearchWorkspace, linkDisputeClaim, linkQuestionClaim, linkQuestionDispute, linkQuestionEntity, linkQuestionFinding, queryResearch, reviewEntityResolutionCandidate } from "../lib/api";
import type { CreateClaimInput, EntityResolutionReviewInput, EpistemicTone, ResearchQueryResult, ResearchWorkspaceSnapshot, WorkspaceClaim, WorkspaceFinding, WorkspaceIdentityCandidate, WorkspaceTimelineEvent } from "../types";
import { demoTreeDetail } from "../lib/api";
import { ResearchResultPanel } from "./research";
import { SectionHeading } from "../components/SectionHeading";
import { StatusBadge } from "../components/StatusBadge";
import { TopBar } from "../components/TopBar";

type IdentityDecision = "approve" | "reject" | "defer";

export function ResearchWorkspacePage() {
  const { questionId } = useParams({ from: "/research/$questionId" });
  const search = useSearch({ from: "/research/$questionId" });
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [result, setResult] = useState<ResearchQueryResult | null>(null);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const [claimDraft, setClaimDraft] = useState({ predicate: "", objectId: "", notes: "" });
  const [evidenceClaimId, setEvidenceClaimId] = useState("");
  const [evidenceStatementId, setEvidenceStatementId] = useState("");
  const workspaceContext = {
    entityType: isUuid(search.entityId) ? search.entityType : undefined,
    entityId: isUuid(search.entityId) ? search.entityId : undefined,
    treeId: isUuid(search.treeId) ? search.treeId : undefined,
    treeVersionId: isUuid(search.treeVersionId) ? search.treeVersionId : undefined,
  };
  const workspaceQuery = useQuery({
    queryKey: ["research-workspace", questionId, workspaceContext.entityType, workspaceContext.entityId, workspaceContext.treeId, workspaceContext.treeVersionId],
    queryFn: async () => {
      if (!isUuid(questionId)) return demoWorkspace(questionId, workspaceContext.entityType, workspaceContext.entityId);
      return fetchResearchWorkspace({ questionId, ...workspaceContext });
    },
    enabled: Boolean(questionId),
  });
  const snapshot = workspaceQuery.data;
  const queryKey = ["research-workspace", questionId];
  const invalidate = async () => queryClient.invalidateQueries({ queryKey });
  const runMutation = useMutation({
    mutationFn: async () => {
      if (!snapshot) throw new Error("مساحة البحث غير جاهزة.");
      const entityType = normalizeEntityType(snapshot.context.entityType);
      return queryResearch({ question: snapshot.context.questionTitleAr, question_id: isUuid(questionId) ? questionId : undefined, entity_type: entityType, entity_id: isUuid(snapshot.context.entityId) ? snapshot.context.entityId : undefined, tree_id: isUuid(snapshot.context.treeId) ? snapshot.context.treeId : undefined, tree_version_id: isUuid(snapshot.context.treeVersionId) ? snapshot.context.treeVersionId : undefined });
    },
    onSuccess: (value) => { setResult(value); setMessage("حُفظ التحقيق في السجل."); setError(""); void invalidate(); },
    onError: (value) => setError(actionError(value, "تعذر تشغيل التحقيق.")),
  });
  const claimMutation = useMutation({
    mutationFn: async () => {
      if (!snapshot) throw new Error("مساحة البحث غير جاهزة.");
      const subjectId = snapshot.context.entityId;
      const subjectType = normalizeEntityType(snapshot.context.entityType) ?? "person";
      if (!subjectId || !claimDraft.predicate.trim() || !claimDraft.objectId.trim()) throw new Error("أكمل وصف العلاقة ومعرف الكائن.");
      const input: CreateClaimInput = { subject_type: subjectType, subject_id: subjectId, predicate: claimDraft.predicate.trim(), object_type: "person", object_id: claimDraft.objectId.trim(), status: "unresolved", notes_ar: claimDraft.notes.trim() || undefined };
      const created = await createClaim(input);
      if (isUuid(questionId)) await linkQuestionClaim(questionId, { claim_id: created.id, role: "concerns" });
      return created;
    },
    onSuccess: async () => { setClaimDraft({ predicate: "", objectId: "", notes: "" }); setMessage("أُضيف الادعاء إلى مساحة العمل."); setError(""); await invalidate(); },
    onError: (value) => setError(actionError(value, "تعذر حفظ الادعاء.")),
  });
  const evidenceMutation = useMutation({
    mutationFn: async () => {
      if (!evidenceClaimId || !evidenceStatementId.trim()) throw new Error("اختر ادعاءً وأدخل معرّف العبارة المصدرية.");
      return addClaimEvidence(evidenceClaimId, { source_statement_id: evidenceStatementId.trim(), relation: "supports", evidence_note_ar: "ربط من مساحة البحث" });
    },
    onSuccess: async () => { setEvidenceClaimId(""); setEvidenceStatementId(""); setMessage("ربط الدليل بالادعاء."); setError(""); await invalidate(); },
    onError: (value) => setError(actionError(value, "تعذر ربط الدليل.")),
  });
  const disputeMutation = useMutation({
    mutationFn: async (claim: WorkspaceClaim) => {
      const created = await createDispute({ title_ar: `مراجعة ادعاء: ${claim.predicate}`, description_ar: "فُتح الخلاف من مساحة البحث.", status: "open" });
      const linked = await linkDisputeClaim(created.dispute.id, { claim_id: claim.id, position: "concerns" });
      if (isUuid(questionId)) await linkQuestionDispute(questionId, { dispute_id: created.dispute.id });
      return linked;
    },
    onSuccess: async () => { setMessage("فُتح خلاف للادعاء."); setError(""); await invalidate(); },
    onError: (value) => setError(actionError(value, "تعذر فتح الخلاف.")),
  });
  const questionMutation = useMutation({
    mutationFn: async () => {
      const created = await createQuestion({ title_ar: "سؤال جديد من مساحة البحث", status: "open", priority: "normal" });
      const entityType = normalizeEntityType(snapshot?.context.entityType);
      if (entityType && isUuid(snapshot?.context.entityId)) await linkQuestionEntity(created.question.id, { entity_type: entityType, entity_id: snapshot?.context.entityId ?? "" });
      return created;
    },
    onSuccess: async (value) => { setMessage("أُنشئ السؤال الجديد."); setError(""); await invalidate(); await navigate({ to: "/research/$questionId", params: { questionId: value.question.id }, search: workspaceContext }); },
    onError: (value) => setError(actionError(value, "تعذر إنشاء السؤال.")),
  });
  const findingMutation = useMutation({
    mutationFn: (finding: WorkspaceFinding) => linkQuestionFinding(questionId, { finding_id: finding.id }),
    onSuccess: async () => { setMessage("ربطت الملاحظة بالسؤال."); setError(""); await invalidate(); },
    onError: (value) => setError(actionError(value, "تعذر ربط الملاحظة.")),
  });
  const identityMutation = useMutation({
    mutationFn: ({ candidate, decision }: { candidate: WorkspaceIdentityCandidate; decision: IdentityDecision }) => reviewEntityResolutionCandidate(candidate.id, { decision } satisfies EntityResolutionReviewInput),
    onSuccess: async () => { setMessage("حُفظت مراجعة مرشح المطابقة."); setError(""); await invalidate(); },
    onError: (value) => setError(actionError(value, "تعذر تحديث المرشح.")),
  });

  if (workspaceQuery.isPending) return <WorkspaceState title="جارٍ فتح مساحة البحث…" description="تُجمع مكونات السؤال والسياق من مصادره." />;
  if (workspaceQuery.error || !snapshot) return <WorkspaceState title="تعذر فتح مساحة البحث" description={actionError(workspaceQuery.error, "تحقق من Core API ثم أعد المحاولة.")} error />;
  const permissions = snapshot.permissions;
  const entityLabel = snapshot.context.entityNameAr || "سياق السؤال";
  const evidenceClaims = snapshot.claims.filter((claim) => claim.id === evidenceClaimId);

  return <div className="page-stack workspace-page">
    <TopBar eyebrow="مكتب البحث / مساحة موحدة" title={snapshot.context.questionTitleAr} description={snapshot.context.questionDetailAr || "اجمع الشجرة والأدلة والخلاف والسؤال في مسار واحد قابل للتتبع."} />
    <section className="workspace-context-bar">
      <div className="workspace-context-main"><div className="workspace-context-icon"><UserRound size={18} /></div><div><div className="eyebrow">سياق البحث</div><strong>{entityLabel}</strong><span>{snapshot.context.entityType ? `${snapshot.context.entityType} · ${snapshot.context.entityId}` : "سؤال مفتوح"}</span></div></div>
      <div className="workspace-context-actions"><StatusBadge tone={snapshot.context.questionStatus === "under_investigation" ? "finding" : "question"}>{snapshot.context.questionStatus === "under_investigation" ? "قيد التحقيق" : snapshot.context.questionStatus}</StatusBadge><button className="primary-button" type="button" onClick={() => runMutation.mutate()} disabled={runMutation.isPending || !permissions.canRunResearch}><RefreshCw size={15} /> {runMutation.isPending ? "جارٍ التشغيل…" : "شغّل تحقيقاً"}</button></div>
    </section>
    {snapshot.context.entityAliasesAr?.length ? <div className="workspace-alias-line"><span>أسماء أخرى:</span>{snapshot.context.entityAliasesAr.map((alias) => <span className="workspace-alias" key={alias}>{alias}</span>)}</div> : null}
    {message ? <div className="workspace-feedback workspace-feedback-success" role="status"><Check size={15} />{message}</div> : null}
    {error ? <div className="workspace-feedback workspace-feedback-error" role="alert"><AlertTriangle size={15} />{error}<button type="button" aria-label="إغلاق الرسالة" onClick={() => setError("")}><X size={13} /></button></div> : null}
    <div className="workspace-action-strip"><div><Sparkles size={15} /><span>كل تعديل يبقى داخل سياق السؤال، ولا يغير المصادر التاريخية.</span></div><div className="workspace-action-links"><Link to="/contradictions">مراجعة التعارضات</Link><Link to="/entity-resolution">مطابقة الهوية</Link><Link to="/questions">كل الأسئلة</Link></div></div>
    {result ? <ResearchResultPanel result={result} /> : null}
    <div className="workspace-grid">
      <main className="workspace-main">
        <ClaimsPanel snapshot={snapshot} claimDraft={claimDraft} setClaimDraft={setClaimDraft} onCreateClaim={() => claimMutation.mutate()} claimPending={claimMutation.isPending} evidenceClaimId={evidenceClaimId} setEvidenceClaimId={setEvidenceClaimId} evidenceStatementId={evidenceStatementId} setEvidenceStatementId={setEvidenceStatementId} evidenceClaims={evidenceClaims} onLinkEvidence={() => evidenceMutation.mutate()} evidencePending={evidenceMutation.isPending} onDispute={(claim) => disputeMutation.mutate(claim)} disputePending={disputeMutation.isPending} canCreate={permissions.canCreateClaim} canLink={permissions.canLinkEvidence} canDispute={permissions.canDisputeClaim} />
        <TreePanel snapshot={snapshot} />
        <MapTimelinePanel snapshot={snapshot} />
        <SourcesPanel snapshot={snapshot} />
      </main>
      <aside className="workspace-side">
        <FindingsPanel snapshot={snapshot} onAttach={(finding) => findingMutation.mutate(finding)} canAttach={permissions.canAttachFinding} pending={findingMutation.isPending} />
        <QuestionsPanel snapshot={snapshot} context={workspaceContext} onCreate={() => questionMutation.mutate()} canCreate={permissions.canCreateQuestion} pending={questionMutation.isPending} />
        <IdentityPanel snapshot={snapshot} onReview={(candidate, decision) => identityMutation.mutate({ candidate, decision })} canReview={permissions.canReviewIdentityCandidate} pending={identityMutation.isPending} />
        <NotesPanel snapshot={snapshot} />
        <HistoryPanel snapshot={snapshot} />
      </aside>
    </div>
  </div>;
}

function ClaimsPanel({ snapshot, claimDraft, setClaimDraft, onCreateClaim, claimPending, evidenceClaimId, setEvidenceClaimId, evidenceStatementId, setEvidenceStatementId, evidenceClaims, onLinkEvidence, evidencePending, onDispute, disputePending, canCreate, canLink, canDispute }: { snapshot: ResearchWorkspaceSnapshot; claimDraft: { predicate: string; objectId: string; notes: string }; setClaimDraft: (value: { predicate: string; objectId: string; notes: string }) => void; onCreateClaim: () => void; claimPending: boolean; evidenceClaimId: string; setEvidenceClaimId: (value: string) => void; evidenceStatementId: string; setEvidenceStatementId: (value: string) => void; evidenceClaims: WorkspaceClaim[]; onLinkEvidence: () => void; evidencePending: boolean; onDispute: (claim: WorkspaceClaim) => void; disputePending: boolean; canCreate: boolean; canLink: boolean; canDispute: boolean }) {
  return <section className="workspace-panel workspace-claims-panel"><SectionHeading eyebrow="الطبقة الأولى" title="الادعاءات والأدلة" description="الدليل المؤيد والدليل المضاد يبقيان منفصلين." />
    <div className="workspace-claim-list">{snapshot.claims.length ? snapshot.claims.map((claim) => <article className="workspace-claim-card" key={claim.id}><div className="workspace-item-head"><div><span className="workspace-item-id">{claim.id.slice(0, 8)}</span><strong>{claim.predicate}</strong></div><StatusBadge tone={claimTone(claim.status)}>{claim.status}</StatusBadge></div><p>{claim.subjectType} {claim.subjectId.slice(0, 8)} <ArrowLeft size={11} /> {claim.objectType} {claim.objectId.slice(0, 8)}</p>{claim.notesAr ? <small>{claim.notesAr}</small> : null}<div className="workspace-evidence-columns"><div><strong>داعم</strong>{claim.evidence.filter((item) => item.kind === "evidence").map((item) => <EvidenceLine evidence={item} key={item.id} />)}{claim.evidence.filter((item) => item.kind === "evidence").length === 0 ? <span className="workspace-muted">لا يوجد دليل مباشر.</span> : null}</div><div><strong>مضاد / سياق</strong>{claim.evidence.filter((item) => item.kind === "counter_evidence").map((item) => <EvidenceLine evidence={item} key={item.id} />)}{claim.evidence.filter((item) => item.kind === "counter_evidence").length === 0 ? <span className="workspace-muted">لا يوجد دليل مضاد.</span> : null}</div></div><div className="workspace-card-actions">{canLink ? <button className="text-button" type="button" onClick={() => setEvidenceClaimId(claim.id)}><Link2 size={13} /> ربط دليل</button> : null}{canDispute ? <button className="text-button workspace-danger-action" type="button" onClick={() => onDispute(claim)} disabled={disputePending}><ShieldQuestion size={13} /> افتح خلافاً</button> : null}</div></article>) : <EmptyWorkspace label="لا توجد ادعاءات مرتبطة بالسياق." />}</div>
    {canCreate ? <form className="workspace-inline-form" onSubmit={(event) => { event.preventDefault(); onCreateClaim(); }}><div className="eyebrow">إضافة ادعاء</div><div className="workspace-form-grid"><input required value={claimDraft.predicate} onChange={(event) => setClaimDraft({ ...claimDraft, predicate: event.target.value })} placeholder="وصف العلاقة" /><input required value={claimDraft.objectId} onChange={(event) => setClaimDraft({ ...claimDraft, objectId: event.target.value })} placeholder="معرف الكائن" /><input value={claimDraft.notes} onChange={(event) => setClaimDraft({ ...claimDraft, notes: event.target.value })} placeholder="ملاحظة اختيارية" /><button className="primary-button" type="submit" disabled={claimPending}><Plus size={14} /> احفظ</button></div></form> : null}
    {canLink ? <form className="workspace-inline-form workspace-evidence-form" onSubmit={(event) => { event.preventDefault(); onLinkEvidence(); }}><div className="eyebrow">ربط مصدر بادعاء</div><div className="workspace-form-grid"><select required value={evidenceClaimId} onChange={(event) => setEvidenceClaimId(event.target.value)}><option value="">اختر ادعاءً</option>{snapshot.claims.map((claim) => <option value={claim.id} key={claim.id}>{claim.predicate} · {claim.id.slice(0, 8)}</option>)}</select><input required value={evidenceStatementId} onChange={(event) => setEvidenceStatementId(event.target.value)} placeholder="معرف عبارة المصدر" /><button className="secondary-button" type="submit" disabled={evidencePending || evidenceClaims.length === 0}><Link2 size={14} /> اربط</button></div></form> : null}
  </section>;
}

function TreePanel({ snapshot }: { snapshot: ResearchWorkspaceSnapshot }) {
  return <section className="workspace-panel"><SectionHeading eyebrow="الطبقة المحايدة" title="سياق الشجرة" description="الشجرة تفسير محفوظ، لا حقيقة نهائية." /><div className="workspace-tree-list">{snapshot.treeContexts.length ? snapshot.treeContexts.map((tree) => <article className="workspace-tree-card" key={tree.treeVersionId}><div className="workspace-item-head"><div><span className="eyebrow">نسخة {tree.versionNumber}</span><strong>{tree.treeNameAr}</strong></div><StatusBadge tone="interpretation">{tree.state === "published" ? "منشورة" : tree.state}</StatusBadge></div><div className="workspace-node-strip">{tree.nodes.map((node) => <span key={node.id}>{node.displayNameAr}</span>)}</div><div className="workspace-relationship-list">{tree.relationships.map((relationship) => <span key={relationship.id}>{relationship.predicate} · {relationship.status}{relationship.sourceTitleAr ? ` · ${relationship.sourceTitleAr}` : ""}</span>)}</div><Link className="text-button" to="/tree/$treeId/versions/$versionId" params={{ treeId: tree.treeId, versionId: tree.treeVersionId }}>فتح الشجرة <ArrowLeft size={13} /></Link></article>) : <EmptyWorkspace label="لا يوجد سياق شجرة مرتبط لهذا الكيان." />}</div></section>;
}

function MapTimelinePanel({ snapshot }: { snapshot: ResearchWorkspaceSnapshot }) {
  return <section className="workspace-panel workspace-map-panel"><SectionHeading eyebrow="الجغرافيا والزمن" title="الخريطة والخط الزمني" description="الأمكنة والموضعات تحتفظ بتاريخها." /><div className="workspace-map-summary"><div className="workspace-map-heading"><Map size={16} /><strong>الموضعات</strong><span>{snapshot.mapFeatures.length}</span></div>{snapshot.mapFeatures.length ? snapshot.mapFeatures.map((feature) => <div className="workspace-map-row" key={feature.id}><span>{feature.placeName || "موضع غير مسمى"}</span><small>{feature.kind} · {feature.status}</small>{feature.sourceTitle ? <Link to="/sources">{feature.sourceTitle}</Link> : null}</div>) : <EmptyWorkspace label="لا توجد موضعات مباشرة لهذا الكيان." />}</div><div className="workspace-timeline"><div className="workspace-map-heading"><GitBranch size={16} /><strong>الخط الزمني</strong><span>{snapshot.timeline.length}</span></div>{snapshot.timeline.length ? snapshot.timeline.map((event) => <TimelineLine event={event} key={event.id} />) : <EmptyWorkspace label="لا توجد أحداث زمنية." />}</div></section>;
}

function SourcesPanel({ snapshot }: { snapshot: ResearchWorkspaceSnapshot }) {
  return <section className="workspace-panel"><SectionHeading eyebrow="المكتبة" title="المصادر ذات الصلة" description="كل مصدر يبقى قابلاً للفتح والتتبع." /><div className="workspace-source-list">{snapshot.sources.length ? snapshot.sources.map((source) => <article className="workspace-source-card" key={source.id}><div className="workspace-item-head"><div><BookOpen size={15} /><strong>{source.titleAr}</strong></div><StatusBadge tone={source.dependencyStatus === "independent" ? "source" : "finding"}>{source.dependencyStatus}</StatusBadge></div><p>{source.authorAr || "مؤلف غير محدد"} · {source.sourceType}</p><small>{source.passageCount} مقطع · {source.statementCount} عبارة</small><Link className="text-button" to="/sources">فتح المصدر <ArrowLeft size={13} /></Link></article>) : <EmptyWorkspace label="لا توجد مصادر ظاهرة لهذا السؤال." />}</div></section>;
}

function FindingsPanel({ snapshot, onAttach, canAttach, pending }: { snapshot: ResearchWorkspaceSnapshot; onAttach: (finding: WorkspaceFinding) => void; canAttach: boolean; pending: boolean }) {
  return <section className="workspace-panel"><SectionHeading eyebrow="إشارات النظام" title="الملاحظات والتعارضات" description="الملاحظة لا تحوّل الادعاء إلى حقيقة." /><div className="workspace-finding-list">{snapshot.findings.length ? snapshot.findings.map((finding) => <article className="workspace-finding-card" key={finding.id}><div className="workspace-item-head"><div><span className="workspace-item-id">{finding.findingType}</span><strong>{finding.titleAr}</strong></div><StatusBadge tone={finding.severity === "high" ? "disputed" : "finding"}>{finding.severity}</StatusBadge></div><p>{finding.explanationAr}</p><div className="workspace-traceability"><FileSearch size={14} /><span>{finding.traceability === "evidence" ? "مرتبط بدليل" : "إشارات فقط — لا مصدر مباشر"}</span><small>{finding.claimIds.length} ادعاء</small></div>{canAttach ? <button className="text-button" type="button" onClick={() => onAttach(finding)} disabled={pending}><Link2 size={13} /> اربط بالسؤال</button> : null}</article>) : <EmptyWorkspace label="لا توجد ملاحظات ظاهرة لهذا السياق." />}</div></section>;
}

function QuestionsPanel({ snapshot, context, onCreate, canCreate, pending }: { snapshot: ResearchWorkspaceSnapshot; context: { entityType?: "person" | "family" | "branch"; entityId?: string; treeId?: string; treeVersionId?: string }; onCreate: () => void; canCreate: boolean; pending: boolean }) {
  return <section className="workspace-panel"><SectionHeading eyebrow="الأسئلة" title="مسار السؤال" description="اربط الفجوات بما جمع من الأدلة." /><div className="workspace-list">{snapshot.questions.map((question) => <Link className="workspace-list-row" to="/research/$questionId" params={{ questionId: question.id }} search={context} key={question.id}><MessageCircleQuestion size={15} /><span><strong>{question.titleAr}</strong><small>{question.status} · {question.claimCount} ادعاء</small></span><ArrowLeft size={13} /></Link>)}{canCreate ? <button className="secondary-button workspace-full-button" type="button" onClick={onCreate} disabled={pending}><Plus size={14} /> افتح سؤالاً جديداً</button> : null}</div></section>;
}

function IdentityPanel({ snapshot, onReview, canReview, pending }: { snapshot: ResearchWorkspaceSnapshot; onReview: (candidate: WorkspaceIdentityCandidate, decision: IdentityDecision) => void; canReview: boolean; pending: boolean }) {
  return <section className="workspace-panel"><SectionHeading eyebrow="الهوية" title="مرشحو المطابقة" description="المطابقة عملية قابلة للمراجعة." /><div className="workspace-list">{snapshot.identityCandidates.length ? snapshot.identityCandidates.map((candidate) => <div className="workspace-list-row" key={candidate.id}><UserRound size={15} /><span><strong>{candidate.leftNameAr} ↔ {candidate.rightNameAr}</strong><small>{candidate.matchClass} · {Math.round(candidate.score * 100)}%</small></span>{canReview ? <span className="workspace-inline-actions"><button className="icon-button" type="button" aria-label="اعتماد المرشح" onClick={() => onReview(candidate, "approve")} disabled={pending}><Check size={14} /></button><button className="icon-button" type="button" aria-label="رفض المرشح" onClick={() => onReview(candidate, "reject")} disabled={pending}><X size={14} /></button><button className="icon-button" type="button" aria-label="تأجيل المرشح" onClick={() => onReview(candidate, "defer")} disabled={pending}><RefreshCw size={14} /></button></span> : null}</div>) : <EmptyWorkspace label="لا يوجد مرشحون في هذا النطاق." />}</div></section>;
}

function NotesPanel({ snapshot }: { snapshot: ResearchWorkspaceSnapshot }) {
  return <section className="workspace-panel"><SectionHeading eyebrow="الذاكرة" title="ملاحظات السؤال" description="ملاحظات مربوطة بالسجل الحالي." /><div className="workspace-list">{snapshot.notes.length ? snapshot.notes.map((note) => <div className="workspace-note-row" key={note.id}><span>{note.noteAr}</span><small>{formatDate(note.createdAt)}</small></div>) : <EmptyWorkspace label="لا توجد ملاحظات بعد." />}</div></section>;
}

function HistoryPanel({ snapshot }: { snapshot: ResearchWorkspaceSnapshot }) {
  return <section className="workspace-panel"><SectionHeading eyebrow="السجل" title="مسار التحقيق" description="كل تشغيل يبقى قابلاً للعودة." /><div className="workspace-list">{snapshot.history.length ? snapshot.history.map((run) => <div className="workspace-history-row" key={run.id}><div><strong>{run.query}</strong><small>{formatDate(run.createdAt)} · {run.status}</small></div><div><StatusBadge tone={run.insufficientEvidence ? "question" : "source"}>{run.insufficientEvidence ? "أدلة غير كافية" : "مدعوم"}{run.route ? ` · ${run.route}` : ""}</StatusBadge><small>{run.citationCount} مادة</small></div></div>) : <EmptyWorkspace label="لم يُشغل تحقيق في هذا السؤال بعد." />}</div></section>;
}

function EvidenceLine({ evidence }: { evidence: WorkspaceClaim["evidence"][number] }) {
  return <div className="workspace-evidence-line"><span>{evidence.statementTextAr || evidence.passageTextAr || "دليل مرتبط"}</span><small>{evidence.sourceTitleAr || evidence.locatorAr || "مصدر غير معروض"}{evidence.reviewStatus ? ` · ${evidence.reviewStatus}` : ""}</small></div>;
}

function TimelineLine({ event }: { event: WorkspaceTimelineEvent }) {
  return <div className="workspace-timeline-row"><span className={`workspace-timeline-dot workspace-timeline-${event.kind}`} /><div><strong>{event.labelAr}</strong><small>{formatDate(event.dateFrom)}{event.dateTo && event.dateTo !== event.dateFrom ? ` — ${formatDate(event.dateTo)}` : ""} · {event.approximate ? "نطاق تقريبي" : event.status}</small></div></div>;
}

function EmptyWorkspace({ label }: { label: string }) {
  return <div className="workspace-empty"><CircleHelp size={16} /><span>{label}</span></div>;
}

function WorkspaceState({ title, description, error = false }: { title: string; description: string; error?: boolean }) {
  return <div className="workspace-state"><div className={error ? "workspace-state-icon workspace-state-error" : "workspace-state-icon"}>{error ? <AlertTriangle size={20} /> : <RefreshCw className="spin" size={20} />}</div><h2>{title}</h2><p>{description}</p></div>;
}

function demoWorkspace(questionId: string, entityType?: string, entityId?: string): ResearchWorkspaceSnapshot {
  const question = demoDashboard.openQuestions.find((item) => item.id === questionId) ?? demoDashboard.openQuestions[0];
  const personId = entityId ?? "person-2";
  const evidence = [{ id: "demo-evidence-1", kind: "evidence" as const, relation: "supports", sourceId: "s-1", sourceTitleAr: sources[0].title, statementId: "demo-statement-1", statementTextAr: sources[0].excerpt, locatorAr: sources[0].locator, reviewStatus: "accepted" }];
  return { context: { questionId, questionTitleAr: question.title, questionStatus: question.status, questionPriority: "high", entityType: entityType ?? "person", entityId: personId, entityNameAr: "عبدالله بن محمد", entityAliasesAr: ["أبو بكر", "عبد الله"] }, permissions: { canRunResearch: true, canCreateClaim: false, canDisputeClaim: false, canLinkEvidence: false, canCreateQuestion: false, canManageSelectedQuestion: false, canAttachFinding: false, canReviewFinding: false, canAddNote: false, canReviewIdentityCandidate: false, canMergeIdentity: false }, claims: [{ id: "demo-claim-1", subjectType: "person", subjectId: personId, predicate: "father_of", objectType: "person", objectId: "person-1", status: "supported", notesAr: "ادعاء تجريبي يحتاج إلى مراجعة.", createdAt: new Date().toISOString(), updatedAt: new Date().toISOString(), evidence }], sources: sources.slice(0, 3).map((source) => ({ id: source.id, titleAr: source.title, authorAr: "مؤلف تجريبي", sourceType: source.type, dependencyStatus: source.dependent ? "likely_dependent" : "unknown", visibility: "public", passageCount: 1, statementCount: 1 })), treeContexts: [{ treeId: demoTreeDetail.tree.id, treeNameAr: demoTreeDetail.tree.name, treeVersionId: demoTreeDetail.selectedVersion.id, versionNumber: demoTreeDetail.selectedVersion.number, state: demoTreeDetail.selectedVersion.state, nodes: demoTreeDetail.nodes.map((node) => ({ id: node.id, personId: node.personId, displayNameAr: node.displayName, sortOrder: node.sortOrder })), relationships: demoTreeDetail.relationships.map((relationship) => ({ id: relationship.id, subjectPersonId: demoTreeDetail.nodes.find((node) => node.id === relationship.subjectNodeId)?.personId ?? "", objectPersonId: demoTreeDetail.nodes.find((node) => node.id === relationship.objectNodeId)?.personId ?? "", predicate: relationship.predicate, status: relationship.status })) }], mapFeatures: [], timeline: [], questions: [{ id: question.id, titleAr: question.title, status: question.status, priority: "high", createdAt: new Date().toISOString(), updatedAt: new Date().toISOString(), claimCount: question.claimCount, sourceCount: 2 }], disputes: [{ id: "demo-dispute-1", titleAr: "تعارض روايات والد عبدالله", status: "under_review", createdAt: new Date().toISOString(), updatedAt: new Date().toISOString(), claimCount: 2 }], notes: [], findings: [{ id: "demo-finding-1", findingType: "possible_contradiction", titleAr: "تعارض في والد عبدالله", explanationAr: "تظهر روايتان مختلفتان، ويحتاجان إلى مراجعة المصادر.", status: "needs_review", severity: "high", signals: { relationship: "father_of" }, claimIds: ["demo-claim-1"], entityIds: [personId], evidence, traceability: "evidence", algorithmVersion: "demo-v1" }], identityCandidates: [], history: [] };
}

function isUuid(value: string | undefined): boolean {
  return Boolean(value && /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(value));
}

function normalizeEntityType(value: string | undefined): "person" | "family" | "branch" | undefined {
  return value === "person" || value === "family" || value === "branch" ? value : undefined;
}

function claimTone(value: string): EpistemicTone {
  if (value === "disputed" || value === "contested" || value === "contradicted") return "disputed";
  if (value === "supported") return "source";
  return "claim";
}

function formatDate(value: string | undefined): string {
  if (!value) return "بلا تاريخ";
  return value.slice(0, 10);
}

function actionError(value: unknown, fallback: string): string {
  if (value instanceof ApiError) return value.message;
  if (value instanceof Error) return value.message;
  return fallback;
}
