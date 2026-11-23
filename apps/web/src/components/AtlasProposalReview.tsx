import { useInfiniteQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { decideAtlasProposal, fetchAtlasProposals, importAtlasProposals, type AtlasReadingProposal, type AtlasSourceOccurrence } from "../lib/api";

export function AtlasProposalImport({ row }: { row: AtlasSourceOccurrence }) {
  const client = useQueryClient();
  const importer = useMutation({
    mutationFn: async (file: File) => {
      if (file.size > 1_000_000) throw new Error("حجم الملف أكبر من الحد المسموح.");
      const body = await file.text();
      const stage = JSON.parse(body);
      if (stage?.itemKey !== row.itemKey) throw new Error("الملف يخص عنصر مصدر مختلفًا.");
      return importAtlasProposals(row.sourceId, body);
    },
    onSuccess: () => client.invalidateQueries({ queryKey: ["atlas-proposals"] }),
  });
  return (
    <details>
      <summary>استيراد قراءات للمراجعة</summary>
      <p>عنصر المصدر <bdi>{row.itemKey}</bdi></p>
      <label>
        ملف القراءات المقترحة (JSON)
        <input type="file" accept=".json,application/json" disabled={importer.isPending} onChange={(event) => {
          const file = event.currentTarget.files?.[0];
          event.currentTarget.value = "";
          if (file) importer.mutate(file);
        }} />
      </label>
      {importer.isPending && <p role="status">يُستورد الملف…</p>}
      {importer.isSuccess && <p role="status">{importer.data.added} قراءة جديدة بانتظار المراجعة.</p>}
      {importer.isError && <p role="alert">{importer.error.message}</p>}
    </details>
  );
}

export function AtlasProposalReview() {
  const [open, setOpen] = useState(false);
  const queue = useInfiniteQuery({
    queryKey: ["atlas-proposals"],
    queryFn: ({ signal, pageParam }) => fetchAtlasProposals(pageParam, signal),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next,
    enabled: open,
  });
  const rows = queue.data?.pages.flatMap((page) => page.proposals) ?? [];
  return (
    <details className="atlas-source-search" onToggle={(event) => setOpen(event.currentTarget.open)}>
      <summary>مراجعة قراءات المصادر</summary>
      <div className="atlas-drawer-body">
        {open && queue.isPending && <p className="atlas-drawer-status" role="status">تُقرأ قائمة المراجعة…</p>}
        {queue.isError && <button className="text-button" type="button" onClick={() => void queue.refetch()}>تعذّر فتح القائمة · أعد المحاولة</button>}
        {queue.isSuccess && !rows.length && (
          <div className="atlas-drawer-empty">
            <strong>لا قراءات بانتظار المراجعة.</strong>
            <span>تظهر هنا القراءات المستخرجة بعد رفع ملف مقترحات JSON من «استيراد قراءات للمراجعة» داخل أي شاهد مصدر.</span>
          </div>
        )}
        {rows.length > 0 && (
          <div className="atlas-source-comparison">
            {rows.map((row) => <ProposalCard key={row.id} row={row} />)}
          </div>
        )}
        {queue.hasNextPage && <button type="button" className="text-button" disabled={queue.isFetchingNextPage} onClick={() => void queue.fetchNextPage()}>مزيد من القراءات</button>}
      </div>
    </details>
  );
}

function ProposalCard({ row }: { row: AtlasReadingProposal }) {
  const client = useQueryClient();
  const [original, setOriginal] = useState(row.raw.original.slice(0, 500));
  const [displayAr, setDisplay] = useState("");
  const [language, setLanguage] = useState("und");
  const [noteAr, setNote] = useState("");
  const decision = useMutation({
    mutationFn: (action: "publish" | "reject") => decideAtlasProposal(row.id, { action, original, displayAr, language, noteAr }),
    onSuccess: () => Promise.all([
      client.invalidateQueries({ queryKey: ["atlas-proposals"] }),
      client.invalidateQueries({ queryKey: ["atlas-occurrences"] }),
    ]),
  });
  return (
    <article className="tribe-receipt">
      <h3>{row.titleAr}</h3>
      <a href={row.url} target="_blank" rel="noreferrer">{row.locatorAr}</a>
      <small><bdi>{row.engine}</bdi></small>
      <details>
        <summary>القراءة الأولية ومحدد الموضع</summary>
        <blockquote dir="auto">{row.raw.original}</blockquote>
        <code dir="ltr">{JSON.stringify(row.raw.selector)}</code>
        <small><bdi>{row.sha256}</bdi></small>
      </details>
      <form className="atlas-source-query" onSubmit={(event) => { event.preventDefault(); decision.mutate("publish"); }}>
        <label>النص كما يظهر في المصدر <textarea dir="auto" required maxLength={500} value={original} onChange={(event) => setOriginal(event.target.value)} /></label>
        <label>الاسم المعروض بالعربية <input required maxLength={200} value={displayAr} onChange={(event) => setDisplay(event.target.value)} /></label>
        <label>لغة النص الأصلي
          <select value={language} onChange={(event) => setLanguage(event.target.value)}>
            <option value="und">غير محددة</option><option value="ar">العربية</option><option value="en">الإنجليزية</option>
          </select>
        </label>
        <label>سبب القرار <textarea required maxLength={1000} value={noteAr} onChange={(event) => setNote(event.target.value)} /></label>
        <button type="submit" className="secondary-button" disabled={decision.isPending}>انشر القراءة</button>
        <button type="button" className="text-button" disabled={!noteAr.trim() || decision.isPending} onClick={() => decision.mutate("reject")}>ارفض القراءة</button>
        {decision.isError && <p role="alert">{decision.error.message}</p>}
      </form>
    </article>
  );
}
