import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { BookOpen, CircleHelp, FileSearch, Filter, GitBranch, Search, Sparkles, UserRound } from "lucide-react";
import { FormEvent, useState } from "react";
import { fetchSearch } from "../lib/api";
import { StatusBadge } from "./StatusBadge";
import { TopBar } from "./TopBar";
import type { SearchResult } from "../types";

const kinds = [
  { value: "all", label: "كل النتائج" },
  { value: "names", label: "الأسماء" },
  { value: "sources", label: "المصادر" },
  { value: "claims", label: "الادعاءات" },
  { value: "passages", label: "المقاطع" },
  { value: "questions", label: "الأسئلة" },
];

export function SearchPage() {
  const [input, setInput] = useState("");
  const [submitted, setSubmitted] = useState("");
  const [kind, setKind] = useState("all");
  const [status, setStatus] = useState("");
  const [personId, setPersonId] = useState("");
  const [placeId, setPlaceId] = useState("");
  const [sourceId, setSourceId] = useState("");
  const searchQuery = useQuery({ queryKey: ["search", submitted, kind, status, personId, placeId, sourceId], queryFn: () => fetchSearch({ q: submitted, kind, status: status || undefined, personId: personId || undefined, placeId: placeId || undefined, sourceId: sourceId || undefined, limit: 30 }), enabled: Boolean(submitted) });

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const value = input.trim();
    if (value) setSubmitted(value);
  };
  const reset = () => {
    setInput("");
    setSubmitted("");
    setKind("all");
    setStatus("");
    setPersonId("");
    setPlaceId("");
    setSourceId("");
  };

  return (
    <div className="page-stack">
      <TopBar eyebrow="البحث / retrieval" title="ابحث في الذاكرة" description="البحث يجمع الأسماء والألقاب والمصادر والادعاءات والمقاطع، ثم يعرضها كطبقات قابلة للمراجعة." />
      <section className="search-hero"><div className="search-hero-icon"><Search size={21} /></div><div><div className="eyebrow">بحث عربي موحّد</div><h2>اكتب الكلمة، واترك السياق يظهر</h2><p>نستخدم التطبيع العربي، الألقاب، والتشابه النصي داخل PostgreSQL — دون تحويل النص الأصلي إلى حقيقة.</p></div><Sparkles size={18} /></section>
      <form className="search-form" onSubmit={submit}><label className="search-main-input"><Search size={18} /><input autoFocus value={input} onChange={(event) => setInput(event.target.value)} placeholder="مثال: عبدالله، أبو بكر، رواية، أو عبارة من مصدر..." /><kbd>Enter</kbd></label><button className="primary-button" type="submit"><Search size={15} /> ابحث</button></form>
      {submitted ? <section className="search-filter-bar"><div className="search-filter-label"><Filter size={14} /> تضييق النتائج</div><label>النوع<select value={kind} onChange={(event) => setKind(event.target.value)}>{kinds.map((item) => <option value={item.value} key={item.value}>{item.label}</option>)}</select></label><label>الحالة<select value={status} onChange={(event) => setStatus(event.target.value)}><option value="">الكل</option><option value="disputed">متنازع عليه</option><option value="supported">مدعوم</option><option value="under_investigation">قيد التحقيق</option><option value="open">مفتوح</option></select></label><details className="search-advanced"><summary>مرشحات معرفية</summary><label>شخص UUID<input value={personId} onChange={(event) => setPersonId(event.target.value)} placeholder="person_id" /></label><label>موضع UUID<input value={placeId} onChange={(event) => setPlaceId(event.target.value)} placeholder="place_id" /></label><label>مصدر UUID<input value={sourceId} onChange={(event) => setSourceId(event.target.value)} placeholder="source_id" /></label></details><button className="text-button" type="button" onClick={reset}>مسح</button></section> : null}
      {submitted && searchQuery.isPending ? <div className="search-state">جارٍ ترتيب النتائج…</div> : searchQuery.error ? <div className="search-state search-error" role="alert">{searchError(searchQuery.error)}</div> : submitted && searchQuery.data ? <SearchResults response={searchQuery.data} /> : <div className="search-empty"><FileSearch size={23} /><strong>ابدأ باستعلام</strong><span>جرّب اسماً عربياً، لقباً، أو كلمة من نص مصدر. يمكنك بعد ذلك تضييق النتائج بالحالة أو المعرف.</span></div>}
    </div>
  );
}

function SearchResults({ response }: { response: Awaited<ReturnType<typeof fetchSearch>> }) {
  return <section className="search-results"><div className="search-results-head"><div><div className="eyebrow">نتائج مرتبة</div><h2>{response.total} نتيجة عن «{response.query}»</h2></div><small>التطبيع: {response.normalizedQuery}</small></div>{response.groups.length > 0 ? response.groups.map((group) => <section className="search-result-group" key={group.key}><div className="search-group-head"><div className="search-group-title"><span className={`search-group-icon search-group-icon-${group.key}`}>{groupIcon(group.key)}</span><h3>{group.label}</h3></div><span>{group.items.length}</span></div><div className="search-result-list">{group.items.map((item) => <SearchResultRow item={item} key={`${group.key}-${item.id}`} />)}</div></section>) : <div className="search-empty"><Search size={20} /><strong>لا توجد نتائج</strong><span>جرّب كلمة أقصر أو أزل بعض المرشحات.</span></div>}</section>;
}

function SearchResultRow({ item }: { item: SearchResult }) {
  const route = item.route === "/sources" || item.route === "/questions" || item.route === "/dictionary" ? item.route : "/research";
  return <Link to={route} className="search-result-row"><span className={`search-result-icon search-result-icon-${item.kind}`}>{resultIcon(item.kind)}</span><span className="search-result-copy"><strong>{item.title}</strong>{item.subtitle ? <small>{item.subtitle}</small> : null}{item.body ? <p>{item.body}</p> : null}</span><span className="search-result-meta"><StatusBadge tone={item.status === "disputed" ? "disputed" : item.kind === "source" || item.kind === "passage" ? "source" : item.kind === "question" ? "question" : "claim"}>{kindLabel(item.kind)}</StatusBadge><b>{formatScore(item.score)}</b></span></Link>;
}

function groupIcon(key: string) {
  if (key === "sources" || key === "passages") return <BookOpen size={15} />;
  if (key === "questions") return <CircleHelp size={15} />;
  if (key === "names") return <UserRound size={15} />;
  return <GitBranch size={15} />;
}

function resultIcon(kind: string) {
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

function formatScore(score: number): string {
  const value = score <= 1 ? score * 100 : score;
  return `${Math.round(value)}%`;
}

function searchError(error: unknown): string {
  return error instanceof Error ? error.message : "تعذر تنفيذ البحث.";
}
