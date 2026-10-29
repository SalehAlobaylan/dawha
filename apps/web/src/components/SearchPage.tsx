import { useQuery } from "@tanstack/react-query";
import { Link, useNavigate, useSearch } from "@tanstack/react-router";
import { BookOpen, CircleHelp, FileSearch, Filter, GitBranch, Search, Sparkles, UserRound } from "lucide-react";
import { FormEvent, useState } from "react";
import { fetchSearch, fetchClaim } from "../lib/api";
import { StatusBadge } from "./StatusBadge";
import { TopBar } from "./TopBar";
import type { SearchResult } from "../types";

const kinds = [
  { value: "all", label: "كل النتائج" },
  { value: "names", label: "الأسماء" },
  { value: "sources", label: "المصادر" },
  { value: "claims", label: "الادعاءات" },
  { value: "passages", label: "المقاطع" },
  { value: "semantic", label: "تشابه دلالي" },
  { value: "questions", label: "الأسئلة" },
];

export function SearchPage({ embedded = false }: { embedded?: boolean }) {
  const params = useSearch({ from: "/search" });
  const navigate = useNavigate();
  const [input, setInput] = useState(params.q ?? "");
  const submitted = params.q ?? "";
  const kind = kinds.some((item) => item.value === params.kind) ? params.kind! : "all";
  const statuses = kind === "claims" ? [{ value: "unresolved", label: "غير محسوم" }, { value: "supported", label: "مدعوم" }, { value: "disputed", label: "متنازع عليه" }] : kind === "questions" ? [{ value: "open", label: "مفتوح" }, { value: "under_investigation", label: "قيد التحقيق" }, { value: "resolved", label: "محسوم مؤقتاً" }, { value: "reopened", label: "أُعيد فتحه" }, { value: "archived", label: "مؤرشف" }] : [];
  const status = statuses.some((item) => item.value === params.status) ? params.status! : "";
  const update = (next: typeof params) => { void navigate({ to: "/search", search: next, replace: true, resetScroll: false }); };
  const [personId, setPersonId] = useState("");
  const [placeId, setPlaceId] = useState("");
  const [sourceId, setSourceId] = useState("");
  const searchQuery = useQuery({ queryKey: ["search", submitted, kind, status, personId, placeId, sourceId], queryFn: ({ signal }) => fetchSearch({ q: submitted, kind, status: kind === "semantic" ? undefined : status || undefined, personId: personId || undefined, placeId: placeId || undefined, sourceId: sourceId || undefined, limit: 30, signal }), enabled: Boolean(submitted) });

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const value = input.trim();
    update({ q: value || undefined, kind: params.kind, status: params.status });
  };
  const reset = () => { setInput(""); setPersonId(""); setPlaceId(""); setSourceId(""); update({}); };

  return (
    <div className={embedded ? "discovery-search-content" : "page-stack"}>
      {!embedded ? <TopBar eyebrow="البحث / retrieval" title="ابحث في الذاكرة" description="البحث يجمع الأسماء والألقاب والمصادر والادعاءات والمقاطع، ويمكن اختيار التشابه الدلالي كإشارة ترتيب منفصلة." /> : null}
      <section className="search-hero"><div className="search-hero-icon"><Search size={21} /></div><div><div className="eyebrow">بحث عربي موحّد</div><h2>اكتب الكلمة، واترك السياق يظهر</h2><p>ابحث باسم أو لقب أو عبارة من مصدر، ثم افتح السجل لقراءة سياقه وأدلته.</p></div><Sparkles size={18} /></section>
      <form className="search-form" onSubmit={submit}><label className="search-main-input"><Search size={18} /><input aria-label="استعلام البحث" autoFocus value={input} onChange={(event) => setInput(event.target.value)} placeholder="مثال: عبدالله، أبو بكر، رواية، أو عبارة من مصدر..." /><kbd>Enter</kbd></label><button className="primary-button" type="submit"><Search size={15} /> ابحث</button></form>
      {submitted ? <section className="search-filter-bar"><div className="search-filter-label"><Filter size={14} /> تضييق النتائج</div><label>النوع<select value={kind} onChange={(event) => update({ ...params, kind: event.target.value, status: undefined })}>{kinds.map((item) => <option value={item.value} key={item.value}>{item.label}</option>)}</select></label>{statuses.length > 0 ? <label>الحالة<select value={status} onChange={(event) => update({ ...params, status: event.target.value || undefined })}><option value="">الكل</option>{statuses.map((item) => <option key={item.value} value={item.value}>{item.label}</option>)}</select></label> : null}<details className="search-advanced"><summary>معرّفات متقدمة (اختياري)</summary><label>شخص UUID<input value={personId} onChange={(event) => setPersonId(event.target.value)} placeholder="person_id" /></label><label>موضع UUID<input value={placeId} onChange={(event) => setPlaceId(event.target.value)} placeholder="place_id" /></label><label>مصدر UUID<input value={sourceId} onChange={(event) => setSourceId(event.target.value)} placeholder="source_id" /></label></details><button className="text-button" type="button" onClick={reset}>مسح</button></section> : null}
      {params.claimId ? <ClaimRecord claimId={params.claimId} /> : null}
      {submitted && kind === "semantic" ? <div className="search-semantic-note"><Sparkles size={15} /><span>يرتّب المقاطع حسب التشابه فقط. لا يثبت صحةَ المصدر ولا تاريخه ولا استقلاله.</span></div> : null}
      {submitted && searchQuery.isPending ? <div className="search-state">جارٍ ترتيب النتائج…</div> : searchQuery.error ? <div className="search-state search-error" role="alert">{searchError(searchQuery.error)} <button className="inline-link" type="button" onClick={() => void searchQuery.refetch()}>إعادة المحاولة</button></div> : submitted && searchQuery.data ? <SearchResults response={searchQuery.data} /> : <div className="search-empty"><FileSearch size={23} /><strong>ابدأ باستعلام</strong><span>جرّب اسماً عربياً، لقباً، أو كلمة من نص مصدر. يمكنك بعد ذلك تضييق النتائج بالحالة أو المعرف.</span></div>}
    </div>
  );
}

function SearchResults({ response }: { response: Awaited<ReturnType<typeof fetchSearch>> }) {
  return <section className="search-results"><div className="search-results-head"><div><div className="eyebrow">نتائج مرتبة</div><h2>{response.total} نتيجة عن «{response.query}»</h2></div><small>التطبيع: {response.normalizedQuery}{response.embeddingModel ? ` · النموذج: ${response.embeddingModel}` : ""}</small></div>{response.groups.length > 0 ? response.groups.map((group) => <section className="search-result-group" key={group.key}><div className="search-group-head"><div className="search-group-title"><span className={`search-group-icon search-group-icon-${group.key}`}>{groupIcon(group.key)}</span><h3>{group.label}</h3></div><span>{group.items.length}</span></div><div className="search-result-list">{group.items.map((item) => <SearchResultRow item={item} key={`${group.key}-${item.id}`} />)}</div></section>) : <div className="search-empty"><Search size={20} /><strong>لا توجد نتائج</strong><span>جرّب كلمة أقصر أو أزل بعض المرشحات.</span></div>}</section>;
}

function SearchResultRow({ item }: { item: SearchResult }) {
  const semantic = item.matchKind === "semantic";
  return <SearchResultLink item={item} className={`search-result-row${semantic ? " search-result-row-semantic" : ""}`}><span className={`search-result-icon search-result-icon-${item.kind}`}>{resultIcon(item.kind, semantic)}</span><span className="search-result-copy"><strong>{item.title}</strong>{item.subtitle ? <small>{item.subtitle}</small> : null}{item.body ? <p>{item.body}</p> : null}{semantic ? <small className="search-result-provenance">{semanticProvenance(item)}</small> : null}</span><span className="search-result-meta"><StatusBadge tone={semantic ? (item.reviewStatus === "accepted" ? "source" : "question") : item.status === "disputed" ? "disputed" : item.kind === "source" || item.kind === "passage" ? "source" : item.kind === "question" ? "question" : "claim"}>{semantic ? "تشابه دلالي" : kindLabel(item.kind)}</StatusBadge><b>{semantic ? `تشابه ${formatScore(item.score)}` : formatScore(item.score)}</b></span></SearchResultLink>;
}

function groupIcon(key: string) {
  if (key === "semantic") return <Sparkles size={15} />;
  if (key === "sources" || key === "passages") return <BookOpen size={15} />;
  if (key === "questions") return <CircleHelp size={15} />;
  if (key === "names") return <UserRound size={15} />;
  return <GitBranch size={15} />;
}

function resultIcon(kind: string, semantic = false) {
  if (semantic) return <Sparkles size={15} />;
  if (kind === "source" || kind === "passage") return <BookOpen size={15} />;
  if (kind === "question") return <CircleHelp size={15} />;
  if (kind === "person" || kind === "family" || kind === "tribe" || kind === "branch" || kind === "place") return <UserRound size={15} />;
  return <GitBranch size={15} />;
}

function kindLabel(kind: string): string {
  if (kind === "source") return "مصدر";
  if (kind === "passage") return "مقطع";
  if (kind === "claim") return "ادعاء";
  if (kind === "question") return "سؤال";
  if (kind === "person") return "شخص";
  if (kind === "place") return "موضع";
  return "اسم";
}

function semanticProvenance(item: SearchResult): string {
  const location = item.locatorAr || (item.pageNumber ? `صفحة ${item.pageNumber}` : "دون موضع");
  const review = reviewStatusLabel(item.reviewStatus || item.status);
  const dependency = item.dependencyStatus === "derived" ? "مصدر مشتق" : item.dependencyStatus === "likely_dependent" ? "ارتباط يحتاج مراجعة" : "";
  return [location, review, dependency, item.embeddingModel ? `نموذج ${item.embeddingModel}` : ""].filter(Boolean).join(" · ");
}

function reviewStatusLabel(value?: string): string {
  if (value === "accepted") return "عبارة مقبولة";
  if (value === "unreviewed") return "دون مراجعة";
  if (value === "needs_review") return "تحتاج مراجعة";
  if (value === "rejected") return "مرفوضة";
  return "دون حالة";
}

function formatScore(score: number): string {
  const value = score <= 1 ? score * 100 : score;
  return `${Math.round(Math.max(0, Math.min(100, value)))}%`;
}

function searchError(error: unknown): string {
  return error instanceof Error ? error.message : "تعذر تنفيذ البحث.";
}


function SearchResultLink({ item, className, children }: { item: SearchResult; className: string; children: React.ReactNode }) {
  if (item.kind === "source" || item.kind === "passage" || item.kind === "statement") return <Link className={className} to="/sources" search={{ sourceId: item.sourceId || (item.kind === "source" ? item.id : undefined) }}>{children}</Link>;
  if (item.kind === "question") return <Link className={className} to="/research/$questionId" params={{ questionId: item.id }}>{children}</Link>;
  if (item.kind === "claim") return <Link className={className} to="/search" search={{ q: item.title, kind: "claims", claimId: item.id }}>{children}</Link>;
  const kind = ({ person: "people", family: "families", tribe: "tribes", place: "places" } as Record<string, string>)[item.kind];
  if (kind) return <Link className={className} to="/search" search={{ view: "index", kind, id: item.id }}>{children}</Link>;
  return <Link className={className} to="/research" search={{ view: "evidence", q: item.title, entityType: item.kind === "branch" ? "branch" : undefined, entityId: item.kind === "branch" ? item.id : undefined }}>{children}</Link>;
}

function ClaimRecord({ claimId }: { claimId: string }) {
  const query = useQuery({ queryKey: ["claim", claimId], queryFn: () => fetchClaim(claimId) });
  if (query.isPending) return <p role="status">جارٍ فتح الادعاء…</p>;
  if (query.error || !query.data) return <p role="alert">تعذر فتح الادعاء. <button className="inline-link" type="button" onClick={() => void query.refetch()}>إعادة المحاولة</button></p>;
  const claim = query.data;
  return <section className="search-claim-record"><h2>{claim.notesAr || claim.predicate}</h2><p>{claim.predicate} · {claim.status}</p><Link className="inline-link" to="/research" search={{ view: "evidence", entityType: claim.subjectType === "family" ? "family" : "person", entityId: claim.subjectId }}>افتح سياق الادعاء في البحث</Link><h3>أدلة الادعاء</h3>{claim.evidence.length ? claim.evidence.map((evidence) => <p key={evidence.id}>{evidence.evidenceNoteAr || evidence.relation} {evidence.sourceId ? <Link className="inline-link" to="/sources" search={{ sourceId: evidence.sourceId }}>فتح المصدر</Link> : null}</p>) : <p>لا يوجد دليل مرتبط بهذا الادعاء.</p>}</section>;
}
