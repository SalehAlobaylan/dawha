import { AtlasProposalImport } from "./AtlasProposalReview";
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import { attachAtlasClaimReceipt, fetchAtlasOccurrences, fetchAtlasSourceRelationship, fetchDictionaryIndex, reviewAtlasOccurrenceIdentity, type AtlasSourceOccurrence } from "../lib/api";
import { useDebouncedScalar } from "../lib/useDebounced";

type Props = {
  query?: string;
  tribe?: string;
  first?: string;
  second?: string;
  from?: number;
  to?: number;
  canModerate?: boolean;
  select?: (first?: string, second?: string) => void;
};

export function AtlasSourceSearch({ query, tribe, first, second, from, to, select, canModerate }: Props) {
  const [draft, setDraft] = useState(query ?? "");
  useEffect(() => setDraft(query ?? ""), [query]);
  const [undated, setUndated] = useState(false);
  const q = useDebouncedScalar(draft.trim(), 250);
  const dated = from !== undefined || to !== undefined;
  const enabled = Boolean(q || tribe);
  const results = useInfiniteQuery({
    queryKey: ["atlas-occurrences", q, tribe, from, to, undated],
    initialPageParam: undefined as string | undefined,
    queryFn: ({ signal, pageParam }) => fetchAtlasOccurrences({ q, tribe, after: pageParam, from, to, undated }, signal),
    getNextPageParam: (lastPage) => lastPage.next,
    enabled,
  });
  const rows = results.data?.pages.flatMap((page) => page.occurrences) ?? [];
  const quickExamples = ["مطير", "بريه", "عنزة", "حرب", "بنو خالد"];
  return (
    <details className="atlas-source-search" open={enabled || Boolean(first) || undefined}>
      <summary>البحث في جميع المصادر</summary>
      <div className="atlas-drawer-body">
        {!tribe && (
          <div className="atlas-source-search-controls">
            <label className="atlas-source-query">
              <span>اسم في المصادر</span>
              <span className="atlas-source-input-row">
                <input
                  value={draft}
                  maxLength={120}
                  placeholder="مثال: مطير، بريه، بنو ريام، Mutair…"
                  onChange={(event) => setDraft(event.target.value)}
                />
                {draft ? (
                  <button type="button" className="text-button atlas-source-clear" onClick={() => setDraft("")} aria-label="مسح البحث">
                    مسح
                  </button>
                ) : null}
              </span>
            </label>
            {!draft ? (
              <div className="atlas-source-chips" aria-label="أمثلة بحث سريعة">
                <small>جرّب:</small>
                {quickExamples.map((term) => (
                  <button key={term} type="button" className="atlas-source-chip" onClick={() => setDraft(term)}>
                    {term}
                  </button>
                ))}
              </div>
            ) : null}
          </div>
        )}
        {dated && (
          <label className="atlas-source-undated-check">
            <input type="checkbox" checked={undated} onChange={(event) => setUndated(event.target.checked)} />
            <span>إظهار المصادر مجهولة الفترة</span>
          </label>
        )}
        {enabled && results.isPending && <p className="atlas-drawer-status" role="status">يُبحث في المصادر…</p>}
        {results.isError && <button type="button" className="secondary-button" onClick={() => void results.refetch()}>تعذر البحث · إعادة المحاولة</button>}
        {results.isSuccess && rows.length === 0 && (
          <div className="atlas-drawer-empty">
            <strong>لا شواهد مطابقة.</strong>
            <span>جرّب صيغة أخرى بالعربية أو الإنجليزية (مثل «مطير» أو «Mutair»).</span>
          </div>
        )}
        {rows.length > 0 && (
          <ul className="atlas-source-results">
            {rows.map((row) => (
              <li key={row.id}>
                {select ? (
                  <>
                    <button type="button" className="text-button atlas-source-result-btn" onClick={() => select(row.id)}>
                      <strong>{row.displayAr}</strong>
                      <span>{row.titleAr} · {row.locatorAr}</span>
                    </button>
                    {first && first !== row.id && <button type="button" className="secondary-button" onClick={() => select(first, row.id)}>قارن مع هذا المصدر</button>}
                  </>
                ) : (
                  <Link to="/places" search={{ occurrence: row.id }}>{row.displayAr} · {row.titleAr} · {row.locatorAr}</Link>
                )}
              </li>
            ))}
          </ul>
        )}
        {results.hasNextPage && (
          <button type="button" className="secondary-button" disabled={results.isFetchingNextPage} onClick={() => void results.fetchNextPage()}>مزيد من الشواهد</button>
        )}
        {first && (
          <section className="atlas-source-detail-wrap" aria-label={second ? "مقارنة مصدرين" : "شاهد من المصدر"}>
            <div className="atlas-source-detail-head">
              <strong>{second ? "مقارنة مصدرين" : "شاهد من المصدر"}</strong>
              <button type="button" className="text-button" onClick={() => select?.()}>إغلاق الشاهد</button>
            </div>
            {second && second !== first && <SourceRelationship first={first} second={second} />}
            <div className="atlas-source-comparison">
              <OccurrenceDetail id={first} canModerate={canModerate} />
              {second && second !== first && <OccurrenceDetail id={second} canModerate={canModerate} />}
            </div>
          </section>
        )}
      </div>
    </details>
  );
}

export function AtlasOccurrenceOverlay({
  first,
  second,
  canModerate,
  onClose,
  floating = false,
}: {
  first: string;
  second?: string;
  canModerate?: boolean;
  onClose: () => void;
  floating?: boolean;
}) {
  return (
    <aside
      className={floating ? "plate-caption-panel plate-caption-panel-floating atlas-occurrence-overlay" : "atlas-inspector plate-caption-panel"}
      aria-label={second ? "مقارنة مصدرين" : "شاهد من المصدر"}
    >
      <div className="atlas-panel-head plate-caption-head">
        <div>
          <div className="eyebrow">أرشيف المصادر</div>
          <h2>{second ? "مقارنة مصدرين" : "شاهد من المصدر"}</h2>
        </div>
        <button type="button" className="text-button" onClick={onClose}>إغلاق الشاهد</button>
      </div>
      {second && second !== first && <SourceRelationship first={first} second={second} />}
      <div className="atlas-source-comparison">
        <OccurrenceDetail id={first} canModerate={canModerate} />
        {second && second !== first && <OccurrenceDetail id={second} canModerate={canModerate} />}
      </div>
    </aside>
  );
}

function useOccurrence(id: string) {
  return useQuery({
    queryKey: ["atlas-occurrence", id],
    queryFn: ({ signal }) => fetchAtlasOccurrences({ id }, signal),
  });
}

function SourceRelationship({ first, second }: { first: string; second: string }) {
  const a = useOccurrence(first).data?.occurrences[0]?.sourceId;
  const b = useOccurrence(second).data?.occurrences[0]?.sourceId;
  const relationship = useQuery({
    queryKey: ["atlas-source-relationship", a, b],
    queryFn: ({ signal }) => fetchAtlasSourceRelationship(a!, b!, signal),
    enabled: Boolean(a && b && a !== b),
  });
  if (!a || !b) return null;
  let status = "استقلال المصدرين غير محقّق.";
  if (a === b) status = "شاهدان من المصدر نفسه.";
  else if (relationship.isPending) status = "تُفحص صلة المصدرين…";
  else if (relationship.data?.status === "needs_review") status = "صلة المصدرين بانتظار المراجعة.";
  else if (relationship.data?.status === "confirmed") {
    status = relationship.data.kind === "shared_origin" || relationship.data.kind === "derived_from"
      ? "توجد صلة موثّقة في أصل المصدرين."
      : "توجد إحالة أو صلة مسجّلة بين المصدرين. استقلال الشاهدين غير محقّق.";
  }
  return <p className="atlas-source-relationship" role="status">{status} <Link to="/sources" search={{ sourceId: a }}>سجل المصدر</Link></p>;
}

function OccurrenceDetail({ id, canModerate }: { id: string; canModerate?: boolean }) {
  const query = useOccurrence(id);
  if (query.isPending) return <p role="status">يُقرأ الشاهد…</p>;
  if (query.isError) return <p role="alert">تعذر فتح الشاهد. <button type="button" onClick={() => void query.refetch()}>إعادة المحاولة</button></p>;
  const row = query.data.occurrences[0];
  return row ? <OccurrenceCard row={row} canModerate={canModerate} /> : null;
}

function OccurrenceCard({ row, canModerate }: { row: AtlasSourceOccurrence; canModerate?: boolean }) {
  const period = row.periodAr || (row.depictedFrom === row.depictedTo
    ? row.depictedFrom
    : [row.depictedFrom, row.depictedTo].filter((year) => year !== undefined).join("–"));
  return (
    <article className="tribe-receipt">
      <h3>{row.original === row.displayAr ? row.original : row.displayAr}</h3>
      {row.original !== row.displayAr && <p><bdi lang={row.language}>{row.original}</bdi></p>}
      <p>{row.titleAr} · {row.locatorAr}</p>
      {row.sourceSelector?.figure && row.sourceSelector.legend && (
        <p>رمز الخريطة {row.sourceSelector.figure.symbol} · صف المفتاح {row.sourceSelector.legend.row}</p>
      )}
      {row.sourceSelector?.start && row.sourceSelector.end && (
        <p>المقطع <bdi>{row.sourceSelector.start}–{row.sourceSelector.end}</bdi></p>
      )}
      <p>{period ? `الفترة التي يصفها المصدر · ${period}` : "الفترة التي يصفها المصدر غير محددة"}</p>
      {row.publicationYear && <p>سنة النشر · {row.publicationYear}</p>}
      {row.scanSha256 && !row.plateKey && <small>نسخة المصدر · <bdi>{row.scanSha256.slice(0, 12)}</bdi></small>}
      {row.method === "catalogue" && <p>قراءة من الفهرس</p>}
      {row.excerptAr && <blockquote>{row.excerptAr}</blockquote>}
      {row.noteAr && <p>{row.noteAr}</p>}
      {row.creditAr && <small>{row.creditAr}</small>}
      {row.rightsAr && <small>{row.rightsAr}</small>}
      <div className="atlas-occurrence-actions">
        {row.url
          ? <a className="secondary-button" href={row.url} target="_blank" rel="noreferrer">افتح المصدر عند الشاهد</a>
          : <Link className="secondary-button" to="/sources" search={{ sourceId: row.sourceId }}>افتح سجل المصدر</Link>}
        {row.plateKey && row.caption && (
          <Link className="secondary-button" to="/places" search={{ view: "plate", plates: row.plateKey, caption: row.caption }}>اعرض الاسم على اللوحة</Link>
        )}
        {row.placeId && <Link className="secondary-button" to="/places" search={{ place: row.placeId }}>افتح الموضع في الأطلس</Link>}
        {row.branchId && row.branchName && <Link className="secondary-button" to="/search" search={{ kind: "branches", q: row.branchName }}>ابحث عن الفرع</Link>}
        {row.tribeId && <Link className="secondary-button" to="/search" search={{ kind: "tribes", id: row.tribeId }}>افتح سجل القبيلة</Link>}
      </div>
      {canModerate && row.itemKey && (
        <details>
          <summary>معرّفات إسناد الشاهد</summary>
          <p>المصدر <bdi>{row.sourceId}</bdi></p>
          <p>الشاهد <bdi>{row.id}</bdi></p>
        </details>
      )}
      {canModerate && row.itemKey && <IdentityReview key={row.id} row={row} />}
      {canModerate && row.itemKey && <AtlasProposalImport row={row} />}
      {canModerate && row.itemKey && <ClaimReceipt occurrenceId={row.id} />}
    </article>
  );
}

function IdentityReview({ row }: { row: AtlasSourceOccurrence }) {
  const client = useQueryClient();
  const [open, setOpen] = useState(false);
  const [term, setTerm] = useState(row.displayAr);
  const [kind, setKind] = useState<"tribes" | "places" | "branches">(row.placeId ? "places" : row.branchId ? "branches" : "tribes");
  const [targetId, setTargetId] = useState(row.tribeId ?? row.placeId ?? row.branchId ?? "");
  const [noteAr, setNoteAr] = useState("");
  const needle = useDebouncedScalar(term.trim(), 250);
  const candidates = useQuery({
    queryKey: ["atlas-identity-candidates", kind, needle],
    queryFn: () => fetchDictionaryIndex(kind, needle),
    enabled: open && Boolean(needle),
  });
  const review = useMutation({
    mutationFn: () => reviewAtlasOccurrenceIdentity(row.id, {
      tribeId: kind === "tribes" ? targetId : undefined,
      placeId: kind === "places" ? targetId : undefined,
      branchId: kind === "branches" ? targetId : undefined,
      noteAr,
    }),
    onSuccess: async () => {
      await Promise.all([
        client.invalidateQueries({ queryKey: ["atlas-occurrence", row.id] }),
        client.invalidateQueries({ queryKey: ["atlas-occurrences"] }),
      ]);
      setNoteAr("");
      setOpen(false);
    },
  });
  return (
    <details open={open} onToggle={(event) => setOpen(event.currentTarget.open)}>
      <summary>مراجعة ربط الاسم</summary>
      {open && (
        <form className="atlas-source-query" onSubmit={(event) => { event.preventDefault(); review.mutate(); }}>
          <label>نوع السجل
            <select value={kind} onChange={(event) => {
              setKind(event.target.value as typeof kind);
              setTargetId("");
            }}>
              <option value="tribes">قبيلة</option>
              <option value="branches">فرع</option>
              <option value="places">موضع</option>
            </select>
          </label>
          <label>ابحث في الفهرس <input value={term} onChange={(event) => setTerm(event.target.value)} /></label>
          <label>السجل المقترح
            <select value={targetId} onChange={(event) => setTargetId(event.target.value)}>
              <option value="">اترك الهوية معلّقة</option>
              {targetId && !candidates.data?.items.some((item) => item.id === targetId) && <option value={targetId}>السجل المختار</option>}
              {candidates.data?.items.map((item) => <option value={item.id} key={item.id}>{item.nameAr}{item.secondaryAr ? ` · ${item.secondaryAr}` : ""}</option>)}
            </select>
          </label>
          {candidates.isError && <p role="alert">تعذر البحث في الفهرس.</p>}
          <label>سبب القرار <textarea required maxLength={1000} value={noteAr} onChange={(event) => setNoteAr(event.target.value)} /></label>
          <button className="secondary-button" type="submit" disabled={review.isPending || !noteAr.trim()}>احفظ قرار الربط</button>
          {review.isError && <p role="alert">تعذر حفظ القرار.</p>}
        </form>
      )}
    </details>
  );
}

function ClaimReceipt({ occurrenceId }: { occurrenceId: string }) {
  const attach = useMutation({
    mutationFn: (form: FormData) => attachAtlasClaimReceipt(
      occurrenceId, String(form.get("claimId") ?? "").trim(), String(form.get("note") ?? "").trim(),
    ),
  });
  return (
    <details>
      <summary>أضف الشاهد إلى ادعاء</summary>
      <p>يُسجَّل الشاهد بوصفه سياقًا للادعاء القائم.</p>
      <form className="atlas-source-query" onSubmit={(event) => { event.preventDefault(); attach.mutate(new FormData(event.currentTarget)); }}>
        <label>معرّف الادعاء <input name="claimId" required dir="ltr" maxLength={36} /></label>
        <label>وجه الصلة <textarea name="note" required maxLength={1000} /></label>
        <button type="submit" className="secondary-button" disabled={attach.isPending}>أرفق الشاهد</button>
      </form>
      {attach.isSuccess && <p role="status">أُرفق الشاهد.</p>}
      {attach.isError && <p role="alert">تعذّر إرفاق الشاهد. تحقق من المصدر ومعرّف الادعاء.</p>}
    </details>
  );
}
