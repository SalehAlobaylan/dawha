import { AtlasSourceSearch } from "./AtlasSourceSearch";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate, useSearch } from "@tanstack/react-router";
import { BookOpen, ChevronLeft, CircleHelp, GitBranch, MapPinned, Search, ShieldAlert, Users } from "lucide-react";
import { useState } from "react";
import { ApiError, challengeClaim, decideTribeDuplicate, decideTribeHint, fetchDictionaryDetail, fetchDictionaryIndex, fetchResearchAccess, fetchTribeDuplicateCandidates, fetchTribeEvidence, fetchTribeHints } from "../lib/api";
import { StatusBadge } from "./StatusBadge";
import { TopBar } from "./TopBar";
import type { DictionaryClaimReference, DictionaryDetail, DictionaryIndexItem, DictionaryKind, DictionaryReference } from "../types";

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
  const access = useQuery({ queryKey: ["research-access"], queryFn: fetchResearchAccess, staleTime: 60_000 });
  return <div className="dictionary-detail-content"><div className="dictionary-detail-head"><div><div className="eyebrow">صفحة قاموس</div><h2>{detail.nameAr}</h2><p>{detail.descriptionAr || "لا يوجد وصف إضافي."}</p></div><StatusBadge tone={detail.kind === "person" ? "claim" : detail.kind === "place" ? "source" : "interpretation"}>{dictionaryKindLabel(detail.kind)}</StatusBadge></div><div className="dictionary-tag-row">{detail.aliases.map((alias) => <span className="dictionary-tag" key={`${alias.valueAr}-${alias.type}`}>{alias.valueAr}<small>{alias.type}</small></span>)}{detail.historicalNames.map((name) => <span className="dictionary-tag dictionary-tag-muted" key={name.id}>{name.nameAr}<small>اسم تاريخي</small></span>)}</div><div className="dictionary-detail-grid"><ReferenceSection kind="branches" title="الفروع" items={detail.branches} empty="لا توجد فروع مرتبطة." /><ReferenceSection kind="places" title="المواضع" items={detail.places} empty="لا توجد مواضع مرتبطة." /><ReferenceSection kind="people" title="الأشخاص" items={detail.people} empty="لا يوجد أشخاص مرتبطون." /><ReferenceSection kind="families" title="العائلات" items={detail.families} empty="لا توجد ارتباطات عائلية." /><ReferenceSection kind="tribes" title="القبائل" items={detail.tribes} empty="لا توجد ارتباطات قبلية." />{detail.kind === "person" ? <section className="dictionary-reference-section"><div className="detail-label">الأشجار المنشورة</div>{detail.publishedTrees.length ? detail.publishedTrees.map((tree) => <Link key={tree.id} className="dictionary-reference-row" to="/tree/$treeId/versions/$versionId" params={{ treeId: tree.id, versionId: tree.versionId }} search={{ from: "explore" }}>{tree.nameAr} · النسخة {tree.versionNumber}</Link>) : <p className="dictionary-related-empty">لا تظهر هذه الشخصية في نسخة منشورة بعد.</p>}</section> : null}<ReferenceSection kind="sources" title="المصادر" items={detail.sources} empty="لا توجد مصادر مرتبطة." /><ReferenceSection title="الهجرات" items={detail.migrations} empty="لا توجد هجرات مرتبطة." /></div>{detail.kind === "tribe" ? <><AtlasSourceSearch key={detail.id} tribe={detail.id} /><TribeReceipts tribeID={detail.id} /><TribeHintQueue tribeID={detail.id} canMerge={access.data?.canMerge ?? false} /><TribeDuplicates tribeID={detail.id} tribeName={detail.nameAr} canMerge={access.data?.canMerge ?? false} /></> : null}<div className="dictionary-related-columns"><div><div className="detail-label">ادعاءات ذات صلة</div>{detail.claims.length > 0 ? <>{detail.claims.map((claim) => <div className="dictionary-related-row" key={claim.id}><span>{claimBadgeLabel(claim)}</span><strong>{receiptStatusChip(claim.status)}</strong><small>{claim.claimNotes || claim.predicate} <em>{claim.predicate}</em></small><ChallengeButton claim={claim} /></div>)}<p className="dictionary-badge-caption">{detail.badgeCaption}</p></> : <><p className="dictionary-related-empty">لا توجد ادعاءات مرتبطة.</p><p className="dictionary-badge-caption">{detail.badgeCaption}</p></>}</div><div><div className="detail-label">أسئلة مفتوحة</div>{detail.questions.length > 0 ? detail.questions.map((question) => <div className="dictionary-related-row" key={question.id}><Link to="/research/$questionId" params={{ questionId: question.id }}>{question.titleAr}</Link><strong>{question.status}</strong><small>{question.noteCount} ملاحظات</small></div>) : <p className="dictionary-related-empty">لا توجد أسئلة مرتبطة.</p>}</div></div></div>;
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

/**
 * The tribe dossier's receipt folio (plan 031-A): the named reviewer's
 * acceptances behind this tribe, each with its own status chip and its
 * source. A moderator may open the needs-review band; the band is a request —
 * the server re-checks the role and refuses anybody else. An empty folio says
 * so: undocumented absence is stated, never blank.
 */
function TribeReceipts({ tribeID }: { tribeID: string }) {
  const access = useQuery({ queryKey: ["research-access"], queryFn: fetchResearchAccess, staleTime: 60_000 });
  const [laneOpen, setLaneOpen] = useState(false);
  const canMerge = access.data?.canMerge ?? false;
  const folio = useQuery({
    queryKey: ["tribe-evidence", tribeID, laneOpen && canMerge],
    queryFn: ({ signal }) => fetchTribeEvidence(tribeID, { includeNeedsReview: laneOpen && canMerge, signal }),
  });

  if (folio.isPending) {
    return <section className="dictionary-receipts"><div className="detail-label">سجل الشواهد</div><p className="dictionary-related-empty">يُقرأ سجل الشواهد…</p></section>;
  }
  if (folio.isError) {
    return (
      <section className="dictionary-receipts">
        <div className="detail-label">سجل الشواهد</div>
        <div className="dictionary-error" role="alert">{dictionaryError(folio.error)} <button className="inline-link" type="button" onClick={() => void folio.refetch()}>إعادة المحاولة</button></div>
      </section>
    );
  }
  const report = folio.data;
  return (
    <section className="dictionary-receipts" aria-label="سجل الشواهد">
      <div className="detail-label">
        سجل الشواهد
        <span className="dictionary-receipt-caption">
          شواهد مراجعة القبيلة حسب المصادر التي يفتحها قارئك، محدودة عند {report.limit}{report.truncated ? " والمقطوعة" : ""}.
        </span>
      </div>
      {canMerge ? (
        <button type="button" className="text-button" onClick={() => setLaneOpen((open) => !open)}>
          {laneOpen ? "إخفاء بانتظار المراجعة" : "إظهار ما بانتظار المراجعة"}
        </button>
      ) : null}
      {report.evidence.length === 0 ? (
        <p className="dictionary-related-empty">لا شواهد موثقة بعد.</p>
      ) : (
        <ul className="tribe-receipts">
          {report.evidence.map((receipt) => (
            <li key={receipt.id} className={`tribe-receipt tribe-receipt-${receipt.kind}`}>
              <div className="tribe-receipt-copy">
                {receipt.passageText ? <blockquote>{receipt.passageText}</blockquote> : null}
                <small>
                  {receipt.sourceTitle ? `«${receipt.sourceTitle}»` : "سجل منصتي"}
                  {receipt.locator ? ` · ${receipt.locator}` : ""}
                  {receipt.pageNumber ? ` · ص ${receipt.pageNumber}` : ""}
                </small>
              </div>
              <div className="tribe-receipt-chips">
                <StatusBadge tone={receipt.kind === "spatial_evidence" ? "source" : "claim"}>{receiptStatusChip(receipt.status)}</StatusBadge>
                {receipt.relation ? <small>{receiptRelationLabel(receipt.relation)}</small> : null}
              </div>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

function receiptStatusChip(status: string): string {
  const chips: Record<string, string> = {
    accepted: "موثَّق",
    documented: "موثَّق",
    supported: "مدعوم",
    needs_review: "بانتظار المراجعة",
    unresolved: "مطروح",
    disputed: "متنازع عليه",
    rejected: "مرفوض",
    superseded: "عُودِل بإصدار أحدث",
    platform_generated: "مولَّد منصتياً",
    contradicted: "مُناقَض",
    contested: "متحدّى",
    inferred: "استنتاجي",
    unknown: "مجهول الحال",
  };
  return chips[status] ?? status;
}

function receiptRelationLabel(relation: string): string {
  const labels: Record<string, string> = {
    supports: "يدعم",
    contradicts: "يناقض",
    refutes: "ينقض",
    contextualizes: "سياق",
    documented_in: "موثَّق في",
    originated_in: "أصله من",
    lived_in: "سكنه",
    associated_with: "صلة به",
    mentioned_at: "مذكور عنده",
  };
  return labels[relation] ?? relation;
}

/**
 * The badge trio's label (plan 031-B): ✓ يدعم / ✗ يناقض / ○ بلا دليل — the
 * numbers are the server's per-request, per-reader, sources-distinct tallies;
 * the page counts nothing. The caption travels in the response beside them.
 */
function claimBadgeLabel(claim: DictionaryClaimReference): string {
  const parts: string[] = [];
  if (claim.supportingSources > 0) parts.push(`إيداع: ${claim.supportingSources}`);
  if (claim.contradictingSources > 0) parts.push(`نفي: ${claim.contradictingSources}`);
  if (claim.mentionOnlySources > 0) parts.push(`إشارة: ${claim.mentionOnlySources}`);
  if (parts.length === 0) return "بلا شواهد بعد";
  return parts.join(" · ");
}

/**
 * The طعن button on one dossier claim (plan 031-D): a signed-in reader
 * disputes the claim with their own words; the claim's own row flips to
 * `disputed` in the challenge's transaction, so every surface's chip changes
 * with it. The button asks for the thread's title and the grounds; it asserts
 * nothing and decides nothing — the adjudication is the moderator's close.
 */
function ChallengeButton({ claim }: { claim: DictionaryClaimReference }) {
  const [open, setOpen] = useState(false);
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [message, setMessage] = useState("");
  const queryClient = useQueryClient();
  const challenge = useMutation({
    mutationFn: () => challengeClaim(claim.id, { titleAr: title.trim(), descriptionAr: description.trim() || undefined }),
    onSuccess: () => {
      setOpen(false);
      setTitle("");
      setDescription("");
      setMessage("فُتح خلاف الطعن، وحال الادعاء أصبح متنازعاً عليه في كل السطوح.");
      void queryClient.invalidateQueries({ queryKey: ["dictionary-detail"] });
      void queryClient.invalidateQueries({ queryKey: ["tribe-evidence"] });
    },
    onError: (error) => setMessage(error instanceof ApiError && error.status === 403 ? "الطعن بجلسة موثقة؛ سجّل الدخول أولاً." : dictionaryErrorMessage(error)),
  });

  if (claim.status === "disputed" || claim.status === "rejected" || claim.status === "superseded") {
    return null;
  }
  return (
    <span className="challenge-control">
      <button type="button" className="text-button" aria-label={`طعن في الادعاء ${claim.predicate}`} onClick={() => setOpen((value) => !value)} disabled={challenge.isPending}>
        <ShieldAlert size={12} /> طعن
      </button>
      {open ? (
        <form
          className="challenge-form"
          onSubmit={(event) => { event.preventDefault(); if (title.trim()) challenge.mutate(); }}
        >
          <input value={title} onChange={(event) => setTitle(event.target.value)} placeholder="عنوان الخلاف" aria-label="عنوان الخلاف" required />
          <input value={description} onChange={(event) => setDescription(event.target.value)} placeholder="لماذا تخالف؟" aria-label="سبب الطعن" />
          <button type="submit" disabled={challenge.isPending || !title.trim()}>{challenge.isPending ? "جارٍ الفتح…" : "افتح الخلاف"}</button>
        </form>
      ) : null}
      {message ? <small className="challenge-message">{message}</small> : null}
    </span>
  );
}

function dictionaryErrorMessage(error: unknown): string {
  if (error instanceof ApiError) return error.message;
  return "تعذر الحفظ؛ أعد المحاولة.";
}

/**
 * The record-hints queue, already scoped to this tribe (plan 031-C): the
 * ingest lanes' paragraph mentions as suggestion cards, read by a moderator,
 * with the receipts' own words beside the count. NOTHING here applies on its
 * own: the queue never auto-promotes, the accept = a named reviewer's receipt
 * (a documented mention claim with its passage evidence), and a dismissal
 * hides the card for the dismissing reviewer only.
 */
function TribeHintQueue({ tribeID, canMerge }: { tribeID: string; canMerge: boolean }) {
  const queryClient = useQueryClient();
  const [message, setMessage] = useState("");
  const hints = useQuery({
    queryKey: ["tribe-hints", tribeID],
    queryFn: ({ signal }) => fetchTribeHints(tribeID, "suggested", signal),
  });
  const decide = useMutation({
    mutationFn: (input: { sourceId: string; action: "accept" | "dismiss" }) =>
      decideTribeHint(tribeID, input.sourceId, input.action),
    onSuccess: (result) => {
      setMessage(result.action === "accept"
        ? `أصبحت الإشارات موثقة بعد: سجل الشواهد يحملها الآن (${result.createdReceipts} مقطعاً).`
        : "أخفيتم البطاقة عن لوحتكم؛ تظهر لبقية المراجعين.");
      void queryClient.invalidateQueries({ queryKey: ["tribe-hints", tribeID] });
      void queryClient.invalidateQueries({ queryKey: ["tribe-evidence", tribeID] });
      void queryClient.invalidateQueries({ queryKey: ["dictionary-detail"] });
    },
    onError: (error) => setMessage(dictionaryErrorMessage(error)),
  });

  const cards = (hints.data?.hints ?? []).filter((card) => card.tribeId === tribeID);
  if (!canMerge) return null;
  return (
    <section className="dictionary-hints" aria-label="إرشادات الاستيراد">
      <div className="detail-label">
        إرشادات الاستيراد
        <span className="dictionary-receipt-caption">إشارات المصادر المقروءة لا تطبَّق بنفسها: القبول ثبوت، والرفض إخفاء عن هويتك.</span>
      </div>
      {message ? <div className="question-workspace-message" role="status">{message}</div> : null}
      {hints.isPending ? <p className="dictionary-related-empty">يُقرأ طابور التلميحات…</p> : null}
      {hints.isError ? (
        <div className="dictionary-error" role="alert">{dictionaryError(hints.error)} <button className="inline-link" type="button" onClick={() => void hints.refetch()}>إعادة المحاولة</button></div>
      ) : null}
      {cards.length === 0 ? <p className="dictionary-related-empty">لا إشارات مقترحة لهذه القبيلة في المصادر المستوردة بعد.</p> : null}
      <ul className="tribe-hint-cards">
        {cards.map((card) => (
          <li key={card.sourceId} className="tribe-hint-card">
            <div>
              <strong>{card.mentions} إشارة إلى {card.tribeName}</strong>
              <small>في «{card.sourceTitle}» — {card.passageCount} {card.passageCount === 1 ? "مقطعاً" : "مقاطع"} · بمصادر مقروءة لكم؛ القبول سجلُّ إثبات، لا استنتاجٌ انتماء.</small>
            </div>
            <div className="tribe-hint-actions">
              <button type="button" className="text-button" onClick={() => decide.mutate({ sourceId: card.sourceId, action: "accept" })} disabled={decide.isPending}>
                ثبِّت الإشارات
              </button>
              <button type="button" className="inactive" onClick={() => decide.mutate({ sourceId: card.sourceId, action: "dismiss" })} disabled={decide.isPending}>
                أخفِ البطاقة
              </button>
            </div>
          </li>
        ))}
      </ul>
    </section>
  );
}

/**
 * المِسلك (plan 031-E): the tribe-duplicate pairs this tribe stands in, as a
 * moderator band. A candidate carries its measured similarity and BOTH matched
 * forms verbatim; the reviewer's effects are exactly three — link the loser's
 * form as the canonical's alias, point the loser at the canonical, reject —
 * and NONE of them writes a tribes row (plan 005's no-merge deferral). The
 * canonical picker is the card's own radio: the reviewer says which side is
 * the original; the lane's undo is data, not surgery.
 */
function TribeDuplicates({ tribeID, tribeName, canMerge }: { tribeID: string; tribeName: string; canMerge: boolean }) {
  const queryClient = useQueryClient();
  const [choice, setChoice] = useState<Record<string, string>>({});
  const [message, setMessage] = useState("");
  const candidates = useQuery({
    queryKey: ["tribe-duplicates", tribeID],
    queryFn: ({ signal }) => fetchTribeDuplicateCandidates("suggested", signal),
  });
  const decide = useMutation({
    mutationFn: (input: { candidateID: string; action: "link_alias" | "canonize" | "reject"; canonical?: string }) =>
      decideTribeDuplicate(input.candidateID, { action: input.action, canonical: input.canonical, noteAr: "" }),
    onSuccess: (result) => {
      const labels: Record<string, string> = { link_alias: "رُبط اسم أسير مغيرًا للقانون", canonize: "وَجَّهت المرجعية إلى الأصل", reject: "رُفض الاقتراح، والصفوف كما كانت" };
      setMessage(labels[result.action] ?? "سُجل القرار.");
      void queryClient.invalidateQueries({ queryKey: ["tribe-duplicates", tribeID] });
      void queryClient.invalidateQueries({ queryKey: ["dictionary-detail"] });
      void queryClient.invalidateQueries({ queryKey: ["tribe-evidence", tribeID] });
    },
    onError: (error) => setMessage(dictionaryErrorMessage(error)),
  });

  const related = (candidates.data?.candidates ?? []).filter(
    (candidate) => candidate.tribeAId === tribeID || candidate.tribeBId === tribeID,
  );
  if (!canMerge || (candidates.isError && related.length === 0)) return null;
  return (
    <section className="dictionary-duplicates" aria-label="مِسلك التكرار">
      <div className="detail-label">
        مِسلك التكرار
        <span className="dictionary-receipt-caption">اسم مشابه = عبور لا إغلاق: لا دمج بقبيلة هنا؛ الاسم يُربط أو يُوَجَّه، والصفوف تبقى.</span>
      </div>
      {message ? <div className="question-workspace-message" role="status">{message}</div> : null}
      {candidates.isPending ? <p className="dictionary-related-empty">يُقرأ المِسلك…</p> : null}
      {candidates.isError ? (
        <div className="dictionary-error" role="alert">{dictionaryError(candidates.error)} <button className="inline-link" type="button" onClick={() => void candidates.refetch()}>إعادة المحاولة</button></div>
      ) : null}
      {related.length === 0 ? <p className="dictionary-related-empty">لا اقتراحات تكرار لهذه القبيلة.</p> : null}
      <ul className="tribe-duplicate-cards">
        {related.map((candidate) => {
          const otherID = candidate.tribeAId === tribeID ? candidate.tribeBId : candidate.tribeAId;
          const picked = choice[candidate.id] ?? otherID;
          return (
            <li key={candidate.id} className="tribe-duplicate-card">
              <div>
                <strong>«{candidate.matchedA}» ⇄ «{candidate.matchedB}»</strong>
                <small>شبهة {Math.round(candidate.nameSimilarity * 100)}٪ — الأشبه: مقطارٌ محسوب لا حُكم.</small>
                <div className="tribe-duplicate-choice" role="radiogroup" aria-label="المرجعية">
                  {[candidate.tribeAName, candidate.tribeBName].map((name, index) => {
                    const id = index === 0 ? candidate.tribeAId : candidate.tribeBId;
                    return (
                      <label key={id}>
                        <input type="radio" name={`canonical-${candidate.id}`} checked={picked === id} onChange={() => setChoice((state) => ({ ...state, [candidate.id]: id }))} />
                        {name === tribeName ? "هذه القبيلة" : name}
                      </label>
                    );
                  })}
                  <small>المرجعية ← الآخر يصير ظلها</small>
                </div>
              </div>
              <div className="tribe-hint-actions">
                <button type="button" className="text-button" disabled={decide.isPending} onClick={() => decide.mutate({ candidateID: candidate.id, action: "link_alias", canonical: picked })}>اربط الاسم</button>
                <button type="button" className="text-button" disabled={decide.isPending} onClick={() => decide.mutate({ candidateID: candidate.id, action: "canonize", canonical: picked })}>وَجِّه المرجعية</button>
                <button type="button" className="inactive" disabled={decide.isPending} onClick={() => decide.mutate({ candidateID: candidate.id, action: "reject" })}>رفض الاقتراح</button>
              </div>
            </li>
          );
        })}
      </ul>
    </section>
  );
}
