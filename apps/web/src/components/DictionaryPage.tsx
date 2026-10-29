import { useQuery } from "@tanstack/react-query";
import { Link, useNavigate, useSearch } from "@tanstack/react-router";
import { BookOpen, ChevronLeft, CircleHelp, GitBranch, MapPinned, Search, Users } from "lucide-react";
import { useState } from "react";
import { fetchDictionaryDetail, fetchDictionaryIndex } from "../lib/api";
import { StatusBadge } from "./StatusBadge";
import { TopBar } from "./TopBar";
import type { DictionaryDetail, DictionaryIndexItem, DictionaryKind, DictionaryReference } from "../types";

const kinds: Array<{ value: DictionaryKind; label: string; icon: typeof Users }> = [
  { value: "families", label: "العائلات", icon: Users },
  { value: "tribes", label: "القبائل", icon: Users },
  { value: "branches", label: "الفروع", icon: GitBranch },
  { value: "people", label: "الأشخاص", icon: Users },
  { value: "places", label: "المواضع", icon: MapPinned },
  { value: "sources", label: "المصادر", icon: BookOpen },
  { value: "questions", label: "الأسئلة", icon: CircleHelp },
  { value: "disputed-claims", label: "ادعاءات متنازع عليها", icon: GitBranch },
];

export function DictionaryPage({ embedded = false }: { embedded?: boolean }) {
  const search = useSearch({ from: "/search" });
  const navigate = useNavigate();
  const kind = kinds.some((item) => item.value === search.kind) ? search.kind as DictionaryKind : "people";
  const query = search.q ?? "";
  const [input, setInput] = useState(query);
  const update = (next: typeof search) => { void navigate({ to: "/search", search: next, replace: true, resetScroll: false }); };
  const indexQuery = useQuery({ queryKey: ["dictionary", kind, query], queryFn: () => fetchDictionaryIndex(kind, query) });
  const detailKind = isDetailKind(kind) ? kind : null;
  const selected = indexQuery.data?.items.find((item) => item.id === search.id);
  const detailQuery = useQuery({ queryKey: ["dictionary-detail", detailKind, search.id], queryFn: () => fetchDictionaryDetail(detailKind!, search.id!), enabled: Boolean(detailKind && search.id) });

  const items = indexQuery.data?.items ?? [];
  const activeKind = kinds.find((item) => item.value === kind) ?? kinds[0];

  return (
    <div className={embedded ? "discovery-index-content" : "page-stack"}>
      {!embedded ? <TopBar eyebrow="قاموس دَوْحة / فهرس عام" title="ابحث في الذاكرة، لا في نسخة مفردة" description="الأسماء والمواضع والمصادر والأسئلة مرتبطة بنفس الرسم البياني، وكل فهرس يفتحك إلى سياقه." /> : null}
      <section className="dictionary-toolbar"><div className="dictionary-tabs" role="tablist" aria-label="أنواع الفهرس">{kinds.map(({ value, label, icon: Icon }) => <button type="button" role="tab" aria-selected={kind === value} className={`dictionary-tab${kind === value ? " dictionary-tab-active" : ""}`} key={value} onClick={() => update({ view: "index", kind: value, q: search.q })}><Icon size={14} /> {label}</button>)}</div><form className="dictionary-search" onSubmit={(event) => { event.preventDefault(); update({ view: "index", kind, q: input.trim() || undefined }); }}><Search size={15} /><input aria-label="البحث في الفهرس" value={input} onChange={(event) => setInput(event.target.value)} placeholder="ابحث بالاسم أو اللقب..." /><button className="text-button" type="submit">ابحث</button></form></section>
      <section className="dictionary-workspace"><aside className="dictionary-index-panel"><div className="dictionary-index-heading"><div><div className="eyebrow">فهرس {activeKind.label}</div><h2>{items.length} نتيجة</h2></div><StatusBadge tone="interpretation">عام</StatusBadge></div>{indexQuery.isPending ? <p className="dictionary-empty">جارٍ البحث في الرسم البياني…</p> : indexQuery.error ? <div className="dictionary-error" role="alert">{dictionaryError(indexQuery.error)} <button className="inline-link" type="button" onClick={() => void indexQuery.refetch()}>إعادة المحاولة</button></div> : items.length > 0 ? <div className="dictionary-index-list">{items.map((item) => <button type="button" className={`dictionary-index-item${search.id === item.id ? " dictionary-index-item-active" : ""}`} key={`${kind}-${item.id}`} onClick={() => update({ ...search, id: item.id })}><span className="dictionary-index-item-icon">{kindIcon(kind)}</span><span className="dictionary-index-item-copy"><strong>{item.nameAr}</strong><small>{item.secondaryAr || item.status || dictionaryCountLabel(kind, item.count)}</small></span><span className="dictionary-index-item-count">{item.count}</span><ChevronLeft size={14} /></button>)}</div> : <div className="dictionary-empty"><Search size={18} /><strong>لا توجد نتيجة</strong><span>جرّب اسمًا آخر أو ابحث عن لقب.</span></div>}</aside><article className="dictionary-detail-panel">{detailQuery.data ? <DictionaryDetailView detail={detailQuery.data} /> : search.id && detailKind ? detailQuery.isPending ? <div className="dictionary-detail-empty">جارٍ فتح صفحة القاموس…</div> : detailQuery.error ? <div className="dictionary-error" role="alert">{dictionaryError(detailQuery.error)} <button className="inline-link" type="button" onClick={() => void detailQuery.refetch()}>إعادة المحاولة</button></div> : <div className="dictionary-detail-empty"><BookOpen size={20} /><strong>اختر اسماً من الفهرس</strong><span>ستظهر هنا الألقاب والعلاقات المنشورة والمصادر والأسئلة المرتبطة.</span></div> : selected ? <SelectedSummary item={selected} kind={kind} /> : <div className="dictionary-detail-empty"><BookOpen size={20} /><strong>ابدأ من فهرس عام</strong><span>اختر نوعاً ثم ابحث عن اسم، أو افتح عنصراً لعرض علاقاته.</span></div>}</article></section>
      <section className="dictionary-method-note"><GitBranch size={18} /><div><strong>الفهرس لا يخزّن نسخة ثانية</strong><span>كل نتيجة محسوبة من الجداول نفسها: الأشخاص، المواضع، المصادر، الادعاءات، الأشجار المنشورة، والأسئلة المفتوحة.</span></div><Link to="/research">افتح البحث <ChevronLeft size={14} /></Link></section>
    </div>
  );
}

function DictionaryDetailView({ detail }: { detail: DictionaryDetail }) {
  return <div className="dictionary-detail-content"><div className="dictionary-detail-head"><div><div className="eyebrow">صفحة قاموس</div><h2>{detail.nameAr}</h2><p>{detail.descriptionAr || "لا يوجد وصف إضافي."}</p></div><StatusBadge tone={detail.kind === "person" ? "claim" : detail.kind === "place" ? "source" : "interpretation"}>{dictionaryKindLabel(detail.kind)}</StatusBadge></div><div className="dictionary-tag-row">{detail.aliases.map((alias) => <span className="dictionary-tag" key={`${alias.valueAr}-${alias.type}`}>{alias.valueAr}<small>{alias.type}</small></span>)}{detail.historicalNames.map((name) => <span className="dictionary-tag dictionary-tag-muted" key={name.id}>{name.nameAr}<small>اسم تاريخي</small></span>)}</div><div className="dictionary-detail-grid"><ReferenceSection kind="branches" title="الفروع" items={detail.branches} empty="لا توجد فروع مرتبطة." /><ReferenceSection kind="places" title="المواضع" items={detail.places} empty="لا توجد مواضع مرتبطة." /><ReferenceSection kind="people" title="الأشخاص" items={detail.people} empty="لا يوجد أشخاص مرتبطون." /><ReferenceSection kind="families" title="العائلات" items={detail.families} empty="لا توجد ارتباطات عائلية." /><ReferenceSection kind="tribes" title="القبائل" items={detail.tribes} empty="لا توجد ارتباطات قبلية." />{detail.kind === "person" ? <section className="dictionary-reference-section"><div className="detail-label">الأشجار المنشورة</div>{detail.publishedTrees.length ? detail.publishedTrees.map((tree) => <Link key={tree.id} className="dictionary-reference-row" to="/tree/$treeId/versions/$versionId" params={{ treeId: tree.id, versionId: tree.versionId }} search={{ from: "explore" }}>{tree.nameAr} · النسخة {tree.versionNumber}</Link>) : <p className="dictionary-related-empty">لا تظهر هذه الشخصية في نسخة منشورة بعد.</p>}</section> : null}<ReferenceSection kind="sources" title="المصادر" items={detail.sources} empty="لا توجد مصادر مرتبطة." /><ReferenceSection title="الهجرات" items={detail.migrations} empty="لا توجد هجرات مرتبطة." /></div><div className="dictionary-related-columns"><div><div className="detail-label">ادعاءات ذات صلة</div>{detail.claims.length > 0 ? detail.claims.map((claim) => <div className="dictionary-related-row" key={claim.id}><span>{claim.predicate}</span><strong>{claim.status}</strong><small>{claim.evidenceCount} إشارات</small></div>) : <p className="dictionary-related-empty">لا توجد ادعاءات مرتبطة.</p>}</div><div><div className="detail-label">أسئلة مفتوحة</div>{detail.questions.length > 0 ? detail.questions.map((question) => <div className="dictionary-related-row" key={question.id}><Link to="/research/$questionId" params={{ questionId: question.id }}>{question.titleAr}</Link><strong>{question.status}</strong><small>{question.noteCount} ملاحظات</small></div>) : <p className="dictionary-related-empty">لا توجد أسئلة مرتبطة.</p>}</div></div></div>;
}

function ReferenceSection({ title, items, empty, kind }: { title: string; items: DictionaryReference[]; empty: string; kind?: DictionaryKind }) {
  return <section className="dictionary-reference-section"><div className="detail-label">{title}</div>{items.length > 0 ? items.map((item) => <div className="dictionary-reference-row" key={item.id}><span>{kind ? <ReferenceLink item={item} kind={kind} /> : item.nameAr}</span><small>{item.detailAr || item.status || "مرتبط"}</small></div>) : <p className="dictionary-related-empty">{empty}</p>}</section>;
}

function SelectedSummary({ item, kind }: { item: DictionaryIndexItem; kind: DictionaryKind }) {
  if (kind === "sources") return <div className="dictionary-detail-empty"><BookOpen size={20} /><strong>{item.nameAr}</strong><span>افتح مكتبة المصادر لقراءة المقاطع والعبارات المرتبطة.</span><Link to="/sources" search={{ sourceId: item.id }} className="inline-link">افتح المصدر <ChevronLeft size={14} /></Link></div>;
  if (kind === "questions") return <div className="dictionary-detail-empty"><CircleHelp size={20} /><strong>{item.nameAr}</strong><span>افتح مساحة الأسئلة لمتابعة الحالة والملاحظات والروابط.</span><Link to="/research/$questionId" params={{ questionId: item.id }} className="inline-link">افتح السؤال <ChevronLeft size={14} /></Link></div>;
  return <div className="dictionary-detail-empty"><GitBranch size={20} /><strong>{item.nameAr}</strong><span>هذا الفهرس يقودك إلى البحث حيث تظهر العلاقات والأدلة.</span><Link to="/research" search={{ view: "evidence", q: item.nameAr, entityType: kind === "branches" ? "branch" : undefined, entityId: kind === "branches" ? item.id : undefined }} className="inline-link">افتح البحث <ChevronLeft size={14} /></Link></div>;
}

function isDetailKind(kind: DictionaryKind): kind is "families" | "tribes" | "people" | "places" {
  return kind === "families" || kind === "tribes" || kind === "people" || kind === "places";
}

function kindIcon(kind: DictionaryKind) {
  return kind === "places" ? <MapPinned size={15} /> : kind === "sources" ? <BookOpen size={15} /> : kind === "questions" ? <CircleHelp size={15} /> : <Users size={15} />;
}

function dictionaryKindLabel(kind: DictionaryDetail["kind"]): string {
  if (kind === "person") return "شخص";
  if (kind === "place") return "موضع";
  if (kind === "family") return "عائلة";
  return "قبيلة";
}

function dictionaryCountLabel(kind: DictionaryKind, count: number): string {
  if (kind === "people") return `${count} ألقاب`;
  if (kind === "places") return `${count} أسماء تاريخية`;
  if (kind === "sources") return `${count} عبارات`;
  if (kind === "questions") return `${count} ملاحظات`;
  return `${count} ارتباطات`;
}

function dictionaryError(error: unknown): string {
  return error instanceof Error ? error.message : "تعذر فتح الفهرس.";
}


function ReferenceLink({ item, kind }: { item: DictionaryReference; kind: DictionaryKind }) {
  if (kind === "sources") return <Link to="/sources" search={{ sourceId: item.id }}>{item.nameAr}</Link>;
  if (kind === "branches") return <Link to="/research" search={{ view: "evidence", entityType: "branch", entityId: item.id, q: item.nameAr }}>{item.nameAr}</Link>;
  return <Link to="/search" search={{ view: "index", kind, id: item.id }}>{item.nameAr}</Link>;
}
